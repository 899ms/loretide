import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Brand/account operating diagnosis (specs/035 PR 1): report versions and
// work marks.
//
// Named "operating diagnosis" throughout, never "diagnostics": that is the
// development diagnostics panel, and the two are kept apart (FR-090 to
// FR-092).
//
// Three rules this file keeps:
//
// - A stored version is shown as stored. The result is never recomputed on
//   the page, and whether its inputs have moved on since is the server's
//   derivation (inputs_changed). The AI judgement state is always
//   "pending_data": a response that says anything else is read as
//   pending_data, never as a judgement.
// - A value outside a controlled set degrades to "unknown" and is shown as
//   unknown, never as one of the known values.
// - PR 1 computes the scope, the sections and the report-level gaps. A
//   dimension's result is passed through untouched until PR 2 gives it a
//   schema; every number in it will be a string.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md

/** R-057's six dimensions, exactly (FR-003). */
export const OPDIAG_DIMENSIONS = [
  "consistency", "coverage", "cadence", "performance", "audience_feedback", "execution_flow",
] as const;
export type OpDiagDimension = (typeof OPDIAG_DIMENSIONS)[number];

export const OPDIAG_SCOPES = ["account", "brand"] as const;
export type OpDiagScope = (typeof OPDIAG_SCOPES)[number];

export const OPDIAG_MARK_KINDS = ["pillar", "consistency"] as const;
export type OpDiagMarkKind = (typeof OPDIAG_MARK_KINDS)[number];

export const OPDIAG_MARK_VERDICTS = ["tagged", "untagged", "consistent", "inconsistent", "unsure"] as const;
export type OpDiagMarkVerdict = (typeof OPDIAG_MARK_VERDICTS)[number];

/** FR-073: this version has people's records and nothing simulated or fetched. */
export const OPDIAG_DATA_ORIGINS = ["manual_only"] as const;
export type OpDiagDataOrigin = (typeof OPDIAG_DATA_ORIGINS)[number];

export const OPDIAG_SECTIONS = ["account", "unknown_account", "brand"] as const;
export type OpDiagSection = (typeof OPDIAG_SECTIONS)[number];

/** Builds a path under /api/content-operating-diagnosis with each segment encoded. */
export function opdiagPath(...parts: string[]): string {
  return parts.map(encodeURIComponent).join("/");
}

const text = z.string().optional();

function oneOf<T extends readonly [string, ...string[]]>(values: T) {
  return z.enum(values).or(z.literal("unknown")).catch("unknown");
}

// ---------------------------------------------------------------- result

export interface OpDiagScopeAccount {
  accountId: string;
  platform: string;
  displayName: string;
  /** "" when the account has no expression profile revision yet. */
  profileRevisionId: string;
}

export interface OpDiagInputCounts {
  publications: number;
  works: number;
  metrics: number;
  excerpts: number;
  workMarks: number;
  reviews: number;
  deliveryTasks: number;
}

export interface OpDiagGap {
  gapKey: string;
  kind: string;
  /** "" for a report-level gap. */
  dimension: string;
  ref: { kind: string; id: string };
  accountId: string;
  /** A page route id, never a URL. */
  fixRoute: string;
}

export interface OpDiagResult {
  calcVersion: string;
  dataOrigin: OpDiagDataOrigin | "unknown";
  scope: {
    kind: OpDiagScope | "unknown";
    accounts: OpDiagScopeAccount[];
    window: { start: string; end: string; timezone: string };
    comparisonWindow: { start: string; end: string } | null;
    inputCounts: OpDiagInputCounts;
    historicalImportPublications: number;
  };
  sections: {
    section: OpDiagSection | "unknown";
    accountId: string;
    /** Per dimension, untouched until PR 2 gives it a schema. */
    dimensions: Record<string, unknown>;
  }[];
  gaps: OpDiagGap[];
  /** Rule ids; the page translates them (FR-015). */
  rules: string[];
  /** Reference keys a judgement may cite (contract §5.8). */
  refs: string[];
}

const count = z.number().int().nonnegative();

