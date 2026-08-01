import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";
import { globals } from "./UIGlobals";
import { back, exit, forwardFull } from "./replay";
import * as unifiedControllerModule from "./unifiedController";

afterEach(() => {
  globals.store = null;
  globals.lobby.conn = null;
});

function setUpHistoricalProjection() {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
    playing: false,
    shadowing: true,
    replay: {
      ...base.replay,
      active: true,
      segment: 5,
      states: Array.from({ length: 6 }, () => base.ongoingGame),
    },
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 1,
      currentTurnSeat: 0,
      selectedBoundary: 5,
      liveBoundary: 10,
      projectionRevision: 4,
      finished: false,
      projectionInstalled: true,
      terminationVote: false,
      capabilities: {
        canAct: false,
        canEditViewedPlayerNotes: true,
        canPause: true,
        canTerminate: true,
        canRestart: true,
      },
    },
  } as State);
  globals.lobby.tableID = 31;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  return send;
}

test("unified replay navigation requests the same viewed seat at a new boundary", () => {
  const send = setUpHistoricalProjection();

  back();

  expect(send).toHaveBeenCalledWith("researchPerspective", {
    tableID: 31,
    viewedSeat: 1,
    selectedBoundary: 4,
    expectedProjectionRevision: 4,
  });
  expect(globals.state.unifiedController?.selectedBoundary).toBe(5);
});

test("exiting history and forwarding fully request the authoritative live boundary", () => {
  const send = setUpHistoricalProjection();

  exit();
  forwardFull();

  expect(send).toHaveBeenNthCalledWith(1, "researchPerspective", {
    tableID: 31,
    viewedSeat: 1,
    selectedBoundary: 10,
    expectedProjectionRevision: 4,
  });
  expect(send).toHaveBeenNthCalledWith(2, "researchPerspective", {
    tableID: 31,
    viewedSeat: 1,
    selectedBoundary: 10,
    expectedProjectionRevision: 4,
  });
});

test("replay controls use the authoritative live boundary, not the projected endpoint", () => {
  setUpHistoricalProjection();
  const { getReplayFinalBoundary } = unifiedControllerModule as unknown as {
    getReplayFinalBoundary: (state: State) => number | null;
  };

  expect(globals.state.ongoingGame.turn.segment).toBeNull();
  expect(getReplayFinalBoundary(globals.state)).toBe(10);
});
