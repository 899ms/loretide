// Derives where a reader should look next, given one diagnostic event: the
// technical trace behind an audit entry, and the versions recorded for the
// object that entry touched.
//
// This lives in core rather than in the panel because constitution principle II
// forbids UI unit tests: anything that needs a test to prove it correct has to
// sit where a test can reach it. The panel used to do the jump inline as a run
// of setState calls, which meant the rule "an event without a trace id must not
// silently show the unfiltered log" had nowhere to be asserted.
//
// Neither function issues a request. Both read only what the caller already
// fetched, and the jump filter uses only dimensions the existing event query
// already accepts (FR-004, FR-019).
//
// Contract and boundary tables:
// specs/008-diag-linkage-and-invariants/contracts/trace-jump.md
import type {DiagnosticEvent} from "./contract";

/** Why a jump cannot be offered. Named rather than boolean so a future reason
 *  forces callers to handle it, the same reason server enum switches need a
 *  default branch. */
export type TraceJumpBlocker = "noTrace";

export type TraceJumpFilter = {
  kind: "technical";
  traceId: string;
  /** May be empty: a trace can span runs, so a missing run number narrows
   *  nothing but must not block the jump. */
  runId: string;
  after: number;
};

export type TraceJump =
  | {kind: "filter"; filter: TraceJumpFilter}
  | {kind: "unavailable"; reason: TraceJumpBlocker};

/** Trace ids come from NewID(): 16 random bytes hex-encoded (contract.go:56).
 *  Sanitize already blanks anything that fails this on the server, so a
 *  malformed value reaching the client means the same thing as an absent one. */
const TRACE_ID_PATTERN = /^[0-9a-f]{32}$/;

export function describeTraceJump(event: DiagnosticEvent): TraceJump {
  const traceId = event.traceId ?? "";
  if (!TRACE_ID_PATTERN.test(traceId)) {
    // Not a filter with an empty trace: that would show the whole technical log
    // right after the reader asked for one trace, and they would read it as the
    // cause of the failure they were looking at (FR-002).
    return {kind: "unavailable", reason: "noTrace"};
  }
  // Clearing the other dimensions is part of the result, not something the
  // caller has to remember. Whatever was filtered before is gone (FR-003).
  return {
    kind: "filter",
    filter: {kind: "technical", traceId, runId: event.runId ?? "", after: 0},
  };
}

export type ObjectVersionEntry = {
  version: string;
  /** Lets the caller get back to a concrete event for this version. */
  firstEventId: string;
  count: number;
};

export type ObjectVersionState = "present" | "unversioned" | "unknownObject";

export type ObjectVersionGroup = {
  objectType: string;
  objectId: string;
  /** Deduplicated, in first-seen order: the reader wants the sequence, not the
   *  alphabet. */
  versions: ObjectVersionEntry[];
  state: ObjectVersionState;
};

/**
 * Groups the versions recorded for one object across the events the caller
 * already holds.
 *
 * Scope is deliberately that array and nothing more. The event query has no
 * object dimension (its filter carries trace, component, severity, code, run
 * and time only), so reaching further would mean a new server read path, which
 * Q2 ruled out. The cost is that events for the same object on other pages are
 * not counted here; the panel says so, and manual item O-3 checks that it does.
 */
export function describeObjectVersions(
  events: readonly DiagnosticEvent[],
  target: Pick<DiagnosticEvent, "objectType" | "objectId">,
): ObjectVersionGroup {
  const objectType = target.objectType ?? "";
  const objectId = target.objectId ?? "";
  if (objectType === "" || objectId === "") {
    return {objectType, objectId, versions: [], state: "unknownObject"};
  }

  const order: string[] = [];
  const byVersion = new Map<string, ObjectVersionEntry>();
  for (const candidate of events) {
    // Both dimensions must match: the same id under a different object type is
    // a different object, not another sighting of this one.
    if ((candidate.objectType ?? "") !== objectType) continue;
    if ((candidate.objectId ?? "") !== objectId) continue;
    const version = candidate.objectVersion ?? "";
    if (version === "") continue;
    const seen = byVersion.get(version);
    if (seen) {
      seen.count += 1;
      continue;
    }
    byVersion.set(version, {version, firstEventId: candidate.eventId, count: 1});
    order.push(version);
  }

  const versions = order.map(v => byVersion.get(v)!);
  return {
    objectType,
    objectId,
    versions,
    // No version recorded is a fact about the data, not a gap to fill with a
    // placeholder (FR-007).
    state: versions.length > 0 ? "present" : "unversioned",
  };
}
