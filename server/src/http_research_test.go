package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	gsessions "github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestResearchControlAPIRejectsInvalidDeckOrder(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.SeededInitialLayout.DeckOrder = payload.SeededInitialLayout.DeckOrder[:49]

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("Deck Order must contain 50 cards")) {
		t.Fatalf("missing deck-length validation message: %s", response.Body.String())
	}
}

func TestResearchControlAPIRejectsInvalidLayoutDetails(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResearchCreatePayload)
		message string
	}{
		{
			name: "card range",
			mutate: func(payload *ResearchCreatePayload) {
				payload.SeededInitialLayout.DeckOrder[0] = ResearchCardIdentity{Color: 9, Rank: 0}
			},
			message: "outside JAXMARL card ranges",
		},
		{
			name: "card counts",
			mutate: func(payload *ResearchCreatePayload) {
				payload.SeededInitialLayout.DeckOrder[0] = ResearchCardIdentity{Color: 0, Rank: 4}
			},
			message: "Deck Order has",
		},
		{
			name: "seat order permutation",
			mutate: func(payload *ResearchCreatePayload) {
				payload.SeededInitialLayout.SeatOrder = []int{0, 0}
			},
			message: "Seat Order must be a permutation",
		},
		{
			name: "assignment map",
			mutate: func(payload *ResearchCreatePayload) {
				payload.SeededInitialLayout.RosterPlayerToSeatID = map[string]string{
					"0": "seat_0",
					"1": "seat_1",
				}
			},
			message: "assignment must be derived from Seat Order",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			researchTestInit(t)
			router := researchTestRouter()
			payload := researchSingleGamePayload()
			test.mutate(&payload)

			response := researchJSONRequest(
				t,
				router,
				http.MethodPost,
				"/research/single-game",
				payload,
				"secret",
			)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
			}
			if !bytes.Contains(response.Body.Bytes(), []byte(test.message)) {
				t.Fatalf("missing validation message %q: %s", test.message, response.Body.String())
			}
		})
	}
}

func TestResearchControlAPIRequiresAdminToken(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		researchSingleGamePayload(),
		"",
	)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("Admin token is required")) {
		t.Fatalf("missing admin-token validation message: %s", response.Body.String())
	}
}

func TestResearchWebsocketConnectDataDefaultsToJSONArrays(t *testing.T) {
	data := newWebsocketConnectData()
	if !data.Settings.KeldonMode {
		t.Fatal("expected websocket defaults to use Keldon mode")
	}
	payload, err := json.Marshal(struct {
		Friends         []string `json:"friends"`
		PlayingAtTables []uint64 `json:"playingAtTables"`
	}{
		Friends:         data.FriendsList,
		PlayingAtTables: data.PlayingAtTables,
	})
	if err != nil {
		t.Fatalf("marshal welcome defaults: %v", err)
	}

	expected := `{"friends":[],"playingAtTables":[]}`
	if string(payload) != expected {
		t.Fatalf("expected welcome defaults to marshal as %s, got %s", expected, string(payload))
	}
}

func TestResearchControlAPIAcceptsZeroGameSeed(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.Game.Seed = 0
	payload.Game.GameIndex = 0
	payload.Game.GameSeed = researchIntPtr(0)

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	if created.GameSeed != 0 || created.GameID != "single_game_0" {
		t.Fatalf("expected zero Game Seed session, got %#v", created)
	}
}

func TestResearchControlAPIRejectsMissingGameSeedMetadata(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.Game.GameSeed = nil

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("game.game_seed metadata")) {
		t.Fatalf("missing game_seed validation message: %s", response.Body.String())
	}
}

