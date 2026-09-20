// @vitest-environment node

import { describe, expect, it } from "vitest";
import { todayLinks } from "./links";

describe("todayLinks", () => {
  it("builds every destination under the given slug", () => {
    expect(todayLinks("acme")).toEqual({
      topics: "/acme/topics",
      accounts: "/acme/accounts",
      sources: "/acme/sources",
    });
  });

  // The regression this function exists for (Issue #191). A workspace id where
  // the slug belongs produces a URL that parses fine and resolves to nothing:
  // the route matches, the workspace lookup compares against `slug` only, and
  // the page lands on "no access". Nothing about the STRING is wrong, which is
  // why only the caller can get this right - so this pins the shape a caller
  // must produce, and the page test pins that the page produces it.
  it("puts the slug in the workspace position, whatever it is given", () => {
    const uuidish = "3f2b1c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d";
    expect(todayLinks(uuidish).topics).toBe(`/${uuidish}/topics`);
  });

  it("encodes a slug that would otherwise change the path", () => {
    expect(todayLinks("a/b").topics).toBe("/a%2Fb/topics");
  });
});
