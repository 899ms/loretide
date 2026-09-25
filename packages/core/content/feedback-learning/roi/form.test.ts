// @vitest-environment node
import { describe, expect, it } from "vitest";

import type { RoiAdjustment, RoiDeal } from "./contract";
import {
  roiCheckShares,
  roiCheckStoredShares,
  roiDealGrossState,
  roiDecimalOf,
  roiMinorOf,
  roiMinorText,
  roiTouchForEvidence,
  roiTouchProblems,
  roiTouchRules,
} from "./form";

// specs/034 PR 5: the page's form rules. The server checks all of them
// again; these decide when a button is disabled and what it says.

const touch = { evidenceType: "", platform: "", accountId: "", workId: "", publicationRecordId: "" };

describe("roiTouchRules (FR-019)", () => {
  it("follows the table row by row", () => {
    expect(roiTouchRules("platform_linked_content")).toEqual({ content: "required", account: "optional", platform: "required" });
    expect(roiTouchRules("content_comment")).toEqual({ content: "required", account: "optional", platform: "required" });
    expect(roiTouchRules("customer_statement")).toEqual({ content: "optional", account: "optional", platform: "optional" });
    expect(roiTouchRules("dedicated_channel")).toEqual({ content: "optional", account: "optional", platform: "optional" });
    expect(roiTouchRules("account_only")).toEqual({ content: "forbidden", account: "required", platform: "required" });
    expect(roiTouchRules("unknown")).toEqual({ content: "forbidden", account: "forbidden", platform: "forbidden" });
  });

  it("names the work when platform-linked content has none", () => {
    expect(roiTouchProblems({ ...touch, evidenceType: "platform_linked_content", platform: "douyin" })).toEqual(["work_id"]);
    expect(
      roiTouchProblems({ ...touch, evidenceType: "platform_linked_content", platform: "douyin", publicationRecordId: "p1" }),
    ).toEqual([]);
  });

  it("names every field an unknown source must leave empty", () => {
    expect(
      roiTouchProblems({ ...touch, evidenceType: "unknown", platform: "douyin", accountId: "a1", workId: "w1" }),
    ).toEqual(["work_id", "account_id", "platform"]);
  });

  it("clears what a newly chosen type forbids, and keeps a customer statement's work", () => {
    const filled = { ...touch, evidenceType: "customer_statement", platform: "douyin", accountId: "a1", workId: "w1" };
    expect(roiTouchForEvidence(filled, "account_only")).toMatchObject({ workId: "", accountId: "a1", platform: "douyin" });
    expect(roiTouchForEvidence(filled, "unknown")).toMatchObject({ workId: "", accountId: "", platform: "" });
    expect(roiTouchForEvidence(filled, "customer_statement")).toMatchObject({ workId: "w1", evidenceType: "customer_statement" });
  });
});

describe("exact amounts", () => {
  it("reads and writes minor units without a float", () => {
    expect(roiMinorOf("3000.00", "CNY")).toBe(300000n);
    expect(roiMinorOf("3000.5", "CNY")).toBe(300050n);
    expect(roiMinorOf("92233720368547758.07", "CNY")).toBe(9223372036854775807n);
    expect(roiMinorOf("3000.005", "CNY")).toBeNull();
    expect(roiMinorOf("3000", "JPY")).toBe(3000n);
    expect(roiDecimalOf(300000n, "CNY")).toBe("3000.00");
    expect(roiDecimalOf(-5n, "CNY")).toBe("-0.05");
    expect(roiDecimalOf(3000n, "JPY")).toBe("3000");
    expect(roiMinorText("300000", "CNY")).toBe("3000.00");
    expect(roiMinorText("300000", "XXX")).toBe("300000");
  });
});

describe("roiCheckShares (FR-031)", () => {
  it("says the shares match the cost", () => {
    expect(roiCheckShares(["33.34", "33.33", "33.33"], "100.00", "CNY")).toEqual({
      total: "100.00",
      original: "100.00",
      gap: "0.00",
      matches: true,
    });
  });

  it("says how far apart they are when they do not", () => {
    expect(roiCheckShares(["30", "30", "30"], "100.00", "CNY")).toEqual({
      total: "90.00",
      original: "100.00",
      gap: "10.00",
      matches: false,
    });
  });

  it("gives no verdict while an amount is still not valid", () => {
    expect(roiCheckShares(["30", "3x"], "100.00", "CNY")).toBeNull();
    expect(roiCheckShares([], "100.00", "CNY")).toBeNull();
  });

  it("checks the server's stored shares the same way", () => {
    expect(roiCheckStoredShares(["3334", "3333", "3333"], "10000", "CNY")).toMatchObject({ total: "100.00", matches: true });
    expect(roiCheckStoredShares(["3334"], null, "CNY")).toBeNull();
  });
});

function deal(overrides: Partial<RoiDeal>): RoiDeal {
  return {
    dealId: "d1", revision: 1, voided: false, leadId: "", orderRef: "", amountMinor: "1000000", amount: "10000.00",
    currency: "CNY", closedAt: "", grossBasis: "stated_gross_profit", grossProfitMinor: "300000",
    grossProfit: "3000.00", cogsMinor: null, cogs: null, note: "", notDuplicateOf: [], sourceType: "manual",
    importBatchId: "", recordedBy: "", createdAt: "", ...overrides,
  };
}

function adjustment(overrides: Partial<RoiAdjustment>): RoiAdjustment {
  return {
    adjustmentId: "a1", revision: 1, voided: false, dealId: "d1", kind: "refund", revenueDeltaMinor: "100000",
    revenueDelta: "1000.00", grossDeltaMinor: null, grossDelta: null, currency: "CNY", occurredAt: "", note: "",
    recordedBy: "", createdAt: "", ...overrides,
  };
}

describe("roiDealGrossState (FR-025)", () => {
  it("has nothing to say without a basis", () => {
    expect(roiDealGrossState(deal({ grossBasis: "none" }), [adjustment({})])).toEqual({ kind: "no_basis" });
  });

  it("is not computable when a refund in effect left the gross change out", () => {
    expect(roiDealGrossState(deal({ grossBasis: "cogs" }), [adjustment({})])).toEqual({
      kind: "refund_without_gross_delta",
      adjustmentIds: ["a1"],
    });
    expect(roiDealGrossState(deal({}), [adjustment({ voided: true })])).toEqual({ kind: "stated", grossProfit: "3000.00" });
  });

  it("shows a stated figure as is, and leaves anything that needs a sum to the report", () => {
    expect(roiDealGrossState(deal({}), [])).toEqual({ kind: "stated", grossProfit: "3000.00" });
    expect(roiDealGrossState(deal({}), [adjustment({ grossDeltaMinor: "-30000", grossDelta: "-300.00" })])).toEqual({
      kind: "in_report",
    });
    expect(roiDealGrossState(deal({ grossBasis: "cogs", cogs: "7000.00" }), [])).toEqual({ kind: "in_report" });
  });
});
