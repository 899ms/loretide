// @vitest-environment node
// Canonical layer for the audit-to-technical-trace jump and the object/version grouping.
// FR map (specs/008-diag-linkage-and-invariants/spec.md):
//   FR-001 the jump is a standalone derivation, assertable without a view
//   FR-002 a missing trace id yields "unavailable", never an empty filter
//   FR-003 the produced filter is decided only by this event; prior filters are cleared
//   FR-004 only existing query dimensions are used; no request is issued
//   FR-005 FR-007 object versions are grouped from already-fetched events; never invented
// Boundary tables: contracts/trace-jump.md
import {describe, it, expect} from "vitest";
import {describeTraceJump, describeObjectVersions} from "./linkage";
import type {DiagnosticEvent} from "./contract";

const TRACE = "c".repeat(32);
const RUN = "0123456789abcdef0123456789abcdef";

// Only the fields these two derivations read are meaningful; the rest satisfy the type.
function event(over: Partial<DiagnosticEvent> = {}): DiagnosticEvent {
  return {
    eventId: "e1", sequence: 1, occurredAt: "", receivedAt: "",
    actorKind: "system", actorId: "", workspaceId: "w", accountId: "",
    objectType: "simulation", objectId: "o1", objectVersion: "1",
    action: "query", outcome: "success", errorCode: "",
    operationId: "", traceId: TRACE, spanId: "", parentSpanId: "",
    runId: RUN, attempt: 1, step: "", component: "api",
    severity: "info", durationMs: 0, safeMessage: "", retryable: false,
    nextAction: "", build: "b", isTest: true,
    route: "", status: 0, headersPresent: [], upstreamTrace: "",
    ...over,
  };
}

describe("describeTraceJump boundary table", () => {
  it("produces a technical filter when trace and run are both present", () => {
    const jump = describeTraceJump(event());
    expect(jump.kind).toBe("filter");
    if (jump.kind !== "filter") return;
    expect(jump.filter).toEqual({kind: "technical", traceId: TRACE, runId: RUN, after: 0});
  });

  it("still jumps on trace alone when the run id is empty", () => {
    // A trace can span runs; a missing run number must not block the jump (US1 case 3).
    const jump = describeTraceJump(event({runId: ""}));
    expect(jump.kind).toBe("filter");
    if (jump.kind !== "filter") return;
    expect(jump.filter.traceId).toBe(TRACE);
    expect(jump.filter.runId).toBe("");
  });

  // The defect this feature exists to fix. Asserting only `reason` would pass against
  // an implementation that also returned a filter, which is exactly the bug.
  it("returns unavailable, and NOT a filter, when the trace id is empty", () => {
    const jump = describeTraceJump(event({traceId: "", runId: RUN}));
    expect(jump.kind).toBe("unavailable");
    expect(jump).not.toHaveProperty("filter");
    if (jump.kind !== "unavailable") return;
    expect(jump.reason).toBe("noTrace");
  });

  it("does not degrade to a run-scoped jump when the trace id is empty", () => {
    // Q1 ruled A; option B (fall back to runId) is explicitly excluded.
    const jump = describeTraceJump(event({traceId: "", runId: RUN}));
    expect(JSON.stringify(jump)).not.toContain(RUN);
  });

  it("returns unavailable when both trace and run are empty", () => {
    expect(describeTraceJump(event({traceId: "", runId: ""})).kind).toBe("unavailable");
  });

  it("treats a malformed trace id the same as an absent one", () => {
    for (const bad of ["nope", "ABC", TRACE.slice(0, 31), `${TRACE}f`]) {
      expect(describeTraceJump(event({traceId: bad})).kind, bad).toBe("unavailable");
    }
  });
});

