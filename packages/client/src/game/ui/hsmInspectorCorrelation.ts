import type {
  HSMPhysicalTruthIdentity,
  HSMPhysicalTruthRejectedMessage,
  HSMResponseIdentity,
  HSMSnapshotRejectedMessage,
} from "./hsmInspectorContract";
import { HSM_PROTOCOL_VERSION } from "./hsmInspectorContract";

export const HSM_REQUEST_TIMEOUT_MS = 10_000;

export interface PendingHSMResponse {
  readonly serverRequestID: number | null;
  readonly clientRequestID: number;
  readonly archiveGenerationID: number;
  readonly targetBoundary: number;
  readonly evidenceBoundary: number;
  readonly perspectivePlayer: number;
  readonly actorPlayer: number;
  readonly semanticProfileID: number | null;
  readonly authorityLegalProjectionDigest: string | null;
}

export interface PendingHSMPhysicalTruth {
  readonly serverRequestID: number | null;
  readonly clientRequestID: number;
  readonly archiveGenerationID: number;
  readonly targetBoundary: number;
  readonly perspectivePlayer: number;
}

export function bindPendingSnapshot(
  message: HSMResponseIdentity,
  pending: PendingHSMResponse,
): PendingHSMResponse | null {
  if (
    pending.serverRequestID !== null
    || !matchesSnapshotCoordinates(message, pending)
    || message.protocolVersion !== HSM_PROTOCOL_VERSION
    || message.actorPlayer !== pending.actorPlayer
    || message.semanticProfileID < 0
    || !message.authorityLegalProjectionDigest.startsWith("sha256:")
  ) {
    return null;
  }
  return {
    ...pending,
    serverRequestID: message.serverRequestID,
    semanticProfileID: message.semanticProfileID,
    authorityLegalProjectionDigest: message.authorityLegalProjectionDigest,
  };
}

export function matchesPendingSnapshotResponse(
  message: HSMResponseIdentity,
  pending: PendingHSMResponse,
): boolean {
  return (
    message.protocolVersion === HSM_PROTOCOL_VERSION
    && matchesSnapshotCoordinates(message, pending)
    && pending.serverRequestID !== null
    && message.serverRequestID === pending.serverRequestID
    && pending.semanticProfileID !== null
    && message.semanticProfileID === pending.semanticProfileID
    && message.actorPlayer === pending.actorPlayer
    && pending.authorityLegalProjectionDigest !== null
    && message.authorityLegalProjectionDigest
      === pending.authorityLegalProjectionDigest
  );
}

export function matchesPendingSnapshotRejection(
  message: HSMSnapshotRejectedMessage,
  pending: PendingHSMResponse,
): boolean {
  return (
    message.protocolVersion === HSM_PROTOCOL_VERSION
    && matchesSnapshotCoordinates(message, pending)
  );
}

function matchesSnapshotCoordinates(
  message: Pick<
    HSMResponseIdentity,
    | "clientRequestID"
    | "archiveGenerationID"
    | "targetBoundary"
    | "evidenceBoundary"
    | "perspectivePlayer"
  >,
  pending: PendingHSMResponse,
): boolean {
  return (
    message.clientRequestID === pending.clientRequestID
    && message.archiveGenerationID === pending.archiveGenerationID
    && message.targetBoundary === pending.targetBoundary
    && message.evidenceBoundary === pending.evidenceBoundary
    && message.perspectivePlayer === pending.perspectivePlayer
  );
}

export function bindPendingPhysicalTruth(
  message: HSMPhysicalTruthIdentity,
  pending: PendingHSMPhysicalTruth,
): PendingHSMPhysicalTruth | null {
  if (
    pending.serverRequestID !== null
    || message.protocolVersion !== HSM_PROTOCOL_VERSION
    || !matchesPhysicalTruthCoordinates(message, pending)
  ) {
    return null;
  }
  return {
    ...pending,
    serverRequestID: message.serverRequestID,
  };
}

export function matchesPendingPhysicalTruthResponse(
  message: HSMPhysicalTruthIdentity,
  pending: PendingHSMPhysicalTruth,
): boolean {
  return (
    message.protocolVersion === HSM_PROTOCOL_VERSION
    && matchesPhysicalTruthCoordinates(message, pending)
    && pending.serverRequestID !== null
    && message.serverRequestID === pending.serverRequestID
  );
}

export function matchesPendingPhysicalTruthRejection(
  message: HSMPhysicalTruthRejectedMessage,
  pending: PendingHSMPhysicalTruth,
): boolean {
  return (
    message.protocolVersion === HSM_PROTOCOL_VERSION
    && matchesPhysicalTruthCoordinates(message, pending)
  );
}

function matchesPhysicalTruthCoordinates(
  message: Pick<
    HSMPhysicalTruthIdentity,
    | "clientRequestID"
    | "archiveGenerationID"
    | "targetBoundary"
    | "perspectivePlayer"
  >,
  pending: PendingHSMPhysicalTruth,
): boolean {
  return (
    message.clientRequestID === pending.clientRequestID
    && message.archiveGenerationID === pending.archiveGenerationID
    && message.targetBoundary === pending.targetBoundary
    && message.perspectivePlayer === pending.perspectivePlayer
  );
}

export class HSMRequestTimeouts {
  private snapshotTimeout: ReturnType<typeof globalThis.setTimeout> | undefined;
  private physicalTruthTimeout:
    | ReturnType<typeof globalThis.setTimeout>
    | undefined;

  public scheduleSnapshot(onTimeout: () => void): void {
    this.clearSnapshot();
    this.snapshotTimeout = globalThis.setTimeout(
      onTimeout,
      HSM_REQUEST_TIMEOUT_MS,
    );
  }

  public clearSnapshot(): void {
    if (this.snapshotTimeout !== undefined) {
      globalThis.clearTimeout(this.snapshotTimeout);
      this.snapshotTimeout = undefined;
    }
  }

  public schedulePhysicalTruth(onTimeout: () => void): void {
    this.clearPhysicalTruth();
    this.physicalTruthTimeout = globalThis.setTimeout(
      onTimeout,
      HSM_REQUEST_TIMEOUT_MS,
    );
  }

  public clearPhysicalTruth(): void {
    if (this.physicalTruthTimeout !== undefined) {
      globalThis.clearTimeout(this.physicalTruthTimeout);
      this.physicalTruthTimeout = undefined;
    }
  }

  public clearAll(): void {
    this.clearSnapshot();
    this.clearPhysicalTruth();
  }
}