func TestResearchControlAPICreatesWaitingTableWithMagicJoinLink(t *testing.T) {
	t.Setenv("DOMAIN", "localhost")
	t.Setenv("PORT", "1212")
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}

	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	if created.GameSeed != 102 {
		t.Fatalf("expected Game Seed 102, got %d", created.GameSeed)
	}
	if created.TableID == 0 {
		t.Fatal("expected response to expose the created table ID")
	}
	if len(created.JoinLinks) != 1 {
		t.Fatalf("expected one human join link, got %#v", created.JoinLinks)
	}
	if !bytes.Contains([]byte(created.JoinLinks["roster_player_0"]), []byte("/join/")) {
		t.Fatalf("expected a Research Magic Join Link, got %#v", created.JoinLinks)
	}
	if !bytes.HasPrefix([]byte(created.JoinLinks["roster_player_0"]), []byte("http://localhost:1212/join/")) {
		t.Fatalf("expected configured local hostname in join link, got %#v", created.JoinLinks)
	}

	tableList := tables.GetList(true)
	if len(tableList) != 1 {
		t.Fatalf("expected one created table, got %d", len(tableList))
	}
	table := tableList[0]
	if table.Running {
		t.Fatal("research-created table should wait for the human magic-join before starting")
	}
	if table.Visible {
		t.Fatal("research-created table should not be visible in the public lobby")
	}
	if table.Players[0].Present != true || table.Players[1].Present != false {
		t.Fatalf("expected bot seat present and human seat pending, got %#v", []bool{
			table.Players[0].Present,
			table.Players[1].Present,
		})
	}
	if table.Players[0].Name != "Player 1" || table.Players[1].Name != "Player 2" {
		t.Fatalf("players were not assigned from Seat Order: %#v", []string{
			table.Players[0].Name,
			table.Players[1].Name,
		})
	}
	for _, player := range table.Players {
		if player.Stats == nil || player.Stats.Variant == nil {
			t.Fatalf("expected research player stats to include variant stats: %#v", player)
		}
	}
}

func TestResearchMagicJoinGuestConnectionAutoStartsInjectedTable(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}

	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	token := path.Base(created.JoinLinks["roster_player_0"])
	join, ok := researchJoinTokens[token]
	if !ok {
		t.Fatalf("magic join token was not registered: %s", token)
	}

	redirect := httptest.NewRecorder()
	router.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/join/"+token, nil))
	if redirect.Code != http.StatusFound {
		t.Fatalf("expected magic join redirect, got %d: %s", redirect.Code, redirect.Body.String())
	}
	expectedLocation := "/pre-game/" + strconv.FormatUint(created.TableID, 10) + "?researchMagicJoin=" + token
	if redirect.Header().Get("Location") != expectedLocation {
		t.Fatalf("expected magic join redirect to %q, got %q", expectedLocation, redirect.Header().Get("Location"))
	}
	userID, username, ok := researchMagicJoinTokenCredentials(token)
	if !ok {
		t.Fatal("expected magic join token to resolve websocket credentials")
	}
	if userID != join.UserID || username != join.Username {
		t.Fatalf(
			"expected token credentials (%d,%q), got (%d,%q)",
			join.UserID,
			join.Username,
			userID,
			username,
		)
	}

	session := NewFakeSession(join.UserID, join.Username)
	researchHandleGuestConnected(session)

	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	defer table.Unlock(nil)

	if !table.Running {
		t.Fatal("table should auto-start after every reserved seat is present")
	}
	if table.Players[1].Session != session {
		t.Fatal("magic-joined guest session was not bound to its reserved human seat")
	}
	if table.Game.Seed != "102" {
		t.Fatalf("expected public Game Seed to be recorded, got %q", table.Game.Seed)
	}
	if len(table.Game.Deck) != 50 {
		t.Fatalf("expected a 50-card deck, got %d", len(table.Game.Deck))
	}
	for index, expected := range payload.SeededInitialLayout.DeckOrder {
		card := table.Game.Deck[index]
		expectedSuitIndex := researchExpectedLiveSuitIndexForJAXMARLColor(expected.Color)
		if card.SuitIndex != expectedSuitIndex || card.Rank != expected.Rank+1 {
			t.Fatalf(
				"deck card %d mismatch: got (%d,%d), want (%d,%d)",
				index,
				card.SuitIndex,
				card.Rank,
				expectedSuitIndex,
				expected.Rank+1,
			)
		}
	}
}

func TestResearchTunnelWebSocketHealthAcceptsTrycloudflareOrigin(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/research/tunnel-ws-health"
	headers := http.Header{}
	headers.Set("Origin", "https://quiet-river-123.trycloudflare.com")

	conn, response, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		status := "<nil>"
		if response != nil {
			status = response.Status
		}
		t.Fatalf("expected tunnel websocket health to connect, got %s: %v", status, err)
	}
	defer conn.Close()

	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected websocket health message: %v", err)
	}
	if string(message) != "ok" {
		t.Fatalf("expected websocket health message ok, got %q", string(message))
	}
}

