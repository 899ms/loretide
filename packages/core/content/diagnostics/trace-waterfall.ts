// Derives the trace waterfall from a run's events: hierarchy from
// parent_span_id, horizontal position from occurred_at, and a bounded set of
// rows that keeps failing spans visible.
//
// This lives in core rather than in the panel because constitution principle II
// forbids UI unit tests: anything that needs a test to prove it correct has to
// sit where a test can reach it. The view renders rows it is handed.
//
// Contract and invariants: specs/006-diag-trace-waterfall-regression/contracts/trace-waterfall.md
import {STREAM_EVENT_CAP, type DiagnosticEvent} from "./contract";

export type WaterfallAnomaly =
  | "cycle"
  | "orphan"
  | "invalidTime"
  | "clockSkew"
  | "invalidDuration";

export type WaterfallRow = {
  /** "span" is a real event; "collapsed" stands in for hidden descendants. */
  kind: "span" | "collapsed";
  eventId: string;
  spanId: string;
  parentSpanId: string;
  depth: number;
  startOffsetMs: number;
  durationMs: number;
  anomaly: WaterfallAnomaly | null;
  /** Non-zero only on a "collapsed" row: how many spans it stands for. */
  collapsedCount: number;
};

export type WaterfallResult = {
  rows: WaterfallRow[];
  totalSpans: number;
  collapsed: boolean;
  anomalies: number;
};

type Node = {
  event: DiagnosticEvent;
  parent: string;
  depth: number;
  startOffsetMs: number;
  durationMs: number;
  anomaly: WaterfallAnomaly | null;
  children: string[];
};

/** Milliseconds since the epoch, or null when the value is not a usable date. */
function parseTime(value: string): number | null {
  const ms = Date.parse(value);
  return Number.isNaN(ms) ? null : ms;
}

