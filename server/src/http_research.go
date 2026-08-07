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
	researchDeckColors              = 5
	researchDeckRanks               = 5
	researchUnifiedManualCapability = "unified_manual_v1"
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
	Seed             int      `json:"seed"`
	GameIndex        int      `json:"game_index"`
	GameSeed         *int     `json:"game_seed"`
	IdentityDisplay  string   `json:"identity_display"`
	SeatDisplayNames []string `json:"seat_display_names,omitempty"`
	ChatEnabled      bool     `json:"chat_enabled"`
}

type ResearchRosterPlayer struct {
	RosterIndex    int    `json:"roster_index"`
	RosterPlayerID string `json:"roster_player_id"`
	Type           string `json:"type"`
	Location       string `json:"location,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	RequiredSeat   *int   `json:"required_seat,omitempty"`
	ModelPath      string `json:"model_path,omitempty"`
}

type ResearchCreatePayload struct {
	Mode                      string                      `json:"mode"`
	Unified                   bool                        `json:"unified,omitempty"`
	Game                      ResearchGamePayload         `json:"game"`
	RosterPlayers             []ResearchRosterPlayer      `json:"roster_players"`
	SeededInitialLayout       ResearchSeededInitialLayout `json:"seeded_initial_layout"`
	ControllerSeatAssignments map[string]string           `json:"controller_seat_assignments,omitempty"`
}

type CreatedResearchSingleGame struct {
	TableID         uint64            `json:"table_id"`
	GameID          string            `json:"game_id"`
	Mode            string            `json:"mode"`
	GameSeed        int               `json:"game_seed"`
	LayoutSource    string            `json:"layout_source"`
	SeatOrder       []int             `json:"seat_order"`
	JoinLinks       map[string]string `json:"join_links"`
	UnifiedJoinLink string            `json:"unified_join_link,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
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
	GameID                          string
	TableID                         uint64
	Mode                            string
	Seed                            int
	CurrentGameIndex                int
	ReadyStatus                     map[string]bool
	CompletedGames                  []map[string]interface{}
	SeatOrder                       []int
	RosterPlayerToSeatID            map[string]string
	RosterPlayerIDsBySeat           []string
	RosterPlayerNamesBySeat         []string
	ControllerRosterPlayerIDsBySeat []string
	BotRosterPlayerIDs              map[string]bool
	RestartControllerUserID         int
	PendingRestartRequest           *ResearchRestartRequest
	NextRestartRequestID            int
	RosterPlayers                   []ResearchRosterPlayer
	IdentityDisplay                 string
	SeatDisplayNames                []string
	// LifecycleMutex protects the mutable run/layout fields above. It must never be
	// held while acquiring or holding a table mutex or researchSessionsMutex.
	LifecycleMutex              sync.Mutex
	LifecycleMutationInProgress bool
}

type researchSessionLifecycleSnapshot struct {
	GameID                          string
	TableID                         uint64
	Mode                            string
	Seed                            int
	CurrentGameIndex                int
	ReadyStatus                     map[string]bool
	CompletedGames                  []map[string]interface{}
	SeatOrder                       []int
	RosterPlayerToSeatID            map[string]string
	RosterPlayerIDsBySeat           []string
	RosterPlayerNamesBySeat         []string
	ControllerRosterPlayerIDsBySeat []string
	BotRosterPlayerIDs              map[string]bool
	RestartControllerUserID         int
	PendingRestartRequest           *ResearchRestartRequest
	NextRestartRequestID            int
	RosterPlayers                   []ResearchRosterPlayer
	IdentityDisplay                 string
	SeatDisplayNames                []string
	MutationInProgress              bool
}