func TestResearchBotActionEndpointAppliesLegalBotMove(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	token := path.Base(created.JoinLinks["roster_player_0"])
	join := researchJoinTokens[token]
	researchHandleGuestConnected(NewFakeSession(join.UserID, join.Username))

	statusResponse := researchJSONRequest(
		t,
		router,
		http.MethodGet,
		"/research/sessions/"+created.GameID+"/status",
		nil,
		"secret",
	)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status map[string]interface{}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse status response: %v", err)
	}
	if status["current_turn_roster_player_id"] != "roster_player_1" {
		t.Fatalf("expected bot to take the first turn, got %#v", status["current_turn_roster_player_id"])
	}
	legalActions := status["legal_actions"].([]interface{})
	if len(legalActions) == 0 {
		t.Fatal("expected legal bot actions")
	}

	actionResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/bot-action",
		map[string]interface{}{
			"roster_player_id": "roster_player_1",
			"action":           legalActions[0].(string),
		},
		"secret",
	)
	if actionResponse.Code != http.StatusOK {
		t.Fatalf("expected bot action 200, got %d: %s", actionResponse.Code, actionResponse.Body.String())
	}

	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	defer table.Unlock(nil)
	game := table.Game
	if len(game.Actions2) != 1 {
		t.Fatalf("expected one applied game action, got %d", len(game.Actions2))
	}
}

func TestResearchControlAPIMintsBotJoinSession(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}

	joinResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/bot-join-session",
		map[string]interface{}{"roster_player_id": "roster_player_1"},
		"secret",
	)
	if joinResponse.Code != http.StatusCreated {
		t.Fatalf("expected bot join-session 201, got %d: %s", joinResponse.Code, joinResponse.Body.String())
	}
	var joined CreatedResearchBotJoinSession
	if err := json.Unmarshal(joinResponse.Body.Bytes(), &joined); err != nil {
		t.Fatalf("failed to parse bot join-session response: %v", err)
	}
	if joined.GameID != created.GameID {
		t.Fatalf("expected game ID %s, got %#v", created.GameID, joined)
	}
	if joined.RosterPlayerID != "roster_player_1" {
		t.Fatalf("expected bot roster player, got %#v", joined)
	}
	if joined.JoinCredential == "" {
		t.Fatalf("expected non-empty join credential, got %#v", joined)
	}
	if joined.OurPlayerIndex != 0 {
		t.Fatalf("expected bot to own seat index 0, got %#v", joined)
	}
	userID, username, ok := researchMagicJoinTokenCredentials(joined.JoinCredential)
	if !ok {
		t.Fatal("expected bot join credential to resolve websocket credentials")
	}
	if username == "" || userID == 0 {
		t.Fatalf("expected concrete bot credentials, got userID=%d username=%q", userID, username)
	}
	oldSession, ok := sessions.Get(userID)
	if !ok || !oldSession.FakeUser {
		t.Fatalf("expected bot seat to start with a reserved fake session, got %#v", oldSession)
	}
	realSession := NewFakeSession(userID, username)
	realSession.FakeUser = false
	if !researchKeepsTableSeatOnSessionReplacement(realSession, oldSession) {
		t.Fatal("expected native bot websocket replacement to preserve table membership")
	}
	websocketDisconnectRemoveFromMap(oldSession)
	sessions.Set(realSession.UserID, realSession)
	researchHandleGuestConnected(realSession)
	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	if table.Players[0].Session != realSession {
		t.Fatal("native bot websocket session was not rebound to its reserved seat")
	}
	if !table.Players[0].Present {
		t.Fatal("native bot websocket session should keep its reserved seat present")
	}
	table.Unlock(nil)

	humanResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/bot-join-session",
		map[string]interface{}{"roster_player_id": "roster_player_0"},
		"secret",
	)
	if humanResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected human join-session rejection 422, got %d: %s", humanResponse.Code, humanResponse.Body.String())
	}
}

func TestResearchControlAPIMintsPregameBotJoinSessionForActualTable(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.Mode = "pregame_table"
	payload.Game.GameIndex = 0
	payload.Game.GameSeed = researchIntPtr(payload.Game.Seed)

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/pregame-table",
		payload,
		"secret",
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchPregameTable
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}

	joinResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.TableID+"/bot-join-session",
		map[string]interface{}{"roster_player_id": "roster_player_1"},
		"secret",
	)
	if joinResponse.Code != http.StatusCreated {
		t.Fatalf("expected bot join-session 201, got %d: %s", joinResponse.Code, joinResponse.Body.String())
	}
	var joined CreatedResearchBotJoinSession
	if err := json.Unmarshal(joinResponse.Body.Bytes(), &joined); err != nil {
		t.Fatalf("failed to parse bot join-session response: %v", err)
	}
	join, ok := researchJoinTokens[joined.JoinCredential]
	if !ok {
		t.Fatal("expected bot join credential to register a magic-join token")
	}
	if join.TableID == 0 {
		t.Fatalf("expected bot join credential to bind to a real Hanabi.live table, got %#v", join)
	}
	table, ok := tables.Get(join.TableID, true)
	if !ok {
		t.Fatalf("expected table %d to exist for Pregame Table join session", join.TableID)
	}
	table.Lock(nil)
	if table.Players[join.SeatIndex].UserID != join.UserID {
		t.Fatalf("expected join user %d in seat %d, got table players %#v", join.UserID, join.SeatIndex, table.Players)
	}
	table.Unlock(nil)
}

