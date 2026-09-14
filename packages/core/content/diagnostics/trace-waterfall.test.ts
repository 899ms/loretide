// @vitest-environment node
// Canonical layer for the trace waterfall derivation.
// FR map (specs/006-diag-trace-waterfall-regression/spec.md):
//   FR-001 parent_span_id builds the hierarchy; an unreachable parent is a top-level orphan
//   FR-002 horizontal position is the offset from the earliest occurred_at
//   FR-003 cycles and self-references terminate and degrade to top level
//   FR-004 the 200 cap keeps error spans and their ancestors, collapsing the rest
//   FR-014 positions are never clamped into the parent interval
// Invariants and boundary rows come from contracts/trace-waterfall.md.
import {describe, it, expect} from "vitest";
import {buildTraceWaterfall} from "./trace-waterfall";
import {STREAM_EVENT_CAP} from "./contract";
import type {DiagnosticEvent} from "./contract";

const BASE = "2026-01-01T00:00:00.000Z";

function at(msFromBase: number): string {
  return new Date(Date.parse(BASE) + msFromBase).toISOString();
}

// Only the fields the waterfall reads are meaningful; the rest satisfy the type.
function span(over: Partial<DiagnosticEvent> & {spanId: string}): DiagnosticEvent {
  return {
    eventId: `e-${over.spanId}`, sequence: 0, occurredAt: BASE, receivedAt: BASE,
    actorKind: "system", actorId: "a", workspaceId: "w", accountId: "",
    objectType: "simulation", objectId: "o", objectVersion: "1",
    action: "step", outcome: "success", errorCode: "",
    operationId: "op", traceId: "t", parentSpanId: "",
    runId: "r", attempt: 1, step: over.spanId, component: "api",
    severity: "info", durationMs: 0, safeMessage: "", retryable: false,
    nextAction: "", build: "b", isTest: true,
    route: "", status: 0, headersPresent: [], upstreamTrace: "",
    ...over,
  };
}

function rowOf(result: ReturnType<typeof buildTraceWaterfall>, spanId: string) {
  return result.rows.find(r => r.kind === "span" && r.spanId === spanId);
}

