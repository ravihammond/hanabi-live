package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

type researchRecordingOutbound struct {
	ready    bool
	writeErr error
	messages []string
}

type researchHSMResponseWire struct {
	ProtocolVersion                int    `json:"protocolVersion"`
	ServerRequestID                int    `json:"serverRequestID"`
	ClientRequestID                int    `json:"clientRequestID"`
	ArchiveGenerationID            uint32 `json:"archiveGenerationID"`
	TargetBoundary                 int    `json:"targetBoundary"`
	EvidenceBoundary               int    `json:"evidenceBoundary"`
	PerspectivePlayer              int    `json:"perspectivePlayer"`
	ActorPlayer                    int    `json:"actorPlayer"`
	SemanticProfileID              int    `json:"semanticProfileID"`
	AuthorityLegalProjectionDigest string `json:"authorityLegalProjectionDigest"`
}

type researchHSMSnapshotWire struct {
	researchHSMResponseWire
	Snapshot ResearchHSMSnapshot `json:"snapshot"`
}

type researchHSMSnapshotFailureWire struct {
	researchHSMResponseWire
	Error   string             `json:"error"`
	Failure ResearchHSMFailure `json:"failure"`
}

type researchHSMPhysicalTruthWireIdentity struct {
	ProtocolVersion     int    `json:"protocolVersion"`
	ServerRequestID     int    `json:"serverRequestID"`
	ClientRequestID     int    `json:"clientRequestID"`
	ArchiveGenerationID uint32 `json:"archiveGenerationID"`
	TargetBoundary      int    `json:"targetBoundary"`
	PerspectivePlayer   int    `json:"perspectivePlayer"`
}

type researchHSMPhysicalTruthWire struct {
	researchHSMPhysicalTruthWireIdentity
	Overlay ResearchHSMPhysicalTruthOverlay `json:"overlay"`
}

type researchHSMPhysicalTruthFailureWire struct {
	researchHSMPhysicalTruthWireIdentity
	Error string `json:"error"`
}

type researchHSMTransportGolden struct {
	ProtocolVersion       int                                  `json:"protocolVersion"`
	SnapshotPending       researchHSMResponseWire              `json:"snapshotPending"`
	SnapshotMessage       researchHSMSnapshotWire              `json:"snapshotMessage"`
	SnapshotFailure       researchHSMSnapshotFailureWire       `json:"snapshotFailure"`
	SnapshotRejected      ResearchHSMRequestRejection          `json:"snapshotRejected"`
	PhysicalTruthPending  researchHSMPhysicalTruthWireIdentity `json:"physicalTruthPending"`
	PhysicalTruthMessage  researchHSMPhysicalTruthWire         `json:"physicalTruthMessage"`
	PhysicalTruthFailure  researchHSMPhysicalTruthFailureWire  `json:"physicalTruthFailure"`
	PhysicalTruthRejected ResearchHSMPhysicalTruthRejection    `json:"physicalTruthRejected"`
}

func (outbound *researchRecordingOutbound) Ready() bool {
	return outbound.ready
}

func (outbound *researchRecordingOutbound) Write(message []byte) error {
	if outbound.writeErr != nil {
		return outbound.writeErr
	}
	outbound.messages = append(outbound.messages, string(message))
	return nil
}

func researchHSMTestViewer(join *ResearchJoinToken) *Session {
	viewer := NewFakeSession(join.UserID, join.Username)
	viewer.outbound = &researchRecordingOutbound{ready: true}
	return viewer
}

func TestSingleGameRestartControllerRequestAppearsInStatus(t *testing.T) {
	researchTestInit(t)
	commandInit()
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
	controller := NewFakeSession(join.UserID, join.Username)
	researchHandleGuestConnected(controller)
	commandResearchRestart(context.Background(), controller, &CommandData{
		TableID:     created.TableID,
		RestartKind: "same_seed",
	})

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
	request, ok := status["restart_request"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected pending restart request, got %#v", status["restart_request"])
	}
	if request["request_id"] != float64(1) || request["kind"] != "same_seed" {
		t.Fatalf("unexpected restart request: %#v", request)
	}
	if status["current_game_index"] != float64(2) || status["game_seed"] != float64(102) {
		t.Fatalf("expected current index/seed 2/102, got %#v", status)
	}
}

func TestSingleGameCreatesOnlyAuthorizedHSMJoinLinks(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = "switchable"
	payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
		Identity:              "hsm_debug_spectator",
		Capability:            "switchable",
		HSMPhysicalTruthGrant: true,
	}

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

	if _, ok := created.JoinLinks["roster_player_0"]; !ok {
		t.Fatal("authorized Debug Participant did not receive its normal join link")
	}
	if _, ok := created.JoinLinks["hsm_debug_spectator"]; !ok {
		t.Fatal("configured HSM Debug Spectator did not receive a join link")
	}
	if _, ok := created.JoinLinks["roster_player_1"]; ok {
		t.Fatal("bot or unauthorized Roster Player received a browser join link")
	}

	participantToken := path.Base(created.JoinLinks["roster_player_0"])
	participant := researchJoinTokens[participantToken]
	if participant.HSMDebugCapability != "switchable" ||
		participant.HSMIdentity != "roster_player_0" ||
		participant.HSMPhysicalTruthGrant {
		t.Fatalf("participant capability was not bound to its join token: %#v", participant)
	}
	spectatorToken := path.Base(created.JoinLinks["hsm_debug_spectator"])
	spectator := researchJoinTokens[spectatorToken]
	if spectator.SeatIndex != -1 ||
		spectator.HSMDebugCapability != "switchable" ||
		!spectator.HSMPhysicalTruthGrant {
		t.Fatalf("debug spectator must remain seatless and switchable: %#v", spectator)
	}
	participantInit := researchHSMDebugInitForUser(participant.UserID)
	spectatorInit := researchHSMDebugInitForUser(spectator.UserID)
	if participantInit.HSMArchiveGenerationID != created.HSMArchiveGenerationID ||
		participantInit.PhysicalTruthGranted {
		t.Fatalf("participant initialization widened authority: %#v", participantInit)
	}
	if spectatorInit.HSMArchiveGenerationID != created.HSMArchiveGenerationID ||
		!spectatorInit.PhysicalTruthGranted {
		t.Fatalf("spectator initialization lost its explicit grant: %#v", spectatorInit)
	}
}

func TestSingleGameRejectsInvalidHSMViewerAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResearchCreatePayload)
	}{
		{
			name: "unknown participant capability",
			mutate: func(payload *ResearchCreatePayload) {
				payload.RosterPlayers[0].HSMDebugCapability = "own-perspective"
			},
		},
		{
			name: "unknown spectator capability",
			mutate: func(payload *ResearchCreatePayload) {
				payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
					Identity:   "auditor",
					Capability: "omniscient",
				}
			},
		},
		{
			name: "seatless spectator cannot have own-perspective capability",
			mutate: func(payload *ResearchCreatePayload) {
				payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
					Identity:   "auditor",
					Capability: ResearchHSMCapabilityOwnPerspective,
				}
			},
		},
		{
			name: "spectator identity collides with roster identity",
			mutate: func(payload *ResearchCreatePayload) {
				payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
					Identity:   payload.RosterPlayers[0].RosterPlayerID,
					Capability: "switchable",
				}
			},
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
				t.Fatalf(
					"expected invalid HSM authorization to return 422, got %d: %s",
					response.Code,
					response.Body.String(),
				)
			}
		})
	}
}

