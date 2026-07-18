import { afterEach, describe, expect, jest, test } from "@jest/globals";
import {
  destroyHSMInspector,
  handleHSMSnapshot,
  initHSMInspector,
  setHSMTargetBoundary,
} from "./hsmInspector";

describe("native HSM inspector", () => {
  afterEach(() => {
    destroyHSMInspector();
  });

  test("renders only for an authorized viewer and keeps Historical as the default", () => {
    const send = jest.fn();
    initHSMInspector(
      {
        capability: "switchable",
        identity: "alice",
        ownPerspective: 0,
        playerNames: ["Alice", "Bob"],
        physicalTruthAllowed: true,
      },
      send,
    );

    expect(document.querySelector("#hsm-debug-toolbar")).not.toBeNull();
    expect(
      document
        .querySelector("#hsm-debug-mode-historical")
        ?.getAttribute("aria-pressed"),
    ).toBe("true");
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(true);

    document
      .querySelector<HTMLButtonElement>("#hsm-debug-drawer-close")
      ?.click();
    expect(document.body.classList.contains("hsm-inspector-open")).toBe(false);
    expect(document.querySelector("#hsm-debug-restore")).not.toBeNull();

    destroyHSMInspector();
    initHSMInspector(null, send);
    expect(document.querySelector("#hsm-debug-root")).toBeNull();
  });

  test("uses replay target boundaries, explicit hindsight evidence, and read-only pinned perspectives", () => {
    const send = jest.fn();
    initHSMInspector(
      {
        capability: "switchable",
        identity: "alice",
        ownPerspective: 0,
        playerNames: ["Alice", "Bob"],
        physicalTruthAllowed: true,
      },
      send,
    );

    setHSMTargetBoundary(3, 8, 1);
    document
      .querySelector<HTMLButtonElement>("#hsm-debug-mode-hindsight")
      ?.click();
    const evidence = document.querySelector<HTMLInputElement>(
      "#hsm-debug-evidence",
    );
    if (evidence === null) {
      throw new TypeError("missing hindsight evidence control");
    }
    evidence.value = "6";
    evidence.dispatchEvent(new Event("change", { bubbles: true }));
    document.querySelector<HTMLSelectElement>("#hsm-debug-perspective")!.value =
      "0";
    document
      .querySelector<HTMLSelectElement>("#hsm-debug-perspective")!
      .dispatchEvent(new Event("change", { bubbles: true }));

    expect(
      document.querySelector("#hsm-debug-read-only")?.textContent,
    ).toContain("not Alice's decision boundary");
    expect(
      document.querySelector("#hsm-debug-timeline")?.textContent,
    ).toContain("Target 3");
    expect(
      document.querySelector("#hsm-debug-timeline")?.textContent,
    ).toContain("Evidence 6");
    expect(send).toHaveBeenLastCalledWith(
      "researchHSMRequest",
      expect.objectContaining({
        targetBoundary: 3,
        evidenceBoundary: 6,
        perspectivePlayer: 0,
      }),
    );
  });

  test("shows loading instead of stale data and labels recorded versus current interpretation", () => {
    const send = jest.fn();
    initHSMInspector(
      {
        capability: "own_perspective",
        identity: "alice",
        ownPerspective: 0,
        playerNames: ["Alice", "Bob"],
        physicalTruthAllowed: false,
      },
      send,
    );
    setHSMTargetBoundary(2, 5, 0);
    expect(document.querySelector("#hsm-debug-loading")?.textContent).toContain(
      "Computing",
    );

    handleHSMSnapshot({
      requestID: 1,
      snapshot: {
        targetBoundary: 2,
        evidenceBoundary: 2,
        actionTimeClassification: "follow (recorded action 3)",
        diagnosticInterpretation: "2 follow, 1 violation",
        cards: [
          {
            stableCardID: 12,
            summary: "Ready Play · Prompt",
            notes: ["Play Ready", "Prompt"],
            connections: ["9 → 12"],
            obligations: ["Prompt Bob candidates 9"],
          },
        ],
        legalActions: [],
        ruleRows: [],
        relaxedSources: [],
        solver: { exactStatus: "exact_trusted" },
        capacity: {},
        plainText: "[hsm] exact",
      },
    });

    expect(document.querySelector("#hsm-debug-loading")).toBeNull();
    expect(
      document.querySelector("#hsm-debug-recorded")?.textContent,
    ).toContain("Immutable recorded action-time classification");
    expect(document.querySelector("#hsm-debug-current")?.textContent).toContain(
      "Current diagnostic interpretation",
    );
    expect(document.querySelector("#hsm-debug-plain-text")?.textContent).toBe(
      "[hsm] exact",
    );
    document.querySelector<HTMLButtonElement>(".hsm-debug-card-badge")?.click();
    expect(
      document
        .querySelector(".hsm-debug-card-badge")
        ?.getAttribute("aria-pressed"),
    ).toBe("true");
    expect(document.querySelector("#hsm-debug-drawer")?.textContent).toContain(
      "9 → 12",
    );
  });

  test("applies the selected card-label presentation", () => {
    const send = jest.fn();
    initHSMInspector(
      {
        capability: "own_perspective",
        identity: "alice",
        ownPerspective: 0,
        playerNames: ["Alice", "Bob"],
        physicalTruthAllowed: false,
      },
      send,
    );
    handleHSMSnapshot({
      requestID: 1,
      snapshot: {
        targetBoundary: 0,
        evidenceBoundary: 0,
        perspectivePlayer: 0,
        actionTimeClassification: null,
        diagnosticInterpretation: "neutral",
        cards: [
          {
            stableCardID: 12,
            summary: "Ready Play",
            notes: ["Play Ready"],
          },
        ],
        legalActions: [],
        ruleRows: [],
        relaxedSources: [],
        solver: {},
        capacity: {},
        plainText: "[hsm] exact",
      },
    });

    const labels = document.querySelector<HTMLSelectElement>(
      "#hsm-debug-card-labels",
    )!;
    labels.value = "summary";
    labels.dispatchEvent(new Event("change", { bubbles: true }));
    expect(document.querySelector(".hsm-debug-card-badge")).toBeNull();
    expect(
      document.querySelector(".hsm-debug-card-summary")?.textContent,
    ).toContain("Ready Play");

    labels.value = "off";
    labels.dispatchEvent(new Event("change", { bubbles: true }));
    expect(document.querySelector(".hsm-debug-card-summary")).toBeNull();
    expect(document.querySelector("#hsm-debug-drawer")?.textContent).toContain(
      "Card labels are hidden",
    );
  });

  test("rejects a late Physical Truth response after the overlay is disabled", () => {
    const send = jest.fn();
    initHSMInspector(
      {
        capability: "switchable",
        identity: "hsm_debug_spectator",
        ownPerspective: 0,
        playerNames: ["Alice", "Bob"],
        physicalTruthAllowed: true,
      },
      send,
    );

    handleHSMSnapshot({
      requestID: 1,
      snapshot: {
        targetBoundary: 0,
        evidenceBoundary: 0,
        perspectivePlayer: 0,
        actionTimeClassification: null,
        diagnosticInterpretation: "neutral",
        cards: [],
        legalActions: [],
        ruleRows: [],
        relaxedSources: [],
        solver: {},
        capacity: {},
        plainText: "[hsm] exact",
        physicalTruth: { cards: [{ stableCardID: 1, identity: 4 }] },
      },
    });

    expect(
      document.querySelector(".hsm-debug-physical-truth-panel"),
    ).toBeNull();
    expect(document.querySelector("#hsm-debug-loading")).not.toBeNull();
  });
});
