// PROTOTYPE — selected hybrid, informed by the real Hanabi.live game UI.
const CARD_ROOT = "./assets/cards";
const NATIVE_ROOT = "./assets/native";

const boundaries = [
  [8, "Alice", "clues 2 to Bob"],
  [9, "Bob", "plays b1"],
  [10, "Diana", "clues Red to Cathy"],
  [11, "Alice", "plays g1"],
  [12, "Bob", "clues Red to Cathy"],
  [13, "Cathy", "discards y5"],
  [14, "Diana", "clues Blue to Bob"],
  [15, "Alice", "plays r2"],
  [16, "Bob", "clues 5 to Diana"],
  [17, "Cathy", "discards b5"],
  [18, "Diana", "plays w1"],
];

const cards = {
  1: ["Trash {b5}", "No supported convention meaning survives for this card.", "b5", "none"],
  2: ["Play {r4} · waiting", "Focused by Bob's Red clue. Every surviving Play explanation still requires red 3.", "r4, r5", "r3 → r4"],
  3: ["Save | Play", "Two observer-relative explanations remain possible at this boundary.", "g2, g4, g5", "ambiguous"],
  4: ["Good Touch", "The card is positively clued but has no current play obligation.", "y1, y2", "none"],
  5: ["Unknown", "No convention label is supported by the observer's grounded knowledge.", "b1, b2, b3, b4, b5", "none"],
};

const actions = {
  "play-1": ["Play card 1", "violate", "Save Principle", "Playing the chop conflicts with the protected interpretation."],
  "discard-1": ["Discard card 1", "follow", "Chop convention", "Discard is consistent with the current observer-relative model."],
  "play-2": ["Play card 2", "follow", "Focused clue", "The action follows the focused Play interpretation."],
  "discard-2": ["Discard card 2", "violate", "Focused clue", "Discarding violates the surviving Play interpretation."],
  "play-3": ["Play card 3", "uncertain", "Insufficient evidence", "Multiple meanings survive; exact classification is neutral."],
  "discard-3": ["Discard card 3", "uncertain", "Insufficient evidence", "The HSM cannot prove follow or violation."],
  "play-4": ["Play card 4", "violate", "Good Touch", "The card is not currently known playable."],
  "discard-4": ["Discard card 4", "follow", "Good Touch", "No active convention forbids this discard."],
  "play-5": ["Play card 5", "uncertain", "No active rule", "No exact classification is available."],
  "discard-5": ["Discard card 5", "follow", "Chop convention", "This is the current chop action."],
};

const state = {
  perspective: "Cathy",
  mode: "historical",
  boundary: 12,
  anchor: 18,
  inspectorOpen: true,
  labels: true,
  truth: false,
  truthConfirm: false,
  selectedCard: 2,
  selectedAction: "play-2",
  tab: "card",
  loading: false,
};

function icon(name, size = 16) {
  const paths = {
    eye: '<path d="M2 8s2.2-4 6-4 6 4 6 4-2.2 4-6 4-6-4-6-4Z"/><circle cx="8" cy="8" r="2"/>',
    lock: '<rect x="3.5" y="7" width="9" height="7" rx="1.5"/><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2"/>',
    shield: '<path d="M8 1.5 13 3.4v3.8c0 3.2-2 5.8-5 7.3-3-1.5-5-4.1-5-7.3V3.4L8 1.5Z"/><path d="m5.7 8 1.4 1.4 3.2-3.2"/>',
    panel: '<rect x="2" y="2.5" width="12" height="11" rx="1.5"/><path d="M9.5 2.5v11M5.5 6 3.8 8l1.7 2"/>',
    close: '<path d="m4 4 8 8M12 4l-8 8"/>',
    warning: '<path d="M8 2 15 14H1L8 2Z"/><path d="M8 6v3.5M8 12h.01"/>',
    text: '<path d="M3 4h10M3 8h10M3 12h7"/>',
  };
  return `<svg class="icon" width="${size}" height="${size}" viewBox="0 0 16 16" aria-hidden="true">${paths[name]}</svg>`;
}

function toggle(label, key, danger = false) {
  return `<button class="toolbar-toggle ${danger ? "danger" : ""}" data-do="toggle" data-key="${key}" aria-pressed="${state[key]}">
    ${danger ? icon("lock") : icon("eye")}<span>${label}</span><i class="switch ${state[key] ? "on" : ""}"><b></b></i>
  </button>`;
}

