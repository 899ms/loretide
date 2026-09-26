import type { OpDiagDimensionResult, OpDiagNumber } from "./contract";

export type OpDiagDisplayKey =
  | "status.ok"
  | "status.not_computable"
  | "status.unknown"
  | "reason.no_data"
  | "reason.missing_config"
  | "reason.missing_comparison_window"
  | "reason.missing_observation_window"
  | "reason.no_delivery_channel"
  | "reason.unknown"
  | "reason.default"
  | "kind.account"
  | "kind.unknown_account"
  | "kind.brand"
  | "kind.unknown"
  | "gap.profile_field_pending"
  | "gap.work_unchecked"
  | "gap.work_untagged"
  | "gap.cadence_unset"
  | "gap.observation_unset"
  | "gap.published_at_missing"
  | "gap.metric_missing"
  | "gap.account_unresolved"
  | "gap.work_missing"
  | "gap.publication_status_unknown"
  | "gap.excerpt_untagged"
  | "rule.common.unknown_is_not_zero"
  | "rule.common.no_cross_platform_ranking"
  | "rule.common.no_score"
  | "rule.common.window_in_brand_timezone"
  | "rule.performance.difference_is_not_cause"
  | "rule.consistency.marks_on_older_profile"
  | "rule.coverage.multi_pillar_not_additive"
  | "rule.cadence.target_is_brand_channel_level"
  | "rule.cadence.incomplete_week_not_compared"
  | "rule.performance.samples_taken_at_different_ages"
  | "rule.performance.stat_window_mixed"
  | "rule.audience_feedback.tags_not_additive"
  | "rule.execution_flow.no_threshold"
  | "rule.roi_reference.shown_as_is"
  | "rule.scope.historical_import_account_unknown"
  | "rule.default";

export function opdiagStatusKey(status: OpDiagDimensionResult["status"]): OpDiagDisplayKey {
  switch (status) {
    case "ok": return "status.ok";
    case "not_computable": return "status.not_computable";
    default: return "status.unknown";
  }
}

export function opdiagReasonKey(reason: string): OpDiagDisplayKey {
  switch (reason) {
    case "no_data": return "reason.no_data";
    case "missing_config": return "reason.missing_config";
    case "missing_comparison_window": return "reason.missing_comparison_window";
    case "missing_observation_window": return "reason.missing_observation_window";
    case "no_delivery_channel": return "reason.no_delivery_channel";
    case "unknown": return "reason.unknown";
    default: return "reason.default";
  }
}

export function opdiagGapKey(kind: string): OpDiagDisplayKey {
  switch (kind) {
    case "profile_field_pending": return "gap.profile_field_pending";
    case "work_unchecked": return "gap.work_unchecked";
    case "work_untagged": return "gap.work_untagged";
    case "cadence_unset": return "gap.cadence_unset";
    case "observation_unset": return "gap.observation_unset";
    case "published_at_missing": return "gap.published_at_missing";
    case "metric_missing": return "gap.metric_missing";
    case "account_unresolved": return "gap.account_unresolved";
    case "work_missing": return "gap.work_missing";
    case "publication_status_unknown": return "gap.publication_status_unknown";
    case "excerpt_untagged": return "gap.excerpt_untagged";
    default: return "kind.unknown";
  }
}

export function opdiagSectionKey(section: string): OpDiagDisplayKey {
  switch (section) {
    case "account": return "kind.account";
    case "unknown_account": return "kind.unknown_account";
    case "brand": return "kind.brand";
    default: return "kind.unknown";
  }
}

export function opdiagRuleKey(rule: string): OpDiagDisplayKey {
  switch (rule) {
    case "common.unknown_is_not_zero": return "rule.common.unknown_is_not_zero";
    case "common.no_cross_platform_ranking": return "rule.common.no_cross_platform_ranking";
    case "common.no_score": return "rule.common.no_score";
    case "common.window_in_brand_timezone": return "rule.common.window_in_brand_timezone";
    case "performance.difference_is_not_cause": return "rule.performance.difference_is_not_cause";
    case "consistency.marks_on_older_profile": return "rule.consistency.marks_on_older_profile";
    case "coverage.multi_pillar_not_additive": return "rule.coverage.multi_pillar_not_additive";
    case "cadence.target_is_brand_channel_level": return "rule.cadence.target_is_brand_channel_level";
    case "cadence.incomplete_week_not_compared": return "rule.cadence.incomplete_week_not_compared";
    case "performance.samples_taken_at_different_ages": return "rule.performance.samples_taken_at_different_ages";
    case "performance.stat_window_mixed": return "rule.performance.stat_window_mixed";
    case "audience_feedback.tags_not_additive": return "rule.audience_feedback.tags_not_additive";
    case "execution_flow.no_threshold": return "rule.execution_flow.no_threshold";
    case "roi_reference.shown_as_is": return "rule.roi_reference.shown_as_is";
    case "scope.historical_import_account_unknown": return "rule.scope.historical_import_account_unknown";
    default: return "rule.default";
  }
}

/** Server display strings are presentation data, not numbers to parse or format. */
export function opdiagNumberDisplay(value: OpDiagNumber): string | null {
  return value.status === "ok" ? value.display : null;
}
