import { Button } from "./controls/Button";
import { showResearchRestartModal } from "./researchRestartModal";

interface ResearchRestartButtonConfig {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** Create the Restart Controller's persistent Single Game continuation control. */
export function createResearchRestartButton(
  config: ResearchRestartButtonConfig,
): Button {
  const button = new Button({
    ...config,
    text: "New Game",
    fontSize: 0.32 * config.height,
  });
  button.on("click tap", showResearchRestartModal);
  button.tooltipName = "new-game";
  button.tooltipContent =
    "Restart this game with the same seed or start the next game.";
  return button;
}
