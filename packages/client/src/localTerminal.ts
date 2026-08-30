import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import interact from "interactjs";

const SESSION_KEY = "hanabi-live.local-terminal.v1";
const LAYOUT_KEY = "hanabi-live.local-terminal.layout.v1";
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
  version: 1;
}

interface TerminalLayout {
  height: number;
  left: number;
  minimized: boolean;
  top: number;
  width: number;
}

let removeViewportListener: (() => void) | undefined;

export function initLocalTerminal(): void {
  removeViewportListener?.();
  removeViewportListener = undefined;
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
  socket.addEventListener("open", () => {
    mountTerminal(terminal, fitAddon, socket);
  });
  socket.addEventListener("message", (event) => {
    if (typeof event.data === "string") {
      const replayMessage = replayMessageType(event.data);
      if (replayMessage === "start") {
        replayFinished = false;
      } else if (replayMessage === "end") {
        terminal.write("", () => {
          replayFinished = true;
        });
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
  const resizeHandle = document.createElement("div");
  resizeHandle.className = "local-terminal-resize";
  resizeHandle.setAttribute("aria-hidden", "true");
  panel.append(header, terminalElement, resizeHandle);
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
  configureInteraction(panel, header, resizeHandle, layout, terminal, fitAddon, socket);
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
  resizeHandle: HTMLElement,
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
      edges: { right: resizeHandle, bottom: resizeHandle },
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

function replayMessageType(message: string): "start" | "end" | undefined {
  try {
    const payload = JSON.parse(message) as { type?: string };
    if (payload.type === "replay-start") {
      return "start";
    }
    if (payload.type === "replay-end") {
      return "end";
    }
  } catch {
    return undefined;
  }
  return undefined;
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
    const candidate = JSON.parse(stored) as Partial<TerminalDescriptor>;
    const endpoint = new URL(candidate.endpoint ?? "");
    if (
      candidate.version !== 1
      || !isLoopbackTerminalEndpoint(endpoint)
    ) {
      throw new Error("invalid local terminal descriptor");
    }
    return { version: 1, endpoint: endpoint.href };
  } catch {
    sessionStorage.removeItem(SESSION_KEY);
    return undefined;
  }
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
