import type {
  HSMClassification,
  HSMConnectionObligation,
  HSMDebugInit,
  HSMDiagnosis,
  HSMFailure,
  HSMPhysicalTruthFailureMessage,
  HSMPhysicalTruthMessage,
  HSMPhysicalTruthOverlay,
  HSMPhysicalTruthPendingMessage,
  HSMPhysicalTruthRejectedMessage,
  HSMPlayConnection,
  HSMSemanticValue,
  HSMSnapshot,
  HSMSnapshotFailureMessage,
  HSMSnapshotMessage,
  HSMSnapshotPendingMessage,
  HSMSnapshotRejectedMessage,
  HSMSnapshotUnavailableMessage,
  SendHSMCommand,
} from "./hsmInspectorContract";
import { HSM_PROTOCOL_VERSION } from "./hsmInspectorContract";
import type {
  PendingHSMPhysicalTruth,
  PendingHSMResponse,
} from "./hsmInspectorCorrelation";
import {
  bindPendingPhysicalTruth,
  bindPendingSnapshot,
  HSMRequestTimeouts,
  matchesPendingPhysicalTruthRejection,
  matchesPendingPhysicalTruthResponse,
  matchesPendingSnapshotRejection,
  matchesPendingSnapshotResponse,
} from "./hsmInspectorCorrelation";

export type {
  HSMDebugInit,
  HSMFailure,
  HSMPhysicalTruthFailureMessage,
  HSMPhysicalTruthMessage,
  HSMPhysicalTruthPendingMessage,
  HSMPhysicalTruthRejectedMessage,
  HSMSnapshot,
  HSMSnapshotFailureMessage,
  HSMSnapshotMessage,
  HSMSnapshotPendingMessage,
  HSMSnapshotRejectedMessage,
  HSMSnapshotUnavailableMessage,
} from "./hsmInspectorContract";

interface InspectorState {
  targetBoundary: number;
  maxBoundary: number;
  gameFinished: boolean;
  evidenceBoundary: number;
  actorPlayer: number;
  perspectivePlayer: number;
  followActor: boolean;
  followLiveEdge: boolean;
  targetPinned: boolean;
  historical: boolean;
  drawerOpen: boolean;
  nextClientRequestID: number;
  pendingSnapshot: PendingHSMResponse | null;
  physicalTruth: boolean;
  nextPhysicalTruthClientRequestID: number;
  pendingPhysicalTruth: PendingHSMPhysicalTruth | null;
  physicalTruthOverlay: HSMPhysicalTruthOverlay | null;
  physicalTruthFailure: string | null;
  semanticProfileID: number | null;
  selectedLedger: "action" | "card";
  selectedCardID: number | null;
  selectedActionID: number | null;
  snapshot: HSMSnapshot | null;
  loading: boolean;
  failure: string | null;
  typedFailure: HSMFailure | null;
}

interface InspectorPreferences {
  readonly drawerOpen: boolean;
}

let config: HSMDebugInit | null = null;
let sendCommand: SendHSMCommand | null = null;
let root: HTMLElement | null = null;
let state: InspectorState | null = null;
const requestTimeouts = new HSMRequestTimeouts();

export function initHSMInspector(
  debug: HSMDebugInit | null,
  send: SendHSMCommand,
): void {
  if (debug === null) {
    destroyHSMInspector();
    return;
  }
  const previousPreferenceKey = preferenceKey(config);
  const nextPreferenceKey = preferenceKey(debug);
  if (
    previousPreferenceKey !== null
    && previousPreferenceKey !== nextPreferenceKey
  ) {
    removePreferences(previousPreferenceKey);
  }
  teardownHSMInspector();
  if (debug.protocolVersion !== HSM_PROTOCOL_VERSION) {
    root = document.createElement("div");
    root.id = "hsm-debug-root";
    root.append(
      textElement(
        "div",
        `HSM Debug unavailable: protocol ${debug.protocolVersion} is not supported.`,
        "hsm-debug-unavailable",
      ),
    );
    document.body.append(root);
    return;
  }
  const preferences = loadPreferences(nextPreferenceKey);
  config = debug;
  sendCommand = send;
  state = {
    targetBoundary: 0,
    maxBoundary: 0,
    gameFinished: false,
    evidenceBoundary: 0,
    actorPlayer: debug.ownPerspective,
    perspectivePlayer: debug.ownPerspective,
    followActor: debug.capability === "switchable",
    followLiveEdge: true,
    targetPinned: false,
    historical: true,
    drawerOpen: preferences?.drawerOpen ?? true,
    nextClientRequestID: 0,
    pendingSnapshot: null,
    physicalTruth: false,
    nextPhysicalTruthClientRequestID: 0,
    pendingPhysicalTruth: null,
    physicalTruthOverlay: null,
    physicalTruthFailure: null,
    semanticProfileID: null,
    selectedLedger: "action",
    selectedCardID: null,
    selectedActionID: null,
    snapshot: null,
    loading: true,
    failure: null,
    typedFailure: null,
  };
  root = document.createElement("div");
  root.id = "hsm-debug-root";
  document.body.append(root);
  document.body.classList.add("hsm-debug-authorized");
  document.addEventListener("click", handleFullDetailsClick);
  render();
  requestSnapshot();
}

export function destroyHSMInspector(): void {
  const key = preferenceKey(config);
  teardownHSMInspector();
  if (key !== null) {
    removePreferences(key);
  }
}

function teardownHSMInspector() {
  requestTimeouts.clearAll();
  document.removeEventListener("click", handleFullDetailsClick);
  root?.remove();
  root = null;
  config = null;
  sendCommand = null;
  state = null;
  document.body.classList.remove(
    "hsm-inspector-open",
    "hsm-inspection-read-only",
    "hsm-debug-authorized",
  );
}

export function getHSMInspectorReservedWidth(): number {
  if (state?.drawerOpen !== true) {
    return 0;
  }
  return Math.min(390, Math.floor(window.innerWidth * 0.38));
}

export function getHSMToolbarReservedHeight(): number {
  if (root === null) {
    return 0;
  }
  return window.innerWidth <= 760 ? 190 : 60;
}

