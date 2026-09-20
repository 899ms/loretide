// @vitest-environment node
import { describe, expect, it } from "vitest";
import { emptyInputSnapshot, type InputSnapshot } from "./snapshot";
import {
  PENDING_SNAPSHOT_FIELDS,
  canSubmitStart,
  pendingSnapshotRows,
  recordedSnapshotRows,
  scopeDiffersFromPreference,
  startBlocker,
  startFormDraft,
} from "./start-form";

function snapshot(overrides: Partial<InputSnapshot> = {}): InputSnapshot {
  return { ...emptyInputSnapshot(), ...overrides };
}

const ready = { accountId: "account-1", revisionId: "rev-1", canStart: true };

describe("what blocks a start", () => {
  it("reports the first missing thing, in the server's order", () => {
    // Readiness is about an account: naming missing fields before one is
    // chosen would name fields belonging to nobody.
    expect(startBlocker({ ...ready, accountId: "", canStart: false })).toBe("account");
    expect(startBlocker({ ...ready, revisionId: "", canStart: false })).toBe("revision");
    expect(startBlocker({ ...ready, canStart: false })).toBe("readiness");
    expect(startBlocker(ready)).toBeNull();
  });

  it("refuses while a start is in flight", () => {
    // The endpoint is deliberately not idempotent, so a second click would be
    // a second real start rather than a retry of the first.
    expect(canSubmitStart({ ...ready, pending: false })).toBe(true);
    expect(canSubmitStart({ ...ready, pending: true })).toBe(false);
    expect(canSubmitStart({ ...ready, canStart: false, pending: false })).toBe(false);
  });
});

describe("fields with no source yet", () => {
  it("names all nine, one by one", () => {
    expect(PENDING_SNAPSHOT_FIELDS).toEqual([
      "config_version",
      "sop_version",
      "skill_version",
      "rule_version",
      "executor_version",
      "required_sources",
      "excluded_sources",
      "grants",
      "file_hashes",
    ]);
    expect(pendingSnapshotRows(snapshot()).every((row) => row.empty)).toBe(true);
    expect(pendingSnapshotRows(undefined).every((row) => row.empty)).toBe(true);
  });

  it("stops claiming a field is unavailable once a backend fills it", () => {
    const rows = pendingSnapshotRows(
      snapshot({ grants: ["grant-1"], sop_version: "2026-09", file_hashes: { "a.md": "h" } }),
    );
    const filled = rows.filter((row) => !row.empty).map((row) => row.field);
    expect(filled).toEqual(["sop_version", "grants", "file_hashes"]);
  });
});

describe("what a recorded start says", () => {
  it("keeps the two extension keys apart from the aligned sixteen", () => {
    const rows = recordedSnapshotRows(snapshot({ source_scope: "web", persona_ref: "rev-9" }));
    const extensions = rows.filter((row) => row.extension).map((row) => row.field);
    expect(extensions).toEqual(["auto_precheck", "uses_neutral_expression"]);
    expect(rows.find((row) => row.field === "source_scope")?.value).toBe("web");
    expect(rows.find((row) => row.field === "persona_ref")?.value).toBe("rev-9");
    // Numbers render as text rather than as blanks: 0 is a recorded value.
    expect(rows.find((row) => row.field === "temperature")?.value).toBe("0");
  });

  it("tells a reader when this start did not use the stored preference", () => {
    expect(
      scopeDiffersFromPreference(snapshot({ source_scope: "web", saved_preference: "all" })),
    ).toBe(true);
    expect(
      scopeDiffersFromPreference(snapshot({ source_scope: "all", saved_preference: "all" })),
    ).toBe(false);
    // A degraded snapshot says nothing rather than claiming a difference.
    expect(scopeDiffersFromPreference(snapshot())).toBe(false);
  });
});

describe("the form's starting state", () => {
  it("seeds the scope from the account preference and picks the newest revision", () => {
    expect(startFormDraft({ savedPreference: "local", latestRevisionId: "rev-3" })).toEqual({
      revisionId: "rev-3",
      sourceScope: "local",
      projectId: "",
    });
    // No revisions yet is a real state, not an error: the block says so and
    // the button stays disabled.
    expect(startFormDraft({ savedPreference: "all", latestRevisionId: "" }).revisionId).toBe("");
  });
});
