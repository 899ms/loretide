"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  opdiagNumberDisplay,
  useDecideOpDiagSuggestion,
  useGenerateOpDiagReport,
  useOpDiagAnnotations,
  useOpDiagProfileProposals,
  useOpDiagReportVersion,
  useOpDiagReportVersions,
  useOpDiagReports,
  useOpDiagTodos,
  usePreviewOpDiag,
  useRecordOpDiagWorkMark,
  useSettleOpDiagProposal,
  useWriteOpDiagJudgement,
  useWriteOpDiagSuggestion,
  useWriteOpDiagTodo,
  type OpDiagDimension,
  type OpDiagDimensionResult, type OpDiagFactsByDimension,
  type OpDiagReportVersion,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@multica/ui/components/ui/select";
import { PageHeader } from "@multica/views/layout/page-header";
import { SettingsCard, SettingsContent, SettingsSection, SettingsTab } from "@multica/views/settings/layout";
import { DiagnosisGeneration } from "./generation";
import type { OperatingDiagnosisOption, OperatingDiagnosisRoiReport, OperatingDiagnosisWork } from "./shared";

export type { OperatingDiagnosisOption, OperatingDiagnosisRoiReport, OperatingDiagnosisWork } from "./shared";
export interface OperatingDiagnosisPageProps {
  wsId: string;
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
  const selectedReport = useOpDiagReportVersion(props.wsId, reportId, versionNo);
  const versions = useOpDiagReportVersions(props.wsId, reportId);
  const annotations = useOpDiagAnnotations(props.wsId, reportId, versionNo);
  const todos = useOpDiagTodos(props.wsId);
  const proposals = useOpDiagProfileProposals(props.wsId);
  const [preview, setPreview] = useState<OpDiagReportVersion["result"] | null>(null);
  const generate = useGenerateOpDiagReport(props.wsId);
  const previewReport = usePreviewOpDiag();
  const openReport = (id: string, version: number) => { setReportId(id); setVersionNo(version); setPreview(null); };
  return <>
    <PageHeader className="xl:hidden"><span className="text-body font-medium">{t(($) => $.contentOperatingDiagnosis.title)}</span></PageHeader>
    <SettingsContent><SettingsTab title={t(($) => $.contentOperatingDiagnosis.title)} description={t(($) => $.contentOperatingDiagnosis.description)}><DiagnosisGeneration timezone={props.brandTimezone} accounts={props.accounts} roiReports={props.roiReports} busy={generate.isPending || previewReport.isPending} onPreview={(params) => previewReport.mutate({ params }, { onSuccess: setPreview })} onGenerate={(title, params) => generate.mutate({ path: "reports", body: { title, params } }, { onSuccess: (result) => result && openReport(result.reportId, result.versionNo) })} />
    {preview && <ReportView title={t(($) => $.contentOperatingDiagnosis.preview)} result={preview} />}
    <ReportsBlock reports={reports.data ?? []} versions={versions.data?.versions ?? []} selected={selectedReport.data ?? null} onOpen={openReport} />
    <ReportView title={selectedReport.data?.title ?? ""} result={selectedReport.data?.result ?? null} />
    {selectedReport.data && <ActionsBlock wsId={props.wsId} report={selectedReport.data} accounts={props.accounts} works={props.works} annotations={annotations.data ?? null} />}
    <FollowUpBlock wsId={props.wsId} report={selectedReport.data ?? null} todos={todos.data ?? []} proposals={proposals.data ?? []} /></SettingsTab></SettingsContent>
  </>;
}

function ReportsBlock({ reports, versions, selected, onOpen }: { reports: { reportId: string; versionNo: number; title: string }[]; versions: { reportId: string; versionNo: number; title: string }[]; selected: OpDiagReportVersion | null; onOpen: (id: string, version: number) => void }) {
  const { t } = useT("common");
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.reports)}><SettingsCard><div className="space-y-2">{reports.length === 0 && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.empty)}</p>}{reports.map((report) => <div className="flex flex-wrap items-center justify-between gap-2" key={report.reportId}><span>{report.title || report.reportId}</span><Button size="sm" variant={selected?.reportId === report.reportId ? "brand" : "outline"} onClick={() => onOpen(report.reportId, report.versionNo)}>{t(($) => $.contentOperatingDiagnosis.versions)} {report.versionNo}</Button></div>)}{versions.map((version) => <Button key={`${version.reportId}-${version.versionNo}`} size="sm" variant="outline" onClick={() => onOpen(version.reportId, version.versionNo)}>{version.title || version.reportId} · {version.versionNo}</Button>)}</div></SettingsCard></SettingsSection>;
}

