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

function setUp(canPause: boolean) {
  const defaultMetadata = getDefaultMetadata(2);
  const base = initialState({
    ...defaultMetadata,
    options: { ...defaultMetadata.options, timed: true },
  });
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
        canPause,
        canTerminate: true,
        canRestart: true,
      },
    },
  } as State);
  globals.lobby.tableID = 51;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  return send;
}

function requestPauseFromTimer() {
  const request = (
    drawUIModule as unknown as { requestPauseFromTimer: () => void }
  ).requestPauseFromTimer;
  request();
}

test("a capable unified controller can pause from any viewed seat", () => {
  const send = setUp(true);

  requestPauseFromTimer();

  expect(send).toHaveBeenCalledWith("pause", {
    tableID: 51,
    setting: "pause",
  });
});

test("pause is inert without the explicit unified capability", () => {
  const send = setUp(false);

  requestPauseFromTimer();

  expect(send).not.toHaveBeenCalled();
});
