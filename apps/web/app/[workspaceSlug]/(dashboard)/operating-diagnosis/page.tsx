"use client";

import { useWorkspaceId } from "@multica/core/hooks";
import { paths, useCurrentWorkspace, useRequiredWorkspaceSlug } from "@multica/core/paths";
import { getWorkspaceTimezone } from "@multica/core/workspace";
import { useContentAccounts } from "@multica/core/content/ip-profile";
import { useContentWorks } from "@multica/core/content/work-editor";
import { useRoiReports } from "@multica/core/content/feedback-learning";
import { useContentTopics } from "@multica/core/content/topic-planning";
import { OperatingDiagnosisPage } from "@multica/views/content/feedback-learning";

export default function Page() {
  const wsId = useWorkspaceId(); const workspace = useCurrentWorkspace(); const slug = useRequiredWorkspaceSlug(); const workspacePaths = paths.workspace(slug);
  const gapLinks = { account_settings: workspacePaths.accounts(), operating_rules: workspacePaths.settings(), publications: workspacePaths.today(), feedback: workspacePaths.today(), works: workspacePaths.today(), work_marks: workspacePaths.operatingDiagnosis(), unknown: workspacePaths.operatingDiagnosis() };
  const accounts = useContentAccounts(wsId); const works = useContentWorks(wsId); const roiReports = useRoiReports(wsId); const topics = useContentTopics(wsId, "");
  return <OperatingDiagnosisPage key={wsId} wsId={wsId} gapLinks={gapLinks} brandTimezone={getWorkspaceTimezone(workspace ?? undefined)} accounts={(accounts.data ?? []).map((account) => ({ id: account.account_id, label: account.display_name || account.account_id }))} accountsLoading={accounts.isLoading} accountsFailed={accounts.isError} works={(works.data ?? []).map((work) => ({ id: work.workId, label: work.title || work.workId, historicalImport: work.historicalImport }))} worksLoading={works.isLoading} worksFailed={works.isError} roiReports={(roiReports.data ?? []).map((report) => ({ id: report.reportId, label: report.title || report.reportId, versionNo: report.versionNo }))} roiReportsLoading={roiReports.isLoading} roiReportsFailed={roiReports.isError} topicCards={(topics.data ?? []).map((topic) => ({ id: topic.topicCardId, accountId: topic.accountId, label: topic.audienceProblemJudgment }))} topicsLoading={topics.isLoading} topicsFailed={topics.isError} />;
}
