// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  EXCERPT_SOURCES,
  METRIC_NAMES,
  METRIC_PLATFORMS,
  METRIC_SOURCES,
  REVIEW_STATES,
  describeMetricValue,
  parseFeedbackList,
  parseManualMetric,
  parseManualMetrics,
  parsePendingFeedback,
  sameMetricValue,
} from "./contract";

// The controlled sets are the server's. Restating them here is unavoidable - a
// component has to switch on them - so they are compared against the Go source
// rather than against my memory of it, the same way 024's and 025's are.
const goSource = readFileSync(
  join(__dirname, "../../../../server/internal/content/feedback-learning/contract.go"),
  "utf8",
);

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goSource);
  if (!declaration) throw new Error(`${varName} not found in contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goSource);
    if (!value) throw new Error(`${name} has no literal in contract.go`);
    return value[1]!;
  });
}

describe("the controlled sets match the Go source", () => {
  it.each([
    ["Platforms", METRIC_PLATFORMS],
    ["Metrics", METRIC_NAMES],
    ["MetricSources", METRIC_SOURCES],
    ["ExcerptSources", EXCERPT_SOURCES],
    ["ReviewStates", REVIEW_STATES],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goSetValues(goName));
  });

  // SOP 10.1 names eleven metrics and three excerpt sources. An "other" turns
  // a controlled set into a suggestion.
  it("has exactly the counts the SOP gives, and no escape hatch", () => {
    expect(METRIC_NAMES).toHaveLength(11);
    expect(EXCERPT_SOURCES).toHaveLength(3);
    expect(REVIEW_STATES).toHaveLength(7);
    for (const forbidden of ["other", "misc", "custom"]) {
      expect(METRIC_NAMES as readonly string[]).not.toContain(forbidden);
      expect(EXCERPT_SOURCES as readonly string[]).not.toContain(forbidden);
    }
  });

  // SOP 10.1: "不同平台的阅读和播放分别保留，不直接合并排名".
  it("keeps read and play as two separate names", () => {
    expect(METRIC_NAMES as readonly string[]).toContain("read");
    expect(METRIC_NAMES as readonly string[]).toContain("play");
  });
});

const metricWire = {
  manual_metric_id: "m1",
  workspace_id: "ws-1",
  publication_record_id: "pub-1",
  platform: "xiaohongshu",
  account_id: "acct-1",
  metric: "read",
  value: 1200,
  unit: "次",
  stat_window: "发布后 14 天累计",
  sampled_at: "2026-09-20T10:00:00Z",
  recorded_by: "user-1",
  evidence_note: "",
  source_type: "manual",
  created_at: "2026-09-20T10:00:00Z",
  version_id: "v3",
};

describe("parseManualMetric", () => {
  it("reads a recorded number", () => {
    const metric = parseManualMetric(metricWire);
    expect(metric.value).toBe(1200);
    expect(metric.versionId).toBe("v3");
  });

  // Ruling Q2: a record entered for history has no delivery task behind it, so
  // there is no version. "" is a real answer, not missing data.
  it("reads an unresolved version as empty rather than missing", () => {
    expect(parseManualMetric({ ...metricWire, version_id: "" }).versionId).toBe("");
    const { version_id: _omitted, ...withoutVersion } = metricWire;
    expect(parseManualMetric(withoutVersion).versionId).toBe("");
  });

  it("degrades a malformed response instead of throwing", () => {
    expect(parseManualMetric({ nonsense: true }).manualMetricId).toBe("");
    expect(parseManualMetrics({ metrics: null })).toEqual([]);
    expect(parseManualMetrics("not an object")).toEqual([]);
  });

  // An installed desktop build talks to whatever backend is deployed.
  it("keeps a metric name it has never heard of", () => {
    expect(parseManualMetric({ ...metricWire, metric: "watch_time" }).metric).toBe("watch_time");
  });
});

describe("parseFeedbackList", () => {
  it("keeps the quote and the reading apart", () => {
    const list = parseFeedbackList({
      excerpts: [
        {
          feedback_excerpt_id: "e1",
          source_type: "comment",
          redacted_excerpt: "看完就去买了",
          interpretation: "转化点在第三段",
          tags: ["转化"],
        },
      ],
      review_state: "pending_data",
    });
    expect(list.excerpts[0]!.redactedExcerpt).toBe("看完就去买了");
    expect(list.excerpts[0]!.interpretation).toBe("转化点在第三段");
  });

  it("reads null tags as an empty array", () => {
    const list = parseFeedbackList({
      excerpts: [{ feedback_excerpt_id: "e1", tags: null }],
    });
    expect(list.excerpts[0]!.tags).toEqual([]);
  });

  // A response that did not say degrades to pending_data: claiming a report
  // exists when we do not know is the direction that misleads.
  it("degrades an unknown review state to pending_data", () => {
    expect(parseFeedbackList({ excerpts: [] }).reviewState).toBe("pending_data");
    expect(parseFeedbackList(null).reviewState).toBe("pending_data");
  });

  it("keeps a review state from a newer backend", () => {
    expect(parseFeedbackList({ excerpts: [], review_state: "generating" }).reviewState)
      .toBe("generating");
  });
});

describe("parsePendingFeedback", () => {
  it("reads a null publication time as null, not as a date", () => {
    const [item] = parsePendingFeedback({
      pending: [{ publication_record_id: "pub-1", published_at: null }],
    });
    expect(item!.publishedAt).toBeNull();
  });

  it("degrades a malformed response instead of throwing", () => {
    expect(parsePendingFeedback({ pending: null })).toEqual([]);
    expect(parsePendingFeedback(7)).toEqual([]);
  });
});

describe("comparing and describing values", () => {
  it.each([
    [null, null, true],
    [0, 0, true],
    [null, 0, false],
    [0, null, false],
    [0, 1, false],
  ])("sameMetricValue(%s, %s) is %s", (left, right, expected) => {
    expect(sameMetricValue(left as number | null, right as number | null)).toBe(expected);
  });

  it("never renders an unknown value as a number or a blank", () => {
    expect(describeMetricValue(null)).toEqual({ known: false, text: "unknown" });
    expect(describeMetricValue(0)).toEqual({ known: true, text: "0" });
    expect(describeMetricValue(null).text).not.toBe("0");
    expect(describeMetricValue(null).text).not.toBe("");
  });
});
