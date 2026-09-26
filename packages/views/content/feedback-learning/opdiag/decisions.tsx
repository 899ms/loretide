"use client";

import { useState } from "react";
import {
  OPDIAG_JUDGEMENT_BASES, OPDIAG_JUDGEMENT_KINDS, OPDIAG_SUGGESTION_TARGETS,
  useDecideOpDiagSuggestion, useOpDiagAnnotations, useWriteOpDiagJudgement, useWriteOpDiagSuggestion,
  type OpDiagReportVersion,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsSection } from "@multica/views/settings/layout";
import { useT } from "@multica/views/i18n";
import { suggestionRequest, topicCardsForTargetAccount, type OperatingDiagnosisOption, type OperatingDiagnosisTopicCard } from "./shared";

const PROFILE_FIELDS = ["audience", "common_questions", "experience", "positioning", "content_pillars", "expression_style", "forbidden_expressions", "content_goals"] as const;

export function DiagnosisDecisions({ wsId, report, accounts, annotations, topicCards, topicsLoading, topicsFailed }: { wsId: string; report: OpDiagReportVersion; accounts: OperatingDiagnosisOption[]; annotations: ReturnType<typeof useOpDiagAnnotations>["data"]; topicCards: OperatingDiagnosisTopicCard[]; topicsLoading: boolean; topicsFailed: boolean }) {
  const { t } = useT("common");
  const [kind, setKind] = useState<(typeof OPDIAG_JUDGEMENT_KINDS)[number]>("judgement");
  const [basis, setBasis] = useState<(typeof OPDIAG_JUDGEMENT_BASES)[number]>("qualitative");
  const [about, setAbout] = useState(""); const [body, setBody] = useState(""); const [refs, setRefs] = useState<string[]>([]);
  const [targetKind, setTargetKind] = useState<(typeof OPDIAG_SUGGESTION_TARGETS)[number]>("todo");
  const [suggestionJudgementIds, setSuggestionJudgementIds] = useState<string[]>([]);
  const [accountId, setAccountId] = useState(""); const [todoTitle, setTodoTitle] = useState(""); const [patchField, setPatchField] = useState<(typeof PROFILE_FIELDS)[number]>("audience"); const [patchValue, setPatchValue] = useState(""); const [patches, setPatches] = useState<Array<{ field: string; value: string }>>([]);
  const [decisionChoices, setDecisionChoices] = useState<Record<string, { mode: "create" | "link"; linkTargetId: string }>>({});
  const judgement = useWriteOpDiagJudgement(wsId); const suggestion = useWriteOpDiagSuggestion(wsId); const decide = useDecideOpDiagSuggestion(wsId);
  const path = `reports/${encodeURIComponent(report.reportId)}/versions/${report.versionNo}`;
  const refsInReport = report.result.refs;
  const messages = [judgement.error, suggestion.error, decide.error].filter(Boolean).map((error) => error instanceof Error ? error.message : t(($) => $.contentOperatingDiagnosis.failed));
  const selectedRefs = (ref: string) => setRefs((current) => current.includes(ref) ? current.filter((item) => item !== ref) : [...current, ref]);
  const selectedSuggestionJudgement = (id: string) => setSuggestionJudgementIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  const judgementReady = Boolean(body.trim()) && (basis === "qualitative" || refs.length > 0) && (kind !== "alternative_explanation" || Boolean(about));
  const suggestionReady = Boolean(body.trim()) && Boolean(accountId) && (targetKind !== "todo" || Boolean(todoTitle.trim())) && (targetKind !== "profile_proposal" || patches.length > 0);
  const titleForKind = (value: string) => value === "alternative_explanation" ? t(($) => $.contentOperatingDiagnosisDetails.alternativeExplanation) : value === "limitation" ? t(($) => $.contentOperatingDiagnosisDetails.limitation) : value === "judgement" ? t(($) => $.contentOperatingDiagnosis.judgement) : t(($) => $.contentOperatingDiagnosis.unknown);
  const decisionFor = (suggestionId: string, revision: number) => annotations?.decisions.find((entry) => entry.suggestionId === suggestionId && entry.suggestionRevision === revision);
  const saveJudgement = () => judgement.mutate({ path: `${path}/judgements`, body: { kind, basis, evidence_refs: basis === "evidence" ? refs : [], about_judgement_id: kind === "alternative_explanation" ? about : "", body } });
  const target = targetKind === "topic_card" ? { account_id: accountId } : targetKind === "todo" ? { account_id: accountId, title: todoTitle } : { account_id: accountId, patches };
  const saveSuggestion = () => suggestion.mutate({ path: `${path}/suggestions`, body: suggestionRequest(body, targetKind, target, suggestionJudgementIds, refs) });
  const decideEntry = (entry: NonNullable<typeof annotations>["suggestions"][number], decision: "adopt" | "reject", choice: { mode: "create" | "link"; linkTargetId: string }) => decide.mutate({ path: `suggestions/${encodeURIComponent(entry.suggestionId)}/decisions`, body: { suggestion_revision: entry.revision, decision, ...(decision === "adopt" && entry.targetKind === "topic_card" ? { mode: choice.mode, ...(choice.mode === "link" ? { link_target_id: choice.linkTargetId } : {}) } : {}), note: "" } });
  return <SettingsSection title={t(($) => $.contentOperatingDiagnosis.blocks.decisions)}><div className="space-y-4">
    <SettingsCard><div className="space-y-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.judgement)}</h3>
      <Select items={OPDIAG_JUDGEMENT_KINDS.map((value) => ({ value, label: titleForKind(value) }))} value={kind} onValueChange={(value) => setKind((value as typeof kind) ?? "judgement")}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{OPDIAG_JUDGEMENT_KINDS.map((value) => <SelectItem value={value} key={value}>{titleForKind(value)}</SelectItem>)}</SelectContent></Select>
      <Select items={OPDIAG_JUDGEMENT_BASES.map((value) => ({ value, label: value === "evidence" ? t(($) => $.contentOperatingDiagnosisDetails.evidenceBased) : t(($) => $.contentOperatingDiagnosisDetails.qualitative) }))} value={basis} onValueChange={(value) => setBasis((value as typeof basis) ?? "qualitative")}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="evidence">{t(($) => $.contentOperatingDiagnosisDetails.evidenceBased)}</SelectItem><SelectItem value="qualitative">{t(($) => $.contentOperatingDiagnosisDetails.qualitative)}</SelectItem></SelectContent></Select>
      {basis === "evidence" && <div className="max-h-48 space-y-2 overflow-auto rounded-md border p-2">{refsInReport.map((ref) => <label className="flex gap-2 text-sm" key={ref}><Checkbox checked={refs.includes(ref)} onCheckedChange={() => selectedRefs(ref)} />{ref}</label>)}</div>}
      {kind === "alternative_explanation" && <Select items={(annotations?.judgements ?? []).filter((entry) => !entry.voided).map((entry) => ({ value: entry.judgementId, label: entry.body }))} value={about} onValueChange={(value) => setAbout(value ?? "")}><SelectTrigger><SelectValue placeholder={t(($) => $.contentOperatingDiagnosisDetails.aboutJudgement)} /></SelectTrigger><SelectContent>{annotations?.judgements.filter((entry) => !entry.voided).map((entry) => <SelectItem value={entry.judgementId} key={entry.judgementId}>{entry.body}</SelectItem>)}</SelectContent></Select>}
      <Textarea value={body} onChange={(event) => setBody(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosis.body)} />
      {basis === "qualitative" && <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.qualitativeNoData)}</p>}
      <Button size="sm" variant="outline" disabled={!judgementReady || judgement.isPending} onClick={saveJudgement}>{t(($) => $.contentOperatingDiagnosis.saveJudgement)}</Button>
      {annotations?.judgements.map((entry) => <p className="text-sm" key={entry.judgementId}>{titleForKind(entry.kind)} · {entry.body} · {entry.basis === "qualitative" ? t(($) => $.contentOperatingDiagnosisDetails.qualitativeNoData) : entry.evidenceRefs.join(", ")} · {t(($) => $.contentOperatingDiagnosisDetails.writtenBy)} {entry.recordedBy}</p>)}
    </div></SettingsCard>
    <SettingsCard><div className="space-y-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosis.suggestion)}</h3>
      <Select items={OPDIAG_SUGGESTION_TARGETS.map((value) => ({ value, label: targetLabel(t, value) }))} value={targetKind} onValueChange={(value) => setTargetKind((value as typeof targetKind) ?? "todo")}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{OPDIAG_SUGGESTION_TARGETS.map((value) => <SelectItem key={value} value={value}>{targetLabel(t, value)}</SelectItem>)}</SelectContent></Select>
      <Select items={accounts.map((entry) => ({ value: entry.id, label: entry.label }))} value={accountId} onValueChange={(value) => setAccountId(value ?? "")}><SelectTrigger><SelectValue placeholder={t(($) => $.contentOperatingDiagnosis.account)} /></SelectTrigger><SelectContent>{accounts.map((entry) => <SelectItem value={entry.id} key={entry.id}>{entry.label}</SelectItem>)}</SelectContent></Select>
      {targetKind === "todo" && <Input value={todoTitle} onChange={(event) => setTodoTitle(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosisDetails.todoTitle)} />}
      {targetKind === "profile_proposal" && <div className="space-y-2"><div className="grid gap-2 sm:grid-cols-[12rem_1fr_auto]"><Select items={PROFILE_FIELDS.map((field) => ({ value: field, label: profileFieldLabel(t, field) }))} value={patchField} onValueChange={(value) => setPatchField((value as typeof patchField) ?? "audience")}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{PROFILE_FIELDS.map((field) => <SelectItem value={field} key={field}>{profileFieldLabel(t, field)}</SelectItem>)}</SelectContent></Select><Input value={patchValue} onChange={(event) => setPatchValue(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosisDetails.proposedValue)} /><Button size="sm" variant="outline" disabled={!patchValue.trim() || patches.some((entry) => entry.field === patchField)} onClick={() => { setPatches((current) => [...current, { field: patchField, value: patchValue }]); setPatchValue(""); }}>{t(($) => $.contentOperatingDiagnosisDetails.addPatch)}</Button></div>{patches.map((patch) => <p className="text-sm" key={patch.field}>{profileFieldLabel(t, patch.field)}: {patch.value} <Button size="sm" variant="ghost" onClick={() => setPatches((current) => current.filter((item) => item.field !== patch.field))}>{t(($) => $.contentOperatingDiagnosisDetails.remove)}</Button></p>)}</div>}
      <div className="max-h-40 space-y-2 overflow-auto rounded-md border p-2">{refsInReport.map((ref) => <label className="flex gap-2 text-sm" key={ref}><Checkbox checked={refs.includes(ref)} onCheckedChange={() => selectedRefs(ref)} />{ref}</label>)}</div>
      {(annotations?.judgements ?? []).filter((entry) => !entry.voided).length > 0 && <fieldset className="space-y-2"><legend className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosisDetails.relatedJudgements)}</legend>{annotations?.judgements.filter((entry) => !entry.voided).map((entry) => <label className="flex gap-2 text-sm" key={entry.judgementId}><Checkbox checked={suggestionJudgementIds.includes(entry.judgementId)} onCheckedChange={() => selectedSuggestionJudgement(entry.judgementId)} />{entry.body}</label>)}</fieldset>}
      <Textarea value={body} onChange={(event) => setBody(event.target.value)} placeholder={t(($) => $.contentOperatingDiagnosis.body)} />
      <Button size="sm" variant="outline" disabled={!suggestionReady || suggestion.isPending} onClick={saveSuggestion}>{t(($) => $.contentOperatingDiagnosis.saveSuggestion)}</Button>
    </div></SettingsCard>
    <SettingsCard><div className="space-y-3"><h3 className="text-sm font-medium">{t(($) => $.contentOperatingDiagnosisDetails.decisions)}</h3>
      {annotations?.suggestions.map((entry) => {
        const existing = decisionFor(entry.suggestionId, entry.revision);
        const decision = existing;
        const choiceKey = `${entry.suggestionId}:${entry.revision}`;
        const choice = decisionChoices[choiceKey] ?? { mode: "create" as const, linkTargetId: "" };
        const matchingCards = topicCardsForTargetAccount(topicCards, entry.target.accountId);
        const supportedTarget = OPDIAG_SUGGESTION_TARGETS.some((target) => target === entry.targetKind);
        const isTopicAdoption = entry.targetKind === "topic_card" && !existing;
        return <article className="space-y-2 border-t pt-3" key={entry.suggestionId}>
          <p className="text-sm">{entry.body} · {targetLabel(t, entry.targetKind)}</p>
          {!existing && isTopicAdoption && <>
            <Select items={[{ value: "create", label: t(($) => $.contentOperatingDiagnosisDetails.createNewCard) }, { value: "link", label: t(($) => $.contentOperatingDiagnosisDetails.linkExistingCard) }]} value={choice.mode} onValueChange={(value) => setDecisionChoices((current) => ({ ...current, [choiceKey]: { ...choice, mode: (value as typeof choice.mode) ?? "create", linkTargetId: "" } }))}>
              <SelectTrigger><SelectValue /></SelectTrigger><SelectContent>
                <SelectItem value="create">{t(($) => $.contentOperatingDiagnosisDetails.createNewCard)}</SelectItem>
                <SelectItem value="link">{t(($) => $.contentOperatingDiagnosisDetails.linkExistingCard)}</SelectItem>
              </SelectContent>
            </Select>
            {choice.mode === "link" && <>
              {topicsLoading ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loading)}</p> : topicsFailed ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosis.loadFailed)}</p> : matchingCards.length === 0 ? <p className="text-sm text-muted-foreground">{t(($) => $.contentOperatingDiagnosisDetails.noMatchingTopicCards)}</p> : <Select items={matchingCards.map((card) => ({ value: card.id, label: `${card.id} · ${card.label}` }))} value={choice.linkTargetId} onValueChange={(value) => setDecisionChoices((current) => ({ ...current, [choiceKey]: { ...choice, linkTargetId: value ?? "" } }))}>
                <SelectTrigger><SelectValue placeholder={t(($) => $.contentOperatingDiagnosisDetails.linkTarget)} /></SelectTrigger><SelectContent>{matchingCards.map((card) => <SelectItem value={card.id} key={card.id}>{card.id} · {card.label}</SelectItem>)}</SelectContent>
              </Select>}
            </>}
          </>}
        {decision && <DecisionStatus t={t} wsId={wsId} decision={decision} />}
        {!existing && <div className="flex gap-2"><Button size="sm" variant="outline" disabled={decide.isPending || !supportedTarget || (isTopicAdoption && choice.mode === "link" && (!matchingCards.some((card) => card.id === choice.linkTargetId) || topicsLoading || topicsFailed))} onClick={() => decideEntry(entry, "adopt", choice)}>{t(($) => $.contentOperatingDiagnosis.adopt)}</Button><Button size="sm" variant="outline" disabled={decide.isPending} onClick={() => decideEntry(entry, "reject", choice)}>{t(($) => $.contentOperatingDiagnosis.reject)}</Button></div>}
      </article>; })}
    </div></SettingsCard>
    {messages.map((message, index) => <p className="text-sm text-muted-foreground" key={`${message}-${index}`}>{t(($) => $.contentOperatingDiagnosis.failed)} · {message}</p>)}
  </div></SettingsSection>;
}

