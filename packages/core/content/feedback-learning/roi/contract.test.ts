// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  ROI_ADJUSTMENT_KINDS,
  ROI_ALLOCATION_METHODS,
  ROI_ALLOCATION_TARGETS,
  ROI_ATTRIBUTION_METHODS,
  ROI_CURRENCIES,
  ROI_EVIDENCE_TYPES,
  ROI_GROSS_BASES,
  ROI_IMPORT_KINDS,
  ROI_IMPORT_OUTCOMES,
  ROI_JUDGEMENTS,
  ROI_METRIC_IDS,
  ROI_PRICINGS,
  ROI_REASON_CODES,
  ROI_RECORD_SOURCES,
  ROI_TOUCH_PLATFORMS,
  ROI_TOUCH_ROLES,
  parseRoiAdjustment,
  parseRoiAttribution,
  parseRoiCost,
  parseRoiCostHistory,
  parseRoiCostList,
  parseRoiDeal,
  parseRoiDealDetail,
  parseRoiDealList,
  parseRoiImportBatch,
  parseRoiImportList,
  parseRoiImportResult,
  parseRoiLead,
  parseRoiLeadDetail,
  parseRoiLeadList,
  parseRoiResult,
  parseRoiTouch,
  roiPath,
} from "./contract";

// specs/034 PR 1. The sets are held to the Go source, and every schema has a
// malformed-response case - including an amount sent as a JSON number, which
// must be refused rather than trusted.

