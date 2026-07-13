package main

import "context"

const (
	researchRestartSameSeed = "same_seed"
	researchRestartNextGame = "next_game"
)

// commandResearchRestart records a controller request for the localhost research orchestrator.
func commandResearchRestart(_ context.Context, s *Session, d *CommandData) {
	if d.RestartKind != researchRestartSameSeed && d.RestartKind != researchRestartNextGame {
		s.Warning("The requested research restart kind is not valid.")
		return
	}

	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()
	for _, session := range researchSessions {
		if session.TableID != d.TableID || session.Mode != "single_game" {
			continue
		}
		if session.RestartControllerUserID == 0 || s.UserID != session.RestartControllerUserID {
			s.Warning("Only the Single Game restart controller can restart this run.")
			return
		}
		if session.PendingRestartRequest != nil {
			return
		}
		session.NextRestartRequestID++
		session.PendingRestartRequest = &ResearchRestartRequest{
			RequestID: session.NextRestartRequestID,
			Kind:      d.RestartKind,
		}
		return
	}
	s.Warning("This table is not a persistent Single Game run.")
}
