// @vitest-environment node
import { describe, expect, it } from "vitest";
import { opdiagGapKey, opdiagNumberDisplay, opdiagReasonKey, opdiagRuleKey, opdiagSectionKey, opdiagStatusKey } from "./display";

describe("opdiag display", () => {
  it("keeps server display strings untouched", () => {
    expect(opdiagNumberDisplay({ status: "ok", value: "0", display: "0.00" })).toBe("0.00");
    expect(opdiagNumberDisplay({ status: "not_computable", reason: "no_data" })).toBeNull();
  });

  it("degrades unknown controlled values safely", () => {
    expect(opdiagStatusKey("unknown")).toBe("status.unknown");
    expect(opdiagReasonKey("future_reason")).toBe("reason.default");
    expect(opdiagSectionKey("future_section")).toBe("kind.unknown");
    expect(opdiagRuleKey("future.rule")).toBe("rule.default");
    expect(opdiagGapKey("future_gap")).toBe("kind.unknown");
  });

  it("has a specific display key for every current rule", () => {
    const rules = [
      "common.unknown_is_not_zero", "common.no_cross_platform_ranking", "common.no_score", "common.window_in_brand_timezone",
      "consistency.marks_on_older_profile", "coverage.multi_pillar_not_additive", "cadence.target_is_brand_channel_level",
      "cadence.incomplete_week_not_compared", "performance.difference_is_not_cause", "performance.samples_taken_at_different_ages",
      "performance.stat_window_mixed", "audience_feedback.tags_not_additive", "execution_flow.no_threshold",
      "roi_reference.shown_as_is", "scope.historical_import_account_unknown",
    ];
    for (const rule of rules) expect(opdiagRuleKey(rule), rule).not.toBe("rule.default");
  });
});
