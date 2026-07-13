import { describe, expect, test } from "@jest/globals";
import jquery from "jquery";
import { Connection } from "../../Connection";
import { globals } from "../../Globals";
import * as modals from "../../modals";
import * as game from "../main";
import { showResearchRestartModal } from "./researchRestartModal";

describe("persistent Single Game New Game modal", () => {
  test("renders as a compact modal with the standard button UI", () => {
    const modal = document.querySelector("#research-restart-modal");
    if (!(modal instanceof HTMLElement)) {
      throw new TypeError("Failed to find the New Game modal.");
    }

    expect(modal.classList.contains("modal-prompt")).toBe(true);
    expect(modal.querySelector("h2")?.textContent).toBe("New Game");
    expect(modal.querySelector("p")?.textContent.trim()).toBe(
      "Restart this game with the same seed or start the next game.",
    );

    const buttons = [...modal.querySelectorAll("button")];
    expect(
      buttons.map((button) => ({
        label: button.textContent.trim(),
        standardUI: button.matches(".button.small"),
      })),
    ).toEqual([
      { label: "Cancel", standardUI: true },
      { label: "Restart this seed", standardUI: true },
      { label: "Start next game", standardUI: true },
    ]);
  });

  test("dismisses or sends the selected replacement through the live connection", () => {
    class RecordingWebSocket extends EventTarget {
      static readonly OPEN = 1;
      readonly readyState = RecordingWebSocket.OPEN;
      readonly sentMessages: string[] = [];

      send(message: string): void {
        this.sentMessages.push(message);
      }

      close(): void {
        this.dispatchEvent(new Event("close"));
      }
    }
    Object.defineProperty(globalThis, "WebSocket", {
      configurable: true,
      value: RecordingWebSocket,
    });
    Object.assign(globalThis, { $: jquery });

    globals.conn = new Connection("ws://example.test", false);
    globals.tableID = 42;
    const socket = globals.conn.ws as unknown as RecordingWebSocket;
    modals.init();
    game.init();

    showResearchRestartModal();
    expect(modals.isModalVisible()).toBe(true);
    document
      .querySelector("#research-restart-cancel")
      ?.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    expect(modals.isModalVisible()).toBe(false);
    expect(socket.sentMessages).toEqual([]);

    showResearchRestartModal();
    document
      .querySelector("#page-cover")
      ?.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    expect(modals.isModalVisible()).toBe(false);

    showResearchRestartModal();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(modals.isModalVisible()).toBe(false);

    showResearchRestartModal();
    document
      .querySelector("#research-restart-same-seed")
      ?.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    showResearchRestartModal();
    document
      .querySelector("#research-restart-next-game")
      ?.dispatchEvent(new Event("pointerdown", { bubbles: true }));

    expect(socket.sentMessages).toEqual([
      'researchRestart {"tableID":42,"restartKind":"same_seed"}',
      'researchRestart {"tableID":42,"restartKind":"next_game"}',
    ]);
  });
});