func TestPregameTableRejectsHSMViewerAuthorization(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.Mode = "pregame_table"
	payload.Game.GameIndex = 0
	payload.Game.GameSeed = nil
	payload.RosterPlayers[0].HSMDebugCapability =
		ResearchHSMCapabilityOwnPerspective

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/pregame-table",
		payload,
		"secret",
	)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"expected pregame HSM authorization to be rejected with 422, got %d: %s",
			response.Code,
			response.Body.String(),
		)
	}
}

func TestHSMViewerRoleAndPrincipalAreSeparateFromDisplayIdentity(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilitySwitchable
	payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
		Identity:   "audit-viewer",
		Capability: ResearchHSMCapabilitySwitchable,
	}

	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	participant := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	spectator := researchJoinTokens[path.Base(created.JoinLinks["audit-viewer"])]
	if participant.HSMPrincipalID == "" ||
		spectator.HSMPrincipalID == "" ||
		participant.HSMPrincipalID == spectator.HSMPrincipalID {
		t.Fatalf("viewer principals are missing or not unique: %#v %#v", participant, spectator)
	}

	participantInit := researchHSMDebugInitForUser(participant.UserID)
	spectatorInit := researchHSMDebugInitForUser(spectator.UserID)
	if participantInit.ViewerKind != ResearchHSMViewerKindParticipant {
		t.Fatalf("participant viewer kind was inferred incorrectly: %#v", participantInit)
	}
	if spectatorInit.ViewerKind != ResearchHSMViewerKindSpectator {
		t.Fatalf("arbitrary spectator identity lost its explicit viewer kind: %#v", spectatorInit)
	}
}

func TestHSMViewerKindCannotBecomePlayerAuthorityOrPlayerChatIdentity(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
		Identity:   "audit-viewer",
		Capability: ResearchHSMCapabilitySwitchable,
	}
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	participantJoin :=
		researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	researchHandleGuestConnected(
		NewFakeSession(participantJoin.UserID, participantJoin.Username),
	)
	spectatorJoin := researchJoinTokens[path.Base(created.JoinLinks["audit-viewer"])]
	spectator := researchHSMTestViewer(spectatorJoin)
	researchHandleGuestConnected(spectator)

	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d is missing", created.TableID)
	}
	table.Lock(nil)
	beforeActions := len(table.Game.Actions2)
	table.Unlock(nil)
	commandAction(context.Background(), spectator, &CommandData{
		TableID: created.TableID,
		Type:    ActionTypePlay,
		Target:  0,
	})
	commandResearchRestart(context.Background(), spectator, &CommandData{
		TableID:     created.TableID,
		RestartKind: researchRestartSameSeed,
	})
	commandChat(context.Background(), spectator, &CommandData{
		Room:     "table" + strconv.FormatUint(created.TableID, 10),
		Msg:      "observer note",
		Username: participantJoin.Username,
	})

	table.Lock(nil)
	defer table.Unlock(nil)
	if len(table.Game.Actions2) != beforeActions {
		t.Fatal("seatless HSM spectator exercised player action authority")
	}
	if researchSessions[created.GameID].PendingRestartRequest != nil {
		t.Fatal("seatless HSM spectator exercised restart-controller authority")
	}
	if len(table.Chat) == 0 {
		t.Fatal("expected spectator note to reach chat under its observer identity")
	}
	lastChat := table.Chat[len(table.Chat)-1]
	if lastChat.Username != spectator.Username ||
		lastChat.Username == participantJoin.Username {
		t.Fatalf("spectator chat impersonated a Roster Player: %#v", lastChat)
	}
}

func TestPhysicalTruthUsesItsOwnGrantedRequestAndPublicationChannel(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.HSMDebugSpectator = &ResearchHSMDebugSpectator{
		Identity:              "hsm_debug_spectator",
		Capability:            "switchable",
		HSMPhysicalTruthGrant: true,
	}
	response := researchJSONRequest(t, router, http.MethodPost, "/research/single-game", payload, "secret")
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	participantJoin := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	participant := NewFakeSession(participantJoin.UserID, participantJoin.Username)
	researchHandleGuestConnected(participant)
	join := researchJoinTokens[path.Base(created.JoinLinks["hsm_debug_spectator"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)

	commandResearchHSMPhysicalTruthRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     73,
		HSMTargetBoundary:      0,
		HSMPerspectivePlayer:   0,
	})

	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	if len(session.PendingHSMSnapshotRequests) != 0 || len(session.PendingHSMPhysicalTruthRequests) != 1 {
		t.Fatalf(
			"Physical Truth crossed semantic queues: snapshots=%#v truth=%#v",
			session.PendingHSMSnapshotRequests,
			session.PendingHSMPhysicalTruthRequests,
		)
	}
	var request ResearchHSMPhysicalTruthRequest
	for _, pending := range session.PendingHSMPhysicalTruthRequests {
		request = *pending
	}
	session.HSMMutex.Unlock()
	if request.ClientRequestID != 73 ||
		request.ArchiveGenerationID != created.HSMArchiveGenerationID ||
		request.Identity != "hsm_debug_spectator" {
		t.Fatalf("Physical Truth correlation or authority was lost: %#v", request)
	}

	publication := ResearchHSMPhysicalTruthPublication{
		ResearchHSMPhysicalTruthIdentity: physicalTruthIdentityForRequest(&request),
		Overlay: ResearchHSMPhysicalTruthOverlay{
			Cards: []ResearchHSMPhysicalTruthCard{
				{StableCardID: 12, Identity: 4},
			},
		},
	}
	published := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-physical-truth", publication, "secret")
	if published.Code != http.StatusOK {
		t.Fatalf("expected Physical Truth publication 200, got %d: %s", published.Code, published.Body.String())
	}
	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	if len(session.PendingHSMPhysicalTruthRequests) != 0 {
		t.Fatalf("published Physical Truth remained pending: %#v", session.PendingHSMPhysicalTruthRequests)
	}
}

func TestPhysicalTruthAuthorizationDependsOnlyOnItsExplicitGrant(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilityOwnPerspective
	payload.RosterPlayers[0].HSMPhysicalTruthGrant = true
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)

	commandResearchHSMPhysicalTruthRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMPerspectivePlayer:   join.SeatIndex,
	})

	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	if len(session.PendingHSMPhysicalTruthRequests) != 1 {
		t.Fatalf(
			"explicit Physical Truth grant did not authorize own-perspective live inspection: %#v",
			session.PendingHSMPhysicalTruthRequests,
		)
	}
}

