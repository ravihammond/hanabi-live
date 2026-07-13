import Konva from "konva";
import type { ResearchRestartKind } from "./researchRestartModalModel";
import { createResearchRestartModalModel } from "./researchRestartModalModel";

interface ResearchRestartModalOptions {
  width: number;
  height: number;
  onRestartRequested: (kind: ResearchRestartKind) => void;
}

function createModalButton(config: {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
  text: string;
  onClick: () => void;
}): Konva.Group {
  const button = new Konva.Group({
    id: config.id,
    x: config.x,
    y: config.y,
    width: config.width,
    height: config.height,
    listening: true,
  });
  button.add(
    new Konva.Rect({
      width: config.width,
      height: config.height,
      cornerRadius: 0.12 * config.height,
      fill: "#173923",
      listening: true,
    }),
  );
  button.add(
    new Konva.Text({
      y: 0.31 * config.height,
      width: config.width,
      text: config.text,
      align: "center",
      fill: "white",
      fontFamily: "Verdana",
      fontSize: 0.28 * config.height,
      listening: false,
    }),
  );
  button.on("click tap", config.onClick);
  return button;
}

/** Build the confirmation modal used by a persistent Single Game restart controller. */
export function createResearchRestartModal({
  width,
  height,
  onRestartRequested,
}: ResearchRestartModalOptions): Konva.Group {
  const model = createResearchRestartModalModel(onRestartRequested);
  const modal = new Konva.Group({
    width,
    height,
    visible: false,
    listening: true,
  });
  modal.add(
    new Konva.Rect({
      id: "research-restart-background",
      width,
      height,
      fill: model.backgroundFill,
      cornerRadius: 0.025 * height,
      shadowColor: "black",
      shadowBlur: 20,
      shadowOpacity: 0.55,
      listening: true,
    }),
  );
  modal.add(
    new Konva.Text({
      y: 0.13 * height,
      width,
      text: model.title,
      align: "center",
      fill: "#202020",
      fontFamily: "Verdana",
      fontSize: 0.13 * height,
      listening: false,
    }),
  );
  modal.add(
    new Konva.Text({
      y: 0.34 * height,
      width,
      text: model.description,
      align: "center",
      fill: "#404040",
      fontFamily: "Verdana",
      fontSize: 0.045 * height,
      listening: false,
    }),
  );

  const buttonY = 0.63 * height;
  const buttonHeight = 0.16 * height;
  const gap = 0.035 * width;
  const cancelWidth = 0.18 * width;
  const sameSeedWidth = 0.31 * width;
  const nextGameWidth = 0.27 * width;
  const rowWidth = cancelWidth + sameSeedWidth + nextGameWidth + gap * 2;
  let buttonX = (width - rowWidth) / 2;

  const cancelButton = createModalButton({
    id: model.buttons[0].id,
    x: buttonX,
    y: buttonY,
    width: cancelWidth,
    height: buttonHeight,
    text: model.buttons[0].label,
    onClick: () => {
      modal.hide();
      model.buttons[0].select();
    },
  });
  modal.add(cancelButton);
  buttonX += cancelWidth + gap;

  const sameSeedButton = createModalButton({
    id: model.buttons[1].id,
    x: buttonX,
    y: buttonY,
    width: sameSeedWidth,
    height: buttonHeight,
    text: model.buttons[1].label,
    onClick: () => {
      modal.hide();
      model.buttons[1].select();
    },
  });
  modal.add(sameSeedButton);
  buttonX += sameSeedWidth + gap;

  const nextGameButton = createModalButton({
    id: model.buttons[2].id,
    x: buttonX,
    y: buttonY,
    width: nextGameWidth,
    height: buttonHeight,
    text: model.buttons[2].label,
    onClick: () => {
      modal.hide();
      model.buttons[2].select();
    },
  });
  modal.add(nextGameButton);

  return modal;
}
