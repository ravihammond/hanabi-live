import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { globals as lobbyGlobals } from "../../Globals";
import * as gameMain from "../main";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";

afterEach(() => {
  lobbyGlobals.ui = null;
  lobbyGlobals.conn = null;
});

function setUp(canRestart: boolean) {
  const base = initialState(getDefaultMetadata(2));
  const store = createStore(stateReducer, {
    ...base,
    playing: false,
    shadowing: true,
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 0,
      currentTurnSeat: 0,
      selectedBoundary: 3,
      liveBoundary: 3,
      projectionRevision: 5,
      finished: false,
      projectionInstalled: true,
      terminationVote: false,
      capabilities: {
        canAct: true,
        canEditViewedPlayerNotes: true,
        canPause: true,
        canTerminate: true,
        canRestart,
      },
    },
  } as State);
  lobbyGlobals.ui = {
    globals: { store, researchRestartController: false },
  } as never;
  lobbyGlobals.tableID = 71;
  const send = jest.fn();
  lobbyGlobals.conn = { send } as never;
  return send;
}

function requestRestart(restartKind: "same_seed" | "next_game") {
  const request = (
    gameMain as unknown as {
      requestResearchRestart: (kind: "same_seed" | "next_game") => void;
    }
  ).requestResearchRestart;
  request(restartKind);
}

test("the unified controller owns both Single Game restart commands", () => {
  const send = setUp(true);

  requestRestart("same_seed");
  requestRestart("next_game");

  expect(send).toHaveBeenNthCalledWith(1, "researchRestart", {
    tableID: 71,
    restartKind: "same_seed",
  });
  expect(send).toHaveBeenNthCalledWith(2, "researchRestart", {
    tableID: 71,
    restartKind: "next_game",
  });
});

test("restart is inert without the explicit unified capability", () => {
  const send = setUp(false);

  requestRestart("same_seed");

  expect(send).not.toHaveBeenCalled();
});