export function isHSMInspectionReadOnly(): boolean {
  if (state === null || config === null) {
    return false;
  }
  return (
    config.viewerKind === "spectator"
    || state.perspectivePlayer !== config.ownPerspective
    || state.targetBoundary !== state.maxBoundary
  );
}

export function setHSMTargetBoundary(
  targetBoundary: number,
  maxBoundary: number,
  actorPlayer: number,
  gameFinished = false,
): void {
  if (state === null || config === null) {
    return;
  }
  const nextMax = Math.max(0, maxBoundary);
  const atLiveEdge = targetBoundary >= nextMax;
  const latestAcceptedTarget = Math.max(0, nextMax - 1);
  const nextTarget = state.targetPinned
    ? Math.min(state.targetBoundary, latestAcceptedTarget)
    : atLiveEdge
      ? latestAcceptedTarget
      : Math.max(0, Math.min(targetBoundary, latestAcceptedTarget));
  if (
    state.targetBoundary === nextTarget
    && state.maxBoundary === nextMax
    && state.actorPlayer === actorPlayer
    && state.gameFinished === gameFinished
  ) {
    return;
  }
  state.targetBoundary = nextTarget;
  state.maxBoundary = nextMax;
  state.gameFinished = gameFinished;
  state.actorPlayer = actorPlayer;
  state.followLiveEdge = atLiveEdge && !state.targetPinned;
  if (state.followActor) {
    state.perspectivePlayer = actorPlayer;
  }
  if (state.historical) {
    state.evidenceBoundary = state.targetBoundary;
  } else if (state.followLiveEdge) {
    state.evidenceBoundary = state.maxBoundary;
  } else {
    state.evidenceBoundary = Math.min(
      state.maxBoundary,
      Math.max(state.targetBoundary, state.evidenceBoundary),
    );
  }
  state.snapshot = null;
  state.pendingPhysicalTruth = null;
  requestTimeouts.clearPhysicalTruth();
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = null;
  state.selectedCardID = null;
  state.selectedActionID = null;
  state.loading = true;
  state.failure = null;
  state.typedFailure = null;
  render();
  requestSnapshot();
  requestPhysicalTruth();
}

export function handleHSMSnapshotPending(
  message: HSMSnapshotPendingMessage,
): void {
  if (state === null) {
    return;
  }
  const pending = state.pendingSnapshot;
  if (pending === null) {
    return;
  }
  const bound = bindPendingSnapshot(message, pending);
  if (bound !== null) {
    state.pendingSnapshot = bound;
    state.semanticProfileID = message.semanticProfileID;
    if (message.actorPlayer >= 0) {
      state.actorPlayer = message.actorPlayer;
      if (
        state.followActor
        && state.perspectivePlayer !== message.actorPlayer
      ) {
        state.perspectivePlayer = message.actorPlayer;
        state.physicalTruth = false;
        renderAndRequest();
        return;
      }
    }
    render();
  }
}

export function handleHSMSnapshot(message: HSMSnapshotMessage): void {
  if (state === null || state.pendingSnapshot === null) {
    return;
  }
  if (!matchesPendingSnapshotResponse(message, state.pendingSnapshot)) {
    return;
  }
  const { snapshot } = message;
  state.snapshot = snapshot;
  state.pendingSnapshot = null;
  state.selectedActionID = null;
  state.loading = false;
  state.failure = null;
  state.typedFailure = null;
  requestTimeouts.clearSnapshot();
  render();
}

export function handleHSMSnapshotFailure(
  message: HSMSnapshotFailureMessage,
): void {
  if (
    state === null
    || state.pendingSnapshot === null
    || !matchesPendingSnapshotResponse(message, state.pendingSnapshot)
  ) {
    return;
  }
  state.snapshot = null;
  state.pendingSnapshot = null;
  state.loading = false;
  state.failure = message.error;
  state.typedFailure = message.failure;
  requestTimeouts.clearSnapshot();
  render();
}

export function handleHSMSnapshotUnavailable(
  message: HSMSnapshotUnavailableMessage,
): void {
  if (
    state === null
    || state.pendingSnapshot === null
    || !matchesPendingSnapshotResponse(message, state.pendingSnapshot)
  ) {
    return;
  }
  state.snapshot = null;
  state.pendingSnapshot = null;
  state.selectedActionID = null;
  state.loading = false;
  state.failure = message.error;
  state.typedFailure = null;
  requestTimeouts.clearSnapshot();
  render();
}

export function handleHSMSnapshotRejected(
  message: HSMSnapshotRejectedMessage,
): void {
  if (
    state === null
    || state.pendingSnapshot === null
    || !matchesPendingSnapshotRejection(message, state.pendingSnapshot)
  ) {
    return;
  }
  state.snapshot = null;
  state.pendingSnapshot = null;
  state.loading = false;
  state.failure = `Snapshot request rejected: ${message.reasonCode}`;
  state.typedFailure = null;
  requestTimeouts.clearSnapshot();
  render();
}

export function handleHSMPhysicalTruthPending(
  message: HSMPhysicalTruthPendingMessage,
): void {
  if (state === null) {
    return;
  }
  const pending = state.pendingPhysicalTruth;
  if (pending === null) {
    return;
  }
  const bound = bindPendingPhysicalTruth(message, pending);
  if (bound !== null) {
    state.pendingPhysicalTruth = bound;
  }
}

export function handleHSMPhysicalTruth(message: HSMPhysicalTruthMessage): void {
  if (
    state === null
    || !state.physicalTruth
    || state.pendingPhysicalTruth === null
    || !matchesPendingPhysicalTruthResponse(message, state.pendingPhysicalTruth)
  ) {
    return;
  }
  state.pendingPhysicalTruth = null;
  state.physicalTruthOverlay = message.overlay;
  state.physicalTruthFailure = null;
  requestTimeouts.clearPhysicalTruth();
  render();
}

export function handleHSMPhysicalTruthFailure(
  message: HSMPhysicalTruthFailureMessage,
): void {
  if (
    state === null
    || state.pendingPhysicalTruth === null
    || !matchesPendingPhysicalTruthResponse(message, state.pendingPhysicalTruth)
  ) {
    return;
  }
  state.pendingPhysicalTruth = null;
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = message.error;
  requestTimeouts.clearPhysicalTruth();
  render();
}

