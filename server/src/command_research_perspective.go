package main

import "context"

// researchProjectUnifiedAction publishes the actor-relative action delta before scheduling the
// guarded perspective follow. The caller holds the table mutex; the scheduled callback does not.
func researchProjectUnifiedAction(
	table *Table,
	game *Game,
	actorSeat int,
	previousLiveBoundary int,
) {
	controller := table.ResearchUnifiedController
	if controller == nil {
		return
	}
	liveBoundary := researchUnifiedLiveBoundary(game)
	for len(controller.ActionCutoffs) <= liveBoundary {
		controller.ActionCutoffs = append(controller.ActionCutoffs, len(game.Actions))
	}
	controller.ActionCutoffs[liveBoundary] = len(game.Actions)

	invalidateResearchUnifiedFollow(controller)
	if controller.ViewedSeat != actorSeat || controller.SelectedBoundary != previousLiveBoundary {
		controller.ProjectionRevision++
		emitResearchUnifiedProjectionForController(table, controller)
		return
	}

	controller.ViewedSeat = actorSeat
	controller.SelectedBoundary = liveBoundary
	controller.TransitionKind = researchUnifiedTransitionAcceptedAction
	if game.EndCondition == EndConditionInProgress && game.ActivePlayerIndex != actorSeat {
		controller.NextFollowToken++
		controller.PendingFollowToken = controller.NextFollowToken
	}
	controller.ProjectionRevision++
	updateResearchUnifiedShadow(table, controller, actorSeat)
	emitResearchUnifiedProjectionForController(table, controller)

	if controller.PendingFollowToken == 0 {
		return
	}
	token := controller.PendingFollowToken
	nextSeat := game.ActivePlayerIndex
	scheduleResearchUnifiedFollow(researchUnifiedFollowDelay, func() {
		researchCompleteUnifiedFollow(table, game, controller, token, liveBoundary, nextSeat)
	})
}

func researchCompleteUnifiedFollow(
	table *Table,
	game *Game,
	controller *ResearchUnifiedController,
	token uint64,
	liveBoundary int,
	nextSeat int,
) {
	// Resolve the registered table before taking its mutex so a replaced table generation cannot be
	// mistaken for the generation that scheduled this callback.
	registeredTable, exists := tables.Get(table.ID, true)
	if !exists || registeredTable != table {
		return
	}
	table.Lock(nil)
	defer table.Unlock(nil)
	if table.Deleted || table.Game != game || table.ResearchUnifiedController != controller ||
		controller.PendingFollowToken != token ||
		controller.TransitionKind != researchUnifiedTransitionAcceptedAction ||
		controller.SelectedBoundary != liveBoundary ||
		researchUnifiedLiveBoundary(game) != liveBoundary ||
		game.EndCondition != EndConditionInProgress || game.ActivePlayerIndex != nextSeat {
		return
	}

	controller.ViewedSeat = nextSeat
	invalidateResearchUnifiedFollow(controller)
	controller.ProjectionRevision++
	updateResearchUnifiedShadow(table, controller, nextSeat)
	emitResearchUnifiedProjectionForController(table, controller)
}

func updateResearchUnifiedShadow(
	table *Table,
	controller *ResearchUnifiedController,
	viewedSeat int,
) {
	spectatorIndex := table.GetSpectatorIndexFromID(controller.UserID)
	if spectatorIndex < 0 {
		return
	}
	spectator := table.Spectators[spectatorIndex]
	spectator.ShadowingPlayerIndex = viewedSeat
	spectator.ShadowingPlayerUsername = table.Players[viewedSeat].Name
}

func emitResearchUnifiedProjectionForController(
	table *Table,
	controller *ResearchUnifiedController,
) {
	if spectatorIndex := table.GetSpectatorIndexFromID(controller.UserID); spectatorIndex >= 0 {
		emitResearchUnifiedProjection(table.Spectators[spectatorIndex].Session, table, controller)
	}
}

// commandResearchPerspective atomically replaces the observer projection for a unified controller.
func commandResearchPerspective(ctx context.Context, s *Session, d *CommandData) {
	table, exists := getTableAndLock(ctx, s, d.TableID, true, true)
	if !exists {
		return
	}
	defer table.Unlock(ctx)
	controller, unified := table.researchUnifiedControllerForSession(s)
	if !unified {
		if _, registered := table.researchUnifiedController(s.UserID); registered {
			s.Warning("This session is no longer the active unified connection.")
		} else {
			s.Warning("This session cannot switch unified perspectives.")
		}
		return
	}
	if d.UnifiedViewedSeat == nil ||
		d.UnifiedSelectedBoundary == nil ||
		d.ExpectedProjectionRevision == nil {
		s.Warning("Unified perspective requests require a viewed seat, selected boundary, and projection revision.")
		return
	}
	if *d.ExpectedProjectionRevision != controller.ProjectionRevision {
		s.Warning("The unified perspective request is stale.")
		return
	}
	viewedSeat := *d.UnifiedViewedSeat
	if viewedSeat < 0 || viewedSeat >= len(table.Players) {
		s.Warning("That is an invalid unified perspective.")
		return
	}
	selectedBoundary := *d.UnifiedSelectedBoundary
	liveBoundary := researchUnifiedLiveBoundary(table.Game)
	if selectedBoundary < 0 || selectedBoundary > liveBoundary {
		s.Warning("That is an invalid unified history boundary.")
		return
	}
	if viewedSeat == controller.ViewedSeat && selectedBoundary == controller.SelectedBoundary {
		return
	}
	spectatorIndex := table.GetSpectatorIndexFromID(s.UserID)
	if spectatorIndex < 0 {
		s.Warning("The unified session is not attached to this game.")
		return
	}
	candidate := *controller
	candidate.ViewedSeat = viewedSeat
	candidate.SelectedBoundary = selectedBoundary
	invalidateResearchUnifiedFollow(&candidate)
	candidate.ProjectionRevision++
	projection, err := resolveResearchUnifiedProjection(table, &candidate)
	if err != nil {
		s.Warning(err.Error())
		return
	}
	if err := s.EmitChecked("researchUnifiedProjection", projection); err != nil {
		return
	}

	table.Spectators[spectatorIndex].ShadowingPlayerIndex = viewedSeat
	table.Spectators[spectatorIndex].ShadowingPlayerUsername = table.Players[viewedSeat].Name
	controller.ViewedSeat = candidate.ViewedSeat
	controller.SelectedBoundary = candidate.SelectedBoundary
	controller.ProjectionRevision = candidate.ProjectionRevision
	controller.TransitionKind = candidate.TransitionKind
	controller.PendingFollowToken = candidate.PendingFollowToken
}
