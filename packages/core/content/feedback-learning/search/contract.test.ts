// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  RANK_RESULT_KINDS,
  RANK_SINGLE_OBSERVATION_RULE,
  SEARCH_METRICS,
  SEARCH_METRIC_SOURCES,
  parseRankObservation,
  parseRankObservationList,
  parseSearchMetric,
  parseSearchMetricList,
  searchMetricSamples,
  searchObservationPath,
} from "./contract";

// specs/036 PR 4 (T093). The sets are held to the Go source, and every
// schema has a malformed-response case.

const goDir = join(__dirname, "../../../../../server/internal/content/feedback-learning");
const goContract = readFileSync(join(goDir, "search_contract.go"), "utf8");

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goContract);
  if (!declaration) throw new Error(`${varName} not found in search_contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goContract);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("controlled sets", () => {
  it("are the Go sets, in order", () => {
    expect([...SEARCH_METRICS]).toEqual(goSetValues("SearchMetrics"));
    expect([...RANK_RESULT_KINDS]).toEqual(goSetValues("RankResultKinds"));
    expect([...SEARCH_METRIC_SOURCES]).toEqual(goSetValues("SearchMetricSources"));
  });

  it("names the same single-observation rule as the server", () => {
    expect(goContract).toContain(`RuleRankSingleObservation = "${RANK_SINGLE_OBSERVATION_RULE}"`);
  });

  it("encodes each path segment", () => {
    expect(searchObservationPath("rank-observations", "a/b", "revisions")).toBe("rank-observations/a%2Fb/revisions");
  });
});

const metricWire = {
  search_metric_id: "m1", publication_record_id: "p1", platform: "xiaohongshu", account_id: "",
  metric: "search_impression", value: 1240, unit: "次", stat_window: "发布后 7 天累计",
  sampled_at: "2026-10-02T05:30:00Z", evidence_note: "截图", recorded_by: "u1", source_type: "manual",
  created_at: "2026-10-02T06:00:00Z", data_origin: "manual_only",
};

describe("parseSearchMetric", () => {
  it("reads a metric", () => {
    expect(parseSearchMetric(metricWire)).toMatchObject({
      searchMetricId: "m1", metric: "search_impression", value: 1240, statWindow: "发布后 7 天累计",
      sourceType: "manual", dataOrigin: "manual_only",
    });
  });

  it("keeps null (unknown) and 0 (confirmed zero) apart", () => {
    expect(parseSearchMetric({ ...metricWire, value: null })?.value).toBeNull();
    expect(parseSearchMetric({ ...metricWire, value: 0 })?.value).toBe(0);
  });

  it("never shows a number that is not a count", () => {
    expect(parseSearchMetric({ ...metricWire, value: "1240" })?.value).toBeNull();
    expect(parseSearchMetric({ ...metricWire, value: -3 })?.value).toBeNull();
    expect(parseSearchMetric({ ...metricWire, value: 1.5 })?.value).toBeNull();
  });

  it("reads a value outside a set as unknown", () => {
    const metric = parseSearchMetric({ ...metricWire, metric: "search_volume", source_type: "crawler", data_origin: "ai" });
    expect(metric?.metric).toBe("unknown");
    expect(metric?.sourceType).toBe("unknown");
    expect(metric?.dataOrigin).toBe("unknown");
  });

  it("returns null for a malformed response", () => {
    expect(parseSearchMetric({ ...metricWire, search_metric_id: 1 })).toBeNull();
    expect(parseSearchMetric("not json")).toBeNull();
  });
});

describe("parseSearchMetricList and searchMetricSamples", () => {
  it("reads every sample and shows a metric nobody recorded as unknown, not 0", () => {
    const list = parseSearchMetricList({ publication_record_id: "p1", metrics: [metricWire, { ...metricWire, search_metric_id: "m2", value: null }] });
    expect(list?.metrics.map((metric) => metric.value)).toEqual([1240, null]);
    expect(searchMetricSamples(list!.metrics, "search_visit")).toEqual({ status: "unknown" });
    const impressions = searchMetricSamples(list!.metrics, "search_impression");
    expect(impressions.status).toBe("recorded");
  });

  it("treats a null list as empty and a malformed one as null", () => {
    expect(parseSearchMetricList({ publication_record_id: "p1", metrics: null })?.metrics).toEqual([]);
    expect(parseSearchMetricList({ metrics: [] })).toBeNull();
  });
});

const observationWire = {
  observation_id: "o1", revision: 1, voided: false, platform: "xiaohongshu", account_id: "",
  query: "羊绒大衣能机洗吗", theme_id: "t1", publication_record_id: "", observed_at: "2026-10-02T13:30:00Z",
  conditions: "未登录 上海 综合排序", result_kind: "position", position: 7, scanned_depth: null,
  evidence_note: "截图", recorded_by: "u1", created_at: "2026-10-02T14:00:00Z", data_origin: "manual_only",
  rule: "rank.single_observation",
};

describe("parseRankObservation", () => {
  it("reads an observation with its rule", () => {
    expect(parseRankObservation(observationWire)).toMatchObject({
      observationId: "o1", resultKind: "position", position: 7, scannedDepth: null,
      rule: "rank.single_observation", dataOrigin: "manual_only",
    });
  });

  it("keeps only the integer the result kind asks for", () => {
    const notFound = parseRankObservation({ ...observationWire, result_kind: "not_found", position: 7, scanned_depth: 30 });
    expect(notFound?.position).toBeNull();
    expect(notFound?.scannedDepth).toBe(30);
  });

  it("is always a single observation, whatever the rule field says", () => {
    expect(parseRankObservation({ ...observationWire, rule: "rank.current" })?.rule).toBe("rank.single_observation");
  });

  it("reads a kind outside the set as unknown and shows neither integer", () => {
    const observation = parseRankObservation({ ...observationWire, result_kind: "top10" });
    expect(observation?.resultKind).toBe("unknown");
    expect(observation?.position).toBeNull();
  });

  it("returns null for a malformed response", () => {
    expect(parseRankObservation({ ...observationWire, revision: 0 })).toBeNull();
    expect(parseRankObservation({ ...observationWire, observed_at: undefined })).toBeNull();
  });
});

describe("parseRankObservationList", () => {
  it("reads one entry per observation, in the server's order", () => {
    const list = parseRankObservationList({ observations: [{ ...observationWire, observation_id: "o2" }, observationWire] });
    expect(list.map((observation) => observation.observationId)).toEqual(["o2", "o1"]);
  });

  it("carries no summary of the observations", () => {
    const [observation] = parseRankObservationList({ observations: [{ ...observationWire, best_position: 1, avg: 3 }] });
    expect(observation).not.toHaveProperty("bestPosition");
    expect(observation).not.toHaveProperty("avg");
  });

  it("returns [] for a malformed response", () => {
    expect(parseRankObservationList({ observations: "x" })).toEqual([]);
    expect(parseRankObservationList(null)).toEqual([]);
  });
});
