import type { PlayerIndex } from "@hanabi-live/game";
import type { State } from "../types/State";
import type {
  UnifiedControllerState,
  UnifiedProjectionData,
} from "../types/UnifiedController";
import { globals } from "./UIGlobals";

export function getUnifiedController(
  state: State,
): UnifiedControllerState | null {
  return state.unifiedController ?? null;
}

export function isUnifiedController(state: State): boolean {
  return getUnifiedController(state) !== null;
}

export function isUnifiedControllerFinished(state: State): boolean {
  return getUnifiedController(state)?.finished === true;
}

export function canUnifiedControllerAct(state: State): boolean {
  const controller = getUnifiedController(state);
  return (
    controller !== null
    && controller.projectionInstalled
    && !controller.finished
    && controller.capabilities.canAct
    && controller.viewedSeat === controller.currentTurnSeat
    && controller.selectedBoundary === controller.liveBoundary
  );
}

export function isPlayerProjection(state: State): boolean {
  return state.playing || isUnifiedController(state);
}

export function getReplayFinalBoundary(state: State): number | null {
  return (
    getUnifiedController(state)?.liveBoundary ?? state.ongoingGame.turn.segment
  );
}

export function canEditViewedPlayerNotes(state: State): boolean {
  const controller = getUnifiedController(state);
  return (
    controller === null || controller.capabilities.canEditViewedPlayerNotes
  );
}

export function canUnifiedControllerPause(state: State): boolean {
  return getUnifiedController(state)?.capabilities.canPause === true;
}

export function canPauseGame(state: State): boolean {
  return state.playing || canUnifiedControllerPause(state);
}

export function canUnifiedControllerTerminate(state: State): boolean {
  return getUnifiedController(state)?.capabilities.canTerminate === true;
}

export function canTerminateGame(state: State): boolean {
  return state.playing || canUnifiedControllerTerminate(state);
}

export function canUnifiedControllerRestart(state: State): boolean {
  return getUnifiedController(state)?.capabilities.canRestart === true;
}

export function canRestartGame(
  state: State,
  regularRestartController: boolean,
): boolean {
  const controller = getUnifiedController(state);
  return controller === null
    ? regularRestartController
    : controller.capabilities.canRestart;
}

export interface UnifiedActionEnvelope {
  readonly expectedActorSeat: PlayerIndex;
  readonly expectedLiveBoundary: number;
  readonly expectedProjectionRevision: number;
}

export function unifiedActionEnvelope(
  state: State,
): UnifiedActionEnvelope | null {
  const controller = getUnifiedController(state);
  if (controller === null || !canUnifiedControllerAct(state)) {
    return null;
  }
  return {
    expectedActorSeat: controller.viewedSeat,
    expectedLiveBoundary: controller.liveBoundary,
    expectedProjectionRevision: controller.projectionRevision,
  };
}

/** Request one complete projection without changing pages, stores, or sockets. */
export function requestUnifiedProjection(
  viewedSeat: PlayerIndex,
  selectedBoundary?: number,
): boolean {
  if (globals.store === null || globals.lobby.conn === null) {
    return false;
  }

  const controller = getUnifiedController(globals.store.getState());
  if (controller === null) {
    return false;
  }

  globals.lobby.conn.send("researchPerspective", {
    tableID: globals.lobby.tableID,
    viewedSeat,
    selectedBoundary: selectedBoundary ?? controller.selectedBoundary,
    expectedProjectionRevision: controller.projectionRevision,
  });
  return true;
}

/** Install one complete server projection with monotonic revision filtering. */
export function installUnifiedProjection(
  projection: UnifiedProjectionData,
): boolean {
  if (globals.store === null || projection.tableID !== globals.lobby.tableID) {
    return false;
  }

  const controller = getUnifiedController(globals.store.getState());
  if (
    controller === null
    || projection.projectionRevision < controller.projectionRevision
    || (projection.projectionRevision === controller.projectionRevision
      && controller.projectionInstalled)
  ) {
    return false;
  }

  const { stateObserver, store } = globals;
  globals.editingNote = null;
  globals.lastNote = "";
  stateObserver?.unregisterObservers();
  try {
    store.dispatch({
      type: "unifiedProjection",
      projection,
    });
  } catch (error: unknown) {
    stateObserver?.registerObservers(store);
    throw error;
  }

  if (globals.lobby.ui === null) {
    stateObserver?.registerObservers(store);
  } else {
    globals.lobby.ui.redrawUnifiedProjection();
  }
  return true;
}

/** Unified live events wait for the next complete projection to prevent mixed revisions. */
export function shouldApplyLiveGameAction(state: State): boolean {
  return !isUnifiedController(state);
}