function topbar() {
  return `<header class="topbar">
    <div class="brand"><img src="./assets/navbar.svg" alt=""><strong>Hanabi.live</strong><span>HSM Debug Spectator</span></div>
    <div class="debug-toolbar">
      <span class="capability">${icon("shield")} Switchable · read-only</span>
      <label>Perspective<select data-do="perspective">${["Alice", "Bob", "Cathy", "Diana"].map((name) => `<option ${name === state.perspective ? "selected" : ""}>${name}</option>`).join("")}</select></label>
      <div class="segmented"><button data-do="mode" data-mode="historical" class="${state.mode === "historical" ? "active" : ""}">Historical</button><button data-do="mode" data-mode="hindsight" class="${state.mode === "hindsight" ? "active hindsight" : ""}">Hindsight</button></div>
      ${state.mode === "hindsight" ? `<label class="anchor-select">Anchor<select data-do="anchor">${boundaries.filter(([id]) => id >= state.boundary).map(([id]) => `<option value="${id}" ${id === state.anchor ? "selected" : ""}>Boundary ${id}</option>`).join("")}</select></label>` : ""}
      ${toggle("Card labels", "labels")}${toggle("Physical truth", "truth", true)}
    </div>
  </header>`;
}

function actionBadge(id, label) {
  const outcome = actions[id][1];
  const mark = outcome === "follow" ? "✓" : outcome === "violate" ? "!" : "?";
  return `<button class="${outcome} ${state.selectedAction === id ? "active" : ""}" data-do="action" data-id="${id}">${label} ${mark}</button>`;
}

function gameCard(suit, rank, options = {}) {
  const hidden = options.hidden && !state.truth;
  const index = options.index || 0;
  const src = hidden ? `${CARD_ROOT}/back.png` : `${CARD_ROOT}/${suit}${rank}.png`;
  return `<div class="card-shell ${index === state.selectedCard ? "selected" : ""}">
    ${options.hidden ? `<div class="action-badges">${actionBadge(`play-${index}`, "Play")}${actionBadge(`discard-${index}`, "Discard")}</div>` : ""}
    <button class="game-card" data-do="card" data-card="${index}"><img src="${src}" alt="">
      ${options.note ? `<span class="card-note">${options.note}</span>` : ""}
      ${options.chop ? '<span class="chop">CHOP</span>' : ""}
      ${options.hidden && state.truth ? `<span class="truth">${suit}${rank}</span>` : ""}
    </button>
  </div>`;
}

function hand(data, options = {}) {
  return `<div class="hand ${options.vertical ? "vertical" : ""}">${data.map(([suit, rank, note], offset) => gameCard(suit, rank, {
    hidden: options.hidden,
    index: options.hidden ? offset + 1 : 0,
    note: state.labels ? (options.hidden ? cards[offset + 1][0] : note) : "",
    chop: options.hidden && offset === 4,
  })).join("")}</div>`;
}

function replayArea() {
  const current = ((state.boundary - 8) / 10) * 100;
  const anchor = ((state.anchor - 8) / 10) * 100;
  const nativeButton = (command, image, label) => `<button data-do="replay" data-command="${command}" aria-label="${label}"><img src="${NATIVE_ROOT}/${image}" alt=""></button>`;
  return `<section class="native-replay">
    <div class="replay-status"><span>Replay boundary <b>${state.boundary}</b> of 18</span>${state.mode === "historical" ? '<strong class="historical-status">Historical · knowledge at this boundary</strong>' : `<strong class="hindsight-status">${icon("eye")} Hindsight from boundary ${state.anchor}</strong>`}</div>
    <div class="shuttle">
      ${state.mode === "hindsight" ? `<i class="future-range" style="left:${current}%;width:${Math.max(0, anchor - current)}%"></i>` : ""}
      ${boundaries.map(([id]) => `<button data-do="boundary" data-boundary="${id}" class="${id === state.boundary ? "current" : ""} ${state.mode === "hindsight" && id === state.anchor ? "anchor" : ""}" style="left:${((id - 8) / 10) * 100}%"><i></i><small>${id}</small></button>`).join("")}
    </div>
    <div class="replay-buttons">${nativeButton("first", "replay-back-full.png", "First")}${nativeButton("back", "replay-back.png", "Back")}${nativeButton("forward", "replay-forward.png", "Forward")}${nativeButton("last", "replay-forward-full.png", "Last")}<button class="exit-replay">Exit Replay</button></div>
  </section>`;
}

