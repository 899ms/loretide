"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  SUGGESTION_ASPECTS, useAbandonSearchSuggestion, useAdoptSearchSuggestion,
  useCompareSearchSuggestions, useCreateSearchSuggestion, useReviseSearchSuggestion,
  useRetrySearchSuggestionDecision, useSearchSuggestion, useSearchSuggestions,
  authorKindDisplay, suggestionAspectDisplay, suggestionFailureDisplay, suggestionStateDisplay,
  validateSearchSuggestionEvidence,
  type SearchSuggestion, type SearchSuggestionContentInput,
} from "@multica/core/content/topic-planning";
import { IDLE_SUGGESTION_DRAFT, beginSuggestionCreate, beginSuggestionRevision, suggestionCreateRequest, suggestionDraftContextMatches, suggestionRevisionRequest, updateSuggestionDraft, type SuggestionDraftSession } from "@multica/core/content/topic-planning";
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
  items, selected, onChange, disabled = false,
}: { items: { id: string; label: string }[]; selected: string[]; onChange: (next: string[]) => void; disabled?: boolean }) {
  return <div className="flex flex-wrap gap-x-4 gap-y-2">{items.map((item) => <label key={item.id} className="flex items-center gap-2 text-body"><Checkbox disabled={disabled} checked={selected.includes(item.id)} onCheckedChange={(checked) => onChange(checked === true ? [...selected, item.id] : selected.filter((id) => id !== item.id))} /><span>{item.label}</span></label>)}</div>;
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

function SuggestionDiff({ suggestion, t }: { suggestion: SearchSuggestion; t: Translate }) {
  if (!suggestion.diff) return <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.unknown)}</span>;
  return <div className="space-y-1 whitespace-pre-wrap text-caption">{suggestion.diff.ops.map((op, index) => (
    <div key={`${op.op}-${index}`}>
      {op.op === "equal" ? <span>{op.text}</span> : op.op === "insert" ? <ins className="underline decoration-current">+ {op.text}</ins> : <del className="line-through">− {op.text}</del>}
    </div>
  ))}</div>;
}

function draftFromSuggestion(item: SearchSuggestion): Draft {
  return {
    target_question: item.targetQuestion,
    aspects: item.aspects.filter((aspect): aspect is Draft["aspects"][number] => aspect !== "unknown"),
    rationale: item.rationale, evidence_source_ids: item.evidenceSourceIds, proposed_body: item.proposedBody,
  };
}

