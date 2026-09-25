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
// - Each dimension is ok with its facts or not computable with a reason
//   (PR 2). A count is a JSON integer; a sum, mean or difference is a string
//   the server wrote, shown as it is and never parsed into a number here. A
//   dimension whose shape is wrong reads as "unknown", and the rest of the
//   report still reads.
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

/** Why a dimension, or one fact of it, cannot be computed (FR-012). */
export const OPDIAG_DIMENSION_REASONS = [
  "no_data", "missing_config", "missing_comparison_window", "missing_observation_window", "no_delivery_channel",
] as const;
export type OpDiagDimensionReason = (typeof OPDIAG_DIMENSION_REASONS)[number];

/** What a gap is missing: R-057's 补录待办 (FR-031). */
export const OPDIAG_GAP_KINDS = [
  "profile_field_pending", "work_unchecked", "work_untagged", "cadence_unset", "observation_unset",
  "published_at_missing", "metric_missing", "account_unresolved", "work_missing", "publication_status_unknown",
  "excerpt_untagged",
] as const;
export type OpDiagGapKind = (typeof OPDIAG_GAP_KINDS)[number];

/** The rule ids a result names; the page translates them (FR-015). */
export const OPDIAG_RULE_IDS = [
  "common.unknown_is_not_zero", "common.no_cross_platform_ranking", "common.no_score",
  "common.window_in_brand_timezone", "consistency.marks_on_older_profile", "coverage.multi_pillar_not_additive",
  "cadence.target_is_brand_channel_level", "cadence.incomplete_week_not_compared",
  "performance.difference_is_not_cause", "performance.samples_taken_at_different_ages",
  "performance.stat_window_mixed", "audience_feedback.tags_not_additive", "execution_flow.no_threshold",
  "roi_reference.shown_as_is", "scope.historical_import_account_unknown",
] as const;
export type OpDiagRuleId = (typeof OPDIAG_RULE_IDS)[number];

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
  kind: OpDiagGapKind | "unknown";
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
    /** The selected dimensions of this section; the brand section has
     *  cadence and execution_flow only. */
    dimensions: Partial<Record<OpDiagDimension, OpDiagDimensionResult>>;
  }[];
  gaps: OpDiagGap[];
  /** Rule ids; the page translates them (FR-015). */
  rules: string[];
  /** Reference keys a judgement may cite (contract §5.8). */
  refs: string[];
  /** The 034 ROI summary a person chose to show (Q6), as the server copied
   *  it: shown as it is, never added to or compared with anything here. */
  roiReference: Record<string, unknown> | null;
}

// ---------------------------------------------------------------- dimensions

/** A sum, mean or difference: the server's reduced value and its display
 *  string, or not computable with a reason. Never parsed into a number. */
export type OpDiagNumber =
  | { status: "ok"; value: string; display: string }
  | { status: "not_computable" | "unknown"; reason: OpDiagDimensionReason | "unknown" };

export interface OpDiagCompleteness {
  expected: number;
  present: number;
  gapKeys: string[];
}

interface OpDiagDimensionCommon {
  completeness: OpDiagCompleteness;
  limits: string[];
  records: { kind: string; id: string }[];
}

export interface OpDiagFactsByDimension {
  consistency: {
    works: number;
    items: { item: string; profileStatus: string; consistent: number; inconsistent: number; unsure: number; unchecked: number }[];
    marksOnOlderRevision: number;
  };
  coverage: { works: number; pillars: { pillar: string; works: number }[]; untagged: number };
  cadence: {
    channels: {
      channel: string;
      target: { set: false } | { set: true; perWeek: number };
      weeks: { isoWeek: string; start: string; complete: boolean; published: number; compared: boolean; met: boolean | null }[];
    }[];
    publishedAtMissing: number;
  };
  performance: {
    /** One group per (platform, metric); there is no total over groups. */
    groups: {
      platform: string;
      metric: string;
      current: OpDiagPerformanceWindow;
      baseline: OpDiagPerformanceWindow;
      change: OpDiagNumber;
      statWindows: string[];
      statWindowMixed: boolean;
    }[];
  };
  audience_feedback: {
    excerpts: number;
    bySource: { platform: string; sourceType: string; excerpts: number }[];
    byTag: { tag: string; excerpts: number }[];
    untagged: number;
  };
  execution_flow: Record<string, unknown>;
}

