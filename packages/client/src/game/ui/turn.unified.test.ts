import type { CardOrder } from "@hanabi-live/game";
import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import { ActionType } from "../types/ActionType";
import type { UnifiedControllerState } from "../types/UnifiedController";
import { globals } from "./UIGlobals";
import { end } from "./turn";

afterEach(() => {
  globals.store = null;
  globals.lobby.conn = null;
});

function setUpController(overrides: Partial<UnifiedControllerState> = {}) {
  const base = initialState(getDefaultMetadata(2));
  const controller: UnifiedControllerState = {
    protocolCapability: "unified_manual_v1",
    viewedSeat: 0,
    currentTurnSeat: 0,
    selectedBoundary: 7,
    liveBoundary: 7,
    projectionRevision: 9,
    finished: false,
    projectionInstalled: true,
    terminationVote: false,
    capabilities: {
      canAct: true,
      canEditViewedPlayerNotes: true,
      canPause: true,
      canTerminate: true,
      canRestart: true,
    },
    ...overrides,
  };
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
    playing: false,
    shadowing: true,
    metadata: {
      ...base.metadata,
      ourPlayerIndex: controller.viewedSeat,
    },
    unifiedController: controller,
  });
  globals.lobby.tableID = 21;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  globals.elements.clueArea = { hide: jest.fn() } as never;
  globals.elements.waitingOnServer = { show: jest.fn() } as never;
  globals.elements.waitingOnServerAnimation = {
    start: jest.fn(),
  } as never;
  globals.elements.playerHands[0] = {
    checkSetDraggableAll: jest.fn(),
  } as never;
  return send;
}

test("a unified action is bound to its actor, live boundary, and projection revision", () => {
  const send = setUpController();

  end({ type: ActionType.Play, target: 0 as CardOrder });

  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith("action", {
    tableID: 21,
    type: ActionType.Play,
    target: 0,
    expectedActorSeat: 0,
    expectedLiveBoundary: 7,
    expectedProjectionRevision: 9,
  });
});

test("off-turn and historical unified projections neither act nor premove", () => {
  const send = setUpController({
    viewedSeat: 1,
    selectedBoundary: 6,
    capabilities: {
      canAct: false,
      canEditViewedPlayerNotes: true,
      canPause: true,
      canTerminate: true,
      canRestart: true,
    },
  });

  end({ type: ActionType.Discard, target: 0 as CardOrder });
  globals.store!.dispatch({
    type: "premove",
    premove: { type: ActionType.Discard, target: 0 as CardOrder },
  });

  expect(send).not.toHaveBeenCalled();
  expect(globals.state.premove).toBeNull();
});

test("a terminal unified projection cannot act even with inconsistent capabilities", () => {
  const send = setUpController({ finished: true });

  end({ type: ActionType.Play, target: 0 as CardOrder });

  expect(send).not.toHaveBeenCalled();
});

test("regular player actions keep their established payload and premove behavior", () => {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
  });
  globals.lobby.tableID = 22;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  globals.elements.clueArea = { hide: jest.fn() } as never;
  globals.elements.waitingOnServer = { show: jest.fn() } as never;
  globals.elements.waitingOnServerAnimation = {
    start: jest.fn(),
  } as never;
  globals.elements.playerHands[0] = {
    checkSetDraggableAll: jest.fn(),
  } as never;

  end({ type: ActionType.Play, target: 0 as CardOrder });

  expect(send).toHaveBeenCalledWith("action", {
    tableID: 22,
    type: ActionType.Play,
    target: 0,
  });
});
