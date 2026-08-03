import { SoundMove } from "@hanabi-live/data";
import type { CardOrder } from "@hanabi-live/game";
import { draw, getDefaultMetadata, rankClue } from "@hanabi-live/game";
import { afterEach, expect, jest, test } from "@jest/globals";
import Konva from "konva";
import { createStore } from "redux";
import { initialState } from "../reducers/initialStates/initialState";
import { stateReducer } from "../reducers/stateReducer";
import type { State } from "../types/State";
import type { UnifiedProjectionData } from "../types/UnifiedController";
import { CardLayout } from "./CardLayout";
import * as gameCommandsModule from "./gameCommands";
import { gameCommands } from "./gameCommands";
import type { LayoutChild } from "./LayoutChild";
import { PlayStack } from "./PlayStack";
import { onNewSoundEffect } from "./reactive/views/soundView";
import { globals } from "./UIGlobals";

afterEach(() => {
  globals.store = null;
  globals.stateObserver = null;
  globals.lobby.ui = null;
  globals.lobby.conn = null;
  globals.game = null;
  globals.editingNote = null;
  globals.deck = [];
  globals.stackBases = [];
  jest.restoreAllMocks();
});

function setUpUnifiedStore() {
  const base = initialState(getDefaultMetadata(2));
  const store = createStore(stateReducer, {
    ...base,
    playing: false,
    shadowing: true,
    cardIdentities: base.cardIdentities.map((identity, index) =>
      index === 5 ? { suitIndex: 0, rank: 5 } : identity,
    ),
    unifiedController: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 0,
      currentTurnSeat: 0,
      selectedBoundary: 0,
      liveBoundary: 3,
      projectionRevision: 7,
      finished: false,
      projectionInstalled: false,
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
  globals.store = store;
  globals.lobby.tableID = 17;
  globals.lobby.conn = { send: jest.fn() } as never;
  return store;
}

function projection(
  overrides: Partial<UnifiedProjectionData> = {},
): UnifiedProjectionData {
  return {
    tableID: 17,
    viewedSeat: 1,
    currentTurnSeat: 0,
    selectedBoundary: 0,
    liveBoundary: 3,
    projectionRevision: 8,
    finished: false,
    paused: false,
    pausePlayerIndex: 0,
    terminationVote: false,
    capabilities: {
      canAct: false,
      canEditViewedPlayerNotes: true,
      canPause: true,
      canTerminate: true,
      canRestart: true,
    },
    actions: [],
    notes: ["Cathy's note"],
    ...overrides,
  };
}

function historicalProjectionActions() {
  return [
    draw(0, 0, 0, 1),
    draw(0, 1, 0, 2),
    draw(0, 2, 0, 3),
    draw(0, 3, 0, 4),
    draw(0, 4, 0, 5),
    draw(1, 5),
    draw(1, 6),
    draw(1, 7),
    draw(1, 8),
    draw(1, 9),
    rankClue(1, 0, [], 1),
  ] as const;
}

function fakeLayoutChild(isStackBase = false): LayoutChild {
  const child = new Konva.Group({ width: 100, height: 200 });
  Object.assign(child, {
    tween: null,
    doMisplayAnimation: false,
    checkSetDraggable: jest.fn(),
    card: {
      isStackBase,
      setRaiseAndShadowOffset: jest.fn(),
      setEmpathy: jest.fn(),
    },
  });
  return child as unknown as LayoutChild;
}

async function nextAnimationFrame(): Promise<number> {
  return await new Promise<number>((resolve) => {
    requestAnimationFrame((timestamp) => {
      resolve(timestamp);
    });
  });
}

test("a complete unified projection installs atomically without reloading", () => {
  const store = setUpUnifiedStore();
  const originalStore = globals.store;
  const originalConnection = globals.lobby.conn;
  const reload = jest.spyOn(globalThis.history, "go");
  const navigate = jest.spyOn(globalThis.history, "replaceState");
  const unregisterObservers = jest.fn();
  let animateFastDuringRebuild = false;
  const redrawUnifiedProjection = jest.fn(() => {
    animateFastDuringRebuild = globals.animateFast;
  });
  globals.stateObserver = { unregisterObservers } as never;
  globals.lobby.ui = { redrawUnifiedProjection } as never;
  globals.animateFast = false;
  globals.editingNote = 0 as CardOrder;
  let updates = 0;
  store.subscribe(() => {
    updates++;
  });

  gameCommands.get("researchUnifiedProjection")!(
    projection({
      selectedBoundary: 1,
      actions: historicalProjectionActions(),
    }),
  );

  const state = store.getState();
  expect(globals.store).toBe(originalStore);
  expect(globals.lobby.conn).toBe(originalConnection);
  expect(reload).not.toHaveBeenCalled();
  expect(navigate).not.toHaveBeenCalled();
  expect(updates).toBe(1);
  expect(unregisterObservers).toHaveBeenCalledTimes(1);
  expect(redrawUnifiedProjection).toHaveBeenCalledTimes(1);
  expect(animateFastDuringRebuild).toBe(true);
  expect(globals.animateFast).toBe(false);
  expect(globals.editingNote).toBeNull();
  expect(state.metadata.ourPlayerIndex).toBe(1);
  expect(state.playing).toBe(false);
  expect(state.shadowing).toBe(true);
  expect(state.replay.active).toBe(true);
  expect(state.unifiedController).toMatchObject({
    viewedSeat: 1,
    selectedBoundary: 1,
    liveBoundary: 3,
    projectionRevision: 8,
    projectionInstalled: true,
  });
  expect(state.notes.ourNotes[0]?.text).toBe("Cathy's note");
  expect(state.visibleState?.turn.segment).toBe(1);
  expect(state.visibleState?.deck[5]).toMatchObject({
    suitIndex: null,
    rank: null,
  });
  expect(state.visibleState?.deck[0]).toMatchObject({
    suitIndex: 0,
    rank: 1,
  });
  expect(state.cardIdentities[5]).toEqual({ suitIndex: null, rank: null });
});

test("same-seat history projections retain observers and the renderer", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  const unregisterObservers = jest.fn();
  const redrawUnifiedProjection = jest.fn();
  globals.stateObserver = { unregisterObservers } as never;
  globals.lobby.ui = { redrawUnifiedProjection } as never;
  handler(projection({ viewedSeat: 0 }));
  unregisterObservers.mockClear();
  redrawUnifiedProjection.mockClear();
  const originalStore = globals.store;
  const originalConnection = globals.lobby.conn;
  const retainedNode = {} as never;
  globals.deck = [retainedNode];
  const reload = jest.spyOn(globalThis.history, "go");
  const navigate = jest.spyOn(globalThis.history, "replaceState");

  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      projectionRevision: 9,
      actions: historicalProjectionActions(),
    }),
  );

  expect(unregisterObservers).not.toHaveBeenCalled();
  expect(redrawUnifiedProjection).not.toHaveBeenCalled();
  expect(globals.deck[0]).toBe(retainedNode);
  expect(globals.store).toBe(originalStore);
  expect(globals.lobby.conn).toBe(originalConnection);
  expect(reload).not.toHaveBeenCalled();
  expect(navigate).not.toHaveBeenCalled();
  expect(store.getState().unifiedController).toMatchObject({
    viewedSeat: 0,
    selectedBoundary: 1,
    projectionRevision: 9,
  });
});