export function handleHSMPhysicalTruthRejected(
  message: HSMPhysicalTruthRejectedMessage,
): void {
  if (
    state === null
    || state.pendingPhysicalTruth === null
    || !matchesPendingPhysicalTruthRejection(
      message,
      state.pendingPhysicalTruth,
    )
  ) {
    return;
  }
  state.pendingPhysicalTruth = null;
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = `Physical Truth request rejected: ${message.reasonCode}`;
  requestTimeouts.clearPhysicalTruth();
  render();
}

function requestSnapshot() {
  if (state === null || sendCommand === null || config === null) {
    return;
  }
  state.loading = true;
  state.snapshot = null;
  state.failure = null;
  state.typedFailure = null;
  state.nextClientRequestID++;
  state.pendingSnapshot = {
    serverRequestID: null,
    clientRequestID: state.nextClientRequestID,
    archiveGenerationID: config.archiveGenerationID,
    targetBoundary: state.targetBoundary,
    evidenceBoundary: state.evidenceBoundary,
    perspectivePlayer: state.perspectivePlayer,
    actorPlayer: null,
    semanticProfileID: null,
    authorityLegalProjectionDigest: null,
  };
  const clientRequestID = state.nextClientRequestID;
  sendCommand("researchHSMRequest", {
    tableID: config.tableID,
    protocolVersion: HSM_PROTOCOL_VERSION,
    archiveGenerationID: config.archiveGenerationID,
    clientRequestID,
    targetBoundary: state.targetBoundary,
    evidenceBoundary: state.evidenceBoundary,
    perspectivePlayer: state.perspectivePlayer,
  });
  requestTimeouts.scheduleSnapshot(() => {
    if (state?.pendingSnapshot?.clientRequestID !== clientRequestID) {
      return;
    }
    state.pendingSnapshot = null;
    state.loading = false;
    state.failure = "Diagnostic Snapshot request timed out.";
    state.typedFailure = null;
    render();
  });
}

function requestPhysicalTruth() {
  if (
    state === null
    || sendCommand === null
    || config === null
    || !state.physicalTruth
    || !config.physicalTruthGranted
  ) {
    return;
  }
  state.nextPhysicalTruthClientRequestID++;
  state.pendingPhysicalTruth = {
    serverRequestID: null,
    clientRequestID: state.nextPhysicalTruthClientRequestID,
    archiveGenerationID: config.archiveGenerationID,
    targetBoundary: state.targetBoundary,
    perspectivePlayer: state.perspectivePlayer,
  };
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = null;
  sendCommand("researchHSMPhysicalTruthRequest", {
    tableID: config.tableID,
    protocolVersion: HSM_PROTOCOL_VERSION,
    archiveGenerationID: config.archiveGenerationID,
    clientRequestID: state.nextPhysicalTruthClientRequestID,
    targetBoundary: state.targetBoundary,
    perspectivePlayer: state.perspectivePlayer,
  });
  const clientRequestID = state.nextPhysicalTruthClientRequestID;
  requestTimeouts.schedulePhysicalTruth(() => {
    if (state?.pendingPhysicalTruth?.clientRequestID !== clientRequestID) {
      return;
    }
    state.pendingPhysicalTruth = null;
    state.physicalTruthOverlay = null;
    state.physicalTruthFailure = "Physical Truth request timed out.";
    render();
  });
}

function render() {
  if (root === null || state === null || config === null) {
    return;
  }
  root.replaceChildren(buildToolbar(), buildDrawer(), buildRestoreHandle());
  document.body.classList.toggle("hsm-inspector-open", state.drawerOpen);
  document.body.classList.toggle(
    "hsm-inspection-read-only",
    isHSMInspectionReadOnly(),
  );
  globalThis.dispatchEvent(new Event("hsm-diagnostic-render"));
}

export type HSMActionAnnotation = "follow" | "violation" | null;

export function getHSMActionAnnotation(actionID: number): HSMActionAnnotation {
  const snapshot = state?.snapshot;
  if (
    snapshot === null
    || snapshot === undefined
    || state?.loading === true
    || state?.failure !== null
    || snapshot.perspective_player !== state.actorPlayer
  ) {
    return null;
  }
  const classifications = snapshot.diagnoses.map((diagnosis) =>
    finalActionClassification(diagnosis, actionID),
  );
  if (classifications.includes(undefined)) {
    return null;
  }
  if (
    classifications.every(
      (classification) =>
        classification?.follow === true && !classification.violation,
    )
  ) {
    return "follow";
  }
  if (
    classifications.every(
      (classification) =>
        classification?.violation === true && !classification.follow,
    )
  ) {
    return "violation";
  }
  return null;
}

function buildToolbar(): HTMLElement {
  const toolbar = element("header", "hsm-debug-toolbar");
  toolbar.id = "hsm-debug-toolbar";
  const label = textElement("strong", "\u25c6 HSM Debug \u00b7 purple");
  label.className = "hsm-debug-label-purple";
  toolbar.append(
    label,
    buildCoordinateSummary(),
    labelledControl("Perspective", buildPerspectiveSelect()),
    buildTruthControl(),
    buildModeControl(),
  );
  return toolbar;
}

function buildCoordinateSummary(): HTMLElement {
  const perspective = state?.perspectivePlayer ?? 0;
  const perspectiveName =
    config?.playerNames[perspective] ?? `Player ${perspective + 1}`;
  const semanticProfile =
    state?.snapshot?.semantic_profile_id ?? state?.semanticProfileID;
  const target =
    (state?.maxBoundary ?? 0) === 0
      ? "Target action unavailable \u00b7 pre-action boundary 0"
      : `Target action ${(state?.targetBoundary ?? 0) + 1}`
        + ` \u00b7 pre-action boundary ${state?.targetBoundary ?? 0}`;
  const viewpoint =
    state?.historical === true ? "Historical viewpoint" : "Hindsight viewpoint";
  return textElement(
    "span",
    `Perspective ${perspectiveName}`
      + ` | ${capabilityLabel()}`
      + ` | Semantic profile ${semanticProfile ?? "pending"}`
      + ` | ${target}`
      + ` | ${viewpoint} \u00b7 evidence boundary ${state?.evidenceBoundary ?? 0}`,
    "hsm-debug-coordinate-summary",
  );
}