export interface OpDiagPerformanceWindow {
  publications: number;
  withValue: number;
  unknown: number;
  /** null when no sample had a value: unknown, not 0. */
  sum: string | null;
  mean: OpDiagNumber;
}

export type OpDiagDimensionResult =
  | (OpDiagDimensionCommon & { status: "ok"; facts: OpDiagFactsByDimension[OpDiagDimension] })
  | (OpDiagDimensionCommon & { status: "not_computable"; reason: OpDiagDimensionReason | "unknown" })
  | { status: "unknown" };

const count = z.number().int().nonnegative();

const numberSchema = z.union([
  z.object({ status: z.literal("ok"), value: z.string(), display: z.string() }),
  z.object({ status: z.literal("not_computable"), reason: oneOf(OPDIAG_DIMENSION_REASONS) }),
]);

function toNumber(wire: z.infer<typeof numberSchema>): OpDiagNumber {
  return wire.status === "ok"
    ? { status: "ok", value: wire.value, display: wire.display }
    : { status: "not_computable", reason: wire.reason };
}

const performanceWindowSchema = z.object({
  publications: count, with_value: count, unknown: count, sum: z.string().nullable(), mean: numberSchema,
});

function toPerformanceWindow(wire: z.infer<typeof performanceWindowSchema>): OpDiagPerformanceWindow {
  return {
    publications: wire.publications, withValue: wire.with_value, unknown: wire.unknown, sum: wire.sum,
    mean: toNumber(wire.mean),
  };
}

const factsSchemas = {
  consistency: z.object({
    works: count,
    items: z.array(z.object({
      item: z.string(), profile_status: z.string(), consistent: count, inconsistent: count, unsure: count, unchecked: count,
    })),
    marks_on_older_revision: count,
  }).transform((wire): OpDiagFactsByDimension["consistency"] => ({
    works: wire.works,
    items: wire.items.map((item) => ({
      item: item.item, profileStatus: item.profile_status, consistent: item.consistent,
      inconsistent: item.inconsistent, unsure: item.unsure, unchecked: item.unchecked,
    })),
    marksOnOlderRevision: wire.marks_on_older_revision,
  })),
  coverage: z.object({
    works: count, pillars: z.array(z.object({ pillar: z.string(), works: count })), untagged: count,
  }),
  cadence: z.object({
    channels: z.array(z.object({
      channel: z.string(),
      target: z.object({ set: z.boolean(), per_week: z.number().int().optional() }),
      weeks: z.array(z.object({
        iso_week: z.string(), start: z.string(), complete: z.boolean(), published: count, compared: z.boolean(),
        met: z.boolean().optional(),
      })),
    })),
    published_at_missing: count,
  }).transform((wire): OpDiagFactsByDimension["cadence"] => ({
    channels: wire.channels.map((channel) => ({
      channel: channel.channel,
      // A target that says it is set but has no number is read as not set:
      // never as 0.
      target: channel.target.set && channel.target.per_week !== undefined
        ? { set: true, perWeek: channel.target.per_week }
        : { set: false },
      weeks: channel.weeks.map((week) => ({
        isoWeek: week.iso_week, start: week.start, complete: week.complete, published: week.published,
        compared: week.compared, met: week.compared && week.met !== undefined ? week.met : null,
      })),
    })),
    publishedAtMissing: wire.published_at_missing,
  })),
  performance: z.object({
    groups: z.array(z.object({
      platform: z.string(), metric: z.string(), current: performanceWindowSchema, baseline: performanceWindowSchema,
      change: numberSchema, stat_windows: z.array(z.string()), stat_window_mixed: z.boolean(),
    })),
  }).transform((wire): OpDiagFactsByDimension["performance"] => ({
    groups: wire.groups.map((group) => ({
      platform: group.platform, metric: group.metric,
      current: toPerformanceWindow(group.current), baseline: toPerformanceWindow(group.baseline),
      change: toNumber(group.change), statWindows: group.stat_windows, statWindowMixed: group.stat_window_mixed,
    })),
  })),
  audience_feedback: z.object({
    excerpts: count,
    by_source: z.array(z.object({ platform: z.string(), source_type: z.string(), excerpts: count })),
    by_tag: z.array(z.object({ tag: z.string(), excerpts: count })),
    untagged: count,
  }).transform((wire): OpDiagFactsByDimension["audience_feedback"] => ({
    excerpts: wire.excerpts,
    bySource: wire.by_source.map((row) => ({ platform: row.platform, sourceType: row.source_type, excerpts: row.excerpts })),
    byTag: wire.by_tag,
    untagged: wire.untagged,
  })),
  // The lists are read as the server wrote them; the page shows their
  // counts and ids and nothing is computed from them.
  execution_flow: z.record(z.string(), z.unknown()),
} as const;