describe("buildTraceWaterfall invariants", () => {
  // Invariant 1. A cycle must not recurse forever; the contract lets us assert a
  // return value rather than a timeout.
  it("terminates on a two-node cycle and degrades both to top level", () => {
    const result = buildTraceWaterfall([
      span({spanId: "a", parentSpanId: "b"}),
      span({spanId: "b", parentSpanId: "a"}),
    ]);
    expect(result.rows).toHaveLength(2);
    for (const id of ["a", "b"]) {
      expect(rowOf(result, id)?.depth).toBe(0);
      expect(rowOf(result, id)?.anomaly).toBe("cycle");
    }
  });

  it("terminates on a self-reference", () => {
    const result = buildTraceWaterfall([span({spanId: "a", parentSpanId: "a"})]);
    expect(rowOf(result, "a")?.depth).toBe(0);
    expect(rowOf(result, "a")?.anomaly).toBe("cycle");
  });

  it("terminates on a long parent chain without overflowing", () => {
    const deep = Array.from({length: 500}, (_, i) =>
      span({spanId: `s${i}`, parentSpanId: i === 0 ? "" : `s${i - 1}`, occurredAt: at(i)}));
    const result = buildTraceWaterfall(deep, {cap: 500});
    expect(result.rows).toHaveLength(500);
    expect(rowOf(result, "s499")?.depth).toBe(499);
  });

  // Invariant 2. Every visible span's parent is visible, or the span is a root.
  // This is the whole meaning of "collapsing does not break the hierarchy".
  it("leaves no dangling row: each visible span's parent is visible or it is a root", () => {
    const events = [
      span({spanId: "root", occurredAt: at(0)}),
      span({spanId: "mid", parentSpanId: "root", occurredAt: at(10)}),
      span({spanId: "leaf", parentSpanId: "mid", occurredAt: at(20), errorCode: "TIMEOUT"}),
      ...Array.from({length: 30}, (_, i) =>
        span({spanId: `noise${i}`, parentSpanId: "root", occurredAt: at(100 + i)})),
    ];
    const result = buildTraceWaterfall(events, {cap: 5});
    const visible = new Set(result.rows.filter(r => r.kind === "span").map(r => r.spanId));
    for (const row of result.rows) {
      if (row.kind !== "span") continue;
      if (row.depth === 0) continue;
      expect(visible.has(row.parentSpanId)).toBe(true);
    }
  });

  // Invariant 3. The failing step is why anyone opens the waterfall, so it must
  // survive collapsing together with every ancestor above it.
  it("keeps an error span and its full ancestor chain even when collapsing", () => {
    const events = [
      span({spanId: "root", occurredAt: at(0)}),
      span({spanId: "mid", parentSpanId: "root", occurredAt: at(10)}),
      span({spanId: "boom", parentSpanId: "mid", occurredAt: at(900), errorCode: "DATABASE_UNAVAILABLE"}),
      ...Array.from({length: 50}, (_, i) =>
        span({spanId: `filler${i}`, parentSpanId: "root", occurredAt: at(20 + i)})),
    ];
    const result = buildTraceWaterfall(events, {cap: 4});
    expect(result.collapsed).toBe(true);
    for (const id of ["boom", "mid", "root"]) {
      expect(rowOf(result, id), `${id} must stay visible`).toBeDefined();
    }
  });

  // Invariant 4.
  it("produces non-negative offsets with at least one zero", () => {
    const result = buildTraceWaterfall([
      span({spanId: "a", occurredAt: at(500)}),
      span({spanId: "b", occurredAt: at(1500)}),
    ]);
    const offsets = result.rows.map(r => r.startOffsetMs);
    expect(Math.min(...offsets)).toBe(0);
    expect(offsets.every(o => o >= 0)).toBe(true);
    expect(rowOf(result, "b")?.startOffsetMs).toBe(1000);
  });

  // Invariant 5. A single unparseable timestamp must not poison the whole chart:
  // one NaN offset makes every width computed against it collapse.
  it("never yields NaN, even for an unparseable occurredAt", () => {
    const result = buildTraceWaterfall([
      span({spanId: "ok", occurredAt: at(0)}),
      span({spanId: "bad", occurredAt: "not-a-date"}),
    ]);
    for (const row of result.rows) {
      expect(Number.isNaN(row.startOffsetMs)).toBe(false);
      expect(Number.isNaN(row.durationMs)).toBe(false);
    }
    expect(rowOf(result, "bad")?.anomaly).toBe("invalidTime");
  });

  // Invariant 6. Clock skew is annotated, never corrected: clamping would hide
  // exactly what the clock_skew scenario exists to surface.
  it("does not clamp a child that starts before its parent", () => {
    const result = buildTraceWaterfall([
      span({spanId: "parent", occurredAt: at(1000)}),
      span({spanId: "child", parentSpanId: "parent", occurredAt: at(0)}),
    ]);
    expect(rowOf(result, "child")?.anomaly).toBe("clockSkew");
    // The child is the earliest event, so it is the baseline and stays at 0;
    // the parent sits 1000ms later. A clamped implementation would invert this.
    expect(rowOf(result, "child")?.startOffsetMs).toBe(0);
    expect(rowOf(result, "parent")?.startOffsetMs).toBe(1000);
  });

  // Invariant 7.
  it("conserves span count across collapsing", () => {
    const events = Array.from({length: 40}, (_, i) =>
      span({spanId: `s${i}`, parentSpanId: i === 0 ? "" : "s0", occurredAt: at(i)}));
    const result = buildTraceWaterfall(events, {cap: 6});
    expect(result.totalSpans).toBe(40);
    const shown = result.rows.filter(r => r.kind === "span").length;
    const hidden = result.rows.reduce((n, r) => n + r.collapsedCount, 0);
    expect(shown + hidden).toBe(40);
  });
});

