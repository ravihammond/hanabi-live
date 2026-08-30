import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import { beforeEach, expect, jest, test } from "@jest/globals";
import { TextEncoder as NodeTextEncoder } from "node:util";

import { initLocalTerminal } from "./localTerminal";

jest.mock("@xterm/xterm", () => ({ Terminal: jest.fn() }));
jest.mock("@xterm/addon-fit", () => ({ FitAddon: jest.fn() }));

let mockDataHandler: ((data: string) => void) | undefined;
let mockWriteCallbacks: Array<() => void> = [];
const mockTerminal = {
  cols: 80,
  focus: jest.fn(),
  loadAddon: jest.fn(),
  onData: jest.fn((handler: (data: string) => void) => {
    mockDataHandler = handler;
  }),
  open: jest.fn(),
  rows: 24,
  write: jest.fn((_data: string | Uint8Array, callback?: () => void) => {
    if (callback !== undefined) {
      mockWriteCallbacks.push(callback);
    }
  }),
};
const mockFitAddon = { fit: jest.fn() };
const mockTerminalConstructor = jest.mocked(Terminal);
const mockFitAddonConstructor = jest.mocked(FitAddon);

class RecordingWebSocket extends EventTarget {
  static readonly OPEN = 1;
  static readonly instances: RecordingWebSocket[] = [];
  binaryType = "";
  readonly readyState = RecordingWebSocket.OPEN;
  readonly sent: Array<string | ArrayBufferView> = [];
  readonly url: string;
  closed = false;

  constructor(url: string) {
    super();
    this.url = url;
    RecordingWebSocket.instances.push(this);
  }

  close(): void {
    this.closed = true;
  }

  send(data: string | ArrayBufferView): void {
    this.sent.push(data);
  }
}

beforeEach(() => {
  window.name = "";
  sessionStorage.clear();
  localStorage.clear();
  RecordingWebSocket.instances.length = 0;
  mockDataHandler = undefined;
  mockWriteCallbacks = [];
  jest.clearAllMocks();
  mockTerminalConstructor.mockImplementation(() => mockTerminal as never);
  mockFitAddonConstructor.mockImplementation(() => mockFitAddon as never);
  document.querySelector("#local-terminal-panel")?.remove();
  document.querySelector("#local-terminal-launcher")?.remove();
  Object.defineProperty(globalThis, "WebSocket", {
    configurable: true,
    value: RecordingWebSocket,
  });
  Object.defineProperty(globalThis, "TextEncoder", {
    configurable: true,
    value: NodeTextEncoder,
  });
  Object.defineProperty(globalThis, "innerWidth", { configurable: true, value: 1024 });
  Object.defineProperty(globalThis, "innerHeight", { configurable: true, value: 768 });
});

test("a normal Hanabi page has no local terminal surface", () => {
  initLocalTerminal();

  expect(RecordingWebSocket.instances).toHaveLength(0);
  expect(mockTerminalConstructor).not.toHaveBeenCalled();
  expect(document.querySelector("#local-terminal-panel")).toBeNull();
  expect(document.querySelector("#local-terminal-launcher")).toBeNull();
});

test.each([
  "not JSON",
  JSON.stringify({ version: 1, endpoint: "wss://127.0.0.1/terminal/private" }),
  JSON.stringify({ version: 1, endpoint: "ws://public.example/terminal/private" }),
  JSON.stringify({ version: 1, endpoint: "ws://user@127.0.0.1/terminal/private" }),
  JSON.stringify({ version: 1, endpoint: "ws://127.0.0.1/terminal/private#stale" }),
  JSON.stringify({ version: 2, endpoint: "ws://127.0.0.1/terminal/private" }),
])("an invalid or non-loopback descriptor is discarded", (descriptor) => {
  window.name = descriptor;

  initLocalTerminal();

  expect(window.name).toBe("");
  expect(sessionStorage.getItem("hanabi-live.local-terminal.v1")).toBeNull();
  expect(RecordingWebSocket.instances).toHaveLength(0);
  expect(document.querySelector("#local-terminal-panel")).toBeNull();
});

test("a failed terminal connection clears its stale descriptor", () => {
  sessionStorage.setItem("hanabi-live.local-terminal.v1", JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  }));

  initLocalTerminal();
  RecordingWebSocket.instances[0]?.dispatchEvent(new Event("error"));

  expect(sessionStorage.getItem("hanabi-live.local-terminal.v1")).toBeNull();
  expect(document.querySelector("#local-terminal-panel")).toBeNull();
});

test("a local descriptor mounts only after its socket opens", () => {
  const descriptor = {
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  };
  window.name = JSON.stringify(descriptor);

  initLocalTerminal();

  expect(window.name).toBe("");
  expect(JSON.parse(sessionStorage.getItem("hanabi-live.local-terminal.v1") ?? "")).toEqual(
    descriptor,
  );
  expect(RecordingWebSocket.instances).toHaveLength(1);
  expect(document.querySelector("#local-terminal-panel")).toBeNull();

  RecordingWebSocket.instances[0]?.dispatchEvent(new Event("open"));

  expect(document.querySelector("#local-terminal-panel")).not.toBeNull();
  expect(mockTerminal.open).toHaveBeenCalledTimes(1);
});

test("the terminal uses the effective iTerm2 dark palette", () => {
  window.name = JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  });

  initLocalTerminal();
  RecordingWebSocket.instances[0]?.dispatchEvent(new Event("open"));

  expect(mockTerminalConstructor).toHaveBeenCalledWith({
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
  });
});

