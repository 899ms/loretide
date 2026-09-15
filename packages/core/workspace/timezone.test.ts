// @vitest-environment node
// Canonical layer for the workspace timezone setting.
// FR map (specs/004-lt009-brand-workspace/spec.md):
//   FR-002 optional at creation; absent or unreadable reads as Asia/Shanghai
//   FR-004 a malformed response must not throw; it falls back to the default
//   FR-007 the value lives at settings["loretide.timezone"], never a column
// Contract: contracts/workspace-timezone.md
import { describe, it, expect } from "vitest";
import {
  TIMEZONE_SETTINGS_KEY,
  DEFAULT_TIMEZONE,
  isValidTimezone,
  getWorkspaceTimezone,
  withWorkspaceTimezone,
  supportedTimezones,
  hasStoredTimezone,
} from "./timezone";

function ws(settings: unknown): { settings?: unknown } {
  return { settings };
}

describe("isValidTimezone", () => {
  it("accepts real IANA names", () => {
    for (const name of ["Asia/Shanghai", "America/Los_Angeles", "Europe/London", "UTC"]) {
      expect(isValidTimezone(name), name).toBe(true);
    }
  });

  it("rejects invented names, empty values and non-strings", () => {
    for (const name of ["Mars/Olympus", "", "  ", "Asia/Shangai", "not a zone"]) {
      expect(isValidTimezone(name), JSON.stringify(name)).toBe(false);
    }
    for (const value of [null, undefined, 42, {}, []]) {
      expect(isValidTimezone(value as unknown as string), JSON.stringify(value)).toBe(false);
    }
  });

  // An offset string is not an IANA zone; accepting it would let a value
  // through that the server's time.LoadLocation rejects, so the two ends would
  // disagree about what is storable.
  it("rejects a fixed offset, which is not a zone name", () => {
    expect(isValidTimezone("+08:00")).toBe(false);
  });
});

describe("getWorkspaceTimezone", () => {
  it("returns a valid stored value unchanged", () => {
    expect(getWorkspaceTimezone(ws({ [TIMEZONE_SETTINGS_KEY]: "America/Los_Angeles" })))
      .toBe("America/Los_Angeles");
  });

  // Every one of these is reachable from a real response: an older workspace
  // with no settings, a server that sent null, or a settings blob written by
  // something that did not know about this key.
  it("falls back to the default for anything unreadable", () => {
    const cases: unknown[] = [
      undefined,
      null,
      {},
      { other: 1 },
      { [TIMEZONE_SETTINGS_KEY]: "" },
      { [TIMEZONE_SETTINGS_KEY]: "Mars/Olympus" },
      { [TIMEZONE_SETTINGS_KEY]: 42 },
      { [TIMEZONE_SETTINGS_KEY]: null },
      { [TIMEZONE_SETTINGS_KEY]: { name: "Asia/Shanghai" } },
      "a string, not an object",
      42,
      [],
    ];
    for (const settings of cases) {
      expect(getWorkspaceTimezone(ws(settings)), JSON.stringify(settings)).toBe(DEFAULT_TIMEZONE);
    }
  });

  it("does not throw on a workspace with no settings property at all", () => {
    expect(getWorkspaceTimezone({})).toBe(DEFAULT_TIMEZONE);
    expect(getWorkspaceTimezone(undefined as unknown as {settings?: unknown})).toBe(DEFAULT_TIMEZONE);
  });

  it("defaults to Asia/Shanghai", () => {
    expect(DEFAULT_TIMEZONE).toBe("Asia/Shanghai");
  });
});

describe("withWorkspaceTimezone", () => {
  // The server replaces settings wholesale, so a caller that sends only the
  // timezone key would wipe every other setting. This merge is what prevents
  // that, which is why it is tested rather than inlined at the call site.
  it("keeps every other settings key", () => {
    const merged = withWorkspaceTimezone({ a: 1, b: "two" }, "Europe/London");
    expect(merged).toEqual({ a: 1, b: "two", [TIMEZONE_SETTINGS_KEY]: "Europe/London" });
  });

  it("overwrites an existing timezone without touching siblings", () => {
    const merged = withWorkspaceTimezone(
      { a: 1, [TIMEZONE_SETTINGS_KEY]: "Asia/Shanghai" }, "UTC");
    expect(merged).toEqual({ a: 1, [TIMEZONE_SETTINGS_KEY]: "UTC" });
  });

  it("produces a single-key object when there were no settings", () => {
    for (const empty of [null, undefined, "nonsense", 42]) {
      expect(withWorkspaceTimezone(empty, "UTC")).toEqual({ [TIMEZONE_SETTINGS_KEY]: "UTC" });
    }
  });

  it("does not mutate the settings it was given", () => {
    const original = { a: 1 };
    withWorkspaceTimezone(original, "UTC");
    expect(original).toEqual({ a: 1 });
  });

  it("refuses to write an invalid zone", () => {
    expect(() => withWorkspaceTimezone({}, "Mars/Olympus")).toThrow();
  });
});

describe("supportedTimezones", () => {
  it("offers only zones isValidTimezone accepts", () => {
    const zones = supportedTimezones();
    expect(zones.length).toBeGreaterThan(0);
    for (const zone of zones) {
      expect(isValidTimezone(zone), zone).toBe(true);
    }
  });

  // The picker must always be able to show the value a workspace defaults to,
  // or a user with the default would see an empty selection.
  it("always includes the default", () => {
    expect(supportedTimezones()).toContain(DEFAULT_TIMEZONE);
  });

  it("returns a usable list even when the runtime has no supportedValuesOf", () => {
    const original = (Intl as unknown as Record<string, unknown>).supportedValuesOf;
    delete (Intl as unknown as Record<string, unknown>).supportedValuesOf;
    try {
      const zones = supportedTimezones();
      expect(zones).toContain(DEFAULT_TIMEZONE);
      expect(zones.length).toBeGreaterThan(5);
    } finally {
      (Intl as unknown as Record<string, unknown>).supportedValuesOf = original;
    }
  });
});

describe("hasStoredTimezone", () => {
  // The whole point: these two read the same through getWorkspaceTimezone and
  // must not read the same here, or the settings page mislabels a deliberate
  // choice as a default.
  it("separates a deliberate Asia/Shanghai from never having chosen", () => {
    expect(hasStoredTimezone(ws({ [TIMEZONE_SETTINGS_KEY]: "Asia/Shanghai" }))).toBe(true);
    expect(hasStoredTimezone(ws({}))).toBe(false);
    expect(getWorkspaceTimezone(ws({ [TIMEZONE_SETTINGS_KEY]: "Asia/Shanghai" })))
      .toBe(getWorkspaceTimezone(ws({})));
  });

  it("treats an unusable stored value as not set", () => {
    for (const bad of ["", "Mars/Olympus", 42, null]) {
      expect(hasStoredTimezone(ws({ [TIMEZONE_SETTINGS_KEY]: bad })), JSON.stringify(bad)).toBe(false);
    }
    expect(hasStoredTimezone(undefined)).toBe(false);
    expect(hasStoredTimezone({})).toBe(false);
  });
});
