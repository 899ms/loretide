import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Platform search optimization (specs/036 PR 1): search themes.
//
// Three rules this file keeps:
//
// - Search volume and competition are always unknown in this version, with a
//   reason. The response carries them as {status, reason}; anything else in
//   their place - a number above all - is read as unknown, never shown.
// - A value outside a controlled set degrades to "unknown" and is shown as
//   unknown, never as one of the known values.
// - There is no rank on a theme. Ranking observations belong to
//   feedback-learning (PR 4) and are joined on the page.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

/** Q7: a person's judgement, never inferred (FR-019). */
export const SEARCH_INTENTS = ["learn", "solve", "compare", "buy", "find", "unclassified"] as const;
export type SearchIntent = (typeof SEARCH_INTENTS)[number];

/** R-060: 人工输入关键词、已有客户提问及授权素材. */
export const THEME_ORIGINS = ["manual_keyword", "customer_question", "authorized_material"] as const;
export type ThemeOrigin = (typeof THEME_ORIGINS)[number];

/** FR-003: people's records only in this version. */
export const SEARCH_DATA_ORIGINS = ["manual_only"] as const;
export type SearchDataOrigin = (typeof SEARCH_DATA_ORIGINS)[number];

/** FR-080: why a value is unknown. */
export const SEARCH_UNKNOWN_REASONS = ["no_data_source"] as const;
export type SearchUnknownReason = (typeof SEARCH_UNKNOWN_REASONS)[number];

/** Builds a path under /api/content-search with each segment encoded. */
export function searchPath(...parts: string[]): string {
  return parts.map(encodeURIComponent).join("/");
}

function oneOf<T extends readonly [string, ...string[]]>(values: T) {
  return z.enum(values).or(z.literal("unknown")).catch("unknown");
}

const text = z.string().optional();
const ids = z.array(z.string()).nullable().optional();

/** A value this version cannot know. Only "unknown" is ever shown. */
export interface SearchUnknown {
  status: "unknown";
  reason: SearchUnknownReason | "unknown";
}

// A number, a string or anything else where the unknown object belongs is
// read as unknown: the page must never show a volume nobody measured.
const unknownSchema = z
  .object({
    status: z.literal("unknown").catch("unknown"),
    reason: oneOf(SEARCH_UNKNOWN_REASONS),
  })
  .catch({ status: "unknown", reason: "unknown" });

export interface SearchTheme {
  themeId: string;
  revision: number;
  voided: boolean;
  name: string;
  platform: string;
  /** "" = brand-level, no account. */
  accountId: string;
  businessGoal: string;
  questions: string[];
  keywords: string[];
  intent: SearchIntent | "unknown";
  origin: ThemeOrigin | "unknown";
  originNote: string;
  sourceIds: string[];
  topicCardIds: string[];
  briefRevisionIds: string[];
  note: string;
  recordedBy: string;
  createdAt: string;
  searchVolume: SearchUnknown;
  competition: SearchUnknown;
  dataOrigin: SearchDataOrigin | "unknown";
}

const themeSchema = z.object({
  theme_id: z.string(),
  revision: z.number().int().positive(),
  voided: z.boolean(),
  name: z.string(),
  platform: z.string(),
  account_id: text,
  business_goal: text,
  questions: ids,
  keywords: ids,
  intent: oneOf(SEARCH_INTENTS),
  origin: oneOf(THEME_ORIGINS),
  origin_note: text,
  source_ids: ids,
  topic_card_ids: ids,
  brief_revision_ids: ids,
  note: text,
  recorded_by: text,
  created_at: text,
  search_volume: unknownSchema,
  competition: unknownSchema,
  data_origin: oneOf(SEARCH_DATA_ORIGINS),
});

const themeListSchema = z.object({ themes: z.array(themeSchema).nullable().optional() });

const revisionListSchema = z.object({
  theme_id: z.string(),
  revisions: z.array(themeSchema).nullable().optional(),
});

function toTheme(wire: z.infer<typeof themeSchema>): SearchTheme {
  return {
    themeId: wire.theme_id,
    revision: wire.revision,
    voided: wire.voided,
    name: wire.name,
    platform: wire.platform,
    accountId: wire.account_id ?? "",
    businessGoal: wire.business_goal ?? "",
    questions: wire.questions ?? [],
    keywords: wire.keywords ?? [],
    intent: wire.intent,
    origin: wire.origin,
    originNote: wire.origin_note ?? "",
    sourceIds: wire.source_ids ?? [],
    topicCardIds: wire.topic_card_ids ?? [],
    briefRevisionIds: wire.brief_revision_ids ?? [],
    note: wire.note ?? "",
    recordedBy: wire.recorded_by ?? "",
    createdAt: wire.created_at ?? "",
    searchVolume: wire.search_volume,
    competition: wire.competition,
    dataOrigin: wire.data_origin,
  };
}

/** GET themes/{themeId}, or either POST. null when malformed. */
export function parseSearchTheme(data: unknown): SearchTheme | null {
  const parsed = parseWithFallback<z.infer<typeof themeSchema> | null>(
    data, themeSchema, null, { endpoint: "content-search/theme" },
  );
  return parsed ? toTheme(parsed) : null;
}

/** GET themes: the current revision of each theme. [] when malformed. */
export function parseSearchThemeList(data: unknown): SearchTheme[] {
  const parsed = parseWithFallback<z.infer<typeof themeListSchema>>(
    data, themeListSchema, { themes: [] }, { endpoint: "content-search/themes" },
  );
  return (parsed.themes ?? []).map(toTheme);
}

/** GET themes/{themeId}/revisions: newest first. null when malformed. */
export function parseSearchThemeRevisions(data: unknown): { themeId: string; revisions: SearchTheme[] } | null {
  const parsed = parseWithFallback<z.infer<typeof revisionListSchema> | null>(
    data, revisionListSchema, null, { endpoint: "content-search/theme-revisions" },
  );
  if (!parsed) return null;
  return { themeId: parsed.theme_id, revisions: (parsed.revisions ?? []).map(toTheme) };
}

/** The request body of a create, in the server's field names. There is no
 *  search_volume, competition, rank, scope or budget: the server refuses
 *  them by name (FR-002, FR-014). */
export interface SearchThemeInput {
  name: string;
  platform: string;
  account_id: string;
  business_goal: string;
  questions: string[];
  keywords: string[];
  intent: SearchIntent;
  origin: ThemeOrigin;
  origin_note: string;
  source_ids: string[];
  topic_card_ids: string[];
  brief_revision_ids: string[];
  note: string;
}

/** A revision: the whole content again, the revision it was based on, and
 *  voided = true to archive. */
export interface SearchThemeRevisionInput extends SearchThemeInput {
  base_revision: number;
  voided: boolean;
}
