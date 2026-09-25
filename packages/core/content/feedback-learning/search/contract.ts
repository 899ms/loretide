import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Search metrics and ranking observations (specs/036 PR 4), kept in
// feedback-learning.
//
// Four rules this file keeps:
//
// - A search metric's value is nullable. null is "the platform does not show
//   this" and 0 is a confirmed zero; the two are never folded together. A
//   value that is not a non-negative integer reads as null (unknown), never
//   as a number.
// - A metric nobody recorded is absent from the list; the page shows it as
//   unknown, never as 0.
// - Each observation is one look. There is no field that combines two of
//   them, and every one carries rank.single_observation, which the page
//   translates as "单次观察，不代表稳定排名或全平台排名".
// - A value outside a controlled set degrades to "unknown".
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md §4.3, §4.4

/** FR-071: exactly what a platform offers about search. */
export const SEARCH_METRICS = ["search_impression", "search_visit"] as const;
export type SearchMetric = (typeof SEARCH_METRICS)[number];

/** FR-074: what one look found. */
export const RANK_RESULT_KINDS = ["position", "not_found"] as const;
export type RankResultKind = (typeof RANK_RESULT_KINDS)[number];

/** The one entry point in this version. */
export const SEARCH_METRIC_SOURCES = ["manual"] as const;
export type SearchMetricSource = (typeof SEARCH_METRIC_SOURCES)[number];

/** FR-003: people's records only in this version. */
export const SEARCH_OBSERVATION_DATA_ORIGINS = ["manual_only"] as const;
export type SearchObservationDataOrigin = (typeof SEARCH_OBSERVATION_DATA_ORIGINS)[number];

/** FR-075: the rule id every observation carries. */
export const RANK_SINGLE_OBSERVATION_RULE = "rank.single_observation" as const;

/** Builds a path under /api/content-search with each segment encoded. */
export function searchObservationPath(...parts: string[]): string {
  return parts.map(encodeURIComponent).join("/");
}

function oneOf<T extends readonly [string, ...string[]]>(values: T) {
  return z.enum(values).or(z.literal("unknown")).catch("unknown");
}

// Anything but a non-negative integer or null is unknown: the page must
// never show a number nobody recorded.
const nullableCount = z.number().int().nonnegative().nullable().catch(null);
const text = z.string().optional();

export interface SearchMetricRecord {
  searchMetricId: string;
  publicationRecordId: string;
  platform: string;
  accountId: string;
  metric: SearchMetric | "unknown";
  /** null = unknown (the platform does not show it); 0 = confirmed zero. */
  value: number | null;
  unit: string;
  statWindow: string;
  sampledAt: string;
  evidenceNote: string;
  recordedBy: string;
  sourceType: SearchMetricSource | "unknown";
  createdAt: string;
  dataOrigin: SearchObservationDataOrigin | "unknown";
}

const metricSchema = z.object({
  search_metric_id: z.string(),
  publication_record_id: z.string(),
  platform: z.string(),
  account_id: text,
  metric: oneOf(SEARCH_METRICS),
  value: nullableCount.optional(),
  unit: text,
  stat_window: text,
  sampled_at: text,
  evidence_note: text,
  recorded_by: text,
  source_type: oneOf(SEARCH_METRIC_SOURCES),
  created_at: text,
  data_origin: oneOf(SEARCH_OBSERVATION_DATA_ORIGINS),
});

const metricListSchema = z.object({
  publication_record_id: z.string(),
  metrics: z.array(metricSchema).nullable().optional(),
});

function toMetric(wire: z.infer<typeof metricSchema>): SearchMetricRecord {
  return {
    searchMetricId: wire.search_metric_id,
    publicationRecordId: wire.publication_record_id,
    platform: wire.platform,
    accountId: wire.account_id ?? "",
    metric: wire.metric,
    value: wire.value ?? null,
    unit: wire.unit ?? "",
    statWindow: wire.stat_window ?? "",
    sampledAt: wire.sampled_at ?? "",
    evidenceNote: wire.evidence_note ?? "",
    recordedBy: wire.recorded_by ?? "",
    sourceType: wire.source_type,
    createdAt: wire.created_at ?? "",
    dataOrigin: wire.data_origin,
  };
}

/** POST metrics. null when malformed. */
export function parseSearchMetric(data: unknown): SearchMetricRecord | null {
  const parsed = parseWithFallback<z.infer<typeof metricSchema> | null>(
    data, metricSchema, null, { endpoint: "content-search/metric" },
  );
  return parsed ? toMetric(parsed) : null;
}

/** GET metrics?publication_record_id=: every sample, newest first. null when malformed. */
export function parseSearchMetricList(data: unknown): { publicationRecordId: string; metrics: SearchMetricRecord[] } | null {
  const parsed = parseWithFallback<z.infer<typeof metricListSchema> | null>(
    data, metricListSchema, null, { endpoint: "content-search/metrics" },
  );
  if (!parsed) return null;
  return { publicationRecordId: parsed.publication_record_id, metrics: (parsed.metrics ?? []).map(toMetric) };
}

