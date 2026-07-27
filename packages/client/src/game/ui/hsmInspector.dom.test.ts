import { hsmTransportGolden } from "@hanabi-live/data";
import { afterEach, describe, expect, jest, test } from "@jest/globals";
// eslint-disable-next-line import-x/no-relative-packages
import goldenFixture from "../../../../../testdata/research-hsm/transport-v1.json";
import {
  destroyHSMInspector,
  getHSMCardTooltipHTML,
  handleHSMSnapshot,
  handleHSMSnapshotFailure,
  handleHSMSnapshotPending,
  initHSMInspector,
  setHSMTargetBoundary,
} from "./hsmInspector";
import type { HSMDebugInit, SendHSMCommand } from "./hsmInspectorContract";

const golden = hsmTransportGolden.parse(goldenFixture);
const goldenDiagnosis = golden.snapshotMessage.snapshot.diagnoses[0]!;
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

function noOpSend(_command: string, _data: Readonly<Record<string, unknown>>) {
  // This test injects transport messages directly.
}

describe("native HSM inspector", () => {
  afterEach(() => {
    destroyHSMInspector();
    for (const action of document.querySelectorAll("[data-hsm-action-id]")) {
      action.remove();
    }
  });

  test("renders only for an authorized viewer and keeps Historical as default", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);

    expect(document.querySelector("#hsm-debug-toolbar")).not.toBeNull();
    expect(
      document.querySelector("#hsm-debug-toolbar strong")?.classList,
    ).toContain("hsm-debug-label-purple");
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

  test("renders selectable Action and Card Ledgers with correlated diagnosis detail", () => {
    const first = {
      ...goldenDiagnosis,
      label: `hsm-diagnosis:${"a".repeat(64)}` as const,
      card_beliefs: [
        {
          stable_card_id: 7,
          candidate_identity_mask: 3,
          reason_identity_masks: [{ reason: "direct-clue", identity_mask: 1 }],
        },
      ],
      play_connections: [
        {
          source_transition: 1,
          available_from_boundary: 2,
          provenance_id: 11,
          focus_card_id: 7,
          focus_identity_mask: 1,
          prerequisites: [{ stable_card_id: 8, identity_mask: 4 }],
        },
      ],
      connection_obligations: [],
      classifications: [
        {
          action_id: 3,
          classifier: "hierarchy-resolved" as const,
          follow: true,
          violation: false,
        },
      ],
      semantic_values: [
        {
          action_id: 3,
          category: "semantic-evidence",
          name: "direct-clue",
          active: true,
        },
      ],
    };
    const second = {
      ...first,
      label: `hsm-diagnosis:${"b".repeat(64)}` as const,
      card_beliefs: [
        {
          stable_card_id: 7,
          candidate_identity_mask: 3,
          reason_identity_masks: [{ reason: "finesse", identity_mask: 2 }],
        },
      ],
      play_connections: [],
      connection_obligations: [
        {
          source_transition: 1,
          available_from_boundary: 2,
          provenance_id: 12,
          kind: "prompt" as const,
          owner_player: 1,
          focus_card_id: 7,
          focus_identity_mask: 2,
          current_candidate_index: 0,
          candidates: [{ stable_card_id: 9, identity_mask: 8 }],
        },
      ],
      classifications: [
        {
          action_id: 3,
          classifier: "hierarchy-resolved" as const,
          follow: false,
          violation: true,
        },
      ],
    };
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses: [first, second],
      },
    });

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(
      [...document.querySelectorAll(".hsm-debug-ledger-tab")].map(
        (tab) => tab.textContent,
      ),
    ).toEqual(["Action Ledger", "Card Ledger"]);
    document
      .querySelector<HTMLButtonElement>("[data-hsm-ledger-action='3']")
      ?.click();
    const actionDetail =
      document.querySelector("#hsm-debug-ledger-detail")?.textContent ?? "";
    expect(actionDetail).toContain(
      "Violation possible \u2014 1 of 2 diagnoses",
    );
    expect(actionDetail).toContain("HSM Semantic Evidence (explanation)");
    expect(actionDetail).toContain("D1");
    expect(actionDetail).toContain("D2");

    document
      .querySelector<HTMLButtonElement>("#hsm-debug-card-ledger-tab")
      ?.click();
    document
      .querySelector<HTMLButtonElement>("[data-hsm-ledger-card='7']")
      ?.click();
    const cardDetail =
      document.querySelector("#hsm-debug-ledger-detail")?.textContent ?? "";
    expect(cardDetail).toContain("direct-clue: mask 1");
    expect(cardDetail).toContain("finesse: mask 2");
    expect(cardDetail).toContain("ordered prerequisites: #8 mask 4");
    expect(cardDetail).toContain("ordered candidates: #9 mask 8 (current)");
    expect(document.querySelector("select[data-hsm-diagnosis]")).toBeNull();
  });

  test("outlines only authority-legal actions with unanimous classifications", () => {
    for (const actionID of [1, 2, 3, 4, 5, 6]) {
      const square = document.createElement("button");
      square.dataset["hsmActionId"] = String(actionID);
      square.dataset["hsmAuthorityLegal"] = String(actionID !== 6);
      document.body.append(square);
    }
    function classify(action_id: number, follow: boolean, violation: boolean) {
      return {
        action_id,
        classifier: "hierarchy-resolved" as const,
        follow,
        violation,
      };
    }
    const first = {
      ...goldenDiagnosis,
      label: `hsm-diagnosis:${"a".repeat(64)}` as const,
      classifications: [
        classify(1, true, false),
        classify(2, false, true),
        classify(3, true, false),
        classify(4, false, true),
        classify(5, false, false),
        classify(6, true, false),
      ],
    };
    const second = {
      ...first,
      label: `hsm-diagnosis:${"b".repeat(64)}` as const,
      classifications: [
        classify(1, true, false),
        classify(2, false, true),
        classify(3, false, true),
        classify(4, false, false),
        classify(5, false, false),
        classify(6, true, false),
      ],
    };

    initHSMInspector(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses: [first, second],
      },
    });

    expect(
      document.querySelector('[data-hsm-action-id="1"]')?.className,
    ).toContain("hsm-board-action-follow");
    expect(
      document.querySelector('[data-hsm-action-id="2"]')?.className,
    ).toContain("hsm-board-action-violation");
    for (const actionID of [3, 4, 5, 6]) {
      const square = document.querySelector(
        `[data-hsm-action-id="${actionID}"]`,
      );
      expect(square?.classList.contains("hsm-board-action-follow")).toBe(false);
      expect(square?.classList.contains("hsm-board-action-violation")).toBe(
        false,
      );
    }
  });

  test("puts every diagnosis-labelled card clause and one Full details action in the tooltip", () => {
    const first = {
      ...goldenDiagnosis,
      label: `hsm-diagnosis:${"a".repeat(64)}` as const,
      card_beliefs: [
        {
          stable_card_id: 7,
          candidate_identity_mask: 3,
          reason_identity_masks: [{ reason: "direct-clue", identity_mask: 1 }],
        },
      ],
    };
    const second = {
      ...first,
      label: `hsm-diagnosis:${"b".repeat(64)}` as const,
      card_beliefs: [
        {
          stable_card_id: 7,
          candidate_identity_mask: 3,
          reason_identity_masks: [
            { reason: "save-principle", identity_mask: 2 },
          ],
        },
      ],
    };
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses: [first, second],
      },
    });

    const tooltip = getHSMCardTooltipHTML(7);
    expect(tooltip).toContain("D1:");
    expect(tooltip).toContain("direct-clue: mask 1");
    expect(tooltip).toContain("D2:");
    expect(tooltip).toContain("save-principle: mask 2");
    expect(tooltip.match(/Full details/g)).toHaveLength(1);
    expect(tooltip).not.toContain("HSM");
    expect(document.querySelector(".hsm-debug-card-badge")).toBeNull();
    expect(document.querySelector(".hsm-debug-card-summary")).toBeNull();

    const tooltipHost = document.createElement("div");
    tooltipHost.innerHTML = tooltip;
    document.body.append(tooltipHost);
    tooltipHost
      .querySelector<HTMLButtonElement>("[data-hsm-full-details]")
      ?.click();
    expect(
      document
        .querySelector("#hsm-debug-card-ledger-tab")
        ?.getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      document.querySelector("#hsm-debug-ledger-detail")?.textContent,
    ).toContain("Stable Card 7");
    tooltipHost.remove();
  });

  test("shows many diagnoses simultaneously without ranking, paging, or hidden identities", () => {
    const diagnoses = Array.from({ length: 24 }, (_, index) => ({
      ...goldenDiagnosis,
      label: `hsm-diagnosis:${index.toString(16).padStart(64, "0")}` as const,
      classifications: [
        {
          action_id: 3,
          classifier: "hierarchy-resolved" as const,
          follow: index % 2 === 0,
          violation: index % 2 === 1,
        },
      ],
    }));
    initHSMInspector(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses,
      },
    });
    document
      .querySelector<HTMLButtonElement>("[data-hsm-ledger-action='3']")
      ?.click();

    expect(
      document.querySelectorAll(".hsm-debug-diagnosis-alternative"),
    ).toHaveLength(24);
    const drawer =
      document.querySelector("#hsm-debug-drawer")?.textContent ?? "";
    expect(drawer).toContain("Violation possible \u2014 12 of 24 diagnoses");
    expect(drawer).not.toMatch(/probability|confidence|majority|next page/i);
    expect(drawer).not.toContain("identity 4");
    expect(document.querySelector("[data-hsm-diagnosis]")).toBeNull();
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

    initHSMInspector(debug, noOpSend as SendHSMCommand);
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(false);
    expect(document.querySelector("#hsm-debug-card-labels")).toBeNull();

    initHSMInspector({ ...debug, tableID: 84 }, noOpSend as SendHSMCommand);
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
