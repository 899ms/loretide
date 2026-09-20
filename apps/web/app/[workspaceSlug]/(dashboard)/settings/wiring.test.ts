// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Issue #208 moved SOP §3.2's operating rules out of the shared workspace tab
// and into this route, so the tab's own render stops depending on a
// downstream module's data hooks.
//
// That trade buys a new failure mode: the slot can be left unfilled and the
// section just stops appearing, with nothing to say it went. Before, deleting
// it meant deleting a line from a file that obviously rendered it; now it
// means passing one fewer prop.
//
// This reads the route source rather than mounting anything (constitution II:
// no UI unit tests). It is a wiring assertion, in the layer CLAUDE.md puts
// wiring assertions in.

const route = readFileSync(join(__dirname, "page.tsx"), "utf8");

describe("the settings route", () => {
  it("fills the workspace tab's extras slot", () => {
    expect(route).toContain("renderWorkspaceExtras");
  });

  it("fills it with the operating rules sections", () => {
    expect(route).toContain("OperatingRulesSections");
    expect(route).toContain("@multica/views/content/workspace-core");
  });

  // Both of them: a section rendered without `canManage` would be editable
  // for every member, and one without `wsId` could not read anything.
  it("passes the slot's context through", () => {
    expect(route).toContain("wsId");
    expect(route).toContain("canManage");
  });
});
