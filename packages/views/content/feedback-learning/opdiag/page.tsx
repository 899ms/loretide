"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  useGenerateOpDiagReport,
  useOpDiagAnnotations,
  useOpDiagProfileProposals,
  useOpDiagReportVersion,
  useOpDiagReportVersions,
  useOpDiagReports,
  useOpDiagTodos,
  usePreviewOpDiag,
  useWriteOpDiagTodo,
  type OpDiagReportHeader,
  type OpDiagReportVersion,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { PageHeader } from "@multica/views/layout/page-header";
import { SettingsContent, SettingsTab } from "@multica/views/settings/layout";
import { DiagnosisDecisions } from "./decisions";
import { DiagnosisGeneration } from "./generation";
import { FollowUp } from "./follow-up";
import { DiagnosisReport } from "./report";
import { WorkMarks } from "./work-marks";
import type { OperatingDiagnosisOption, OperatingDiagnosisRoiReport, OperatingDiagnosisWork } from "./shared";

export type { OperatingDiagnosisOption, OperatingDiagnosisRoiReport, OperatingDiagnosisWork } from "./shared";

export interface OperatingDiagnosisPageProps {
  wsId: string;
  gapLinks: Record<string, string>;
  topicCards: Array<{ id: string; accountId: string | null; label: string }>;
  topicsLoading: boolean;
  topicsFailed: boolean;
  accountsLoading: boolean;
  accountsFailed: boolean;
  worksLoading: boolean;
  worksFailed: boolean;
  roiReportsLoading: boolean;
  roiReportsFailed: boolean;
  brandTimezone: string;
  accounts: OperatingDiagnosisOption[];
  works: OperatingDiagnosisWork[];
  roiReports: OperatingDiagnosisRoiReport[];
}

