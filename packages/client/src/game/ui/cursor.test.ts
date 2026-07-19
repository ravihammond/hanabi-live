import { afterEach, describe, expect, jest, test } from "@jest/globals";
import { globals } from "./UIGlobals";
import { elementOverlaps } from "./cursor";

describe("game cursor overlap", () => {
  afterEach(() => {
    globals.loading = true;
    globals.isResizing = false;
    jest.restoreAllMocks();
  });

  test("does not query Konva before the stage has registered a pointer", () => {
    globals.loading = false;
    globals.isResizing = false;
    const warning = jest.spyOn(console, "warn").mockImplementation(() => undefined);

    expect(elementOverlaps({} as never)).toBe(false);
    expect(warning).not.toHaveBeenCalled();
  });
});
