"use client";

import { useEffect, useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  SUGGESTION_ASPECTS, useAbandonSearchSuggestion, useAdoptSearchSuggestion,
  useCompareSearchSuggestions, useCreateSearchSuggestion, useReviseSearchSuggestion,
  useRetrySearchSuggestionDecision, useSearchSuggestion, useSearchSuggestions,
  authorKindDisplay, suggestionAspectDisplay, suggestionFailureDisplay, suggestionStateDisplay,
  type SearchSuggestion, type SearchSuggestionContentInput,
} from "@multica/core/content/topic-planning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";

type Translate = ReturnType<typeof useT<"common">>["t"];
type Draft = SearchSuggestionContentInput;
type WorkOption = { workId: string; title: string };
type ArtifactOption = { workId: string; artifactId: string; title: string; draftBody: string; draftStatus: string };
type VersionOption = { workId: string; artifactId: string; versionId: string; revision: number; body: string; createdAt: string };
type ThemeOption = { themeId: string; name: string; voided: boolean };
type SourceOption = { id: string; label: string };
type SuggestionStateFilter = Exclude<SearchSuggestion["state"], "unknown" | undefined>;
const EMPTY_DRAFT: Draft = { target_question: "", aspects: [], rationale: "", evidence_source_ids: [], proposed_body: "" };

function SelectionList({
  items, selected, onChange,
}: { items: { id: string; label: string }[]; selected: string[]; onChange: (next: string[]) => void }) {
  return <div className="flex flex-wrap gap-x-4 gap-y-2">{items.map((item) => <label key={item.id} className="flex items-center gap-2 text-body"><Checkbox checked={selected.includes(item.id)} onCheckedChange={(checked) => onChange(checked === true ? [...selected, item.id] : selected.filter((id) => id !== item.id))} /><span>{item.label}</span></label>)}</div>;
}

function stateText(t: Translate, state: string) {
  const key = suggestionStateDisplay(state);
  return t(($) => $.search_optimization.values.state[key]);
}

function aspectText(t: Translate, aspect: string) {
  const key = suggestionAspectDisplay(aspect);
  return t(($) => $.search_optimization.values.aspect[key]);
}

function versionLabel(version: VersionOption) {
  return `#${version.revision} · ${version.createdAt || version.versionId}`;
}