func (session *ResearchSession) lifecycleSnapshot() researchSessionLifecycleSnapshot {
	session.LifecycleMutex.Lock()
	defer session.LifecycleMutex.Unlock()

	var pendingRestartRequest *ResearchRestartRequest
	if session.PendingRestartRequest != nil {
		requestCopy := *session.PendingRestartRequest
		pendingRestartRequest = &requestCopy
	}
	readyStatus := make(map[string]bool, len(session.ReadyStatus))
	for playerID, ready := range session.ReadyStatus {
		readyStatus[playerID] = ready
	}
	botRosterPlayerIDs := make(map[string]bool, len(session.BotRosterPlayerIDs))
	for playerID, bot := range session.BotRosterPlayerIDs {
		botRosterPlayerIDs[playerID] = bot
	}

	return researchSessionLifecycleSnapshot{
		GameID:                          session.GameID,
		TableID:                         session.TableID,
		Mode:                            session.Mode,
		Seed:                            session.Seed,
		CurrentGameIndex:                session.CurrentGameIndex,
		ReadyStatus:                     readyStatus,
		CompletedGames:                  append([]map[string]interface{}(nil), session.CompletedGames...),
		SeatOrder:                       append([]int(nil), session.SeatOrder...),
		RosterPlayerToSeatID:            copyStringMap(session.RosterPlayerToSeatID),
		RosterPlayerIDsBySeat:           append([]string(nil), session.RosterPlayerIDsBySeat...),
		RosterPlayerNamesBySeat:         append([]string(nil), session.RosterPlayerNamesBySeat...),
		ControllerRosterPlayerIDsBySeat: append([]string(nil), session.ControllerRosterPlayerIDsBySeat...),
		BotRosterPlayerIDs:              botRosterPlayerIDs,
		RestartControllerUserID:         session.RestartControllerUserID,
		PendingRestartRequest:           pendingRestartRequest,
		NextRestartRequestID:            session.NextRestartRequestID,
		RosterPlayers:                   append([]ResearchRosterPlayer(nil), session.RosterPlayers...),
		IdentityDisplay:                 session.IdentityDisplay,
		SeatDisplayNames:                append([]string(nil), session.SeatDisplayNames...),
		MutationInProgress:              session.LifecycleMutationInProgress,
	}
}

func (session *ResearchSession) tryBeginLifecycleMutation() bool {
	session.LifecycleMutex.Lock()
	defer session.LifecycleMutex.Unlock()
	if session.LifecycleMutationInProgress {
		return false
	}
	session.LifecycleMutationInProgress = true
	return true
}

func (session *ResearchSession) endLifecycleMutation() {
	session.LifecycleMutex.Lock()
	session.LifecycleMutationInProgress = false
	session.LifecycleMutex.Unlock()
}

