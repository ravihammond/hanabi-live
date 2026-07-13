import { describe, expect, test } from "@jest/globals";
import {
  canNavigateToLobby,
  shouldShowLegacyRestart,
  singleGameBottomLeftControls,
} from "./researchSingleGameControls";

describe("persistent Single Game controls", () => {
  test("keeps Replay and replaces Lobby with New Game for the Restart Controller", () => {
    expect(
      singleGameBottomLeftControls({
        persistentSingleGame: true,
        restartController: true,
      }),
    ).toEqual(["replay", "chat", "new_game"]);
  });

  test("gives other persistent Single Game players no lobby or continuation control", () => {
    expect(
      singleGameBottomLeftControls({
        persistentSingleGame: true,
        restartController: false,
      }),
    ).toEqual(["replay", "chat"]);
  });

  test("locks every persistent Single Game participant out of lobby navigation", () => {
    expect(canNavigateToLobby({ persistentSingleGame: true })).toBe(false);
  });

  test("never reveals the legacy top-slot Restart control in a persistent run", () => {
    expect(
      shouldShowLegacyRestart({
        persistentSingleGame: true,
        otherwiseVisible: true,
      }),
    ).toBe(false);
  });
});
