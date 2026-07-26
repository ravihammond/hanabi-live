import {
  HSM_PROTOCOL_VERSION,
  type HSMCardBelief,
  type HSMClassification,
  type HSMConnectionObligation,
  type HSMConventionApplication,
  type HSMDebugInit,
  type HSMDiagnosis,
  type HSMDiagnosticProjection,
  type HSMMistakenAction,
  type HSMPhysicalTruthFailureMessage,
  type HSMPhysicalTruthMessage,
  type HSMPhysicalTruthOverlay,
  type HSMPhysicalTruthPendingMessage,
  type HSMPhysicalTruthRejectedMessage,
  type HSMPlayConnection,
  type HSMSemanticValue,
  type HSMSnapshot,
  type HSMSnapshotFailureMessage,
  type HSMSnapshotMessage,
  type HSMSnapshotPendingMessage,
  type HSMSnapshotRejectedMessage,
  type HSMSnapshotUnavailableMessage,
  type SendHSMCommand,
} from "./hsmInspectorContract";
import {
  bindPendingPhysicalTruth,
  bindPendingSnapshot,
  HSMRequestTimeouts,
  matchesPendingPhysicalTruthRejection,
  matchesPendingPhysicalTruthResponse,
  matchesPendingSnapshotRejection,
  matchesPendingSnapshotResponse,
  type PendingHSMPhysicalTruth,
  type PendingHSMResponse,
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
  historical: boolean;
  drawerOpen: boolean;
  nextClientRequestID: number;
  pendingSnapshot: PendingHSMResponse | null;
  physicalTruth: boolean;
  nextPhysicalTruthClientRequestID: number;
  pendingPhysicalTruth: PendingHSMPhysicalTruth | null;
  physicalTruthOverlay: HSMPhysicalTruthOverlay | null;
  physicalTruthFailure: string | null;
  cardLabels: "badges" | "summary" | "off";
  selectedCardID: number | null;
  selectedActionID: number | null;
  snapshot: HSMSnapshot | null;
  loading: boolean;
  failure: string | null;
}

interface InspectorPreferences {
  readonly drawerOpen: boolean;
  readonly cardLabels: InspectorState["cardLabels"];
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
    historical: true,
    drawerOpen: preferences?.drawerOpen ?? true,
    nextClientRequestID: 0,
    pendingSnapshot: null,
    physicalTruth: false,
    nextPhysicalTruthClientRequestID: 0,
    pendingPhysicalTruth: null,
    physicalTruthOverlay: null,
    physicalTruthFailure: null,
    cardLabels: preferences?.cardLabels ?? "badges",
    selectedCardID: null,
    selectedActionID: null,
    snapshot: null,
    loading: true,
    failure: null,
  };
  root = document.createElement("div");
  root.id = "hsm-debug-root";
  document.body.append(root);
  document.body.classList.add("hsm-debug-authorized");
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
  return window.innerWidth <= 760 ? 140 : 43;
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
  const nextTarget = Math.max(0, Math.min(targetBoundary, maxBoundary));
  const nextMax = Math.max(0, maxBoundary);
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
  if (state.followActor) {
    state.perspectivePlayer = actorPlayer;
  }
  state.evidenceBoundary = state.historical
    ? state.targetBoundary
    : Math.max(state.targetBoundary, state.evidenceBoundary);
  state.snapshot = null;
  state.pendingPhysicalTruth = null;
  requestTimeouts.clearPhysicalTruth();
  state.physicalTruthOverlay = null;
  state.physicalTruthFailure = null;
  state.selectedCardID = null;
  state.selectedActionID = null;
  state.loading = true;
  state.failure = null;
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
  state.nextClientRequestID++;
  state.pendingSnapshot = {
    serverRequestID: null,
    clientRequestID: state.nextClientRequestID,
    archiveGenerationID: config.archiveGenerationID,
    targetBoundary: state.targetBoundary,
    evidenceBoundary: state.evidenceBoundary,
    perspectivePlayer: state.perspectivePlayer,
    actorPlayer: state.actorPlayer,
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
}

