type HSMDebugCapability = "own_perspective" | "switchable";

export interface HSMDebugInit {
  readonly tableID: number;
  readonly capability: HSMDebugCapability;
  readonly identity: string;
  readonly ownPerspective: number;
  readonly playerNames: readonly string[];
  readonly physicalTruthAllowed: boolean;
}

interface HSMSnapshot {
  readonly targetBoundary: number;
  readonly evidenceBoundary: number;
  readonly perspectivePlayer?: number;
  readonly actionTimeClassification: string | null;
  readonly diagnosticInterpretation: string;
  readonly cards: readonly Record<string, unknown>[];
  readonly legalActions: readonly Record<string, unknown>[];
  readonly ruleRows: readonly Record<string, unknown>[];
  readonly relaxedSources: readonly string[];
  readonly solver: Record<string, unknown>;
  readonly capacity: Record<string, unknown>;
  readonly plainText: string;
  readonly physicalTruth?: Record<string, unknown>;
}

export interface HSMSnapshotMessage {
  readonly requestID: number;
  readonly snapshot: HSMSnapshot;
}

export interface HSMSnapshotFailureMessage {
  readonly requestID: number;
  readonly targetBoundary: number;
  readonly evidenceBoundary: number;
  readonly perspectivePlayer: number;
  readonly error: string;
  readonly failure?: Record<string, unknown>;
}

type SendCommand = (command: string, data: Record<string, unknown>) => void;

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
  physicalTruth: boolean;
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
let sendCommand: SendCommand | null = null;
let root: HTMLElement | null = null;
let state: InspectorState | null = null;

export function initHSMInspector(
  debug: HSMDebugInit | null,
  send: SendCommand,
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
    physicalTruth: false,
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

function teardownHSMInspector(): void {
  root?.remove();
  root = null;
  config = null;
  sendCommand = null;
  state = null;
  document.body.classList.remove("hsm-inspector-open");
  document.body.classList.remove("hsm-inspection-read-only");
  document.body.classList.remove("hsm-debug-authorized");
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
    config.identity === "hsm_debug_spectator"
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
  if (state.historical) {
    state.evidenceBoundary = state.targetBoundary;
  } else {
    state.evidenceBoundary = Math.max(
      state.targetBoundary,
      state.evidenceBoundary,
    );
  }
  state.snapshot = null;
  state.selectedCardID = null;
  state.selectedActionID = null;
  state.loading = true;
  state.failure = null;
  render();
  requestSnapshot();
}

export function handleHSMSnapshot(message: HSMSnapshotMessage): void {
  if (state === null) {
    return;
  }
  const { snapshot } = message;
  if (
    snapshot.targetBoundary !== state.targetBoundary
    || snapshot.evidenceBoundary !== state.evidenceBoundary
    || (snapshot.perspectivePlayer !== undefined
      && snapshot.perspectivePlayer !== state.perspectivePlayer)
    || (snapshot.physicalTruth !== undefined) !== state.physicalTruth
  ) {
    return;
  }
  state.snapshot = snapshot;
  state.selectedActionID = null;
  state.loading = false;
  state.failure = null;
  render();
}

export function handleHSMSnapshotFailure(
  message: HSMSnapshotFailureMessage,
): void {
  if (
    state === null
    || message.targetBoundary !== state.targetBoundary
    || message.evidenceBoundary !== state.evidenceBoundary
    || message.perspectivePlayer !== state.perspectivePlayer
  ) {
    return;
  }
  state.snapshot = null;
  state.loading = false;
  state.failure = message.error;
  render();
}

function requestSnapshot(): void {
  if (state === null || sendCommand === null || config === null) {
    return;
  }
  state.loading = true;
  state.snapshot = null;
  state.failure = null;
  sendCommand("researchHSMRequest", {
    tableID: config.tableID,
    targetBoundary: state.targetBoundary,
    evidenceBoundary: state.evidenceBoundary,
    perspectivePlayer: state.perspectivePlayer,
    actorPlayer: state.actorPlayer,
    physicalTruth: state.physicalTruth,
  });
}

function render(): void {
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
  return config.identity === "hsm_debug_spectator"
    ? "HSM Debug Spectator · read-only"
    : `${config.capability.replace("_", " ")} · read-only inspection`;
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
        `Own perspective · ${config.playerNames[config.ownPerspective]}`,
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
  const allowed =
    config?.physicalTruthAllowed === true
    && (config.identity === "hsm_debug_spectator"
      || state?.perspectivePlayer !== config.ownPerspective
      || state?.gameFinished === true);
  input.disabled = !allowed;
  input.addEventListener("change", () => {
    if (state === null) {
      return;
    }
    state.physicalTruth = input.checked && allowed;
    renderAndRequest();
  });
  label.append(input, document.createTextNode(" Physical Truth"));
  label.title = allowed
    ? "Privileged physical identities; graphics only"
    : "Unavailable for own-perspective unfinished live play";
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
    const failure = textElement(
      "div",
      state.failure,
      "hsm-debug-failure",
    );
    failure.setAttribute("role", "alert");
    drawer.append(failure);
  } else if (state?.loading === true || snapshot === null) {
    const loading = textElement(
      "div",
      "Computing Diagnostic Snapshot…",
      "hsm-debug-loading",
    );
    loading.setAttribute("role", "status");
    drawer.append(loading);
  } else {
    drawer.append(buildSnapshot(snapshot));
  }
  return drawer;
}

