// The start screen's non-visual logic (EP-04b).
//
// What may be started, what is still missing, what a recorded start says, and
// which parts of a snapshot have no source yet. None of it needs a browser to
// be true, so none of it is asserted through one.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

import { type InputSnapshot, type SourceScope } from "./snapshot";

/**
 * Why the start button is not available, or null when it is.
 *
 * The order mirrors the server's decision order: an account must be chosen
 * before its readiness can be judged at all, and a revision must be chosen
 * before there is anything to start. Reporting readiness while no account is
 * selected would name fields belonging to nobody.
 */
export type StartBlocker = "account" | "revision" | "readiness" | null;

export function startBlocker(input: {
  accountId: string;
  revisionId: string;
  canStart: boolean;
}): StartBlocker {
  if (!input.accountId) return "account";
  if (!input.revisionId) return "revision";
  if (!input.canStart) return "readiness";
  return null;
}

export function canSubmitStart(input: {
  accountId: string;
  revisionId: string;
  canStart: boolean;
  pending: boolean;
}): boolean {
  // In flight means not again: the endpoint is deliberately not idempotent, so
  // a double click would be a second real start rather than a retry.
  return !input.pending && startBlocker(input) === null;
}

/**
 * The nine snapshot fields that have no source yet, each with the card that
 * will bring it.
 *
 * Listed one by one rather than as "the empty ones": when one of them starts
 * arriving, the page should name that one, and a person looking at a blank row
 * should be able to tell who owns it. `empty` is computed rather than assumed
 * so a field a newer backend fills stops claiming to be unavailable.
 */
export const PENDING_SNAPSHOT_FIELDS = [
  "config_version",
  "sop_version",
  "skill_version",
  "rule_version",
  "executor_version",
  "required_sources",
  "excluded_sources",
  "grants",
  "file_hashes",
] as const;

export type PendingSnapshotField = (typeof PENDING_SNAPSHOT_FIELDS)[number];

export interface PendingFieldRow {
  field: PendingSnapshotField;
  empty: boolean;
}

export function pendingSnapshotRows(
  snapshot: InputSnapshot | undefined,
): PendingFieldRow[] {
  return PENDING_SNAPSHOT_FIELDS.map((field) => ({
    field,
    empty: isSnapshotFieldEmpty(snapshot, field),
  }));
}

function isSnapshotFieldEmpty(
  snapshot: InputSnapshot | undefined,
  field: PendingSnapshotField,
): boolean {
  if (!snapshot) return true;
  const value = snapshot[field];
  if (typeof value === "string") return value === "";
  if (Array.isArray(value)) return value.length === 0;
  if (value && typeof value === "object")
    return Object.keys(value).length === 0;
  return true;
}

/**
 * The fields a recorded start actually fixed, in reading order.
 *
 * `extension` marks the two keys that are topic-planning's own rather than part
 * of the sixteen aligned with diagnostics.Snapshot. The page shows them apart
 * for the same reason the contract lists them apart: every one of the sixteen
 * already means something else.
 */
export interface SnapshotRow {
  field: keyof InputSnapshot;
  value: string;
  extension: boolean;
}

export function recordedSnapshotRows(snapshot: InputSnapshot): SnapshotRow[] {
  return [
    {
      field: "source_scope" as const,
      value: snapshot.source_scope,
      extension: false,
    },
    {
      field: "saved_preference" as const,
      value: snapshot.saved_preference,
      extension: false,
    },
    {
      field: "persona_ref" as const,
      value: snapshot.persona_ref,
      extension: false,
    },
    { field: "executor" as const, value: snapshot.executor, extension: false },
    {
      field: "temperature" as const,
      value: String(snapshot.temperature),
      extension: false,
    },
    {
      field: "budget" as const,
      value: String(snapshot.budget),
      extension: false,
    },
    {
      field: "timeout_ms" as const,
      value: String(snapshot.timeout_ms),
      extension: false,
    },
    {
      field: "auto_precheck" as const,
      value: String(snapshot.auto_precheck),
      extension: true,
    },
    {
      field: "uses_neutral_expression" as const,
      value: String(snapshot.uses_neutral_expression),
      extension: true,
    },
  ];
}

/**
 * True when this start used a scope other than the account's stored preference.
 *
 * The two are separate fields on purpose, and they are equal in most starts —
 * which is exactly why a reader needs to be told when they are not.
 */
export function scopeDiffersFromPreference(snapshot: InputSnapshot): boolean {
  return (
    snapshot.source_scope !== "" &&
    snapshot.saved_preference !== "" &&
    snapshot.source_scope !== snapshot.saved_preference
  );
}

/** The form's state before anyone touches it. */
export interface StartFormDraft {
  revisionId: string;
  sourceScope: SourceScope;
  projectId: string;
}

export function startFormDraft(input: {
  savedPreference: SourceScope;
  latestRevisionId: string;
}): StartFormDraft {
  return {
    // The newest revision is the likely one, not the only one: FR-006 allows
    // starting any revision that exists.
    revisionId: input.latestRevisionId,
    // Seeded from the account's stored preference, and changeable — this start
    // is not obliged to repeat the last one.
    sourceScope: input.savedPreference,
    projectId: "",
  };
}
