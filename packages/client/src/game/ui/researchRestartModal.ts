import { closeModals, showPrompt } from "../../modals";
import { getHTMLElement } from "../../utils";

export type ResearchRestartKind = "same_seed" | "next_game";

/** Initialize the browser modal used by a persistent Single Game Restart Controller. */
export function initResearchRestartModal(
  onRestartRequested: (kind: ResearchRestartKind) => void,
): void {
  const cancelButton = getHTMLElement("#research-restart-cancel");
  cancelButton.addEventListener("pointerdown", () => {
    closeModals();
  });

  const sameSeedButton = getHTMLElement("#research-restart-same-seed");
  sameSeedButton.addEventListener("pointerdown", () => {
    closeModals();
    onRestartRequested("same_seed");
  });

  const nextGameButton = getHTMLElement("#research-restart-next-game");
  nextGameButton.addEventListener("pointerdown", () => {
    closeModals();
    onRestartRequested("next_game");
  });
}

/** Show the browser modal used to replace the current Game Attempt. */
export function showResearchRestartModal(): void {
  showPrompt("#research-restart-modal");
}
