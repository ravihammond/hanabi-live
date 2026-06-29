import { describe, expect, test } from "@jest/globals";
import { websocketHostIsAllowed } from "./websocketHost";

describe("websocketHostIsAllowed", () => {
  test("accepts the configured localhost domain", () => {
    expect(websocketHostIsAllowed("localhost", "localhost")).toBe(true);
  });

  test("accepts Cloudflare Quick Tunnel hosts for localhost research sessions", () => {
    expect(
      websocketHostIsAllowed(
        "dec-leg-vocational-preston.trycloudflare.com",
        "localhost",
      ),
    ).toBe(true);
  });

  test("rejects retired localhost.run hosts for localhost research sessions", () => {
    expect(
      websocketHostIsAllowed("quiet-river-123.localhost.run", "localhost"),
    ).toBe(false);
    expect(websocketHostIsAllowed("f0691c1ab27edb.lhr.life", "localhost")).toBe(
      false,
    );
  });

  test("rejects unrelated hosts", () => {
    expect(websocketHostIsAllowed("example.com", "localhost")).toBe(false);
  });
});
