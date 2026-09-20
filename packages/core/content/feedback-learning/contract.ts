import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Manual metrics and feedback excerpts (SOP 10.1, 7.1).
//
// The controlled sets are the server's; they are restated here as literal
// unions so a component can switch on them, and `contract.test.ts` holds them
// to the Go source rather than to my memory of it.
//
// The one thing to get right in this file: a metric value of `null` means the
// platform does not show that number, and `0` means somebody checked and it is
// zero. SOP 10.1: "未知填空；0 只表示已确认的零值". Every default, every
// fallback and every render path below keeps the two apart - a `?? 0` anywhere
// in here would put a number nobody observed into whatever reads it next.
//
// Contract: specs/027-feedback-manual/contracts/feedback-manual.md

/** The same four channels 025 delivers to. */
export const METRIC_PLATFORMS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;
export type MetricPlatform = (typeof METRIC_PLATFORMS)[number];

/** SOP 10.1's eleven names, exactly. There is no "other": the SOP named these
 *  and stopped, and an escape hatch would turn the set into a suggestion.
 *  `read` and `play` are separate and stay separate. */
export const METRIC_NAMES = [
  "impression", "read", "play", "completion", "like", "comment",
  "favorite", "share", "follow", "direct_message", "conversion",
] as const;
export type MetricName = (typeof METRIC_NAMES)[number];

/** 10.1's "数据来源类型". Written by the server from the endpoint used, never
 *  sent in a request body. */
export const METRIC_SOURCES = ["manual", "csv_import"] as const;
export type MetricSource = (typeof METRIC_SOURCES)[number];

/** 10.1's three: 评论、私信、线索. No "other". */
export const EXCERPT_SOURCES = ["comment", "private_message", "lead"] as const;
export type ExcerptSource = (typeof EXCERPT_SOURCES)[number];

/** SOP 7.1's AI review row, all seven values. This phase only ever sees
 *  `pending_data`; the rest are reserved so EP-08 does not have to widen the
 *  set later. */
export const REVIEW_STATES = [
  "pending_data", "queued", "generating", "generated", "failed",
  "edited", "superseded",
] as const;
export type ReviewState = (typeof REVIEW_STATES)[number];

export interface ManualMetric {
  manualMetricId: string;
  workspaceId: string;
  publicationRecordId: string;
  platform: string;
  accountId: string;
  metric: string;
  /** null is "the platform does not show me this". 0 is "I looked, it is
   *  zero". Never coerce one into the other. */
  value: number | null;
  unit: string;
  /** 10.1's 统计窗口, free text: "发布后 14 天累计" cannot be written as a
   *  start and an end. */
  statWindow: string;
  sampledAt: string;
  recordedBy: string;
  evidenceNote: string;
  sourceType: string;
  createdAt: string;
  /** Resolved by the server on read, two hops. "" when it cannot be resolved -
   *  a record entered for history has no delivery task behind it - and that is
   *  a real answer, not an error. */
  versionId: string;
}

export interface FeedbackExcerpt {
  feedbackExcerptId: string;
  workspaceId: string;
  publicationRecordId: string;
  sourceType: string;
  /** What somebody else said, redacted BY A PERSON. Nothing redacts for them. */
  redactedExcerpt: string;
  /** What the operator makes of it. Kept apart so a later reader can still
   *  tell the evidence from the judgement (R-045). */
  interpretation: string;
  tags: string[];
  occurredAt: string;
  recordedBy: string;
  createdAt: string;
}

/** One publication record nobody has recorded numbers for: SOP 2's fifth
 *  workbench item. Derived by the server, stored nowhere. */
