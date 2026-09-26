import type { RankObservation, SearchMetric, SearchMetricRecord } from "./contract";

export type SearchMetricDisplayState = "unrecorded" | "unknown" | "recorded";
export type RankDisplayValue = "position" | "not_found" | "unknown";
export type SearchRuleDisplay = "rank_single_observation" | "unknown";

export function searchMetricValueState(
  samples: readonly SearchMetricRecord[],
  metric: SearchMetric,
): SearchMetricDisplayState {
  const matches = samples.filter((sample) => sample.metric === metric);
  if (matches.length === 0) return "unrecorded";
  return matches.some((sample) => sample.value !== null) ? "recorded" : "unknown";
}

export function searchMetricValueDisplay(value: number | null | undefined): SearchMetricDisplayState {
  if (value === null || value === undefined) return "unknown";
  return "recorded";
}

export function rankResultDisplay(value: RankObservation["resultKind"] | string): RankDisplayValue {
  return value === "position" || value === "not_found" ? value : "unknown";
}

export function rankRuleDisplay(value: string): SearchRuleDisplay {
  return value === "rank.single_observation" ? "rank_single_observation" : "unknown";
}

export function searchMetricDisplay(value: SearchMetric | string): SearchMetric | "unknown" {
  return value === "search_impression" || value === "search_visit" ? value : "unknown";
}
