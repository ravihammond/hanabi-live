package main

import (
	"context"
)

func commandResearchHSMRequest(ctx context.Context, s *Session, d *CommandData) {
	researchSessionsMutex.Lock()
	join, ok := researchGuestUsers[s.UserID]
	if !ok || join.TableID != d.TableID || join.HSMDebugCapability == "" || join.HSMDebugCapability == "none" {
		researchSessionsMutex.Unlock()
		return
	}
	session, ok := researchSessions[join.GameID]
	researchSessionsMutex.Unlock()
	if !ok {
		return
	}

	table, exists := getTableAndLock(ctx, s, d.TableID, true, true)
	if !exists {
		return
	}
	if !table.Running || table.Game == nil {
		table.Unlock(ctx)
		return
	}
	currentBoundary := len(table.Game.Actions2)
	actor := table.Game.ActivePlayerIndex
	targetIsTerminal := table.Game.EndCondition > EndConditionInProgress && d.HSMTargetBoundary == currentBoundary
	if targetIsTerminal {
		actor = -1
	}
	session.HSMMutex.Lock()
	if recordedActor, exists := session.HSMActorsByBoundary[d.HSMTargetBoundary]; exists && !targetIsTerminal {
		actor = recordedActor
	}
	session.HSMMutex.Unlock()
	numPlayers := len(table.Game.Players)
	table.Unlock(ctx)

	if d.HSMTargetBoundary < 0 || d.HSMTargetBoundary > currentBoundary ||
		d.HSMEvidenceBoundary < d.HSMTargetBoundary || d.HSMEvidenceBoundary > currentBoundary ||
		d.HSMPerspectivePlayer < 0 || d.HSMPerspectivePlayer >= numPlayers {
		return
	}
	if join.HSMDebugCapability == "own_perspective" && d.HSMPerspectivePlayer != join.SeatIndex {
		return
	}
	isDebugSpectator := join.SeatIndex < 0
	physicalTruthAllowed := isDebugSpectator ||
		(join.HSMDebugCapability == "switchable" &&
			(d.HSMPerspectivePlayer != join.SeatIndex || targetIsTerminal))
	if d.HSMPhysicalTruth && !physicalTruthAllowed {
		return
	}

	session.HSMMutex.Lock()
	defer session.HSMMutex.Unlock()
	session.NextHSMSnapshotRequestID++
	requestID := session.NextHSMSnapshotRequestID
	session.PendingHSMSnapshotRequests[requestID] = &ResearchHSMSnapshotRequest{
		RequestID:         requestID,
		Identity:          join.HSMIdentity,
		TargetBoundary:    d.HSMTargetBoundary,
		EvidenceBoundary:  d.HSMEvidenceBoundary,
		PerspectivePlayer: d.HSMPerspectivePlayer,
		ActorPlayer:       actor,
		PhysicalTruth:     d.HSMPhysicalTruth,
		Client:            s,
	}
}

func researchRecordHSMDecisionBoundary(ctx context.Context, tableID uint64) {
	researchSessionsMutex.Lock()
	sessionsForTable := make([]*ResearchSession, 0)
	for _, session := range researchSessions {
		if session.TableID == tableID && session.PendingHSMSnapshotRequests != nil {
			sessionsForTable = append(sessionsForTable, session)
		}
	}
	researchSessionsMutex.Unlock()
	if len(sessionsForTable) == 0 {
		return
	}
	table, ok := tables.Get(tableID, true)
	if !ok {
		return
	}
	table.Lock(ctx)
	defer table.Unlock(ctx)
	if table.Game == nil {
		return
	}
	for _, session := range sessionsForTable {
		session.HSMMutex.Lock()
		boundary := len(table.Game.Actions2)
		session.HSMLegalActionsByBoundary[boundary] = append([]string(nil), researchLegalActions(table.Game)...)
		session.HSMActorsByBoundary[boundary] = table.Game.ActivePlayerIndex
		session.HSMMutex.Unlock()
	}
}
