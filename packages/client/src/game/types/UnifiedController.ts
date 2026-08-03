import type { CardIdentity, GameAction, PlayerIndex } from "@hanabi-live/game";

export const UNIFIED_MANUAL_PROTOCOL = "unified_manual_v1" as const;
export const UNIFIED_TRANSITION_ACCEPTED_ACTION = "acceptedAction" as const;
export type UnifiedTransitionKind = typeof UNIFIED_TRANSITION_ACCEPTED_ACTION;

export interface UnifiedCapabilities {
  readonly canAct: boolean;
  readonly canEditViewedPlayerNotes: boolean;
  readonly canPause: boolean;
  readonly canTerminate: boolean;
  readonly canRestart: boolean;
}

/** Server-authoritative state for one seatless unified controller session. */
export interface UnifiedControllerInit {
  readonly protocolCapability: typeof UNIFIED_MANUAL_PROTOCOL;
  readonly viewedSeat: PlayerIndex;
  readonly currentTurnSeat: PlayerIndex | -1;
  readonly selectedBoundary: number;
  readonly liveBoundary: number;
  readonly projectionRevision: number;
  readonly finished: boolean;
  readonly capabilities: UnifiedCapabilities;
}

export interface UnifiedControllerState extends UnifiedControllerInit {
  /** Distinguishes init metadata from the first complete projection. */
  readonly projectionInstalled: boolean;
  readonly terminationVote: boolean;
  readonly transitionKind?: UnifiedTransitionKind;
  readonly pendingFollowToken?: number;
}

/** Complete, atomically-installable projection sent by the server. */
export interface UnifiedProjectionData extends Omit<
  UnifiedControllerInit,
  "protocolCapability"
> {
  readonly tableID: number;
  readonly paused: boolean;
  readonly pausePlayerIndex: PlayerIndex;
  readonly terminationVote: boolean;
  readonly actions: readonly GameAction[];
  readonly notes: readonly string[];
  readonly cardIdentities?: readonly CardIdentity[];
  readonly transitionKind?: UnifiedTransitionKind;
  readonly pendingFollowToken?: number;
}
