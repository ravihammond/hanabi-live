package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestResearchControlAPICreatesRunningTableFromInjectedLayout(t *testing.T) {
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
	if len(created.JoinLinks) != 1 {
		t.Fatalf("expected one human join link, got %#v", created.JoinLinks)
	}

	tableList := tables.GetList(true)
	if len(tableList) != 1 {
		t.Fatalf("expected one created table, got %d", len(tableList))
	}
	table := tableList[0]
	if !table.Running {
		t.Fatal("research-created table should be running")
	}
	if table.Players[0].Name != "roster_player_1" || table.Players[1].Name != "roster_player_0" {
		t.Fatalf("players were not assigned from Seat Order: %#v", []string{
			table.Players[0].Name,
			table.Players[1].Name,
		})
	}

	game := table.Game
	if game.Seed != "102" {
		t.Fatalf("expected public Game Seed to be recorded, got %q", game.Seed)
	}
	if len(game.Deck) != 50 {
		t.Fatalf("expected a 50-card deck, got %d", len(game.Deck))
	}
	for index, expected := range payload.SeededInitialLayout.DeckOrder {
		card := game.Deck[index]
		if card.SuitIndex != expected.Color || card.Rank != expected.Rank+1 {
			t.Fatalf(
				"deck card %d mismatch: got (%d,%d), want (%d,%d)",
				index,
				card.SuitIndex,
				card.Rank,
				expected.Color,
				expected.Rank+1,
			)
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
	isDev = true
	colorsInit()
	suitsInit()
	variantsInit()
	charactersInit()
}

func researchTestRouter() *gin.Engine {
	router := gin.New()
	registerResearchRoutes(router)
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
			GameSeed:        102,
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