func TestAuthorizedHSMRequestIsPolledAndPublishedExactlyOnce(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = "switchable"
	response := researchJSONRequest(t, router, http.MethodPost, "/research/single-game", payload, "secret")
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})

	statusResponse := researchJSONRequest(t, router, http.MethodGet, "/research/sessions/"+created.GameID+"/status", nil, "secret")
	var status struct {
		Requests        []ResearchHSMSnapshotRequest `json:"hsm_snapshot_requests"`
		LegalByBoundary map[string][]string          `json:"hsm_legal_actions_by_boundary"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse status response: %v", err)
	}
	if len(status.Requests) != 1 {
		t.Fatalf("expected one pending HSM request, got %#v", status.Requests)
	}
	if len(status.LegalByBoundary["0"]) == 0 {
		t.Fatalf("authority legal actions were not retained for Replay Boundary 0: %#v", status.LegalByBoundary)
	}
	request := status.Requests[0]
	if request.Identity != "roster_player_0" || request.PerspectivePlayer != 0 {
		t.Fatalf("request lost its server-bound identity or controls: %#v", request)
	}

	publish := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-snapshot", ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&request),
		Snapshot:                    researchValidHSMSnapshotForRequest(request),
	}, "secret")
	if publish.Code != http.StatusOK {
		t.Fatalf("expected snapshot publication 200, got %d: %s", publish.Code, publish.Body.String())
	}
	statusResponse = researchJSONRequest(t, router, http.MethodGet, "/research/sessions/"+created.GameID+"/status", nil, "secret")
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse final status response: %v", err)
	}
	if len(status.Requests) != 0 {
		t.Fatalf("published request remained pending: %#v", status.Requests)
	}

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     2,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})
	statusResponse = researchJSONRequest(t, router, http.MethodGet, "/research/sessions/"+created.GameID+"/status", nil, "secret")
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse failure-pending status response: %v", err)
	}
	if len(status.Requests) != 1 {
		t.Fatalf("expected one pending request before failure, got %#v", status.Requests)
	}
	failedRequest := status.Requests[0]
	typedFailure := researchValidHSMFailureForRequest(failedRequest)
	failure := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-snapshot-failure", ResearchHSMSnapshotFailurePublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&failedRequest),
		Error:                       "HSM diagnostics unavailable.",
		Failure:                     typedFailure,
	}, "secret")
	if failure.Code != http.StatusOK {
		t.Fatalf("expected snapshot failure publication 200, got %d: %s", failure.Code, failure.Body.String())
	}
	statusResponse = researchJSONRequest(t, router, http.MethodGet, "/research/sessions/"+created.GameID+"/status", nil, "secret")
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse failure-final status response: %v", err)
	}
	if len(status.Requests) != 0 {
		t.Fatalf("failed request remained pending: %#v", status.Requests)
	}
}

func TestHSMSnapshotPublicationRetriesFailedWebsocketDeliveryExactlyOnce(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	outbound := &researchRecordingOutbound{ready: true}
	viewer.outbound = outbound
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)
	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})
	request := onlyPendingHSMSnapshotRequest(t, created.GameID)
	publication := ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&request),
		Snapshot:                    researchValidHSMSnapshotForRequest(request),
	}
	outbound.messages = nil
	outbound.writeErr = errors.New("closed websocket")

	failed := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected failed websocket delivery to return 503, got %d: %s", failed.Code, failed.Body.String())
	}
	if current := onlyPendingHSMSnapshotRequest(t, created.GameID); current.ServerRequestID != request.ServerRequestID {
		t.Fatalf("failed delivery consumed or replaced pending response: %#v", current)
	}

	outbound.writeErr = nil
	delivered := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if delivered.Code != http.StatusOK {
		t.Fatalf("expected retry delivery 200, got %d: %s", delivered.Code, delivered.Body.String())
	}
	if len(outbound.messages) != 1 || !strings.HasPrefix(outbound.messages[0], "hsmSnapshot ") {
		t.Fatalf("expected one captured snapshot websocket envelope, got %#v", outbound.messages)
	}

	duplicate := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected delivered response retry conflict, got %d: %s", duplicate.Code, duplicate.Body.String())
	}
	if len(outbound.messages) != 1 {
		t.Fatalf("delivered response was emitted more than once: %#v", outbound.messages)
	}
}

func TestHSMSnapshotUnavailableIsCorrelatedRetryableAndDoesNotMutateGameAuthority(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	outbound := &researchRecordingOutbound{ready: true}
	viewer.outbound = outbound
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)
	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})
	request := onlyPendingHSMSnapshotRequest(t, created.GameID)
	publication := ResearchHSMSnapshotUnavailablePublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&request),
		ReasonCode:                  researchHSMSnapshotUnavailableReasonCode,
		Error:                       researchHSMSnapshotUnavailableError,
	}
	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	gameBefore := table.Game
	turnBefore := table.Game.Turn
	actorBefore := table.Game.ActivePlayerIndex
	actionCountBefore := len(table.Game.Actions2)
	table.Unlock(nil)

	for _, unsupported := range []ResearchHSMSnapshotUnavailablePublication{
		func() ResearchHSMSnapshotUnavailablePublication {
			invalid := publication
			invalid.ReasonCode = "internal_error"
			return invalid
		}(),
		func() ResearchHSMSnapshotUnavailablePublication {
			invalid := publication
			invalid.Error = "Internal diagnostics exception."
			return invalid
		}(),
	} {
		rejected := researchJSONRequest(
			t,
			router,
			http.MethodPost,
			"/research/sessions/"+created.GameID+"/hsm-snapshot-unavailable",
			unsupported,
			"secret",
		)
		if rejected.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected unsupported unavailable message 422, got %d: %s", rejected.Code, rejected.Body.String())
		}
	}
	if current := onlyPendingHSMSnapshotRequest(t, created.GameID); current.ServerRequestID != request.ServerRequestID {
		t.Fatalf("invalid unavailable response consumed pending request: %#v", current)
	}

	outbound.messages = nil
	outbound.writeErr = errors.New("closed websocket")
	failed := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot-unavailable",
		publication,
		"secret",
	)
	if failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected failed websocket delivery 503, got %d: %s", failed.Code, failed.Body.String())
	}
	if current := onlyPendingHSMSnapshotRequest(t, created.GameID); current.ServerRequestID != request.ServerRequestID {
		t.Fatalf("failed unavailable delivery consumed pending request: %#v", current)
	}

	outbound.writeErr = nil
	delivered := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot-unavailable",
		publication,
		"secret",
	)
	if delivered.Code != http.StatusOK {
		t.Fatalf("expected retry delivery 200, got %d: %s", delivered.Code, delivered.Body.String())
	}
	if len(outbound.messages) != 1 || !strings.HasPrefix(outbound.messages[0], "hsmSnapshotUnavailable ") {
		t.Fatalf("expected one unavailable websocket envelope, got %#v", outbound.messages)
	}
	var message map[string]interface{}
	if err := json.Unmarshal(
		[]byte(strings.TrimPrefix(outbound.messages[0], "hsmSnapshotUnavailable ")),
		&message,
	); err != nil {
		t.Fatalf("failed to decode unavailable websocket message: %v", err)
	}
	if message["reasonCode"] != researchHSMSnapshotUnavailableReasonCode ||
		message["error"] != researchHSMSnapshotUnavailableError ||
		message["serverRequestID"] != float64(request.ServerRequestID) {
		t.Fatalf("unavailable websocket message lost its fixed reason or identity: %#v", message)
	}
	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	pendingAfterDelivery := len(session.PendingHSMSnapshotRequests)
	session.HSMMutex.Unlock()
	if pendingAfterDelivery != 0 {
		t.Fatal("successfully delivered unavailable response remained pending")
	}

	table.Lock(nil)
	defer table.Unlock(nil)
	if table.Game != gameBefore ||
		table.Game.Turn != turnBefore ||
		table.Game.ActivePlayerIndex != actorBefore ||
		len(table.Game.Actions2) != actionCountBefore {
		t.Fatal("diagnostics-unavailable publication mutated Hanabi game authority")
	}
}

func TestHSMSnapshotGoldenFixtureMatchesTheGoTransportContract(t *testing.T) {
	fixturePath := path.Join(
		"..",
		"..",
		"testdata",
		"research-hsm",
		"transport-v1.json",
	)
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("failed to open shared HSM snapshot fixture: %v", err)
	}
	defer fixture.Close()

	var golden researchHSMTransportGolden
	decoder := json.NewDecoder(fixture)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&golden); err != nil {
		t.Fatalf("shared HSM transport does not match Go contract: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("shared HSM transport contains trailing JSON: %v", err)
	}
	if golden.ProtocolVersion != ResearchHSMProtocolVersion {
		t.Fatalf("unexpected shared HSM protocol version: %d", golden.ProtocolVersion)
	}

	snapshot := golden.SnapshotMessage.Snapshot
	if len(snapshot.raw) == 0 {
		t.Fatal("Go did not retain the Python-owned snapshot payload")
	}
	if snapshot.SemanticProgramID == "" || len(snapshot.Diagnoses) != 1 {
		t.Fatalf("canonical semantic identity or diagnosis was lost: %#v", snapshot)
	}
	if !strings.HasPrefix(snapshot.Diagnoses[0].Label, "hsm-diagnosis:") {
		t.Fatalf("unexpected diagnosis labels: %#v", snapshot.Diagnoses)
	}
	firstClassification := snapshot.Diagnoses[0].Classifications[0]
	if !firstClassification.Follow || firstClassification.Violation {
		t.Fatalf("unexpected first diagnosis action flags: %#v", firstClassification)
	}
	messageIdentity := golden.SnapshotMessage.researchHSMResponseWire
	if golden.SnapshotPending != messageIdentity ||
		golden.SnapshotFailure.researchHSMResponseWire != messageIdentity ||
		messageIdentity.ActorPlayer != 0 {
		t.Fatal("golden pending, success, failure, or actor identities drifted")
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("failed to forward the canonical snapshot: %v", err)
	}
	if bytes.Contains(snapshotJSON, []byte(`"world_ids"`)) ||
		!bytes.Contains(snapshotJSON, []byte(`"clause_ids"`)) {
		t.Fatalf("snapshot did not preserve the privacy-safe guard: %s", snapshotJSON)
	}
	failureJSON, err := json.Marshal(golden.SnapshotFailure.Failure)
	if err != nil {
		t.Fatalf("failed to forward the canonical failure: %v", err)
	}
	if !bytes.Contains(failureJSON, []byte(`"coordinate_kind":["stable_card"]`)) ||
		bytes.Contains(failureJSON, []byte(`"valid"`)) {
		t.Fatalf("failure did not preserve the trimmed canonical core: %s", failureJSON)
	}
	if err := golden.PhysicalTruthMessage.Overlay.validate(); err != nil {
		t.Fatalf("golden Physical Truth is not publishable: %v", err)
	}
}

func TestActionTimeClassificationRemainsAnEqualBoundaryFactDuringHindsight(t *testing.T) {
	request := &ResearchHSMSnapshotRequest{
		ArchiveGenerationID:      7,
		TargetBoundary:           3,
		EvidenceBoundary:         8,
		PerspectivePlayer:        1,
		SemanticProfileID:        11,
		AuthorityLegalProjection: newResearchHSMLegalProjection(nil),
	}
	snapshot := researchValidHSMSnapshotForRequest(*request)
	snapshot.ActionTimeClassification = &ResearchHSMActionTimeClassification{
		GenerationID:      7,
		TargetBoundary:    3,
		EvidenceBoundary:  3,
		PerspectivePlayer: 1,
		SemanticProfileID: 11,
		SelectedActionID:  3,
		RuleFollow:        []bool{},
		RuleViolation:     []bool{},
	}

	if err := snapshot.validateForRequest(request); err != nil {
		t.Fatalf("equal-boundary action-time fact was rejected during hindsight: %v", err)
	}

	snapshot.ActionTimeClassification.EvidenceBoundary = request.EvidenceBoundary
	if err := snapshot.validateForRequest(request); err == nil {
		t.Fatal("action-time record accepted hindsight evidence instead of its target boundary")
	}
}

func TestHSMSnapshotRequestBindsSemanticProfileAndAuthorityLegalProjection(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})
	request := onlyPendingHSMSnapshotRequest(t, created.GameID)
	if request.SemanticProfileID != payload.HSMSemanticProfileID {
		t.Fatalf("pending request lost its semantic profile: %#v", request)
	}
	if err := request.AuthorityLegalProjection.validate(); err != nil {
		t.Fatalf("pending request legal projection is not canonical: %v", err)
	}
	if request.AuthorityLegalProjection.Digest == "" {
		t.Fatal("pending request did not bind a legal-projection digest")
	}

	snapshot := researchValidHSMSnapshotForRequest(request)
	publication := ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&request),
		Snapshot:                    snapshot,
	}

	wrongProfile := publication
	wrongProfile.SemanticProfileID++
	rejected := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		wrongProfile,
		"secret",
	)
	if rejected.Code != http.StatusConflict {
		t.Fatalf("expected mismatched profile conflict, got %d: %s", rejected.Code, rejected.Body.String())
	}

	wrongDigest := publication
	wrongDigest.AuthorityLegalProjectionDigest = "sha256:wrong"
	rejected = researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		wrongDigest,
		"secret",
	)
	if rejected.Code != http.StatusConflict {
		t.Fatalf("expected mismatched legal digest conflict, got %d: %s", rejected.Code, rejected.Body.String())
	}

	wrongProjection := publication
	wrongProjection.Snapshot.AuthorityLegalActionProjection =
		append([]bool(nil), publication.Snapshot.AuthorityLegalActionProjection...)
	wrongProjection.Snapshot.AuthorityLegalActionProjection[0] =
		!wrongProjection.Snapshot.AuthorityLegalActionProjection[0]
	rejected = researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		wrongProjection,
		"secret",
	)
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected mismatched legal projection 422, got %d: %s", rejected.Code, rejected.Body.String())
	}

	accepted := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if accepted.Code != http.StatusOK {
		t.Fatalf("expected exactly bound snapshot 200, got %d: %s", accepted.Code, accepted.Body.String())
	}
}

func TestHSMSnapshotRequestPreservesActorLegalProjectionForSwitchedObserver(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)

	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	expectedProjection := session.HSMLegalProjectionsByBoundary[0]
	session.HSMMutex.Unlock()

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   1,
	})

	request := onlyPendingHSMSnapshotRequest(t, created.GameID)
	if request.ActorPlayer == request.PerspectivePlayer {
		t.Fatalf("test requires a switched non-actor perspective: %#v", request)
	}
	if request.AuthorityLegalProjection.Digest != expectedProjection.Digest {
		t.Fatalf(
			"switched observer replaced the actor's authority legal projection: got %s, want %s",
			request.AuthorityLegalProjection.Digest,
			expectedProjection.Digest,
		)
	}
}

func TestHSMPublicationRejectsIncompleteNestedSuccessAndFailurePayloads(t *testing.T) {
	tests := []struct {
		name        string
		publication func(ResearchHSMSnapshotRequest) interface{}
		pathSuffix  string
	}{
		{
			name: "success missing required projection collection",
			publication: func(request ResearchHSMSnapshotRequest) interface{} {
				snapshot := researchValidHSMSnapshotForRequest(request)
				snapshot.Diagnoses[0].PlayConnections = nil
				return ResearchHSMSnapshotPublication{
					ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(
						&request,
					),
					Snapshot: snapshot,
				}
			},
			pathSuffix: "/hsm-snapshot",
		},
		{
			name: "failure missing complete typed provenance",
			publication: func(request ResearchHSMSnapshotRequest) interface{} {
				failure := researchValidHSMFailureForRequest(request)
				failure.SemanticProgramID = ""
				return ResearchHSMSnapshotFailurePublication{
					ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(
						&request,
					),
					Error:   "HSM diagnostics unavailable.",
					Failure: failure,
				}
			},
			pathSuffix: "/hsm-snapshot-failure",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			researchTestInit(t)
			commandInit()
			router := researchTestRouter()
			payload := researchSingleGamePayload()
			payload.RosterPlayers[0].HSMDebugCapability =
				ResearchHSMCapabilitySwitchable
			response := researchJSONRequest(
				t,
				router,
				http.MethodPost,
				"/research/single-game",
				payload,
				"secret",
			)
			var created CreatedResearchSingleGame
			if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
				t.Fatalf("failed to parse creation response: %v", err)
			}
			join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
			viewer := researchHSMTestViewer(join)
			researchHandleGuestConnected(viewer)
			commandResearchHSMRequest(context.Background(), viewer, &CommandData{
				TableID:                created.TableID,
				HSMProtocolVersion:     ResearchHSMProtocolVersion,
				HSMArchiveGenerationID: created.HSMArchiveGenerationID,
				HSMClientRequestID:     1,
				HSMTargetBoundary:      0,
				HSMEvidenceBoundary:    0,
				HSMPerspectivePlayer:   0,
			})
			request := onlyPendingHSMSnapshotRequest(t, created.GameID)
			rejected := researchJSONRequest(
				t,
				router,
				http.MethodPost,
				"/research/sessions/"+created.GameID+test.pathSuffix,
				test.publication(request),
				"secret",
			)
			if rejected.Code != http.StatusUnprocessableEntity {
				t.Fatalf(
					"expected incomplete payload 422, got %d: %s",
					rejected.Code,
					rejected.Body.String(),
				)
			}
			if pending := onlyPendingHSMSnapshotRequest(t, created.GameID); pending.ServerRequestID != request.ServerRequestID {
				t.Fatalf("incomplete payload consumed pending request: %#v", pending)
			}
		})
	}
}

func TestPhysicalTruthPublicationRejectsDuplicateOrInvalidCards(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability =
		ResearchHSMCapabilityOwnPerspective
	payload.RosterPlayers[0].HSMPhysicalTruthGrant = true
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	commandResearchHSMPhysicalTruthRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMPerspectivePlayer:   join.SeatIndex,
	})
	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	var pending *ResearchHSMPhysicalTruthRequest
	for _, request := range session.PendingHSMPhysicalTruthRequests {
		pending = request
	}
	session.HSMMutex.Unlock()
	if pending == nil {
		t.Fatal("expected one pending Physical Truth request")
	}
	publication := ResearchHSMPhysicalTruthPublication{
		ResearchHSMPhysicalTruthIdentity: physicalTruthIdentityForRequest(pending),
		Overlay: ResearchHSMPhysicalTruthOverlay{
			Cards: []ResearchHSMPhysicalTruthCard{
				{StableCardID: 12, Identity: 4},
				{StableCardID: 12, Identity: -1},
			},
		},
	}
	rejected := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-physical-truth",
		publication,
		"secret",
	)
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"expected invalid Physical Truth 422, got %d: %s",
			rejected.Code,
			rejected.Body.String(),
		)
	}
	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	if len(session.PendingHSMPhysicalTruthRequests) != 1 {
		t.Fatal("invalid Physical Truth consumed its pending request")
	}
}

func TestHSMSnapshotPublicationRejectsObsoleteFieldsAndEmptyDiagnosisSet(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = "switchable"
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)
	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     1,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})
	request := onlyPendingHSMSnapshotRequest(t, created.GameID)
	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d is missing", created.TableID)
	}
	table.Lock(nil)
	gameBefore := table.Game
	actionsBefore := len(table.Game.Actions2)
	actorBefore := table.Game.ActivePlayerIndex
	table.Unlock(nil)

	fixtureBytes, err := os.ReadFile(path.Join(
		"..",
		"..",
		"testdata",
		"research-hsm",
		"transport-v1.json",
	))
	if err != nil {
		t.Fatalf("failed to read shared snapshot fixture: %v", err)
	}
	var golden struct {
		SnapshotMessage struct {
			Snapshot map[string]interface{} `json:"snapshot"`
		} `json:"snapshotMessage"`
	}
	if err := json.Unmarshal(fixtureBytes, &golden); err != nil {
		t.Fatalf("failed to parse shared snapshot fixture: %v", err)
	}
	snapshot := golden.SnapshotMessage.Snapshot
	snapshot["generation_id"] = request.ArchiveGenerationID
	snapshot["target_boundary"] = request.TargetBoundary
	snapshot["evidence_boundary"] = request.EvidenceBoundary
	snapshot["perspective_player"] = request.PerspectivePlayer
	snapshot["semantic_profile_id"] = request.SemanticProfileID
	snapshot["authority_legal_action_projection"] =
		request.AuthorityLegalProjection.legality()
	snapshot["physical_truth"] = map[string]interface{}{"cards": []interface{}{}}
	publication := map[string]interface{}{
		"protocol_version":                  ResearchHSMProtocolVersion,
		"server_request_id":                 request.ServerRequestID,
		"client_request_id":                 request.ClientRequestID,
		"archive_generation_id":             request.ArchiveGenerationID,
		"target_boundary":                   request.TargetBoundary,
		"evidence_boundary":                 request.EvidenceBoundary,
		"perspective_player":                request.PerspectivePlayer,
		"semantic_profile_id":               request.SemanticProfileID,
		"authority_legal_projection_digest": request.AuthorityLegalProjection.Digest,
		"snapshot":                          snapshot,
	}

	rejected := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected obsolete embedded truth to be rejected with 400, got %d: %s",
			rejected.Code,
			rejected.Body.String(),
		)
	}
	pending := onlyPendingHSMSnapshotRequest(t, created.GameID)
	if pending.ServerRequestID != request.ServerRequestID {
		t.Fatalf("rejected publication consumed pending request: %#v", pending)
	}

	delete(snapshot, "physical_truth")
	snapshot["diagnoses"] = []interface{}{}
	emptyDiagnoses := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		publication,
		"secret",
	)
	if emptyDiagnoses.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"expected empty successful diagnosis set to be rejected with 422, got %d: %s",
			emptyDiagnoses.Code,
			emptyDiagnoses.Body.String(),
		)
	}
	pending = onlyPendingHSMSnapshotRequest(t, created.GameID)
	if pending.ServerRequestID != request.ServerRequestID {
		t.Fatalf("invalid empty snapshot consumed pending request: %#v", pending)
	}
	table.Lock(nil)
	defer table.Unlock(nil)
	if table.Game != gameBefore ||
		len(table.Game.Actions2) != actionsBefore ||
		table.Game.ActivePlayerIndex != actorBefore {
		t.Fatal("invalid diagnostics publication mutated live game authority")
	}
}

func TestResearchHSMPublicationRequiresOneJSONDocumentWithJSONContentType(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	for _, test := range []struct {
		name        string
		contentType string
		body        string
	}{
		{
			name:        "trailing JSON document",
			contentType: "application/json",
			body:        "{} {}",
		},
		{
			name:        "unknown publication field",
			contentType: "application/json",
			body:        `{"unexpected":true}`,
		},
		{
			name:        "non-JSON content type",
			contentType: "text/plain",
			body:        "{}",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"/research/sessions/run/hsm-snapshot",
				strings.NewReader(test.body),
			)
			request.Header.Set("Authorization", "Bearer secret")
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected strict decoder 400, got %d: %s",
					response.Code,
					response.Body.String(),
				)
			}
		})
	}
}

func TestHSMRequestCarriesServerGenerationAndClientCorrelation(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = "switchable"
	response := researchJSONRequest(t, router, http.MethodPost, "/research/single-game", payload, "secret")
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	if created.HSMArchiveGenerationID != 1 {
		t.Fatalf("expected first Archive Generation ID 1, got %d", created.HSMArchiveGenerationID)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     41,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})

	statusResponse := researchJSONRequest(t, router, http.MethodGet, "/research/sessions/"+created.GameID+"/status", nil, "secret")
	var status struct {
		ArchiveGenerationID uint32                       `json:"hsm_archive_generation_id"`
		Requests            []ResearchHSMSnapshotRequest `json:"hsm_snapshot_requests"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse session status: %v", err)
	}
	if status.ArchiveGenerationID != created.HSMArchiveGenerationID {
		t.Fatalf("status generation mismatch: %#v", status)
	}
	if len(status.Requests) != 1 {
		t.Fatalf("expected one request, got %#v", status.Requests)
	}
	request := status.Requests[0]
	if request.ServerRequestID <= 0 ||
		request.ClientRequestID != 41 ||
		request.ArchiveGenerationID != created.HSMArchiveGenerationID ||
		request.TargetBoundary != 0 ||
		request.EvidenceBoundary != 0 ||
		request.PerspectivePlayer != 0 {
		t.Fatalf("request correlation was not preserved: %#v", request)
	}
}