const resultSchema = z.object({
  calc_version: z.string(),
  data_origin: oneOf(OPDIAG_DATA_ORIGINS),
  scope: z.object({
    kind: oneOf(OPDIAG_SCOPES),
    accounts: z.array(z.object({
      account_id: z.string(),
      platform: text,
      display_name: text,
      profile_revision_id: text,
    })).nullable().optional(),
    window: z.object({ start: z.string(), end: z.string(), timezone: text }),
    comparison_window: z.object({ start: z.string(), end: z.string() }).nullable().optional(),
    input_counts: z.object({
      publications: count, works: count, metrics: count, excerpts: count,
      work_marks: count, reviews: count, delivery_tasks: count,
    }),
    historical_import_publications: count.optional(),
  }),
  sections: z.array(z.object({
    section: oneOf(OPDIAG_SECTIONS),
    account_id: text,
    dimensions: z.record(z.string(), z.unknown()).nullable().optional(),
  })).nullable().optional(),
  gaps: z.array(z.object({
    gap_key: z.string(),
    kind: z.string(),
    dimension: text,
    ref: z.object({ kind: z.string(), id: z.string() }),
    account_id: text,
    fix_route: text,
  })).nullable().optional(),
  rules: z.array(z.string()).nullable().optional(),
  refs: z.array(z.string()).nullable().optional(),
});

function toResult(wire: z.infer<typeof resultSchema>): OpDiagResult {
  const counts = wire.scope.input_counts;
  return {
    calcVersion: wire.calc_version,
    dataOrigin: wire.data_origin,
    scope: {
      kind: wire.scope.kind,
      accounts: (wire.scope.accounts ?? []).map((account) => ({
        accountId: account.account_id,
        platform: account.platform ?? "",
        displayName: account.display_name ?? "",
        profileRevisionId: account.profile_revision_id ?? "",
      })),
      window: { start: wire.scope.window.start, end: wire.scope.window.end, timezone: wire.scope.window.timezone ?? "" },
      comparisonWindow: wire.scope.comparison_window ?? null,
      inputCounts: {
        publications: counts.publications, works: counts.works, metrics: counts.metrics, excerpts: counts.excerpts,
        workMarks: counts.work_marks, reviews: counts.reviews, deliveryTasks: counts.delivery_tasks,
      },
      historicalImportPublications: wire.scope.historical_import_publications ?? 0,
    },
    sections: (wire.sections ?? []).map((section) => ({
      section: section.section,
      accountId: section.account_id ?? "",
      dimensions: section.dimensions ?? {},
    })),
    gaps: (wire.gaps ?? []).map((gap) => ({
      gapKey: gap.gap_key,
      kind: gap.kind,
      dimension: gap.dimension ?? "",
      ref: gap.ref,
      accountId: gap.account_id ?? "",
      fixRoute: gap.fix_route ?? "",
    })),
    rules: wire.rules ?? [],
    refs: wire.refs ?? [],
  };
}

// ---------------------------------------------------------------- report versions

export interface OpDiagReportHeader {
  reportId: string;
  versionNo: number;
  scopeKind: OpDiagScope | "unknown";
  accountIds: string[];
  title: string;
  calcVersion: string;
  createdBy: string;
  createdAt: string;
}

export interface OpDiagRecordRef {
  kind: string;
  id: string;
}

/** Derived by the server on each read, never stored (FR-043). */
export interface OpDiagInputsChanged {
  changed: boolean;
  added: OpDiagRecordRef[];
  modified: OpDiagRecordRef[];
  removed: OpDiagRecordRef[];
}

export interface OpDiagReportVersion extends OpDiagReportHeader {
  /** The params as used (contract §4), passed through untouched. */
  params: Record<string, unknown>;
  result: OpDiagResult;
  inputsChanged: OpDiagInputsChanged;
  aiJudgementState: "pending_data";
}

const headerSchema = z.object({
  report_id: z.string(),
  version_no: z.number().int(),
  scope_kind: oneOf(OPDIAG_SCOPES),
  account_ids: z.array(z.string()).nullable().optional(),
  title: text,
  calc_version: z.string(),
  created_by: text,
  created_at: text,
});

const refSchema = z.object({ kind: z.string(), id: z.string() });

const versionSchema = headerSchema.extend({
  params: z.record(z.string(), z.unknown()),
  result: resultSchema,
  inputs_changed: z.object({
    changed: z.boolean(),
    added: z.array(refSchema).nullable().optional(),
    modified: z.array(refSchema).nullable().optional(),
    removed: z.array(refSchema).nullable().optional(),
  }),
  ai_judgement_state: z.literal("pending_data").catch("pending_data"),
});

