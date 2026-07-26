package main

import (
	"context"
	"encoding/json"
	"fmt"
)

func commandResearchHSMRequest(ctx context.Context, s *Session, d *CommandData) {
	authorization, session, ok := researchHSMViewerAuthorizationForUser(s.UserID, d.TableID)
	if !ok {
		return
	}
	if d.HSMProtocolVersion != ResearchHSMProtocolVersion {
		researchRejectHSMSnapshotRequest(s, d, "unsupported_protocol_version")
		return
	}
	if d.HSMClientRequestID <= 0 {
		researchRejectHSMSnapshotRequest(s, d, "invalid_request_coordinate")
		return
	}
	session.HSMMutex.Lock()
	activeGenerationID := session.HSMArchiveGenerationID
	session.HSMMutex.Unlock()
	if d.HSMArchiveGenerationID != activeGenerationID {
		researchRejectHSMSnapshotRequest(s, d, "stale_archive_generation")
		return
	}
	if session.Mode != "single_game" {
		researchRejectHSMSnapshotRequest(s, d, "lifecycle_unavailable")
		return
	}

	table, exists := getTableAndLock(ctx, s, d.TableID, true, true)
	if !exists {
		researchRejectHSMSnapshotRequest(s, d, "lifecycle_unavailable")
		return
	}
	if !table.Running || table.Game == nil {
		table.Unlock(ctx)
		researchRejectHSMSnapshotRequest(s, d, "lifecycle_unavailable")
		return
	}
	currentBoundary := len(table.Game.Actions2)
	actor := table.Game.ActivePlayerIndex
	targetIsTerminal := table.Game.EndCondition > EndConditionInProgress && d.HSMTargetBoundary == currentBoundary
	if targetIsTerminal {
		actor = -1
	}
	currentProjection, currentProjectionErr := researchCanonicalLegalProjection(table.Game)
	session.HSMMutex.Lock()
	if d.HSMArchiveGenerationID != session.HSMArchiveGenerationID {
		session.HSMMutex.Unlock()
		table.Unlock(ctx)
		researchRejectHSMSnapshotRequest(s, d, "stale_archive_generation")
		return
	}
	if recordedActor, exists := session.HSMActorsByBoundary[d.HSMTargetBoundary]; exists && !targetIsTerminal {
		actor = recordedActor
	}
	authorityProjection, projectionExists :=
		session.HSMLegalProjectionsByBoundary[d.HSMTargetBoundary]
	if d.HSMTargetBoundary == currentBoundary && currentProjectionErr == nil {
		authorityProjection = currentProjection
		projectionExists = true
		session.HSMLegalProjectionsByBoundary[currentBoundary] = currentProjection
	}
	semanticProfileID := session.HSMSemanticProfileID
	session.HSMMutex.Unlock()
	numPlayers := len(table.Game.Players)
	table.Unlock(ctx)

	if d.HSMTargetBoundary < 0 || d.HSMTargetBoundary > currentBoundary ||
		d.HSMEvidenceBoundary < d.HSMTargetBoundary || d.HSMEvidenceBoundary > currentBoundary ||
		d.HSMPerspectivePlayer < 0 || d.HSMPerspectivePlayer >= numPlayers {
		researchRejectHSMSnapshotRequest(s, d, "invalid_request_coordinate")
		return
	}
	if !authorization.allowsSnapshot(d.HSMPerspectivePlayer) {
		researchRejectHSMSnapshotRequest(s, d, "perspective_not_authorized")
		return
	}
	if !projectionExists || semanticProfileID <= 0 {
		researchRejectHSMSnapshotRequest(s, d, "authority_projection_unavailable")
		return
	}
	if d.HSMPerspectivePlayer != actor {
		authorityProjection = newResearchHSMLegalProjection(nil)
	}
	session.HSMMutex.Lock()
	if d.HSMArchiveGenerationID != session.HSMArchiveGenerationID {
		session.HSMMutex.Unlock()
		researchRejectHSMSnapshotRequest(s, d, "stale_archive_generation")
		return
	}
	for serverRequestID, pending := range session.PendingHSMSnapshotRequests {
		if pending.PrincipalID == authorization.PrincipalID {
			delete(session.PendingHSMSnapshotRequests, serverRequestID)
		}
	}
	session.NextHSMSnapshotRequestID++
	requestID := session.NextHSMSnapshotRequestID
	request := &ResearchHSMSnapshotRequest{
		ServerRequestID:          requestID,
		ClientRequestID:          d.HSMClientRequestID,
		ArchiveGenerationID:      d.HSMArchiveGenerationID,
		Identity:                 authorization.Identity,
		PrincipalID:              authorization.PrincipalID,
		TargetBoundary:           d.HSMTargetBoundary,
		EvidenceBoundary:         d.HSMEvidenceBoundary,
		PerspectivePlayer:        d.HSMPerspectivePlayer,
		ActorPlayer:              actor,
		SemanticProfileID:        semanticProfileID,
		AuthorityLegalProjection: authorityProjection,
		Client:                   s,
	}
	session.PendingHSMSnapshotRequests[requestID] = request
	identity := responseIdentityForSnapshotRequest(request)
	session.HSMMutex.Unlock()
	session.HSMDeliveryMutex.Lock()
	session.HSMMutex.Lock()
	current, stillPending := session.PendingHSMSnapshotRequests[requestID]
	if !stillPending || current != request ||
		session.HSMArchiveGenerationID != request.ArchiveGenerationID {
		session.HSMMutex.Unlock()
		session.HSMDeliveryMutex.Unlock()
		return
	}
	session.HSMMutex.Unlock()
	if err := s.EmitChecked("hsmSnapshotPending", hsmResponseIdentityMessage(identity)); err != nil {
		session.HSMMutex.Lock()
		if current, exists := session.PendingHSMSnapshotRequests[requestID]; exists && current == request {
			delete(session.PendingHSMSnapshotRequests, requestID)
		}
		session.HSMMutex.Unlock()
	}
	session.HSMDeliveryMutex.Unlock()
}