function capabilityLabel(): string {
  if (config === null) {
    return "";
  }
  return config.viewerKind === "spectator"
    ? "HSM Debug Spectator | read-only"
    : `${config.capability.replace("_", " ")} | read-only inspection`;
}

function buildPerspectiveSelect(): HTMLSelectElement {
  const select = document.createElement("select");
  select.id = "hsm-debug-perspective";
  if (config?.capability === "switchable") {
    const follow = new Option("Follow current actor", "actor");
    follow.selected = state?.followActor === true;
    select.add(follow);
    for (const [index, name] of config.playerNames.entries()) {
      const option = new Option(`Pin ${name}`, String(index));
      option.selected =
        state?.followActor === false && state.perspectivePlayer === index;
      select.add(option);
    }
  } else if (config !== null) {
    select.add(
      new Option(
        `Own perspective | ${config.playerNames[config.ownPerspective]}`,
        String(config.ownPerspective),
        true,
        true,
      ),
    );
    select.disabled = true;
  }
  select.addEventListener("change", () => {
    if (state === null) {
      return;
    }
    state.followActor = select.value === "actor";
    state.perspectivePlayer = state.followActor
      ? state.actorPlayer
      : Number(select.value);
    state.physicalTruth = false;
    renderAndRequest();
  });
  return select;
}

function buildTruthControl(): HTMLElement {
  const label = element("label", "hsm-debug-truth-control");
  const input = document.createElement("input");
  input.id = "hsm-debug-physical-truth";
  input.type = "checkbox";
  input.checked = state?.physicalTruth === true;
  const allowed = config?.physicalTruthGranted === true;
  input.disabled = !allowed;
  input.addEventListener("change", () => {
    if (state === null) {
      return;
    }
    state.physicalTruth = input.checked && allowed;
    if (state.physicalTruth) {
      requestPhysicalTruth();
    } else {
      state.pendingPhysicalTruth = null;
      requestTimeouts.clearPhysicalTruth();
      state.physicalTruthOverlay = null;
      state.physicalTruthFailure = null;
    }
    render();
  });
  label.append(input, document.createTextNode(" Physical Truth"));
  label.title = allowed
    ? "Privileged physical identities; graphics only"
    : "This viewer was not granted Physical Truth access";
  return label;
}

function buildModeControl(): HTMLElement {
  const group = element("div", "hsm-debug-mode");
  const historical = button("Historical", "hsm-debug-mode-historical", () => {
    if (state === null) {
      return;
    }
    state.historical = true;
    state.evidenceBoundary = state.targetBoundary;
    renderAndRequest();
  });
  historical.setAttribute("aria-pressed", String(state?.historical === true));
  const hindsight = button("Hindsight", "hsm-debug-mode-hindsight", () => {
    if (state === null) {
      return;
    }
    state.historical = false;
    state.evidenceBoundary = Math.max(
      state.targetBoundary,
      state.evidenceBoundary,
    );
    renderAndRequest();
  });
  hindsight.setAttribute("aria-pressed", String(state?.historical === false));
  group.append(historical, hindsight);
  return group;
}

function buildDrawer(): HTMLElement {
  const drawer = element("aside", "hsm-debug-drawer");
  drawer.id = "hsm-debug-drawer";
  drawer.hidden = state?.drawerOpen !== true;
  const heading = element("div", "hsm-debug-drawer-heading");
  heading.append(
    textElement("div", "HSM Inspector", "hsm-debug-title"),
    button("Close", "hsm-debug-drawer-close", toggleDrawer),
  );
  drawer.append(heading, buildTimeline(), buildReadOnlyNotice());
  const snapshot = state?.snapshot ?? null;
  if (state?.failure !== null && state?.failure !== undefined) {
    const failure = buildFailure(state.failure, state.typedFailure);
    failure.setAttribute("role", "alert");
    drawer.append(failure);
  } else if (state?.loading === true || snapshot === null) {
    const loading = textElement(
      "div",
      "Computing Diagnostic Snapshot...",
      "hsm-debug-loading",
    );
    loading.setAttribute("role", "status");
    drawer.append(loading);
  } else {
    drawer.append(buildSnapshot(snapshot));
  }
  if (
    state?.physicalTruthFailure !== null
    && state?.physicalTruthFailure !== undefined
  ) {
    drawer.append(
      labelledValue(
        "Physical Truth unavailable",
        state.physicalTruthFailure,
        undefined,
        "hsm-debug-physical-truth-failure",
      ),
    );
  } else if (
    state?.physicalTruthOverlay !== null
    && state?.physicalTruthOverlay !== undefined
  ) {
    drawer.append(buildPhysicalTruthOverlay(state.physicalTruthOverlay));
  }
  return drawer;
}

function buildFailure(
  message: string,
  typedFailure: HSMFailure | null,
): HTMLElement {
  const failure = element("div", "hsm-debug-failure");
  failure.id = "hsm-debug-failure";
  failure.append(textElement("strong", message));
  if (typedFailure === null) {
    return failure;
  }
  const category = sentenceCase(typedFailure.category);
  const phase = sentenceCase(typedFailure.phase);
  const explanation =
    typedFailure.category === "observer_evidence_unsatisfiable"
      ? "Observer-legal evidence could not produce a complete diagnostic result."
      : typedFailure.category === "semantic_program_unsatisfiable"
        ? "The semantic program could not produce a complete diagnostic result."
        : "A structural diagnostic invariant prevented a complete result.";
  failure.append(
    textElement("p", `${category} | ${phase}`),
    textElement("p", explanation),
  );
  return failure;
}