func TestAuthorizedHSMRequestGetsCorrelatedProtocolAndCoordinateRejections(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability =
		ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	outbound := viewer.outbound.(*researchRecordingOutbound)
	researchHandleGuestConnected(viewer)

	tests := []struct {
		name       string
		request    CommandData
		reasonCode string
	}{
		{
			name: "unsupported protocol",
			request: CommandData{
				HSMProtocolVersion:     ResearchHSMProtocolVersion + 1,
				HSMArchiveGenerationID: created.HSMArchiveGenerationID,
				HSMClientRequestID:     71,
				HSMTargetBoundary:      0,
				HSMEvidenceBoundary:    0,
				HSMPerspectivePlayer:   0,
			},
			reasonCode: "unsupported_protocol_version",
		},
		{
			name: "stale generation",
			request: CommandData{
				HSMProtocolVersion:     ResearchHSMProtocolVersion,
				HSMArchiveGenerationID: created.HSMArchiveGenerationID + 1,
				HSMClientRequestID:     72,
				HSMTargetBoundary:      0,
				HSMEvidenceBoundary:    0,
				HSMPerspectivePlayer:   0,
			},
			reasonCode: "stale_archive_generation",
		},
		{
			name: "invalid coordinate",
			request: CommandData{
				HSMProtocolVersion:     ResearchHSMProtocolVersion,
				HSMArchiveGenerationID: created.HSMArchiveGenerationID,
				HSMClientRequestID:     73,
				HSMTargetBoundary:      1,
				HSMEvidenceBoundary:    0,
				HSMPerspectivePlayer:   0,
			},
			reasonCode: "invalid_request_coordinate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outbound.messages = nil
			test.request.TableID = created.TableID
			commandResearchHSMRequest(
				context.Background(),
				viewer,
				&test.request,
			)
			if len(outbound.messages) != 1 ||
				!strings.HasPrefix(outbound.messages[0], "hsmSnapshotRejected ") {
				t.Fatalf("expected one correlated rejection, got %#v", outbound.messages)
			}
			var rejection ResearchHSMRequestRejection
			if err := json.Unmarshal(
				[]byte(strings.TrimPrefix(outbound.messages[0], "hsmSnapshotRejected ")),
				&rejection,
			); err != nil {
				t.Fatalf("failed to parse rejection: %v", err)
			}
			if rejection.ProtocolVersion != ResearchHSMProtocolVersion ||
				rejection.ClientRequestID != test.request.HSMClientRequestID ||
				rejection.ReasonCode != test.reasonCode {
				t.Fatalf("rejection lost correlation or reason: %#v", rejection)
			}
			session := researchSessions[created.GameID]
			session.HSMMutex.Lock()
			pending := len(session.PendingHSMSnapshotRequests)
			session.HSMMutex.Unlock()
			if pending != 0 {
				t.Fatalf("rejected request entered the pending queue: %d", pending)
			}
		})
	}
}