export interface PendingFeedback {
  publicationRecordId: string;
  workId: string;
  artifactId: string;
  channel: string;
  status: string;
  /** null when 025's publication record carried no publication time. A record
   *  with no time cannot have a window that elapsed, so the server answers
   *  `due: "unknown"` for it and keeps it on the list. */
  publishedAt: string | null;
  createdAt: string;
  /**
   * Why this row is here, as the server derived it from SOP 3.2's
   * 反馈观察时点 (specs/029).
   *
   * "passed" - the brand's observation window elapsed.
   * "unknown" - nobody set a window, or there is no publication time, so
   *   "has it been long enough" has no answer. The record is listed anyway.
   * "" - this build got no answer: an older backend, or a value it has not
   *   heard of. The page says nothing rather than guessing, because "passed"
   *   and "unknown" are different sentences and neither is safe to invent.
   *
   * "not_yet" never appears: a record whose window has not elapsed is not on
   * this list at all.
   */
  due: string;
}

// Lenient by design: an installed client talks to whatever backend is
// deployed, and a value this build has not heard of must not blank the page.
// Every controlled value stays `z.string()` for that reason - what the page
// may WRITE is the union above; what it may READ is wider.
const metricSchema = z.object({
  manual_metric_id: z.string(),
  workspace_id: z.string().optional(),
  publication_record_id: z.string().optional(),
  platform: z.string().optional(),
  account_id: z.string().optional(),
  metric: z.string().optional(),
  // Nullable AND optional, and the two are not the same: null is an unknown
  // the server sent, absent is a field this build did not get. Both read back
  // as null, because neither is a number.
  value: z.number().nullable().optional(),
  unit: z.string().optional(),
  stat_window: z.string().optional(),
  sampled_at: z.string().optional(),
  recorded_by: z.string().optional(),
  evidence_note: z.string().optional(),
  source_type: z.string().optional(),
  created_at: z.string().optional(),
  version_id: z.string().optional(),
});

const excerptSchema = z.object({
  feedback_excerpt_id: z.string(),
  workspace_id: z.string().optional(),
  publication_record_id: z.string().optional(),
  source_type: z.string().optional(),
  redacted_excerpt: z.string().optional(),
  interpretation: z.string().optional(),
  tags: z.array(z.string()).nullable().optional(),
  occurred_at: z.string().optional(),
  recorded_by: z.string().optional(),
  created_at: z.string().optional(),
});

const pendingSchema = z.object({
  publication_record_id: z.string(),
  work_id: z.string().optional(),
  artifact_id: z.string().optional(),
  channel: z.string().optional(),
  status: z.string().optional(),
  published_at: z.string().nullable().optional(),
  created_at: z.string().optional(),
  // Optional AND `.catch`, which its siblings are not, because this field is
  // the one specs/029 added: a list that worked before it existed must not
  // start collapsing because one row's `due` came back the wrong shape. The
  // row still names a real publication record waiting for numbers, and that
  // is the part the workbench needs. Absent or unusable both read as "no
  // answer" - never as "the window passed".
  due: z.string().catch("").optional(),
});

const metricListSchema = z.object({ metrics: z.array(metricSchema).nullable().optional() });
const excerptListSchema = z.object({
  excerpts: z.array(excerptSchema).nullable().optional(),
  review_state: z.string().optional(),
});
const pendingListSchema = z.object({
  pending: z.array(pendingSchema).nullable().optional(),
  count: z.number().optional(),
});

const EMPTY_METRIC: ManualMetric = {
  manualMetricId: "", workspaceId: "", publicationRecordId: "", platform: "",
  accountId: "", metric: "",
  // Not 0. A build that could not parse the response has not learned that the
  // number is zero.
  value: null,
  unit: "", statWindow: "", sampledAt: "", recordedBy: "", evidenceNote: "",
  sourceType: "", createdAt: "", versionId: "",
};

function toMetric(wire: z.infer<typeof metricSchema>): ManualMetric {
  return {
    manualMetricId: wire.manual_metric_id,
    workspaceId: wire.workspace_id ?? "",
    publicationRecordId: wire.publication_record_id ?? "",
    platform: wire.platform ?? "",
    accountId: wire.account_id ?? "",
    metric: wire.metric ?? "",
    // `?? null`, never `?? 0`. Absent and null both mean "no number here".
    value: wire.value ?? null,
    unit: wire.unit ?? "",
    statWindow: wire.stat_window ?? "",
    sampledAt: wire.sampled_at ?? "",
    recordedBy: wire.recorded_by ?? "",
    evidenceNote: wire.evidence_note ?? "",
    sourceType: wire.source_type ?? "",
    createdAt: wire.created_at ?? "",
    versionId: wire.version_id ?? "",
  };
}

