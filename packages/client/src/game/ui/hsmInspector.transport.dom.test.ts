import {
  HSM_SNAPSHOT_UNAVAILABLE_ERROR,
  HSM_SNAPSHOT_UNAVAILABLE_REASON,
  hsmSnapshotRequestCommand,
  hsmTransportGolden,
} from "@hanabi-live/data";
import { afterEach, describe, expect, jest, test } from "@jest/globals";
// eslint-disable-next-line import-x/no-relative-packages
import goldenFixture from "../../../../../testdata/research-hsm/transport-v1.json";
import { gameCommands } from "./gameCommands";
import {
  destroyHSMInspector,
  handleHSMPhysicalTruthPending,
  handleHSMPhysicalTruthRejected,
  handleHSMSnapshot,
  handleHSMSnapshotPending,
  handleHSMSnapshotRejected,
  initHSMInspector,
} from "./hsmInspector";
import type { HSMDebugInit, SendHSMCommand } from "./hsmInspectorContract";
import { HSM_REQUEST_TIMEOUT_MS } from "./hsmInspectorCorrelation";

const golden = hsmTransportGolden.parse(goldenFixture);
const participantDebug = {
  protocolVersion: golden.protocolVersion,
  tableID: 42,
  capability: "switchable",
  identity: "opaque-participant",
  viewerKind: "participant",
  ownPerspective: 0,
  playerNames: ["Alice", "Bob"],
  archiveGenerationID: 7,
  physicalTruthGranted: false,
} as const satisfies HSMDebugInit;

function sendRecorder() {
  return jest.fn<
    (command: string, data: Readonly<Record<string, unknown>>) => void
  >();
}

function noOpSend(_command: string, _data: Readonly<Record<string, unknown>>) {
  // This test injects websocket messages through the normal command handlers.
}

