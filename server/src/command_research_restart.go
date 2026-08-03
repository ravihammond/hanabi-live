package main

import "context"

const (
	researchRestartSameSeed = "same_seed"
	researchRestartNextGame = "next_game"
)

// commandResearchRestart records a controller request for the localhost research orchestrator.
func commandResearchRestart(ctx context.Context, s *Session, d *CommandData) {
	if d.RestartKind != researchRestartSameSeed && d.RestartKind != researchRestartNextGame {
		s.Warning("The requested research restart kind is not valid.")
		return
	}

	unifiedController := false
	if table, ok := tables.Get(d.TableID, true); ok {
		table.Lock(ctx)
		if _, registered := table.researchUnifiedController(s.UserID); registered {
			controller, active := table.researchUnifiedControllerForSession(s)
			if !active {
				table.Unlock(ctx)
				s.Warning("This session is no longer the active unified connection.")
				return
			}
			unifiedController = researchUnifiedCapabilities(table, controller).CanRestart
		}
		table.Unlock(ctx)
	}

	var researchSession *ResearchSession
	researchSessionsMutex.Lock()
	for _, session := range researchSessions {
		if session.TableID != d.TableID || session.Mode != "single_game" {
			continue
		}
		researchSession = session
		break
	}
	researchSessionsMutex.Unlock()
	if researchSession == nil {
		s.Warning("This table is not a persistent Single Game run.")
		return
	}

	researchSession.LifecycleMutex.Lock()
	if !unifiedController &&
		(researchSession.RestartControllerUserID == 0 || s.UserID != researchSession.RestartControllerUserID) {
		researchSession.LifecycleMutex.Unlock()
		s.Warning("Only the Single Game restart controller can restart this run.")
		return
	}
	if researchSession.LifecycleMutationInProgress || researchSession.PendingRestartRequest != nil {
		researchSession.LifecycleMutex.Unlock()
		return
	}
	researchSession.NextRestartRequestID++
	researchSession.PendingRestartRequest = &ResearchRestartRequest{
		RequestID: researchSession.NextRestartRequestID,
		Kind:      d.RestartKind,
	}
	researchSession.LifecycleMutex.Unlock()
	if !unifiedController {
		return
	}
	if table, ok := tables.Get(d.TableID, true); ok {
		table.Lock(ctx)
		if controller, unified := table.researchUnifiedControllerForSession(s); unified {
			invalidateResearchUnifiedFollow(controller)
			reviseResearchUnifiedProjection(table)
		}
		table.Unlock(ctx)
	}
}