function toExcerpt(wire: z.infer<typeof excerptSchema>): FeedbackExcerpt {
  return {
    feedbackExcerptId: wire.feedback_excerpt_id,
    workspaceId: wire.workspace_id ?? "",
    publicationRecordId: wire.publication_record_id ?? "",
    sourceType: wire.source_type ?? "",
    redactedExcerpt: wire.redacted_excerpt ?? "",
    interpretation: wire.interpretation ?? "",
    // null and absent both become []. A caller should not have to know which
    // of the two it received before mapping over it.
    tags: wire.tags ?? [],
    occurredAt: wire.occurred_at ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toPending(wire: z.infer<typeof pendingSchema>): PendingFeedback {
  return {
    publicationRecordId: wire.publication_record_id,
    workId: wire.work_id ?? "",
    artifactId: wire.artifact_id ?? "",
    channel: wire.channel ?? "",
    status: wire.status ?? "",
    publishedAt: wire.published_at ?? null,
    createdAt: wire.created_at ?? "",
    // `?? ""`, never `?? "passed"`. Absent means this build was not told.
    due: wire.due ?? "",
  };
}

export function parseManualMetric(data: unknown): ManualMetric {
  const parsed = parseWithFallback(data, metricSchema, { manual_metric_id: "" }, {
    endpoint: "content-metrics/detail",
  });
  return parsed.manual_metric_id ? toMetric(parsed) : EMPTY_METRIC;
}

export function parseManualMetrics(data: unknown): ManualMetric[] {
  const parsed = parseWithFallback(data, metricListSchema, { metrics: [] }, {
    endpoint: "content-metrics/list",
  });
  return (parsed.metrics ?? []).map(toMetric);
}

export function parseFeedbackExcerpt(data: unknown): FeedbackExcerpt {
  const parsed = parseWithFallback(data, excerptSchema, { feedback_excerpt_id: "" }, {
    endpoint: "content-feedback/detail",
  });
  return toExcerpt(parsed);
}

export interface FeedbackList {
  excerpts: FeedbackExcerpt[];
  /** SOP 7.1's AI review state. This phase only ever gets `pending_data`; a
   *  build that gets something else shows what it got rather than pretending. */
  reviewState: string;
}

export function parseFeedbackList(data: unknown): FeedbackList {
  const parsed = parseWithFallback<z.infer<typeof excerptListSchema>>(
    data,
    excerptListSchema,
    { excerpts: [], review_state: "pending_data" },
    { endpoint: "content-feedback/list" },
  );
  return {
    excerpts: (parsed.excerpts ?? []).map(toExcerpt),
    // A response that did not say degrades to pending_data: claiming a report
    // exists when we do not know is the direction that misleads.
    reviewState: parsed.review_state ?? "pending_data",
  };
}

export function parsePendingFeedback(data: unknown): PendingFeedback[] {
  const parsed = parseWithFallback(data, pendingListSchema, { pending: [] }, {
    endpoint: "content-feedback/pending",
  });
  return (parsed.pending ?? []).map(toPending);
}

/**
 * How a metric value should read.
 *
 * An absent value is NOT "0" and NOT "". It is unknown and says so, because a
 * blank cell in a report gets read as zero by the next person along. This is
 * the function a page uses instead of `String(value ?? "")`.
 */
export function describeMetricValue(value: number | null): { known: boolean; text: string } {
  if (value === null) return { known: false, text: "unknown" };
  return { known: true, text: String(value) };
}

/** Whether two metric values are the same observation. nil never equals 0. */
export function sameMetricValue(left: number | null, right: number | null): boolean {
  if (left === null || right === null) return left === null && right === null;
  return left === right;
}