test("accepted-action projections require a verified appended action suffix", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  const allActions = historicalProjectionActions();
  const initialActions = allActions.slice(0, 10);
  const unregisterObservers = jest.fn();
  const redrawUnifiedProjection = jest.fn();
  globals.stateObserver = { unregisterObservers } as never;
  globals.lobby.ui = { redrawUnifiedProjection } as never;
  handler(projection({ viewedSeat: 0, actions: initialActions }));
  const installed = store.getState();
  unregisterObservers.mockClear();
  redrawUnifiedProjection.mockClear();
  const mismatchedActions = [draw(0, 0, 1, 1), ...allActions.slice(1)];

  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      projectionRevision: 9,
      actions: mismatchedActions,
      transitionKind: "acceptedAction",
      pendingFollowToken: 1,
    } as never),
  );

  expect(store.getState()).toBe(installed);

  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      projectionRevision: 9,
      actions: allActions,
      transitionKind: "acceptedAction",
      pendingFollowToken: 1,
    } as never),
  );

  expect(store.getState().unifiedController).toMatchObject({
    projectionRevision: 9,
    transitionKind: "acceptedAction",
    pendingFollowToken: 1,
  });
  expect(store.getState().replay.actions).toHaveLength(allActions.length);
  expect(unregisterObservers).not.toHaveBeenCalled();
  expect(redrawUnifiedProjection).not.toHaveBeenCalled();

  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      projectionRevision: 10,
      actions: allActions,
      notes: ["updated during transition"],
      transitionKind: "acceptedAction",
      pendingFollowToken: 1,
    }),
  );

  expect(store.getState().unifiedController?.projectionRevision).toBe(10);
  expect(store.getState().notes.ourNotes[0]?.text).toBe(
    "updated during transition",
  );
});

