export type BottomLeftControl = "replay" | "chat" | "lobby" | "new_game";

interface SingleGameControlContext {
  persistentSingleGame: boolean;
  restartController: boolean;
}

interface SingleGameAttendanceContext {
  persistentSingleGame: boolean;
}

interface LegacyRestartContext extends SingleGameAttendanceContext {
  otherwiseVisible: boolean;
}

/** Return whether a participant may navigate away from the game to the lobby. */
export function canNavigateToLobby({
  persistentSingleGame,
}: SingleGameAttendanceContext): boolean {
  return !persistentSingleGame;
}

/** Preserve upstream restart behavior outside persistent Single Game runs only. */
export function shouldShowLegacyRestart({
  persistentSingleGame,
  otherwiseVisible,
}: LegacyRestartContext): boolean {
  return !persistentSingleGame && otherwiseVisible;
}

/** Return the ordered controls shown in the lower-left game area. */
export function singleGameBottomLeftControls({
  persistentSingleGame,
  restartController,
}: SingleGameControlContext): readonly BottomLeftControl[] {
  if (persistentSingleGame) {
    return restartController
      ? ["replay", "chat", "new_game"]
      : ["replay", "chat"];
  }
  return ["replay", "chat", "lobby"];
}
