import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import Konva from "konva";
import type * as Redux from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import type { Action } from "../types/actions";
import type { State } from "../types/State";
import { LABEL_COLOR } from "./constants";
import { NameFrame } from "./NameFrame";
import { globals } from "./UIGlobals";

afterEach(() => {
  globals.store = null;
  globals.elements.fullActionLog = null;
  document.querySelector("#game")?.classList.remove("game-cursor-pointer");
});

test("unified player names select perspectives and advertise interaction", () => {
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  const state = initialState(getDefaultMetadata(2));
  Object.assign(state, {
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 0,
      currentTurnSeat: 0,
      selectedBoundary: 7,
      liveBoundary: 7,
      projectionRevision: 3,
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
    },
  });
  globals.store = {
    getState: () => state,
  } as Redux.Store<State, Action>;
  const frame = new NameFrame({ width: 200, height: 20, name: "Bob" }, 1);
  const layer = new Konva.Layer();
  layer.add(frame as unknown as Konva.Group);
  const batchDraw = jest.spyOn(layer, "batchDraw").mockReturnValue(layer);

  frame.playerName.fire("mouseenter", {});

  expect(frame.playerName.fill()).toBe("#ffdf00");
  expect(batchDraw).toHaveBeenCalledTimes(1);
  expect(document.querySelector("#game")?.classList).toContain(
    "game-cursor-pointer",
  );

  frame.playerName.fire("click", { evt: { button: 0 } });

  expect(send).toHaveBeenCalledWith("researchPerspective", {
    tableID: globals.lobby.tableID,
    viewedSeat: 1,
    selectedBoundary: 7,
    expectedProjectionRevision: 3,
  });

  frame.rightClick();
  expect(send).toHaveBeenCalledTimes(2);
  expect(send).toHaveBeenLastCalledWith("researchPerspective", {
    tableID: globals.lobby.tableID,
    viewedSeat: 1,
    selectedBoundary: 7,
    expectedProjectionRevision: 3,
  });
  expect(send).not.toHaveBeenCalledWith("tableSpectate", expect.anything());

  frame.playerName.fire("mouseleave", {});
  expect(frame.playerName.fill()).toBe(LABEL_COLOR);
  expect(batchDraw).toHaveBeenCalledTimes(2);
});

test("the currently viewed unified player name is inert", () => {
  const send = jest.fn();
  const showPlayerActions = jest.fn();
  globals.lobby.conn = { send } as never;
  globals.elements.fullActionLog = { showPlayerActions } as never;
  const state = initialState(getDefaultMetadata(2));
  Object.assign(state, {
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 0,
      currentTurnSeat: 0,
      selectedBoundary: 7,
      liveBoundary: 7,
      projectionRevision: 3,
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
    },
  });
  globals.store = {
    getState: () => state,
  } as Redux.Store<State, Action>;
  const frame = new NameFrame({ width: 200, height: 20, name: "Alice" }, 0);

  frame.playerName.fire("mouseenter", {});
  frame.playerName.fire("click", { evt: { button: 0 } });
  frame.playerName.fire("click", { evt: { button: 2 } });

  expect(frame.playerName.fill()).toBe(LABEL_COLOR);
  expect(document.querySelector("#game")?.classList).not.toContain(
    "game-cursor-pointer",
  );
  expect(send).not.toHaveBeenCalled();
  expect(showPlayerActions).not.toHaveBeenCalled();
});