func TestResearchLegalActionsIncludeEveryVisibleClue(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	token := path.Base(created.JoinLinks["roster_player_0"])
	join := researchJoinTokens[token]
	researchHandleGuestConnected(NewFakeSession(join.UserID, join.Username))

	statusResponse := researchJSONRequest(
		t,
		router,
		http.MethodGet,
		"/research/sessions/"+created.GameID+"/status",
		nil,
		"secret",
	)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status map[string]interface{}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse status response: %v", err)
	}
	legalActions := status["legal_actions"].([]interface{})
	legalActionSet := make(map[string]bool, len(legalActions))
	for _, action := range legalActions {
		legalActionSet[action.(string)] = true
	}

	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	targetHand := append([]*Card(nil), table.Game.Players[1].Hand...)
	table.Unlock(nil)

	for _, card := range targetHand {
		expectedRankClue := researchEncodeAction(ResearchBotAction{
			Type:   ActionTypeRankClue,
			Target: 1,
			Value:  card.Rank,
		})
		if !legalActionSet[expectedRankClue] {
			t.Fatalf("missing legal rank clue %s from %#v", expectedRankClue, legalActions)
		}
		expectedColorClue := researchEncodeAction(ResearchBotAction{
			Type:   ActionTypeColorClue,
			Target: 1,
			Value:  card.SuitIndex,
		})
		if !legalActionSet[expectedColorClue] {
			t.Fatalf("missing legal color clue %s from %#v", expectedColorClue, legalActions)
		}
	}
}

func researchTestInit(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	os.Setenv("HANABI_LIVE_ADMIN_TOKEN", "secret")
	projectPath = path.Clean("../..")
	jsonPath = path.Join(projectPath, "packages", "game", "src", "json")
	tables = NewTables()
	sessions = NewSessions()
	researchSessions = make(map[string]*ResearchSession)
	researchJoinTokens = make(map[string]*ResearchJoinToken)
	researchGuestUsers = make(map[int]*ResearchJoinToken)
	isDev = true
	colorsInit()
	suitsInit()
	variantsInit()
	charactersInit()
	actionsFunctionsInit()
}

func researchTestRouter() *gin.Engine {
	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret"))
	router.Use(gsessions.Sessions(HTTPSessionName, store))
	registerResearchRoutes(router)
	registerResearchPublicRoutes(router)
	return router
}

func researchJSONRequest(
	t *testing.T,
	router *gin.Engine,
	method string,
	url string,
	payload interface{},
	adminToken string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal request payload: %v", err)
	}
	request := httptest.NewRequest(method, url, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if adminToken != "" {
		request.Header.Set("Authorization", "Bearer "+adminToken)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func researchSingleGamePayload() ResearchCreatePayload {
	return ResearchCreatePayload{
		Mode: "single_game",
		Game: ResearchGamePayload{
			Seed:            100,
			GameIndex:       2,
			GameSeed:        researchIntPtr(102),
			IdentityDisplay: "anonymous",
			ChatEnabled:     false,
		},
		RosterPlayers: []ResearchRosterPlayer{
			{
				RosterIndex:    0,
				RosterPlayerID: "roster_player_0",
				Type:           "human",
				Location:       "local",
				DisplayName:    "Ada",
			},
			{
				RosterIndex:    1,
				RosterPlayerID: "roster_player_1",
				Type:           "bot",
				ModelPath:      "/models/random",
			},
		},
		SeededInitialLayout: ResearchSeededInitialLayout{
			DeckOrder: researchValidDeck(),
			SeatOrder: []int{1, 0},
			RosterPlayerToSeatID: map[string]string{
				"0": "seat_1",
				"1": "seat_0",
			},
		},
	}
}

func researchIntPtr(value int) *int {
	return &value
}

func researchValidDeck() []ResearchCardIdentity {
	counts := []int{3, 2, 2, 2, 1}
	deck := make([]ResearchCardIdentity, 0, 50)
	for color := 0; color < 5; color++ {
		for rank, count := range counts {
			for i := 0; i < count; i++ {
				deck = append(deck, ResearchCardIdentity{Color: color, Rank: rank})
			}
		}
	}
	return deck
}

func researchExpectedLiveSuitIndexForJAXMARLColor(color int) int {
	if color == 3 {
		return 4
	}
	if color == 4 {
		return 3
	}
	return color
}
