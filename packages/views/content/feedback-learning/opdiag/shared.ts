import { OPDIAG_DIMENSIONS, type OpDiagDimension } from "@multica/core/content/feedback-learning";

export interface OperatingDiagnosisOption { id: string; label: string; }
export interface OperatingDiagnosisWork extends OperatingDiagnosisOption { historicalImport: boolean; }
export interface OperatingDiagnosisRoiReport extends OperatingDiagnosisOption { versionNo: number; }

export interface DiagnosisDraft {
  scope: "account" | "brand";
  accountId: string;
  accountIds: string[];
  start: string;
  end: string;
  comparisonStart: string;
  comparisonEnd: string;
  dimensions: OpDiagDimension[];
  items: string;
  pillars: string;
  metrics: string;
  platforms: string;
  sources: string;
  roiReportId: string;
}

export const initialDiagnosisDraft: DiagnosisDraft = {
  scope: "account", accountId: "", accountIds: [], start: "", end: "", comparisonStart: "", comparisonEnd: "",
  dimensions: [...OPDIAG_DIMENSIONS], items: "", pillars: "", metrics: "", platforms: "", sources: "", roiReportId: "",
};

export function splitList(value: string): string[] {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

export function diagnosisParams(draft: DiagnosisDraft, timezone: string, roiReports: OperatingDiagnosisRoiReport[]): Record<string, unknown> {
  const roi = roiReports.find((report) => roiReportSelectionKey(report) === draft.roiReportId);
  return {
    scope: { kind: draft.scope, account_ids: draft.scope === "account" ? [draft.accountId] : draft.accountIds },
    window: { start: draft.start, end: draft.end, timezone },
    comparison_window: draft.comparisonStart && draft.comparisonEnd ? { start: draft.comparisonStart, end: draft.comparisonEnd } : null,
    dimensions: draft.dimensions.map((key) => {
      if (key === "consistency") return { key, items: splitList(draft.items) };
      if (key === "coverage") return { key, pillars: splitList(draft.pillars) };
      if (key === "performance") return { key, metrics: splitList(draft.metrics), platforms: splitList(draft.platforms) };
      if (key === "audience_feedback") return { key, sources: splitList(draft.sources) };
      return { key };
    }),
    roi_report_ref: roi ? { report_id: roi.id, version_no: roi.versionNo } : null,
  };
}

export function roiReportSelectionKey(report: OperatingDiagnosisRoiReport): string {
  return `${report.id}@${report.versionNo}`;
}
