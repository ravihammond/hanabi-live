export type ResearchRestartKind = "same_seed" | "next_game";

interface ResearchRestartModalButton {
  id: string;
  label: string;
  select: () => void;
}

export interface ResearchRestartModalModel {
  backgroundFill: "white";
  title: string;
  description: string;
  buttons: readonly [
    ResearchRestartModalButton,
    ResearchRestartModalButton,
    ResearchRestartModalButton,
  ];
}

/** Return the presentation and user actions for the persistent Single Game modal. */
export function createResearchRestartModalModel(
  onRestartRequested: (kind: ResearchRestartKind) => void,
): ResearchRestartModalModel {
  return {
    backgroundFill: "white",
    title: "Restart the game?",
    description: "Choose the same deal or advance to the next game index.",
    buttons: [
      {
        id: "research-restart-cancel",
        label: "Cancel",
        select: () => undefined,
      },
      {
        id: "research-restart-same-seed",
        label: "Restart this seed",
        select: () => {
          onRestartRequested("same_seed");
        },
      },
      {
        id: "research-restart-next-game",
        label: "Start new game",
        select: () => {
          onRestartRequested("next_game");
        },
      },
    ],
  };
}