func commandResearchHSMPhysicalTruthRequest(ctx context.Context, s *Session, d *CommandData) {
	authorization, session, ok := researchHSMViewerAuthorizationForUser(s.UserID, d.TableID)
	if !ok {
		return
	}
	if d.HSMProtocolVersion != ResearchHSMProtocolVersion {
		researchRejectHSMPhysicalTruthRequest(s, d, "unsupported_protocol_version")
		return
	}
	if !authorization.PhysicalTruthGrant {
		researchRejectHSMPhysicalTruthRequest(s, d, "physical_truth_not_authorized")
		return
	}
	if d.HSMClientRequestID <= 0 {
		researchRejectHSMPhysicalTruthRequest(s, d, "invalid_request_coordinate")
		return
	}
	session.HSMMutex.Lock()
	activeGenerationID := session.HSMArchiveGenerationID
	session.HSMMutex.Unlock()
	if d.HSMArchiveGenerationID != activeGenerationID {
		researchRejectHSMPhysicalTruthRequest(s, d, "stale_archive_generation")
		return
	}
	if session.Mode != "single_game" {
		researchRejectHSMPhysicalTruthRequest(s, d, "lifecycle_unavailable")
		return
	}

	table, exists := getTableAndLock(ctx, s, d.TableID, true, true)
	if !exists {
		researchRejectHSMPhysicalTruthRequest(s, d, "lifecycle_unavailable")
		return
	}
	if !table.Running || table.Game == nil {
		table.Unlock(ctx)
		researchRejectHSMPhysicalTruthRequest(s, d, "lifecycle_unavailable")
		return
	}
	currentBoundary := len(table.Game.Actions2)
	numPlayers := len(table.Game.Players)
	table.Unlock(ctx)

	if d.HSMTargetBoundary < 0 ||
		d.HSMTargetBoundary > currentBoundary ||
		d.HSMPerspectivePlayer < 0 ||
		d.HSMPerspectivePlayer >= numPlayers {
		researchRejectHSMPhysicalTruthRequest(s, d, "invalid_request_coordinate")
		return
	}

	session.HSMMutex.Lock()
	if d.HSMArchiveGenerationID != session.HSMArchiveGenerationID || d.HSMClientRequestID <= 0 {
		session.HSMMutex.Unlock()
		researchRejectHSMPhysicalTruthRequest(s, d, "stale_archive_generation")
		return
	}
	for serverRequestID, pending := range session.PendingHSMPhysicalTruthRequests {
		if pending.PrincipalID == authorization.PrincipalID {
			delete(session.PendingHSMPhysicalTruthRequests, serverRequestID)
		}
	}
	session.NextHSMPhysicalTruthRequestID++
	serverRequestID := session.NextHSMPhysicalTruthRequestID
	request := &ResearchHSMPhysicalTruthRequest{
		ServerRequestID:     serverRequestID,
		ClientRequestID:     d.HSMClientRequestID,
		ArchiveGenerationID: d.HSMArchiveGenerationID,
		Identity:            authorization.Identity,
		PrincipalID:         authorization.PrincipalID,
		TargetBoundary:      d.HSMTargetBoundary,
		PerspectivePlayer:   d.HSMPerspectivePlayer,
		Client:              s,
	}
	session.PendingHSMPhysicalTruthRequests[serverRequestID] = request
	identity := physicalTruthIdentityForRequest(request)
	session.HSMMutex.Unlock()
	session.HSMDeliveryMutex.Lock()
	session.HSMMutex.Lock()
	current, stillPending := session.PendingHSMPhysicalTruthRequests[serverRequestID]
	if !stillPending || current != request ||
		session.HSMArchiveGenerationID != request.ArchiveGenerationID {
		session.HSMMutex.Unlock()
		session.HSMDeliveryMutex.Unlock()
		return
	}
	session.HSMMutex.Unlock()
	if err := s.EmitChecked(
		"hsmPhysicalTruthPending",
		hsmPhysicalTruthIdentityMessage(identity),
	); err != nil {
		session.HSMMutex.Lock()
		if current, exists := session.PendingHSMPhysicalTruthRequests[serverRequestID]; exists && current == request {
			delete(session.PendingHSMPhysicalTruthRequests, serverRequestID)
		}
		session.HSMMutex.Unlock()
	}
	session.HSMDeliveryMutex.Unlock()
}