test("changed-seat rebuilds do not accumulate card renderer state", () => {
  setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  const staleDeckNode = {} as never;
  const staleStackBase = {} as never;
  globals.deck = [staleDeckNode];
  globals.stackBases = [staleStackBase];
  const registrySizesAtRebuild: Array<readonly [number, number]> = [];
  globals.stateObserver = { unregisterObservers: jest.fn() } as never;
  globals.lobby.ui = {
    redrawUnifiedProjection: () => {
      registrySizesAtRebuild.push([
        globals.deck.length,
        globals.stackBases.length,
      ]);
      globals.deck.push({} as never);
      globals.stackBases.push({} as never);
    },
  } as never;

  handler(projection());
  handler(projection({ viewedSeat: 0, projectionRevision: 9 }));

  expect(registrySizesAtRebuild).toEqual([
    [0, 0],
    [0, 0],
  ]);
  expect(globals.deck).toHaveLength(1);
  expect(globals.stackBases).toHaveLength(1);
});

test("fast perspective layouts create no tweens and stay fixed across frames", async () => {
  const base = initialState(getDefaultMetadata(2));
  globals.store = {
    getState: () => ({ ...base, visibleState: base.ongoingGame }),
  } as never;
  const previousAnimateFast = globals.animateFast;
  globals.animateFast = true;
  jest
    .spyOn(globals.layers.card, "batchDraw")
    .mockImplementation(() => globals.layers.card);
  const hand = new CardLayout({ width: 400, height: 200 });
  const handCard = fakeLayoutChild();
  const playStack = new PlayStack({ width: 100, height: 200 });
  const playedCard = fakeLayoutChild(true);

  hand.addChild(handCard);
  playStack.addChild(playedCard);
  const firstFrame = {
    hand: handCard.position(),
    handScale: handCard.scale(),
    play: playedCard.position(),
    playScale: playedCard.scale(),
  };

  await nextAnimationFrame();
  await nextAnimationFrame();

  expect(handCard.tween).toBeNull();
  expect(playedCard.tween).toBeNull();
  expect({
    hand: handCard.position(),
    handScale: handCard.scale(),
    play: playedCard.position(),
    playScale: playedCard.scale(),
  }).toEqual(firstFrame);
  globals.animateFast = previousAnimateFast;
});