function DecisionStatus({ t, wsId, decision }: { t: ReturnType<typeof useT<"common">>["t"]; wsId: string; decision: NonNullable<ReturnType<typeof useOpDiagAnnotations>["data"]>["decisions"][number] }) {
  const retry = useDecideOpDiagSuggestion(wsId);
  const state = decision.effectState;
  const label = decision.decision === "reject" ? t(($) => $.contentOperatingDiagnosisDetails.rejected) : decision.decision !== "adopt" ? t(($) => $.contentOperatingDiagnosis.unknown) : state === "done" ? t(($) => $.contentOperatingDiagnosisDetails.adopted) : state === "failed" ? t(($) => $.contentOperatingDiagnosisDetails.actionFailed) : state === "unrecorded" ? t(($) => $.contentOperatingDiagnosisDetails.effectUnrecorded) : state === "none" ? t(($) => $.contentOperatingDiagnosisDetails.decisionRecorded) : t(($) => $.contentOperatingDiagnosis.unknown);
  return <div className="space-y-1 text-sm text-muted-foreground"><p>{label} · {decision.decidedBy} · {decision.createdAt}</p>{decision.effects.map((effect) => <p key={effect.effectId}>{effect.outcome === "done" ? t(($) => $.contentOperatingDiagnosisDetails.effectCompleted) : effect.outcome === "failed" ? t(($) => $.contentOperatingDiagnosisDetails.effectFailed) : t(($) => $.contentOperatingDiagnosis.unknown)} · {effect.failureCode || effect.targetId || "—"}</p>)}{retry.error instanceof Error && <p role="alert">{t(($) => $.contentOperatingDiagnosis.failed)} · {retry.error.message}</p>}{decision.decision === "adopt" && (state === "failed" || state === "unrecorded") && <Button size="sm" variant="outline" disabled={retry.isPending} onClick={() => retry.mutate({ path: `decisions/${decision.decisionId}/retry`, body: { mode: decision.mode, link_target_id: decision.linkTargetId } })}>{t(($) => $.contentOperatingDiagnosis.retry)}</Button>}</div>;
}

function targetLabel(t: ReturnType<typeof useT<"common">>["t"], target: string) {
  return target === "topic_card" ? t(($) => $.contentOperatingDiagnosisDetails.targetTopicCard) : target === "profile_proposal" ? t(($) => $.contentOperatingDiagnosisDetails.targetProfileProposal) : target === "todo" ? t(($) => $.contentOperatingDiagnosisDetails.targetTodo) : t(($) => $.contentOperatingDiagnosis.unknown);
}

function profileFieldLabel(t: ReturnType<typeof useT<"common">>["t"], field: string) {
  return PROFILE_FIELDS.includes(field as (typeof PROFILE_FIELDS)[number]) ? t(`contentOperatingDiagnosisDetails.profileFields.${field}` as never) : t(($) => $.contentOperatingDiagnosis.unknown);
}
