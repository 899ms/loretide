// @vitest-environment node
import { describe, expect, it } from "vitest";

import { ApiError } from "../../api";
import {
  emptyInputSnapshot,
  missingStartFields,
  parseStartSnapshot,
  parseStartSnapshots,
  startInputToWire,
} from "./snapshot";

// What this file covers is the parse, not the rule. The assembly rules live on
// the Go side (server/internal/content/topic-planning/snapshot.go) and are
// tested there; here the question is only what this client makes of a response.

const wire = {
  snapshot_id: "snap-1",
  workspace_id: "ws-1",
  topic_card_id: "card-1",
  brief_revision_id: "rev-1",
  account_id: "acct-1",
  project_id: "",
  actor_id: "user-1",
  created_at: "2026-09-20T00:00:00Z",
  snapshot: {
    config_version: "",
    persona_ref: "persona-9",
    sop_version: "",
    skill_version: "",
    rule_version: "",
    executor: "disabled",
    executor_version: "",
    source_scope: "web",
    saved_preference: "local",
    required_sources: [],
    excluded_sources: [],
    grants: [],
    file_hashes: {},
    temperature: 0,
    budget: 0,
    timeout_ms: 0,
    auto_precheck: true,
    uses_neutral_expression: true,
  },
};

describe("parseStartSnapshot", () => {
  it("keeps the chosen scope and the saved preference apart", () => {
    const snapshot = parseStartSnapshot(wire);
    expect(snapshot.snapshot.source_scope).toBe("web");
    expect(snapshot.snapshot.saved_preference).toBe("local");
    expect(snapshot.snapshot.persona_ref).toBe("persona-9");
  });

  it("reads an empty project id as a start with no project, not as missing", () => {
    expect(parseStartSnapshot(wire).projectId).toBe("");
  });

  it("degrades a malformed response to an empty snapshot rather than throwing", () => {
    const snapshot = parseStartSnapshot({ nonsense: true });
    expect(snapshot.snapshotId).toBe("");
    expect(snapshot.snapshot).toEqual(emptyInputSnapshot());
  });

  it("fills null collections so no caller has to know which of null and [] it got", () => {
    const snapshot = parseStartSnapshot({
      ...wire,
      snapshot: {
        ...wire.snapshot,
        required_sources: null,
        excluded_sources: null,
        grants: null,
        file_hashes: null,
      },
    });
    expect(snapshot.snapshot.required_sources).toEqual([]);
    expect(snapshot.snapshot.excluded_sources).toEqual([]);
    expect(snapshot.snapshot.grants).toEqual([]);
    expect(snapshot.snapshot.file_hashes).toEqual({});
  });

  it("fills a field a newer backend did not send without blanking the rest", () => {
    const { persona_ref: _dropped, ...withoutPersona } = wire.snapshot;
    const snapshot = parseStartSnapshot({ ...wire, snapshot: withoutPersona });
    expect(snapshot.snapshot.persona_ref).toBe("");
    expect(snapshot.snapshot.source_scope).toBe("web");
  });

  it("defaults the two markers towards the more careful reading", () => {
    // A build that cannot tell whether the precheck was on, or whether the
    // account wrote neutral, should say "yes" to both: each means be more
    // careful, and neither claims a permission nobody granted.
    const empty = emptyInputSnapshot();
    expect(empty.auto_precheck).toBe(true);
    expect(empty.uses_neutral_expression).toBe(true);
    expect(empty.fit_sources).toEqual([]);
    expect(empty.evidence_sources).toEqual([]);
  });

  it("parses source extensions when present and defaults null to empty array", () => {
    const withSources = parseStartSnapshot({
      ...wire,
      snapshot: {
        ...wire.snapshot,
        fit_sources: ["src-fit-1"],
        evidence_sources: ["src-evi-1", "src-evi-2"],
      },
    });
    expect(withSources.snapshot.fit_sources).toEqual(["src-fit-1"]);
    expect(withSources.snapshot.evidence_sources).toEqual(["src-evi-1", "src-evi-2"]);

    const withNullSources = parseStartSnapshot({
      ...wire,
      snapshot: {
        ...wire.snapshot,
        fit_sources: null,
        evidence_sources: null,
      },
    });
    expect(withNullSources.snapshot.fit_sources).toEqual([]);
    expect(withNullSources.snapshot.evidence_sources).toEqual([]);
  });
});

describe("parseStartSnapshots", () => {
  it("reads a list and survives a null one", () => {
    expect(parseStartSnapshots({ start_snapshots: [wire, wire] })).toHaveLength(2);
    expect(parseStartSnapshots({ start_snapshots: null })).toEqual([]);
    expect(parseStartSnapshots("not an object")).toEqual([]);
  });
});

describe("startInputToWire", () => {
  it("always sends project_id, as empty when there is none", () => {
    // The server rejects unknown fields and reads an absent project_id as a
    // client that predates the field. "" says "no project" explicitly.
    expect(startInputToWire({ accountId: "a", sourceScope: "all" })).toEqual({
      account_id: "a",
      source_scope: "all",
      project_id: "",
    });
  });
});

describe("missingStartFields", () => {
  it("names the fields a refused start is waiting on", () => {
    const error = new ApiError("bad request", 400, "Bad Request", {
      code: "INPUT_CONFLICT",
      missing: ["content_pillars", "weekly_hours"],
    });
    expect(missingStartFields(error)).toEqual(["content_pillars", "weekly_hours"]);
  });

  it("returns nothing rather than throwing on any other failure", () => {
    expect(missingStartFields(new Error("network"))).toEqual([]);
    expect(missingStartFields(null)).toEqual([]);
    expect(
      missingStartFields(new ApiError("not found", 404, "Not Found", { code: "NOT_FOUND" })),
    ).toEqual([]);
  });
});
