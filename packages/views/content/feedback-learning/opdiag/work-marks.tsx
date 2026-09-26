"use client";

import { useMemo, useState } from "react";
import { useOpDiagWorkMarks, useRecordOpDiagWorkMark, type OpDiagMarkVerdict, type OpDiagReportVersion, type OpDiagWorkMark } from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsSection } from "@multica/views/settings/layout";
import { useT } from "@multica/views/i18n";
import type { OperatingDiagnosisWork } from "./shared";

export function WorkMarks({ wsId, report, works, worksLoading, worksFailed }: { wsId: string; report: OpDiagReportVersion; works: OperatingDiagnosisWork[]; worksLoading: boolean; worksFailed: boolean }) {
  const { t } = useT("common");
  const marks = useOpDiagWorkMarks(wsId);
  const record = useRecordOpDiagWorkMark(wsId);
  const [note, setNote] = useState("");
  const inScope = useMemo(() => scopeWorks(report), [report]);
  const rows = inScope.map(({ workId, accountId }) => ({ work: works.find((work) => work.id === workId), workId, accountId })).filter((entry) => entry.work);
  const markLatest = new Map<string, OpDiagWorkMark>();
  for (const mark of marks.data ?? []) {
    const key = `${mark.workId}/${mark.kind}/${mark.item}`;
    const previous = markLatest.get(key);
    if (!previous || mark.createdAt > previous.createdAt) markLatest.set(key, mark);
  }
  const consistencyItems: string[] = reportItems(report, "consistency", "items");
  const pillars: string[] = reportItems(report, "coverage", "pillars");
  const labelVerdict = (verdict: string) => verdict === "consistent" ? t(($) => $.contentOperatingDiagnosisDetails.consistent) : verdict === "inconsistent" ? t(($) => $.contentOperatingDiagnosisDetails.inconsistent) : verdict === "unsure" ? t(($) => $.contentOperatingDiagnosisDetails.unsure) : t(($) => $.contentOperatingDiagnosis.unknown);
  const submit = (workId: string, accountId: string, kind: "pillar" | "consistency", item: string, verdict: OpDiagMarkVerdict) => record.mutate({ path: "work-marks", body: { work_id: workId, account_id: accountId, kind, item, verdict, note } });
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.blocks.marks)}><SettingsCard>
    {worksLoading || marks.isLoading ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p> : worksFailed || marks.isError ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p> : rows.length === 0 ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.noWindowWorks)}</p> : <div className="space-y-4">
      <Textarea value={note} onChange={(event) => setNote(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosis.note)} />
      {rows.map(({ work, workId, accountId }) => <article className="space-y-3 border-t pt-3" key={workId}><h3 className="text-sm font-medium">{work!.label}{work!.historicalImport ? ` · ${t(($) => $.contentOperatingDiagnosisDetails.historicalImport)}` : ""}</h3>
        {pillars.map((pillar) => { const key = `${workId}/pillar/${pillar}`; const latest = markLatest.get(key); return <label className="flex items-center gap-2 text-sm" key={pillar}><Checkbox checked={latest?.verdict === "tagged"} disabled={record.isPending || !accountId} onCheckedChange={(checked) => submit(workId, accountId, "pillar", pillar, checked === true ? "tagged" : "untagged")} />{pillar} · {latest?.verdict === "tagged" ? t(($) => $.contentOperatingDiagnosisDetails.tagged) : latest?.verdict === "untagged" ? t(($) => $.contentOperatingDiagnosisDetails.untagged) : latest ? t(($) => $.contentOperatingDiagnosis.unknown) : t(($) => $.contentOperatingDiagnosisDetails.unmarked)}</label>; })}
        {consistencyItems.map((item) => { const key = `${workId}/consistency/${item}`; const latest = markLatest.get(key); return <div className="grid gap-2 sm:grid-cols-[1fr_14rem]" key={item}><span className="text-sm">{item} · {latest ? labelVerdict(latest.verdict) : t(($) => $.contentOperatingDiagnosisDetails.unmarked)}</span><Select items={[{ label: t(($) => $.contentOperatingDiagnosisDetails.consistent), value: "consistent" }, { label: t(($) => $.contentOperatingDiagnosisDetails.inconsistent), value: "inconsistent" }, { label: t(($) => $.contentOperatingDiagnosisDetails.unsure), value: "unsure" }]} value={latest?.verdict && ["consistent", "inconsistent", "unsure"].includes(latest.verdict) ? latest.verdict : ""} onValueChange={(value) => { if (value) submit(workId, accountId, "consistency", item, value as OpDiagMarkVerdict); }}><SelectTrigger disabled={record.isPending || !accountId}><SelectValue placeholder={latest && !["consistent", "inconsistent", "unsure"].includes(latest.verdict) ? t(($) => $.contentOperatingDiagnosis.unknown) : t(($) => $.contentOperatingDiagnosis.mark)} /></SelectTrigger><SelectContent><SelectItem value="consistent">{t(($) => $.contentOperatingDiagnosisDetails.consistent)}</SelectItem><SelectItem value="inconsistent">{t(($) => $.contentOperatingDiagnosisDetails.inconsistent)}</SelectItem><SelectItem value="unsure">{t(($) => $.contentOperatingDiagnosisDetails.unsure)}</SelectItem></SelectContent></Select></div>; })}
      </article>)}
      {record.isError && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.failed)}</p>}
    </div>}
  </SettingsCard></SettingsSection>;
}

function scopeWorks(report: OpDiagReportVersion) {
  const scoped = new Map<string, string>();
  for (const section of report.result.sections) for (const dimension of Object.values(section.dimensions)) {
    if (dimension?.status !== "ok") continue;
    for (const ref of dimension.records) if (ref.kind === "work") scoped.set(ref.id, section.accountId);
  }
  return [...scoped].map(([workId, accountId]) => ({ workId, accountId }));
}

function reportItems(report: OpDiagReportVersion, dimensionKey: "consistency" | "coverage", field: "items" | "pillars") {
  const found: string[] = [];
  for (const section of report.result.sections) {
    const dimension = section.dimensions[dimensionKey];
    if (dimension?.status === "ok") {
      const facts = dimension.facts;
      const values = facts as { items?: Array<{ item: string }>; pillars?: Array<{ pillar: string }> };
      if (dimensionKey === "consistency" && values.items) found.push(...values.items.map((entry) => entry.item));
      if (dimensionKey === "coverage" && values.pillars) found.push(...values.pillars.map((entry) => entry.pillar));
    }
  }
  if (found.length) return [...new Set(found)];
  const params = report.params;
  if (!Array.isArray(params.dimensions)) return [];
  for (const entry of params.dimensions) {
    if (!entry || typeof entry !== "object" || !("key" in entry) || entry.key !== dimensionKey || !(field in entry)) continue;
    const values = entry[field];
    if (Array.isArray(values)) return values.filter((value): value is string => typeof value === "string");
  }
  return [];
}