func TestHSMRequestRollsBackWhenPendingAcknowledgementCannotBeDelivered(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability =
		ResearchHSMCapabilitySwitchable
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		payload,
		"secret",
	)
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	outbound := viewer.outbound.(*researchRecordingOutbound)
	researchHandleGuestConnected(viewer)
	outbound.writeErr = errors.New("closed websocket")

	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     81,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   0,
	})

	session := researchSessions[created.GameID]
	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	if len(session.PendingHSMSnapshotRequests) != 0 {
		t.Fatalf(
			"undelivered pending acknowledgement left orphaned work: %#v",
			session.PendingHSMSnapshotRequests,
		)
	}
}

func TestHSMPublicationRejectsSupersededAndMismatchedResponseIdentity(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability = "switchable"
	response := researchJSONRequest(t, router, http.MethodPost, "/research/single-game", payload, "secret")
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	researchRecordHSMDecisionBoundary(context.Background(), created.TableID)

	request := func(clientRequestID int) {
		commandResearchHSMRequest(context.Background(), viewer, &CommandData{
			TableID:                created.TableID,
			HSMProtocolVersion:     ResearchHSMProtocolVersion,
			HSMArchiveGenerationID: created.HSMArchiveGenerationID,
			HSMClientRequestID:     clientRequestID,
			HSMTargetBoundary:      0,
			HSMEvidenceBoundary:    0,
			HSMPerspectivePlayer:   0,
		})
	}
	request(51)
	first := onlyPendingHSMSnapshotRequest(t, created.GameID)
	viewer = researchHSMTestViewer(join)
	researchHandleGuestConnected(viewer)
	request(52)
	second := onlyPendingHSMSnapshotRequest(t, created.GameID)
	if first.ServerRequestID == second.ServerRequestID {
		t.Fatalf("new browser request reused server correlation: %#v", second)
	}

	stale := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-snapshot", ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&first),
		Snapshot:                    researchValidHSMSnapshotForRequest(first),
	}, "secret")
	if stale.Code != http.StatusConflict {
		t.Fatalf("expected superseded response conflict, got %d: %s", stale.Code, stale.Body.String())
	}

	mismatched := ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(&second),
		Snapshot:                    researchValidHSMSnapshotForRequest(second),
	}
	mismatched.ClientRequestID++
	conflict := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-snapshot", mismatched, "secret")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected mismatched response conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}
	if current := onlyPendingHSMSnapshotRequest(t, created.GameID); current.ServerRequestID != second.ServerRequestID ||
		current.ClientRequestID != second.ClientRequestID ||
		current.AuthorityLegalProjection.Digest != second.AuthorityLegalProjection.Digest {
		t.Fatalf("mismatched publication consumed the pending request: %#v", current)
	}

	mismatched.ClientRequestID = second.ClientRequestID
	published := researchJSONRequest(t, router, http.MethodPost, "/research/sessions/"+created.GameID+"/hsm-snapshot", mismatched, "secret")
	if published.Code != http.StatusOK {
		t.Fatalf("expected exact publication 200, got %d: %s", published.Code, published.Body.String())
	}
}

