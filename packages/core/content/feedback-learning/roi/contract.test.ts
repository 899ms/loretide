// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  ROI_ADJUSTMENT_KINDS,
  ROI_CURRENCIES,
  ROI_EVIDENCE_TYPES,
  ROI_GROSS_BASES,
  ROI_PRICINGS,
  ROI_RECORD_SOURCES,
  ROI_TOUCH_PLATFORMS,
  ROI_TOUCH_ROLES,
  parseRoiAdjustment,
  parseRoiCost,
  parseRoiCostHistory,
  parseRoiCostList,
  parseRoiDeal,
  parseRoiDealDetail,
  parseRoiDealList,
  parseRoiLead,
  parseRoiLeadDetail,
  parseRoiLeadList,
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
