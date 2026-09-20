import {
  EXCERPT_SOURCES,
  METRIC_NAMES,
  METRIC_PLATFORMS,
} from "./contract";
import { parsePastedMetrics, type CsvProblem, type CsvRow } from "./csv";
import type { RecordExcerptInput, RecordMetricInput } from "./queries";

// What the metric and excerpt forms consider submittable, and how a pasted
// block reads before it is sent.
//
// These are rules, not layout, so they live here with node tests rather than
// inside JSX. The page asks `canSubmit*` whether the button is live and
// `*Problem` which field to point at; it never decides either for itself.
//
// The rules mirror `server/internal/content/feedback-learning/states.go`
// deliberately: a form that lets you press a button the server will refuse has
// taught you nothing except that the product is unreliable. `form.test.ts`
// reads the Go source for the three limits so the two cannot drift apart
// silently.
//
// The one thing this file must never do: turn a blank value cell into 0.
// `Number("")` is 0 in JavaScript, so every path below that could touch an
// empty string is written to produce `null` instead. SOP 10.1: "未知填空；0 只
// 表示已确认的零值".

/** Free text note limit, in runes. Mirrors MaxNoteRunes. */
export const MAX_NOTE_RUNES = 20000;
/** Unit, window and tag limit, in runes. Mirrors MaxShortRunes. */
export const MAX_SHORT_RUNES = 500;
/** How many tags one excerpt carries. Mirrors MaxTags. */
export const MAX_TAGS = 50;

/**
 * Counts characters the way the server does.
 *
 * Go counts runes; `String.length` counts UTF-16 units, so an emoji is 2 and a
 * CJK character is 1. Spreading the string iterates code points, which is what
 * a rune is. Counting bytes - which is what a naive limit does - would give
 * Chinese a third of the room English gets for the same rule.
 */
export function countRunes(text: string): number {
  return [...text].length;
}

// RFC 3339, the format `time.Parse(time.RFC3339, …)` accepts. Checked with a
// pattern rather than `Date.parse`, which also accepts "2026-09-25" and
// "September 25 2026" - both of which the server refuses, so accepting them
// here would just move the refusal later.
const RFC3339 = /^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$/;

export function isTimestamp(text: string): boolean {
  if (!RFC3339.test(text)) return false;
  return !Number.isNaN(Date.parse(text));
}

export type ProblemReason =
  | "missing"
  | "outside-the-set"
  | "not-a-number"
  | "not-a-timestamp"
  | "too-long"
  | "too-many";

/** Which field is not ready, and why. The page points at the field; it does
 *  not compose a sentence out of this. */
export interface DraftProblem {
  field: string;
  reason: ProblemReason;
}

// ------------------------------------------------------------------ metric

/**
 * One metric observation as the form holds it.
 *
 * `value` is the string the person typed, not a number. That is the whole
 * point: "" is a blank they left on purpose and it has to stay
 * distinguishable from "0" all the way to the request body. A `number` field
 * here would have needed a sentinel, and every sentinel eventually gets read
 * as a measurement.
 */
export interface MetricDraft {
  publicationRecordId: string;
  platform: string;
  accountId: string;
  metric: string;
  value: string;
  unit: string;
  statWindow: string;
  sampledAt: string;
  evidenceNote: string;
}

export function emptyMetricDraft(publicationRecordId = ""): MetricDraft {
  return {
    publicationRecordId,
    platform: "",
    accountId: "",
    metric: "",
    value: "",
    unit: "",
    statWindow: "",
    sampledAt: "",
    evidenceNote: "",
  };
}

/**
 * Reads what was typed into the value box.
 *
 * Blank is `null` - unknown - and "0" is 0. A string that is neither is
 * reported rather than silently becoming NaN or 0.
 */
export function parseValueInput(raw: string): { ok: true; value: number | null } | { ok: false } {
  const text = raw.trim();
  if (text === "") return { ok: true, value: null };
  const parsed = Number(text.replace(/,/g, ""));
  if (!Number.isFinite(parsed)) return { ok: false };
  return { ok: true, value: parsed };
}

export function metricDraftProblem(draft: MetricDraft): DraftProblem | null {
  if (!draft.publicationRecordId) return { field: "publication_record_id", reason: "missing" };
  if (!draft.platform) return { field: "platform", reason: "missing" };
  if (!(METRIC_PLATFORMS as readonly string[]).includes(draft.platform)) {
    return { field: "platform", reason: "outside-the-set" };
  }
  if (!draft.metric) return { field: "metric", reason: "missing" };
  if (!(METRIC_NAMES as readonly string[]).includes(draft.metric)) {
    return { field: "metric", reason: "outside-the-set" };
  }
  if (!draft.sampledAt) return { field: "sampled_at", reason: "missing" };
  if (!isTimestamp(draft.sampledAt)) return { field: "sampled_at", reason: "not-a-timestamp" };
  // A blank value is fine and is the reason this check is not "required".
  if (!parseValueInput(draft.value).ok) return { field: "value", reason: "not-a-number" };
  for (const [field, text] of [
    ["account_id", draft.accountId],
    ["unit", draft.unit],
    ["stat_window", draft.statWindow],
  ] as const) {
    if (countRunes(text) > MAX_SHORT_RUNES) return { field, reason: "too-long" };
  }
  if (countRunes(draft.evidenceNote) > MAX_NOTE_RUNES) {
    return { field: "evidence_note", reason: "too-long" };
  }
  return null;
}

