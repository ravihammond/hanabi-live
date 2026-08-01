import { expect, test } from "@jest/globals";
import * as websocketInitModule from "./websocketInit";

function allowed(commandName: string) {
  const predicate = (
    websocketInitModule as unknown as {
      gameCommandAllowedWhileLoading: (name: string) => boolean;
    }
  ).gameCommandAllowedWhileLoading;
  return predicate(commandName);
}

test("the complete unified projection may initialize the shared renderer", () => {
  expect(allowed("init")).toBe(true);
  expect(allowed("gameActionList")).toBe(true);
  expect(allowed("researchUnifiedProjection")).toBe(true);
  expect(allowed("gameAction")).toBe(false);
});
