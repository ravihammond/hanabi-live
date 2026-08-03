import { getDefaultMetadata } from "@hanabi-live/game";
import { afterEach, describe, expect, jest, test } from "@jest/globals";
import { readFileSync } from "node:fs";
import path from "node:path";
import { initialState } from "../reducers/initialStates/initialState";
import { globals } from "./UIGlobals";
import { elementOverlaps, set } from "./cursor";

describe("game cursor overlap", () => {
  afterEach(() => {
    globals.loading = true;
    globals.isResizing = false;
    jest.restoreAllMocks();
  });

  test("does not query Konva before the stage has registered a pointer", () => {
    globals.loading = false;
    globals.isResizing = false;
    const warning = jest
      .spyOn(console, "warn")
      .mockImplementation(() => undefined);

    expect(elementOverlaps({} as never)).toBe(false);
    expect(warning).not.toHaveBeenCalled();
  });
});

test("selectable names use a pointer without changing drag cursors", () => {
  const style = document.createElement("style");
  style.textContent = readFileSync(
    path.resolve(__dirname, "../../../../../public/css/hanabi.css"),
    "utf8",
  );
  document.head.append(style);
  const state = initialState(getDefaultMetadata(2));
  globals.store = { getState: () => state } as never;
  const game = document.querySelector<HTMLElement>("#game")!;

  set("pointer");
  expect(getComputedStyle(game).cursor).toBe("pointer");

  set("hand");
  expect(getComputedStyle(game).cursor).toBe("grab");

  set("dragging");
  expect(getComputedStyle(game).cursor).toBe("grabbing");

  style.remove();
  globals.store = null;
});
