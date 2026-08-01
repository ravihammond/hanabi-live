import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";
import { globals } from "./UIGlobals";
import { update } from "./timer";

jest.mock("../../tooltips", () => ({ setInstanceContent: jest.fn() }));

afterEach(() => {
  globals.store = null;
  globals.elements.timer1 = null;
  globals.elements.timer2 = null;
  globals.playerTimes = [];
  globals.activePlayerIndex = -1;
  jest.restoreAllMocks();
});

test("a terminal unified projection cannot restart a live player clock", () => {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
    playing: false,
    shadowing: true,
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 1,
      currentTurnSeat: -1,
      selectedBoundary: 7,
      liveBoundary: 7,
      projectionRevision: 9,
      finished: true,
      projectionInstalled: true,
      terminationVote: false,
      capabilities: {
        canAct: false,
        canEditViewedPlayerNotes: false,
        canPause: false,
        canTerminate: false,
        canRestart: true,
      },
    },
  } as State);
  const setTimerText = jest.fn();
  const oval = {
    fill: jest.fn().mockReturnThis(),
    opacity: jest.fn().mockReturnThis(),
  };
  globals.elements.timer1 = { setTimerText, oval } as never;
  globals.elements.timer2 = {
    setTimerText: jest.fn(),
    setLabelText: jest.fn(),
    visible: jest.fn(),
    oval,
  } as never;

  update({ times: [30_000, 30_000], activePlayerIndex: 0, timeTaken: 0 });

  expect(setTimerText).not.toHaveBeenCalled();
});

test("a clock snapshot received before an unpause projection is retained", () => {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
    playing: false,
    shadowing: true,
    pause: { active: true, playerIndex: 1, queued: false },
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 1,
      currentTurnSeat: 0,
      selectedBoundary: 7,
      liveBoundary: 7,
      projectionRevision: 9,
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

  update({ times: [12_000, 34_000], activePlayerIndex: 0, timeTaken: 500 });

  expect(globals.playerTimes).toEqual([12_000, 34_000]);
  expect(globals.activePlayerIndex).toBe(0);
  expect(globals.timeTaken).toBe(500);
});
