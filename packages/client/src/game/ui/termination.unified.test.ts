import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";
import { globals } from "./UIGlobals";
import * as drawUIModule from "./drawUI";

afterEach(() => {
  globals.store = null;
  globals.lobby.conn = null;
});

function setUp(canTerminate: boolean) {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
    playing: false,
    shadowing: true,
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 1,
      currentTurnSeat: 0,
      selectedBoundary: 2,
      liveBoundary: 3,
      projectionRevision: 5,
      finished: false,
      projectionInstalled: true,
      terminationVote: false,
      capabilities: {
        canAct: false,
        canEditViewedPlayerNotes: true,
        canPause: true,
        canTerminate,
        canRestart: true,
      },
    },
  } as State);
  globals.lobby.tableID = 61;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  return send;
}

function requestTermination(immediate: boolean) {
  const request = (
    drawUIModule as unknown as {
      requestTermination: (terminateImmediately: boolean) => void;
    }
  ).requestTermination;
  request(immediate);
}

test("a capable unified controller can vote or terminate immediately", () => {
  const send = setUp(true);

  requestTermination(false);
  requestTermination(true);

  expect(send).toHaveBeenNthCalledWith(1, "tableVoteForTermination", {
    tableID: 61,
  });
  expect(send).toHaveBeenNthCalledWith(2, "tableTerminate", {
    tableID: 61,
  });
});

test("termination is inert without the explicit unified capability", () => {
  const send = setUp(false);

  requestTermination(true);

  expect(send).not.toHaveBeenCalled();
});