test("unified replay navigation is silent while accepted actions retain live sounds", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  const allActions = historicalProjectionActions();
  const initialActions = allActions.slice(0, 10);
  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      liveBoundary: 1,
      actions: allActions,
    }),
  );
  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 0,
      liveBoundary: 1,
      projectionRevision: 9,
      actions: initialActions,
    }),
  );
  const historicalState = store.getState();
  handler(
    projection({
      viewedSeat: 0,
      selectedBoundary: 1,
      liveBoundary: 1,
      projectionRevision: 10,
      actions: allActions,
    }),
  );
  const play = jest.fn();
  globals.game = { sounds: { play } } as never;
  const previousSettings = globals.lobby.settings;
  globals.lobby.settings = { ...previousSettings, soundMove: SoundMove.All };
  const liveState = store.getState();

  onNewSoundEffect(
    {
      gameState: liveState.ongoingGame,
      actions: liveState.replay.actions,
    },
    {
      gameState: historicalState.ongoingGame,
      actions: historicalState.replay.actions,
    },
  );

  expect(historicalState.replay.active).toBe(true);
  expect(liveState.replay.active).toBe(false);
  expect(play).not.toHaveBeenCalled();

  handler(
    projection({
      viewedSeat: 0,
      currentTurnSeat: 0,
      selectedBoundary: 2,
      liveBoundary: 2,
      projectionRevision: 11,
      actions: [...allActions, rankClue(2, 1, [], 0)],
      transitionKind: "acceptedAction",
      pendingFollowToken: 2,
    }),
  );
  const acceptedActionState = store.getState();
  onNewSoundEffect(
    {
      gameState: acceptedActionState.ongoingGame,
      actions: acceptedActionState.replay.actions,
    },
    {
      gameState: liveState.ongoingGame,
      actions: liveState.replay.actions,
    },
  );
  globals.lobby.settings = previousSettings;

  expect(play).toHaveBeenCalledTimes(1);
});

test("terminal identities reveal and rehydrate only through the atomic projection", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  handler(projection());
  const cardIdentities = store
    .getState()
    .cardIdentities.map((_identity, index) =>
      index === 5
        ? { suitIndex: 1 as const, rank: 2 as const }
        : { suitIndex: 0 as const, rank: 1 as const },
    );

  handler({
    ...projection({
      projectionRevision: 9,
      selectedBoundary: 1,
      liveBoundary: 1,
      actions: historicalProjectionActions(),
    }),
    cardIdentities,
  } as UnifiedProjectionData);

  expect(store.getState().cardIdentities[5]).toEqual({
    suitIndex: 1,
    rank: 2,
  });
  expect(
    store
      .getState()
      .replay.actions.find(
        (action) => action.type === "draw" && action.order === 5,
      ),
  ).toMatchObject({ suitIndex: 1, rank: 2 });
});

test("installed unified projections reject duplicate and stale revisions", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  handler(projection());
  const installed = store.getState();

  handler(projection({ viewedSeat: 0 }));
  handler(projection({ projectionRevision: 6, viewedSeat: 0 }));

  expect(store.getState()).toBe(installed);
  expect(store.getState().unifiedController?.viewedSeat).toBe(1);
});

test("pause state is replaced atomically with the unified projection", () => {
  const store = setUpUnifiedStore();

  gameCommands.get("researchUnifiedProjection")!(
    projection({ paused: true, pausePlayerIndex: 1 }),
  );

  expect(store.getState().pause).toEqual({
    active: true,
    playerIndex: 1,
    queued: false,
  });
});

test("termination vote state belongs to the atomic viewed-seat projection", () => {
  const store = setUpUnifiedStore();

  gameCommands.get("researchUnifiedProjection")!(
    projection({ terminationVote: true }),
  );
  gameCommands.get("voteChange")!({ vote: false });

  expect(store.getState().unifiedController?.terminationVote).toBe(true);
});

