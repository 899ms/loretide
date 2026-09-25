// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ApiError } from "@multica/core/api";

import { ROI_METRIC_IDS, ROI_REASON_CODES, type RoiMetric } from "./contract";
import {
  grossDeltaShown,
  isNegativeDisplay,
  roiAmountProblem,
  roiAmountView,
  roiMetricLabelKey,
  roiMetricView,
  roiReasonKey,
  roiWriteOutcome,
  signedGrossDelta,
} from "./display";

// specs/034 PR 5 (T085): which i18n key the page shows. No arithmetic.

function metric(overrides: Partial<RoiMetric>): RoiMetric {
  return {
    status: "ok",
    value: "",
    display: "",
    unit: "",
    numerator: "",
    denominator: "",
    reason: "",
    formula: "",
    records: [],
    ...overrides,
  };
}

describe("roiMetricView", () => {
  it("passes the display string through untouched", () => {
    expect(roiMetricView(metric({ value: "2", display: "200.00%" }))).toEqual({
      kind: "value",
      display: "200.00%",
      negative: false,
    });
    expect(roiMetricView(metric({ value: "10", display: "10.00 倍" }))).toEqual({
      kind: "value",
      display: "10.00 倍",
      negative: false,
    });
  });

  it("does not read value: a display that disagrees with value is still shown as written", () => {
    expect(roiMetricView(metric({ value: "999", display: "200.00%" }))).toMatchObject({ display: "200.00%" });
  });

  it("marks a negative return by its minus sign, and still shows it", () => {
    expect(roiMetricView(metric({ value: "-1/2", display: "−50.00%" }))).toEqual({
      kind: "value",
      display: "−50.00%",
      negative: true,
    });
  });

  it("picks the reason key for a not-computable metric", () => {
    for (const reason of ROI_REASON_CODES) {
      expect(roiMetricView(metric({ status: "not_computable", reason }))).toEqual({
        kind: "not_computable",
        reason,
      });
    }
  });

  it("sends an unknown reason to the default branch", () => {
    expect(roiMetricView(metric({ status: "not_computable", reason: "something_new" }))).toEqual({
      kind: "not_computable",
      reason: "default",
    });
    expect(roiReasonKey("")).toBe("default");
  });

  it("never turns a missing or empty metric into a number", () => {
    expect(roiMetricView(undefined)).toEqual({ kind: "not_computable", reason: "default" });
    expect(roiMetricView(metric({ status: "ok", display: "" }))).toEqual({
      kind: "not_computable",
      reason: "default",
    });
  });

  it("treats a breakdown amount the same way", () => {
    expect(roiAmountView({ status: "ok", display: "300.00", reason: "" })).toEqual({
      kind: "value",
      display: "300.00",
      negative: false,
    });
    expect(roiAmountView({ status: "not_computable", display: "", reason: "currency_unconverted" })).toEqual({
      kind: "not_computable",
      reason: "currency_unconverted",
    });
  });
});

describe("roiMetricLabelKey", () => {
  it("keeps the three ratios on their own keys", () => {
    expect(roiMetricLabelKey("business_roi")).toBe("business_roi");
    expect(roiMetricLabelKey("revenue_to_spend")).toBe("revenue_to_spend");
    expect(roiMetricLabelKey("ad_roas")).toBe("ad_roas");
  });

  it("knows all sixteen and nothing else", () => {
    for (const id of ROI_METRIC_IDS) expect(roiMetricLabelKey(id)).toBe(id);
    expect(roiMetricLabelKey("profit_roi")).toBe("default");
  });
});

describe("isNegativeDisplay", () => {
  it("reads the first character only", () => {
    expect(isNegativeDisplay("−0.50%")).toBe(true);
    expect(isNegativeDisplay("-300.00")).toBe(true);
    expect(isNegativeDisplay("300.00")).toBe(false);
    expect(isNegativeDisplay("")).toBe(false);
  });
});

describe("roiAmountProblem", () => {
  it("accepts an amount within the currency's places", () => {
    expect(roiAmountProblem("3000.00", "CNY")).toBeNull();
    expect(roiAmountProblem("3000", "JPY")).toBeNull();
  });

  it("refuses more places than the currency has, without rounding", () => {
    expect(roiAmountProblem("3000.005", "CNY")).toBe("too_many_decimals");
    expect(roiAmountProblem("3000.5", "JPY")).toBe("too_many_decimals");
  });

  it("refuses an empty, malformed or unknown-currency amount", () => {
    expect(roiAmountProblem("  ", "CNY")).toBe("empty");
    expect(roiAmountProblem("3,000", "CNY")).toBe("format");
    expect(roiAmountProblem("-3", "CNY")).toBe("format");
    expect(roiAmountProblem("3", "RMB")).toBe("unknown_currency");
  });

  it("allows a sign only where asked", () => {
    expect(roiAmountProblem("-3.00", "CNY", { signed: true })).toBeNull();
  });
});

describe("signedGrossDelta and grossDeltaShown", () => {
  it("stores a reduction of 300 as -300", () => {
    expect(signedGrossDelta("reduction", "300")).toBe("-300");
    expect(signedGrossDelta("reduction", " 300.00 ")).toBe("-300.00");
  });

  it("stores an increase as typed and 'not given' as absent, never 0", () => {
    expect(signedGrossDelta("increase", "300")).toBe("300");
    expect(signedGrossDelta("not_given", "300")).toBeNull();
  });

  it("reads the sign back for the label", () => {
    expect(grossDeltaShown("-300.00")).toBe("reduction");
    expect(grossDeltaShown("300.00")).toBe("increase");
    expect(grossDeltaShown("0.00")).toBe("zero");
    expect(grossDeltaShown(null)).toBe("not_given");
  });
});

describe("roiWriteOutcome", () => {
  const fail = (status: number, body: unknown) => new ApiError("x", status, "", body);

  it("tells a stale revision from a possible duplicate and an idempotency clash", () => {
    expect(roiWriteOutcome(fail(409, { field: "base_revision", reason: "stale revision" }))).toEqual({ kind: "stale" });
    expect(roiWriteOutcome(fail(409, { code: "possible_duplicate", matches: ["c1", "c2"] }))).toEqual({
      kind: "duplicate",
      matches: ["c1", "c2"],
    });
    expect(roiWriteOutcome(fail(409, { field: "Idempotency-Key" }))).toEqual({ kind: "idempotency" });
  });

  it("names the field and the row of a refused input", () => {
    expect(roiWriteOutcome(fail(400, { field: "currency", reason: "not an allowed value", row: 2 }))).toEqual({
      kind: "invalid",
      field: "currency",
      reason: "not an allowed value",
      row: 2,
    });
  });

  it("answers a refusal and a missing record alike", () => {
    expect(roiWriteOutcome(fail(404, { code: "NOT_FOUND" }))).toEqual({ kind: "not_found" });
  });

  it("does not throw on anything else", () => {
    expect(roiWriteOutcome(new Error("offline"))).toEqual({ kind: "failed", nextAction: "", traceId: "" });
    expect(roiWriteOutcome(fail(503, "not json"))).toMatchObject({ kind: "failed" });
  });
});
