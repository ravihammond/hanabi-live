import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import interact from "interactjs";

const SESSION_KEY = "hanabi-live.local-terminal.v1";
const LAYOUT_KEY = "hanabi-live.local-terminal.layout.v1";
const RESIZE_DIRECTIONS = [
  "top",
  "top-right",
  "right",
  "bottom-right",
  "bottom",
  "bottom-left",
  "left",
  "top-left",
] as const;
const TERMINAL_OPTIONS = {
  drawBoldTextInBrightColors: true,
  fontFamily: '"Hack Nerd Font Mono", Hack, Menlo, monospace',
  fontSize: 12,
  theme: {
    background: "#001E27",
    black: "#002831",
    blue: "#2176C7",
    brightBlack: "#006488",
    brightBlue: "#178EC8",
    brightCyan: "#00B39E",
    brightGreen: "#51EF84",
    brightMagenta: "#E24D8E",
    brightRed: "#F5163B",
    brightWhite: "#FCF4DC",
    brightYellow: "#B27E28",
    cursor: "#609192",
    cursorAccent: "#002831",
    cyan: "#259286",
    foreground: "#C5D8D9",
    green: "#6CBE6C",
    magenta: "#C61C6F",
    red: "#D11C24",
    selectionBackground: "#003748",
    selectionForeground: "#7A8F8E",
    white: "#EAE3CB",
    yellow: "#A57706",
  },
} as const;

interface TerminalDescriptor {
  endpoint: string;
  mode?: "hsm-profiler";
  version: 1;
}

interface TerminalLayout {
  height: number;
  left: number;
  minimized: boolean;
  top: number;
  width: number;
}

interface SessionViewport {
  atBottom: boolean;
  line: number;
}

interface ProfilerConnection {
  activeBoundary: number | undefined;
  generation: number | undefined;
  opened: boolean;
  pendingBoundary: number | undefined;
  sentBoundary: number | undefined;
  setReplayFinished: (finished: boolean) => void;
  socket: WebSocket;
  terminal: Terminal;
  viewports: Map<number, SessionViewport>;
}

interface ReplayMessage {
  boundary?: number;
  generation?: number;
  type: "replay-start" | "replay-end" | "profiler-reset";
}

let removeViewportListener: (() => void) | undefined;
let profilerConnection: ProfilerConnection | undefined;

export function selectLocalTerminalBoundary(boundary: number): void {
  const connection = profilerConnection;
  if (
    connection === undefined
    || !Number.isInteger(boundary)
    || boundary < 0
    || connection.sentBoundary === boundary
  ) {
    return;
  }
  connection.pendingBoundary = boundary;
  connection.setReplayFinished(false);
  if (!connection.opened) {
    return;
  }
  sendSelectedBoundary(connection);
}

export function initLocalTerminal(): void {
  removeViewportListener?.();
  removeViewportListener = undefined;
  profilerConnection = undefined;
  const descriptor = readDescriptor();
  if (descriptor === undefined) {
    return;
  }
  const terminal = new Terminal(TERMINAL_OPTIONS);
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  const socket = new WebSocket(descriptor.endpoint);
  socket.binaryType = "arraybuffer";
  let replayFinished = false;
  if (descriptor.mode === "hsm-profiler") {
    profilerConnection = {
      activeBoundary: undefined,
      generation: undefined,
      opened: false,
      pendingBoundary: undefined,
      sentBoundary: undefined,
      setReplayFinished: (finished) => {
        replayFinished = finished;
      },
      socket,
      terminal,
      viewports: new Map(),
    };
  }
  socket.addEventListener("open", () => {
    mountTerminal(terminal, fitAddon, socket);
    if (profilerConnection?.socket === socket) {
      profilerConnection.opened = true;
      sendSelectedBoundary(profilerConnection);
    }
  });
  socket.addEventListener("message", (event) => {
    if (typeof event.data === "string") {
      const replayMessage = parseReplayMessage(event.data);
      const connection =
        profilerConnection?.socket === socket ? profilerConnection : undefined;
      if (replayMessage?.type === "replay-start") {
        replayFinished = false;
        if (connection !== undefined) {
          startProfilerReplay(connection, replayMessage);
        }
      } else if (replayMessage?.type === "replay-end") {
        if (
          connection !== undefined
          && !matchesActiveProfilerReplay(connection, replayMessage)
        ) {
          return;
        }
        terminal.write("", () => {
          if (connection !== undefined) {
            restoreProfilerViewport(connection, replayMessage);
          }
          replayFinished = true;
        });
      } else if (
        replayMessage?.type === "profiler-reset"
        && connection !== undefined
      ) {
        resetProfiler(connection, replayMessage);
      }
    } else if (event.data instanceof ArrayBuffer) {
      terminal.write(new Uint8Array(event.data));
    }
  });
  socket.addEventListener("error", () => {
    sessionStorage.removeItem(SESSION_KEY);
  });
  terminal.onData((data) => {
    if (replayFinished && socket.readyState === WebSocket.OPEN) {
      socket.send(new TextEncoder().encode(data));
    }
  });
}