function buildTimeline(): HTMLElement {
  const wrapper = element("section", "hsm-debug-timeline");
  wrapper.id = "hsm-debug-timeline";
  wrapper.append(textElement("strong", `Target ${state?.targetBoundary ?? 0}`));
  if (state?.historical === true) {
    wrapper.append(textElement("span", "Historical · evidence equals target"));
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
        `Hindsight interval ${state?.targetBoundary ?? 0} → ${state?.evidenceBoundary ?? 0}`,
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
      ? `${selectedName}'s actual decision boundary · diagnostic indicators do not change legal actions.`
      : `This is not ${selectedName}'s decision boundary. Inspection is read-only; hypothetical action classification is unavailable.`;
  return notice;
}

function buildSnapshot(snapshot: HSMSnapshot): HTMLElement {
  const content = element("div", "hsm-debug-content");
  content.append(
    labelledValue(
      "Immutable recorded action-time classification",
      snapshot.actionTimeClassification ?? "Not recorded",
      "hsm-debug-recorded",
    ),
    labelledValue(
      "Current diagnostic interpretation",
      snapshot.diagnosticInterpretation,
      "hsm-debug-current",
    ),
    buildCards(snapshot.cards),
    buildLegalActions(snapshot.legalActions),
    buildCollection("Rule rows", snapshot.ruleRows, "rule"),
    buildObjectDetails("Solver statistics", snapshot.solver),
    buildObjectDetails("Capacity diagnostics", snapshot.capacity),
  );
  if (snapshot.relaxedSources.length > 0) {
    content.append(
      labelledValue("Relaxed sources", snapshot.relaxedSources.join(", ")),
    );
  }
  if (snapshot.physicalTruth !== undefined) {
    content.append(
      labelledValue(
        "PHYSICAL TRUTH · graphics only",
        JSON.stringify(snapshot.physicalTruth),
        undefined,
        "hsm-debug-physical-truth-panel",
      ),
    );
  }
  const plain = document.createElement("pre");
  plain.id = "hsm-debug-plain-text";
  plain.textContent = snapshot.plainText;
  content.append(labelledControl("Plain-text diagnostic output", plain));
  return content;
}

function buildCards(cards: readonly Record<string, unknown>[]): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Observer-relative cards"));
  if (cards.length === 0) {
    section.append(textElement("p", "No valid cards at this boundary"));
    return section;
  }
  if (state?.cardLabels === "off") {
    section.append(textElement("p", "Card labels are hidden"));
    return section;
  }
  const badges = element("div", "hsm-debug-card-badges");
  for (const card of cards) {
    const stableCardID = Number(card["stableCardID"]);
    const summary = String(card["summary"] ?? "No HSM note");
    if (state?.cardLabels === "summary") {
      const cardSummary = textElement("span", `#${stableCardID} · ${summary}`);
      cardSummary.className = "hsm-debug-card-summary";
      cardSummary.title = Array.isArray(card["notes"])
        ? (card["notes"] as unknown[]).join(" · ")
        : summary;
      badges.append(cardSummary);
      continue;
    }
    const badge = button(`#${stableCardID} · ${summary}`, "", () => {
      if (state === null) {
        return;
      }
      state.selectedCardID = stableCardID;
      render();
    });
    badge.removeAttribute("id");
    badge.className = "hsm-debug-card-badge";
    badge.title = Array.isArray(card["notes"])
      ? (card["notes"] as unknown[]).join(" · ")
      : summary;
    badge.setAttribute(
      "aria-pressed",
      String(state?.selectedCardID === stableCardID),
    );
    badges.append(badge);
  }
  section.append(badges);
  const selected = cards.find(
    (card) => Number(card["stableCardID"]) === state?.selectedCardID,
  );
  if (selected !== undefined) {
    const details = element("section", "hsm-debug-panel");
    details.append(
      textElement(
        "h3",
        `Selected Stable Card ${String(selected["stableCardID"])}`,
      ),
    );
    for (const key of ["notes", "connections", "obligations"] as const) {
      const values = selected[key];
      if (Array.isArray(values) && values.length > 0) {
        details.append(
          textElement("p", `${fieldLabel(key)}: ${values.join(" · ")}`),
        );
      }
    }
    section.append(details);
  }
  return section;
}

