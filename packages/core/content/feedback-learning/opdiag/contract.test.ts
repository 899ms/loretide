// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  OPDIAG_DATA_ORIGINS,
  OPDIAG_DIMENSION_REASONS,
  OPDIAG_DIMENSIONS,
  OPDIAG_GAP_KINDS,
  OPDIAG_MARK_KINDS,
  OPDIAG_MARK_VERDICTS,
  OPDIAG_RULE_IDS,
  OPDIAG_SCOPES,
  opdiagPath,
  parseOpDiagDimension,
  parseOpDiagPreview,
  parseOpDiagReportList,
  parseOpDiagReportVersion,
  parseOpDiagReportVersionList,
  parseOpDiagWorkMark,
  parseOpDiagWorkMarkList,
} from "./contract";

// specs/035 PR 1 (T032). The sets are held to the Go source, and every
// schema has a malformed-response case.

const goDir = join(__dirname, "../../../../../server/internal/content/feedback-learning");
const goContract = readFileSync(join(goDir, "opdiag_contract.go"), "utf8");

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goContract);
  if (!declaration) throw new Error(`${varName} not found in opdiag_contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    // Typed constants (`Name Type = "v"`) and untyped ones (`name = "v"`).
    const value = new RegExp(`\\b${name}\\s+(?:[A-Za-z]+\\s+)?=\\s+"([^"]+)"`).exec(goContract);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("controlled sets", () => {
  it("are the Go sets, in order", () => {
    expect([...OPDIAG_DIMENSIONS]).toEqual(goSetValues("DiagnosisDimensions"));
    expect([...OPDIAG_SCOPES]).toEqual(goSetValues("DiagnosisScopes"));
    expect([...OPDIAG_MARK_KINDS]).toEqual(goSetValues("MarkKinds"));
    expect([...OPDIAG_MARK_VERDICTS]).toEqual(goSetValues("MarkVerdicts"));
    expect([...OPDIAG_DATA_ORIGINS]).toEqual(goSetValues("DataOrigins"));
    expect([...OPDIAG_DIMENSION_REASONS]).toEqual(goSetValues("DimensionReasons"));
    expect([...OPDIAG_GAP_KINDS]).toEqual(goSetValues("GapKinds"));
    expect([...OPDIAG_RULE_IDS]).toEqual(goSetValues("DiagnosisRuleIDs"));
  });

  it("encodes each path segment", () => {
    expect(opdiagPath("reports", "a/b", "versions")).toBe("reports/a%2Fb/versions");
  });
});

const headerWire = {
  report_id: "r1", version_no: 1, scope_kind: "brand", account_ids: ["a1", "a2"], title: "九月诊断",
  calc_version: "opdiag-calc/1", created_by: "u1", created_at: "2026-10-02T01:00:00Z",
};

const resultWire = {
  calc_version: "opdiag-calc/1",
  data_origin: "manual_only",
  scope: {
    kind: "brand",
    accounts: [
      { account_id: "a2", platform: "douyin", display_name: "副号", profile_revision_id: "" },
      { account_id: "a1", platform: "xiaohongshu", display_name: "品牌主号", profile_revision_id: "pr9" },
    ],
    window: { start: "2026-09-01", end: "2026-09-30", timezone: "Asia/Shanghai" },
    comparison_window: null,
    input_counts: { publications: 4, works: 4, metrics: 1, excerpts: 1, work_marks: 1, reviews: 2, delivery_tasks: 1 },
    historical_import_publications: 1,
  },
  sections: [
    { section: "account", account_id: "a2", dimensions: {} },
    { section: "account", account_id: "a1", dimensions: {} },
    { section: "unknown_account", dimensions: {} },
    { section: "brand", dimensions: {} },
  ],
  gaps: [{
    gap_key: "scope/profile_field_pending/profile_field/a1/positioning", kind: "profile_field_pending",
    dimension: "", ref: { kind: "profile_field", id: "a1/positioning" }, account_id: "a1", fix_route: "account_settings",
  }],
  roi_reference: null,
  rules: ["common.unknown_is_not_zero", "scope.historical_import_account_unknown"],
  refs: ["scope", "gap/scope/profile_field_pending/profile_field/a1/positioning"],
};

const versionWire = {
  ...headerWire,
  params: { scope: { kind: "brand", account_ids: ["a1", "a2"] }, window: { start: "2026-09-01", end: "2026-09-30", timezone: "Asia/Shanghai" } },
  inputs: [{ kind: "account", id: "a1", fingerprint: "f", fields: {} }],
  result: resultWire,
  inputs_changed: { changed: true, added: [{ kind: "work_mark", id: "m1" }], modified: [], removed: [] },
  ai_judgement_state: "pending_data",
};

