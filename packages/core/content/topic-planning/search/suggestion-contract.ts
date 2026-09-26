import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

export const SUGGESTION_ASPECTS = ["title", "body", "topics", "description"] as const;
export const SUGGESTION_AUTHOR_KINDS = ["human"] as const;
export const SUGGESTION_DECISIONS = ["adopt", "abandon"] as const;
export const SUGGESTION_EFFECT_OUTCOMES = ["done", "failed"] as const;
export const SUGGESTION_EFFECT_FAILURES = ["base_moved", "draft_unsaved", "no_change", "target_not_found", "storage"] as const;
export const SUGGESTION_STATES = ["open", "adopted", "adopt_failed", "adopt_unrecorded", "abandoned"] as const;

function oneOf<T extends readonly [string, ...string[]]>(values: T) {
  return z.enum(values).or(z.literal("unknown")).catch("unknown");
}
const revision = z.number().int().positive();
const failure = z.union([z.literal(""), oneOf(SUGGESTION_EFFECT_FAILURES)]);
const diffSchema = z.object({
  ops: z.array(z.object({ op: oneOf(["equal", "delete", "insert"] as const), text: z.string() })),
  inserted_lines: z.number().int().nonnegative(),
  deleted_lines: z.number().int().nonnegative(),
}).transform((wire) => ({ ops: wire.ops, insertedLines: wire.inserted_lines, deletedLines: wire.deleted_lines }));

const decisionSchema = z.object({
  decision_id: z.string(), suggestion_id: z.string(), suggestion_revision: revision,
  decision: oneOf(SUGGESTION_DECISIONS), note: z.string(), decided_by: z.string(), created_at: z.string(),
}).transform((wire) => ({
  decisionId: wire.decision_id, suggestionId: wire.suggestion_id, suggestionRevision: wire.suggestion_revision,
  decision: wire.decision, note: wire.note, decidedBy: wire.decided_by, createdAt: wire.created_at,
}));

const effectSchema = z.object({
  effect_id: z.string(), decision_id: z.string(), outcome: oneOf(SUGGESTION_EFFECT_OUTCOMES),
  version_id: z.string(), failure_code: failure, created_at: z.string(),
}).transform((wire) => ({
  effectId: wire.effect_id, decisionId: wire.decision_id, outcome: wire.outcome,
  versionId: wire.version_id, failureCode: wire.failure_code, createdAt: wire.created_at,
}));

const suggestionSchema = z.object({
  suggestion_id: z.string(), revision,
  work_id: z.string(), artifact_id: z.string(), base_version_id: z.string(), theme_id: z.string(),
  theme_revision: revision, target_question: z.string(), aspects: z.array(oneOf(SUGGESTION_ASPECTS)),
  rationale: z.string(), evidence_source_ids: z.array(z.string()).nullable(), proposed_body: z.string(),
  author_kind: oneOf(SUGGESTION_AUTHOR_KINDS), recorded_by: z.string(), created_at: z.string(),
  state: oneOf(SUGGESTION_STATES), failure_code: failure.optional(),
  base_is_current: z.boolean(), theme_changed: z.boolean(), diff: diffSchema.nullable().optional(),
  decision: decisionSchema.nullable(), effects: z.array(effectSchema).nullable(),
}).transform((wire) => ({
  suggestionId: wire.suggestion_id, revision: wire.revision,
  workId: wire.work_id, artifactId: wire.artifact_id, baseVersionId: wire.base_version_id, themeId: wire.theme_id,
  themeRevision: wire.theme_revision, targetQuestion: wire.target_question, aspects: wire.aspects,
  rationale: wire.rationale, evidenceSourceIds: wire.evidence_source_ids ?? [], proposedBody: wire.proposed_body,
  authorKind: wire.author_kind, recordedBy: wire.recorded_by, createdAt: wire.created_at,
  state: wire.state, failureCode: wire.failure_code ?? "", baseIsCurrent: wire.base_is_current,
  themeChanged: wire.theme_changed, diff: wire.diff ?? null, decision: wire.decision, effects: wire.effects ?? [],
}));

export type SearchSuggestion = z.output<typeof suggestionSchema>;
export type SearchSuggestionState = (typeof SUGGESTION_STATES)[number];
export type SearchSuggestionDiff = z.output<typeof diffSchema>;
export type SearchSuggestionComparison = { suggestions: SearchSuggestion[]; sameBase: boolean };

/** The server owns state, decision and effect facts. Never infer adoption from a write attempt. */
export function parseSearchSuggestion(data: unknown): SearchSuggestion | null {
  return parseWithFallback<SearchSuggestion | null>(data, suggestionSchema, null, { endpoint: "content-search/suggestion" });
}
export function parseSearchSuggestionList(data: unknown): SearchSuggestion[] {
  const schema = z.object({ suggestions: z.array(suggestionSchema).nullable() });
  return parseWithFallback<{ suggestions: SearchSuggestion[] | null }>(data, schema, { suggestions: [] }, { endpoint: "content-search/suggestions" }).suggestions ?? [];
}
export function parseSearchSuggestionComparison(data: unknown): SearchSuggestionComparison | null {
  const schema = z.object({ suggestions: z.array(suggestionSchema), same_base: z.boolean() })
    .transform((wire) => ({ suggestions: wire.suggestions, sameBase: wire.same_base }));
  return parseWithFallback<SearchSuggestionComparison | null>(data, schema, null, { endpoint: "content-search/suggestions/compare" });
}

export interface SearchSuggestionContentInput {
  target_question: string;
  aspects: (typeof SUGGESTION_ASPECTS)[number][];
  rationale: string;
  evidence_source_ids: string[];
  proposed_body: string;
}
export interface SearchSuggestionInput extends SearchSuggestionContentInput {
  work_id: string;
  artifact_id: string;
  base_version_id: string;
  theme_id: string;
}
/** Targets stay immutable; revisions send only content and the concurrency token. */
export interface SearchSuggestionRevisionInput extends SearchSuggestionContentInput { base_revision: number }
/** Adoption is deliberately unavailable until the ApplyBody stage. */
export interface SearchSuggestionAbandonInput { decision: "abandon"; revision: number; note: string }
