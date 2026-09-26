"use client";

import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace } from "@multica/core/paths";
import { getWorkspaceTimezone } from "@multica/core/workspace";
import { useContentAccounts } from "@multica/core/content/ip-profile";
import { useContentWorks } from "@multica/core/content/work-editor";
import { useRoiReports } from "@multica/core/content/feedback-learning";
import { OperatingDiagnosisPage } from "@multica/views/content/feedback-learning";

export default function Page() {
  const wsId = useWorkspaceId(); const workspace = useCurrentWorkspace();
  const accounts = useContentAccounts(wsId); const works = useContentWorks(wsId); const roiReports = useRoiReports(wsId);
  return <OperatingDiagnosisPage key={wsId} wsId={wsId} brandTimezone={getWorkspaceTimezone(workspace ?? undefined)} accounts={(accounts.data ?? []).map((account) => ({ id: account.account_id, label: account.display_name || account.account_id }))} works={(works.data ?? []).map((work) => ({ id: work.workId, label: work.title || work.workId, historicalImport: work.historicalImport }))} roiReports={(roiReports.data ?? []).map((report) => ({ id: report.reportId, label: report.title || report.reportId, versionNo: report.versionNo }))} />;
}
