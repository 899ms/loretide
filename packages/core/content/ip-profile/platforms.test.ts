// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { CONTENT_PLATFORMS } from "./platforms";

// A6. The frontend keeps its own copy of the platform list, so something has
// to make a divergence impossible to land. This reads the Go source rather
// than restating the list a third time: restating it would mean a change in Go
// needs two edits here to go green, and a test you have to edit to make pass
// is not holding anything.
//
// The same technique guards the Go side against migration 477's CHECK.

const HERE = dirname(fileURLToPath(import.meta.url));
const ACCOUNT_GO = resolve(
  HERE,
  "../../../../server/internal/content/ip-profile/account.go",
);

function platformsDeclaredInGo(): string[] {
  const source = readFileSync(ACCOUNT_GO, "utf8");
  // Each constant reads: PlatformXiaohongshu Platform = "xiaohongshu"
  const declared = [...source.matchAll(/Platform\w+\s+Platform\s*=\s*"([a-z_]+)"/g)]
    .map((match) => match[1] ?? "");
  // The const block is the declaration; `Platforms` is the set that is actually
  // enforced. A constant declared but left out of the slice is not a platform,
  // so the comparison runs against the slice.
  const slice = source.match(/var Platforms = \[\]Platform\{([^}]*)\}/s);
  if (!slice) throw new Error("Platforms slice not found in account.go");
  const names = [...(slice[1] ?? "").matchAll(/Platform(\w+)/g)].map(
    (match) => match[1] ?? "",
  );
  const byName = new Map(
    [...source.matchAll(/Platform(\w+)\s+Platform\s*=\s*"([a-z_]+)"/g)].map(
      (match) => [match[1] ?? "", match[2] ?? ""] as const,
    ),
  );
  expect(declared.length).toBeGreaterThan(0);
  return names.map((name) => {
    const value = byName.get(name);
    if (!value) throw new Error(`Platform${name} has no string value`);
    return value;
  });
}

describe("content platforms", () => {
  it("matches the Go controlled set value for value, in order", () => {
    expect([...CONTENT_PLATFORMS]).toEqual(platformsDeclaredInGo());
  });

  it("has no duplicates", () => {
    expect(new Set(CONTENT_PLATFORMS).size).toBe(CONTENT_PLATFORMS.length);
  });
});
