// @vitest-environment node
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  CONTENT_SCOPES,
  DEFAULT_SCOPE,
  SCOPE_SETTINGS_KEY,
  getAccountScope,
  hasStoredScope,
  isContentScope,
  withAccountScope,
} from "./scope";

// A11. Same technique as platforms.test.ts: read the Go source rather than
// restating the list a third time. A list you have to edit in two places to
// make a test pass is not holding anything.
const HERE = dirname(fileURLToPath(import.meta.url));
const SCOPE_GO = resolve(HERE, "../../../../server/internal/content/ip-profile/scope.go");

function scopesDeclaredInGo(): string[] {
  const source = readFileSync(SCOPE_GO, "utf8");
  const byName = new Map(
    [...source.matchAll(/Scope(\w+)\s+Scope\s*=\s*"([a-z_]+)"/g)].map(
      (match) => [match[1] ?? "", match[2] ?? ""] as const,
    ),
  );
  const slice = source.match(/var Scopes = \[\]Scope\{([^}]*)\}/s);
  if (!slice) throw new Error("Scopes slice not found in scope.go");
  return [...(slice[1] ?? "").matchAll(/Scope(\w+)/g)].map((match) => {
    const value = byName.get(match[1] ?? "");
    if (!value) throw new Error(`Scope${match[1]} has no string value`);
    return value;
  });
}

// The key and the default are just as much a contract as the list: a mismatch
// there means the frontend reads a key nobody writes, or shows a different
// default from the one the server fills in.
function constantInGo(name: string): string {
  const source = readFileSync(SCOPE_GO, "utf8");
  const match = source.match(new RegExp(`${name}\\s*=\\s*(?:string\\()?(?:Scope\\w+|"([^"]*)")`));
  if (!match) throw new Error(`${name} not found in scope.go`);
  if (match[1] !== undefined) return match[1];
  const scopeName = source.match(new RegExp(`${name}\\s*=\\s*string\\(Scope(\\w+)\\)`));
  const byName = new Map(
    [...source.matchAll(/Scope(\w+)\s+Scope\s*=\s*"([a-z_]+)"/g)].map(
      (m) => [m[1] ?? "", m[2] ?? ""] as const,
    ),
  );
  return byName.get(scopeName?.[1] ?? "") ?? "";
}

describe("content scopes", () => {
  it("matches the Go controlled set value for value, in order", () => {
    expect([...CONTENT_SCOPES]).toEqual(scopesDeclaredInGo());
  });

  it("uses the same settings key as Go", () => {
    expect(SCOPE_SETTINGS_KEY).toBe(constantInGo("ScopeSettingsKey"));
  });

  it("uses the same default as Go", () => {
    expect(DEFAULT_SCOPE).toBe(constantInGo("DefaultScope"));
  });

  it("recognises exactly the three values", () => {
    for (const scope of CONTENT_SCOPES) expect(isContentScope(scope)).toBe(true);
    for (const value of ["All", "LOCAL", "", " ", "everything", 7, null, undefined, ["local"]]) {
      expect(isContentScope(value)).toBe(false);
    }
  });
});

describe("reading an account's scope", () => {
  it("reads a stored value", () => {
    expect(getAccountScope({ settings: { [SCOPE_SETTINGS_KEY]: "local" } })).toBe("local");
  });

  // Every one of these is reachable from a real response.
  it("falls back to the default for anything unreadable", () => {
    expect(getAccountScope(undefined)).toBe("all");
    expect(getAccountScope({})).toBe("all");
    expect(getAccountScope({ settings: null })).toBe("all");
    expect(getAccountScope({ settings: "nope" })).toBe("all");
    expect(getAccountScope({ settings: ["local"] })).toBe("all");
    expect(getAccountScope({ settings: { other: "key" } })).toBe("all");
    expect(getAccountScope({ settings: { [SCOPE_SETTINGS_KEY]: "everything" } })).toBe("all");
    expect(getAccountScope({ settings: { [SCOPE_SETTINGS_KEY]: 7 } })).toBe("all");
  });

  // A page that wants to label one of them "default" has to tell them apart,
  // and getAccountScope cannot: both answer "all".
  it("distinguishes a stored choice from a filled-in default", () => {
    expect(hasStoredScope({ settings: { [SCOPE_SETTINGS_KEY]: "all" } })).toBe(true);
    expect(hasStoredScope({ settings: {} })).toBe(false);
    expect(hasStoredScope({ settings: { [SCOPE_SETTINGS_KEY]: "everything" } })).toBe(false);
    expect(hasStoredScope(undefined)).toBe(false);
  });

  it("does not write the default into the settings it was given", () => {
    const settings = { other: "key" };
    getAccountScope({ settings });
    expect(settings).toEqual({ other: "key" });
  });
});

describe("writing an account's scope", () => {
  it("keeps every other setting", () => {
    expect(withAccountScope({ other: "key", n: 7 }, "web")).toEqual({
      other: "key",
      n: 7,
      [SCOPE_SETTINGS_KEY]: "web",
    });
  });

  it("works from nothing readable", () => {
    expect(withAccountScope(null, "local")).toEqual({ [SCOPE_SETTINGS_KEY]: "local" });
    expect(withAccountScope("nope", "local")).toEqual({ [SCOPE_SETTINGS_KEY]: "local" });
  });

  it("does not mutate the caller's object", () => {
    const settings = { other: "key" };
    withAccountScope(settings, "web");
    expect(settings).toEqual({ other: "key" });
  });

  it("refuses to build a blob no reader would accept", () => {
    expect(() => withAccountScope({}, "everything")).toThrow();
  });
});