test("legacy projection fragments cannot mix into an installed unified revision", () => {
  const store = setUpUnifiedStore();
  const handler = gameCommands.get("researchUnifiedProjection")!;
  handler(projection());
  const installed = store.getState();

  gameCommands.get("noteListPlayer")!({ notes: ["wrong revision"] });
  gameCommands.get("gameActionList")!({ tableID: 17, list: [] });
  gameCommands.get("gameAction")!({
    tableID: 17,
    projectionRevision: 7,
    action: draw(0, 0, 0, 5),
  });
  gameCommands.get("pause")!({ active: true, playerIndex: 1 });

  expect(store.getState()).toBe(installed);
  expect(store.getState().notes.ourNotes[0]?.text).toBe("Cathy's note");
  expect(store.getState().pause.active).toBe(false);
});

test("the unified controller uses its distinct route while regular paths stay unchanged", () => {
  const { getGamePath } = gameCommandsModule as unknown as {
    getGamePath: (data: {
      tableID: number;
      sharedReplay: boolean;
      replay: boolean;
      databaseID: number;
      shadowing: boolean;
      ourPlayerIndex: number;
      unifiedController?: unknown;
    }) => string;
  };
  const common = {
    tableID: 12,
    sharedReplay: false,
    replay: false,
    databaseID: 0,
    shadowing: false,
    ourPlayerIndex: 0,
  };

  expect(
    getGamePath({ ...common, unifiedController: { protocolCapability: true } }),
  ).toBe("/unified-game/12");
  expect(getGamePath({ ...common, shadowing: true, ourPlayerIndex: 1 })).toBe(
    "/game/12/shadow/1",
  );
});

test("the unified browser route fails closed when init lacks the protocol", () => {
  const { validateUnifiedProtocol } = gameCommandsModule as unknown as {
    validateUnifiedProtocol: (
      data: { unifiedController?: unknown },
      pathname: string,
    ) => void;
  };

  expect(() => {
    validateUnifiedProtocol({}, "/unified-game/17");
  }).toThrow("Unified manual controller protocol is unavailable");
  expect(() => {
    validateUnifiedProtocol({}, "/game/17");
  }).not.toThrow();
});

test("the ordinary shared-replay finish command cannot replace a terminal unified projection", () => {
  const store = setUpUnifiedStore();
  const projectionHandler = gameCommands.get("researchUnifiedProjection")!;
  projectionHandler(
    projection({
      currentTurnSeat: -1,
      finished: true,
      selectedBoundary: 1,
      liveBoundary: 1,
      actions: historicalProjectionActions(),
      capabilities: {
        canAct: false,
        canEditViewedPlayerNotes: false,
        canPause: false,
        canTerminate: false,
        canRestart: true,
      },
    }),
  );
  const terminalProjection = store.getState();
  const navigate = jest.spyOn(globalThis.history, "replaceState");

  gameCommands.get("finishOngoingGame")!({
    databaseID: 123,
    sharedReplayLeader: "Alice",
  });

  expect(navigate).not.toHaveBeenCalled();
  expect(store.getState()).toBe(terminalProjection);
  expect(store.getState().unifiedController).toMatchObject({
    currentTurnSeat: -1,
    projectionRevision: 8,
    projectionInstalled: true,
  });
});

test("unified init clears legacy shared-replay flags on terminal reconnect", () => {
  const base = initialState(getDefaultMetadata(2));
  const store = createStore(stateReducer, {
    ...base,
    playing: false,
    finished: true,
    replay: {
      ...base.replay,
      active: true,
      segment: 3,
      shared: {
        segment: 3,
        useSharedSegments: true,
        leader: "Alice",
        amLeader: false,
      },
    },
  } as State);

  store.dispatch({
    type: "unifiedControllerInit",
    controller: {
      protocolCapability: "unified_manual_v1",
      viewedSeat: 1,
      currentTurnSeat: -1,
      selectedBoundary: 3,
      liveBoundary: 3,
      projectionRevision: 12,
      finished: true,
      capabilities: {
        canAct: false,
        canEditViewedPlayerNotes: false,
        canPause: false,
        canTerminate: false,
        canRestart: true,
      },
    },
  });

  expect(store.getState().finished).toBe(false);
  expect(store.getState().replay.shared).toBeNull();
  expect(store.getState().replay.active).toBe(false);
});
