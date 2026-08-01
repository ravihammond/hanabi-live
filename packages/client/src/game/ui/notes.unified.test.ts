import type { CardOrder } from "@hanabi-live/game";
import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";
import { globals } from "./UIGlobals";
import { set } from "./notes";

afterEach(() => {
  globals.store = null;
  globals.lobby.conn = null;
});

function setUp(canEditViewedPlayerNotes: boolean) {
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
        canEditViewedPlayerNotes,
        canPause: true,
        canTerminate: true,
        canRestart: true,
      },
    },
  } as State);
  globals.lobby.tableID = 41;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;
  return send;
}

test("unified note edits use the viewed player's notes when allowed", () => {
  const send = setUp(true);
  const oldNote = globals.state.notes.ourNotes[0]?.text;

  set(0 as CardOrder, "Cathy private note");

  expect(send).toHaveBeenCalledWith("note", {
    tableID: 41,
    order: 0,
    note: "Cathy private note",
    expectedViewedSeat: 1,
    expectedProjectionRevision: 5,
  });
  expect(globals.state.notes.ourNotes[0]?.text).toBe(oldNote);
});

test("unified note edits are inert without the explicit capability", () => {
  const send = setUp(false);
  const oldNote = globals.state.notes.ourNotes[0]?.text;

  set(0 as CardOrder, "must not persist");

  expect(send).not.toHaveBeenCalled();
  expect(globals.state.notes.ourNotes[0]?.text).toBe(oldNote);
});

test("ordinary player note edits retain their established payload", () => {
  const base = initialState(getDefaultMetadata(2));
  globals.store = createStore(stateReducer, {
    ...base,
    visibleState: base.ongoingGame,
  });
  globals.lobby.tableID = 42;
  const send = jest.fn();
  globals.lobby.conn = { send } as never;

  set(0 as CardOrder, "ordinary note");

  expect(send).toHaveBeenCalledWith("note", {
    tableID: 42,
    order: 0,
    note: "ordinary note",
  });
  expect(globals.state.notes.ourNotes[0]?.text).toBe("ordinary note");
});
