package main

import (
	"context"
	"strconv"
)

// commandTableTerminate is sent when the user double clicks the terminate button in the bottom-left-hand
// corner
//
// Example data:
//
//	{
//	  tableID: 5,
//	  server: true, // True if a server-initiated termination, otherwise omitted
//	}
func commandTableTerminate(ctx context.Context, s *Session, d *CommandData) {
	t, exists := getTableAndLock(ctx, s, d.TableID, !d.NoTableLock, !d.NoTablesLock)
	if !exists {
		return
	}
	if !d.NoTableLock {
		defer t.Unlock(ctx)
	}

	// Validate that they are in the game
	playerIndex := t.GetPlayerIndexFromID(s.UserID)
	if playerIndex == -1 {
		controller, unified := t.researchUnifiedControllerForSession(s)
		if !unified {
			if _, registered := t.researchUnifiedController(s.UserID); registered {
				s.Warning("This session is no longer the active unified connection.")
				return
			}
			s.Warning("You are not playing at table " + strconv.FormatUint(t.ID, 10) + ", " +
				"so you cannot terminate it.")
			return
		}
		if !researchUnifiedCapabilities(t, controller).CanTerminate {
			s.Warning("The unified controller cannot terminate this game.")
			return
		}
		playerIndex = controller.ViewedSeat
	}

	// Validate that the game has started
	if !t.Running {
		s.Warning("You can not terminate a game that has not started yet.")
		return
	}

	// Validate that it is not a replay
	if t.Replay {
		s.Warning("You can not terminate a replay.")
		return
	}

	terminate(ctx, s, d, t, playerIndex)
}

func terminate(ctx context.Context, s *Session, d *CommandData, t *Table, playerIndex int) {
	_, unified := t.researchUnifiedControllerForSession(s)
	commandAction(ctx, s, &CommandData{ // nolint: exhaustivestruct
		TableID:                  t.ID,
		Type:                     ActionTypeEndGame,
		Target:                   playerIndex,
		Value:                    EndConditionTerminatedByPlayer,
		NoTableLock:              true,
		UnifiedControlAuthorized: unified,
	})
}
