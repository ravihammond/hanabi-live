import type {
  HSMPhysicalTruthRequestCommand,
  HSMSnapshotRequestCommand,
} from "@hanabi-live/data";

export {
  HSM_LEGAL_ACTION_COUNT,
  HSM_PROTOCOL_VERSION,
  HSM_SNAPSHOT_UNAVAILABLE_ERROR,
  HSM_SNAPSHOT_UNAVAILABLE_REASON,
} from "@hanabi-live/data";

export type {
  HSMActionTimeClassification,
  HSMBeliefReason,
  HSMCardBelief,
  HSMClassification,
  HSMConnectionCard,
  HSMConnectionObligation,
  HSMConventionApplication,
  HSMDiagnosis,
  HSMDiagnosticProjection,
  HSMFailure,
  HSMInvariantFailure,
  HSMMistakenAction,
  HSMPhysicalGuard,
  HSMPhysicalTruthFailureMessage,
  HSMPhysicalTruthIdentity,
  HSMPhysicalTruthMessage,
  HSMPhysicalTruthOverlay,
  HSMPhysicalTruthPendingMessage,
  HSMPhysicalTruthRejectedMessage,
  HSMPlayConnection,
  HSMResponseIdentity,
  HSMSemanticValue,
  HSMSnapshot,
  HSMSnapshotFailureMessage,
  HSMSnapshotMessage,
  HSMSnapshotPendingMessage,
  HSMSnapshotRejectedMessage,
  HSMSnapshotUnavailableMessage,
  HSMTransportGolden,
  HSMUnsatisfiableCore,
  HSMViolationWarning,
} from "@hanabi-live/data";

export type HSMDebugCapability = "own_perspective" | "switchable";
export type HSMViewerKind = "participant" | "spectator";

/** Server authorization plus the game-local fields needed by the inspector. */
export interface HSMDebugInit {
  readonly protocolVersion: number;
  readonly tableID: number;
  readonly capability: HSMDebugCapability;
  readonly identity: string;
  readonly viewerKind: HSMViewerKind;
  readonly ownPerspective: number;
  readonly playerNames: readonly string[];
  readonly archiveGenerationID: number;
  readonly physicalTruthGranted: boolean;
}

export interface HSMClientCommandData {
  readonly researchHSMRequest: HSMSnapshotRequestCommand;
  readonly researchHSMPhysicalTruthRequest: HSMPhysicalTruthRequestCommand;
}

export type SendHSMCommand = <Command extends keyof HSMClientCommandData>(
  command: Command,
  data: HSMClientCommandData[Command],
) => void;