export function canSubmitMetricDraft(draft: MetricDraft): boolean {
  return metricDraftProblem(draft) === null;
}

/**
 * The form draft as the record endpoint takes it.
 *
 * There is no `sourceType` here and there must not be: which endpoint is
 * called is what the server writes as the origin.
 */
export function metricDraftToInput(draft: MetricDraft): RecordMetricInput {
  const parsed = parseValueInput(draft.value);
  return {
    publicationRecordId: draft.publicationRecordId,
    platform: draft.platform,
    accountId: draft.accountId,
    metric: draft.metric,
    // A draft that does not parse never gets here - the button is dead - but
    // if it did, `null` is the honest answer. Not 0.
    value: parsed.ok ? parsed.value : null,
    unit: draft.unit,
    statWindow: draft.statWindow,
    sampledAt: draft.sampledAt,
    evidenceNote: draft.evidenceNote,
  };
}

// ----------------------------------------------------------------- excerpt

export interface ExcerptDraft {
  publicationRecordId: string;
  sourceType: string;
  /** Redacted by the person typing it. Nothing redacts for them. */
  redactedExcerpt: string;
  /** Their own reading of it, kept in its own field (R-045). */
  interpretation: string;
  /** What is in the tag box, before it is split. */
  tagInput: string;
  occurredAt: string;
}

export function emptyExcerptDraft(publicationRecordId = ""): ExcerptDraft {
  return {
    publicationRecordId,
    sourceType: "",
    redactedExcerpt: "",
    interpretation: "",
    tagInput: "",
    occurredAt: "",
  };
}

/** Splits a tag box into tags on commas - ASCII and full-width - dropping
 *  blanks and repeats while keeping the order they were typed in. */
export function parseTagInput(text: string): string[] {
  const seen = new Set<string>();
  const tags: string[] = [];
  for (const part of text.split(/[,，]/)) {
    const tag = part.trim();
    if (!tag || seen.has(tag)) continue;
    seen.add(tag);
    tags.push(tag);
  }
  return tags;
}

export function excerptDraftProblem(draft: ExcerptDraft): DraftProblem | null {
  if (!draft.publicationRecordId) return { field: "publication_record_id", reason: "missing" };
  if (!draft.sourceType) return { field: "source_type", reason: "missing" };
  if (!(EXCERPT_SOURCES as readonly string[]).includes(draft.sourceType)) {
    return { field: "source_type", reason: "outside-the-set" };
  }
  if (!draft.occurredAt) return { field: "occurred_at", reason: "missing" };
  if (!isTimestamp(draft.occurredAt)) return { field: "occurred_at", reason: "not-a-timestamp" };
  // Either field alone is an ordinary row - a quote nobody has an opinion
  // about, an observation not worth quoting anyone for - but a row with
  // neither records nothing at all.
  if (!draft.redactedExcerpt.trim() && !draft.interpretation.trim()) {
    return { field: "redacted_excerpt", reason: "missing" };
  }
  if (countRunes(draft.redactedExcerpt) > MAX_NOTE_RUNES) {
    return { field: "redacted_excerpt", reason: "too-long" };
  }
  if (countRunes(draft.interpretation) > MAX_NOTE_RUNES) {
    return { field: "interpretation", reason: "too-long" };
  }
  const tags = parseTagInput(draft.tagInput);
  if (tags.length > MAX_TAGS) return { field: "tags", reason: "too-many" };
  if (tags.some((tag) => countRunes(tag) > MAX_SHORT_RUNES)) {
    return { field: "tags", reason: "too-long" };
  }
  return null;
}

export function canSubmitExcerptDraft(draft: ExcerptDraft): boolean {
  return excerptDraftProblem(draft) === null;
}

export function excerptDraftToInput(draft: ExcerptDraft): RecordExcerptInput {
  return {
    publicationRecordId: draft.publicationRecordId,
    sourceType: draft.sourceType,
    redactedExcerpt: draft.redactedExcerpt,
    interpretation: draft.interpretation,
    tags: parseTagInput(draft.tagInput),
    occurredAt: draft.occurredAt,
  };
}

// --------------------------------------------------------------- csv paste

/**
 * What the preview under the paste box shows.
 *
 * `unknown` is counted and shown BEFORE the import, because that is the only
 * moment a person can still tell us we read their blanks wrong. Finding out
 * afterwards that forty rows landed as zero is finding out too late.
 */
export interface CsvPreview {
  rows: CsvRow[];
  /** How many parsed rows carry no number. */
  unknown: number;
  /** The first bad row, or null. Parsing stops there: all or nothing. */
  problem: CsvProblem | null;
  /** True when there is at least one row and nothing is wrong. */
  canImport: boolean;
  /** True when the box is empty - which is not an error, just nothing yet. */
  empty: boolean;
}

export function csvPreview(text: string): CsvPreview {
  const parsed = parsePastedMetrics(text);
  if (!parsed.ok) {
    return { rows: [], unknown: 0, problem: parsed.problem, canImport: false, empty: false };
  }
  return {
    rows: parsed.rows,
    unknown: parsed.rows.filter((row) => row.value === null).length,
    problem: null,
    canImport: parsed.rows.length > 0,
    empty: parsed.rows.length === 0,
  };
}
