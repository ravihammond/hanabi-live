package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	researchDeckColors = 5
	researchDeckRanks  = 5
)

var (
	researchDeckCountsByRank = []int{3, 2, 2, 2, 1}
	researchSessions         = make(map[string]*ResearchSession)
	researchSessionsMutex    sync.Mutex
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
	GameSeed        int    `json:"game_seed"`
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

type OpenedResearchReplay struct {
	GameID       string `json:"game_id"`
	ReplayURL    string `json:"replay_url"`
	LayoutSource string `json:"layout_source"`
	SeatOrder    []int  `json:"seat_order"`
}

type ResearchSession struct {
	GameID               string
	Mode                 string
	Seed                 int
	CurrentGameIndex     int
	ReadyStatus          map[string]bool
	CompletedGames       []map[string]interface{}
	SeatOrder            []int
	RosterPlayerToSeatID map[string]string
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
}

func researchHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

	gameID := fmt.Sprintf("single_game_%d", payload.Game.GameSeed)
	created := CreatedResearchSingleGame{
		GameID:       gameID,
		Mode:         "single_game",
		GameSeed:     payload.Game.GameSeed,
		LayoutSource: "payload",
		SeatOrder:    append([]int(nil), layout.seatOrder...),
		JoinLinks:    researchJoinLinks(payload.RosterPlayers, table.ID, "game"),
	}
	researchSessionsMutex.Lock()
	researchSessions[gameID] = &ResearchSession{
		GameID:               gameID,
		Mode:                 "single_game",
		SeatOrder:            append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID: copyStringMap(layout.rosterPlayerToSeatID),
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
		JoinLinks:        researchPregameJoinLinks(payload.RosterPlayers, tableID),
		ReadyStatus:      readyStatus,
		UsesPublicLobby:  false,
	}

	researchSessionsMutex.Lock()
	researchSessions[tableID] = &ResearchSession{
		GameID:               tableID,
		Mode:                 "pregame_table",
		Seed:                 payload.Game.Seed,
		CurrentGameIndex:     0,
		ReadyStatus:          readyStatus,
		CompletedGames:       make([]map[string]interface{}, 0),
		SeatOrder:            append([]int(nil), layout.seatOrder...),
		RosterPlayerToSeatID: copyStringMap(layout.rosterPlayerToSeatID),
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
		GameSeed            int                         `json:"game_seed"`
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
	if payload.GameSeed != session.Seed+session.CurrentGameIndex {
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
	if payload.Game.GameSeed == 0 && payload.Mode == "single_game" {
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
			SuitIndex: card.Color,
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
	playerByRosterIndex := make(map[int]ResearchRosterPlayer)
	for _, player := range payload.RosterPlayers {
		playerByRosterIndex[player.RosterIndex] = player
	}

	tables.Lock(ctx)
	defer tables.Unlock(ctx)

	tableName := fmt.Sprintf("research-%d-%d", payload.Game.GameSeed, time.Now().UnixNano())
	ownerRosterIndex := layout.seatOrder[0]
	owner := playerByRosterIndex[ownerRosterIndex]
	table := NewTable(tableName, 0)
	baseUserID := -100000 - int(table.ID)*10
	ownerID := baseUserID
	table.OwnerID = ownerID
	table.OwnerUsername = owner.RosterPlayerID
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
		ResearchGameSeed:             strconv.Itoa(payload.Game.GameSeed),
		ResearchSeatOrder:            append([]int(nil), layout.seatOrder...),
		ResearchRosterPlayerToSeatID: copyStringMap(layout.rosterPlayerToSeatID),
	}
	for seatIndex, rosterIndex := range layout.seatOrder {
		rosterPlayer := playerByRosterIndex[rosterIndex]
		userID := baseUserID - seatIndex
		session := NewFakeSession(userID, rosterPlayer.RosterPlayerID)
		sessions.Set(userID, session)
		table.Players = append(table.Players, &Player{
			UserID:    userID,
			Name:      rosterPlayer.RosterPlayerID,
			Session:   session,
			Present:   true,
			Stats:     &PregameStats{},
			LastTyped: time.Time{},
		})
		tables.AddPlaying(userID, table.ID)
	}
	tables.Set(table.ID, table)

	table.Lock(ctx)
	defer table.Unlock(ctx)
	tableStart(ctx, table.Players[0].Session, &CommandData{
		TableID:      table.ID,
		NoTableLock:  true,
		NoTablesLock: true,
	}, table)
	return table, nil
}

func researchJoinLinks(players []ResearchRosterPlayer, tableID uint64, view string) map[string]string {
	links := make(map[string]string)
	for _, player := range players {
		if player.Type != "human" {
			continue
		}
		links[player.RosterPlayerID] = fmt.Sprintf("%s/%s/%d", researchPublicBaseURL(), view, tableID)
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
	return "http://127.0.0.1:" + port
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
		"auto_action_taken":             false,
		"timeout_action_taken":          false,
		"last_action":                   nil,
	}
	if session.Mode == "pregame_table" {
		status["current_game_index"] = session.CurrentGameIndex
		status["ready_status"] = session.ReadyStatus
		status["active_game_started"] = false
		status["completed_games"] = session.CompletedGames
	}
	return status
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
