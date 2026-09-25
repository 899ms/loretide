// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  OPDIAG_DATA_ORIGINS,
  OPDIAG_DIMENSIONS,
  OPDIAG_MARK_KINDS,
  OPDIAG_MARK_VERDICTS,
  OPDIAG_SCOPES,
  opdiagPath,
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
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goContract);
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
