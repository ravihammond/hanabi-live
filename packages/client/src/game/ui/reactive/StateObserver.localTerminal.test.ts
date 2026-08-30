import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import type { Store } from "redux";
import { selectLocalTerminalBoundary } from "../../../localTerminal";
import { initialState } from "../../reducers/initialStates/initialState";
import type { State } from "../../types/State";
import type { Action } from "../../types/actions";
import { globals } from "../UIGlobals";
import { StateObserver } from "./StateObserver";

jest.mock("../../../localTerminal", () => ({
  selectLocalTerminalBoundary: jest.fn(),
}));
jest.mock("./views/animateFastView");
jest.mock("./views/cardLayoutView");
jest.mock("./views/cardsView");
jest.mock("./views/cluesView");
jest.mock("./views/currentPlayerAreaView");
jest.mock("./views/deckView");
jest.mock("./views/gameInfoView");
jest.mock("./views/hypotheticalView");
jest.mock("./views/initView");
jest.mock("./views/logView");
jest.mock("./views/pauseView");
jest.mock("./views/premoveView");
jest.mock("./views/replayView");
jest.mock("./views/soundView");
jest.mock("./views/spectatorsView");
jest.mock("./views/statsView");
jest.mock("./views/turnView");

const selectBoundary = jest.mocked(selectLocalTerminalBoundary);

afterEach(() => {
  globals.store = null;
  jest.clearAllMocks();
});

test.each([
  ["new game", { replay: false, unified: false, liveBoundary: 0 }, 0],
  ["ordinary live play", { replay: false, unified: false }, 7],
  ["ordinary replay", { replay: true, unified: false }, 3],
  [
    "ordinary hypothetical",
    { replay: true, unified: false, hypothetical: true },
    3,
  ],
  ["unified history", { replay: true, unified: true }, 4],
])("selects the real boundary during %s", (_name, mode, expected) => {
  const state = stateAtBoundaries(mode);
  const store = staticStore(state);
  globals.store = store;

  const observer = new StateObserver(store);

  expect(selectBoundary).toHaveBeenLastCalledWith(expected);
  observer.unregisterObservers();
});

function stateAtBoundaries(mode: {
  hypothetical?: boolean;
  liveBoundary?: number;
  replay: boolean;
  unified: boolean;
}): State {
  const base = initialState(getDefaultMetadata(2));
  const ongoingGame = {
    ...base.ongoingGame,
    turn: { ...base.ongoingGame.turn, segment: mode.liveBoundary ?? 7 },
  };
  return {
    ...base,
    visibleState: ongoingGame,
    ongoingGame,
    replay: {
      ...base.replay,
      active: mode.replay,
      segment: 3,
      hypothetical:
        mode.hypothetical === true
          ? {
              ongoing: {
                ...ongoingGame,
                turn: { ...ongoingGame.turn, segment: 6 },
              },
              states: [ongoingGame],
              showDrawnCards: true,
              drawnCardsInHypothetical: [],
              morphedIdentities: [],
              startingPlayerIndex: 0,
            }
          : null,
    },
    unifiedController: mode.unified
      ? {
          protocolCapability: "unified_manual_v1",
          viewedSeat: 0,
          currentTurnSeat: 1,
          selectedBoundary: 4,
          liveBoundary: 9,
          projectionRevision: 2,
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
        }
      : null,
  };
}

function staticStore(state: State): Store<State, Action> {
  return {
    dispatch: jest.fn(),
    getState: () => state,
    replaceReducer: jest.fn(),
    subscribe: jest.fn(() => jest.fn()),
  } as unknown as Store<State, Action>;
}
