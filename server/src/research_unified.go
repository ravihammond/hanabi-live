package main

import (
	"fmt"
	"time"
)

const (
	researchUnifiedTransitionAcceptedAction = "acceptedAction"
	researchUnifiedFollowDelay              = 1500 * time.Millisecond
)

// This scheduler is a production time boundary and is replaced by a deterministic fake in tests.
var scheduleResearchUnifiedFollow = func(delay time.Duration, follow func()) {
	time.AfterFunc(delay, follow)
}

// Unified lock policy:
//   - A ResearchUnifiedController belongs to one Table and is protected by that table's mutex.
//   - researchSessionsMutex protects only the global research registries and join-token lookup.
//   - Code holding a table mutex must never acquire researchSessionsMutex.
//   - Code that needs both must snapshot registry data, release researchSessionsMutex, and only then
//     acquire the table mutex.
type ResearchUnifiedController struct {
	UserID             int
	ViewedSeat         int
	SelectedBoundary   int
	ProjectionRevision uint64
	Initialized        bool
	TransitionKind     string
	PendingFollowToken uint64
	NextFollowToken    uint64
	// ActionCutoffs is indexed by replay segment and stores the exclusive end of that segment in
	// Game.Actions. It lets historical projections reuse the canonical action stream without
	// reconstructing or copying Hanabi state-transition logic.
	ActionCutoffs []int
}

type ResearchUnifiedCapabilities struct {
	CanAct                   bool `json:"canAct"`
	CanEditViewedPlayerNotes bool `json:"canEditViewedPlayerNotes"`
	CanPause                 bool `json:"canPause"`
	CanTerminate             bool `json:"canTerminate"`
	CanRestart               bool `json:"canRestart"`
}

type ResearchUnifiedControllerInit struct {
	ProtocolCapability string                      `json:"protocolCapability"`
	ViewedSeat         int                         `json:"viewedSeat"`
	CurrentTurnSeat    int                         `json:"currentTurnSeat"`
	SelectedBoundary   int                         `json:"selectedBoundary"`
	LiveBoundary       int                         `json:"liveBoundary"`
	ProjectionRevision uint64                      `json:"projectionRevision"`
	Finished           bool                        `json:"finished"`
	Capabilities       ResearchUnifiedCapabilities `json:"capabilities"`
}

type ResearchUnifiedProjection struct {
	TableID            uint64                      `json:"tableID"`
	ViewedSeat         int                         `json:"viewedSeat"`
	CurrentTurnSeat    int                         `json:"currentTurnSeat"`
	SelectedBoundary   int                         `json:"selectedBoundary"`
	LiveBoundary       int                         `json:"liveBoundary"`
	ProjectionRevision uint64                      `json:"projectionRevision"`
	Finished           bool                        `json:"finished"`
	Paused             bool                        `json:"paused"`
	PausePlayerIndex   int                         `json:"pausePlayerIndex"`
	TerminationVote    bool                        `json:"terminationVote"`
	Capabilities       ResearchUnifiedCapabilities `json:"capabilities"`
	Actions            []interface{}               `json:"actions"`
	Notes              []string                    `json:"notes"`
	CardIdentities     []*CardIdentity             `json:"cardIdentities,omitempty"`
	TransitionKind     string                      `json:"transitionKind,omitempty"`
	PendingFollowToken uint64                      `json:"pendingFollowToken,omitempty"`
}

func (t *Table) researchUnifiedController(userID int) (*ResearchUnifiedController, bool) {
	controller := t.ResearchUnifiedController
	return controller, controller != nil && controller.UserID == userID
}

func (t *Table) researchUnifiedControllerForSession(
	s *Session,
) (*ResearchUnifiedController, bool) {
	if s == nil {
		return nil, false
	}
	controller, unified := t.researchUnifiedController(s.UserID)
	if !unified {
		return nil, false
	}
	spectatorIndex := t.GetSpectatorIndexFromID(s.UserID)
	if spectatorIndex < 0 || t.Spectators[spectatorIndex].Session != s {
		return nil, false
	}
	return controller, true
}

func researchUnifiedLiveBoundary(game *Game) int {
	if game == nil {
		return 0
	}
	// Hanabi.live's replay segment index is the authoritative history position presented by the UI.
	return game.Turn
}

func researchUnifiedCapabilities(
	table *Table,
	controller *ResearchUnifiedController,
) ResearchUnifiedCapabilities {
	game := table.Game
	liveBoundary := researchUnifiedLiveBoundary(game)
	inProgress := table.Running && game != nil && game.EndCondition == EndConditionInProgress
	canAct := inProgress &&
		!game.Paused &&
		controller.ViewedSeat == game.ActivePlayerIndex &&
		controller.SelectedBoundary == liveBoundary
	return ResearchUnifiedCapabilities{
		CanAct:                   canAct,
		CanEditViewedPlayerNotes: inProgress,
		CanPause:                 inProgress && table.Options.Timed,
		CanTerminate:             inProgress,
		CanRestart:               table.ExtraOptions.ResearchPersistentSingleGame,
	}
}