describe("HSM transport correlation", () => {
  afterEach(() => {
    destroyHSMInspector();
    jest.useRealTimers();
  });

  test("negotiates protocol v1 and sends it on every client request", () => {
    const send = sendRecorder();
    initHSMInspector(participantDebug, send as SendHSMCommand);

    expect(send).toHaveBeenCalledWith("researchHSMRequest", {
      tableID: 42,
      protocolVersion: 1,
      archiveGenerationID: 7,
      clientRequestID: 1,
      targetBoundary: 0,
      evidenceBoundary: 0,
      perspectivePlayer: 0,
    });

    destroyHSMInspector();
    initHSMInspector(
      {
        ...participantDebug,
        viewerKind: "spectator",
        physicalTruthGranted: true,
      },
      send as SendHSMCommand,
    );
    document
      .querySelector<HTMLInputElement>("#hsm-debug-physical-truth")!
      .click();
    expect(send).toHaveBeenLastCalledWith("researchHSMPhysicalTruthRequest", {
      tableID: 42,
      protocolVersion: 1,
      archiveGenerationID: 7,
      clientRequestID: 1,
      targetBoundary: 0,
      perspectivePlayer: 0,
    });
  });

  test("the browser request cannot choose server-owned authorization or result fields", () => {
    const browserRequest = {
      tableID: 42,
      protocolVersion: 1,
      archiveGenerationID: 7,
      clientRequestID: 1,
      targetBoundary: 0,
      evidenceBoundary: 0,
      perspectivePlayer: 0,
    };
    expect(hsmSnapshotRequestCommand.safeParse(browserRequest).success).toBe(
      true,
    );

    for (const forbiddenField of [
      "identity",
      "principalID",
      "capability",
      "actorPlayer",
      "semanticProfileID",
      "authorityLegalProjection",
      "physicalTruthGranted",
    ]) {
      expect(
        hsmSnapshotRequestCommand.safeParse({
          ...browserRequest,
          [forbiddenField]: "browser-chosen",
        }).success,
      ).toBe(false);
    }
  });

  test("refuses a mismatched init protocol without sending a request", () => {
    const send = sendRecorder();
    initHSMInspector(
      { ...participantDebug, protocolVersion: 2 },
      send as SendHSMCommand,
    );

    expect(send).not.toHaveBeenCalled();
    expect(document.querySelector("#hsm-debug-unavailable")?.textContent).toBe(
      "HSM Debug unavailable: protocol 2 is not supported.",
    );
  });

  test("accepts only the server-bound profile, digest, and exact coordinates", () => {
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);

    handleHSMSnapshot({
      ...golden.snapshotMessage,
      authorityLegalProjectionDigest: "sha256:wrong",
    });
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();

    handleHSMSnapshot(golden.snapshotMessage);
    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(document.querySelector("#hsm-debug-current")?.textContent).toContain(
      golden.snapshotMessage.snapshot.semantic_program_id,
    );
  });

  test("a replacement generation clears prior output and ignores stale responses", () => {
    const send = sendRecorder();
    initHSMInspector(participantDebug, send as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot(golden.snapshotMessage);
    expect(document.querySelector("#hsm-debug-current")).not.toBeNull();

    initHSMInspector(
      {
        ...participantDebug,
        archiveGenerationID: 8,
      },
      send as SendHSMCommand,
    );

    expect(document.querySelector("#hsm-debug-current")).toBeNull();
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();
    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        archiveGenerationID: 8,
      }),
    );

    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot(golden.snapshotMessage);
    expect(document.querySelector("#hsm-debug-current")).toBeNull();
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();
  });

  test("normal websocket dispatch renders the rich canonical success and failure", () => {
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);
    gameCommands.get("hsmSnapshotPending")!(golden.snapshotPending);
    gameCommands.get("hsmSnapshot")!(golden.snapshotMessage);
    document
      .querySelector<HTMLButtonElement>("[data-hsm-ledger-action='3']")
      ?.click();

    const drawer =
      document.querySelector("#hsm-debug-drawer")?.textContent ?? "";
    expect(drawer).toContain("Action Ledger");
    expect(drawer).toContain("Card Ledger");
    expect(drawer).toContain("D1");
    expect(drawer).toContain("hierarchy-resolved");
    expect(drawer).toContain("Play Connections");
    expect(drawer).toContain("Connection Obligations");
    expect(drawer).toContain(
      golden.snapshotMessage.snapshot.semantic_program_id,
    );

    destroyHSMInspector();
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);
    gameCommands.get("hsmSnapshotPending")!(golden.snapshotPending);
    gameCommands.get("hsmSnapshotFailure")!(golden.snapshotFailure);
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toContain(
      golden.snapshotFailure.error,
    );
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toContain(
      "Semantic program unsatisfiable",
    );
  });

  test("a correlated unavailable response clears loading with one fixed privacy-safe message", () => {
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);
    gameCommands.get("hsmSnapshotPending")!(golden.snapshotPending);

    const unavailable = {
      ...golden.snapshotPending,
      reasonCode: HSM_SNAPSHOT_UNAVAILABLE_REASON,
      error: HSM_SNAPSHOT_UNAVAILABLE_ERROR,
    };
    gameCommands.get("hsmSnapshotUnavailable")!({
      ...unavailable,
      reasonCode: "internal_error",
    });
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();
    expect(document.querySelector("#hsm-debug-failure")).toBeNull();

    gameCommands.get("hsmSnapshotUnavailable")!({
      ...unavailable,
      serverRequestID: unavailable.serverRequestID + 1,
    });
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();

    gameCommands.get("hsmSnapshotUnavailable")!(unavailable);
    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(document.querySelector("#hsm-debug-plain-text")).toBeNull();
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toBe(
      HSM_SNAPSHOT_UNAVAILABLE_ERROR,
    );
  });

  test("correlated rejections leave loading instead of hanging", () => {
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);
    handleHSMSnapshotRejected(golden.snapshotRejected);
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toContain(
      golden.snapshotRejected.reasonCode,
    );

    destroyHSMInspector();
    initHSMInspector(
      {
        ...participantDebug,
        viewerKind: "spectator",
        physicalTruthGranted: true,
      },
      noOpSend as SendHSMCommand,
    );
    document
      .querySelector<HTMLInputElement>("#hsm-debug-physical-truth")!
      .click();
    handleHSMPhysicalTruthPending(golden.physicalTruthPending);
    handleHSMPhysicalTruthRejected(golden.physicalTruthRejected);
    expect(
      document.querySelector(".hsm-debug-physical-truth-failure")?.textContent,
    ).toContain(golden.physicalTruthRejected.reasonCode);
  });

  test("times out missing responses and clears the loading state", () => {
    jest.useFakeTimers();
    initHSMInspector(participantDebug, noOpSend as SendHSMCommand);

    jest.advanceTimersByTime(HSM_REQUEST_TIMEOUT_MS);

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toBe(
      "Diagnostic Snapshot request timed out.",
    );
  });

  test("spectator read-only state comes from viewerKind, not a magic identity", () => {
    initHSMInspector(
      {
        ...participantDebug,
        identity: "entirely-opaque-principal",
        viewerKind: "spectator",
      },
      noOpSend as SendHSMCommand,
    );

    expect(document.body.classList.contains("hsm-inspection-read-only")).toBe(
      true,
    );
    expect(document.querySelector("#hsm-debug-toolbar")?.textContent).toContain(
      "HSM Debug Spectator",
    );
  });
});