export function OperatingDiagnosisPage(props: OperatingDiagnosisPageProps) {
  const { t } = useT("common");
  const reports = useOpDiagReports(props.wsId);
  const [reportId, setReportId] = useState("");
  const [versionNo, setVersionNo] = useState(0);
  const selected = useOpDiagReportVersion(props.wsId, reportId, versionNo);
  const versions = useOpDiagReportVersions(props.wsId, reportId);
  const annotations = useOpDiagAnnotations(props.wsId, reportId, versionNo);
  const todos = useOpDiagTodos(props.wsId);
  const proposals = useOpDiagProfileProposals(props.wsId);
  const [preview, setPreview] = useState<OpDiagReportVersion["result"] | null>(null);
  const generate = useGenerateOpDiagReport(props.wsId);
  const previewMutation = usePreviewOpDiag();
  const addTodo = useWriteOpDiagTodo(props.wsId);
  const openReport = (id: string, version: number) => { setReportId(id); setVersionNo(version); setPreview(null); };
  const makeTodo = (report: OpDiagReportVersion, gap: OpDiagReportVersion["result"]["gaps"][number]) => addTodo.mutate({ path: "todos", body: { origin_report_id: report.reportId, origin_version_no: report.versionNo, origin_gap_key: gap.gapKey, title: gap.kind, note: "" } });
  const selectedReport = selected.data ?? null;
  const onGenerate = (title: string, params: Record<string, unknown>) => generate.mutate({ path: "reports", body: { title, params } }, { onSuccess: (result) => result && openReport(result.reportId, result.versionNo) });
  const latest = reports.data?.[0];
  return <>
    <PageHeader className="xl:hidden"><span className="text-body font-medium">{t(($) => $.contentOperatingDiagnosis.title)}</span></PageHeader>
    <SettingsContent><SettingsTab title={t(($) => $.contentOperatingDiagnosis.title)} description={t(($) => $.contentOperatingDiagnosis.description)}>
      <DiagnosisGeneration timezone={props.brandTimezone} accounts={props.accounts} accountsLoading={props.accountsLoading} accountsFailed={props.accountsFailed} roiReports={props.roiReports} roiReportsLoading={props.roiReportsLoading} roiReportsFailed={props.roiReportsFailed} busy={generate.isPending || previewMutation.isPending} onPreview={(params) => previewMutation.mutate({ params }, { onSuccess: setPreview })} onGenerate={onGenerate} />
      {preview && <DiagnosisReport report={null} result={preview} title={t(($) => $.contentOperatingDiagnosis.preview)} gapLinks={props.gapLinks} todos={todos.data ?? []} addTodo={() => undefined} onNewVersion={() => undefined} busy canAddTodo={false} />}
      <ReportsList loading={reports.isLoading} failed={reports.isError} reports={reports.data} versionsLoading={versions.isLoading} versionsFailed={versions.isError || (versions.isSuccess && !versions.data)} versions={versions.data?.versions} selectedId={reportId} selectedVersion={versionNo} onSelect={openReport} />
      {selected.isLoading && reportId && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p>}
      {(selected.isError || (selected.isSuccess && reportId && !selected.data)) && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p>}
      {selectedReport && <>
        {selectedReport.inputsChanged.changed && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.inputsChanged)}</p>}
        <DiagnosisReport report={selectedReport} result={selectedReport.result} title={selectedReport.title} gapLinks={props.gapLinks} todos={todos.data ?? []} addTodo={(gap) => makeTodo(selectedReport, gap)} onNewVersion={(report) => generate.mutate({ path: `reports/${encodeURIComponent(report.reportId)}/versions`, body: {} }, { onSuccess: (next) => next && openReport(next.reportId, next.versionNo) })} busy={generate.isPending || addTodo.isPending} canAddTodo={!todos.isLoading && !todos.isError} />
        <QueryState loading={annotations.isLoading} failed={annotations.isError} label={t(($) => $.contentOperatingDiagnosis.blocks.decisions)} />
        {annotations.isSuccess && !annotations.data && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p>}
        {annotations.data && <DiagnosisDecisions key={`${props.wsId}:${selectedReport.reportId}:${selectedReport.versionNo}`} wsId={props.wsId} report={selectedReport} accounts={props.accounts} annotations={annotations.data} topicCards={props.topicCards} topicsLoading={props.topicsLoading} topicsFailed={props.topicsFailed} />}
        <WorkMarks wsId={props.wsId} report={selectedReport} works={props.works} worksLoading={props.worksLoading} worksFailed={props.worksFailed} />
      </>}
      {addTodo.isError && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
      <FollowUp wsId={props.wsId} todos={todos.data ?? []} proposals={proposals.data ?? []} todosLoading={todos.isLoading} proposalsLoading={proposals.isLoading} todosFailed={todos.isError} proposalsFailed={proposals.isError} refreshTodos={() => { void todos.refetch(); }} refreshProposals={() => { void proposals.refetch(); }} />
      {generate.isError && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
      {previewMutation.isError && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
      {previewMutation.isSuccess && !previewMutation.data && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
      {generate.isSuccess && !generate.data && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
      {latest && !reportId && <div className="flex justify-end"><Button size="sm" variant="outline" onClick={() => openReport(latest.reportId, latest.versionNo)}>{t(($) => $.contentOperatingDiagnosisDetails.openLatest)}</Button></div>}
    </SettingsTab></SettingsContent>
  </>;
}

function ReportsList({ loading, failed, reports, versionsLoading, versionsFailed, versions, selectedId, selectedVersion, onSelect }: {
  loading: boolean; failed: boolean; reports: OpDiagReportHeader[] | undefined; versionsLoading: boolean; versionsFailed: boolean;
  versions: Array<{ reportId: string; versionNo: number; title: string }> | undefined; selectedId: string; selectedVersion: number;
  onSelect: (id: string, version: number) => void;
}) {
  const { t } = useT("common");
  return <section className="space-y-2"><h2 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.reports)}</h2>
    {loading && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p>}
    {failed && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p>}
    {!loading && !failed && reports?.length === 0 && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.empty)}</p>}
    {reports?.map((report) => <Button key={`${report.reportId}-${report.versionNo}`} size="sm" variant={selectedId === report.reportId && selectedVersion === report.versionNo ? "brand" : "outline"} onClick={() => onSelect(report.reportId, report.versionNo)}>{report.title || report.reportId} · {report.versionNo}</Button>)}
    {selectedId && <div className="flex flex-wrap gap-2">{versionsLoading && <span className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</span>}{versionsFailed && <span className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</span>}{versions?.map((version) => <Button key={version.versionNo} size="sm" variant="outline" onClick={() => onSelect(version.reportId, version.versionNo)}>{t(($) => $.contentOperatingDiagnosis.versions)} {version.versionNo}</Button>)}</div>}
  </section>;
}

function QueryState({ loading, failed, label }: { loading: boolean; failed: boolean; label: string }) {
  const { t } = useT("common");
  if (loading) return <p className="text-sm text-muted-foreground">{label}: {t(($) => $.contentOperatingDiagnosis.loading)}</p>;
  if (failed) return <p className="text-sm text-muted-foreground">{label}: {t(($) => $.contentOperatingDiagnosis.loadFailed)}</p>;
  return null;
}