describe("describeTraceJump invariants", () => {
  // FR-003. Clearing is part of the output, not something the caller must remember.
  it("carries no component, severity, code or time window", () => {
    const jump = describeTraceJump(event({component: "daemon", severity: "error", errorCode: "TIMEOUT"}));
    if (jump.kind !== "filter") throw new Error("expected a filter");
    for (const key of ["component", "severity", "errorCode", "from", "until"]) {
      expect(jump.filter, key).not.toHaveProperty(key);
    }
    expect(jump.filter.after).toBe(0);
  });

  it("is decided only by the event it is given", () => {
    const one = describeTraceJump(event({component: "web", severity: "warn"}));
    const two = describeTraceJump(event({component: "daemon", severity: "error"}));
    expect(one).toEqual(two);
  });

  it("is pure and does not mutate its input", () => {
    const subject = event();
    const before = JSON.stringify(subject);
    describeTraceJump(subject);
    expect(JSON.stringify(subject)).toBe(before);
    expect(describeTraceJump(subject)).toEqual(describeTraceJump(subject));
  });

  // FR-004: every key must be one the existing event query already accepts.
  it("uses only existing query dimensions", () => {
    const jump = describeTraceJump(event());
    if (jump.kind !== "filter") throw new Error("expected a filter");
    expect(Object.keys(jump.filter).sort()).toEqual(["after", "kind", "runId", "traceId"]);
  });
});

describe("describeObjectVersions boundary table", () => {
  const target = {objectType: "simulation", objectId: "o1"};

  it("returns unversioned for an empty event list", () => {
    const group = describeObjectVersions([], target);
    expect(group.versions).toEqual([]);
    expect(group.state).toBe("unversioned");
  });

  it("reports unknownObject when the object type or id is missing", () => {
    expect(describeObjectVersions([event()], {objectType: "", objectId: "o1"}).state).toBe("unknownObject");
    expect(describeObjectVersions([event()], {objectType: "simulation", objectId: ""}).state).toBe("unknownObject");
  });

  it("deduplicates and keeps first-seen order, not alphabetical order", () => {
    const group = describeObjectVersions([
      event({eventId: "a", objectVersion: "9"}),
      event({eventId: "b", objectVersion: "2"}),
      event({eventId: "c", objectVersion: "9"}),
    ], target);
    expect(group.versions.map(v => v.version)).toEqual(["9", "2"]);
    expect(group.versions.map(v => v.count)).toEqual([2, 1]);
    expect(group.versions[0]?.firstEventId).toBe("a");
  });

  it("reports unversioned without inventing a placeholder", () => {
    const group = describeObjectVersions([
      event({objectVersion: ""}), event({objectVersion: ""}),
    ], target);
    expect(group.state).toBe("unversioned");
    expect(group.versions).toEqual([]);
  });

  it("ignores events belonging to other objects", () => {
    const group = describeObjectVersions([
      event({objectVersion: "1"}),
      event({objectId: "other", objectVersion: "7"}),
    ], target);
    expect(group.versions.map(v => v.version)).toEqual(["1"]);
  });

  // The same id under a different object type is a different object.
  it("does not merge the same objectId across different objectTypes", () => {
    const group = describeObjectVersions([
      event({objectType: "simulation", objectId: "o1", objectVersion: "1"}),
      event({objectType: "run", objectId: "o1", objectVersion: "2"}),
    ], target);
    expect(group.versions.map(v => v.version)).toEqual(["1"]);
  });
});

describe("describeObjectVersions invariants", () => {
  const target = {objectType: "simulation", objectId: "o1"};

  // Counts must reconcile, or the panel would under- or over-report sightings.
  it("conserves the count of matching versioned events", () => {
    const events = [
      event({objectVersion: "1"}), event({objectVersion: "1"}), event({objectVersion: "2"}),
      event({objectVersion: ""}), event({objectId: "other", objectVersion: "3"}),
    ];
    const group = describeObjectVersions(events, target);
    const total = group.versions.reduce((n, v) => n + v.count, 0);
    const expected = events.filter(e =>
      e.objectType === target.objectType && e.objectId === target.objectId && e.objectVersion !== "").length;
    expect(total).toBe(expected);
  });

  it("does not mutate the array it is given", () => {
    const events = [event({objectVersion: "1"})];
    const before = JSON.stringify(events);
    describeObjectVersions(events, target);
    expect(JSON.stringify(events)).toBe(before);
  });
});