const goDir = join(__dirname, "../../../../../server/internal/content/feedback-learning");
const goContract = readFileSync(join(goDir, "roi_contract.go"), "utf8");
const goMoney = readFileSync(join(goDir, "roi_money.go"), "utf8");
const goFeedback = readFileSync(join(goDir, "contract.go"), "utf8");

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goContract);
  if (!declaration) throw new Error(`${varName} not found in roi_contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goContract + goFeedback);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("the ROI controlled sets match the Go source", () => {
  it.each([
    ["EvidenceTypes", ROI_EVIDENCE_TYPES],
    ["Pricings", ROI_PRICINGS],
    ["TouchRoles", ROI_TOUCH_ROLES],
    ["GrossBases", ROI_GROSS_BASES],
    ["AdjustmentKinds", ROI_ADJUSTMENT_KINDS],
    ["RecordSources", ROI_RECORD_SOURCES],
    ["TouchPlatforms", ROI_TOUCH_PLATFORMS],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goSetValues(goName));
  });

  it("has exactly six evidence types and no other", () => {
    expect(ROI_EVIDENCE_TYPES).toHaveLength(6);
    expect(ROI_EVIDENCE_TYPES as readonly string[]).not.toContain("other");
  });

  it("has the ruling's nine currencies with the Go table's places", () => {
    const rows = [...goMoney.matchAll(/\{Code: "([A-Z]{3})", Digits: (\d)\}/g)].map((m) => ({
      code: m[1],
      digits: Number(m[2]),
    }));
    expect(rows).toHaveLength(9);
    expect(ROI_CURRENCIES.map((row) => ({ code: row.code, digits: row.digits }))).toEqual(rows);
  });
});

const costWire = {
  cost_id: "c1", revision: 1, voided: false, category: "拍摄", pricing: "amount",
  amount_minor: "300000", amount: "3000.00", amount_status: "ok", currency: "CNY",
  labor_minutes: null, labor_rate_minor: null, labor_rate: null,
  incurred_at: "2026-09-10T02:00:00Z", ad_spend: false, account_id: "", work_id: "",
  campaign_label: "", evidence_note: "", note: "", not_duplicate_of: [],
  source_type: "manual", import_batch_id: "", recorded_by: "u1", created_at: "2026-09-10T02:00:00Z",
};
const leadWire = {
  lead_id: "l1", revision: 1, voided: false, customer_ref: "客户-0412", stage: "咨询",
  qualified: true, first_seen_at: "2026-09-10T02:00:00Z", merged_into: "", note: "",
  not_duplicate_of: [], source_type: "manual", import_batch_id: "", recorded_by: "u1",
  created_at: "2026-09-10T02:00:00Z",
};
const touchWire = {
  touch_id: "t1", revision: 1, voided: false, lead_id: "l1", evidence_type: "account_only",
  platform: "douyin", account_id: "a1", work_id: "", publication_record_id: "", role: "first_touch",
  paid: false, occurred_at: "2026-09-10T02:00:00Z", evidence_note: "", note: "",
  recorded_by: "u1", created_at: "2026-09-10T02:00:00Z",
};
const dealWire = {
  deal_id: "d1", revision: 1, voided: false, lead_id: "l1", order_ref: "TB-1",
  amount_minor: "1000000", amount: "10000.00", currency: "CNY", closed_at: "2026-09-12T02:00:00Z",
  gross_basis: "cogs", gross_profit_minor: null, gross_profit: null, cogs_minor: "700000",
  cogs: "7000.00", note: "", not_duplicate_of: [], source_type: "manual", import_batch_id: "",
  recorded_by: "u1", created_at: "2026-09-12T02:00:00Z",
};
const adjustmentWire = {
  adjustment_id: "a1", revision: 1, voided: false, deal_id: "d1", kind: "refund",
  revenue_delta_minor: "100000", revenue_delta: "1000.00", gross_delta_minor: null,
  gross_delta: null, currency: "CNY", occurred_at: "2026-09-20T02:00:00Z", note: "",
  recorded_by: "u1", created_at: "2026-09-20T02:00:00Z",
};

describe("costs", () => {
  it("keeps amounts as strings", () => {
    const cost = parseRoiCost(costWire);
    expect(cost?.amountMinor).toBe("300000");
    expect(cost?.amount).toBe("3000.00");
  });

  it("keeps a labor cost without a rate not computable, not zero", () => {
    const cost = parseRoiCost({ ...costWire, pricing: "labor_time", amount_minor: null, amount: null,
      amount_status: "labor_rate_missing", labor_minutes: 360 });
    expect(cost?.amountMinor).toBeNull();
    expect(cost?.amountStatus).toBe("labor_rate_missing");
  });

  it("refuses an amount sent as a number", () => {
    expect(parseRoiCost({ ...costWire, amount_minor: 300000 })).toBeNull();
  });

  it("degrades malformed responses", () => {
    expect(parseRoiCost({ nonsense: true })).toBeNull();
    expect(parseRoiCostList({ costs: null })).toEqual([]);
    expect(parseRoiCostList("not an object")).toEqual([]);
    expect(parseRoiCostHistory({ cost_id: "c1", current: costWire })).toBeNull();
    expect(parseRoiCostHistory({ cost_id: "c1", current: costWire, revisions: [costWire] })?.revisions)
      .toHaveLength(1);
  });
});

describe("leads and touches", () => {
  it("reads a lead", () => {
    expect(parseRoiLead(leadWire)?.customerRef).toBe("客户-0412");
    expect(parseRoiLead({ ...leadWire, revision: "1" })).toBeNull();
    expect(parseRoiLeadList({ leads: [leadWire] })).toHaveLength(1);
    expect(parseRoiLeadList(42)).toEqual([]);
  });

  it("keeps an empty work on a touch as empty", () => {
    const touch = parseRoiTouch(touchWire);
    expect(touch?.workId).toBe("");
    expect(touch?.evidenceType).toBe("account_only");
    expect(parseRoiTouch({ ...touchWire, touch_id: 7 })).toBeNull();
  });

  it("reads a lead's touches", () => {
    const detail = parseRoiLeadDetail({
      lead_id: "l1", current: leadWire, revisions: [leadWire],
      touches: [{ touch_id: "t1", current: touchWire, revisions: [touchWire] }],
    });
    expect(detail?.touches.map((touch) => touch.touchId)).toEqual(["t1"]);
    expect(parseRoiLeadDetail({ lead_id: "l1" })).toBeNull();
  });
});

describe("deals and adjustments", () => {
  it("reads a deal and its refunds", () => {
    const detail = parseRoiDealDetail({
      deal_id: "d1", current: dealWire, revisions: [dealWire],
      adjustments: [{ adjustment_id: "a1", current: adjustmentWire, revisions: [adjustmentWire] }],
    });
    expect(detail?.current.cogsMinor).toBe("700000");
    expect(detail?.current.grossProfitMinor).toBeNull();
    expect(detail?.adjustments[0]?.current.grossDeltaMinor).toBeNull();
  });

  it("refuses amounts sent as numbers", () => {
    expect(parseRoiDeal({ ...dealWire, amount_minor: 1000000 })).toBeNull();
    expect(parseRoiAdjustment({ ...adjustmentWire, revenue_delta_minor: 100000 })).toBeNull();
  });

  it("degrades malformed responses", () => {
    expect(parseRoiDeal(null)).toBeNull();
    expect(parseRoiDealList({ deals: "x" })).toEqual([]);
    expect(parseRoiDealDetail({ deal_id: "d1", current: dealWire, revisions: "x" })).toBeNull();
    expect(parseRoiAdjustment({})).toBeNull();
  });
});

describe("roiPath", () => {
  it("encodes every segment", () => {
    expect(roiPath("costs", "a/b", "revisions")).toBe("costs/a%2Fb/revisions");
  });
});

// ---------------------------------------------------------------- PR 2

const goPR2 = ["roi_allocate.go", "roi_attribution.go", "roi_calc.go"]
  .map((name) => readFileSync(join(goDir, name), "utf8"))
  .join("\n");

function goPR2Values(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goPR2);
  if (!declaration) throw new Error(`${varName} not found in the PR 2 Go files`);
  return declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean).map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goPR2);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("the PR 2 sets match the Go source", () => {
  it.each([
    ["AllocationTargets", ROI_ALLOCATION_TARGETS],
    ["AllocationMethods", ROI_ALLOCATION_METHODS],
    ["Judgements", ROI_JUDGEMENTS],
    ["AttributionMethods", ROI_ATTRIBUTION_METHODS],
    ["ReasonCodes", ROI_REASON_CODES],
    ["MetricIDs", ROI_METRIC_IDS],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goPR2Values(goName));
  });
});

describe("allocations and judgements", () => {
  const share = {
    target_kind: "work", target_id: "w1", method: "weights", weight: 1,
    allocated_minor: "3334", allocated: "33.34",
  };

  it("reads a cost's shares as strings", () => {
    const cost = parseRoiCost({ ...costWire, allocations: [share] });
    expect(cost?.allocations).toEqual([{
      targetKind: "work", targetId: "w1", method: "weights", weight: 1,
      allocatedMinor: "3334", allocated: "33.34",
    }]);
    expect(parseRoiCost(costWire)?.allocations).toEqual([]);
    expect(parseRoiCost({ ...costWire, allocations: [{ ...share, allocated_minor: 3334 }] })).toBeNull();
  });

  const attributionWire = {
    deal_id: "d1", revision: 1, voided: false, judgement: "multi_touch", touch_ids: ["t1", "t2"],
    weights: [2, 1], note: "", recorded_by: "u1", created_at: "2026-09-25T00:00:00Z",
  };

  it("reads a judgement and the deal's judgements", () => {
    expect(parseRoiAttribution(attributionWire)?.touchIds).toEqual(["t1", "t2"]);
    expect(parseRoiAttribution({ ...attributionWire, deal_id: 1 })).toBeNull();
    const detail = parseRoiDealDetail({
      deal_id: "d1", current: dealWire, revisions: [dealWire], attributions: [attributionWire],
    });
    expect(detail?.attributions.map((a) => a.judgement)).toEqual(["multi_touch"]);
    expect(parseRoiDealDetail({ deal_id: "d1", current: dealWire, revisions: [dealWire] })?.attributions)
      .toEqual([]);
  });
});

// The server's own figures for contract §5.4's D14-V12 samples.
const resultWire = {
  calc_version: "roi-calc/1",
  window: { start: "2026-09-01", end: "2026-09-30", timezone: "Asia/Shanghai" },
  report_currency: "CNY",
  attribution_method: "even_split",
  generated_at: "2026-10-01T00:00:00Z",
  metrics: {
    business_roi: {
      status: "ok", value: "2", display: "200.00%", unit: "percent", numerator: "200000",
      denominator: "100000", formula: "business_roi/1",
      records: [{ kind: "cost", id: "c-1", revision: 1, amount_minor: "100000", currency: "CNY", converted_minor: "100000" }],
    },
    revenue_to_spend: {
      status: "ok", value: "10", display: "10.00 倍", unit: "times", numerator: "1000000",
      denominator: "100000", formula: "revenue_to_spend/1", records: [],
    },
    ad_roas: { status: "not_computable", reason: "no_data", unit: "times", formula: "ad_roas/1", records: [] },
  },
  breakdown: {
    by_work: [{
      kind: "work", id: "w-1", deals_touched: 1,
      attributed_net_revenue: { status: "ok", value: "1000000", display: "10000.00" },
      attributed_gross_profit: { status: "not_computable", reason: "missing_gross_profit" },
    }],
    by_account: [],
    account_level_unknown_work: null,
    brand_level_unknown_account: null,
    unattributed: null,
    even_split_fallback: [],
  },
  rules: ["阶段是自由文本，没有先后顺序；转化率只按线索是否到达过某阶段计算，不反映阶段之间的先后。"],
};

describe("a computed report", () => {
  it("passes D14-V12's display strings through untouched", () => {
    const result = parseRoiResult(resultWire);
    expect(result?.metrics.business_roi?.display).toBe("200.00%");
    expect(result?.metrics.revenue_to_spend?.display).toBe("10.00 倍");
    expect(result?.metrics.ad_roas?.status).toBe("not_computable");
    expect(result?.metrics.ad_roas?.reason).toBe("no_data");
    expect(result?.metrics.ad_roas?.display).toBe("");
    expect(result?.breakdown.byWork[0]?.attributedGrossProfit.status).toBe("not_computable");
    expect(result?.rules).toHaveLength(1);
  });

  it("never labels the revenue ratio as a profit ROI", () => {
    for (const label of ["revenue_to_spend", resultWire.metrics.revenue_to_spend.formula]) {
      expect(label.toLowerCase()).not.toContain("roi");
      expect(label.toLowerCase()).not.toContain("profit");
    }
  });

  it("reads an unknown status as not computable, never as a number", () => {
    const result = parseRoiResult({
      ...resultWire,
      metrics: { business_roi: { ...resultWire.metrics.business_roi, status: "estimated" } },
    });
    expect(result?.metrics.business_roi?.status).toBe("not_computable");
    expect(result?.metrics.business_roi?.display).toBe("");
  });

  it("refuses numbers where strings belong, and degrades malformed responses", () => {
    expect(parseRoiResult({
      ...resultWire,
      metrics: { business_roi: { ...resultWire.metrics.business_roi, value: 2 } },
    })).toBeNull();
    expect(parseRoiResult({ ...resultWire, metrics: "x" })).toBeNull();
    expect(parseRoiResult({ calc_version: "roi-calc/1" })).toBeNull();
    expect(parseRoiResult(null)).toBeNull();
  });
});

// ---------------------------------------------------------------- import (PR 3)

const goImport = readFileSync(join(goDir, "roi_import.go"), "utf8");

function goImportSet(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goImport);
  if (!declaration) throw new Error(`${varName} not found in roi_import.go`);
  return declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean).map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goImport);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("the import sets match the Go source", () => {
  it.each([
    ["ImportRecordKinds", ROI_IMPORT_KINDS],
    ["ImportOutcomes", ROI_IMPORT_OUTCOMES],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goImportSet(goName));
  });
});

const importRowsWire = [
  { row: 1, outcome: "written", record_id: "d1", duplicate_of: [] },
  { row: 2, outcome: "duplicate", record_id: "", duplicate_of: ["d0"] },
  { row: 3, outcome: "confirmed_not_duplicate", record_id: "d3", duplicate_of: ["d9"] },
];
const importResultWire = {
  dry_run: false, import_batch_id: "b1", record_kind: "deal", row_count: 3, written_count: 2,
  skipped_count: 1, rows: importRowsWire, recorded_by: "u1", created_at: "2026-09-25T01:02:03.456Z",
};
const importBatchWire = {
  import_batch_id: "b1", workspace_id: "ws", record_kind: "deal", row_count: 3, written_count: 2,
  skipped_count: 1, rows: importRowsWire, idempotency_key: "k1", recorded_by: "u1",
  created_at: "2026-09-25T01:02:03.456Z",
};

describe("import responses", () => {
  it("reads a real import's answer", () => {
    const result = parseRoiImportResult(importResultWire);
    expect(result).toMatchObject({ dryRun: false, importBatchId: "b1", writtenCount: 2, skippedCount: 1 });
    expect(result?.rows[1]).toEqual({ row: 2, outcome: "duplicate", recordId: "", duplicateOf: ["d0"], duplicateOfRows: [] });
  });

  it("reads a dry run, which has no batch and no time", () => {
    const result = parseRoiImportResult({
      ...importResultWire, dry_run: true, import_batch_id: "", created_at: null,
      rows: [{ row: 1, outcome: "duplicate", record_id: "", duplicate_of: [], duplicate_of_rows: [1] }],
    });
    expect(result).toMatchObject({ dryRun: true, importBatchId: "", createdAt: null });
    expect(result?.rows[0]?.duplicateOfRows).toEqual([1]);
  });

  it("reads an outcome it has not heard of as unknown, never as written", () => {
    const result = parseRoiImportResult({
      ...importResultWire, rows: [{ row: 1, outcome: "merged", record_id: "", duplicate_of: [] }],
    });
    expect(result?.rows[0]?.outcome).toBe("unknown");
  });

  it("reads a batch and a list of batches", () => {
    expect(parseRoiImportBatch(importBatchWire)).toMatchObject({ importBatchId: "b1", idempotencyKey: "k1" });
    expect(parseRoiImportList({ imports: [importBatchWire] })).toHaveLength(1);
    expect(parseRoiImportList({ imports: null })).toEqual([]);
  });

  it.each([
    ["a count as a string", { ...importResultWire, written_count: "2" }],
    ["a negative count", { ...importResultWire, skipped_count: -1 }],
    ["rows that are not a list", { ...importResultWire, rows: {} }],
    ["a record id as a number", { ...importResultWire, rows: [{ ...importRowsWire[0], record_id: 7 }] }],
    ["row 0", { ...importResultWire, rows: [{ ...importRowsWire[0], row: 0 }] }],
    ["duplicate_of that is not a list of ids", { ...importResultWire, rows: [{ ...importRowsWire[1], duplicate_of: "d0" }] }],
    ["no dry_run flag", { ...importResultWire, dry_run: undefined }],
  ])("degrades an import answer with %s to null", (_name, wire) => {
    expect(parseRoiImportResult(wire)).toBeNull();
  });

  it.each([
    ["no batch id", { ...importBatchWire, import_batch_id: undefined }],
    ["a row count as a float", { ...importBatchWire, row_count: 2.5 }],
    ["rows missing", { ...importBatchWire, rows: undefined }],
  ])("degrades a batch with %s to null", (_name, wire) => {
    expect(parseRoiImportBatch(wire)).toBeNull();
  });

  it("degrades a malformed list to empty", () => {
    expect(parseRoiImportList({ imports: [{ ...importBatchWire, rows: "x" }] })).toEqual([]);
    expect(parseRoiImportList("nope")).toEqual([]);
  });
});