export function SearchSuggestionsSection({
  wsId, works, artifacts, versions, themes, sources, initialWorkId = "", initialArtifactId = "", onWorkIdChange, onArtifactIdChange, onConflict,
}: { wsId: string; works: WorkOption[]; artifacts: ArtifactOption[]; versions: VersionOption[]; themes: ThemeOption[]; sources: SourceOption[]; initialWorkId?: string; initialArtifactId?: string; onWorkIdChange?: (workId: string) => void; onArtifactIdChange?: (artifactId: string) => void; onConflict?: () => void }) {
  const { t } = useT("common");
  const [workId, setWorkId] = useState(initialWorkId);
  const [artifactId, setArtifactId] = useState(initialArtifactId);
  const [themeId, setThemeId] = useState("");
  const [state, setState] = useState<SuggestionStateFilter | "">("");
  const [searchText, setSearchText] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [compareIds, setCompareIds] = useState<string[]>([]);
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [decisionNote, setDecisionNote] = useState("");
  const [editing, setEditing] = useState(false);
  const work = works.find((item) => item.workId === workId) ?? null;
  const artifact = artifacts.find((item) => item.workId === workId && item.artifactId === artifactId) ?? null;
  const versionList = versions.filter((item) => item.workId === workId && item.artifactId === artifactId);
  const latest = versionList[0] ?? null;
  const suggestions = useSearchSuggestions(wsId, { workId, artifactId, themeId, state: state || undefined });
  const selectedList = (suggestions.data ?? []).find((item) => item.suggestionId === selectedId) ?? null;
  const detail = useSearchSuggestion(wsId, selectedId);
  const selected = detail.data ?? selectedList;
  const baseVersion = versionList.find((item) => item.versionId === selected?.baseVersionId) ?? null;
  const comparison = useCompareSearchSuggestions(wsId, compareIds);
  const create = useCreateSearchSuggestion(wsId);
  const revise = useReviseSearchSuggestion(wsId);
  const abandon = useAbandonSearchSuggestion(wsId);
  const adopt = useAdoptSearchSuggestion(wsId);
  const retry = useRetrySearchSuggestionDecision(wsId);
  const list = (suggestions.data ?? []).filter((item) => {
    const query = searchText.trim().toLocaleLowerCase();
    return !query || [item.targetQuestion, item.rationale, item.proposedBody, ...item.aspects, ...item.evidenceSourceIds]
      .some((value) => value.toLocaleLowerCase().includes(query));
  });

  useEffect(() => {
    if (initialWorkId) setWorkId(initialWorkId);
  }, [initialWorkId]);
  useEffect(() => {
    if (initialArtifactId) setArtifactId(initialArtifactId);
  }, [initialArtifactId]);
  useEffect(() => {
    if (!selected) return;
    setDraft({
      target_question: selected.targetQuestion, aspects: selected.aspects.filter((item): item is Draft["aspects"][number] => item !== "unknown"),
      rationale: selected.rationale, evidence_source_ids: selected.evidenceSourceIds, proposed_body: selected.proposedBody,
    });
  }, [selected]);

  function refreshAfterConflict(error: Error) {
    if (!error.message.includes("409")) return;
    onConflict?.();
  }
  function setWork(value: string) {
    setWorkId(value); onWorkIdChange?.(value); setArtifactId(""); onArtifactIdChange?.(""); setSelectedId(""); setCompareIds([]); setDraft(EMPTY_DRAFT);
  }
  function setArtifact(value: string) {
    setArtifactId(value); onArtifactIdChange?.(value); setSelectedId(""); setCompareIds([]); setDraft(EMPTY_DRAFT);
  }
  function loadSuggestion(item: SearchSuggestion) {
    setSelectedId(item.suggestionId); setDraft({
      target_question: item.targetQuestion, aspects: item.aspects.filter((aspect): aspect is Draft["aspects"][number] => aspect !== "unknown"),
      rationale: item.rationale, evidence_source_ids: item.evidenceSourceIds, proposed_body: item.proposedBody,
    }); setDecisionNote(""); setEditing(false);
  }
  function onSave() {
    if (selected && editing) {
      revise.mutate({ suggestionId: selected.suggestionId, input: { ...draft, base_revision: selected.revision } }, { onError: refreshAfterConflict, onSuccess: () => setEditing(false) });
      return;
    }
    if (!workId || !artifactId || !latest || !themeId) return;
    create.mutate({ ...draft, work_id: workId, artifact_id: artifactId, base_version_id: latest.versionId, theme_id: themeId }, { onError: refreshAfterConflict, onSuccess: (result) => { if (result) loadSuggestion(result); } });
  }
  function onAdopt() {
    if (!selected) return;
    adopt.mutate({ suggestionId: selected.suggestionId, workId: selected.workId, artifactId: selected.artifactId, input: { decision: "adopt", revision: selected.revision, note: decisionNote } }, { onError: refreshAfterConflict });
  }
  function onAbandon() {
    if (!selected) return;
    abandon.mutate({ suggestionId: selected.suggestionId, input: { decision: "abandon", revision: selected.revision, note: decisionNote } }, { onError: refreshAfterConflict });
  }
  function onRetry() {
    if (!selected?.decision?.decisionId) return;
    retry.mutate({ decisionId: selected.decision.decisionId, workId: selected.workId, artifactId: selected.artifactId }, { onError: refreshAfterConflict });
  }

  const worksItems = works.map((item) => ({ value: item.workId, label: item.title || item.workId }));
  const artifactItems = artifacts.filter((item) => item.workId === workId).map((item) => ({ value: item.artifactId, label: item.title || item.artifactId }));
  const themeItems = themes.filter((item) => !item.voided).map((item) => ({ value: item.themeId, label: item.name }));
  const stateItems = ["", "open", "adopted", "adopt_failed", "adopt_unrecorded", "abandoned"].map((value) => ({ value, label: value ? stateText(t, value) : t(($) => $.search_optimization.all) }));
  const sourceItems = sources;
  const selectedForCompare = compareIds.length >= 2 && compareIds.length <= 4;
  const pending = create.isPending || revise.isPending || abandon.isPending || adopt.isPending || retry.isPending;
  const mutationError = create.error ?? revise.error ?? abandon.error ?? adopt.error ?? retry.error;
  const showConflict = [create.error, revise.error, abandon.error, adopt.error, retry.error].some((error) => error?.message.includes("409"));

  return (
    <SettingsSection title={t(($) => $.search_optimization.suggestions.title)} description={t(($) => $.search_optimization.suggestions.description)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.work)}>
          <Select items={worksItems} value={workId} onValueChange={(value) => setWork(value ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.work)}><SelectValue placeholder={t(($) => $.search_optimization.select)} /></SelectTrigger><SelectContent>{worksItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.artifact)}>
          <Select items={artifactItems} value={artifactId} onValueChange={(value) => setArtifact(value ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.artifact)}><SelectValue placeholder={t(($) => $.search_optimization.select)} /></SelectTrigger><SelectContent>{artifactItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.theme)}>
          <Select items={themeItems} value={themeId} onValueChange={(value) => setThemeId(value ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.theme)}><SelectValue placeholder={t(($) => $.search_optimization.select)} /></SelectTrigger><SelectContent>{themeItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.base)} description={latest ? versionLabel(latest) : t(($) => $.search_optimization.unknown)}><span className="text-caption text-muted-foreground">{latest?.versionId ?? t(($) => $.search_optimization.unknown)}</span></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.filterState)}>
          <Select items={stateItems} value={state} onValueChange={(value) => setState((value ?? "") as SuggestionStateFilter | "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.filterState)}><SelectValue /></SelectTrigger><SelectContent>{stateItems.map((item) => <SelectItem key={item.value || "all"} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.search)} size="text"><Input value={searchText} onChange={(event) => setSearchText(event.target.value)} placeholder={t(($) => $.search_optimization.suggestions.searchHint)} /></SettingsRow>
        {list.length === 0 ? <SettingsRow label={t(($) => $.search_optimization.suggestions.empty)}><span className="text-caption text-muted-foreground" /></SettingsRow> : list.map((item) => (
          <SettingsRow key={item.suggestionId} label={`${item.targetQuestion} · #${item.revision}`} description={`${stateText(t, item.state)} · ${item.baseVersionId}`}>
            <div className="flex items-center gap-3"><Checkbox checked={compareIds.includes(item.suggestionId)} onCheckedChange={(checked) => setCompareIds((current) => checked === true ? [...current, item.suggestionId].slice(-4) : current.filter((id) => id !== item.suggestionId))} /><Button variant="outline" onClick={() => loadSuggestion(item)}>{t(($) => $.search_optimization.revise)}</Button></div>
          </SettingsRow>
        ))}
        <SettingsRow label={t(($) => $.search_optimization.suggestions.question)} size="text"><Input value={draft.target_question} onChange={(event) => setDraft((item) => ({ ...item, target_question: event.target.value }))} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.aspects)} size="text"><SelectionList items={SUGGESTION_ASPECTS.map((id) => ({ id, label: aspectText(t, id) }))} selected={draft.aspects} onChange={(value) => setDraft((item) => ({ ...item, aspects: value as Draft["aspects"] }))} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.evidence)} size="text"><SelectionList items={sourceItems} selected={draft.evidence_source_ids} onChange={(value) => setDraft((item) => ({ ...item, evidence_source_ids: value }))} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.rationale)} size="text"><Textarea rows={3} value={draft.rationale} onChange={(event) => setDraft((item) => ({ ...item, rationale: event.target.value }))} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.body)} size="text"><Textarea rows={14} value={draft.proposed_body} onChange={(event) => setDraft((item) => ({ ...item, proposed_body: event.target.value }))} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.history)} description={selected ? `#${selected.revision} · ${selected.createdAt} · ${t(($) => $.search_optimization.suggestions.historyCurrentOnly)}` : t(($) => $.search_optimization.empty)}>
          <div className="flex flex-wrap items-center gap-3">
            {selected ? <><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.values.author[authorKindDisplay(selected.authorKind)])}</span><Button variant="outline" disabled={!selected.state || selected.state === "unknown" || selected.state !== "open"} onClick={() => setEditing((value) => !value)}>{editing ? t(($) => $.search_optimization.save) : t(($) => $.search_optimization.suggestions.revise)}</Button></> : null}
            <Button disabled={pending || !work || !artifact || !latest || !themeId || !draft.target_question.trim() || draft.aspects.length === 0 || !draft.proposed_body.trim()} onClick={onSave}>{selected && editing ? t(($) => $.search_optimization.suggestions.revise) : t(($) => $.search_optimization.suggestions.create)}</Button>
          </div>
        </SettingsRow>
        {selected ? <>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.state)} description={stateText(t, selected.state)}><span className="text-caption text-muted-foreground">{selected.failureCode ? t(($) => $.search_optimization.values.failure[suggestionFailureDisplay(selected.failureCode)]) : ""}</span></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.baseVersion)} description={selected.baseVersionId}><div className="max-h-56 overflow-auto whitespace-pre-wrap text-caption text-muted-foreground">{baseVersion?.body ?? t(($) => $.search_optimization.unknown)}</div></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.diff)}><div className="space-y-2">{selected.diff?.ops.map((op, index) => <div key={`${op.op}-${index}`} className="whitespace-pre-wrap text-caption text-muted-foreground">{op.op === "equal" ? "  " : op.op === "insert" ? "+ " : "− "}{op.text}</div>) ?? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.unknown)}</span>}</div></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.diff)} description={`${t(($) => $.search_optimization.suggestions.inserted, { count: selected.diff?.insertedLines ?? 0 })} · ${t(($) => $.search_optimization.suggestions.deleted, { count: selected.diff?.deletedLines ?? 0 })}`}><span className="text-caption text-muted-foreground">{selected.themeChanged ? t(($) => $.search_optimization.suggestions.themeChanged) : ""}</span></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.decisionNote)} size="text"><Textarea rows={2} value={decisionNote} onChange={(event) => setDecisionNote(event.target.value)} /></SettingsRow>
          {selected.state === "adopt_unrecorded" ? <SettingsRow label={t(($) => $.search_optimization.suggestions.adoptUnrecorded)}><Button disabled={pending || !selected.decision?.decisionId} onClick={onRetry}>{t(($) => $.search_optimization.suggestions.retry)}</Button></SettingsRow> : null}
          {selected.state === "adopt_failed" ? <SettingsRow label={t(($) => $.search_optimization.suggestions.adoptFailed)} description={t(($) => $.search_optimization.values.failure[suggestionFailureDisplay(selected.failureCode)])}><Button disabled={pending || !selected.decision?.decisionId} onClick={onRetry}>{t(($) => $.search_optimization.suggestions.retry)}</Button></SettingsRow> : null}
          <SettingsRow label={t(($) => $.search_optimization.suggestions.adopt)} description={!selected.baseIsCurrent ? t(($) => $.search_optimization.suggestions.baseMoved) : undefined}>
            <div className="flex flex-wrap gap-3"><Button disabled={pending || !selected.baseIsCurrent || selected.state !== "open"} onClick={onAdopt}>{t(($) => $.search_optimization.suggestions.adopt)}</Button><Button variant="outline" disabled={pending || selected.state !== "open"} onClick={onAbandon}>{t(($) => $.search_optimization.suggestions.abandon)}</Button></div>
          </SettingsRow>
        </> : null}
        <SettingsRow label={t(($) => $.search_optimization.suggestions.compare)}>
          <Button variant="outline" disabled={!selectedForCompare} onClick={() => void comparison.refetch()}>{t(($) => $.search_optimization.suggestions.compare)}</Button>
        </SettingsRow>
        {comparison.data ? <SettingsRow label={comparison.data.sameBase ? t(($) => $.search_optimization.suggestions.sameBase) : t(($) => $.search_optimization.suggestions.differentBase)}>
          <div className="space-y-4">{comparison.data.suggestions.map((item) => <div key={item.suggestionId} className="space-y-2"><div className="text-body">{item.targetQuestion} · {item.baseVersionId} · {stateText(t, item.state)}</div><div className="max-h-48 overflow-auto whitespace-pre-wrap text-caption text-muted-foreground">{item.proposedBody}</div></div>)}</div>
        </SettingsRow> : null}
        {mutationError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{showConflict ? t(($) => $.search_optimization.suggestions.errorConflict) : t(($) => $.search_optimization.saveFailed)}</span></SettingsRow> : null}
        <SettingsRow label={t(($) => $.search_optimization.noAi)}><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.fixedGuidance)} {t(($) => $.search_optimization["optimization.no_ranking_promise"])}</span></SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