function mountTerminal(
  terminal: Terminal,
  fitAddon: FitAddon,
  socket: WebSocket,
) {
  const panel = document.createElement("section");
  panel.id = "local-terminal-panel";
  const header = document.createElement("header");
  header.className = "local-terminal-header";
  header.innerHTML = '<span><i class="fas fa-terminal"></i> Terminal</span>';
  const minimize = document.createElement("button");
  minimize.id = "local-terminal-minimize";
  minimize.type = "button";
  minimize.setAttribute("aria-label", "Minimize terminal");
  minimize.innerHTML = '<i class="fas fa-minus"></i>';
  header.append(minimize);
  const terminalElement = document.createElement("div");
  terminalElement.className = "local-terminal-screen";
  panel.append(header, terminalElement);
  for (const direction of RESIZE_DIRECTIONS) {
    const resizeHandle = document.createElement("div");
    const edgeClasses = direction.split("-").map(
      (edge) => `local-terminal-resize-${edge}`,
    );
    resizeHandle.classList.add("local-terminal-resize", ...edgeClasses);
    resizeHandle.dataset["direction"] = direction;
    resizeHandle.setAttribute("aria-hidden", "true");
    panel.append(resizeHandle);
  }
  for (const type of ["keydown", "keyup", "keypress"]) {
    panel.addEventListener(type, (event) => {
      event.stopPropagation();
    });
  }

  const launcher = document.createElement("button");
  launcher.id = "local-terminal-launcher";
  launcher.type = "button";
  launcher.setAttribute("aria-label", "Restore terminal");
  launcher.innerHTML = '<i class="fas fa-terminal"></i>';
  const layout = loadLayout();
  applyLayout(panel, layout);
  panel.hidden = layout.minimized;
  launcher.hidden = !layout.minimized;
  document.body.append(panel, launcher);

  terminal.open(terminalElement);
  if (!layout.minimized) {
    fitTerminal(terminal, fitAddon, socket);
  }
  minimize.addEventListener("click", () => {
    layout.minimized = true;
    panel.hidden = true;
    launcher.hidden = false;
    saveLayout(layout);
  });
  launcher.addEventListener("click", () => {
    layout.minimized = false;
    panel.hidden = false;
    launcher.hidden = true;
    saveLayout(layout);
    fitTerminal(terminal, fitAddon, socket);
    terminal.focus();
  });
  configureInteraction(panel, header, layout, terminal, fitAddon, socket);
  function handleViewportResize() {
    Object.assign(layout, clampLayout(layout));
    applyLayout(panel, layout);
    saveLayout(layout);
    if (!layout.minimized) {
      fitTerminal(terminal, fitAddon, socket);
    }
  };
  window.addEventListener("resize", handleViewportResize);
  removeViewportListener = () => {
    window.removeEventListener("resize", handleViewportResize);
  };
}