function board() {
  return `<main class="game-stage" style="--native-background:url('${NATIVE_ROOT}/background.jpg')">
    <div class="action-legend"><span class="follow">✓ follows</span><span class="violate">! violates</span><span class="uncertain">? neutral / unknown</span></div>
    <section class="player player-top"><span class="player-name"><i class="dot green"></i>Alice <small>2 clues</small></span>${hand([["r",4,"Play"],["g",1,"Play"],["y",2,"Save"],["b",3,"Prompt"],["k",5,"5 Save"]])}</section>
    <section class="player player-left"><span class="player-name"><i class="dot blue"></i>Bob <small>3 clues</small></span>${hand([["y",3],["b",2],["r",1],["k",4],["g",5]], { vertical: true })}</section>
    <section class="player player-right"><span class="player-name"><i class="dot red"></i>Diana <small>2 clues</small></span>${hand([["k",1],["r",2],["g",3],["b",4],["y",5]], { vertical: true })}</section>
    <div class="fireworks">${[["r",3],["y",2],["g",1],["b",2],["k",1]].map(([suit, rank]) => gameCard(suit, rank)).join("")}</div>
    <div class="resources"><b>3</b> clues <span>♥</span> <b>2</b> lives <b>27</b> deck <b>11</b> discard</div>
    <section class="player player-bottom"><span class="player-name active"><i class="dot observer"></i>${state.perspective} (perspective) <small>your turn · debug action previews</small></span>${hand([["b",5],["r",4],["g",2],["y",1],["k",3]], { hidden: true })}</section>
    <div class="public-log"><b>Recent actions</b>${boundaries.slice(3,7).map(([id, actor, action]) => `<button data-do="boundary" data-boundary="${id}" class="${id === state.boundary ? "active" : ""}"><span>${id}</span>${actor} ${action}</button>`).join("")}</div>
    ${state.truth ? '<div class="truth-banner">PRIVILEGED PHYSICAL TRUTH · diagnostic overlay only</div>' : ""}${replayArea()}
  </main>`;
}

function cardPanel() {
  const [title, body, mask, path] = cards[state.selectedCard];
  return `<section class="panel observer-panel"><div class="panel-kicker">${icon("eye")} Observer-relative <span>Card ${state.selectedCard}</span></div><h2>${title}</h2><p>${body}</p><dl><div><dt>Candidate mask</dt><dd>${mask}</dd></div><div><dt>Prerequisite path</dt><dd>${path}</dd></div><div><dt>Perspective</dt><dd>${state.perspective} at boundary ${state.boundary}</dd></div></dl><details open><summary>Why?</summary><p>Derived only from grounded information available to the selected observer under the active temporal mode.</p></details></section>`;
}

function actionPanel() {
  const [label, outcome, rule, detail] = actions[state.selectedAction];
  const result = outcome === "follow" ? "✓ Follows" : outcome === "violate" ? "! Violates" : "? Neutral / unknown";
  return `<section class="panel action-panel ${outcome}"><div class="panel-kicker">Turn action preview <span>${outcome}</span></div><h2>${label}</h2><div class="outcome">${result}</div><p>${detail}</p><dl><div><dt>Relevant rule</dt><dd>${rule}</dd></div><div><dt>Actor knowledge</dt><dd>${state.perspective}, boundary ${state.boundary}</dd></div></dl></section>`;
}

function rulesPanel() {
  return `<section class="panel"><div class="panel-kicker">Rule classification <span>Exact · trusted</span></div><div class="rule-table">${[["Focused clue","follow"],["Good Touch","follow"],["Save Principle","neutral"],["MCVP","off"]].map(([name,result]) => `<div><span>${name}</span><b class="${result}">${result}</b><i>—</i></div>`).join("")}</div><div class="immutable">${icon("lock")} Action-time record is immutable and distinct from this diagnostic snapshot.</div></section>`;
}

function textPanel(expanded = false) {
  const card = cards[state.selectedCard];
  const action = actions[state.selectedAction];
  return `<section class="panel text-output ${expanded ? "expanded" : ""}"><div class="panel-kicker">${icon("text")} Plain-text diagnostic output ${!expanded ? '<button data-do="tab" data-tab="text">Expand</button>' : ""}</div><pre>[hsm] perspective=${state.perspective} target=${state.boundary} mode=${state.mode}${state.mode === "hindsight" ? ` anchor=${state.anchor}` : ""}
[card ${state.selectedCard}] ${card[0]}
  candidates: ${card[2]}
  path: ${card[3]}
[action] ${action[0]}
  classification: ${action[1]}
  rule: ${action[2]}
  detail: ${action[3]}
[record] action-time classification remains immutable
[debug] diagnostic-only; gameplay masks unchanged</pre></section>`;
}