export function SearchSuggestionsSection({
  wsId, works, artifacts, versions, themes, sources, workId, artifactId, onWorkIdChange, onArtifactIdChange, onConflict,
  worksLoading = false, worksError = false, artifactsLoading = false, artifactsError = false,
  versionsLoading = false, versionsError = false, themesLoading = false, themesError = false,
  sourcesLoading = false, sourcesError = false,
}: { wsId: string; works: WorkOption[]; artifacts: ArtifactOption[]; versions: VersionOption[]; themes: ThemeOption[]; sources: SourceOption[]; workId: string; artifactId: string; onWorkIdChange?: (workId: string) => void; onArtifactIdChange?: (artifactId: string) => void; onConflict?: () => void; worksLoading?: boolean; worksError?: boolean; artifactsLoading?: boolean; artifactsError?: boolean; versionsLoading?: boolean; versionsError?: boolean; themesLoading?: boolean; themesError?: boolean; sourcesLoading?: boolean; sourcesError?: boolean }) {
  const { t } = useT("common");
  const [themeId, setThemeId] = useState("");
  const [state, setState] = useState<SuggestionStateFilter | "">("");
  const [searchText, setSearchText] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [compareIds, setCompareIds] = useState<string[]>([]);
  const [session, setSession] = useState<SuggestionDraftSession>(IDLE_SUGGESTION_DRAFT);
  const [decisionNote, setDecisionNote] = useState("");
  const work = works.find((item) => item.workId === workId) ?? null;
  const artifact = artifacts.find((item) => item.workId === workId && item.artifactId === artifactId) ?? null;
  const versionList = versions.filter((item) => item.workId === workId && item.artifactId === artifactId);
  const latest = versionList[0] ?? null;
  const creating = session.mode === "create";
  const pinnedCreateBase = creating ? versions.find((item) => item.workId === session.workId && item.artifactId === session.artifactId && item.versionId === session.baseVersionId) ?? null : null;
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

  const draft = session.mode === "idle" ? (selected ? draftFromSuggestion(selected) : EMPTY_DRAFT) : session.draft;
  const queryLoading = suggestions.isPending || Boolean(worksLoading || artifactsLoading || versionsLoading || themesLoading || sourcesLoading);

  function refreshAfterConflict(error: Error) {
    if (!error.message.includes("409")) return;
    onConflict?.();
  }
  function setWork(value: string) {
    onWorkIdChange?.(value); onArtifactIdChange?.(""); setSelectedId(""); setCompareIds([]); setSession(IDLE_SUGGESTION_DRAFT);
  }
  function setArtifact(value: string) {
    onArtifactIdChange?.(value); setSelectedId(""); setCompareIds([]); setSession(IDLE_SUGGESTION_DRAFT);
  }
  function loadSuggestion(item: SearchSuggestion) {
    setSelectedId(item.suggestionId); setThemeId(item.themeId); setSession(IDLE_SUGGESTION_DRAFT); setDecisionNote("");
  }
  function beginCreate() {
    const next = beginSuggestionCreate({ workspaceId: wsId, workId, artifactId }, themeId, latest?.versionId ?? "", EMPTY_DRAFT);
    if (next.mode === "create") { setSelectedId(""); setSession(next); }
  }
  function changeTheme(value: string) {
    setThemeId(value);
    if (session.mode === "create") setSession((current) => current.mode === "create" ? { ...current, themeId: value } : current);
  }
  function beginRevision() {
    if (!selected) return;
    setSession(beginSuggestionRevision(wsId, selected, draftFromSuggestion(selected)));
  }
  function updateDraft(next: Draft) { setSession((current) => updateSuggestionDraft(current, next)); }
  function onSave() {
    if (!suggestionDraftContextMatches(session, { workspaceId: wsId, workId, artifactId })) return;
    if (sourcesLoading || sourcesError || !evidenceValidation.valid) return;
    if (session.mode === "revise") {
      const request = suggestionRevisionRequest(session);
      if (!request) return;
      revise.mutate(request, { onError: refreshAfterConflict, onSuccess: (result) => { setSession(IDLE_SUGGESTION_DRAFT); if (result) setSelectedId(result.suggestionId); } });
      return;
    }
    const request = suggestionCreateRequest(session);
    if (!request || !themes.some((item) => item.themeId === request.theme_id && !item.voided)) return;
    create.mutate(request, { onError: refreshAfterConflict, onSuccess: (result) => { setSession(IDLE_SUGGESTION_DRAFT); if (result) setSelectedId(result.suggestionId); } });
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
  const evidenceValidation = validateSearchSuggestionEvidence(draft.evidence_source_ids, sources.map((item) => item.id));
  const staleEvidenceIds = !sourcesLoading && !sourcesError ? evidenceValidation.missingEvidenceSourceIds : [];
  const sourceItems = [
    ...sources,
    ...staleEvidenceIds.map((id) => ({ id, label: `${id} · ${t(($) => $.search_optimization.themes.unavailableReference)}` })),
  ];
  const dependencyError = worksError || artifactsError || versionsError || themesError || sourcesError;
  const selectedForCompare = compareIds.length >= 2 && compareIds.length <= 4;
  const editing = session.mode !== "idle";
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
          <Select disabled={session.mode === "revise"} items={themeItems} value={themeId} onValueChange={(value) => changeTheme(value ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.theme)}><SelectValue placeholder={t(($) => $.search_optimization.select)} /></SelectTrigger><SelectContent>{themeItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.base)} description={session.mode === "create" ? session.baseVersionId : latest ? versionLabel(latest) : t(($) => $.search_optimization.unknown)}><span className="text-caption text-muted-foreground">{session.mode === "create" ? session.baseVersionId : latest?.versionId ?? t(($) => $.search_optimization.unknown)}</span></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.filterState)}>
          <Select items={stateItems} value={state} onValueChange={(value) => setState((value ?? "") as SuggestionStateFilter | "")}><SelectTrigger aria-label={t(($) => $.search_optimization.suggestions.filterState)}><SelectValue /></SelectTrigger><SelectContent>{stateItems.map((item) => <SelectItem key={item.value || "all"} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.search)} size="text"><Input value={searchText} onChange={(event) => setSearchText(event.target.value)} placeholder={t(($) => $.search_optimization.suggestions.searchHint)} /></SettingsRow>
        {suggestions.isPending ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : suggestions.isError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : !dependencyError && list.length === 0 ? <SettingsRow label={t(($) => $.search_optimization.suggestions.empty)}><span className="text-caption text-muted-foreground" /></SettingsRow> : !suggestions.isError && list.length > 0 ? list.map((item) => (
          <SettingsRow key={item.suggestionId} label={`${item.targetQuestion} · #${item.revision}`} description={`${stateText(t, item.state)} · ${item.baseVersionId}`}>
            <div className="flex items-center gap-3"><Checkbox checked={compareIds.includes(item.suggestionId)} onCheckedChange={(checked) => setCompareIds((current) => checked === true ? [...current, item.suggestionId].slice(-4) : current.filter((id) => id !== item.suggestionId))} /><Button variant="outline" onClick={() => loadSuggestion(item)}>{t(($) => $.search_optimization.revise)}</Button></div>
          </SettingsRow>
        )) : null}
        {queryLoading ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
        {dependencyError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
        {selectedId && detail.isPending ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
        {selectedId && detail.isError ? <SettingsRow label={t(($) => $.search_optimization.suggestions.history)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
        <SettingsRow label={t(($) => $.search_optimization.suggestions.question)} size="text"><Input disabled={!editing} value={draft.target_question} onChange={(event) => updateDraft({ ...draft, target_question: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.aspects)} size="text"><SelectionList disabled={!editing} items={SUGGESTION_ASPECTS.map((id) => ({ id, label: aspectText(t, id) }))} selected={draft.aspects} onChange={(value) => updateDraft({ ...draft, aspects: value as Draft["aspects"] })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.evidence)} size="text"><SelectionList disabled={!editing || sourcesLoading || sourcesError} items={sourceItems} selected={draft.evidence_source_ids} onChange={(value) => updateDraft({ ...draft, evidence_source_ids: value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.rationale)} size="text"><Textarea disabled={!editing} rows={3} value={draft.rationale} onChange={(event) => updateDraft({ ...draft, rationale: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.body)} size="text"><Textarea disabled={!editing} rows={14} value={draft.proposed_body} onChange={(event) => updateDraft({ ...draft, proposed_body: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.suggestions.history)} description={selected ? `#${selected.revision} · ${selected.createdAt} · ${t(($) => $.search_optimization.suggestions.historyCurrentOnly)}` : t(($) => $.search_optimization.empty)}>
          <div className="flex flex-wrap items-center gap-3">
            {selected ? <><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.values.author[authorKindDisplay(selected.authorKind)])}</span>{session.mode === "idle" ? <Button variant="outline" disabled={!selected.state || selected.state !== "open"} onClick={beginRevision}>{t(($) => $.search_optimization.suggestions.revise)}</Button> : null}</> : null}
            {session.mode === "idle" ? <Button variant="outline" disabled={pending || !work || !artifact || !latest || !themeId || themesLoading || themesError} onClick={beginCreate}>{t(($) => $.search_optimization.suggestions.create)}</Button> : <><Button variant="outline" disabled={pending} onClick={() => setSession(IDLE_SUGGESTION_DRAFT)}>{t(($) => $.cancel)}</Button><Button disabled={pending || !draft.target_question.trim() || draft.aspects.length === 0 || !draft.proposed_body.trim() || themesLoading || themesError || sourcesLoading || sourcesError || !evidenceValidation.valid || (session.mode === "create" && !themes.some((item) => item.themeId === session.themeId && !item.voided))} onClick={onSave}>{t(($) => $.search_optimization.save)}</Button></>}
          </div>
        </SettingsRow>
        {sourcesError ? <SettingsRow label={t(($) => $.search_optimization.suggestions.evidence)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : !sourcesLoading && !evidenceValidation.valid ? <SettingsRow label={t(($) => $.search_optimization.suggestions.evidence)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.themes.unavailableReference)} · {staleEvidenceIds.join(", ")}</span></SettingsRow> : null}
        {session.mode === "create" ? <SettingsRow label={t(($) => $.search_optimization.suggestions.baseVersion)} description={session.baseVersionId}><div className="max-h-56 overflow-auto whitespace-pre-wrap text-caption text-muted-foreground">{versionsLoading ? t(($) => $.search_optimization.loading) : versionsError ? t(($) => $.search_optimization.loadFailed) : pinnedCreateBase?.body ?? t(($) => $.search_optimization.unknown)}</div></SettingsRow> : null}
        {selected ? <>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.state)} description={stateText(t, selected.state)}><span className="text-caption text-muted-foreground">{selected.failureCode ? t(($) => $.search_optimization.values.failure[suggestionFailureDisplay(selected.failureCode)]) : ""}</span></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.baseVersion)} description={selected.baseVersionId}><div className="max-h-56 overflow-auto whitespace-pre-wrap text-caption text-muted-foreground">{versionsLoading ? t(($) => $.search_optimization.loading) : versionsError ? t(($) => $.search_optimization.loadFailed) : baseVersion?.body ?? t(($) => $.search_optimization.unknown)}</div></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.suggestions.diff)}><SuggestionDiff suggestion={selected} t={t} /></SettingsRow>
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
        {comparison.isError ? <SettingsRow label={t(($) => $.search_optimization.suggestions.compare)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : comparison.isPending && selectedForCompare ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : comparison.data ? <SettingsRow label={comparison.data.sameBase ? t(($) => $.search_optimization.suggestions.sameBase) : t(($) => $.search_optimization.suggestions.differentBase)}>
          <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-4">{comparison.data.suggestions.map((item) => <article key={item.suggestionId} className="min-w-0 space-y-3 rounded border p-3">
            <h3 className="text-body">{item.targetQuestion}</h3>
            <p className="text-caption text-muted-foreground">{item.baseVersionId} · {stateText(t, item.state)}</p>
            <div><div className="font-medium text-caption">{t(($) => $.search_optimization.suggestions.aspects)}</div><div className="text-caption text-muted-foreground">{item.aspects.map((aspect) => aspectText(t, aspect)).join(" · ")}</div></div>
            <div><div className="font-medium text-caption">{t(($) => $.search_optimization.suggestions.rationale)}</div><p className="whitespace-pre-wrap text-caption text-muted-foreground">{item.rationale}</p></div>
            <div><div className="font-medium text-caption">{t(($) => $.search_optimization.suggestions.evidence)}</div><p className="break-all text-caption text-muted-foreground">{item.evidenceSourceIds.map((id) => sources.find((source) => source.id === id)?.label ?? id).join(" · ") || t(($) => $.search_optimization.none)}</p></div>
            <div><div className="font-medium text-caption">{t(($) => $.search_optimization.suggestions.body)}</div><div className="max-h-48 overflow-auto whitespace-pre-wrap text-caption text-muted-foreground">{item.proposedBody}</div></div>
            <div><div className="font-medium text-caption">{t(($) => $.search_optimization.suggestions.diff)} · {item.baseVersionId}</div><SuggestionDiff suggestion={item} t={t} /></div>
          </article>)}</div>
        </SettingsRow> : null}
        {mutationError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{showConflict ? t(($) => $.search_optimization.suggestions.errorConflict) : t(($) => $.search_optimization.saveFailed)}</span></SettingsRow> : null}
        <SettingsRow label={t(($) => $.search_optimization.noAi)}><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.fixedGuidance)} {t(($) => $.search_optimization["optimization.no_ranking_promise"])}</span></SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