function configureInteraction(
  panel: HTMLElement,
  header: HTMLElement,
  layout: TerminalLayout,
  terminal: Terminal,
  fitAddon: FitAddon,
  socket: WebSocket,
) {
  /* eslint-disable @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access, @typescript-eslint/restrict-plus-operands */
  interact(panel)
    .draggable({
      allowFrom: header,
      modifiers: [interact.modifiers.restrictRect({ restriction: "parent" })],
      listeners: {
        move: (event: Interact.InteractEvent) => {
          layout.left += event.dx;
          layout.top += event.dy;
          Object.assign(layout, clampLayout(layout));
          applyLayout(panel, layout);
          saveLayout(layout);
        },
      },
    })
    .resizable({
      edges: {
        top: ".local-terminal-resize-top",
        right: ".local-terminal-resize-right",
        bottom: ".local-terminal-resize-bottom",
        left: ".local-terminal-resize-left",
      },
      modifiers: [
        interact.modifiers.restrictSize({ min: { width: 320, height: 180 } }),
      ],
      listeners: {
        move: (event: Interact.ResizeEvent) => {
          layout.left += event.deltaRect?.left ?? 0;
          layout.top += event.deltaRect?.top ?? 0;
          layout.width = event.rect.width;
          layout.height = event.rect.height;
          Object.assign(layout, clampLayout(layout));
          applyLayout(panel, layout);
          saveLayout(layout);
          fitTerminal(terminal, fitAddon, socket);
        },
      },
    });
  /* eslint-enable @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access, @typescript-eslint/restrict-plus-operands */
}

function fitTerminal(
  terminal: Terminal,
  fitAddon: FitAddon,
  socket: WebSocket,
) {
  fitAddon.fit();
  socket.send(JSON.stringify({
    type: "resize",
    rows: terminal.rows,
    cols: terminal.cols,
  }));
}

function loadLayout(): TerminalLayout {
  const fallback = defaultLayout();
  const stored = localStorage.getItem(LAYOUT_KEY);
  if (stored === null) {
    return fallback;
  }
  try {
    const candidate = JSON.parse(stored) as Partial<TerminalLayout>;
    const values = [candidate.left, candidate.top, candidate.width, candidate.height];
    if (values.some((value) => typeof value !== "number" || !Number.isFinite(value))) {
      return fallback;
    }
    return clampLayout({
      left: candidate.left!,
      top: candidate.top!,
      width: candidate.width!,
      height: candidate.height!,
      minimized: candidate.minimized === true,
    });
  } catch {
    return fallback;
  }
}

function defaultLayout(): TerminalLayout {
  const width = Math.min(420, Math.max(1, window.innerWidth - 16));
  const height = Math.min(window.innerHeight * 0.9, Math.max(1, window.innerHeight - 16));
  return {
    width,
    height,
    left: Math.max(0, window.innerWidth - width - 12),
    top: Math.max(0, (window.innerHeight - height) / 2),
    minimized: false,
  };
}

function clampLayout(layout: TerminalLayout): TerminalLayout {
  const width = Math.min(Math.max(320, layout.width), Math.max(1, window.innerWidth - 16));
  const height = Math.min(Math.max(180, layout.height), Math.max(1, window.innerHeight - 16));
  return {
    width,
    height,
    left: Math.min(Math.max(0, layout.left), Math.max(0, window.innerWidth - width)),
    top: Math.min(Math.max(0, layout.top), Math.max(0, window.innerHeight - height)),
    minimized: layout.minimized,
  };
}

function applyLayout(panel: HTMLElement, layout: TerminalLayout) {
  panel.style.left = `${layout.left}px`;
  panel.style.top = `${layout.top}px`;
  panel.style.width = `${layout.width}px`;
  panel.style.height = `${layout.height}px`;
}

function saveLayout(layout: TerminalLayout) {
  localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
}

