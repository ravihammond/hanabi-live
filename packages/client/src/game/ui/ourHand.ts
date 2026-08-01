// Helper functions for doing actions to our own hand.

import { assertDefined } from "complete-common";
import type { CardLayout } from "./CardLayout";
import { globals } from "./UIGlobals";
import { isPlayerProjection } from "./unifiedController";

export function get(): CardLayout {
  if (!isPlayerProjection(globals.state)) {
    throw new Error(
      "Failed to get our hand because we are not currently playing.",
    );
  }

  const { ourPlayerIndex } = globals.metadata;
  const ourHand = globals.elements.playerHands[ourPlayerIndex];
  assertDefined(
    ourHand,
    `Failed to get our hand with an index of: ${ourPlayerIndex}`,
  );

  return ourHand;
}

export function checkSetDraggableAll(): void {
  if (!isPlayerProjection(globals.state)) {
    return;
  }

  const { ourPlayerIndex } = globals.metadata;
  const ourHand = globals.elements.playerHands[ourPlayerIndex];
  assertDefined(
    ourHand,
    `Failed to get our hand with an index of: ${ourPlayerIndex}`,
  );

  ourHand.checkSetDraggableAll();
}
