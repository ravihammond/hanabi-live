package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	gsessions "github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	researchDeckColors = 5
	researchDeckRanks  = 5
)

var (
	researchDeckCountsByRank                  = []int{3, 2, 2, 2, 1}
	researchJAXMARLColorToHanabiLiveSuitIndex = []int{0, 1, 2, 4, 3}
	researchSessions                          = make(map[string]*ResearchSession)
	researchJoinTokens                        = make(map[string]*ResearchJoinToken)
	researchGuestUsers                        = make(map[int]*ResearchJoinToken)
	researchSessionsMutex                     sync.Mutex
)

type ResearchCardIdentity struct {
	Color int `json:"color"`
	Rank  int `json:"rank"`
}

type ResearchSeededInitialLayout struct {
	DeckOrder            []ResearchCardIdentity `json:"deck_order"`
	SeatOrder            []int                  `json:"seat_order"`
	RosterPlayerToSeatID map[string]string      `json:"roster_player_to_seat_id"`
}

type ResearchGamePayload struct {
	Seed            int    `json:"seed"`
	GameIndex       int    `json:"game_index"`
	GameSeed        *int   `json:"game_seed"`
	IdentityDisplay string `json:"identity_display"`
	ChatEnabled     bool   `json:"chat_enabled"`
}

type ResearchRosterPlayer struct {
	RosterIndex    int    `json:"roster_index"`
	RosterPlayerID string `json:"roster_player_id"`
	Type           string `json:"type"`
	Location       string `json:"location,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	ModelPath      string `json:"model_path,omitempty"`
}

type ResearchCreatePayload struct {
	Mode                string                      `json:"mode"`
	Game                ResearchGamePayload         `json:"game"`
	RosterPlayers       []ResearchRosterPlayer      `json:"roster_players"`
	SeededInitialLayout ResearchSeededInitialLayout `json:"seeded_initial_layout"`
}

type CreatedResearchSingleGame struct {
	TableID      uint64            `json:"table_id"`
	GameID       string            `json:"game_id"`
	Mode         string            `json:"mode"`
	GameSeed     int               `json:"game_seed"`
	LayoutSource string            `json:"layout_source"`
	SeatOrder    []int             `json:"seat_order"`
	JoinLinks    map[string]string `json:"join_links"`
}

type CreatedResearchPregameTable struct {
	TableID          string            `json:"table_id"`
	Mode             string            `json:"mode"`
	Seed             int               `json:"seed"`
	CurrentGameIndex int               `json:"current_game_index"`
	JoinLinks        map[string]string `json:"join_links"`
	ReadyStatus      map[string]bool   `json:"ready_status"`
	UsesPublicLobby  bool              `json:"uses_public_lobby"`
}

type ResearchBotJoinSessionPayload struct {
	RosterPlayerID string `json:"roster_player_id"`
}

type CreatedResearchBotJoinSession struct {
	GameID         string `json:"game_id"`
	RosterPlayerID string `json:"roster_player_id"`
	JoinCredential string `json:"join_credential"`
	ServerURL      string `json:"server_url"`
	OurPlayerIndex int    `json:"our_player_index"`
}

type OpenedResearchReplay struct {
	GameID       string `json:"game_id"`
	ReplayURL    string `json:"replay_url"`
	LayoutSource string `json:"layout_source"`
	SeatOrder    []int  `json:"seat_order"`
}

type ResearchSession struct {
	GameID                  string
	TableID                 uint64
	Mode                    string
	Seed                    int
	CurrentGameIndex        int
	ReadyStatus             map[string]bool
	CompletedGames          []map[string]interface{}
	SeatOrder               []int
	RosterPlayerToSeatID    map[string]string
	RosterPlayerIDsBySeat   []string
	RosterPlayerNamesBySeat []string
	BotRosterPlayerIDs      map[string]bool
}

type ResearchJoinToken struct {
	Token          string
	GameID         string
	TableID        uint64
	RosterPlayerID string
	RosterIndex    int
	SeatIndex      int
	UserID         int
	Username       string
}

type ResearchBotActionPayload struct {
	RosterPlayerID string `json:"roster_player_id"`
	Action         string `json:"action"`
}

type ResearchBotAction struct {
	Type   int `json:"type"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

type validatedResearchLayout struct {
	deckOrder            []*CardIdentity
	seatOrder            []int
	rosterPlayerToSeatID map[string]string
}

func registerResearchRoutes(router *gin.Engine) {
	router.GET("/health", researchHealth)
	router.POST("/research/single-game", researchCreateSingleGame)
	router.POST("/research/pregame-table", researchCreatePregameTable)
	router.POST("/research/replay/open", researchOpenReplay)
	router.POST("/research/sessions/:gameID/current-game-layout", researchUpdateCurrentGameLayout)
	router.GET("/research/sessions/:gameID/status", researchGetSessionStatus)
	router.POST("/research/sessions/:gameID/bot-action", researchPostBotAction)
	router.POST("/research/sessions/:gameID/bot-join-session", researchCreateBotJoinSession)
}

func registerResearchPublicRoutes(router *gin.Engine) {
	router.GET("/join/:token", researchMagicJoin)
	router.GET("/research/tunnel-ws-health", researchTunnelWebSocketHealth)
}

func researchHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func researchTunnelWebSocketHealth(c *gin.Context) {
	upgrader := websocket.Upgrader{ // nolint: exhaustivestruct
		CheckOrigin: researchTrustedTunnelOrigin,
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ok")); err != nil {
		return
	}
}

func researchTrustedTunnelOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	if host == researchRequestHostname(r.Host) {
		return true
	}
	return strings.HasSuffix(host, ".trycloudflare.com")
}