function parseReplayMessage(message: string): ReplayMessage | undefined {
  try {
    const payload = JSON.parse(message) as Partial<ReplayMessage>;
    if (
      payload.type === "replay-start"
      || payload.type === "replay-end"
      || payload.type === "profiler-reset"
    ) {
      return payload as ReplayMessage;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function startProfilerReplay(
  connection: ProfilerConnection,
  message: ReplayMessage,
) {
  if (!validProfilerReplayMessage(message)) {
    return;
  }
  if (connection.generation !== message.generation) {
    connection.generation = message.generation;
    connection.activeBoundary = undefined;
    connection.viewports.clear();
  } else if (connection.activeBoundary !== undefined) {
    const buffer = connection.terminal.buffer.active;
    connection.viewports.set(connection.activeBoundary, {
      atBottom: buffer.viewportY === buffer.baseY,
      line: buffer.viewportY,
    });
  }
  connection.activeBoundary = message.boundary;
  connection.terminal.reset();
}

function restoreProfilerViewport(
  connection: ProfilerConnection,
  message: ReplayMessage,
) {
  const { boundary } = message;
  if (boundary === undefined) {
    return;
  }
  const viewport = connection.viewports.get(boundary);
  if (viewport?.atBottom === true) {
    connection.terminal.scrollToBottom();
  } else if (viewport !== undefined) {
    connection.terminal.scrollToLine(viewport.line);
  }
}

function matchesActiveProfilerReplay(
  connection: ProfilerConnection,
  message: ReplayMessage,
) {
  return (
    validProfilerReplayMessage(message)
    && message.generation === connection.generation
    && message.boundary === connection.activeBoundary
  );
}

function resetProfiler(connection: ProfilerConnection, message: ReplayMessage) {
  if (!Number.isInteger(message.generation) || message.generation! < 0) {
    return;
  }
  connection.setReplayFinished(false);
  connection.generation = message.generation;
  connection.activeBoundary = undefined;
  connection.pendingBoundary = undefined;
  connection.sentBoundary = undefined;
  connection.viewports.clear();
  connection.terminal.reset();
}

function validProfilerReplayMessage(
  message: ReplayMessage,
): message is Required<ReplayMessage> {
  return (
    Number.isInteger(message.generation)
    && message.generation! >= 0
    && Number.isInteger(message.boundary)
    && message.boundary! >= 0
  );
}

function readDescriptor(): TerminalDescriptor | undefined {
  const transferred = window.name;
  if (transferred !== "") {
    window.name = "";
    sessionStorage.setItem(SESSION_KEY, transferred);
  }
  const stored = sessionStorage.getItem(SESSION_KEY);
  if (stored === null) {
    return undefined;
  }
  try {
    const candidate = JSON.parse(stored) as {
      endpoint?: string;
      mode?: unknown;
      version?: unknown;
    };
    const endpoint = new URL(candidate.endpoint ?? "");
    if (
      candidate.version !== 1
      || (candidate.mode !== undefined && candidate.mode !== "hsm-profiler")
      || !isLoopbackTerminalEndpoint(endpoint)
    ) {
      throw new Error("invalid local terminal descriptor");
    }
    return { version: 1, endpoint: endpoint.href, mode: candidate.mode };
  } catch {
    sessionStorage.removeItem(SESSION_KEY);
    return undefined;
  }
}

function sendSelectedBoundary(connection: ProfilerConnection) {
  const boundary = connection.pendingBoundary;
  if (boundary === undefined || boundary === connection.sentBoundary) {
    return;
  }
  connection.socket.send(
    JSON.stringify({
      type: "select-boundary",
      boundary,
    }),
  );
  connection.sentBoundary = boundary;
  connection.pendingBoundary = undefined;
}

function isLoopbackTerminalEndpoint(endpoint: URL): boolean {
  const path = endpoint.pathname.split("/");
  return endpoint.protocol === "ws:"
    && ["127.0.0.1", "localhost"].includes(endpoint.hostname)
    && endpoint.port !== ""
    && endpoint.username === ""
    && endpoint.password === ""
    && endpoint.search === ""
    && endpoint.hash === ""
    && path.length === 3
    && path[1] === "terminal"
    && /^[A-Za-z0-9_-]+$/u.test(path[2] ?? "");
}