function ReportView({ title, result }: { title: string; result: OpDiagReportVersion["result"] | null }) {
  const { t } = useT("common");
  if (!result) return null;
  const dimensionLabel = (key: OpDiagDimension) => t(($) => $.contentOperatingDiagnosis.dimension[key]);
  const sectionLabel = (section: string) => section === "account" ? t(($) => $.contentOperatingDiagnosis.sections.account) : section === "brand" ? t(($) => $.contentOperatingDiagnosis.sections.brand) : t(($) => $.contentOperatingDiagnosis.sections.unknown_account);
  const ruleLabel = (rule: string) => rule === "common.no_cross_platform_ranking" ? t(($) => $.contentOperatingDiagnosis.rulesMap["common.no_cross_platform_ranking"]) : rule === "common.no_score" ? t(($) => $.contentOperatingDiagnosis.rulesMap["common.no_score"]) : rule === "common.window_in_brand_timezone" ? t(($) => $.contentOperatingDiagnosis.rulesMap["common.window_in_brand_timezone"]) : rule === "performance.difference_is_not_cause" ? t(($) => $.contentOperatingDiagnosis.rulesMap["performance.difference_is_not_cause"]) : t(($) => $.contentOperatingDiagnosis.unknown);
  return <SettingsSection title={title || t(($) => $.contentOperatingDiagnosis.reports)}><SettingsCard><div className="space-y-4"><p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.manualOnly)} · {result.calcVersion}</p>{result.sections.map((section, index) => <div className="space-y-2" key={`${section.section}-${section.accountId}-${index}`}><h3 className="text-sm font-medium">{sectionLabel(section.section)}</h3>{Object.entries(section.dimensions).map(([key, dimension]) => <DimensionView key={key} name={dimensionLabel(key as OpDiagDimension)} dimension={dimension} />)}</div>)}<List label={t(($) => $.contentOperatingDiagnosis.gaps)} items={result.gaps.map((gap) => `${gap.kind}: ${gap.ref.kind} ${gap.ref.id}`)} /><List label={t(($) => $.contentOperatingDiagnosis.rules)} items={result.rules.map(ruleLabel)} /><List label={t(($) => $.contentOperatingDiagnosis.refs)} items={result.refs} /></div></SettingsCard></SettingsSection>;
}

