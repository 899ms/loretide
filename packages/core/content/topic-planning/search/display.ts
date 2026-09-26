import type { SearchIntent, SearchTheme, ThemeOrigin } from "./contract";
import type { SearchSuggestionState } from "./suggestion-contract";

export type SearchDisplayValue =
  | "unknown"
  | "learn" | "solve" | "compare" | "buy" | "find" | "unclassified"
  | "manual_keyword" | "customer_question" | "authorized_material"
  | "open" | "adopted" | "adopt_failed" | "adopt_unrecorded" | "abandoned"
  | "base_moved" | "draft_unsaved" | "no_change" | "target_not_found" | "storage"
  | "title" | "body" | "topics" | "description" | "human" | "ai"
  | "no_data_source" | "manual_only";

const INTENTS = new Set<string>(["learn", "solve", "compare", "buy", "find", "unclassified"]);
const ORIGINS = new Set<string>(["manual_keyword", "customer_question", "authorized_material"]);
const STATES = new Set<string>(["open", "adopted", "adopt_failed", "adopt_unrecorded", "abandoned"]);
const FAILURES = new Set<string>(["base_moved", "draft_unsaved", "no_change", "target_not_found", "storage"]);
const ASPECTS = new Set<string>(["title", "body", "topics", "description"]);

function safe<T extends string>(value: string, values: Set<string>): T | "unknown" {
  return (values.has(value) ? value : "unknown") as T | "unknown";
}

export function searchIntentDisplay(value: SearchIntent | string): SearchIntent | "unknown" {
  return safe(value, INTENTS);
}

export function searchOriginDisplay(value: ThemeOrigin | string): ThemeOrigin | "unknown" {
  return safe(value, ORIGINS);
}

export function suggestionStateDisplay(value: SearchSuggestionState | string): SearchSuggestionState | "unknown" {
  return safe(value, STATES);
}

export function suggestionFailureDisplay(value: string | null | undefined): "base_moved" | "draft_unsaved" | "no_change" | "target_not_found" | "storage" | "unknown" {
  return value ? safe(value, FAILURES) : "unknown";
}

export function suggestionAspectDisplay(value: string): "title" | "body" | "topics" | "description" | "unknown" {
  return safe(value, ASPECTS);
}

export function authorKindDisplay(value: string): "human" | "ai" | "unknown" {
  return value === "human" || value === "ai" ? value : "unknown";
}

export function themeUnknownReasonDisplay(theme: SearchTheme): "no_data_source" | "unknown" {
  return theme.searchVolume.reason === "no_data_source" && theme.competition.reason === "no_data_source"
    ? "no_data_source"
    : "unknown";
}

export function searchDataOriginDisplay(value: string): "manual_only" | "unknown" {
  return value === "manual_only" ? "manual_only" : "unknown";
}

export function themeHasQuestionOrKeyword(questions: readonly string[], keywords: readonly string[]): boolean {
  return questions.some((value) => value.trim() !== "") || keywords.some((value) => value.trim() !== "");
}

export function themeAccountMatchesPlatform(themePlatform: string, accountPlatform: string | null | undefined): boolean {
  return !accountPlatform || accountPlatform === themePlatform;
}
