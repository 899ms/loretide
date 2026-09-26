import { describe, expect, it } from "vitest";
import { rankResultDisplay, rankRuleDisplay, searchMetricDisplay, searchMetricValueDisplay, searchMetricValueState } from "./display";
import type { SearchMetricRecord } from "./contract";

const sample = (value: number | null, metric = "search_impression"): SearchMetricRecord => ({
  searchMetricId: "m1", publicationRecordId: "p1", platform: "xiaohongshu", accountId: "",
  metric: metric as SearchMetricRecord["metric"], value, unit: "views", statWindow: "7d",
  sampledAt: "2026-09-27", evidenceNote: "manual", recordedBy: "person", sourceType: "manual",
  createdAt: "2026-09-27", dataOrigin: "manual_only",
});

describe("search observation display values", () => {
  it("distinguishes unrecorded, unknown and confirmed zero", () => {
    expect(searchMetricValueState([], "search_impression")).toBe("unrecorded");
    expect(searchMetricValueState([sample(null)], "search_impression")).toBe("unknown");
    expect(searchMetricValueDisplay(null)).toBe("unknown");
    expect(searchMetricValueState([sample(0)], "search_impression")).toBe("recorded");
    expect(searchMetricValueDisplay(0)).toBe("recorded");
  });
  it("maps result kinds, metric names and rule IDs to known display keys", () => {
    expect(rankResultDisplay("position")).toBe("position");
    expect(rankResultDisplay("not_found")).toBe("not_found");
    expect(rankResultDisplay("future")).toBe("unknown");
    expect(searchMetricDisplay("search_visit")).toBe("search_visit");
    expect(searchMetricDisplay("future")).toBe("unknown");
    expect(rankRuleDisplay("rank.single_observation")).toBe("rank_single_observation");
    expect(rankRuleDisplay("future")).toBe("unknown");
  });
});