func onlyPendingHSMSnapshotRequest(t *testing.T, gameID string) ResearchHSMSnapshotRequest {
	t.Helper()
	session := researchSessions[gameID]
	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	if len(session.PendingHSMSnapshotRequests) != 1 {
		t.Fatalf("expected one pending HSM snapshot request, got %#v", session.PendingHSMSnapshotRequests)
	}
	for _, request := range session.PendingHSMSnapshotRequests {
		return *request
	}
	panic("unreachable")
}

func TestUnauthorizedResearchPlayerHasNoHSMInitializationOrRequests(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	response := researchJSONRequest(t, router, http.MethodPost, "/research/single-game", researchSingleGamePayload(), "secret")
	var created CreatedResearchSingleGame
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse creation response: %v", err)
	}
	join := researchJoinTokens[path.Base(created.JoinLinks["roster_player_0"])]
	viewer := researchHSMTestViewer(join)
	if debug := researchHSMDebugInitForUser(viewer.UserID); debug != nil {
		t.Fatalf("unauthorized player received HSM initialization: %#v", debug)
	}
	commandResearchHSMRequest(context.Background(), viewer, &CommandData{
		TableID:              created.TableID,
		HSMTargetBoundary:    0,
		HSMEvidenceBoundary:  0,
		HSMPerspectivePlayer: 0,
	})
	if requests := researchSessions[created.GameID].PendingHSMSnapshotRequests; len(requests) != 0 {
		t.Fatalf("unauthorized request entered the diagnostic queue: %#v", requests)
	}
}

