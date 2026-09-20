// @vitest-environment node
import { describe, expect, it } from "vitest";

import { personalPerformance, personalPerformanceUnknown } from "./performance";

// SOP 3.3: "明确显示『暂无个人表现数据』". specs/031 FR-024 ~ FR-027.

describe("personalPerformance", () => {
  it("says there is no data when nothing has been recorded", () => {
    expect(personalPerformance(0)).toEqual({ noData: true, metricCount: 0 });
  });

  it("stops saying it as soon as one metric exists", () => {
    // One is enough. FR-026: the notice disappears, and nothing takes its
    // place - there is no summary to show, because generating one is EP-08's
    // business and executors stay off (constitution IX).
    expect(personalPerformance(1).noData).toBe(false);
    expect(personalPerformance(1).metricCount).toBe(1);
  });

  it("does not apply a threshold of its own", () => {
    // "Three readings is too thin" is the operator's judgement to make. A
    // threshold here would be a rule the SOP never stated, in a place nobody
    // can see or change. FR-024: the test is row count === 0, nothing else.
    for (const count of [1, 2, 3, 10, 1000]) {
      expect(personalPerformance(count).noData).toBe(false);
    }
  });

  it("keeps metricCount === 0 exactly when noData", () => {
    // The invariant the callers rely on, including for the inputs a count
    // query cannot actually produce.
    for (const input of [-1, 0, 0.4, 1, 2.7, Number.NaN, Number.POSITIVE_INFINITY]) {
      const result = personalPerformance(input);
      expect(result.noData).toBe(result.metricCount === 0);
    }
  });

  it("floors a fractional count rather than rounding it up to data", () => {
    expect(personalPerformance(0.9)).toEqual({ noData: true, metricCount: 0 });
  });
});

describe("personalPerformanceUnknown", () => {
  it("is not the same answer as having no data", () => {
    // A failed read is not evidence of absence. Saying 暂无个人表现数据 on a
    // 503 states something nobody checked - the mistake Issue #167 fixed in
    // useAccountProfile, which swallowed an error and rendered an empty
    // profile as though it were a complete one.
    expect(personalPerformanceUnknown.noData).toBe(false);
    expect(personalPerformanceUnknown).not.toEqual(personalPerformance(0));
  });
});
