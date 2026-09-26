"use client";

import type { ReactNode } from "react";
import {
  opdiagGapKey,
  opdiagNumberDisplay,
  opdiagReasonKey,
  opdiagRuleKey,
  type OpDiagDimension,
  type OpDiagDimensionResult,
  type OpDiagFactsByDimension,
  type OpDiagReportVersion,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { AppLink } from "@multica/views/navigation";
import { SettingsCard, SettingsSection } from "@multica/views/settings/layout";
import { useT } from "@multica/views/i18n";
import type { OpDiagTodo } from "@multica/core/content/feedback-learning";

type DiagnosisT = ReturnType<typeof useT<"common">>["t"];

export function DiagnosisReport({ report, result, title, gapLinks, todos, addTodo, onNewVersion, busy, canAddTodo }: {
  report: OpDiagReportVersion | null;
  result: OpDiagReportVersion["result"];
  title: string;
  gapLinks: Record<string, string>;
  todos: OpDiagTodo[];
  addTodo: (gap: OpDiagReportVersion["result"]["gaps"][number]) => void;
  onNewVersion: (report: OpDiagReportVersion) => void;
  busy: boolean;
  canAddTodo: boolean;
}) {
  const { t } = useT("common");
  const translateRule = (rule: string) => ruleTranslation(t, rule);
  const latestVersion = result.scope;
  return <SettingsSection title={title}>
    <SettingsCard><div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.manualOnly)} · {result.calcVersion} · {result.scope.window.start} – {result.scope.window.end} ({result.scope.window.timezone})</p>
        {report && <Button size="sm" variant="outline" disabled={busy} onClick={() => onNewVersion(report)}>{t(($) => $.contentOperatingDiagnosisDetails.regenerate)}</Button>}
      </div>
      <p className="text-sm">{t(($) => $.contentOperatingDiagnosis.scope)}: {result.scope.kind === "brand" ? t(($) => $.contentOperatingDiagnosis.brand) : result.scope.kind === "account" ? t(($) => $.contentOperatingDiagnosis.account) : t(($) => $.contentOperatingDiagnosis.unknown)} · {latestVersion.accounts.map((account) => `${account.displayName || account.accountId} (${account.platform}, ${account.profileRevisionId || t(($) => $.contentOperatingDiagnosis.unknown)})`).join(" · ") || t(($) => $.contentOperatingDiagnosis.empty)}</p>
      {result.scope.comparisonWindow && <p className="text-sm">{t(($) => $.contentOperatingDiagnosis.comparisonWindow)}: {result.scope.comparisonWindow.start} – {result.scope.comparisonWindow.end}</p>}
      <dl className="grid grid-cols-2 gap-2 text-sm md:grid-cols-4">{Object.entries(result.scope.inputCounts).map(([key, value]) => <div key={key}><dt className="text-muted-foreground">{key}</dt><dd>{value}</dd></div>)}</dl>
      <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.pendingData)}</p>
      {report?.inputsChanged.changed && <div className="rounded-md border p-3 text-sm">{t(($) => $.contentOperatingDiagnosis.inputsChanged)}: {report.inputsChanged.added.length + report.inputsChanged.modified.length + report.inputsChanged.removed.length}<ul className="mt-2 list-disc pl-5 text-muted-foreground">{[...report.inputsChanged.added.map((ref) => `+ ${ref.kind}: ${ref.id}`), ...report.inputsChanged.modified.map((ref) => `~ ${ref.kind}: ${ref.id}`), ...report.inputsChanged.removed.map((ref) => `− ${ref.kind}: ${ref.id}`)].map((entry) => <li key={entry}>{entry}</li>)}</ul></div>}
      {result.sections.map((section, index) => <div className="space-y-3 border-t pt-3" key={`${section.section}-${section.accountId}-${index}`}>
        <h3 className="text-sm font-medium">{sectionLabel(t, section.section)} {section.accountId && `· ${result.scope.accounts.find((account) => account.accountId === section.accountId)?.displayName || section.accountId}`}</h3>
        {Object.entries(section.dimensions).map(([key, dimension]) => <DimensionBlock key={key} t={t} name={t(($) => $.contentOperatingDiagnosis.dimension[key as OpDiagDimension])} dimension={dimension} />)}
      </div>)}
      <div className="space-y-2 border-t pt-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.gaps)}</h3>{result.gaps.length === 0 && <p className="text-sm text-muted-foreground">—</p>}{result.gaps.map((gap) => {
        const alreadyAdded = todos.some((todo) => todo.originGapKey === gap.gapKey && !todo.voided);
        return <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-2 text-sm" key={gap.gapKey}><span>{gapLabel(t, gap.kind)} · {gap.ref.kind}: {gap.ref.id}</span><div className="flex gap-2"><AppLink href={gapLinks[gap.fixRoute] ?? gapLinks.unknown ?? "/"} className="text-sm underline">{t(($) => $.contentOperatingDiagnosisDetails.openFix)}</AppLink><Button size="sm" variant="outline" disabled={alreadyAdded || !report || busy || !canAddTodo} onClick={() => addTodo(gap)}>{alreadyAdded ? t(($) => $.contentOperatingDiagnosisDetails.todoAdded) : t(($) => $.contentOperatingDiagnosis.createTodo)}</Button></div></div>;
      })}</div>
      <div className="space-y-2 border-t pt-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.rules)}</h3>{result.rules.map((rule) => <p className="text-sm text-muted-foreground" key={rule}>{translateRule(rule)}</p>)}</div>
      {result.roiReference && <div className="space-y-1 border-t pt-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.roiReference)}</h3><p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.roiAsIs)}</p><FactsValue value={result.roiReference} /></div>}
    </div></SettingsCard>
  </SettingsSection>;
}