function buildLegalActions(
  actions: readonly Record<string, unknown>[],
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", "Legal action diagnostics"));
  if (actions.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const actionsRow = element("div", "hsm-debug-actions");
  for (const action of actions) {
    const actionID = Number(action["actionID"]);
    const classification = String(action["classification"] ?? "neutral");
    const actionButton = button(String(action["label"] ?? actionID), "", () => {
      if (state === null) {
        return;
      }
      state.selectedActionID = actionID;
      render();
    });
    actionButton.removeAttribute("id");
    actionButton.className = `hsm-debug-action hsm-action-${classification}`;
    actionButton.setAttribute(
      "aria-pressed",
      String(state?.selectedActionID === actionID),
    );
    actionsRow.append(actionButton);
  }
  section.append(actionsRow);
  const selected = actions.find(
    (action) => Number(action["actionID"]) === state?.selectedActionID,
  );
  if (selected !== undefined) {
    const details = buildCollection("Selected action", [selected], "label");
    details.id = "hsm-debug-action-details";
    section.append(details);
  }
  return section;
}

function buildCollection(
  title: string,
  values: readonly Record<string, unknown>[],
  primaryKey: string,
): HTMLElement {
  const section = element("section", "hsm-debug-panel");
  section.append(textElement("h3", title));
  if (values.length === 0) {
    section.append(textElement("p", "None at this boundary"));
    return section;
  }
  const list = document.createElement("ul");
  for (const value of values) {
    const item = document.createElement("li");
    const primary = value[primaryKey] ?? value["classification"] ?? "Detail";
    if (typeof value["classification"] === "string") {
      item.classList.add(`hsm-action-${value["classification"]}`);
    }
    item.append(textElement("strong", String(primary)));
    const details = Object.entries(value)
      .filter(
        ([key]) =>
          key !== primaryKey
          && key !== "label"
          && key !== "classification"
          && key !== "rule",
      )
      .map(([key, entry]) => `${fieldLabel(key)}: ${formatField(entry)}`);
    if (typeof value["classification"] === "string") {
      details.unshift(fieldLabel(String(value["classification"])));
    }
    if (details.length > 0) {
      item.append(document.createTextNode(` · ${details.join(" · ")}`));
    }
    list.append(item);
  }
  section.append(list);
  return section;
}

function formatField(value: unknown): string {
  return Array.isArray(value) ? value.join(", ") : String(value);
}

function fieldLabel(value: string): string {
  const spaced = value
    .replaceAll(/([a-z])([A-Z])/g, "$1 $2")
    .replaceAll("_", " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function buildObjectDetails(
  title: string,
  value: Record<string, unknown>,
): HTMLElement {
  return labelledValue(
    title,
    Object.entries(value)
      .map(([key, entry]) => `${key}: ${String(entry)}`)
      .join(" · "),
  );
}

function buildRestoreHandle(): HTMLButtonElement {
  const restore = button("HSM", "hsm-debug-restore", toggleDrawer);
  restore.hidden = state?.drawerOpen === true;
  restore.title = "Restore HSM Inspector (I)";
  return restore;
}

function toggleDrawer(): void {
  if (state === null) {
    return;
  }
  state.drawerOpen = !state.drawerOpen;
  savePreferences();
  render();
  window.dispatchEvent(new Event("resize"));
}

function renderAndRequest(): void {
  if (state === null) {
    return;
  }
  state.snapshot = null;
  state.loading = true;
  state.failure = null;
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
    const raw = window.sessionStorage.getItem(key);
    if (raw === null) {
      return null;
    }
    const value = JSON.parse(raw) as Partial<InspectorPreferences>;
    if (
      typeof value.drawerOpen !== "boolean"
      || !["badges", "summary", "off"].includes(String(value.cardLabels))
    ) {
      return null;
    }
    return {
      drawerOpen: value.drawerOpen,
      cardLabels: value.cardLabels as InspectorState["cardLabels"],
    };
  } catch {
    return null;
  }
}

function savePreferences(): void {
  const key = preferenceKey(config);
  if (key === null || state === null) {
    return;
  }
  try {
    window.sessionStorage.setItem(
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

function removePreferences(key: string): void {
  try {
    window.sessionStorage.removeItem(key);
  } catch {
    // Storage may be unavailable in privacy-restricted browser contexts.
  }
}

document.addEventListener("keydown", (event) => {
  if (event.key.toLowerCase() === "i" && root !== null) {
    toggleDrawer();
  }
});