describe("a stored diagnosis version", () => {
  it("reads the stored scope, sections, gaps and the derived change list", () => {
    const version = parseOpDiagReportVersion(versionWire);
    expect(version?.versionNo).toBe(1);
    expect(version?.scopeKind).toBe("brand");
    expect(version?.result.dataOrigin).toBe("manual_only");
    expect(version?.result.scope.accounts.map((a) => a.accountId)).toEqual(["a2", "a1"]);
    expect(version?.result.scope.inputCounts.workMarks).toBe(1);
    expect(version?.result.sections.map((s) => `${s.section}:${s.accountId}`))
      .toEqual(["account:a2", "account:a1", "unknown_account:", "brand:"]);
    expect(version?.result.gaps[0]?.fixRoute).toBe("account_settings");
    expect(version?.result.refs[0]).toBe("scope");
    expect(version?.inputsChanged).toEqual({ changed: true, added: [{ kind: "work_mark", id: "m1" }], modified: [], removed: [] });
  });

  it("never reads an AI judgement state other than pending_data", () => {
    expect(parseOpDiagReportVersion({ ...versionWire, ai_judgement_state: "generated" })?.aiJudgementState).toBe("pending_data");
    expect(parseOpDiagReportVersion({ ...versionWire, ai_judgement_state: undefined })?.aiJudgementState).toBe("pending_data");
  });

  it("reads a value outside a set as unknown, not as a known one", () => {
    const version = parseOpDiagReportVersion({
      ...versionWire, scope_kind: "region",
      result: { ...resultWire, data_origin: "simulated", sections: [{ section: "ranking", dimensions: {} }] },
    });
    expect(version?.scopeKind).toBe("unknown");
    expect(version?.result.dataOrigin).toBe("unknown");
    expect(version?.result.sections[0]?.section).toBe("unknown");
  });

  it("degrades malformed versions and lists", () => {
    expect(parseOpDiagReportVersion({ ...versionWire, version_no: "1" })).toBeNull();
    expect(parseOpDiagReportVersion({ ...versionWire, inputs_changed: true })).toBeNull();
    expect(parseOpDiagReportVersion({ ...versionWire, inputs_changed: { changed: "yes" } })).toBeNull();
    expect(parseOpDiagReportVersion({
      ...versionWire,
      result: { ...resultWire, scope: { ...resultWire.scope, input_counts: { ...resultWire.scope.input_counts, metrics: "1" } } },
    })).toBeNull();
    expect(parseOpDiagReportVersion({ ...versionWire, result: undefined })).toBeNull();
    expect(parseOpDiagReportVersion(null)).toBeNull();

    expect(parseOpDiagReportList({ reports: [headerWire] })).toEqual([{
      reportId: "r1", versionNo: 1, scopeKind: "brand", accountIds: ["a1", "a2"], title: "九月诊断",
      calcVersion: "opdiag-calc/1", createdBy: "u1", createdAt: "2026-10-02T01:00:00Z",
    }]);
    expect(parseOpDiagReportList({ reports: [{ ...headerWire, version_no: 1.5 }] })).toEqual([]);
    expect(parseOpDiagReportList("x")).toEqual([]);

    expect(parseOpDiagReportVersionList({ report_id: "r1", versions: [headerWire, { ...headerWire, version_no: 2 }] })?.versions
      .map((v) => v.versionNo)).toEqual([1, 2]);
    expect(parseOpDiagReportVersionList({ versions: [headerWire] })).toBeNull();
  });
});

const markWire = {
  mark_id: "m1", workspace_id: "ws", work_id: "w1", kind: "consistency", item: "positioning", verdict: "unsure",
  account_id: "a1", profile_revision_id: "pr9", note: "", recorded_by: "u1", created_at: "2026-09-12T02:00:00Z",
};

describe("work marks", () => {
  it("read the mark and the profile revision the server recorded", () => {
    expect(parseOpDiagWorkMark(markWire)).toEqual({
      markId: "m1", workId: "w1", kind: "consistency", item: "positioning", verdict: "unsure",
      accountId: "a1", profileRevisionId: "pr9", note: "", recordedBy: "u1", createdAt: "2026-09-12T02:00:00Z",
    });
    expect(parseOpDiagWorkMark({ ...markWire, verdict: "great" })?.verdict).toBe("unknown");
  });

  it("degrade malformed marks and lists", () => {
    expect(parseOpDiagWorkMark({ ...markWire, work_id: 1 })).toBeNull();
    expect(parseOpDiagWorkMark({ ...markWire, item: undefined })).toBeNull();
    expect(parseOpDiagWorkMarkList({ marks: [markWire, markWire] })).toHaveLength(2);
    expect(parseOpDiagWorkMarkList({ marks: [{ ...markWire, mark_id: null }] })).toEqual([]);
    expect(parseOpDiagWorkMarkList(undefined)).toEqual([]);
  });
});

// ---------------------------------------------------------------- PR 2: dimensions (T054)

const common = { completeness: { expected: 2, present: 1, gap_keys: ["performance/metric_missing/publication/p2"] },
  limits: ["performance.difference_is_not_cause"], records: [{ kind: "publication", id: "p1" }] };