function sentenceCase(value: string): string {
  const spaced = value.replaceAll("_", " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function buildPhysicalTruthOverlay(
  overlay: HSMPhysicalTruthOverlay,
): HTMLElement {
  return labelledValue(
    "PHYSICAL TRUTH | graphics only",
    overlay.cards
      .map((card) => `#${card.stableCardID}: identity ${card.identity}`)
      .join(" | "),
    undefined,
    "hsm-debug-physical-truth-panel",
  );
}

function buildTimeline(): HTMLElement {
  const wrapper = element("section", "hsm-debug-timeline");
  wrapper.id = "hsm-debug-timeline";
  if ((state?.maxBoundary ?? 0) === 0) {
    wrapper.append(textElement("strong", "No accepted target action"));
  } else {
    const target = document.createElement("input");
    target.id = "hsm-debug-target";
    target.type = "range";
    target.min = "0";
    target.max = String(Math.max(0, (state?.maxBoundary ?? 0) - 1));
    target.value = String(state?.targetBoundary ?? 0);
    target.setAttribute("aria-label", "Target accepted action");
    target.addEventListener("change", () => {
      if (state === null) {
        return;
      }
      state.targetBoundary = Math.max(
        0,
        Math.min(Number(target.value), state.maxBoundary - 1),
      );
      state.targetPinned = state.targetBoundary < state.maxBoundary - 1;
      state.followLiveEdge = !state.targetPinned;
      state.evidenceBoundary = state.historical
        ? state.targetBoundary
        : Math.min(
            state.maxBoundary,
            Math.max(state.targetBoundary, state.evidenceBoundary),
          );
      renderAndRequest();
    });
    wrapper.append(
      textElement("strong", `Target action ${state!.targetBoundary + 1}`),
      target,
      textElement(
        "span",
        `Pre-action Replay Boundary ${state?.targetBoundary ?? 0}`
          + (state?.followLiveEdge === true
            ? " | following live edge"
            : " | rewound"),
      ),
    );
  }
  if (state?.historical === true) {
    wrapper.append(
      textElement(
        "span",
        `\u25c0 Historical | evidence boundary ${state.targetBoundary}`,
      ),
    );
  } else {
    const input = document.createElement("input");
    input.id = "hsm-debug-evidence";
    input.type = "range";
    input.min = String(state?.targetBoundary ?? 0);
    input.max = String(state?.maxBoundary ?? 0);
    input.value = String(state?.evidenceBoundary ?? 0);
    input.setAttribute("aria-label", "Later evidence accepted action");
    input.addEventListener("change", () => {
      if (state === null) {
        return;
      }
      state.evidenceBoundary = Math.min(
        state.maxBoundary,
        Math.max(state.targetBoundary, Number(input.value)),
      );
      renderAndRequest();
    });
    wrapper.append(
      textElement(
        "span",
        `Evidence ${state?.evidenceBoundary ?? 0}`
          + ` | evidence action ${state?.evidenceBoundary ?? 0}`
          + ` | post-action Replay Boundary ${state?.evidenceBoundary ?? 0}`,
      ),
      input,
      textElement(
        "span",
        `\u25b6 Hindsight interval ${state?.targetBoundary ?? 0} -> ${state?.evidenceBoundary ?? 0}`,
      ),
    );
  }
  return wrapper;
}

function buildReadOnlyNotice(): HTMLElement {
  const perspective = state?.perspectivePlayer ?? 0;
  const actor = state?.actorPlayer ?? 0;
  const selectedName =
    config?.playerNames[perspective] ?? `Player ${perspective + 1}`;
  const notice = element("p", "hsm-debug-read-only");
  notice.id = "hsm-debug-read-only";
  notice.textContent =
    perspective === actor
      ? `${selectedName}'s actual decision boundary | diagnostic indicators do not change legal actions.`
      : `This is not ${selectedName}'s decision boundary. Inspection is read-only; hypothetical action classification is unavailable.`;
  return notice;
}

function buildSnapshot(snapshot: HSMSnapshot): HTMLElement {
  const content = element("div", "hsm-debug-content");
  content.append(
    labelledValue(
      "Canonical diagnostic result",
      `generation ${snapshot.generation_id}`
        + ` | target ${snapshot.target_boundary}`
        + ` | evidence ${snapshot.evidence_boundary}`
        + ` | perspective ${snapshot.perspective_player + 1}`
        + ` | semantic profile ${snapshot.semantic_profile_id}`
        + ` | program ${snapshot.semantic_program_id}`,
      "hsm-debug-current",
    ),
    buildLedgers(snapshot),
  );
  return content;
}

function buildLedgers(snapshot: HSMSnapshot): HTMLElement {
  const section = element("section", "hsm-debug-ledgers");
  const tabs = element("nav", "hsm-debug-ledger-tabs");
  const actionTab = button(
    "Action Ledger",
    "hsm-debug-action-ledger-tab",
    () => {
      selectLedger("action");
    },
  );
  const cardTab = button("Card Ledger", "hsm-debug-card-ledger-tab", () => {
    selectLedger("card");
  });
  for (const [tab, ledger] of [
    [actionTab, "action"],
    [cardTab, "card"],
  ] as const) {
    tab.className = "hsm-debug-ledger-tab";
    tab.setAttribute("aria-pressed", String(state?.selectedLedger === ledger));
  }
  tabs.append(actionTab, cardTab);
  section.append(
    tabs,
    state?.selectedLedger === "card"
      ? buildCardLedger(snapshot)
      : buildActionLedger(snapshot),
  );
  return section;
}

function buildActionLedger(snapshot: HSMSnapshot): HTMLElement {
  const wrapper = element("section", "hsm-debug-ledger");
  const actionIDs = [
    ...new Set(
      snapshot.diagnoses.flatMap((diagnosis) =>
        diagnosis.classifications.map(
          (classification) => classification.action_id,
        ),
      ),
    ),
  ].toSorted((left, right) => left - right);
  wrapper.append(
    buildLedgerTable(
      "Action",
      actionIDs.map((actionID) => {
        const row = button(`Action ${actionID}`, "", () => {
          selectAction(actionID);
        });
        row.removeAttribute("id");
        row.dataset["hsmLedgerAction"] = String(actionID);
        row.setAttribute(
          "aria-pressed",
          String(state?.selectedActionID === actionID),
        );
        return row;
      }),
    ),
  );
  const selectedActionID = state?.selectedActionID ?? null;
  if (selectedActionID === null) {
    wrapper.append(
      buildEmptyLedgerDetail("Select an action row for full details."),
    );
  } else {
    wrapper.append(buildActionLedgerDetail(snapshot, selectedActionID));
  }
  return wrapper;
}

function buildCardLedger(snapshot: HSMSnapshot): HTMLElement {
  const wrapper = element("section", "hsm-debug-ledger");
  const cardIDs = diagnosticCardIDs(snapshot);
  wrapper.append(
    buildLedgerTable(
      "Stable Card",
      cardIDs.map((cardID) => {
        const row = button(`Card #${cardID}`, "", () => {
          selectCard(cardID);
        });
        row.removeAttribute("id");
        row.dataset["hsmLedgerCard"] = String(cardID);
        row.setAttribute(
          "aria-pressed",
          String(state?.selectedCardID === cardID),
        );
        return row;
      }),
    ),
  );
  const selectedCardID = state?.selectedCardID ?? null;
  if (selectedCardID === null) {
    wrapper.append(
      buildEmptyLedgerDetail("Select a card row for full details."),
    );
  } else {
    wrapper.append(buildCardLedgerDetail(snapshot, selectedCardID));
  }
  return wrapper;
}

function buildLedgerTable(
  heading: string,
  rows: readonly HTMLButtonElement[],
): HTMLTableElement {
  const table = document.createElement("table");
  table.className = "hsm-debug-ledger-table";
  const head = table.createTHead().insertRow();
  head.append(textElement("th", heading));
  const body = table.createTBody();
  for (const row of rows) {
    body.insertRow().insertCell().append(row);
  }
  if (rows.length === 0) {
    body.insertRow().insertCell().textContent =
      "No valid rows at this boundary";
  }
  return table;
}

function buildEmptyLedgerDetail(message: string): HTMLElement {
  const detail = textElement("section", message);
  detail.id = "hsm-debug-ledger-detail";
  detail.className = "hsm-debug-panel";
  return detail;
}

function buildActionLedgerDetail(
  snapshot: HSMSnapshot,
  actionID: number,
): HTMLElement {
  const detail = element("section", "hsm-debug-ledger-detail");
  detail.id = "hsm-debug-ledger-detail";
  const classifications = snapshot.diagnoses.map((diagnosis) =>
    finalActionClassification(diagnosis, actionID),
  );
  const violationCount = classifications.filter(
    (classification) => classification?.violation === true,
  ).length;
  const followCount = classifications.filter(
    (classification) => classification?.follow === true,
  ).length;
  const total = snapshot.diagnoses.length;
  let summary = `No Follow or Violation \u2014 ${total} diagnoses`;
  if (followCount === total && violationCount === 0) {
    summary = `Unanimous Follow \u2014 ${total} of ${total} diagnoses`;
  } else if (violationCount === total && followCount === 0) {
    summary = `Unanimous Violation \u2014 ${total} of ${total} diagnoses`;
  } else if (violationCount > 0) {
    summary = `Violation possible \u2014 ${violationCount} of ${total} diagnoses`;
  }
  const actionTime =
    snapshot.action_time_classification?.selected_action_id === actionID
      ? snapshot.action_time_classification
      : null;
  const mistaken = snapshot.mistaken_actions.find(
    (candidate) => candidate.action_id === actionID,
  );
  detail.append(
    textElement("h2", `Action ${actionID}`),
    labelledValue(
      "Used at action time",
      actionTime === null
        ? "Unavailable for this accepted action."
        : classificationText(
            actionTime.final_follow,
            actionTime.final_violation,
          ),
    ),
    labelledValue(
      snapshot.evidence_boundary > snapshot.target_boundary
        ? "Current hindsight interpretation"
        : "Current historical interpretation",
      summary,
    ),
  );
  if (mistaken !== undefined) {
    detail.append(
      labelledValue(
        "\u26a0 Mistaken Action",
        `universally proven accepted transition ${mistaken.transition_index}`
          + ` | actor ${mistaken.historical_actor + 1}`
          + ` | classifiers ${mistaken.violating_classifiers.join(", ")}`,
        undefined,
        "hsm-debug-mistaken-action",
      ),
    );
  }
  for (const [index, diagnosis] of snapshot.diagnoses.entries()) {
    const alternative = buildDiagnosisAlternative(diagnosis, index);
    const rows = diagnosis.classifications.filter(
      (classification) => classification.action_id === actionID,
    );
    alternative.append(buildClassifications(rows));
    const evidence = diagnosis.semantic_values.filter(
      (value) => value.action_id === actionID,
    );
    if (evidence.length > 0) {
      const explanation = buildSemanticValues(evidence);
      explanation.querySelector("h3")!.textContent =
        "HSM Semantic Evidence (explanation)";
      alternative.append(explanation);
    }
    alternative.append(
      buildCardBeliefs(diagnosis.card_beliefs),
      buildPlayConnections(diagnosis.play_connections),
      buildConnectionObligations(diagnosis.connection_obligations),
    );
    detail.append(alternative);
  }
  return detail;
}

function classificationText(follow: boolean, violation: boolean): string {
  if (follow && !violation) {
    return "\u25cf Follow";
  }
  if (violation && !follow) {
    return "\u25b2 Violation";
  }
  return "\u25c7 Neither Follow nor Violation";
}

function buildCardLedgerDetail(
  snapshot: HSMSnapshot,
  cardID: number,
): HTMLElement {
  const detail = element("section", "hsm-debug-ledger-detail");
  detail.id = "hsm-debug-ledger-detail";
  detail.append(textElement("h2", `Stable Card ${cardID}`));
  for (const [index, diagnosis] of snapshot.diagnoses.entries()) {
    const alternative = buildDiagnosisAlternative(diagnosis, index);
    const { belief, connections, obligations } = cardDiagnostics(
      diagnosis,
      cardID,
    );
    alternative.append(
      textElement(
        "p",
        belief === undefined
          ? "No card belief in this diagnosis."
          : cardBeliefText(belief),
      ),
      buildPlayConnections(connections),
      buildConnectionObligations(obligations),
      buildClassifications(diagnosis.classifications),
    );
    detail.append(alternative);
  }
  return detail;
}

function buildDiagnosisAlternative(
  diagnosis: HSMDiagnosis,
  index: number,
): HTMLElement {
  const alternative = element("article", "hsm-debug-diagnosis-alternative");
  alternative.dataset["hsmDiagnosisLabel"] = diagnosis.label;
  alternative.append(
    textElement("h3", `D${index + 1} \u2014 ${diagnosis.label}`),
  );
  return alternative;
}

function finalActionClassification(
  diagnosis: HSMDiagnosis,
  actionID: number,
): HSMClassification | undefined {
  return (
    diagnosis.classifications.find(
      (classification) =>
        classification.action_id === actionID
        && classification.classifier === "hierarchy-resolved",
    )
    ?? diagnosis.classifications.find(
      (classification) => classification.action_id === actionID,
    )
  );
}

function diagnosticCardIDs(snapshot: HSMSnapshot): readonly number[] {
  const cardIDs = new Set<number>();
  for (const diagnosis of snapshot.diagnoses) {
    for (const belief of diagnosis.card_beliefs) {
      cardIDs.add(belief.stable_card_id);
    }
    for (const connection of diagnosis.play_connections) {
      cardIDs.add(connection.focus_card_id);
      for (const card of connection.prerequisites) {
        cardIDs.add(card.stable_card_id);
      }
    }
    for (const obligation of diagnosis.connection_obligations) {
      cardIDs.add(obligation.focus_card_id);
      for (const card of obligation.candidates) {
        cardIDs.add(card.stable_card_id);
      }
    }
  }
  return [...cardIDs].toSorted((left, right) => left - right);
}

export function getHSMCardTooltipHTML(stableCardID: number): string {
  const snapshot = state?.snapshot;
  if (
    snapshot === null
    || snapshot === undefined
    || state?.loading === true
    || state?.failure !== null
  ) {
    return "";
  }
  const clauses: string[] = [];
  for (const [index, diagnosis] of snapshot.diagnoses.entries()) {
    const parts: string[] = [];
    const { belief, connections, obligations } = cardDiagnostics(
      diagnosis,
      stableCardID,
    );
    if (belief !== undefined) {
      parts.push(cardBeliefText(belief));
    }
    for (const connection of connections) {
      const prerequisites = connection.prerequisites
        .map((card) => `#${card.stable_card_id} mask ${card.identity_mask}`)
        .join(" \u2192 ");
      parts.push(
        `Play Connection, ordered prerequisites: ${
          prerequisites === "" ? "none" : prerequisites
        }`,
      );
    }
    for (const obligation of obligations) {
      const candidates = obligation.candidates
        .map((card, candidateIndex) => {
          const current =
            candidateIndex === obligation.current_candidate_index
              ? " (current)"
              : "";
          return `#${card.stable_card_id} mask ${card.identity_mask}${current}`;
        })
        .join(" \u2192 ");
      parts.push(
        `${obligation.kind} obligation, ordered candidates: ${candidates}`,
      );
    }
    if (parts.length > 0) {
      clauses.push(
        `<span class="hsm-card-tooltip-clause"><strong>D${index + 1}:</strong> <code>${escapeHTML(
          diagnosis.label,
        )}</code> ${escapeHTML(parts.join("; "))}.</span>`,
      );
    }
  }
  if (clauses.length === 0) {
    return "";
  }
  return `${clauses.join(" ")} <button type="button" data-hsm-full-details="${
    stableCardID
  }">Full details</button>`;
}

export function hasHSMCardTooltip(stableCardID: number): boolean {
  return getHSMCardTooltipHTML(stableCardID) !== "";
}

function handleFullDetailsClick(event: MouseEvent) {
  const { target } = event;
  if (!(target instanceof Element)) {
    return;
  }
  const detailsButton = target.closest<HTMLElement>("[data-hsm-full-details]");
  if (detailsButton === null || state === null) {
    return;
  }
  const cardID = Number(detailsButton.dataset["hsmFullDetails"]);
  if (!Number.isInteger(cardID)) {
    return;
  }
  state.drawerOpen = true;
  state.selectedLedger = "card";
  state.selectedCardID = cardID;
  savePreferences();
  render();
  globalThis.dispatchEvent(new Event("resize"));
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function cardDiagnostics(diagnosis: HSMDiagnosis, stableCardID: number) {
  return {
    belief: diagnosis.card_beliefs.find(
      (candidate) => candidate.stable_card_id === stableCardID,
    ),
    connections: diagnosis.play_connections.filter(
      (connection) =>
        connection.focus_card_id === stableCardID
        || connection.prerequisites.some(
          (card) => card.stable_card_id === stableCardID,
        ),
    ),
    obligations: diagnosis.connection_obligations.filter(
      (obligation) =>
        obligation.focus_card_id === stableCardID
        || obligation.candidates.some(
          (card) => card.stable_card_id === stableCardID,
        ),
    ),
  };
}

function cardBeliefText(belief: HSMDiagnosis["card_beliefs"][number]): string {
  const reasons =
    belief.reason_identity_masks.length === 0
      ? "no labelled reason"
      : belief.reason_identity_masks
          .map((reason) => `${reason.reason}: mask ${reason.identity_mask}`)
          .join(" | ");
  return `Candidate identity mask: ${belief.candidate_identity_mask} | ${reasons}`;
}

function buildCardBeliefs(beliefs: HSMDiagnosis["card_beliefs"]): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Card beliefs"));
  if (beliefs.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ul");
  for (const belief of beliefs) {
    list.append(
      textElement(
        "li",
        `Stable Card ${belief.stable_card_id} | ${cardBeliefText(belief)}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function buildPlayConnections(
  connections: readonly HSMPlayConnection[],
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Play Connections"));
  if (connections.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ol");
  for (const connection of connections) {
    const prerequisites = connection.prerequisites
      .map((card) => `#${card.stable_card_id} mask ${card.identity_mask}`)
      .join(" \u2192 ");
    list.append(
      textElement(
        "li",
        `Focus #${connection.focus_card_id} mask ${connection.focus_identity_mask}`
          + ` | ordered prerequisites: ${
            prerequisites === "" ? "none" : prerequisites
          }`
          + ` | transition ${connection.source_transition}`
          + ` | available boundary ${connection.available_from_boundary}`
          + ` | provenance ${connection.provenance_id}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function buildConnectionObligations(
  obligations: readonly HSMConnectionObligation[],
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Connection Obligations"));
  if (obligations.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ul");
  for (const obligation of obligations) {
    const candidates = obligation.candidates
      .map((card, index) => {
        const current =
          index === obligation.current_candidate_index ? " (current)" : "";
        return `#${card.stable_card_id} mask ${card.identity_mask}${current}`;
      })
      .join(" \u2192 ");
    list.append(
      textElement(
        "li",
        `${obligation.kind} | owner player ${obligation.owner_player + 1}`
          + ` | focus #${obligation.focus_card_id}`
          + ` mask ${obligation.focus_identity_mask}`
          + ` | ordered candidates: ${candidates}`
          + ` | transition ${obligation.source_transition}`
          + ` | available boundary ${obligation.available_from_boundary}`
          + ` | provenance ${obligation.provenance_id}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function buildClassifications(
  classifications: readonly HSMClassification[],
  title = "Action classifications",
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", title));
  if (classifications.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const actionsRow = element("div", "hsm-debug-actions");
  for (const classification of classifications) {
    const actionID = classification.action_id;
    const actionButton = button(
      `Action ${actionID} | ${classification.classifier}`,
      "",
      () => {
        selectAction(actionID);
      },
    );
    actionButton.removeAttribute("id");
    actionButton.className = "hsm-debug-action";
    if (classification.follow) {
      actionButton.classList.add("hsm-action-follow");
    }
    if (classification.violation) {
      actionButton.classList.add("hsm-action-violation");
    }
    actionButton.setAttribute(
      "aria-pressed",
      String(state?.selectedActionID === actionID),
    );
    actionsRow.append(actionButton);
  }
  section.append(actionsRow);
  const selected = classifications.find(
    (classification) => classification.action_id === state?.selectedActionID,
  );
  if (selected !== undefined) {
    const details = element("section", "hsm-debug-panel");
    details.id = "hsm-debug-action-details";
    details.append(
      textElement("h3", `Action ${selected.action_id}`),
      textElement("p", `Classifier: ${selected.classifier}`),
      textElement(
        "p",
        `Follow: ${selected.follow ? "yes" : "no"} | Violation: ${selected.violation ? "yes" : "no"}`,
      ),
    );
    section.append(details);
  }
  return section;
}

function buildSemanticValues(values: readonly HSMSemanticValue[]): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Semantic values"));
  if (values.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ul");
  for (const value of values) {
    list.append(
      textElement(
        "li",
        `Action ${value.action_id} | ${value.category}: ${value.name}`
          + ` | active: ${value.active ? "yes" : "no"}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function selectCard(stableCardID: number) {
  if (state === null) {
    return;
  }
  state.selectedLedger = "card";
  state.selectedCardID = stableCardID;
  render();
}

function selectAction(actionID: number) {
  if (state === null) {
    return;
  }
  state.selectedLedger = "action";
  state.selectedActionID = actionID;
  render();
}

function selectLedger(ledger: InspectorState["selectedLedger"]) {
  if (state === null) {
    return;
  }
  state.selectedLedger = ledger;
  render();
}

function buildRestoreHandle(): HTMLButtonElement {
  const restore = button("HSM", "hsm-debug-restore", toggleDrawer);
  restore.hidden = state?.drawerOpen === true;
  restore.title = "Restore HSM Inspector (I)";
  return restore;
}

function toggleDrawer() {
  if (state === null) {
    return;
  }
  state.drawerOpen = !state.drawerOpen;
  savePreferences();
  render();
  globalThis.dispatchEvent(new Event("resize"));
}

function renderAndRequest() {
  if (state === null) {
    return;
  }
  state.snapshot = null;
  state.loading = true;
  state.failure = null;
  state.typedFailure = null;
  state.pendingPhysicalTruth = null;
  requestTimeouts.clearPhysicalTruth();
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = null;
  render();
  requestSnapshot();
  requestPhysicalTruth();
}

function element<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  className: string,
): HTMLElementTagNameMap[K] {
  const value = document.createElement(tag);
  value.className = className;
  return value;
}

function textElement<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  text: string,
  id?: string,
): HTMLElementTagNameMap[K] {
  const value = document.createElement(tag);
  value.textContent = text;
  if (id !== undefined) {
    value.id = id;
  }
  return value;
}

function labelledControl(label: string, control: HTMLElement): HTMLElement {
  const wrapper = element("label", "hsm-debug-labelled-control");
  wrapper.append(textElement("span", label), control);
  return wrapper;
}

function labelledValue(
  label: string,
  value: string,
  id?: string,
  className = "hsm-debug-panel",
): HTMLElement {
  const wrapper = element("section", className);
  if (id !== undefined) {
    wrapper.id = id;
  }
  wrapper.append(textElement("h3", label), textElement("p", value));
  return wrapper;
}

function button(
  label: string,
  id: string,
  onClick: () => void,
): HTMLButtonElement {
  const value = document.createElement("button");
  value.type = "button";
  value.id = id;
  value.textContent = label;
  value.addEventListener("click", onClick);
  return value;
}

function preferenceKey(debug: HSMDebugInit | null): string | null {
  return debug === null
    ? null
    : `hanabi-live:hsm-inspector:${debug.tableID}:${debug.identity}`;
}

function loadPreferences(key: string | null): InspectorPreferences | null {
  if (key === null) {
    return null;
  }
  try {
    const raw = globalThis.sessionStorage.getItem(key);
    if (raw === null) {
      return null;
    }
    const value = JSON.parse(raw) as Partial<InspectorPreferences>;
    if (typeof value.drawerOpen !== "boolean") {
      return null;
    }
    return {
      drawerOpen: value.drawerOpen,
    };
  } catch {
    return null;
  }
}

function savePreferences() {
  const key = preferenceKey(config);
  if (key === null || state === null) {
    return;
  }
  try {
    globalThis.sessionStorage.setItem(
      key,
      JSON.stringify({
        drawerOpen: state.drawerOpen,
      } satisfies InspectorPreferences),
    );
  } catch {
    // Presentation preferences must never block diagnostics.
  }
}

function removePreferences(key: string) {
  try {
    globalThis.sessionStorage.removeItem(key);
  } catch {
    // Storage may be unavailable in privacy-restricted browser contexts.
  }
}

document.addEventListener("keydown", (event) => {
  if (event.key.toLowerCase() === "i" && root !== null) {
    toggleDrawer();
  }
});