/**
 * What the page shows for one metric of one record: every recorded sample,
 * or "unknown" when there is none. Never 0 for "nobody recorded it".
 */
export function searchMetricSamples(
  metrics: SearchMetricRecord[],
  metric: SearchMetric,
): { status: "unknown" } | { status: "recorded"; samples: SearchMetricRecord[] } {
  const samples = metrics.filter((record) => record.metric === metric);
  return samples.length === 0 ? { status: "unknown" } : { status: "recorded", samples };
}

export interface RankObservation {
  observationId: string;
  revision: number;
  voided: boolean;
  platform: string;
  accountId: string;
  query: string;
  themeId: string;
  publicationRecordId: string;
  observedAt: string;
  conditions: string;
  resultKind: RankResultKind | "unknown";
  /** Set only when resultKind is position. */
  position: number | null;
  /** Set only when resultKind is not_found: how far the person looked. */
  scannedDepth: number | null;
  evidenceNote: string;
  recordedBy: string;
  createdAt: string;
  dataOrigin: SearchObservationDataOrigin | "unknown";
  /** Always rank.single_observation; anything else reads as that too. */
  rule: typeof RANK_SINGLE_OBSERVATION_RULE;
}

const positive = z.number().int().positive().nullable().catch(null);

const observationSchema = z.object({
  observation_id: z.string(),
  revision: z.number().int().positive(),
  voided: z.boolean(),
  platform: z.string(),
  account_id: text,
  query: z.string(),
  theme_id: text,
  publication_record_id: text,
  observed_at: z.string(),
  conditions: text,
  result_kind: oneOf(RANK_RESULT_KINDS),
  position: positive.optional(),
  scanned_depth: positive.optional(),
  evidence_note: text,
  recorded_by: text,
  created_at: text,
  data_origin: oneOf(SEARCH_OBSERVATION_DATA_ORIGINS),
  rule: z.string().optional(),
});

const observationListSchema = z.object({ observations: z.array(observationSchema).nullable().optional() });

function toObservation(wire: z.infer<typeof observationSchema>): RankObservation {
  return {
    observationId: wire.observation_id,
    revision: wire.revision,
    voided: wire.voided,
    platform: wire.platform,
    accountId: wire.account_id ?? "",
    query: wire.query,
    themeId: wire.theme_id ?? "",
    publicationRecordId: wire.publication_record_id ?? "",
    observedAt: wire.observed_at,
    conditions: wire.conditions ?? "",
    resultKind: wire.result_kind,
    position: wire.result_kind === "position" ? (wire.position ?? null) : null,
    scannedDepth: wire.result_kind === "not_found" ? (wire.scanned_depth ?? null) : null,
    evidenceNote: wire.evidence_note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
    dataOrigin: wire.data_origin,
    // One look is one look, whatever the server sent.
    rule: RANK_SINGLE_OBSERVATION_RULE,
  };
}

/** POST rank-observations or a revision. null when malformed. */
export function parseRankObservation(data: unknown): RankObservation | null {
  const parsed = parseWithFallback<z.infer<typeof observationSchema> | null>(
    data, observationSchema, null, { endpoint: "content-search/rank-observation" },
  );
  return parsed ? toObservation(parsed) : null;
}

/** GET rank-observations: one entry per observation, newest first. [] when malformed. */
export function parseRankObservationList(data: unknown): RankObservation[] {
  const parsed = parseWithFallback<z.infer<typeof observationListSchema>>(
    data, observationListSchema, { observations: [] }, { endpoint: "content-search/rank-observations" },
  );
  return (parsed.observations ?? []).map(toObservation);
}

/** The request body of a search metric, in the server's field names. There
 *  is no search_volume, competition or rank: the server refuses them by name. */
export interface SearchMetricInput {
  publication_record_id: string;
  platform: string;
  account_id: string;
  metric: SearchMetric;
  value: number | null;
  unit: string;
  stat_window: string;
  sampled_at: string;
  evidence_note: string;
}

/** The request body of an observation. Exactly one of position and
 *  scanned_depth is a number, the one result_kind asks for. */
export interface RankObservationInput {
  platform: string;
  account_id: string;
  query: string;
  theme_id: string;
  publication_record_id: string;
  observed_at: string;
  conditions: string;
  result_kind: RankResultKind;
  position: number | null;
  scanned_depth: number | null;
  evidence_note: string;
}

/** A revision: the whole observation again, the revision it was based on,
 *  and voided = true to void it. */
export interface RankObservationRevisionInput extends RankObservationInput {
  base_revision: number;
  voided: boolean;
}
