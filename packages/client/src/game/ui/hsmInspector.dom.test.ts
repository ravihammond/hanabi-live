import { afterEach, describe, expect, jest, test } from "@jest/globals";
import { hsmTransportGolden } from "@hanabi-live/data";
import goldenFixture from "../../../../../testdata/research-hsm/transport-v1.json";
import type {
  HSMDebugInit,
  SendHSMCommand,
} from "./hsmInspectorContract";
import {
  destroyHSMInspector,
  handleHSMSnapshot,
  handleHSMSnapshotFailure,
  handleHSMSnapshotPending,
  initHSMInspector,
  setHSMTargetBoundary,
} from "./hsmInspector";

const golden = hsmTransportGolden.parse(goldenFixture);
const debug = {
  protocolVersion: 1,
  tableID: 42,
  capability: "switchable",
  identity: "opaque-participant",
  viewerKind: "participant",
  ownPerspective: 0,
  playerNames: ["Alice", "Bob"],
  archiveGenerationID: 7,
  physicalTruthGranted: false,
} as const satisfies HSMDebugInit;

function noOpSend(
  _command: string,
  _data: Readonly<Record<string, unknown>>,
): void {
  // This test injects transport messages directly.
}

function acceptGoldenSnapshot(): void {
  handleHSMSnapshotPending(golden.snapshotPending);
  handleHSMSnapshot(golden.snapshotMessage);
}

describe("native HSM inspector", () => {
  afterEach(() => {
    destroyHSMInspector();
  });

  test("renders only for an authorized viewer and keeps Historical as default", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);

    expect(document.querySelector("#hsm-debug-toolbar")).not.toBeNull();
    expect(
      document
        .querySelector("#hsm-debug-mode-historical")
        ?.getAttribute("aria-pressed"),
    ).toBe("true");
    document
      .querySelector<HTMLButtonElement>("#hsm-debug-drawer-close")
      ?.click();
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(false);

    destroyHSMInspector();
    initHSMInspector(null, noOpSend as SendHSMCommand);
    expect(document.querySelector("#hsm-debug-root")).toBeNull();
  });

  test("uses replay target boundaries and explicit hindsight evidence", () => {
    const send =
      jest.fn<
        (command: string, data: Readonly<Record<string, unknown>>) => void
      >();
    initHSMInspector(debug, send as SendHSMCommand);
    setHSMTargetBoundary(3, 8, 1);
    document
      .querySelector<HTMLButtonElement>("#hsm-debug-mode-hindsight")
      ?.click();
    const evidence = document.querySelector<HTMLInputElement>(
      "#hsm-debug-evidence",
    )!;
    evidence.value = "6";
    evidence.dispatchEvent(new Event("change", { bubbles: true }));

    expect(
      document.querySelector("#hsm-debug-timeline")?.textContent,
    ).toContain("Evidence 6");
    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        protocolVersion: 1,
        archiveGenerationID: 7,
        targetBoundary: 3,
        evidenceBoundary: 6,
      }),
    );
  });

  test("renders canonical beliefs, classifications, connections, and obligations", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    acceptGoldenSnapshot();

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(
      document.querySelector("#hsm-debug-recorded")?.textContent,
    ).toContain("Action 3 | Follow: yes | Violation: no");
    document.querySelector<HTMLButtonElement>(".hsm-debug-card-badge")?.click();
    expect(document.querySelector("#hsm-debug-drawer")?.textContent).toContain(
      "direct-clue: mask 1",
    );
    document.querySelector<HTMLButtonElement>(".hsm-debug-action")?.click();
    expect(
      document.querySelector("#hsm-debug-action-details")?.textContent,
    ).toContain("Classifier:");
    expect(document.querySelector("#hsm-debug-drawer")?.textContent).toContain(
      "ordered prerequisites: #8 mask 4 → #9 mask 8",
    );
    expect(document.querySelector("#hsm-debug-drawer")?.textContent).toContain(
      "ordered candidates: #8 mask 4 (current) → #9 mask 8",
    );
  });

  test("leaves loading with a correlated typed failure", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshotFailure(golden.snapshotFailure);

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(document.querySelector("#hsm-debug-failure")?.textContent).toBe(
      golden.snapshotFailure.error,
    );
  });

  test("preserves presentation preferences only within the same run", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    document
      .querySelector<HTMLButtonElement>("#hsm-debug-drawer-close")
      ?.click();
    const labels = document.querySelector<HTMLSelectElement>(
      "#hsm-debug-card-labels",
    )!;
    labels.value = "off";
    labels.dispatchEvent(new Event("change", { bubbles: true }));

    initHSMInspector(debug, noOpSend as SendHSMCommand);
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(false);
    expect(
      document.querySelector<HTMLSelectElement>("#hsm-debug-card-labels")
        ?.value,
    ).toBe("off");

    initHSMInspector(
      { ...debug, tableID: 84 },
      noOpSend as SendHSMCommand,
    );
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(true);
  });

  test("Physical Truth availability is exactly the server grant", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    expect(
      document.querySelector<HTMLInputElement>("#hsm-debug-physical-truth")
        ?.disabled,
    ).toBe(true);

    destroyHSMInspector();
    initHSMInspector(
      { ...debug, physicalTruthGranted: true },
      noOpSend as SendHSMCommand,
    );
    expect(
      document.querySelector<HTMLInputElement>("#hsm-debug-physical-truth")
        ?.disabled,
    ).toBe(false);
  });
});