function inspector() {
  const panel = state.tab === "card" ? cardPanel() : state.tab === "action" ? actionPanel() : state.tab === "rules" ? rulesPanel() : textPanel(true);
  const tab = (id, label) => `<button data-do="tab" data-tab="${id}" class="${state.tab === id ? "active" : ""}">${label}</button>`;
  return `<aside class="inspector ${state.inspectorOpen ? "open" : "closed"}">
    <header><div><h1>HSM Inspector</h1><p>Read-only · cannot affect gameplay or action selection</p></div><button data-do="drawer">${icon("close",18)}</button></header>
    ${state.truthConfirm ? `<div class="truth-confirm">${icon("warning")}<div><b>Enable physical truth?</b><span>Privileged identities remain separate from observer-relative HSM output.</span></div><button data-do="truth-enable">Enable</button><button data-do="truth-cancel">Cancel</button></div>` : ""}
    <nav class="inspector-tabs">${tab("card","Card")}${tab("action","Action")}${tab("rules","Rules")}${tab("text","Text output")}</nav>
    <div class="inspector-scroll">${state.loading ? '<div class="loading">Recomputing diagnostic snapshot…</div>' : ""}${state.mode === "hindsight" ? `<div class="hindsight-callout">${icon("eye")}<div><b>Viewing boundary ${state.boundary} with hindsight</b><span>Future evidence is anchored at boundary ${state.anchor}. Past actors do not gain this knowledge.</span></div></div>` : ""}${panel}${state.tab !== "text" ? textPanel() : ""}</div>
  </aside>`;
}

function render() {
  document.querySelector("#app").innerHTML = `<div class="app-shell ${state.inspectorOpen ? "drawer-open" : "drawer-closed"}">${topbar()}<div class="workspace">${board()}${inspector()}<button class="drawer-handle ${state.inspectorOpen ? "open" : "closed"}" data-do="drawer">${icon("panel",20)}<span>${state.inspectorOpen ? "Hide" : "HSM"}</span></button></div><div class="prototype-mark">PROTOTYPE · intended production host: real Hanabi.live spectator UI</div></div>`;
}

function recompute() {
  state.loading = true;
  render();
  setTimeout(() => {
    state.loading = false;
    render();
  }, 180);
}

function setBoundary(value) {
  state.boundary = Math.max(8, Math.min(18, value));
  if (state.anchor < state.boundary) state.anchor = state.boundary;
  recompute();
}

document.addEventListener("click", (event) => {
  const target = event.target.closest("[data-do]");
  if (!target) return;
  const action = target.dataset.do;
  if (action === "drawer") state.inspectorOpen = !state.inspectorOpen;
  if (action === "mode") state.mode = target.dataset.mode;
  if (action === "toggle") {
    if (target.dataset.key === "truth" && !state.truth) state.truthConfirm = true;
    else state[target.dataset.key] = !state[target.dataset.key];
  }
  if (action === "truth-enable") { state.truth = true; state.truthConfirm = false; }
  if (action === "truth-cancel") state.truthConfirm = false;
  if (action === "boundary") return setBoundary(Number(target.dataset.boundary));
  if (action === "replay") {
    const command = target.dataset.command;
    return setBoundary(command === "first" ? 8 : command === "last" ? 18 : state.boundary + (command === "back" ? -1 : 1));
  }
  if (action === "card" && Number(target.dataset.card)) { state.selectedCard = Number(target.dataset.card); state.tab = "card"; state.inspectorOpen = true; }
  if (action === "action") { state.selectedAction = target.dataset.id; state.tab = "action"; state.inspectorOpen = true; }
  if (action === "tab") state.tab = target.dataset.tab;
  if (action === "mode" || action === "perspective") return recompute();
  render();
});

document.addEventListener("change", (event) => {
  if (event.target.matches('[data-do="perspective"]')) state.perspective = event.target.value;
  if (event.target.matches('[data-do="anchor"]')) state.anchor = Number(event.target.value);
  recompute();
});

document.addEventListener("keydown", (event) => {
  if (event.target.matches("input, textarea, select, [contenteditable]")) return;
  if (event.key === "ArrowLeft") return setBoundary(state.boundary - 1);
  if (event.key === "ArrowRight") return setBoundary(state.boundary + 1);
  if (event.key.toLowerCase() === "h") { state.mode = state.mode === "historical" ? "hindsight" : "historical"; recompute(); }
  if (event.key.toLowerCase() === "i") { state.inspectorOpen = !state.inspectorOpen; render(); }
});

render();