func TestSingleGameAttendanceLockRejectsPlayerUnattend(t *testing.T) {
	researchTestInit(t)
	router := researchTestRouter()
	response := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/single-game",
		researchSingleGamePayload(),
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
	playerSession := NewFakeSession(join.UserID, join.Username)
	researchHandleGuestConnected(playerSession)
	commandTableUnattend(context.Background(), playerSession, &CommandData{
		TableID: created.TableID,
	})

	if playerSession.Status() != StatusPlaying || playerSession.TableID() != created.TableID {
		t.Fatalf(
			"attendance lock should keep status/table playing/%d, got %d/%d",
			created.TableID,
			playerSession.Status(),
			playerSession.TableID(),
		)
	}
	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	defer table.Unlock(nil)
	playerIndex := table.GetPlayerIndexFromID(playerSession.UserID)
	if playerIndex == -1 || table.Players[playerIndex].Session != playerSession {
		t.Fatal("attendance lock should keep the Single Game player attached to the table")
	}
}

func TestSingleGameRestartRequestTransitionsSameTableExactlyOnce(t *testing.T) {
	researchTestInit(t)
	commandInit()
	router := researchTestRouter()
	payload := researchSingleGamePayload()
	payload.RosterPlayers[0].HSMDebugCapability =
		ResearchHSMCapabilitySwitchable

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
	controller := researchHSMTestViewer(join)
	researchHandleGuestConnected(controller)
	commandResearchHSMRequest(context.Background(), controller, &CommandData{
		TableID:                created.TableID,
		HSMProtocolVersion:     ResearchHSMProtocolVersion,
		HSMArchiveGenerationID: created.HSMArchiveGenerationID,
		HSMClientRequestID:     101,
		HSMTargetBoundary:      0,
		HSMEvidenceBoundary:    0,
		HSMPerspectivePlayer:   join.SeatIndex,
	})
	oldSnapshotRequest := onlyPendingHSMSnapshotRequest(t, created.GameID)
	oldPublication := ResearchHSMSnapshotPublication{
		ResearchHSMResponseIdentity: responseIdentityForSnapshotRequest(
			&oldSnapshotRequest,
		),
		Snapshot: researchValidHSMSnapshotForRequest(oldSnapshotRequest),
	}

	table, ok := tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("created table %d does not exist", created.TableID)
	}
	table.Lock(nil)
	oldGame := table.Game
	table.Unlock(nil)
	commandResearchRestart(context.Background(), controller, &CommandData{
		TableID:     created.TableID,
		RestartKind: "next_game",
	})

	nextLayout := ResearchSeededInitialLayout{
		DeckOrder: researchValidDeck(),
		SeatOrder: []int{0, 1},
		RosterPlayerToSeatID: map[string]string{
			"0": "seat_0",
			"1": "seat_1",
		},
	}
	restartPayload := map[string]interface{}{
		"request_id":            1,
		"game_index":            3,
		"game_seed":             103,
		"seeded_initial_layout": nextLayout,
	}
	restartResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/restart",
		restartPayload,
		"secret",
	)
	if restartResponse.Code != http.StatusOK {
		t.Fatalf("expected restart 200, got %d: %s", restartResponse.Code, restartResponse.Body.String())
	}
	var status map[string]interface{}
	if err := json.Unmarshal(restartResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to parse restart response: %v", err)
	}
	if status["current_game_index"] != float64(3) || status["game_seed"] != float64(103) {
		t.Fatalf("expected next index/seed 3/103, got %#v", status)
	}
	if status["restart_request"] != nil {
		t.Fatalf("expected request to be acknowledged, got %#v", status["restart_request"])
	}
	if status["hsm_archive_generation_id"] != float64(created.HSMArchiveGenerationID+1) {
		t.Fatalf("restart did not advance the Archive Generation ID: %#v", status)
	}
	outbound := controller.outbound.(*researchRecordingOutbound)
	outbound.messages = nil
	stalePublication := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/hsm-snapshot",
		oldPublication,
		"secret",
	)
	if stalePublication.Code != http.StatusConflict {
		t.Fatalf(
			"expected pre-restart HSM publication conflict, got %d: %s",
			stalePublication.Code,
			stalePublication.Body.String(),
		)
	}
	for _, message := range outbound.messages {
		if strings.HasPrefix(message, "hsmSnapshot ") {
			t.Fatalf("stale pre-restart HSM snapshot reached the browser: %q", message)
		}
	}

	table, ok = tables.Get(created.TableID, true)
	if !ok {
		t.Fatalf("original table %d disappeared", created.TableID)
	}
	table.Lock(nil)
	if table.Game == oldGame || table.Game.Seed != "103" {
		t.Fatalf("expected a fresh game for seed 103, got %#v", table.Game)
	}
	if table.Players[0].UserID != join.UserID {
		t.Fatalf("expected controller identity to move to seat 0, got players %#v", table.Players)
	}
	table.Unlock(nil)
	if path.Base(created.JoinLinks["roster_player_0"]) != token {
		t.Fatal("restart changed the controller join link")
	}

	replayedResponse := researchJSONRequest(
		t,
		router,
		http.MethodPost,
		"/research/sessions/"+created.GameID+"/restart",
		restartPayload,
		"secret",
	)
	if replayedResponse.Code != http.StatusConflict {
		t.Fatalf("expected consumed request to return 409, got %d: %s", replayedResponse.Code, replayedResponse.Body.String())
	}
}

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
		Mode:                 "single_game",
		HSMSemanticProfileID: 3,
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

