// @vitest-environment node

import { describe, expect, it } from "vitest";
import { diagnosisParams, initialDiagnosisDraft, roiReportSelectionKey, suggestionRequest, topicCardsForTargetAccount } from "./shared";

describe("operating diagnosis request params", () => {
  it("preserves the selected ROI report version and every dimension config", () => {
    const roiReports = [
      { id: "roi-a", label: "ROI", versionNo: 1 },
      { id: "roi-a", label: "ROI", versionNo: 2 },
    ];
    const params = diagnosisParams({
      ...initialDiagnosisDraft,
      accountId: "account-a",
      start: "2026-01-01",
      end: "2026-01-31",
      comparisonStart: "2025-12-01",
      comparisonEnd: "2025-12-31",
      items: "positioning, questions",
      pillars: "education, case-study",
      metrics: "views, saves",
      platforms: "xiaohongshu, douyin",
      sources: "comment, lead",
      roiReportId: roiReportSelectionKey(roiReports[1]),
    }, "Asia/Shanghai", roiReports);

    expect(params.roi_report_ref).toEqual({ report_id: "roi-a", version_no: 2 });
    expect(params.dimensions).toEqual([
      { key: "consistency", items: ["positioning", "questions"] },
      { key: "coverage", pillars: ["education", "case-study"] },
      { key: "cadence" },
      { key: "performance", metrics: ["views", "saves"], platforms: ["xiaohongshu", "douyin"] },
      { key: "audience_feedback", sources: ["comment", "lead"] },
      { key: "execution_flow" },
    ]);
  });
});

describe("operating diagnosis suggestion construction", () => {
  it("uses only exact account matches for linking topic cards", () => {
    const cards = [
      { id: "a", accountId: "account-a", label: "A" },
      { id: "b", accountId: "account-b", label: "B" },
      { id: "unassigned", accountId: null, label: "Unassigned" },
    ];
    expect(topicCardsForTargetAccount(cards, "account-a").map(({ id }) => id)).toEqual(["a"]);
    expect(topicCardsForTargetAccount(cards, "account-b").map(({ id }) => id)).toEqual(["b"]);
    expect(topicCardsForTargetAccount(cards, null).map(({ id }) => id)).toEqual(["unassigned"]);
  });

  it("includes selected current-version judgement IDs in the request", () => {
    expect(suggestionRequest("Try this", "todo", { account_id: "a1", title: "Review" }, ["j1", "j3"], ["work:w1"]))
      .toEqual({ body: "Try this", target_kind: "todo", target: { account_id: "a1", title: "Review" }, judgement_ids: ["j1", "j3"], evidence_refs: ["work:w1"] });
  });
});