function buildToolbar(): HTMLElement {
  const toolbar = element("header", "hsm-debug-toolbar");
  toolbar.id = "hsm-debug-toolbar";
  toolbar.append(
    textElement("strong", "HSM Debug"),
    textElement("span", capabilityLabel()),
    labelledControl("Perspective", buildPerspectiveSelect()),
    labelledControl("Card labels", buildCardLabelSelect()),
    buildTruthControl(),
    buildModeControl(),
  );
  return toolbar;
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

function buildCardLabelSelect(): HTMLSelectElement {
  const select = document.createElement("select");
  select.id = "hsm-debug-card-labels";
  select.add(new Option("Badges", "badges"));
  select.add(new Option("Hover summaries", "summary"));
  select.add(new Option("Off", "off"));
  select.value = state?.cardLabels ?? "badges";
  select.addEventListener("change", () => {
    if (state === null) {
      return;
    }
    state.cardLabels = select.value as InspectorState["cardLabels"];
    savePreferences();
    render();
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
    const failure = textElement("div", state.failure, "hsm-debug-failure");
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
  wrapper.append(textElement("strong", `Target ${state?.targetBoundary ?? 0}`));
  if (state?.historical === true) {
    wrapper.append(textElement("span", "Historical | evidence equals target"));
  } else {
    const input = document.createElement("input");
    input.id = "hsm-debug-evidence";
    input.type = "range";
    input.min = String(state?.targetBoundary ?? 0);
    input.max = String(state?.maxBoundary ?? 0);
    input.value = String(state?.evidenceBoundary ?? 0);
    input.addEventListener("change", () => {
      if (state === null) {
        return;
      }
      state.evidenceBoundary = Math.max(
        state.targetBoundary,
        Number(input.value),
      );
      renderAndRequest();
    });
    wrapper.append(
      textElement("span", `Evidence ${state?.evidenceBoundary ?? 0}`),
      input,
      textElement(
        "span",
        `Hindsight interval ${state?.targetBoundary ?? 0} -> ${state?.evidenceBoundary ?? 0}`,
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
    buildClassifications(
      snapshot.aggregate_action_classifications,
      "Aggregate action classifications",
    ),
    buildMistakenActions(snapshot.mistaken_actions),
    buildProjection("Consensus projection", snapshot.consensus),
    buildDiagnoses(snapshot.diagnoses),
  );
  return content;
}

function buildProjection(
  title: string,
  projection: HSMDiagnosticProjection,
): HTMLElement {
  const section = element("section", "hsm-debug-projection");
  section.append(
    textElement("h2", title),
    buildApplications(projection.applications),
    buildCardBeliefs(projection.card_beliefs),
    buildPlayConnections(projection.play_connections),
    buildConnectionObligations(projection.connection_obligations),
    buildClassifications(projection.classifications),
    buildSemanticValues(projection.semantic_values),
  );
  return section;
}

function buildDiagnoses(diagnoses: readonly HSMDiagnosis[]): HTMLElement {
  const section = element("section", "hsm-debug-diagnoses");
  section.append(textElement("h2", `HSM Diagnoses (${diagnoses.length})`));
  for (const diagnosis of diagnoses) {
    section.append(buildProjection(diagnosis.label, diagnosis));
  }
  return section;
}

function buildMistakenActions(
  mistakenActions: readonly HSMMistakenAction[],
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Universal Mistaken Actions"));
  if (mistakenActions.length === 0) {
    section.append(textElement("p", "None"));
    return section;
  }
  const list = document.createElement("ul");
  for (const mistaken of mistakenActions) {
    list.append(
      textElement(
        "li",
        `Transition ${mistaken.transition_index}`
          + ` | actor ${mistaken.historical_actor + 1}`
          + ` | action ${mistaken.action_id}`
          + ` | classifiers: ${mistaken.violating_classifiers.join(", ")}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function buildApplications(
  applications: readonly HSMConventionApplication[],
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Convention applications"));
  if (applications.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ul");
  for (const application of applications) {
    list.append(
      textElement(
        "li",
        `${application.rule} → ${application.meaning}`
          + ` | transition ${application.source_transition}`
          + ` | actor ${application.historical_actor + 1}`
          + ` | observer ${application.outer_observer + 1}`
          + ` | ${application.subject_kind} #${application.subject_id}`
          + ` | provenance ${application.provenance_id}`
          + ` | applicable: ${application.applicable ? "yes" : "no"}`,
      ),
    );
  }
  section.append(list);
  return section;
}

function buildCardBeliefs(beliefs: readonly HSMCardBelief[]): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Observer-relative card beliefs"));
  if (beliefs.length === 0) {
    section.append(textElement("p", "No card beliefs at this boundary"));
    return section;
  }
  if (state?.cardLabels === "off") {
    section.append(textElement("p", "Card labels are hidden"));
    return section;
  }
  const badges = element("div", "hsm-debug-card-badges");
  for (const belief of beliefs) {
    const stableCardID = belief.stable_card_id;
    const summary = `candidate mask ${belief.candidate_identity_mask}`;
    const reasons = belief.reason_identity_masks
      .map((reason) => `${reason.reason}: mask ${reason.identity_mask}`)
      .join(" | ");
    if (state?.cardLabels === "summary") {
      const cardSummary = textElement("span", `#${stableCardID} | ${summary}`);
      cardSummary.className = "hsm-debug-card-summary";
      cardSummary.title = reasons;
      badges.append(cardSummary);
      continue;
    }
    const badge = button(`#${stableCardID} | ${summary}`, "", () => {
      selectCard(stableCardID);
    });
    badge.removeAttribute("id");
    badge.className = "hsm-debug-card-badge";
    badge.title = reasons;
    badge.setAttribute(
      "aria-pressed",
      String(state?.selectedCardID === stableCardID),
    );
    badges.append(badge);
  }
  section.append(badges);
  const selected = beliefs.find(
    (belief) => belief.stable_card_id === state?.selectedCardID,
  );
  if (selected !== undefined) {
    const details = element("section", "hsm-debug-panel");
    details.append(
      textElement("h3", `Selected Stable Card ${selected.stable_card_id}`),
      textElement(
        "p",
        `Candidate identity mask: ${selected.candidate_identity_mask}`,
      ),
      textElement(
        "p",
        `Reasons: ${selected.reason_identity_masks
          .map((reason) => `${reason.reason}: mask ${reason.identity_mask}`)
          .join(" | ")}`,
      ),
    );
    section.append(details);
  }
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
      .join(" → ");
    list.append(
      textElement(
        "li",
        `Focus #${connection.focus_card_id} mask ${connection.focus_identity_mask}`
          + ` | ordered prerequisites: ${prerequisites || "none"}`
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
      .join(" → ");
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
  state.selectedCardID = stableCardID;
  render();
}

function selectAction(actionID: number) {
  if (state === null) {
    return;
  }
  state.selectedActionID = actionID;
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
  if (!state.physicalTruth) {
    state.pendingPhysicalTruth = null;
    requestTimeouts.clearPhysicalTruth();
    state.physicalTruthOverlay = null;
    state.physicalTruthFailure = null;
  }
  render();
  requestSnapshot();
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
    if (
      typeof value.drawerOpen !== "boolean"
      || !["badges", "off", "summary"].includes(String(value.cardLabels))
    ) {
      return null;
    }
    return {
      drawerOpen: value.drawerOpen,
      cardLabels: value.cardLabels!,
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
        cardLabels: state.cardLabels,
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
