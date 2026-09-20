// @vitest-environment node
//
// The guard for Issue #191.
//
// The Today dashboard shipped with twelve links built as `/${wsId}/topics`,
// where `wsId` came from `useWorkspaceId()` — the workspace's UUID. The route
// segment is `[workspaceSlug]` and `workspaceBySlugOptions` compares against
// `slug` only, so every one of those links resolved to no workspace and landed
// on "no access".
//
// Nothing about the produced string is malformed, which is why no parser and
// no type caught it: only the choice of value was wrong. So this reads the page
// source. That is a blunt instrument, but the alternative — mounting the page
// and clicking twelve buttons — is a UI test, which this card does not write
// (constitution II), and it is the exact class of mistake a source scan can
// state precisely.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const PAGE = join(__dirname, "page.tsx");

function pageSource(): string {
  const source = readFileSync(PAGE, "utf8");
  if (source.trim() === "") throw new Error("the page is empty; every check below would pass vacuously");
  return source;
}

describe("Today dashboard navigation targets", () => {
  // The literal shape of the bug.
  it("never builds a path out of the workspace id", () => {
    const source = pageSource();
    const offenders = [...source.matchAll(/[`"'(]\/\$\{\s*(wsId|workspaceId)\s*\}/g)].map(
      (match) => match[0],
    );
    expect(
      offenders,
      "a path is being built from the workspace id; the URL segment is the slug (Issue #191)",
    ).toEqual([]);
  });

  // The general form: no path literal starting with a slash and an
  // interpolation at all. Whatever the variable is called, a hand-built
  // workspace path is how the id gets in.
  it("builds no workspace path by hand", () => {
    const source = pageSource();
    const handBuilt = [...source.matchAll(/push\(\s*`\//g)].map((match) => match[0]);
    expect(
      handBuilt,
      "navigation targets must come from paths.workspace(slug) via todayLinks(), not from a template literal",
    ).toEqual([]);
  });

  // And the positive half: the page must actually be getting a slug. Without
  // this, deleting every link would make both checks above pass.
  it("resolves the workspace slug and routes through todayLinks", () => {
    const source = pageSource();
    expect(source).toContain("useRequiredWorkspaceSlug");
    expect(source).toContain("todayLinks(");
    // useWorkspaceId is still legitimate - the query hooks key on the id - so
    // this asserts both are present rather than that one is gone.
    expect(source).toContain("useWorkspaceId");
  });

  it("sends every link somewhere todayLinks names", () => {
    const source = pageSource();
    const targets = [...source.matchAll(/navigation\.push\(([^)]*)\)/g)].map((match) =>
      match[1]!.trim(),
    );
    expect(targets.length, "no navigation at all; this guard would pass vacuously").toBeGreaterThan(0);
    for (const target of targets) {
      expect(target, `unexpected navigation target: ${target}`).toMatch(/^links\.(topics|accounts|sources)$/);
    }
  });
});
