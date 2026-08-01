package main

import "context"

func researchFollowUnifiedTurn(table *Table, viewedSeat int) {
	controller := table.ResearchUnifiedController
	if controller == nil {
		return
	}
	controller.ViewedSeat = viewedSeat
	controller.SelectedBoundary = researchUnifiedLiveBoundary(table.Game)
	controller.ProjectionRevision++
	for len(controller.ActionCutoffs) <= controller.SelectedBoundary {
		controller.ActionCutoffs = append(controller.ActionCutoffs, len(table.Game.Actions))
	}
	controller.ActionCutoffs[controller.SelectedBoundary] = len(table.Game.Actions)
	spectatorIndex := table.GetSpectatorIndexFromID(controller.UserID)
	if spectatorIndex < 0 {
		return
	}
	spectator := table.Spectators[spectatorIndex]
	spectator.ShadowingPlayerIndex = viewedSeat
	spectator.ShadowingPlayerUsername = table.Players[viewedSeat].Name
	emitResearchUnifiedProjection(spectator.Session, table, controller)
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
	spectatorIndex := table.GetSpectatorIndexFromID(s.UserID)
	if spectatorIndex < 0 {
		s.Warning("The unified session is not attached to this game.")
		return
	}
	candidate := *controller
	candidate.ViewedSeat = viewedSeat
	candidate.SelectedBoundary = selectedBoundary
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
}