const performanceWire = {
  status: "ok", ...common,
  facts: { groups: [{
    platform: "xiaohongshu", metric: "favorite",
    current: { publications: 2, with_value: 1, unknown: 1, sum: "0", mean: { status: "ok", value: "0", display: "0.00" } },
    baseline: { publications: 0, with_value: 0, unknown: 0, sum: null, mean: { status: "not_computable", reason: "no_data" } },
    change: { status: "not_computable", reason: "no_data" },
    stat_windows: ["发布后 7 天"], stat_window_mixed: false,
  }] },
};

describe("a dimension", () => {
  it("reads performance with its strings as the server wrote them, and an unknown sum as null", () => {
    const dimension = parseOpDiagDimension("performance", performanceWire);
    expect(dimension.status).toBe("ok");
    if (dimension.status !== "ok") return;
    const group = (dimension.facts as { groups: { current: { sum: string | null; mean: unknown }; baseline: { sum: string | null } }[] }).groups[0]!;
    expect(group.current.sum).toBe("0");
    expect(group.current.mean).toEqual({ status: "ok", value: "0", display: "0.00" });
    expect(group.baseline.sum).toBeNull();
    expect(dimension.completeness.gapKeys).toEqual(["performance/metric_missing/publication/p2"]);
  });

  it("reads not_computable with its reason, and an unknown reason as unknown", () => {
    expect(parseOpDiagDimension("coverage", { status: "not_computable", reason: "missing_config", ...common }))
      .toMatchObject({ status: "not_computable", reason: "missing_config" });
    expect(parseOpDiagDimension("coverage", { status: "not_computable", reason: "too_low", ...common }))
      .toMatchObject({ status: "not_computable", reason: "unknown" });
  });

  it("reads a cadence target that is set without a number as not set, never as 0", () => {
    const cadence = parseOpDiagDimension("cadence", { status: "ok", ...common, facts: {
      channels: [{ channel: "douyin", target: { set: true }, weeks: [
        { iso_week: "2026-W37", start: "2026-09-07", complete: true, published: 0, compared: true, met: true },
      ] }, { channel: "wechat_mp", target: { set: true, per_week: 0 }, weeks: [] }],
      published_at_missing: 0,
    } });
    expect(cadence.status).toBe("ok");
    if (cadence.status !== "ok") return;
    const channels = (cadence.facts as { channels: { target: unknown }[] }).channels;
    expect(channels[0]!.target).toEqual({ set: false });
    expect(channels[1]!.target).toEqual({ set: true, perWeek: 0 });
  });

  it("degrades a malformed dimension to unknown and keeps the rest of the report", () => {
    expect(parseOpDiagDimension("performance", { ...performanceWire, facts: { groups: [{ platform: "xiaohongshu" }] } }))
      .toEqual({ status: "unknown" });
    expect(parseOpDiagDimension("performance", { ...performanceWire, status: "great" })).toEqual({ status: "unknown" });
    expect(parseOpDiagDimension("performance", { ...performanceWire, completeness: { expected: "2" } })).toEqual({ status: "unknown" });
    expect(parseOpDiagDimension("ranking", performanceWire)).toEqual({ status: "unknown" });
    // A mean the server wrote as a number is not read as one.
    const numeric = structuredClone(performanceWire);
    (numeric.facts.groups[0]!.current.mean as unknown) = { status: "ok", value: 0, display: 0 };
    expect(parseOpDiagDimension("performance", numeric)).toEqual({ status: "unknown" });

    const version = parseOpDiagReportVersion({
      ...versionWire,
      result: { ...resultWire, sections: [{ section: "account", account_id: "a1", dimensions: {
        performance: { ...performanceWire, facts: "broken" }, audience_feedback: {
          status: "ok", ...common, facts: { excerpts: 1, by_source: [], by_tag: [{ tag: "价格", excerpts: 1 }], untagged: 0 },
        },
      } }] },
    });
    expect(version?.result.sections[0]?.dimensions.performance).toEqual({ status: "unknown" });
    expect(version?.result.sections[0]?.dimensions.audience_feedback?.status).toBe("ok");
  });
});

describe("a preview", () => {
  it("reads the result and the ROI reference as it is", () => {
    const roi = { report_id: "roi-1", version_no: 3, metrics: { attributed_net_revenue: { display: "1,200.00" } } };
    const preview = parseOpDiagPreview({ ...resultWire, roi_reference: roi, rules: ["roi_reference.shown_as_is"] });
    expect(preview?.roiReference).toEqual(roi);
    expect(preview?.gaps[0]?.kind).toBe("profile_field_pending");
    expect(parseOpDiagPreview({ ...resultWire, gaps: [{ ...resultWire.gaps[0], kind: "vibes" }] })?.gaps[0]?.kind).toBe("unknown");
  });

  it("degrades a malformed preview", () => {
    expect(parseOpDiagPreview({ ...resultWire, calc_version: 1 })).toBeNull();
    expect(parseOpDiagPreview({ ...resultWire, roi_reference: "x" })).toBeNull();
    expect(parseOpDiagPreview(undefined)).toBeNull();
  });
});