func researchHSMViewerAuthorizationForUser(
	userID int,
	tableID uint64,
) (ResearchHSMViewerAuthorization, *ResearchSession, bool) {
	researchSessionsMutex.Lock()
	defer researchSessionsMutex.Unlock()

	join, ok := researchGuestUsers[userID]
	if !ok || join.TableID != tableID || join.HSMPrincipalID == "" {
		return ResearchHSMViewerAuthorization{}, nil, false
	}
	session, ok := researchSessions[join.GameID]
	if !ok {
		return ResearchHSMViewerAuthorization{}, nil, false
	}
	return ResearchHSMViewerAuthorization{
		GameID:             join.GameID,
		TableID:            join.TableID,
		PrincipalID:        join.HSMPrincipalID,
		Identity:           join.HSMIdentity,
		ViewerKind:         join.HSMViewerKind,
		Capability:         join.HSMDebugCapability,
		PhysicalTruthGrant: join.HSMPhysicalTruthGrant,
		SeatIndex:          join.SeatIndex,
	}, session, true
}

func researchRejectHSMSnapshotRequest(
	session *Session,
	request *CommandData,
	reasonCode string,
) {
	_ = session.EmitChecked("hsmSnapshotRejected", ResearchHSMRequestRejection{
		ProtocolVersion:     ResearchHSMProtocolVersion,
		ClientRequestID:     request.HSMClientRequestID,
		ArchiveGenerationID: request.HSMArchiveGenerationID,
		TargetBoundary:      request.HSMTargetBoundary,
		EvidenceBoundary:    request.HSMEvidenceBoundary,
		PerspectivePlayer:   request.HSMPerspectivePlayer,
		ReasonCode:          reasonCode,
	})
}

