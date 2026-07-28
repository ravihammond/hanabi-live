import { hsmTransportGolden } from "@hanabi-live/data";
import { ClueType, getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, describe, expect, jest, test } from "@jest/globals";
// eslint-disable-next-line import-x/no-relative-packages
import goldenFixture from "../../../../../testdata/research-hsm/transport-v1.json";
import { initialState } from "../reducers/initialStates/initialState";
import { Elements } from "./Elements";
import { RankButton } from "./RankButton";
import { globals } from "./UIGlobals";
import { checkLegal } from "./clues";
import {
  destroyHSMInspector,
  getHSMActionAnnotation,
  getHSMCardTooltipHTML,
  handleHSMPhysicalTruth,
  handleHSMPhysicalTruthPending,
  handleHSMSnapshot,
  handleHSMSnapshotFailure,
  handleHSMSnapshotPending,
  initHSMInspector,
  isHSMInspectionReadOnly,
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

function initAtAuthoritativeBoundary(
  config: HSMDebugInit,
  send: SendHSMCommand,
  actorPlayer = 0,
): void {
  initHSMInspector(config, send);
  setHSMTargetBoundary(0, 0, actorPlayer);
}

describe("native HSM inspector", () => {
  afterEach(() => {
    destroyHSMInspector();
    globals.elements = new Elements();
    globals.store = null;
  });

  test("renders only for an authorized viewer and keeps Historical as default", () => {
    initHSMInspector(debug, noOpSend as SendHSMCommand);

    const toolbar = document.querySelector("#hsm-debug-toolbar");
    expect(toolbar?.textContent).toContain("HSM Debug \u00B7 purple");
    expect(toolbar?.textContent).toContain("Perspective Alice");
    expect(toolbar?.textContent).toContain("Semantic profile pending");
    expect(toolbar?.textContent).toContain("Target action unavailable");
    expect(toolbar?.textContent).toContain("evidence Replay Boundary 0");
    expect(toolbar?.textContent).toContain("Historical viewpoint");
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
    expect(evidence.min).toBe("4");
    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        targetBoundary: 3,
        evidenceBoundary: 4,
      }),
    );
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

  test("follows the live edge until an explicit rewind freezes the target", () => {
    const send =
      jest.fn<
        (command: string, data: Readonly<Record<string, unknown>>) => void
      >();
    initHSMInspector(debug, send as SendHSMCommand);
    setHSMTargetBoundary(3, 4, 0);
    expect(isHSMInspectionReadOnly()).toBe(true);
    setHSMTargetBoundary(4, 4, 0);
    expect(isHSMInspectionReadOnly()).toBe(false);
    expect(
      document.querySelector("#hsm-debug-coordinate-summary")?.textContent,
    ).toContain("Target action 4 \u00B7 pre-action Replay Boundary 3");

    document
      .querySelector<HTMLButtonElement>("#hsm-debug-mode-hindsight")
      ?.click();
    const evidence = document.querySelector<HTMLInputElement>(
      "#hsm-debug-evidence",
    )!;
    evidence.value = "4";
    evidence.dispatchEvent(new Event("change", { bubbles: true }));
    const target =
      document.querySelector<HTMLInputElement>("#hsm-debug-target")!;
    target.value = "1";
    target.dispatchEvent(new Event("change", { bubbles: true }));
    expect(isHSMInspectionReadOnly()).toBe(true);

    setHSMTargetBoundary(5, 5, 1);

    expect(
      document.querySelector("#hsm-debug-coordinate-summary")?.textContent,
    ).toContain("Target action 2 \u00B7 pre-action Replay Boundary 1");
    expect(
      document.querySelector("#hsm-debug-coordinate-summary")?.textContent,
    ).toContain("Hindsight viewpoint \u00B7 evidence Replay Boundary 4");
    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        targetBoundary: 1,
        evidenceBoundary: 4,
      }),
    );
  });

  test("follows the server-bound historical actor without granting action authority", () => {
    const send =
      jest.fn<
        (command: string, data: Readonly<Record<string, unknown>>) => void
      >();
    initHSMInspector(debug, send as SendHSMCommand);
    setHSMTargetBoundary(4, 4, 1);
    handleHSMSnapshotPending({
      ...golden.snapshotPending,
      clientRequestID: 1,
      targetBoundary: 3,
      evidenceBoundary: 3,
      perspectivePlayer: 1,
      actorPlayer: 0,
    });

    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        clientRequestID: 2,
        targetBoundary: 3,
        perspectivePlayer: 0,
      }),
    );
    expect(
      document.querySelector("#hsm-debug-coordinate-summary")?.textContent,
    ).toContain("Perspective Alice");
    expect(
      document.querySelector("#hsm-debug-read-only")?.textContent,
    ).toContain("diagnostic indicators do not change legal actions");
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
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
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
    expect(actionDetail).toContain(first.label);
    expect(actionDetail).toContain(second.label);
    expect(actionDetail).toContain("direct-clue: mask 1");
    expect(actionDetail).toContain("finesse: mask 2");

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
    expect(cardDetail).toContain("hierarchy-resolved");
    expect(document.querySelector("select[data-hsm-diagnosis]")).toBeNull();
  });

  test("keeps action-time use separate from hindsight reinterpretation", () => {
    const hindsightViolation = {
      ...goldenDiagnosis,
      classifications: [
        {
          action_id: 3,
          classifier: "hierarchy-resolved" as const,
          follow: false,
          violation: true,
        },
      ],
    };
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        evidence_boundary: 1,
        diagnoses: [hindsightViolation],
        mistaken_actions: [
          {
            transition_index: 0,
            historical_actor: 0,
            action_id: 3,
            violating_classifiers: ["hierarchy-resolved" as const],
          },
        ],
      },
    });
    document
      .querySelector<HTMLButtonElement>("[data-hsm-ledger-action='3']")
      ?.click();

    const detail =
      document.querySelector("#hsm-debug-ledger-detail")?.textContent ?? "";
    expect(detail).toContain("Used at action time");
    expect(detail).toContain("Follow");
    expect(detail).toContain("Current hindsight interpretation");
    expect(detail).toContain("Violation");
    expect(detail).toContain("Mistaken Action");
    expect(detail).toContain(
      "universally proven accepted Canonical Transition 0",
    );
  });

  test("classifies only unanimous final action results for board annotations", () => {
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

    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses: [first, second],
      },
    });

    expect(getHSMActionAnnotation(1)).toBe("follow");
    expect(getHSMActionAnnotation(2)).toBe("violation");
    for (const actionID of [3, 4, 5]) {
      expect(getHSMActionAnnotation(actionID)).toBeNull();
    }
  });

  test("renders a unanimous classification on a real clue action square", () => {
    const rankActionID = 15;
    const diagnosis = {
      ...goldenDiagnosis,
      classifications: [
        {
          action_id: rankActionID,
          classifier: "hierarchy-resolved" as const,
          follow: true,
          violation: false,
        },
      ],
    };
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot({
      ...golden.snapshotMessage,
      snapshot: {
        ...golden.snapshotMessage.snapshot,
        diagnoses: [diagnosis],
      },
    });

    const metadata = getDefaultMetadata(2);
    const gameState = initialState(metadata);
    const visibleState = {
      ...gameState.ongoingGame,
      turn: {
        ...gameState.ongoingGame.turn,
        currentPlayerIndex: 0,
      },
    };
    globals.store = {
      getState: () => ({ ...gameState, visibleState }),
    } as unknown as NonNullable<typeof globals.store>;
    globals.elements.clueTargetButtonGroup = {
      getPressed: () => ({ targetPlayerIndex: 1 }),
    } as unknown as NonNullable<typeof globals.elements.clueTargetButtonGroup>;
    globals.elements.clueTypeButtonGroup = {
      getPressed: () => null,
    } as unknown as NonNullable<typeof globals.elements.clueTypeButtonGroup>;
    globals.elements.giveClueButton = {
      setEnabled: jest.fn(),
    } as unknown as NonNullable<typeof globals.elements.giveClueButton>;
    const rankButton = new RankButton({
      width: 20,
      height: 20,
      clue: { type: ClueType.Rank, value: 1 },
      label: "1",
    });
    globals.elements.rankClueButtons = [rankButton];

    checkLegal();

    expect(rankButton.background.stroke()).toBe("#55c870");
    expect(rankButton.background.strokeWidth()).toBe(3);
    expect(rankButton.background.dash()).toEqual([]);
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
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
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
    expect(tooltip).toContain(first.label);
    expect(tooltip).toContain("direct-clue: mask 1");
    expect(tooltip).toContain("D2:");
    expect(tooltip).toContain(second.label);
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
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
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
    initAtAuthoritativeBoundary(debug, noOpSend as SendHSMCommand);
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshotFailure(golden.snapshotFailure);

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    const failure =
      document.querySelector("#hsm-debug-failure")?.textContent ?? "";
    expect(failure).toContain("HSM diagnostics unavailable");
    expect(failure).toContain("Semantic program unsatisfiable");
    expect(failure).toContain("Exact solving");
    expect(failure).toContain(
      "The semantic program could not produce a complete diagnostic result.",
    );
    expect(failure).not.toContain("identity");
    expect(getHSMActionAnnotation(3)).toBeNull();
    expect(getHSMCardTooltipHTML(7)).toBe("");
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

  test("keeps Physical Truth display-only and clears it across coordinates", () => {
    const send =
      jest.fn<
        (command: string, data: Readonly<Record<string, unknown>>) => void
      >();
    initAtAuthoritativeBoundary(
      { ...debug, physicalTruthGranted: true },
      send as SendHSMCommand,
    );
    handleHSMSnapshotPending(golden.snapshotPending);
    handleHSMSnapshot(golden.snapshotMessage);
    expect(getHSMActionAnnotation(3)).toBe("follow");

    document
      .querySelector<HTMLInputElement>("#hsm-debug-physical-truth")!
      .click();
    handleHSMPhysicalTruthPending(golden.physicalTruthPending);
    handleHSMPhysicalTruth(golden.physicalTruthMessage);

    expect(
      document.querySelector(".hsm-debug-physical-truth-panel"),
    ).not.toBeNull();
    expect(getHSMActionAnnotation(3)).toBe("follow");
    const semanticRequestsBefore = send.mock.calls.filter(
      ([command]) => command === "researchHSMRequest",
    );
    expect(semanticRequestsBefore.at(-1)?.[1]).not.toHaveProperty(
      "physicalTruth",
    );

    document
      .querySelector<HTMLButtonElement>("#hsm-debug-mode-hindsight")
      ?.click();

    expect(
      document.querySelector(".hsm-debug-physical-truth-panel"),
    ).toBeNull();
    expect(
      send.mock.calls.filter(
        ([command]) => command === "researchHSMPhysicalTruthRequest",
      ),
    ).toHaveLength(2);
  });
});