type ResearchRestartRequest struct {
	RequestID int    `json:"request_id"`
	Kind      string `json:"kind"`
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
	Unified        bool
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
	router.POST("/research/sessions/:gameID/restart", researchRestartSingleGame)
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
	rosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(payload.RosterPlayers, layout.seatOrder, payload.Game)
	controllerRosterPlayerIDsBySeat := researchControllerRosterPlayerIDsBySeat(payload, layout)
	botRosterPlayerIDs := researchBotRosterPlayerIDs(payload.RosterPlayers)
	joinLinks := make(map[string]string)
	unifiedJoinLink := ""
	if payload.Unified {
		var unifiedJoin *ResearchJoinToken
		unifiedJoinLink, unifiedJoin = researchRegisterUnifiedJoinLink(
			gameID,
			table.ID,
			len(payload.RosterPlayers),
		)
		table.Lock(nil)
		table.ResearchUnifiedController = &ResearchUnifiedController{
			UserID:             unifiedJoin.UserID,
			ViewedSeat:         0,
			SelectedBoundary:   0,
			ProjectionRevision: 1,
			Initialized:        false,
		}
		table.Unlock(nil)
	} else {
		joinLinks = researchRegisterJoinLinks(gameID, table.ID, payload, layout)
	}
	created := CreatedResearchSingleGame{
		TableID:         table.ID,
		GameID:          gameID,
		Mode:            "single_game",
		GameSeed:        gameSeed,
		LayoutSource:    "payload",
		SeatOrder:       append([]int(nil), layout.seatOrder...),
		JoinLinks:       joinLinks,
		UnifiedJoinLink: unifiedJoinLink,
	}
	if payload.Unified {
		created.Capabilities = []string{researchUnifiedManualCapability}
	}
	researchSessionsMutex.Lock()
	researchSessions[gameID] = &ResearchSession{
		GameID:                          gameID,
		TableID:                         table.ID,
		Mode:                            "single_game",
		SeatOrder:                       append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID:            copyStringMap(layout.rosterPlayerToSeatID),
		RosterPlayerIDsBySeat:           rosterPlayerIDsBySeat,
		RosterPlayerNamesBySeat:         rosterPlayerNamesBySeat,
		ControllerRosterPlayerIDsBySeat: controllerRosterPlayerIDsBySeat,
		BotRosterPlayerIDs:              botRosterPlayerIDs,
		Seed:                            payload.Game.Seed,
		CurrentGameIndex:                payload.Game.GameIndex,
		RestartControllerUserID:         researchRestartControllerUserID(table, payload, layout),
		RosterPlayers:                   append([]ResearchRosterPlayer(nil), payload.RosterPlayers...),
		IdentityDisplay:                 payload.Game.IdentityDisplay,
		SeatDisplayNames:                append([]string(nil), payload.Game.SeatDisplayNames...),
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
	rosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(payload.RosterPlayers, layout.seatOrder, payload.Game)
	controllerRosterPlayerIDsBySeat := researchControllerRosterPlayerIDsBySeat(payload, layout)
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
		GameID:                          tableID,
		TableID:                         table.ID,
		Mode:                            "pregame_table",
		Seed:                            payload.Game.Seed,
		CurrentGameIndex:                0,
		ReadyStatus:                     readyStatus,
		CompletedGames:                  make([]map[string]interface{}, 0),
		SeatOrder:                       append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID:            copyStringMap(layout.rosterPlayerToSeatID),
		RosterPlayerIDsBySeat:           rosterPlayerIDsBySeat,
		RosterPlayerNamesBySeat:         rosterPlayerNamesBySeat,
		ControllerRosterPlayerIDsBySeat: controllerRosterPlayerIDsBySeat,
		BotRosterPlayerIDs:              botRosterPlayerIDs,
		RosterPlayers:                   append([]ResearchRosterPlayer(nil), payload.RosterPlayers...),
		IdentityDisplay:                 payload.Game.IdentityDisplay,
		SeatDisplayNames:                append([]string(nil), payload.Game.SeatDisplayNames...),
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
		GameIndex                 int                         `json:"game_index"`
		GameSeed                  *int                        `json:"game_seed"`
		SeededInitialLayout       ResearchSeededInitialLayout `json:"seeded_initial_layout"`
		ControllerSeatAssignments map[string]string           `json:"controller_seat_assignments,omitempty"`
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
	if !session.tryBeginLifecycleMutation() {
		c.JSON(http.StatusConflict, gin.H{"detail": "Another research lifecycle mutation is already in progress."})
		return
	}
	mutationInProgress := true
	defer func() {
		if mutationInProgress {
			session.endLifecycleMutation()
		}
	}()
	lifecycle := session.lifecycleSnapshot()
	if lifecycle.Mode != "pregame_table" {
		c.JSON(http.StatusConflict, gin.H{"detail": "Session is not a Pregame Table."})
		return
	}
	if payload.GameIndex != lifecycle.CurrentGameIndex {
		c.JSON(http.StatusConflict, gin.H{"detail": "Pregame Table layout game_index must match current game index."})
		return
	}
	if payload.GameSeed == nil || *payload.GameSeed != lifecycle.Seed+lifecycle.CurrentGameIndex {
		c.JSON(http.StatusConflict, gin.H{"detail": "Pregame Table layout game_seed must match seed + game_index."})
		return
	}
	layout, err := validateResearchLayout(payload.SeededInitialLayout, len(lifecycle.SeatOrder))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	gameConfig := ResearchCreatePayload{
		Game: ResearchGamePayload{
			IdentityDisplay:  lifecycle.IdentityDisplay,
			SeatDisplayNames: lifecycle.SeatDisplayNames,
		},
		RosterPlayers:             lifecycle.RosterPlayers,
		ControllerSeatAssignments: payload.ControllerSeatAssignments,
	}
	if err := validateResearchSeatPresentationAndControllers(gameConfig, layout); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	controllerIDsBySeat := researchControllerRosterPlayerIDsBySeat(gameConfig, layout)
	table, ok := tables.Get(lifecycle.TableID, true)
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"detail": "Pregame Table no longer exists."})
		return
	}

	table.Lock(nil)
	if err := researchApplyRestartLayout(table, lifecycle, layout, *payload.GameSeed, controllerIDsBySeat); err != nil {
		table.Unlock(nil)
		c.JSON(http.StatusConflict, gin.H{"detail": err.Error()})
		return
	}
	table.Unlock(nil)

	nextRosterPlayerIDsBySeat := researchRosterPlayerIDsBySeat(lifecycle.RosterPlayers, layout.seatOrder)
	nextRosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(lifecycle.RosterPlayers, layout.seatOrder, gameConfig.Game)
	session.LifecycleMutex.Lock()
	session.SeatOrder = append([]int(nil), layout.seatOrder...)
	session.RosterPlayerToSeatID = copyStringMap(layout.rosterPlayerToSeatID)
	session.RosterPlayerIDsBySeat = nextRosterPlayerIDsBySeat
	session.RosterPlayerNamesBySeat = nextRosterPlayerNamesBySeat
	session.ControllerRosterPlayerIDsBySeat = append([]string(nil), controllerIDsBySeat...)
	session.LifecycleMutex.Unlock()
	researchUpdateJoinTokensForLayout(session.lifecycleSnapshot())
	session.endLifecycleMutation()
	mutationInProgress = false
	status := researchSessionStatus(session)
	c.JSON(http.StatusOK, status)
}