func researchValidHSMSnapshot(_ int) ResearchHSMSnapshot {
	emptyProjection := ResearchHSMDiagnosticProjection{
		Applications:          []ResearchHSMConventionApplication{},
		CardBeliefs:           []ResearchHSMCardBelief{},
		PlayConnections:       []ResearchHSMPlayConnection{},
		ConnectionObligations: []ResearchHSMConnectionObligation{},
		Classifications:       []ResearchHSMClassification{},
		SemanticValues:        []ResearchHSMSemanticValue{},
	}
	return ResearchHSMSnapshot{
		SemanticProfileID:              3,
		AggregateActionClassifications: []ResearchHSMClassification{},
		MistakenActions:                []ResearchHSMMistakenAction{},
		Diagnoses: []ResearchHSMDiagnosis{
			{
				Label: "hsm-diagnosis:0000000000000000000000000000000000000000000000000000000000000000",
				PhysicalGuard: ResearchHSMPhysicalGuard{
					WorldIDs:          []int{1},
					EvidenceBoundary:  0,
					SemanticProfileID: 3,
				},
				ResearchHSMDiagnosticProjection: emptyProjection,
			},
		},
		Consensus:         emptyProjection,
		ViolationWarnings: []ResearchHSMViolationWarning{},
		PlainText:         "[hsm] exact",
	}
}

func researchValidHSMSnapshotForRequest(
	request ResearchHSMSnapshotRequest,
) ResearchHSMSnapshot {
	snapshot := researchValidHSMSnapshot(request.ActorPlayer)
	snapshot.GenerationID = request.ArchiveGenerationID
	snapshot.TargetBoundary = request.TargetBoundary
	snapshot.EvidenceBoundary = request.EvidenceBoundary
	snapshot.PerspectivePlayer = request.PerspectivePlayer
	snapshot.SemanticProfileID = request.SemanticProfileID
	snapshot.AuthorityLegalActionProjection =
		request.AuthorityLegalProjection.legality()
	for index := range snapshot.Diagnoses {
		snapshot.Diagnoses[index].PhysicalGuard.EvidenceBoundary =
			request.EvidenceBoundary
		snapshot.Diagnoses[index].PhysicalGuard.SemanticProfileID =
			request.SemanticProfileID
	}
	return snapshot
}

func researchValidHSMFailureForRequest(
	request ResearchHSMSnapshotRequest,
) ResearchHSMFailure {
	return ResearchHSMFailure{
		Category:              "semantic_program_unsatisfiable",
		Phase:                 "exact_solving",
		TopologyID:            1,
		CapacityManifestID:    "manifest-v1",
		SemanticProgramID:     "sha256:0aa06d6cad330fb09d2520a6d35fc79c2f3086f3eb0e08f32a447c485b637bda",
		SemanticProfileID:     request.SemanticProfileID,
		LegalActionProjection: request.AuthorityLegalProjection.legality(),
		UnsatisfiableCore: &ResearchHSMUnsatisfiableCore{
			Valid:            []bool{},
			CoordinateKind:   []int{},
			TransitionIndex:  []int{},
			RuleIndex:        []int{},
			SubjectIndex:     []int{},
			EvidenceBoundary: []int{},
			ProvenanceID:     []int{},
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

func researchExpectedLiveSuitIndexForJAXMARLColor(color int) int {
	if color == 3 {
		return 4
	}
	if color == 4 {
		return 3
	}
	return color
}