const reportListSchema = z.object({ reports: z.array(headerSchema).nullable().optional() });

const versionListSchema = z.object({
  report_id: z.string(),
  versions: z.array(headerSchema).nullable().optional(),
});

function toHeader(wire: z.infer<typeof headerSchema>): OpDiagReportHeader {
  return {
    reportId: wire.report_id,
    versionNo: wire.version_no,
    scopeKind: wire.scope_kind,
    accountIds: wire.account_ids ?? [],
    title: wire.title ?? "",
    calcVersion: wire.calc_version,
    createdBy: wire.created_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

/** One stored version (GET, or either POST). null when malformed. */
export function parseOpDiagReportVersion(data: unknown): OpDiagReportVersion | null {
  const parsed = parseWithFallback<z.infer<typeof versionSchema> | null>(
    data, versionSchema, null, { endpoint: "content-operating-diagnosis/report-version" },
  );
  if (!parsed) return null;
  return {
    ...toHeader(parsed),
    params: parsed.params,
    result: toResult(parsed.result),
    inputsChanged: {
      changed: parsed.inputs_changed.changed,
      added: parsed.inputs_changed.added ?? [],
      modified: parsed.inputs_changed.modified ?? [],
      removed: parsed.inputs_changed.removed ?? [],
    },
    aiJudgementState: "pending_data",
  };
}

/** GET reports: the latest version of each report. [] when malformed. */
export function parseOpDiagReportList(data: unknown): OpDiagReportHeader[] {
  const parsed = parseWithFallback<z.infer<typeof reportListSchema>>(
    data, reportListSchema, { reports: [] }, { endpoint: "content-operating-diagnosis/reports" },
  );
  return (parsed.reports ?? []).map(toHeader);
}

/** GET reports/{reportId}/versions. null when malformed. */
export function parseOpDiagReportVersionList(data: unknown): { reportId: string; versions: OpDiagReportHeader[] } | null {
  const parsed = parseWithFallback<z.infer<typeof versionListSchema> | null>(
    data, versionListSchema, null, { endpoint: "content-operating-diagnosis/report-versions" },
  );
  if (!parsed) return null;
  return { reportId: parsed.report_id, versions: (parsed.versions ?? []).map(toHeader) };
}

// ---------------------------------------------------------------- work marks

export interface OpDiagWorkMark {
  markId: string;
  workId: string;
  kind: OpDiagMarkKind | "unknown";
  item: string;
  verdict: OpDiagMarkVerdict | "unknown";
  accountId: string;
  /** The profile revision the server read when the mark was made. */
  profileRevisionId: string;
  note: string;
  recordedBy: string;
  createdAt: string;
}

const markSchema = z.object({
  mark_id: z.string(),
  work_id: z.string(),
  kind: oneOf(OPDIAG_MARK_KINDS),
  item: z.string(),
  verdict: oneOf(OPDIAG_MARK_VERDICTS),
  account_id: text,
  profile_revision_id: text,
  note: text,
  recorded_by: text,
  created_at: text,
});

const markListSchema = z.object({ marks: z.array(markSchema).nullable().optional() });

function toMark(wire: z.infer<typeof markSchema>): OpDiagWorkMark {
  return {
    markId: wire.mark_id,
    workId: wire.work_id,
    kind: wire.kind,
    item: wire.item,
    verdict: wire.verdict,
    accountId: wire.account_id ?? "",
    profileRevisionId: wire.profile_revision_id ?? "",
    note: wire.note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

/** POST work-marks. null when malformed. */
export function parseOpDiagWorkMark(data: unknown): OpDiagWorkMark | null {
  const parsed = parseWithFallback<z.infer<typeof markSchema> | null>(
    data, markSchema, null, { endpoint: "content-operating-diagnosis/work-mark" },
  );
  return parsed ? toMark(parsed) : null;
}

/** GET work-marks: the current mark of each (work, kind, item). [] when malformed. */
export function parseOpDiagWorkMarkList(data: unknown): OpDiagWorkMark[] {
  const parsed = parseWithFallback<z.infer<typeof markListSchema>>(
    data, markListSchema, { marks: [] }, { endpoint: "content-operating-diagnosis/work-marks" },
  );
  return (parsed.marks ?? []).map(toMark);
}