func researchRequestHostname(host string) string {
	hostname, _, err := net.SplitHostPort(host)
	if err == nil {
		return hostname
	}
	return host
}

func researchCreateSingleGame(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	var payload ResearchCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if payload.Mode != "single_game" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Control API only creates single_game sessions in this route."})
		return
	}

	layout, err := validateResearchPayload(payload)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	table, err := createResearchSingleGameTable(payload, layout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	gameSeed := researchGameSeed(payload.Game)
	gameID := fmt.Sprintf("single_game_%d", gameSeed)
	rosterPlayerIDsBySeat := researchRosterPlayerIDsBySeat(payload.RosterPlayers, layout.seatOrder)
	rosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(payload.RosterPlayers, layout.seatOrder, payload.Game.IdentityDisplay)
	botRosterPlayerIDs := researchBotRosterPlayerIDs(payload.RosterPlayers)
	joinLinks := researchRegisterJoinLinks(gameID, table.ID, payload, layout)
	created := CreatedResearchSingleGame{
		TableID:      table.ID,
		GameID:       gameID,
		Mode:         "single_game",
		GameSeed:     gameSeed,
		LayoutSource: "payload",
		SeatOrder:    append([]int(nil), layout.seatOrder...),
		JoinLinks:    joinLinks,
	}
	researchSessionsMutex.Lock()
	researchSessions[gameID] = &ResearchSession{
		GameID:                  gameID,
		TableID:                 table.ID,
		Mode:                    "single_game",
		SeatOrder:               append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID:    copyStringMap(layout.rosterPlayerToSeatID),
		RosterPlayerIDsBySeat:   rosterPlayerIDsBySeat,
		RosterPlayerNamesBySeat: rosterPlayerNamesBySeat,
		BotRosterPlayerIDs:      botRosterPlayerIDs,
	}
	researchSessionsMutex.Unlock()
	c.JSON(http.StatusCreated, created)
}

func researchCreatePregameTable(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	var payload ResearchCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if payload.Mode != "pregame_table" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Control API only creates pregame_table sessions in this route."})
		return
	}
	if payload.Game.GameIndex != 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Pregame Table current game_index must start at zero."})
		return
	}
	layout, err := validateResearchPayload(payload)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	rosterPlayerIDsBySeat := researchRosterPlayerIDsBySeat(payload.RosterPlayers, layout.seatOrder)
	rosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(payload.RosterPlayers, layout.seatOrder, payload.Game.IdentityDisplay)
	botRosterPlayerIDs := researchBotRosterPlayerIDs(payload.RosterPlayers)
	table, err := createResearchSingleGameTable(payload, layout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	tableID := fmt.Sprintf("pregame_table_%d", payload.Game.Seed)
	readyStatus := make(map[string]bool)
	for _, player := range payload.RosterPlayers {
		readyStatus[player.RosterPlayerID] = player.Type == "bot"
	}
	created := CreatedResearchPregameTable{
		TableID:          tableID,
		Mode:             "pregame_table",
		Seed:             payload.Game.Seed,
		CurrentGameIndex: 0,
		JoinLinks:        researchRegisterJoinLinks(tableID, table.ID, payload, layout),
		ReadyStatus:      readyStatus,
		UsesPublicLobby:  false,
	}

	researchSessionsMutex.Lock()
	researchSessions[tableID] = &ResearchSession{
		GameID:                  tableID,
		TableID:                 table.ID,
		Mode:                    "pregame_table",
		Seed:                    payload.Game.Seed,
		CurrentGameIndex:        0,
		ReadyStatus:             readyStatus,
		CompletedGames:          make([]map[string]interface{}, 0),
		SeatOrder:               append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID:    copyStringMap(layout.rosterPlayerToSeatID),
		RosterPlayerIDsBySeat:   rosterPlayerIDsBySeat,
		RosterPlayerNamesBySeat: rosterPlayerNamesBySeat,
		BotRosterPlayerIDs:      botRosterPlayerIDs,
	}
	researchSessionsMutex.Unlock()
	c.JSON(http.StatusCreated, created)
}