function DimensionBlock({ t, name, dimension }: { t: DiagnosisT; name: string; dimension: OpDiagDimensionResult | undefined }) {
  if (!dimension || dimension.status === "unknown") return <div className="rounded-md border p-3 text-sm text-muted-foreground">{name}: {t(($) => $.contentOperatingDiagnosis.unknown)}</div>;
  if (dimension.status === "not_computable") return <div className="rounded-md border p-3 text-sm text-muted-foreground">{name}: {t(($) => $.contentOperatingDiagnosis.notComputable)} · {reasonLabel(t, dimension.reason)}</div>;
  return <div className="space-y-2 rounded-md border p-3 text-sm">
    <h4 className="font-medium">{name}</h4>
    <p className="text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.completeness)}: {dimension.completeness.present} / {dimension.completeness.expected}</p>
    {dimension.completeness.gapKeys.length > 0 && <p className="text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.missingItems)}: {dimension.completeness.gapKeys.join(", ")}</p>}
    <FactsByDimension t={t} dimension={dimension} />
    {dimension.limits.length > 0 && <div><p className="font-medium">{t(($) => $.contentOperatingDiagnosisDetails.limits)}</p>{dimension.limits.map((rule) => <p className="text-muted-foreground" key={rule}>{ruleTranslation(t, rule)}</p>)}</div>}
    {dimension.records.length > 0 && <p className="text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.records)}: {dimension.records.map((ref) => `${ref.kind}:${ref.id}`).join(", ")}</p>}
  </div>;
}

function FactsByDimension({ t, dimension }: { t: DiagnosisT; dimension: Extract<OpDiagDimensionResult, { status: "ok" }> }) {
  const facts = dimension.facts as Record<string, unknown>;
  if (Array.isArray(facts.groups)) return <div className="space-y-3">{(facts.groups as OpDiagFactsByDimension["performance"]["groups"]).map((group) => <div className="rounded-md border p-2" key={`${group.platform}/${group.metric}`}><p className="font-medium">{group.platform} · {group.metric}</p><WindowFacts t={t} label={t(($) => $.contentOperatingDiagnosisDetails.current)} window={group.current} /><WindowFacts t={t} label={t(($) => $.contentOperatingDiagnosisDetails.baseline)} window={group.baseline} /><p className="text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.change)}: {opdiagNumberDisplay(group.change) ?? t(($) => $.contentOperatingDiagnosis.notComputable)} · {t(($) => $.contentOperatingDiagnosisDetails.statWindows)}: {group.statWindows.join(", ") || "—"}{group.statWindowMixed ? ` · ${t(($) => $.contentOperatingDiagnosisDetails.mixedStatWindow)}` : ""}</p></div>)}</div>;
  return <FactsValue value={facts} />;
}

function WindowFacts({ t, label, window }: { t: DiagnosisT; label: string; window: OpDiagFactsByDimension["performance"]["groups"][number]["current"] }) {
  return <p className="text-muted-foreground">{label}: {t(($) => $.contentOperatingDiagnosisDetails.publications)} {window.publications}, {t(($) => $.contentOperatingDiagnosisDetails.withValue)} {window.withValue}, {t(($) => $.contentOperatingDiagnosis.unknown)} {window.unknown}, {t(($) => $.contentOperatingDiagnosisDetails.sum)} {window.sum ?? t(($) => $.contentOperatingDiagnosis.unknown)}, {t(($) => $.contentOperatingDiagnosisDetails.mean)} {opdiagNumberDisplay(window.mean) ?? t(($) => $.contentOperatingDiagnosis.notComputable)}</p>;
}

function FactsValue({ value }: { value: unknown }): ReactNode {
  if (value === null || value === undefined) return <span className="text-muted-foreground">—</span>;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return <span>{String(value)}</span>;
  if (Array.isArray(value)) return <ul className="list-disc pl-5">{value.map((item, index) => <li key={index}><FactsValue value={item} /></li>)}</ul>;
  if (typeof value === "object") return <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-2">{Object.entries(value).map(([key, item]) => <div className="contents" key={key}><dt className="text-muted-foreground">{key}</dt><dd><FactsValue value={item} /></dd></div>)}</dl>;
  return null;
}

function gapLabel(t: DiagnosisT, kind: string) {
  const key = opdiagGapKey(kind);
  return key.startsWith("gap.") ? t(`contentOperatingDiagnosisDetails.gap.${key.replace("gap.", "")}` as never) : t(($) => $.contentOperatingDiagnosis.unknown);
}

function reasonLabel(t: DiagnosisT, reason: string) {
  const key = opdiagReasonKey(reason);
  if (key === "reason.unknown" || key === "reason.default") return t(($) => $.contentOperatingDiagnosis.unknown);
  return t(`contentOperatingDiagnosis.reasons.${key.replace("reason.", "")}` as never);
}

function sectionLabel(t: DiagnosisT, section: string) {
  if (section === "account") return t(($) => $.contentOperatingDiagnosis.sections.account);
  if (section === "unknown_account") return t(($) => $.contentOperatingDiagnosis.sections.unknown_account);
  if (section === "brand") return t(($) => $.contentOperatingDiagnosis.sections.brand);
  return t(($) => $.contentOperatingDiagnosis.unknown);
}

function ruleTranslation(t: DiagnosisT, rule: string) {
  const key = opdiagRuleKey(rule);
  return key === "rule.default" ? t(($) => $.contentOperatingDiagnosis.unknown) : t(`contentOperatingDiagnosisDetails.ruleMap.${key.replace("rule.", "")}` as never);
}
