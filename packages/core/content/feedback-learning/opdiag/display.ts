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
  | "kind.account"
  | "kind.unknown_account"
  | "kind.brand"
  | "kind.unknown"
  | "rule.common.unknown_is_not_zero"
  | "rule.common.no_cross_platform_ranking"
  | "rule.common.no_score"
  | "rule.common.window_in_brand_timezone"
  | "rule.performance.difference_is_not_cause"
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
    default: return "reason.unknown";
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
    default: return "rule.default";
  }
}

/** Server display strings are presentation data, not numbers to parse or format. */
export function opdiagNumberDisplay(value: OpDiagNumber): string | null {
  return value.status === "ok" ? value.display : null;
}