func researchUnifiedControllerInit(
	table *Table,
	controller *ResearchUnifiedController,
) *ResearchUnifiedControllerInit {
	currentTurnSeat := -1
	if table.Game != nil && table.Game.EndCondition == EndConditionInProgress {
		currentTurnSeat = table.Game.ActivePlayerIndex
	}
	return &ResearchUnifiedControllerInit{
		ProtocolCapability: researchUnifiedManualCapability,
		ViewedSeat:         controller.ViewedSeat,
		CurrentTurnSeat:    currentTurnSeat,
		SelectedBoundary:   controller.SelectedBoundary,
		LiveBoundary:       researchUnifiedLiveBoundary(table.Game),
		ProjectionRevision: controller.ProjectionRevision,
		Finished:           table.Game != nil && table.Game.EndCondition != EndConditionInProgress,
		Capabilities:       researchUnifiedCapabilities(table, controller),
	}
}

func resolveResearchUnifiedProjection(
	table *Table,
	controller *ResearchUnifiedController,
) (*ResearchUnifiedProjection, error) {
	game := table.Game
	if game == nil {
		return nil, fmt.Errorf("unified game is not running")
	}
	if controller.ViewedSeat < 0 || controller.ViewedSeat >= len(game.Players) {
		return nil, fmt.Errorf("unified viewed seat is invalid")
	}
	liveBoundary := researchUnifiedLiveBoundary(game)
	if controller.SelectedBoundary < 0 || controller.SelectedBoundary > liveBoundary {
		return nil, fmt.Errorf("unified selected boundary is invalid")
	}

	actionCutoff := len(game.Actions)
	if controller.SelectedBoundary < liveBoundary {
		if controller.SelectedBoundary >= len(controller.ActionCutoffs) {
			return nil, fmt.Errorf("unified selected boundary is unavailable")
		}
		actionCutoff = controller.ActionCutoffs[controller.SelectedBoundary]
	}
	actions := make([]interface{}, 0, actionCutoff)
	for _, action := range game.Actions[:actionCutoff] {
		actions = append(
			actions,
			CheckScrubForPlayerIndex(table, action, controller.ViewedSeat),
		)
	}

	currentTurnSeat := -1
	if game.EndCondition == EndConditionInProgress {
		currentTurnSeat = game.ActivePlayerIndex
	}
	var cardIdentities []*CardIdentity
	if game.EndCondition != EndConditionInProgress {
		// Ordinary Hanabi.live players receive this reveal only after the game ends. Keep that
		// behavior inside the atomic projection instead of leaking the omniscient spectator event.
		cardIdentities = append([]*CardIdentity(nil), game.CardIdentities...)
	}
	return &ResearchUnifiedProjection{
		TableID:            table.ID,
		ViewedSeat:         controller.ViewedSeat,
		CurrentTurnSeat:    currentTurnSeat,
		SelectedBoundary:   controller.SelectedBoundary,
		LiveBoundary:       liveBoundary,
		ProjectionRevision: controller.ProjectionRevision,
		Finished:           game.EndCondition != EndConditionInProgress,
		Paused:             game.Paused,
		PausePlayerIndex:   game.PausePlayerIndex,
		TerminationVote:    table.Players[controller.ViewedSeat].VoteToKill,
		Capabilities:       researchUnifiedCapabilities(table, controller),
		Actions:            actions,
		Notes:              append([]string(nil), game.Players[controller.ViewedSeat].Notes...),
		CardIdentities:     cardIdentities,
		TransitionKind:     controller.TransitionKind,
		PendingFollowToken: controller.PendingFollowToken,
	}, nil
}

func invalidateResearchUnifiedFollow(controller *ResearchUnifiedController) {
	controller.TransitionKind = ""
	controller.PendingFollowToken = 0
}

func emitResearchUnifiedProjection(
	s *Session,
	table *Table,
	controller *ResearchUnifiedController,
) bool {
	if s == nil {
		return false
	}
	projection, err := resolveResearchUnifiedProjection(table, controller)
	if err != nil {
		s.Warning(err.Error())
		return false
	}
	s.Emit("researchUnifiedProjection", projection)
	return true
}

// reviseResearchUnifiedProjection advances the table-owned revision and publishes the replacement
// to the currently attached controller session, if any. The caller must hold the table mutex.
func reviseResearchUnifiedProjection(table *Table) {
	controller := table.ResearchUnifiedController
	if controller == nil {
		return
	}
	controller.ProjectionRevision++
	if spectatorIndex := table.GetSpectatorIndexFromID(controller.UserID); spectatorIndex >= 0 {
		emitResearchUnifiedProjection(table.Spectators[spectatorIndex].Session, table, controller)
	}
}