export function buildTraceWaterfall(
  events: DiagnosticEvent[],
  options: {cap?: number} = {},
): WaterfallResult {
  const cap = options.cap ?? STREAM_EVENT_CAP;
  if (events.length === 0) {
    return {rows: [], totalSpans: 0, collapsed: false, anomalies: 0};
  }

  // Index by span id so "does this parent exist" is O(1). Walking the parent
  // chain per span against a list would make the whole build O(n²).
  const bySpan = new Map<string, DiagnosticEvent>();
  for (const event of events) {
    if (event.spanId && !bySpan.has(event.spanId)) bySpan.set(event.spanId, event);
  }

  // The baseline is the earliest parseable start. Unparseable timestamps are
  // excluded from the baseline rather than treated as 0, which would drag every
  // other offset to the right by the epoch.
  let baseline = Number.POSITIVE_INFINITY;
  for (const event of events) {
    const ms = parseTime(event.occurredAt);
    if (ms !== null && ms < baseline) baseline = ms;
  }
  if (!Number.isFinite(baseline)) baseline = 0;

  // A parent is only usable if following it terminates at a root. The visited
  // set bounds the walk at the number of spans, so a cycle is detected rather
  // than followed (research D1).
  function resolveParent(event: DiagnosticEvent): {parent: string; anomaly: WaterfallAnomaly | null} {
    if (!event.parentSpanId) return {parent: "", anomaly: null};
    if (!bySpan.has(event.parentSpanId)) return {parent: "", anomaly: "orphan"};
    const seen = new Set<string>([event.spanId]);
    let cursor = bySpan.get(event.parentSpanId);
    while (cursor) {
      if (seen.has(cursor.spanId)) return {parent: "", anomaly: "cycle"};
      seen.add(cursor.spanId);
      if (!cursor.parentSpanId) break;
      cursor = bySpan.get(cursor.parentSpanId);
    }
    return {parent: event.parentSpanId, anomaly: null};
  }

  const nodes = new Map<string, Node>();
  const order: string[] = [];
  for (const event of events) {
    const key = event.spanId || event.eventId;
    if (nodes.has(key)) continue;
    const {parent, anomaly: linkAnomaly} = resolveParent(event);
    const ms = parseTime(event.occurredAt);
    const negativeDuration = event.durationMs < 0;
    nodes.set(key, {
      event,
      parent,
      depth: 0,
      startOffsetMs: ms === null ? 0 : ms - baseline,
      durationMs: negativeDuration ? 0 : event.durationMs,
      // Precedence is fixed by data-model.md: cycle, orphan, invalidTime,
      // clockSkew, invalidDuration. clockSkew needs the parent's time, so it is
      // filled in below once every node exists.
      anomaly: linkAnomaly ?? (ms === null ? "invalidTime" : negativeDuration ? "invalidDuration" : null),
      children: [],
    });
    order.push(key);
  }

  for (const key of order) {
    const node = nodes.get(key)!;
    if (node.parent) nodes.get(node.parent)?.children.push(key);
  }

  // Clock skew: a child that starts before its parent. Reported, never
  // corrected - clamping would hide what the clock_skew scenario exists to show
  // (FR-014). Only overwrites a null anomaly, so higher-precedence flags stand.
  for (const key of order) {
    const node = nodes.get(key)!;
    if (node.anomaly !== null || !node.parent) continue;
    const parent = nodes.get(node.parent);
    if (parent && node.startOffsetMs < parent.startOffsetMs) node.anomaly = "clockSkew";
  }

  // Depth from the roots down, so no node is visited more than once.
  const roots = order.filter(key => !nodes.get(key)!.parent);
  const stack = [...roots];
  while (stack.length > 0) {
    const key = stack.pop()!;
    const node = nodes.get(key)!;
    for (const child of node.children) {
      nodes.get(child)!.depth = node.depth + 1;
      stack.push(child);
    }
  }

  // Selection. Spans carrying an error code, and every ancestor above them, are
  // kept first: truncating by time would cut the failing step, which is the main
  // reason to open this view at all (research D3).
  const keep = new Set<string>();
  if (order.length > cap) {
    for (const key of order) {
      if (!nodes.get(key)!.event.errorCode) continue;
      let cursor: string | undefined = key;
      while (cursor && !keep.has(cursor)) {
        keep.add(cursor);
        cursor = nodes.get(cursor)!.parent || undefined;
      }
    }
    // Fill the remaining budget in start order, pulling in ancestors so a kept
    // node never ends up without a visible parent.
    const remaining = order
      .filter(key => !keep.has(key))
      .sort((a, b) => nodes.get(a)!.startOffsetMs - nodes.get(b)!.startOffsetMs);
    for (const key of remaining) {
      if (keep.size >= cap) break;
      const chain: string[] = [];
      let cursor: string | undefined = key;
      while (cursor && !keep.has(cursor)) {
        chain.push(cursor);
        cursor = nodes.get(cursor)!.parent || undefined;
      }
      if (keep.size + chain.length > cap) continue;
      for (const item of chain) keep.add(item);
    }
  } else {
    for (const key of order) keep.add(key);
  }

  // Hidden spans are attributed to their nearest visible ancestor so the count
  // appears where the user can act on it.
  const hiddenUnder = new Map<string, number>();
  let hiddenTotal = 0;
  for (const key of order) {
    if (keep.has(key)) continue;
    hiddenTotal += 1;
    let cursor: string | undefined = nodes.get(key)!.parent || undefined;
    while (cursor && !keep.has(cursor)) cursor = nodes.get(cursor)!.parent || undefined;
    const anchor = cursor ?? "";
    hiddenUnder.set(anchor, (hiddenUnder.get(anchor) ?? 0) + 1);
  }

  const rows: WaterfallRow[] = [];
  function emit(key: string): void {
    const node = nodes.get(key)!;
    rows.push({
      kind: "span",
      eventId: node.event.eventId,
      spanId: node.event.spanId,
      parentSpanId: node.parent,
      depth: node.depth,
      startOffsetMs: node.startOffsetMs,
      durationMs: node.durationMs,
      anomaly: node.anomaly,
      collapsedCount: 0,
    });
    for (const child of node.children.filter(c => keep.has(c))
      .sort((a, b) => nodes.get(a)!.startOffsetMs - nodes.get(b)!.startOffsetMs)) {
      emit(child);
    }
    const hidden = hiddenUnder.get(key);
    if (hidden) {
      rows.push({
        kind: "collapsed", eventId: `${node.event.eventId}:collapsed`, spanId: "",
        parentSpanId: node.event.spanId, depth: node.depth + 1,
        startOffsetMs: node.startOffsetMs, durationMs: 0,
        anomaly: null, collapsedCount: hidden,
      });
    }
  }
  for (const key of roots.filter(k => keep.has(k))
    .sort((a, b) => nodes.get(a)!.startOffsetMs - nodes.get(b)!.startOffsetMs)) {
    emit(key);
  }
  // Spans hidden with no visible ancestor are reported once at the top level, so
  // the count still reconciles with totalSpans.
  const orphanedHidden = hiddenUnder.get("");
  if (orphanedHidden) {
    rows.push({
      kind: "collapsed", eventId: "collapsed:root", spanId: "", parentSpanId: "",
      depth: 0, startOffsetMs: 0, durationMs: 0, anomaly: null, collapsedCount: orphanedHidden,
    });
  }

  return {
    rows,
    totalSpans: order.length,
    collapsed: hiddenTotal > 0,
    anomalies: rows.filter(r => r.anomaly !== null).length,
  };
}
