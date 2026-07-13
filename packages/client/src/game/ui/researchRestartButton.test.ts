import { expect, test } from "@jest/globals";
import jquery from "jquery";
import * as modals from "../../modals";
import * as tooltips from "../../tooltips";
import { Button } from "./controls/Button";
import { initKonvaTooltips } from "./konvaTooltips";
import { createResearchRestartButton } from "./researchRestartButton";

test("New Game uses the shared button UI and opens its standard modal", () => {
  const button = createResearchRestartButton({
    x: 10,
    y: 20,
    width: 140,
    height: 100,
  });

  expect(button).toBeInstanceOf(Button);
  expect(button.textElement?.text()).toBe("New Game");
  expect(button.textElement?.fontSize()).toBe(32);
  expect(button.tooltipContent).toBe(
    "Restart this game with the same seed or start the next game.",
  );

  button.fire("click", {});
  expect(modals.isModalVisible()).toBe(true);
});

test("New Game registers its text with the shared tooltip UI", () => {
  Object.assign(globalThis, { $: jquery });
  tooltips.initGame();
  const button = createResearchRestartButton({
    x: 10,
    y: 20,
    width: 140,
    height: 100,
  });

  initKonvaTooltips(button, true, false);

  const tooltip = document.querySelector("#tooltip-new-game");
  expect(tooltip).toBeInstanceOf(HTMLElement);
  expect(tooltip?.classList.contains("tooltipstered")).toBe(true);
});