func researchOpenReplay(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	var replay map[string]interface{}
	if err := c.ShouldBindJSON(&replay); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	rawLayout, ok := replay["seeded_initial_layout"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Saved Replay must include seeded_initial_layout."})
		return
	}
	layout, err := researchLayoutFromInterface(rawLayout)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	playersBySeat, ok := replay["players_by_seat"].([]interface{})
	if !ok || len(playersBySeat) < 2 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Saved Replay must include players_by_seat."})
		return
	}
	if _, err := validateResearchLayout(layout, len(playersBySeat)); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	gameID := "replay"
	if rawGameID, ok := replay["game_id"].(string); ok && rawGameID != "" {
		gameID = rawGameID
	}
	c.JSON(http.StatusCreated, OpenedResearchReplay{
		GameID:       gameID,
		ReplayURL:    researchPublicBaseURL() + "/replay/" + gameID,
		LayoutSource: "saved_replay",
		SeatOrder:    append([]int(nil), layout.SeatOrder...),
	})
}

func researchUpdateCurrentGameLayout(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	gameID := c.Param("gameID")
	var payload struct {
		GameIndex           int                         `json:"game_index"`
		GameSeed            *int                        `json:"game_seed"`
		SeededInitialLayout ResearchSeededInitialLayout `json:"seeded_initial_layout"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	researchSessionsMutex.Lock()
	session, ok := researchSessions[gameID]
	researchSessionsMutex.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Session is not valid."})
		return
	}
	if session.Mode != "pregame_table" {
		c.JSON(http.StatusConflict, gin.H{"detail": "Session is not a Pregame Table."})
		return
	}
	if payload.GameIndex != session.CurrentGameIndex {
		c.JSON(http.StatusConflict, gin.H{"detail": "Pregame Table layout game_index must match current game index."})
		return
	}
	if payload.GameSeed == nil || *payload.GameSeed != session.Seed+session.CurrentGameIndex {
		c.JSON(http.StatusConflict, gin.H{"detail": "Pregame Table layout game_seed must match seed + game_index."})
		return
	}
	layout, err := validateResearchLayout(payload.SeededInitialLayout, len(session.SeatOrder))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}

	researchSessionsMutex.Lock()
	session.SeatOrder = append([]int(nil), layout.seatOrder...)
	session.RosterPlayerToSeatID = copyStringMap(layout.rosterPlayerToSeatID)
	status := researchSessionStatus(session)
	researchSessionsMutex.Unlock()
	c.JSON(http.StatusOK, status)
}

func researchGetSessionStatus(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	session, ok := researchSessionForRequest(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, researchSessionStatus(session))
}

func researchCreateBotJoinSession(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	var payload ResearchBotJoinSessionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if payload.RosterPlayerID == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot join session requires roster_player_id."})
		return
	}

	gameID := c.Param("gameID")
	researchSessionsMutex.Lock()
	session, ok := researchSessions[gameID]
	if !ok {
		researchSessionsMutex.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"detail": "Research session is not valid."})
		return
	}
	if !session.BotRosterPlayerIDs[payload.RosterPlayerID] {
		researchSessionsMutex.Unlock()
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Research Bot Join Sessions can only be minted for bot Roster Players."})
		return
	}

	seatIndex := -1
	for index, rosterPlayerID := range session.RosterPlayerIDsBySeat {
		if rosterPlayerID == payload.RosterPlayerID {
			seatIndex = index
			break
		}
	}
	if seatIndex < 0 {
		researchSessionsMutex.Unlock()
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot Roster Player is not assigned to a seat."})
		return
	}

	rosterIndex := 0
	if seatIndex < len(session.SeatOrder) {
		rosterIndex = session.SeatOrder[seatIndex]
	}
	username := payload.RosterPlayerID
	if seatIndex < len(session.RosterPlayerNamesBySeat) && session.RosterPlayerNamesBySeat[seatIndex] != "" {
		username = session.RosterPlayerNamesBySeat[seatIndex]
	}
	token := researchNewJoinToken()
	join := &ResearchJoinToken{
		Token:          token,
		GameID:         session.GameID,
		TableID:        session.TableID,
		RosterPlayerID: payload.RosterPlayerID,
		RosterIndex:    rosterIndex,
		SeatIndex:      seatIndex,
		UserID:         researchUserIDForTableSeat(session.TableID, seatIndex),
		Username:       username,
	}
	researchJoinTokens[token] = join
	researchGuestUsers[join.UserID] = join
	researchSessionsMutex.Unlock()

	c.JSON(http.StatusCreated, CreatedResearchBotJoinSession{
		GameID:         session.GameID,
		RosterPlayerID: payload.RosterPlayerID,
		JoinCredential: token,
		ServerURL:      researchPublicBaseURL(),
		OurPlayerIndex: seatIndex,
	})
}

func researchPostBotAction(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	session, ok := researchSessionForRequest(c)
	if !ok {
		return
	}

	var payload ResearchBotActionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if !session.BotRosterPlayerIDs[payload.RosterPlayerID] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot action roster_player_id is not a bot Roster Player."})
		return
	}

	ctx := context.Background()
	table, exists := getTableAndLock(ctx, nil, session.TableID, true, true)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Research table is not valid."})
		return
	}
	if !table.Running || table.Game == nil {
		table.Unlock(ctx)
		c.JSON(http.StatusConflict, gin.H{"detail": "Research game has not started."})
		return
	}
	game := table.Game
	if game.EndCondition > EndConditionInProgress {
		table.Unlock(ctx)
		c.JSON(http.StatusConflict, gin.H{"detail": "Research game has already completed."})
		return
	}
	currentRosterPlayerID := session.RosterPlayerIDsBySeat[game.ActivePlayerIndex]
	if payload.RosterPlayerID != currentRosterPlayerID {
		table.Unlock(ctx)
		c.JSON(http.StatusConflict, gin.H{"detail": "It is not this bot Roster Player's turn."})
		return
	}
	legalActions := researchLegalActions(game)
	if !stringInSlice(payload.Action, legalActions) {
		table.Unlock(ctx)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot action is not legal in the current state."})
		return
	}
	var botAction ResearchBotAction
	if err := json.Unmarshal([]byte(payload.Action), &botAction); err != nil {
		table.Unlock(ctx)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot action must be a JSON action object."})
		return
	}
	actorSession := table.Players[game.ActivePlayerIndex].Session
	tableID := table.ID
	table.Unlock(ctx)

	commandAction(ctx, actorSession, &CommandData{
		TableID: tableID,
		Type:    botAction.Type,
		Target:  botAction.Target,
		Value:   botAction.Value,
	})

	c.JSON(http.StatusOK, researchSessionStatus(session))
}

func researchMagicJoin(c *gin.Context) {
	token := c.Param("token")
	researchSessionsMutex.Lock()
	join, ok := researchJoinTokens[token]
	researchSessionsMutex.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Research Magic Join Link is not valid."})
		return
	}

	session := gsessions.Default(c)
	session.Set("userID", join.UserID)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "Failed to save research guest session."})
		return
	}

	joinQuery := url.QueryEscape(token)
	path := fmt.Sprintf("/pre-game/%d?researchMagicJoin=%s", join.TableID, joinQuery)
	if table, ok := tables.Get(join.TableID, true); ok {
		table.Lock(context.Background())
		if table.Running {
			path = fmt.Sprintf("/game/%d?researchMagicJoin=%s", join.TableID, joinQuery)
		}
		table.Unlock(context.Background())
	}
	c.Redirect(http.StatusFound, path)
}

func researchMagicJoinTokenCredentials(token string) (int, string, bool) {
	if token == "" || token == "1" {
		return 0, "", false
	}
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	join, ok := researchJoinTokens[token]
	if !ok {
		return 0, "", false
	}
	return join.UserID, join.Username, true
}

func researchSessionForRequest(c *gin.Context) (*ResearchSession, bool) {
	gameID := c.Param("gameID")
	researchSessionsMutex.Lock()
	session, ok := researchSessions[gameID]
	researchSessionsMutex.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Session is not valid."})
		return nil, false
	}
	return session, true
}

func researchRequireAdminToken(c *gin.Context) bool {
	adminToken := os.Getenv("HANABI_LIVE_ADMIN_TOKEN")
	expected := "Bearer " + adminToken
	if adminToken == "" || c.GetHeader("Authorization") != expected {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Admin token is required."})
		return false
	}
	return true
}

func validateResearchPayload(payload ResearchCreatePayload) (*validatedResearchLayout, error) {
	if len(payload.RosterPlayers) < 2 || len(payload.RosterPlayers) > 6 {
		return nil, fmt.Errorf("Payload must include between 2 and 6 Roster Players.")
	}
	if err := validateResearchRoster(payload.RosterPlayers); err != nil {
		return nil, err
	}
	if payload.Game.GameSeed == nil && payload.Mode == "single_game" {
		return nil, fmt.Errorf("Payload must include game.game_seed metadata.")
	}
	return validateResearchLayout(payload.SeededInitialLayout, len(payload.RosterPlayers))
}

func validateResearchRoster(players []ResearchRosterPlayer) error {
	seenIndexes := make(map[int]bool)
	seenIDs := make(map[string]bool)
	for _, player := range players {
		if player.RosterIndex < 0 || player.RosterIndex >= len(players) {
			return fmt.Errorf("Roster Player indexes must be a permutation of 0..%d.", len(players)-1)
		}
		if seenIndexes[player.RosterIndex] {
			return fmt.Errorf("Roster Player indexes must be unique.")
		}
		seenIndexes[player.RosterIndex] = true
		if player.RosterPlayerID == "" {
			return fmt.Errorf("Roster Player must include roster_player_id.")
		}
		if seenIDs[player.RosterPlayerID] {
			return fmt.Errorf("Roster Player IDs must be unique.")
		}
		seenIDs[player.RosterPlayerID] = true
		if player.Type != "human" && player.Type != "bot" {
			return fmt.Errorf("Roster Player type must be human or bot.")
		}
	}
	return nil
}

func validateResearchLayout(layout ResearchSeededInitialLayout, numPlayers int) (*validatedResearchLayout, error) {
	deck, err := validateResearchDeckOrder(layout.DeckOrder)
	if err != nil {
		return nil, err
	}
	seatOrder, err := validateResearchSeatOrder(layout.SeatOrder, numPlayers)
	if err != nil {
		return nil, err
	}
	if err := validateResearchAssignmentMap(seatOrder, layout.RosterPlayerToSeatID); err != nil {
		return nil, err
	}
	return &validatedResearchLayout{
		deckOrder:            deck,
		seatOrder:            seatOrder,
		rosterPlayerToSeatID: copyStringMap(layout.RosterPlayerToSeatID),
	}, nil
}

func validateResearchDeckOrder(deckOrder []ResearchCardIdentity) ([]*CardIdentity, error) {
	expectedSize := researchDeckColors * sumInts(researchDeckCountsByRank)
	if len(deckOrder) != expectedSize {
		return nil, fmt.Errorf("Deck Order must contain %d cards, got %d.", expectedSize, len(deckOrder))
	}
	counts := make(map[string]int)
	converted := make([]*CardIdentity, 0, len(deckOrder))
	for _, card := range deckOrder {
		if card.Color < 0 || card.Color >= researchDeckColors || card.Rank < 0 || card.Rank >= researchDeckRanks {
			return nil, fmt.Errorf("Deck Order card (%d, %d) is outside JAXMARL card ranges.", card.Color, card.Rank)
		}
		counts[researchCardKey(card.Color, card.Rank)]++
		converted = append(converted, &CardIdentity{
			SuitIndex: researchHanabiLiveSuitIndexFromJAXMARLColor(card.Color),
			Rank:      card.Rank + 1,
		})
	}
	for color := 0; color < researchDeckColors; color++ {
		for rank, expected := range researchDeckCountsByRank {
			actual := counts[researchCardKey(color, rank)]
			if actual != expected {
				return nil, fmt.Errorf("Deck Order has %d copies of (%d, %d), expected %d.", actual, color, rank, expected)
			}
		}
	}
	return converted, nil
}

func researchHanabiLiveSuitIndexFromJAXMARLColor(color int) int {
	// JAXMARL cards are R,Y,G,W,B; Hanabi.live No Variant suits are R,Y,G,B,P.
	return researchJAXMARLColorToHanabiLiveSuitIndex[color]
}

func validateResearchSeatOrder(seatOrder []int, numPlayers int) ([]int, error) {
	if len(seatOrder) != numPlayers {
		return nil, fmt.Errorf("Seat Order must contain %d roster indexes, got %d.", numPlayers, len(seatOrder))
	}
	seen := make(map[int]bool)
	for _, rosterIndex := range seatOrder {
		if rosterIndex < 0 || rosterIndex >= numPlayers || seen[rosterIndex] {
			return nil, fmt.Errorf("Seat Order must be a permutation of roster indexes 0..%d.", numPlayers-1)
		}
		seen[rosterIndex] = true
	}
	return append([]int(nil), seatOrder...), nil
}

func validateResearchAssignmentMap(seatOrder []int, assignment map[string]string) error {
	if len(assignment) != len(seatOrder) {
		return fmt.Errorf("Roster Player-to-Seat ID assignment must be derived from Seat Order.")
	}
	for seatIndex, rosterIndex := range seatOrder {
		key := strconv.Itoa(rosterIndex)
		expected := fmt.Sprintf("seat_%d", seatIndex)
		if assignment[key] != expected {
			return fmt.Errorf("Roster Player-to-Seat ID assignment must be derived from Seat Order.")
		}
	}
	return nil
}

func createResearchSingleGameTable(payload ResearchCreatePayload, layout *validatedResearchLayout) (*Table, error) {
	ctx := context.Background()
	gameSeed := researchGameSeed(payload.Game)
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range payload.RosterPlayers {
		playerByRosterIndex[player.RosterIndex] = player
	}

	tables.Lock(ctx)
	defer tables.Unlock(ctx)

	tableName := fmt.Sprintf("research-%d-%d", gameSeed, time.Now().UnixNano())
	table := NewTable(tableName, 0)
	table.Visible = false
	table.MaxPlayers = len(payload.RosterPlayers)
	table.Options = NewOptions()
	table.Options.NumPlayers = len(payload.RosterPlayers)
	table.Options.VariantName = DefaultVariantName
	table.ExtraOptions = &ExtraOptions{
		DatabaseID:                   -1,
		NoWriteToDatabase:            true,
		JSONReplay:                   false,
		CustomNumPlayers:             len(payload.RosterPlayers),
		CustomDeck:                   layout.deckOrder,
		ResearchGameSeed:             strconv.Itoa(gameSeed),
		ResearchSeatOrder:            append([]int(nil), layout.seatOrder...),
		ResearchRosterPlayerToSeatID: copyStringMap(layout.rosterPlayerToSeatID),
	}
	for seatIndex, rosterIndex := range layout.seatOrder {
		rosterPlayer := playerByRosterIndex[rosterIndex]
		userID := researchUserIDForTableSeat(table.ID, seatIndex)
		username := researchDisplayName(payload.Game.IdentityDisplay, rosterPlayer, seatIndex)
		session := NewFakeSession(userID, username)
		present := rosterPlayer.Type == "bot"
		if present {
			sessions.Set(userID, session)
		}
		table.Players = append(table.Players, &Player{
			UserID:    userID,
			Name:      username,
			Session:   session,
			Present:   present,
			Stats:     &PregameStats{NumGames: 0, Variant: NewUserStatsRow()},
			LastTyped: time.Time{},
		})
		tables.AddPlaying(userID, table.ID)
	}
	table.OwnerID = table.Players[0].UserID
	table.OwnerUsername = table.Players[0].Name
	tables.Set(table.ID, table)
	return table, nil
}

func researchRegisterJoinLinks(gameID string, tableID uint64, payload ResearchCreatePayload, layout *validatedResearchLayout) map[string]string {
	links := make(map[string]string)
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range payload.RosterPlayers {
		playerByRosterIndex[player.RosterIndex] = player
	}
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	for seatIndex, rosterIndex := range layout.seatOrder {
		player := playerByRosterIndex[rosterIndex]
		if player.Type != "human" {
			continue
		}
		token := researchNewJoinToken()
		join := &ResearchJoinToken{
			Token:          token,
			GameID:         gameID,
			TableID:        tableID,
			RosterPlayerID: player.RosterPlayerID,
			RosterIndex:    player.RosterIndex,
			SeatIndex:      seatIndex,
			UserID:         researchUserIDForTableSeat(tableID, seatIndex),
			Username:       researchDisplayName(payload.Game.IdentityDisplay, player, seatIndex),
		}
		researchJoinTokens[token] = join
		researchGuestUsers[join.UserID] = join
		links[player.RosterPlayerID] = fmt.Sprintf("%s/join/%s", researchPublicBaseURL(), token)
	}
	return links
}

func researchPregameJoinLinks(players []ResearchRosterPlayer, tableID string) map[string]string {
	links := make(map[string]string)
	for _, player := range players {
		if player.Type != "human" {
			continue
		}
		links[player.RosterPlayerID] = fmt.Sprintf("%s/pre-game/%s", researchPublicBaseURL(), tableID)
	}
	return links
}

func researchPublicBaseURL() string {
	host := os.Getenv("DOMAIN")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	if port == "80" {
		envPort := os.Getenv("HANABI_LIVE_PORT")
		if envPort != "" {
			port = envPort
		}
	}
	return "http://" + host + ":" + port
}

func researchNewJoinToken() string {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes)
}

func researchUserIDForTableSeat(tableID uint64, seatIndex int) int {
	return -100000 - int(tableID)*10 - seatIndex
}

func researchDisplayName(identityDisplay string, player ResearchRosterPlayer, seatIndex int) string {
	if identityDisplay == "show_display_names" {
		if player.DisplayName != "" {
			return player.DisplayName
		}
		return player.RosterPlayerID
	}
	return fmt.Sprintf("Player %d", seatIndex+1)
}

func researchRosterPlayerIDsBySeat(players []ResearchRosterPlayer, seatOrder []int) []string {
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range players {
		playerByRosterIndex[player.RosterIndex] = player
	}
	ids := make([]string, 0, len(seatOrder))
	for _, rosterIndex := range seatOrder {
		ids = append(ids, playerByRosterIndex[rosterIndex].RosterPlayerID)
	}
	return ids
}

func researchRosterPlayerNamesBySeat(players []ResearchRosterPlayer, seatOrder []int, identityDisplay string) []string {
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range players {
		playerByRosterIndex[player.RosterIndex] = player
	}
	names := make([]string, 0, len(seatOrder))
	for seatIndex, rosterIndex := range seatOrder {
		names = append(names, researchDisplayName(identityDisplay, playerByRosterIndex[rosterIndex], seatIndex))
	}
	return names
}

func researchBotRosterPlayerIDs(players []ResearchRosterPlayer) map[string]bool {
	botIDs := make(map[string]bool)
	for _, player := range players {
		if player.Type == "bot" {
			botIDs[player.RosterPlayerID] = true
		}
	}
	return botIDs
}

func researchGameSeed(game ResearchGamePayload) int {
	if game.GameSeed == nil {
		return 0
	}
	return *game.GameSeed
}

func researchCardKey(color int, rank int) string {
	return strconv.Itoa(color) + ":" + strconv.Itoa(rank)
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func copyStringMap(source map[string]string) map[string]string {
	copyMap := make(map[string]string)
	for key, value := range source {
		copyMap[key] = value
	}
	return copyMap
}

func researchSessionStatus(session *ResearchSession) gin.H {
	status := gin.H{
		"game_id":                       session.GameID,
		"paused":                        false,
		"waiting_for_reconnect":         nil,
		"current_turn_roster_player_id": nil,
		"legal_actions":                 []string{},
		"canonical_public_events":       []gin.H{},
		"game_finished":                 false,
		"final_score":                   nil,
		"terminal_reason":               nil,
		"actions":                       []gin.H{},
		"auto_action_taken":             false,
		"timeout_action_taken":          false,
		"last_action":                   nil,
	}
	if session.TableID != 0 {
		researchAttachTableStatus(status, session)
	}
	if session.Mode == "pregame_table" {
		status["current_game_index"] = session.CurrentGameIndex
		status["ready_status"] = session.ReadyStatus
		status["active_game_started"] = false
		status["completed_games"] = session.CompletedGames
	}
	return status
}

func researchAttachTableStatus(status gin.H, session *ResearchSession) {
	ctx := context.Background()
	table, ok := tables.Get(session.TableID, true)
	if !ok {
		return
	}
	table.Lock(ctx)
	defer table.Unlock(ctx)

	status["table_id"] = table.ID
	status["active_game_started"] = table.Running
	if !table.Running || table.Game == nil {
		return
	}

	game := table.Game
	finished := game.EndCondition > EndConditionInProgress
	status["game_finished"] = finished
	status["final_score"] = game.Score
	status["terminal_reason"] = researchTerminalReason(game.EndCondition)
	status["actions"] = researchActionSummaries(game.Actions2)
	status["canonical_public_events"] = researchCanonicalPublicEvents(game)
	if !finished && game.ActivePlayerIndex >= 0 && game.ActivePlayerIndex < len(session.RosterPlayerIDsBySeat) {
		status["current_turn_roster_player_id"] = session.RosterPlayerIDsBySeat[game.ActivePlayerIndex]
		status["legal_actions"] = researchLegalActions(game)
	}
}

func researchCanonicalPublicEvents(game *Game) []gin.H {
	events := make([]gin.H, 0, len(game.Actions2)+1)
	for index, action := range game.Actions2 {
		events = append(events, gin.H{
			"type":       "action_applied",
			"turn_index": index,
			"action": gin.H{
				"type":   action.Type,
				"target": action.Target,
				"value":  action.Value,
			},
		})
	}
	if game.EndCondition > EndConditionInProgress {
		events = append(events, gin.H{
			"type":            "game_finished",
			"terminal_reason": researchTerminalReason(game.EndCondition),
			"final_score":     game.Score,
		})
	}
	return events
}

func researchActionSummaries(actions []*GameAction) []gin.H {
	summaries := make([]gin.H, 0, len(actions))
	for index, action := range actions {
		summaries = append(summaries, gin.H{
			"turn_index": index,
			"type":       action.Type,
			"target":     action.Target,
			"value":      action.Value,
		})
	}
	return summaries
}

func researchTerminalReason(endCondition int) interface{} {
	if endCondition == EndConditionInProgress {
		return nil
	}
	switch endCondition {
	case EndConditionNormal:
		return "normal"
	case EndConditionStrikeout:
		return "strikeout"
	case EndConditionTimeout:
		return "timeout"
	case EndConditionTerminatedByPlayer:
		return "terminated_by_player"
	case EndConditionTerminatedByVote:
		return "terminated_by_vote"
	default:
		return fmt.Sprintf("end_condition_%d", endCondition)
	}
}

func researchLegalActions(game *Game) []string {
	if game == nil || game.EndCondition > EndConditionInProgress {
		return []string{}
	}
	currentPlayer := game.Players[game.ActivePlayerIndex]
	actions := make([]string, 0)
	for _, card := range currentPlayer.Hand {
		actions = append(actions, researchEncodeAction(ResearchBotAction{
			Type:   ActionTypePlay,
			Target: card.Order,
			Value:  0,
		}))
		if !variants[game.Options.VariantName].AtMaxClueTokens(game.ClueTokens) {
			actions = append(actions, researchEncodeAction(ResearchBotAction{
				Type:   ActionTypeDiscard,
				Target: card.Order,
				Value:  0,
			}))
		}
	}
	if game.ClueTokens >= variants[game.Options.VariantName].GetAdjustedClueTokens(1) {
		for targetIndex, targetPlayer := range game.Players {
			if targetIndex == game.ActivePlayerIndex || len(targetPlayer.Hand) == 0 {
				continue
			}
			seenRanks := make(map[int]bool)
			seenColors := make(map[int]bool)
			for _, card := range targetPlayer.Hand {
				if !seenRanks[card.Rank] {
					actions = append(actions, researchEncodeAction(ResearchBotAction{
						Type:   ActionTypeRankClue,
						Target: targetIndex,
						Value:  card.Rank,
					}))
					seenRanks[card.Rank] = true
				}
				if !seenColors[card.SuitIndex] {
					actions = append(actions, researchEncodeAction(ResearchBotAction{
						Type:   ActionTypeColorClue,
						Target: targetIndex,
						Value:  card.SuitIndex,
					}))
					seenColors[card.SuitIndex] = true
				}
			}
		}
	}
	return actions
}

func researchEncodeAction(action ResearchBotAction) string {
	encoded, err := json.Marshal(action)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func researchIsGuestUser(userID int) bool {
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	_, ok := researchGuestUsers[userID]
	return ok
}

func researchKeepsTableSeatOnSessionReplacement(newSession *Session, oldSession *Session) bool {
	return newSession != nil &&
		oldSession != nil &&
		oldSession.FakeUser &&
		researchIsGuestUser(newSession.UserID)
}

func researchHandleGuestConnected(s *Session) {
	researchSessionsMutex.Lock()
	join, ok := researchGuestUsers[s.UserID]
	researchSessionsMutex.Unlock()
	if !ok {
		return
	}

	ctx := NewSessionContext(s)
	table, exists := getTableAndLock(ctx, s, join.TableID, true, true)
	if !exists {
		return
	}
	defer table.Unlock(ctx)

	for _, player := range table.Players {
		if player.UserID == s.UserID {
			player.Session = s
			player.Present = true
			break
		}
	}
	s.SetTableID(table.ID)
	if table.Running {
		s.SetStatus(StatusPlaying)
		s.NotifyTableStart(table)
		return
	}

	s.SetStatus(StatusPregame)
	s.NotifyTableJoined(table)
	table.NotifyPlayerChange()
	s.NotifySpectators(table)
	if researchAllSeatsPresent(table) {
		tableStart(ctx, table.Players[0].Session, &CommandData{
			TableID:      table.ID,
			NoTableLock:  true,
			NoTablesLock: true,
		}, table)
		for _, player := range table.Players {
			if player.Session != nil {
				player.Session.SetStatus(StatusPlaying)
				player.Session.SetTableID(table.ID)
			}
		}
	}
}

func researchGuestUsername(userID int) (string, bool) {
	researchSessionsMutex.Lock()
	join, ok := researchGuestUsers[userID]
	researchSessionsMutex.Unlock()
	if !ok {
		return "", false
	}
	return join.Username, true
}

func researchAllSeatsPresent(table *Table) bool {
	for _, player := range table.Players {
		if !player.Present {
			return false
		}
	}
	return true
}

func researchLayoutFromInterface(raw map[string]interface{}) (ResearchSeededInitialLayout, error) {
	rawDeck, ok := raw["deck_order"].([]interface{})
	if !ok {
		return ResearchSeededInitialLayout{}, fmt.Errorf("seeded_initial_layout.deck_order must be a list.")
	}
	deck := make([]ResearchCardIdentity, 0, len(rawDeck))
	for _, rawCard := range rawDeck {
		card, ok := rawCard.(map[string]interface{})
		if !ok {
			return ResearchSeededInitialLayout{}, fmt.Errorf("Deck Order card must include color and rank.")
		}
		color, ok := researchIntField(card, "color")
		if !ok {
			return ResearchSeededInitialLayout{}, fmt.Errorf("Deck Order card must include color and rank.")
		}
		rank, ok := researchIntField(card, "rank")
		if !ok {
			return ResearchSeededInitialLayout{}, fmt.Errorf("Deck Order card must include color and rank.")
		}
		deck = append(deck, ResearchCardIdentity{Color: color, Rank: rank})
	}

	rawSeatOrder, ok := raw["seat_order"].([]interface{})
	if !ok {
		return ResearchSeededInitialLayout{}, fmt.Errorf("seeded_initial_layout.seat_order must be a list.")
	}
	seatOrder := make([]int, 0, len(rawSeatOrder))
	for _, value := range rawSeatOrder {
		rosterIndex, ok := researchIntValue(value)
		if !ok {
			return ResearchSeededInitialLayout{}, fmt.Errorf("Seat Order must contain integer roster indexes.")
		}
		seatOrder = append(seatOrder, rosterIndex)
	}

	rawAssignment, ok := raw["roster_player_to_seat_id"].(map[string]interface{})
	if !ok {
		return ResearchSeededInitialLayout{}, fmt.Errorf("seeded_initial_layout.roster_player_to_seat_id must be a mapping.")
	}
	assignment := make(map[string]string)
	for key, value := range rawAssignment {
		seatID, ok := value.(string)
		if !ok {
			return ResearchSeededInitialLayout{}, fmt.Errorf("seeded_initial_layout.roster_player_to_seat_id must map to seat IDs.")
		}
		assignment[key] = seatID
	}
	return ResearchSeededInitialLayout{
		DeckOrder:            deck,
		SeatOrder:            seatOrder,
		RosterPlayerToSeatID: assignment,
	}, nil
}

func researchIntField(raw map[string]interface{}, key string) (int, bool) {
	value, ok := raw[key]
	if !ok {
		return 0, false
	}
	return researchIntValue(value)
}

func researchIntValue(value interface{}) (int, bool) {
	floatValue, ok := value.(float64)
	if !ok {
		return 0, false
	}
	intValue := int(floatValue)
	if floatValue != float64(intValue) {
		return 0, false
	}
	return intValue, true
}