func researchRejectHSMPhysicalTruthRequest(
	session *Session,
	request *CommandData,
	reasonCode string,
) {
	_ = session.EmitChecked(
		"hsmPhysicalTruthRejected",
		ResearchHSMPhysicalTruthRejection{
			ProtocolVersion:     ResearchHSMProtocolVersion,
			ClientRequestID:     request.HSMClientRequestID,
			ArchiveGenerationID: request.HSMArchiveGenerationID,
			TargetBoundary:      request.HSMTargetBoundary,
			PerspectivePlayer:   request.HSMPerspectivePlayer,
			ReasonCode:          reasonCode,
		},
	)
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
		if projection, err := researchCanonicalLegalProjection(table.Game); err == nil {
			session.HSMLegalProjectionsByBoundary[boundary] = projection
		}
		session.HSMActorsByBoundary[boundary] = table.Game.ActivePlayerIndex
		session.HSMMutex.Unlock()
	}
}

func researchCanonicalLegalProjection(game *Game) (ResearchHSMLegalProjection, error) {
	legality := make([]bool, researchHSMLegalProjectionSize)
	if game == nil || game.EndCondition > EndConditionInProgress {
		return newResearchHSMLegalProjection(legality), nil
	}
	for _, encoded := range researchLegalActions(game) {
		var action ResearchBotAction
		if err := json.Unmarshal([]byte(encoded), &action); err != nil {
			return ResearchHSMLegalProjection{}, fmt.Errorf(
				"authority legal action is malformed: %w",
				err,
			)
		}
		actionID, err := researchCanonicalActionID(game, action)
		if err != nil {
			return ResearchHSMLegalProjection{}, err
		}
		if actionID < 0 || actionID >= len(legality) {
			return ResearchHSMLegalProjection{}, fmt.Errorf(
				"Canonical Action ID %d is outside the fixed HSM axis",
				actionID,
			)
		}
		legality[actionID] = true
	}
	return newResearchHSMLegalProjection(legality), nil
}

func researchCanonicalActionID(game *Game, action ResearchBotAction) (int, error) {
	actor := game.ActivePlayerIndex
	numPlayers := len(game.Players)
	if actor < 0 || actor >= numPlayers {
		return 0, fmt.Errorf("authority actor is outside the live roster")
	}
	handSize := 4
	if numPlayers <= 3 {
		handSize = 5
	}
	if action.Type == ActionTypePlay || action.Type == ActionTypeDiscard {
		slot := -1
		for index, card := range game.Players[actor].Hand {
			if card.Order == action.Target {
				slot = index
				break
			}
		}
		if slot < 0 || slot >= handSize {
			return 0, fmt.Errorf("authority legal action names a card outside the actor hand")
		}
		if action.Type == ActionTypePlay {
			return handSize + slot, nil
		}
		return slot, nil
	}
	if action.Target < 0 || action.Target >= numPlayers || action.Target == actor {
		return 0, fmt.Errorf("authority clue target is not another live player")
	}
	relativeTarget := (action.Target - actor - 1 + numPlayers) % numPlayers
	clueBase := 2*handSize + relativeTarget*5
	switch action.Type {
	case ActionTypeColorClue:
		if action.Value < 0 || action.Value >= len(researchJAXMARLColorToHanabiLiveSuitIndex) {
			return 0, fmt.Errorf("authority color clue is outside the canonical range")
		}
		// This permutation is its own inverse.
		return clueBase + researchJAXMARLColorToHanabiLiveSuitIndex[action.Value], nil
	case ActionTypeRankClue:
		if action.Value < 1 || action.Value > 5 {
			return 0, fmt.Errorf("authority rank clue is outside the canonical range")
		}
		return 2*handSize + (numPlayers-1)*5 + relativeTarget*5 + action.Value - 1, nil
	default:
		return 0, fmt.Errorf("authority action type %d is not supported by the HSM", action.Type)
	}
}