function DimensionView({ name, dimension }: { name: string; dimension: OpDiagDimensionResult | undefined }) {
  const { t } = useT("common");
  if (!dimension || dimension.status === "unknown") return <p className="text-sm text-muted-foreground">{name}: {t(($) => $.contentOperatingDiagnosis.unknown)}</p>;
  const reasonLabel = dimension.status === "not_computable" && dimension.reason === "missing_config" ? t(($) => $.contentOperatingDiagnosis.reasons.missing_config) : dimension.status === "not_computable" && dimension.reason === "missing_comparison_window" ? t(($) => $.contentOperatingDiagnosis.reasons.missing_comparison_window) : dimension.status === "not_computable" && dimension.reason === "missing_observation_window" ? t(($) => $.contentOperatingDiagnosis.reasons.missing_observation_window) : dimension.status === "not_computable" && dimension.reason === "no_delivery_channel" ? t(($) => $.contentOperatingDiagnosis.reasons.no_delivery_channel) : t(($) => $.contentOperatingDiagnosis.reasons.no_data);
  if (dimension.status === "not_computable") return <p className="text-sm text-muted-foreground">{name}: {t(($) => $.contentOperatingDiagnosis.notComputable)} · {reasonLabel}</p>;
  const performance = "groups" in dimension.facts ? dimension.facts.groups as OpDiagFactsByDimension["performance"]["groups"] : [];
  return <div className="rounded-md border p-3 text-sm"><p className="font-medium">{name} · {t(($) => $.contentOperatingDiagnosis.status)}</p>{performance.map((group) => <p className="mt-1 text-muted-foreground" key={`${group.platform}-${group.metric}`}>{group.platform} · {group.metric}: {opdiagNumberDisplay(group.change) ?? t(($) => $.contentOperatingDiagnosis.notComputable)}</p>)}{performance.length === 0 && <p className="mt-1 text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.facts)}: {dimension.records.length}</p>}</div>;
}

function List({ label, items }: { label: string; items: string[] }) { return <div><p className="text-sm font-medium">{label}</p>{items.length ? <ul className="mt-1 list-disc pl-5 text-sm text-muted-foreground">{items.map((item, index) => <li key={`${item}-${index}`}>{item}</li>)}</ul> : <p className="text-sm text-muted-foreground">—</p>}</div>; }

function ActionsBlock({ wsId, report, accounts, works, annotations }: { wsId: string; report: OpDiagReportVersion; accounts: OperatingDiagnosisOption[]; works: OperatingDiagnosisWork[]; annotations: ReturnType<typeof useOpDiagAnnotations>["data"] }) {
  const { t } = useT("common"); const mark = useRecordOpDiagWorkMark(wsId); const judgement = useWriteOpDiagJudgement(wsId); const suggestion = useWriteOpDiagSuggestion(wsId); const decide = useDecideOpDiagSuggestion(wsId);
  const [workId, setWorkId] = useState(""); const [accountId, setAccountId] = useState(""); const [item, setItem] = useState(""); const [body, setBody] = useState("");
  const path = `reports/${encodeURIComponent(report.reportId)}/versions/${report.versionNo}`;
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.blocks.marks)}><SettingsCard><div className="space-y-3"><Select items={works.map((work) => ({ label: work.label, value: work.id }))} value={workId} onValueChange={(value) => setWorkId(value ?? "")}><SelectTrigger><SelectValue placeholder={t(($) => $.contentOperatingDiagnosis.work)} /></SelectTrigger><SelectContent>{works.map((work) => <SelectItem value={work.id} key={work.id}>{work.label}</SelectItem>)}</SelectContent></Select><Select items={accounts.map((account) => ({ label: account.label, value: account.id }))} value={accountId} onValueChange={(value) => setAccountId(value ?? "")}><SelectTrigger><SelectValue placeholder={t(($) => $.contentOperatingDiagnosis.account)} /></SelectTrigger><SelectContent>{accounts.map((account) => <SelectItem value={account.id} key={account.id}>{account.label}</SelectItem>)}</SelectContent></Select><Input value={item} onChange={(event) => setItem(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosis.markItem)} /><Button size="sm" variant="outline" disabled={!workId || mark.isPending} onClick={() => mark.mutate({ path: "work-marks", body: { work_id: workId, account_id: accountId, kind: "consistency", item, verdict: "unsure", note: "" } })}>{t(($) => $.contentOperatingDiagnosis.saveMark)}</Button></div></SettingsCard><SettingsCard><div className="space-y-3"><Textarea value={body} onChange={(event) => setBody(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosis.body)} /><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => judgement.mutate({ path: `${path}/judgements`, body: { kind: "judgement", basis: "qualitative", evidence_refs: [], body } })}>{t(($) => $.contentOperatingDiagnosis.saveJudgement)}</Button><Button size="sm" variant="outline" onClick={() => suggestion.mutate({ path: `${path}/suggestions`, body: { body, target_kind: "todo", target: {}, judgement_ids: [], evidence_refs: [] } })}>{t(($) => $.contentOperatingDiagnosis.saveSuggestion)}</Button></div>{annotations?.suggestions.map((entry) => <div className="flex flex-wrap gap-2 text-sm" key={entry.suggestionId}><span>{entry.body}</span><Button size="sm" variant="outline" onClick={() => decide.mutate({ path: `suggestions/${entry.suggestionId}/decisions`, body: { suggestion_revision: entry.revision, decision: "adopt", mode: "create", link_target_id: "", note: "" } })}>{t(($) => $.contentOperatingDiagnosis.adopt)}</Button><Button size="sm" variant="outline" onClick={() => decide.mutate({ path: `suggestions/${entry.suggestionId}/decisions`, body: { suggestion_revision: entry.revision, decision: "reject", mode: "", link_target_id: "", note: "" } })}>{t(($) => $.contentOperatingDiagnosis.reject)}</Button></div>)}</div></SettingsCard></SettingsSection>;
}

function FollowUpBlock({ wsId, report, todos, proposals }: { wsId: string; report: OpDiagReportVersion | null; todos: ReturnType<typeof useOpDiagTodos>["data"]; proposals: ReturnType<typeof useOpDiagProfileProposals>["data"] }) {
  const { t } = useT("common"); const todo = useWriteOpDiagTodo(wsId); const settle = useSettleOpDiagProposal(wsId);
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.blocks.followUp)}><SettingsCard><List label={t(($) => $.contentOperatingDiagnosis.todos)} items={(todos ?? []).map((entry) => entry.title)} />{report?.result.gaps.map((gap) => <Button key={gap.gapKey} size="sm" variant="outline" onClick={() => todo.mutate({ path: "todos", body: { origin_report_id: report.reportId, origin_version_no: report.versionNo, origin_gap_key: gap.gapKey, title: gap.kind, note: "" } })}>{t(($) => $.contentOperatingDiagnosis.createTodo)}</Button>)}</SettingsCard><SettingsCard><List label={t(($) => $.contentOperatingDiagnosis.proposals)} items={(proposals ?? []).map((entry) => entry.items.map((item) => `${item.field}: ${item.currentValue} → ${item.proposedValue}`).join(", "))} />{(proposals ?? []).map((entry) => <div className="flex gap-2" key={entry.proposalId}><Button size="sm" variant="outline" disabled={!entry.baseIsCurrent} onClick={() => settle.mutate({ path: `profile-proposals/${entry.proposalId}/confirm`, body: { base_revision_id: entry.baseRevisionId } })}>{t(($) => $.contentOperatingDiagnosis.confirm)}</Button><Button size="sm" variant="outline" onClick={() => settle.mutate({ path: `profile-proposals/${entry.proposalId}/dismiss`, body: {} })}>{t(($) => $.contentOperatingDiagnosis.dismiss)}</Button></div>)}</SettingsCard></SettingsSection>;
}