test("PTY bytes, fitted size, and post-replay input cross the socket", () => {
  window.name = JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  });
  const output = Uint8Array.from([27, 91, 51, 49, 109, 88]);

  initLocalTerminal();
  const socket = RecordingWebSocket.instances[0];
  socket?.dispatchEvent(new Event("open"));
  socket?.dispatchEvent(new MessageEvent("message", { data: output.buffer }));
  mockDataHandler?.("blocked during replay");
  socket?.dispatchEvent(
    new MessageEvent("message", { data: JSON.stringify({ type: "replay-end" }) }),
  );
  mockDataHandler?.("blocked until xterm parses replay");
  expect(socket?.sent).toHaveLength(1);
  expect(mockWriteCallbacks).toHaveLength(1);
  mockWriteCallbacks[0]?.();
  mockDataHandler?.(String.raw`printf ready\n`);

  expect(mockTerminal.write).toHaveBeenCalledWith(output);
  expect(socket?.sent[0]).toBe('{"type":"resize","rows":24,"cols":80}');
  expect(socket?.sent).toHaveLength(2);
  expect([...(socket?.sent[1] as Uint8Array)]).toEqual([
    ...new TextEncoder().encode(String.raw`printf ready\n`),
  ]);
});

test("minimize persists only layout and keeps the terminal connected", () => {
  window.name = JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  });

  initLocalTerminal();
  const socket = RecordingWebSocket.instances[0];
  socket?.dispatchEvent(new Event("open"));
  const panel = document.querySelector("#local-terminal-panel");
  const minimize = document.querySelector("#local-terminal-minimize");
  const launcher = document.querySelector("#local-terminal-launcher");
  if (!(panel instanceof HTMLElement) || !(minimize instanceof HTMLButtonElement)) {
    throw new TypeError("terminal panel did not mount");
  }

  expect(panel.style.height).toBe(`${window.innerHeight * 0.9}px`);
  expect(panel.style.width).toBe("420px");
  expect(panel.style.left).toBe(`${window.innerWidth - 420 - 12}px`);
  expect(Number.parseFloat(panel.style.top)).toBeCloseTo(window.innerHeight * 0.05);
  minimize.click();

  expect(panel.hidden).toBe(true);
  expect(launcher).toBeInstanceOf(HTMLButtonElement);
  expect((launcher as HTMLButtonElement).hidden).toBe(false);
  expect(socket?.closed).toBe(false);
  expect(JSON.parse(localStorage.getItem("hanabi-live.local-terminal.layout.v1") ?? "")).toEqual(
    expect.objectContaining({ minimized: true }),
  );

  (launcher as HTMLButtonElement).click();

  expect(panel.hidden).toBe(false);
  expect((launcher as HTMLButtonElement).hidden).toBe(true);
  expect(mockFitAddon.fit).toHaveBeenCalledTimes(2);
  expect(mockTerminal.focus).toHaveBeenCalledTimes(1);
  expect(panel.querySelector("[data-action='close']")).toBeNull();
  expect(panel.querySelector(".local-terminal-resize")).toBeInstanceOf(HTMLElement);
  expect(panel.textContent).not.toContain("Restart");
  expect(panel.textContent).not.toMatch(/Search|Settings|Tabs/);
  expect(launcher?.querySelector("[class*='badge']")).toBeNull();
});

test("terminal focus suppresses document shortcuts without cancelling input", () => {
  window.name = JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  });
  let documentEvents = 0;
  function recordEvent() {
    documentEvents++;
  }
  for (const type of ["keydown", "keyup", "keypress"]) {
    document.addEventListener(type, recordEvent);
  }

  initLocalTerminal();
  RecordingWebSocket.instances[0]?.dispatchEvent(new Event("open"));
  const screen = document.querySelector(".local-terminal-screen");
  if (!(screen instanceof HTMLElement)) {
    throw new TypeError("terminal screen did not mount");
  }
  const events = ["keydown", "keyup", "keypress"].map(
    (type) => new KeyboardEvent(type, { bubbles: true, cancelable: true, key: "a" }),
  );
  for (const event of events) {
    screen.dispatchEvent(event);
  }
  for (const type of ["keydown", "keyup", "keypress"]) {
    document.removeEventListener(type, recordEvent);
  }

  expect(documentEvents).toBe(0);
  expect(events.every((event) => !event.defaultPrevented)).toBe(true);
});

test("viewport changes clamp saved geometry and refit the terminal", () => {
  window.name = JSON.stringify({
    version: 1,
    endpoint: "ws://127.0.0.1:43210/terminal/private",
  });
  localStorage.setItem("hanabi-live.local-terminal.layout.v1", JSON.stringify({
    left: 500,
    top: 400,
    width: 500,
    height: 500,
    minimized: false,
  }));

  initLocalTerminal();
  RecordingWebSocket.instances[0]?.dispatchEvent(new Event("open"));
  const panel = document.querySelector("#local-terminal-panel");
  if (!(panel instanceof HTMLElement)) {
    throw new TypeError("terminal panel did not mount");
  }
  Object.defineProperty(globalThis, "innerWidth", { configurable: true, value: 360 });
  Object.defineProperty(globalThis, "innerHeight", { configurable: true, value: 300 });
  globalThis.dispatchEvent(new Event("resize"));

  expect(panel.style.width).toBe("344px");
  expect(panel.style.height).toBe("284px");
  expect(panel.style.left).toBe("16px");
  expect(panel.style.top).toBe("16px");
  expect(mockFitAddon.fit).toHaveBeenCalledTimes(2);
});
