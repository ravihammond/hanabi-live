import { describe, expect, test } from "@jest/globals";
import { createResearchRestartModalModel } from "./researchRestartModalModel";

describe("persistent Single Game restart modal", () => {
  test("offers cancel, exact-layout restart, and next-index game actions", () => {
    const requestedKinds: string[] = [];
    const modal = createResearchRestartModalModel((kind) => {
      requestedKinds.push(kind);
    });

    expect(modal.backgroundFill).toBe("white");
    expect(modal.buttons.map((button) => button.label)).toEqual([
      "Cancel",
      "Restart this seed",
      "Start new game",
    ]);

    modal.buttons[0].select();
    expect(requestedKinds).toEqual([]);
    modal.buttons[1].select();
    expect(requestedKinds).toEqual(["same_seed"]);
    modal.buttons[2].select();
    expect(requestedKinds).toEqual(["same_seed", "next_game"]);
  });
});