describe("buildTraceWaterfall boundary inputs", () => {
  it("returns nothing for an empty run", () => {
    const result = buildTraceWaterfall([]);
    expect(result.rows).toEqual([]);
    expect(result.totalSpans).toBe(0);
    expect(result.collapsed).toBe(false);
  });

  it("renders a single span at depth 0 and offset 0", () => {
    const result = buildTraceWaterfall([span({spanId: "only", occurredAt: at(9999)})]);
    expect(result.rows).toHaveLength(1);
    expect(result.rows[0]?.depth).toBe(0);
    expect(result.rows[0]?.startOffsetMs).toBe(0);
  });

  it("handles every duration being zero without dividing by zero", () => {
    const result = buildTraceWaterfall([
      span({spanId: "a", durationMs: 0}),
      span({spanId: "b", durationMs: 0}),
    ]);
    expect(result.rows.every(r => r.durationMs === 0)).toBe(true);
    expect(result.rows.every(r => Number.isFinite(r.startOffsetMs))).toBe(true);
  });

  it("normalises a negative duration to zero and flags it", () => {
    const result = buildTraceWaterfall([span({spanId: "a", durationMs: -50})]);
    expect(rowOf(result, "a")?.durationMs).toBe(0);
    expect(rowOf(result, "a")?.anomaly).toBe("invalidDuration");
  });

  it("treats an unreachable parent as a top-level orphan, not a silent drop", () => {
    const result = buildTraceWaterfall([span({spanId: "a", parentSpanId: "gone"})]);
    expect(result.rows).toHaveLength(1);
    expect(rowOf(result, "a")?.depth).toBe(0);
    expect(rowOf(result, "a")?.anomaly).toBe("orphan");
  });

  it("does not flag a root whose parentSpanId is empty", () => {
    const result = buildTraceWaterfall([span({spanId: "a", parentSpanId: ""})]);
    expect(rowOf(result, "a")?.anomaly).toBeNull();
  });

  it("collapses past the cap and defaults that cap to STREAM_EVENT_CAP", () => {
    const events = Array.from({length: STREAM_EVENT_CAP + 5}, (_, i) =>
      span({spanId: `s${i}`, occurredAt: at(i)}));
    const result = buildTraceWaterfall(events);
    expect(result.totalSpans).toBe(STREAM_EVENT_CAP + 5);
    expect(result.collapsed).toBe(true);
    expect(result.rows.filter(r => r.kind === "span").length).toBeLessThanOrEqual(STREAM_EVENT_CAP);
  });

  it("does not collapse when the run fits", () => {
    const result = buildTraceWaterfall([span({spanId: "a"}), span({spanId: "b"})]);
    expect(result.collapsed).toBe(false);
    expect(result.rows.every(r => r.kind === "span")).toBe(true);
  });
});

describe("buildTraceWaterfall ordering and anomaly precedence", () => {
  it("places a parent immediately before its children, siblings by start offset", () => {
    const result = buildTraceWaterfall([
      span({spanId: "late", parentSpanId: "root", occurredAt: at(200)}),
      span({spanId: "root", occurredAt: at(0)}),
      span({spanId: "early", parentSpanId: "root", occurredAt: at(100)}),
    ]);
    expect(result.rows.map(r => r.spanId)).toEqual(["root", "early", "late"]);
    expect(rowOf(result, "early")?.depth).toBe(1);
  });

  it("reports cycle ahead of orphan when both could apply", () => {
    // `a` points at `b`, `b` points at a span that is not present. `b` is an
    // orphan; `a` is not in a cycle and must resolve through `b`.
    const result = buildTraceWaterfall([
      span({spanId: "a", parentSpanId: "b"}),
      span({spanId: "b", parentSpanId: "missing"}),
    ]);
    expect(rowOf(result, "b")?.anomaly).toBe("orphan");
    expect(rowOf(result, "a")?.anomaly).toBeNull();
    expect(rowOf(result, "a")?.depth).toBe(1);
  });

  it("counts anomalies", () => {
    const result = buildTraceWaterfall([
      span({spanId: "a", parentSpanId: "gone"}),
      span({spanId: "b", durationMs: -1}),
      span({spanId: "c"}),
    ]);
    expect(result.anomalies).toBe(2);
  });
});