const dimensionCommonSchema = z.object({
  completeness: z.object({ expected: count, present: count, gap_keys: z.array(z.string()) }),
  limits: z.array(z.string()),
  records: z.array(z.object({ kind: z.string(), id: z.string() })),
});

const dimensionStatusSchema = z.object({
  status: z.enum(["ok", "not_computable"]),
  reason: oneOf(OPDIAG_DIMENSION_REASONS).optional(),
  facts: z.unknown().optional(),
});

/** Reads one dimension. A shape it does not know is "unknown", never a
 *  zero and never a guess. */
export function parseOpDiagDimension(key: string, data: unknown): OpDiagDimensionResult {
  const schema = (factsSchemas as Record<string, z.ZodType | undefined>)[key];
  const status = dimensionStatusSchema.safeParse(data);
  const common = dimensionCommonSchema.safeParse(data);
  if (!schema || !status.success || !common.success) return { status: "unknown" };
  const shared: OpDiagDimensionCommon = {
    completeness: {
      expected: common.data.completeness.expected, present: common.data.completeness.present,
      gapKeys: common.data.completeness.gap_keys,
    },
    limits: common.data.limits,
    records: common.data.records,
  };
  if (status.data.status === "not_computable") {
    return { ...shared, status: "not_computable", reason: status.data.reason ?? "unknown" };
  }
  const facts = schema.safeParse(status.data.facts);
  if (!facts.success) return { status: "unknown" };
  return { ...shared, status: "ok", facts: facts.data as OpDiagFactsByDimension[OpDiagDimension] };
}

function toDimensions(wire: Record<string, unknown> | null | undefined): Partial<Record<OpDiagDimension, OpDiagDimensionResult>> {
  const out: Partial<Record<OpDiagDimension, OpDiagDimensionResult>> = {};
  for (const key of OPDIAG_DIMENSIONS) {
    if (wire && key in wire) out[key] = parseOpDiagDimension(key, wire[key]);
  }
  return out;
}

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
    kind: oneOf(OPDIAG_GAP_KINDS),
    dimension: text,
    ref: z.object({ kind: z.string(), id: z.string() }),
    account_id: text,
    fix_route: text,
  })).nullable().optional(),
  rules: z.array(z.string()).nullable().optional(),
  refs: z.array(z.string()).nullable().optional(),
  roi_reference: z.record(z.string(), z.unknown()).nullable().optional(),
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
      dimensions: toDimensions(section.dimensions),
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
    roiReference: wire.roi_reference ?? null,
  };
}

/** POST preview: the result computed now and not stored. null when
 *  malformed. */
export function parseOpDiagPreview(data: unknown): OpDiagResult | null {
  const parsed = parseWithFallback<z.infer<typeof resultSchema> | null>(
    data, resultSchema, null, { endpoint: "content-operating-diagnosis/preview" },
  );
  return parsed ? toResult(parsed) : null;
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