func researchRestartSingleGame(c *gin.Context) {
	if !researchRequireAdminToken(c) {
		return
	}

	var payload struct {
		RequestID                 int                         `json:"request_id"`
		GameIndex                 int                         `json:"game_index"`
		GameSeed                  *int                        `json:"game_seed"`
		SeededInitialLayout       ResearchSeededInitialLayout `json:"seeded_initial_layout"`
		ControllerSeatAssignments map[string]string           `json:"controller_seat_assignments,omitempty"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	gameID := c.Param("gameID")
	researchSessionsMutex.Lock()
	session, ok := researchSessions[gameID]
	researchSessionsMutex.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Session is not valid."})
		return
	}
	if !session.tryBeginLifecycleMutation() {
		c.JSON(http.StatusConflict, gin.H{"detail": "Another research lifecycle mutation is already in progress."})
		return
	}
	mutationInProgress := true
	defer func() {
		if mutationInProgress {
			session.endLifecycleMutation()
		}
	}()
	lifecycle := session.lifecycleSnapshot()
	if lifecycle.Mode != "single_game" {
		c.JSON(http.StatusConflict, gin.H{"detail": "Session is not a persistent Single Game run."})
		return
	}
	request := lifecycle.PendingRestartRequest
	if request == nil || request.RequestID != payload.RequestID {
		c.JSON(http.StatusConflict, gin.H{"detail": "Restart request is not pending."})
		return
	}

	expectedGameIndex := lifecycle.CurrentGameIndex
	if request.Kind == researchRestartNextGame {
		expectedGameIndex++
	}
	expectedGameSeed := lifecycle.Seed + expectedGameIndex
	if payload.GameIndex != expectedGameIndex {
		c.JSON(http.StatusConflict, gin.H{"detail": "Restart game_index does not match the requested transition."})
		return
	}
	if payload.GameSeed == nil || *payload.GameSeed != expectedGameSeed {
		c.JSON(http.StatusConflict, gin.H{"detail": "Restart game_seed must equal seed + game_index."})
		return
	}
	layout, err := validateResearchLayout(payload.SeededInitialLayout, len(lifecycle.RosterPlayers))
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	restartConfig := ResearchCreatePayload{
		Game: ResearchGamePayload{
			IdentityDisplay:  lifecycle.IdentityDisplay,
			SeatDisplayNames: lifecycle.SeatDisplayNames,
		},
		RosterPlayers:             lifecycle.RosterPlayers,
		ControllerSeatAssignments: payload.ControllerSeatAssignments,
	}
	if err := validateResearchSeatPresentationAndControllers(restartConfig, layout); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
		return
	}
	nextControllerRosterPlayerIDsBySeat := researchControllerRosterPlayerIDsBySeat(restartConfig, layout)

	ctx := context.Background()
	table, ok := tables.Get(lifecycle.TableID, true)
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"detail": "Single Game table no longer exists."})
		return
	}
	table.Lock(ctx)
	if err := researchApplyRestartLayout(table, lifecycle, layout, expectedGameSeed, nextControllerRosterPlayerIDsBySeat); err != nil {
		table.Unlock(ctx)
		c.JSON(http.StatusConflict, gin.H{"detail": err.Error()})
		return
	}
	starter := table.Players[0].Session
	tableStart(ctx, starter, &CommandData{
		TableID:      table.ID,
		NoTableLock:  true,
		NoTablesLock: true,
	}, table)
	if controller := table.ResearchUnifiedController; controller != nil {
		invalidateResearchUnifiedFollow(controller)
		controller.ViewedSeat = table.Game.ActivePlayerIndex
		controller.SelectedBoundary = researchUnifiedLiveBoundary(table.Game)
		controller.ActionCutoffs = []int{len(table.Game.Actions)}
		controller.Initialized = true
		if spectatorIndex := table.GetSpectatorIndexFromID(controller.UserID); spectatorIndex >= 0 {
			spectator := table.Spectators[spectatorIndex]
			spectator.ShadowingPlayerIndex = controller.ViewedSeat
			spectator.ShadowingPlayerUsername = table.Players[controller.ViewedSeat].Name
		}
		reviseResearchUnifiedProjection(table)
	}
	table.Unlock(ctx)

	nextRosterPlayerIDsBySeat := researchRosterPlayerIDsBySeat(lifecycle.RosterPlayers, layout.seatOrder)
	nextRosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(
		lifecycle.RosterPlayers,
		layout.seatOrder,
		ResearchGamePayload{IdentityDisplay: lifecycle.IdentityDisplay, SeatDisplayNames: lifecycle.SeatDisplayNames},
	)
	session.LifecycleMutex.Lock()
	session.CurrentGameIndex = expectedGameIndex
	session.SeatOrder = append([]int(nil), layout.seatOrder...)
	session.RosterPlayerToSeatID = copyStringMap(layout.rosterPlayerToSeatID)
	session.RosterPlayerIDsBySeat = nextRosterPlayerIDsBySeat
	session.RosterPlayerNamesBySeat = nextRosterPlayerNamesBySeat
	session.ControllerRosterPlayerIDsBySeat = append([]string(nil), nextControllerRosterPlayerIDsBySeat...)
	session.PendingRestartRequest = nil
	session.LifecycleMutex.Unlock()
	researchUpdateJoinTokensForLayout(session.lifecycleSnapshot())
	session.endLifecycleMutation()
	mutationInProgress = false

	c.JSON(http.StatusOK, researchSessionStatus(session))
}

func researchApplyRestartLayout(
	table *Table,
	session researchSessionLifecycleSnapshot,
	layout *validatedResearchLayout,
	gameSeed int,
	nextControllerRosterPlayerIDsBySeat []string,
) error {
	playersByControllerID := make(map[string]*Player, len(table.Players))
	for seatIndex, controllerID := range session.ControllerRosterPlayerIDsBySeat {
		if seatIndex < len(table.Players) {
			playersByControllerID[controllerID] = table.Players[seatIndex]
		}
	}
	rosterPlayerNamesBySeat := researchRosterPlayerNamesBySeat(
		session.RosterPlayers,
		layout.seatOrder,
		ResearchGamePayload{IdentityDisplay: session.IdentityDisplay, SeatDisplayNames: session.SeatDisplayNames},
	)
	nextPlayers := make([]*Player, len(nextControllerRosterPlayerIDsBySeat))
	for seatIndex, controllerID := range nextControllerRosterPlayerIDsBySeat {
		player, ok := playersByControllerID[controllerID]
		if !ok {
			return fmt.Errorf("Controller %q is not assigned to the current table.", controllerID)
		}
		player.Name = rosterPlayerNamesBySeat[seatIndex]
		if player.Session != nil {
			player.Session.Username = player.Name
		}
		nextPlayers[seatIndex] = player
	}
	table.Players = nextPlayers
	table.OwnerID = nextPlayers[0].UserID
	table.OwnerUsername = nextPlayers[0].Name
	table.Running = false
	table.Replay = false
	table.ExtraOptions.CustomDeck = layout.deckOrder
	table.ExtraOptions.ResearchGameSeed = strconv.Itoa(gameSeed)
	table.ExtraOptions.ResearchSeatOrder = append([]int(nil), layout.seatOrder...)
	table.ExtraOptions.ResearchRosterPlayerToSeatID = copyStringMap(layout.rosterPlayerToSeatID)
	return nil
}

func researchUpdateJoinTokensForLayout(session researchSessionLifecycleSnapshot) {
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	for _, join := range researchJoinTokens {
		if join.GameID != session.GameID {
			continue
		}
		for seatIndex, controllerID := range session.ControllerRosterPlayerIDsBySeat {
			if controllerID != join.RosterPlayerID {
				continue
			}
			join.SeatIndex = seatIndex
			join.Username = session.RosterPlayerNamesBySeat[seatIndex]
			break
		}
	}
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
	researchSessionsMutex.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Research session is not valid."})
		return
	}
	lifecycle := session.lifecycleSnapshot()
	if lifecycle.MutationInProgress {
		c.JSON(http.StatusConflict, gin.H{"detail": "A research lifecycle mutation is in progress."})
		return
	}
	if !lifecycle.BotRosterPlayerIDs[payload.RosterPlayerID] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Research Bot Join Sessions can only be minted for bot Roster Players."})
		return
	}

	seatIndex := -1
	for index, controllerID := range lifecycle.ControllerRosterPlayerIDsBySeat {
		if controllerID == payload.RosterPlayerID {
			seatIndex = index
			break
		}
	}
	if seatIndex < 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot Roster Player is not assigned to a seat."})
		return
	}

	rosterIndex := 0
	if seatIndex < len(lifecycle.SeatOrder) {
		rosterIndex = lifecycle.SeatOrder[seatIndex]
	}
	username := payload.RosterPlayerID
	if seatIndex < len(lifecycle.RosterPlayerNamesBySeat) && lifecycle.RosterPlayerNamesBySeat[seatIndex] != "" {
		username = lifecycle.RosterPlayerNamesBySeat[seatIndex]
	}
	researchSessionsMutex.Lock()
	token := researchNewJoinToken()
	join := &ResearchJoinToken{
		Token:          token,
		GameID:         lifecycle.GameID,
		TableID:        lifecycle.TableID,
		RosterPlayerID: payload.RosterPlayerID,
		RosterIndex:    rosterIndex,
		SeatIndex:      seatIndex,
		UserID:         researchUserIDForTableSeat(lifecycle.TableID, seatIndex),
		Username:       username,
	}
	researchJoinTokens[token] = join
	researchGuestUsers[join.UserID] = join
	researchSessionsMutex.Unlock()

	c.JSON(http.StatusCreated, CreatedResearchBotJoinSession{
		GameID:         lifecycle.GameID,
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
	lifecycle := session.lifecycleSnapshot()
	if lifecycle.MutationInProgress {
		c.JSON(http.StatusConflict, gin.H{"detail": "A research lifecycle mutation is in progress."})
		return
	}
	if !lifecycle.BotRosterPlayerIDs[payload.RosterPlayerID] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Bot action roster_player_id is not a bot Roster Player."})
		return
	}

	ctx := context.Background()
	table, exists := getTableAndLock(ctx, nil, lifecycle.TableID, true, true)
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
	currentControllerID := lifecycle.ControllerRosterPlayerIDsBySeat[game.ActivePlayerIndex]
	if payload.RosterPlayerID != currentControllerID {
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
	if join.Unified {
		path = fmt.Sprintf("/unified-game/%d?researchMagicJoin=%s", join.TableID, joinQuery)
	} else if table, ok := tables.Get(join.TableID, true); ok {
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
	if payload.Unified {
		if payload.Mode != "single_game" {
			return nil, fmt.Errorf("unified mode requires a single_game session.")
		}
		for _, player := range payload.RosterPlayers {
			if player.Type != "human" || player.Location != "local" {
				return nil, fmt.Errorf("unified mode requires every Roster Player to be a local human.")
			}
		}
	}
	layout, err := validateResearchLayout(payload.SeededInitialLayout, len(payload.RosterPlayers))
	if err != nil {
		return nil, err
	}
	if err := validateResearchSeatPresentationAndControllers(payload, layout); err != nil {
		return nil, err
	}
	return layout, nil
}

func validateResearchSeatPresentationAndControllers(payload ResearchCreatePayload, layout *validatedResearchLayout) error {
	if payload.Game.IdentityDisplay == "seat_names" {
		if len(payload.Game.SeatDisplayNames) != len(payload.RosterPlayers) {
			return fmt.Errorf("seat_display_names must contain exactly one name per physical seat.")
		}
		for _, name := range payload.Game.SeatDisplayNames {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("seat_display_names must contain non-empty names.")
			}
		}
	} else if len(payload.Game.SeatDisplayNames) != 0 {
		return fmt.Errorf("seat_display_names requires identity_display seat_names.")
	}

	assignments := researchResolvedControllerSeatAssignments(payload, layout)
	if len(assignments) != len(payload.RosterPlayers) {
		return fmt.Errorf("controller_seat_assignments must assign every configured controller.")
	}
	seenSeats := make(map[string]bool)
	for _, player := range payload.RosterPlayers {
		seatID, ok := assignments[player.RosterPlayerID]
		if !ok {
			return fmt.Errorf("controller_seat_assignments must assign every configured controller.")
		}
		seatIndex, err := researchSeatIndex(seatID, len(payload.RosterPlayers))
		if err != nil || seenSeats[seatID] {
			return fmt.Errorf("controller_seat_assignments must be a permutation of physical seats.")
		}
		seenSeats[seatID] = true
		if player.RequiredSeat != nil && *player.RequiredSeat != seatIndex {
			return fmt.Errorf("controller_seat_assignments must satisfy human required_seat.")
		}
	}
	return nil
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
	playerByID := make(map[string]ResearchRosterPlayer, len(payload.RosterPlayers))
	for _, player := range payload.RosterPlayers {
		playerByID[player.RosterPlayerID] = player
	}
	controllerIDsBySeat := researchControllerRosterPlayerIDsBySeat(payload, layout)

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
		controller := playerByID[controllerIDsBySeat[seatIndex]]
		userID := researchUserIDForTableSeat(table.ID, seatIndex)
		username := researchDisplayName(payload.Game, rosterPlayer, seatIndex)
		session := NewFakeSession(userID, username)
		present := controller.Type == "bot"
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
	if payload.Mode == "single_game" {
		table.ExtraOptions.ResearchPersistentSingleGame = true
		table.ExtraOptions.ResearchRestartControllerID = researchRestartControllerUserID(table, payload, layout)
	}
	tables.Set(table.ID, table)
	return table, nil
}

func researchRegisterJoinLinks(gameID string, tableID uint64, payload ResearchCreatePayload, layout *validatedResearchLayout) map[string]string {
	links := make(map[string]string)
	assignments := researchResolvedControllerSeatAssignments(payload, layout)
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	for _, player := range payload.RosterPlayers {
		if player.Type != "human" {
			continue
		}
		seatIndex, _ := researchSeatIndex(assignments[player.RosterPlayerID], len(payload.RosterPlayers))
		token := researchNewJoinToken()
		join := &ResearchJoinToken{
			Token:          token,
			GameID:         gameID,
			TableID:        tableID,
			RosterPlayerID: player.RosterPlayerID,
			RosterIndex:    player.RosterIndex,
			SeatIndex:      seatIndex,
			UserID:         researchUserIDForTableSeat(tableID, seatIndex),
			Username:       researchDisplayName(payload.Game, player, seatIndex),
		}
		researchJoinTokens[token] = join
		researchGuestUsers[join.UserID] = join
		links[player.RosterPlayerID] = fmt.Sprintf("%s/join/%s", researchPublicBaseURL(), token)
	}
	return links
}

func researchRegisterUnifiedJoinLink(
	gameID string,
	tableID uint64,
	numPlayers int,
) (string, *ResearchJoinToken) {
	token := researchNewJoinToken()
	join := &ResearchJoinToken{
		Token:       token,
		GameID:      gameID,
		TableID:     tableID,
		RosterIndex: -1,
		SeatIndex:   -1,
		UserID:      researchUserIDForTableSeat(tableID, numPlayers+1),
		Username:    "Unified Manual Player",
		Unified:     true,
	}
	researchSessionsMutex.Lock()
	researchJoinTokens[token] = join
	researchGuestUsers[join.UserID] = join
	researchSessionsMutex.Unlock()
	return fmt.Sprintf("%s/join/%s", researchPublicBaseURL(), token), join
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

func researchDisplayName(game ResearchGamePayload, player ResearchRosterPlayer, seatIndex int) string {
	if game.IdentityDisplay == "seat_names" {
		return game.SeatDisplayNames[seatIndex]
	}
	if game.IdentityDisplay == "show_display_names" {
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

func researchRosterPlayerNamesBySeat(players []ResearchRosterPlayer, seatOrder []int, game ResearchGamePayload) []string {
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range players {
		playerByRosterIndex[player.RosterIndex] = player
	}
	names := make([]string, 0, len(seatOrder))
	for seatIndex, rosterIndex := range seatOrder {
		names = append(names, researchDisplayName(game, playerByRosterIndex[rosterIndex], seatIndex))
	}
	return names
}

func researchResolvedControllerSeatAssignments(payload ResearchCreatePayload, layout *validatedResearchLayout) map[string]string {
	if len(payload.ControllerSeatAssignments) != 0 {
		return copyStringMap(payload.ControllerSeatAssignments)
	}
	assignments := make(map[string]string, len(payload.RosterPlayers))
	playerByRosterIndex := make(map[int]ResearchRosterPlayer, len(payload.RosterPlayers))
	for _, player := range payload.RosterPlayers {
		playerByRosterIndex[player.RosterIndex] = player
	}
	for seatIndex, rosterIndex := range layout.seatOrder {
		assignments[playerByRosterIndex[rosterIndex].RosterPlayerID] = fmt.Sprintf("seat_%d", seatIndex)
	}
	return assignments
}

func researchControllerRosterPlayerIDsBySeat(payload ResearchCreatePayload, layout *validatedResearchLayout) []string {
	assignments := researchResolvedControllerSeatAssignments(payload, layout)
	controllers := make([]string, len(payload.RosterPlayers))
	for controllerID, seatID := range assignments {
		seatIndex, _ := researchSeatIndex(seatID, len(controllers))
		controllers[seatIndex] = controllerID
	}
	return controllers
}

func researchSeatIndex(seatID string, numPlayers int) (int, error) {
	if !strings.HasPrefix(seatID, "seat_") {
		return 0, fmt.Errorf("invalid physical seat")
	}
	seatIndex, err := strconv.Atoi(strings.TrimPrefix(seatID, "seat_"))
	if err != nil || seatIndex < 0 || seatIndex >= numPlayers {
		return 0, fmt.Errorf("invalid physical seat")
	}
	return seatIndex, nil
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
	lifecycle := session.lifecycleSnapshot()
	status := gin.H{
		"game_id":                       lifecycle.GameID,
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
	if lifecycle.TableID != 0 {
		researchAttachTableStatus(status, lifecycle)
	}
	if lifecycle.Mode == "pregame_table" {
		status["current_game_index"] = lifecycle.CurrentGameIndex
		status["ready_status"] = lifecycle.ReadyStatus
		status["active_game_started"] = false
		status["completed_games"] = lifecycle.CompletedGames
	} else if lifecycle.Mode == "single_game" {
		status["current_game_index"] = lifecycle.CurrentGameIndex
		status["game_seed"] = lifecycle.Seed + lifecycle.CurrentGameIndex
		status["restart_request"] = lifecycle.PendingRestartRequest
	}
	return status
}

func researchRestartControllerUserID(
	table *Table,
	payload ResearchCreatePayload,
	layout *validatedResearchLayout,
) int {
	controllerRosterIndex := -1
	controllerRosterPlayerID := ""
	for _, player := range payload.RosterPlayers {
		if player.Type == "human" && (controllerRosterIndex < 0 || player.RosterIndex < controllerRosterIndex) {
			controllerRosterIndex = player.RosterIndex
			controllerRosterPlayerID = player.RosterPlayerID
		}
	}
	for seatIndex, controllerID := range researchControllerRosterPlayerIDsBySeat(payload, layout) {
		if controllerID == controllerRosterPlayerID {
			return table.Players[seatIndex].UserID
		}
	}
	return 0
}

func researchAttachTableStatus(status gin.H, session researchSessionLifecycleSnapshot) {
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
		status["current_turn_controller_roster_player_id"] = session.ControllerRosterPlayerIDsBySeat[game.ActivePlayerIndex]
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
	if join.Unified {
		researchHandleUnifiedGuestConnected(s, join)
		return
	}
	if join.SeatIndex < 0 {
		commandTableSpectate(NewSessionContext(s), s, &CommandData{
			TableID:              join.TableID,
			ShadowingPlayerIndex: -1,
		})
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

func researchHandleUnifiedGuestConnected(s *Session, join *ResearchJoinToken) {
	ctx := NewSessionContext(s)
	table, exists := getTableAndLock(ctx, s, join.TableID, true, true)
	if !exists {
		return
	}
	if !table.Running {
		for _, player := range table.Players {
			player.Present = true
		}
		tableStart(ctx, table.Players[0].Session, &CommandData{
			TableID:      table.ID,
			NoTableLock:  true,
			NoTablesLock: true,
		}, table)
	}
	controller, ok := table.researchUnifiedController(s.UserID)
	if !ok {
		table.Unlock(ctx)
		s.Warning("The unified controller is not registered for this game.")
		return
	}
	if !controller.Initialized {
		controller.ViewedSeat = table.Game.ActivePlayerIndex
		controller.SelectedBoundary = researchUnifiedLiveBoundary(table.Game)
		controller.ActionCutoffs = []int{len(table.Game.Actions)}
		controller.Initialized = true
	}
	viewedSeat := controller.ViewedSeat
	table.Unlock(ctx)
	commandTableSpectate(ctx, s, &CommandData{
		TableID:                  join.TableID,
		ShadowingPlayerIndex:     viewedSeat,
		UnifiedControlAuthorized: true,
	})
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
