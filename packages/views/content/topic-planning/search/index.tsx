"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  SEARCH_INTENTS, THEME_ORIGINS,
  useCreateSearchTheme, useReviseSearchTheme, useSearchTheme, useSearchThemeRevisions, useSearchThemes,
  searchIntentDisplay, searchOriginDisplay, themeUnknownReasonDisplay,
  themeAccountMatchesPlatform, themeHasQuestionOrKeyword,
  type SearchTheme, type SearchThemeInput,
} from "@multica/core/content/topic-planning";
import { useContentBriefsForTopics, type TopicCard } from "@multica/core/content/topic-planning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";

type Option = { id: string; label: string };
type Translate = ReturnType<typeof useT<"common">>["t"];
type ThemeDraft = Omit<SearchThemeInput, "questions" | "keywords" | "source_ids" | "topic_card_ids" | "brief_revision_ids"> & {
  questionsText: string; keywordsText: string; source_ids: string[]; topic_card_ids: string[]; brief_revision_ids: string[];
};

const BLANK: ThemeDraft = {
  name: "", platform: "xiaohongshu", account_id: "", business_goal: "", questionsText: "", keywordsText: "",
  intent: "unclassified", origin: "manual_keyword", origin_note: "", source_ids: [], topic_card_ids: [], brief_revision_ids: [], note: "",
};

function lines(value: string): string[] {
  return value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean);
}

function CheckboxOptions({
  items, selected, onChange, disabled = false,
}: { items: Option[]; selected: string[]; onChange: (value: string[]) => void; disabled?: boolean }) {
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-2">
      {items.map((item) => (
        <label key={item.id} className="flex items-center gap-2 text-body">
          <Checkbox disabled={disabled} checked={selected.includes(item.id)} onCheckedChange={(checked) => onChange(
            checked === true ? [...selected, item.id] : selected.filter((id) => id !== item.id),
          )} />
          <span>{item.label}</span>
        </label>
      ))}
    </div>
  );
}

function displayIntent(t: Translate, value: string): string {
  const key = searchIntentDisplay(value);
  return t(($) => $.search_optimization.values.intent[key]);
}

function displayOrigin(t: Translate, value: string): string {
  const key = searchOriginDisplay(value);
  return t(($) => $.search_optimization.values.origin[key]);
}

export function SearchThemesSection({
  wsId, topicCards, sources, accounts, platforms, rankObservations = [], onSelectedThemeChange, onConflict,
  topicCardsLoading = false, topicCardsError = false, sourcesLoading = false, sourcesError = false,
  accountsLoading = false, accountsError = false, rankObservationsLoading = false, rankObservationsError = false,
}: {
  wsId: string; topicCards: TopicCard[]; sources: Option[]; accounts: (Option & { platform: string })[]; platforms: string[];
  rankObservations?: { observationId: string; revision: number; query: string; resultKind: string; position: number | null; observedAt: string }[];
  onSelectedThemeChange?: (themeId: string) => void; onConflict?: () => void;
  topicCardsLoading?: boolean; topicCardsError?: boolean; sourcesLoading?: boolean; sourcesError?: boolean;
  accountsLoading?: boolean; accountsError?: boolean; rankObservationsLoading?: boolean; rankObservationsError?: boolean;
}) {
  const { t } = useT("common");
  const [platform, setPlatform] = useState("");
  const [topicCardFilter, setTopicCardFilter] = useState("");
  const [includeArchived, setIncludeArchived] = useState(false);
  const [selectedId, setSelectedId] = useState("");
  const [selectedBaseRevision, setSelectedBaseRevision] = useState<number | null>(null);
  const [draft, setDraft] = useState<ThemeDraft>(BLANK);
  const themes = useSearchThemes(wsId, { platform, topicCardId: topicCardFilter, includeArchived });
  const selectedThemeQuery = useSearchTheme(wsId, selectedId);
  const revisions = useSearchThemeRevisions(wsId, selectedId);
  const create = useCreateSearchTheme(wsId);
  const revise = useReviseSearchTheme(wsId);
  const briefs = useContentBriefsForTopics(wsId, draft.topic_card_ids);
  const list = themes.data ?? [];
  const selected = selectedThemeQuery.data ?? list.find((item) => item.themeId === selectedId) ?? null;
  const selectedAccount = accounts.find((item) => item.id === draft.account_id) ?? null;
  const accountCompatible = !draft.account_id || Boolean(selectedAccount && themeAccountMatchesPlatform(draft.platform, selectedAccount.platform));
  const briefItems = briefs.data.map((item) => ({ id: item.briefRevisionId, label: `${topicCards.find((card) => card.topicCardId === item.topicCardId)?.audienceProblemJudgment || item.topicCardId} · #${item.revision} · ${item.coreProblem}` }));

  function update(patch: Partial<ThemeDraft>) { setDraft((current) => ({ ...current, ...patch })); }
  function toInput(): SearchThemeInput {
    return {
      name: draft.name, platform: draft.platform, account_id: draft.account_id, business_goal: draft.business_goal,
      questions: lines(draft.questionsText), keywords: lines(draft.keywordsText), intent: draft.intent, origin: draft.origin,
      origin_note: draft.origin_note, source_ids: draft.source_ids, topic_card_ids: draft.topic_card_ids,
      brief_revision_ids: draft.brief_revision_ids, note: draft.note,
    };
  }
  function loadTheme(theme: SearchTheme) {
    setSelectedId(theme.themeId);
    setSelectedBaseRevision(theme.revision);
    onSelectedThemeChange?.(theme.themeId);
    setDraft({
      name: theme.name, platform: theme.platform, account_id: theme.accountId, business_goal: theme.businessGoal,
      questionsText: theme.questions.join("\n"), keywordsText: theme.keywords.join("\n"),
      intent: theme.intent === "unknown" ? "unclassified" : theme.intent,
      origin: theme.origin === "unknown" ? "manual_keyword" : theme.origin,
      origin_note: theme.originNote, source_ids: theme.sourceIds, topic_card_ids: theme.topicCardIds,
      brief_revision_ids: theme.briefRevisionIds, note: theme.note,
    });
  }
  function save() {
    const input = toInput();
    if (selectedId) {
      if (!selected || selectedBaseRevision === null) return;
      revise.mutate({ themeId: selectedId, input: { ...input, base_revision: selectedBaseRevision, voided: false } }, {
        onError: refreshOnConflict,
        onSuccess: (result) => { if (result) loadTheme(result); },
      });
      return;
    }
    create.mutate(input, { onError: refreshOnConflict, onSuccess: (result) => { if (result) loadTheme(result); } });
  }
  function archive() {
    if (!selectedId || !selected || selectedBaseRevision === null) return;
    revise.mutate({ themeId: selectedId, input: { ...toInput(), base_revision: selectedBaseRevision, voided: true } }, {
      onError: refreshOnConflict,
      onSuccess: (result) => { if (result) loadTheme(result); },
    });
  }
  function refreshOnConflict(error: Error) {
    if (error.message.includes("409")) onConflict?.();
  }

  const pending = create.isPending || revise.isPending;
  const error = create.error ?? revise.error;
  const selectItems = [{ value: "", label: t(($) => $.search_optimization.themes.allPlatforms) }, ...platforms.map((value) => ({ value, label: value }))];
  const selectedThemeRevisions = revisions.data?.revisions ?? [];
  const optionsForSources = sources;
  const optionsForTopics = topicCards.map((topic) => ({ id: topic.topicCardId, label: topic.audienceProblemJudgment || topic.topicCardId }));
  const unknownReason = selected ? themeUnknownReasonDisplay(selected) : "no_data_source";

  return (
    <SettingsSection title={t(($) => $.search_optimization.themes.title)} description={t(($) => $.search_optimization.themes.description)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.search_optimization.themes.filterPlatform)}>
          <Select items={selectItems} value={platform} onValueChange={(value) => setPlatform(value ?? "")}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.filterPlatform)}><SelectValue /></SelectTrigger>
            <SelectContent>{selectItems.map((item) => <SelectItem key={item.value || "all"} value={item.value}>{item.label}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.filterTopicCard)}>
          <Select items={[{ value: "", label: t(($) => $.search_optimization.themes.allTopicCards) }, ...topicCards.map((item) => ({ value: item.topicCardId, label: item.audienceProblemJudgment || item.topicCardId }))]} value={topicCardFilter} onValueChange={(value) => setTopicCardFilter(value ?? "")}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.filterTopicCard)}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="">{t(($) => $.search_optimization.themes.allTopicCards)}</SelectItem>{topicCards.map((item) => <SelectItem key={item.topicCardId} value={item.topicCardId}>{item.audienceProblemJudgment || item.topicCardId}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.includeArchived)}>
          <Checkbox checked={includeArchived} onCheckedChange={(checked) => setIncludeArchived(checked === true)} />
        </SettingsRow>
        {themes.isPending ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : themes.isError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
        {topicCardsLoading || sourcesLoading || accountsLoading ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
        {topicCardsError || sourcesError || accountsError ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
        {selectedThemeQuery.isError ? <SettingsRow label={t(($) => $.search_optimization.themes.history)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
        <SettingsRow label={t(($) => $.search_optimization.themes.selectTopic)}>
          <Select items={[{ value: "", label: t(($) => $.search_optimization.none) }, ...(selected && !list.some((item) => item.themeId === selected.themeId) ? [{ value: selected.themeId, label: `${selected.name} · ${selected.platform}` }] : []), ...list.map((item) => ({ value: item.themeId, label: `${item.name} · ${item.platform}` }))]} value={selectedId} onValueChange={(value) => {
            const theme = list.find((item) => item.themeId === value);
            if (theme) loadTheme(theme); else if (!value) { setSelectedId(""); setSelectedBaseRevision(null); onSelectedThemeChange?.(""); setDraft(BLANK); }
          }}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.selectTopic)}><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="">{t(($) => $.search_optimization.none)}</SelectItem>{selected && !list.some((item) => item.themeId === selected.themeId) ? <SelectItem value={selected.themeId}>{selected.name} · {selected.platform}</SelectItem> : null}{list.map((item) => <SelectItem key={item.themeId} value={item.themeId}>{item.name} · {item.platform}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        {!themes.isPending && !themes.isError && list.length === 0 ? <SettingsRow label={t(($) => $.search_optimization.empty)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
        {!themes.isPending && !themes.isError ? list.map((theme) => (
          <SettingsRow key={theme.themeId} label={theme.name} description={`${theme.platform} · ${displayIntent(t, theme.intent)} · ${displayOrigin(t, theme.origin)}`}>
            <Button variant="outline" onClick={() => loadTheme(theme)}>{t(($) => $.search_optimization.revise)}</Button>
          </SettingsRow>
        )) : null}
        <SettingsRow label={t(($) => $.search_optimization.themes.name)} size="text"><Input value={draft.name} onChange={(event) => update({ name: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.platform)} size="select-wide">
          <Select items={platforms.map((value) => ({ value, label: value }))} value={draft.platform} onValueChange={(value) => { const next = value ?? platforms[0] ?? ""; update({ platform: next, account_id: themeAccountMatchesPlatform(next, selectedAccount?.platform) ? draft.account_id : "" }); }}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.platform)}><SelectValue /></SelectTrigger><SelectContent>{platforms.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.account)}>
          <Select disabled={accountsLoading || accountsError} items={[{ value: "", label: t(($) => $.search_optimization.themes.brandAccount) }, ...accounts.filter((item) => item.platform === draft.platform).map((item) => ({ value: item.id, label: item.label }))]} value={draft.account_id} onValueChange={(value) => update({ account_id: value ?? "" })}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.account)}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="">{t(($) => $.search_optimization.themes.brandAccount)}</SelectItem>{accounts.filter((item) => item.platform === draft.platform).map((item) => <SelectItem key={item.id} value={item.id}>{item.label}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.goal)} size="text"><Input value={draft.business_goal} onChange={(event) => update({ business_goal: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.questions)} size="text"><Textarea rows={3} value={draft.questionsText} onChange={(event) => update({ questionsText: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.keywords)} size="text"><Textarea rows={3} value={draft.keywordsText} onChange={(event) => update({ keywordsText: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.intent)}>
          <Select items={SEARCH_INTENTS.map((value) => ({ value, label: displayIntent(t, value) }))} value={draft.intent} onValueChange={(value) => update({ intent: (value as ThemeDraft["intent"]) ?? "unclassified" })}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.intent)}><SelectValue /></SelectTrigger><SelectContent>{SEARCH_INTENTS.map((value) => <SelectItem key={value} value={value}>{displayIntent(t, value)}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.origin)}>
          <Select items={THEME_ORIGINS.map((value) => ({ value, label: displayOrigin(t, value) }))} value={draft.origin} onValueChange={(value) => update({ origin: (value as ThemeDraft["origin"]) ?? "manual_keyword" })}>
            <SelectTrigger aria-label={t(($) => $.search_optimization.themes.origin)}><SelectValue /></SelectTrigger><SelectContent>{THEME_ORIGINS.map((value) => <SelectItem key={value} value={value}>{displayOrigin(t, value)}</SelectItem>)}</SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.originNote)} size="text"><Textarea rows={2} value={draft.origin_note} onChange={(event) => update({ origin_note: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.sources)} size="text">{sourcesLoading ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : sourcesError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : <CheckboxOptions items={optionsForSources} selected={draft.source_ids} onChange={(value) => update({ source_ids: value })} />}</SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.topicCards)} size="text">{topicCardsLoading ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : topicCardsError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : <CheckboxOptions items={optionsForTopics} selected={draft.topic_card_ids} onChange={(value) => update({ topic_card_ids: value, brief_revision_ids: [] })} />}</SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.briefs)} size="text">{briefs.isPending ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : briefs.isError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : <CheckboxOptions items={briefItems} selected={draft.brief_revision_ids} onChange={(value) => update({ brief_revision_ids: value })} />}</SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.note)} size="text"><Textarea rows={2} value={draft.note} onChange={(event) => update({ note: event.target.value })} /></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.volume)} description={t(($) => $.search_optimization.themes.unknown)}><span className="text-caption text-muted-foreground">{unknownReason === "no_data_source" ? t(($) => $.search_optimization.values.no_data_source) : t(($) => $.search_optimization.unknown)}</span></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.competition)} description={t(($) => $.search_optimization.themes.unknown)}><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.unknown)}</span></SettingsRow>
        <SettingsRow label={t(($) => $.search_optimization.themes.rank)} description={t(($) => $.search_optimization.themes.noObservations)}>
          {rankObservationsLoading ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : rankObservationsError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : rankObservations.length ? <div className="space-y-1">{rankObservations.map((item) => <div key={`${item.observationId}-${item.revision}`} className="text-caption text-muted-foreground">{item.query} · {item.resultKind === "position" ? `${t(($) => $.search_optimization.observations.position)} ${item.position ?? t(($) => $.search_optimization.unknown)}` : t(($) => $.search_optimization.values.resultKind[item.resultKind === "not_found" ? "not_found" : "unknown"])} · {item.observedAt}</div>)}</div> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.themes.noObservations)}</span>}
        </SettingsRow>
        <SettingsRow label={selected ? `${t(($) => $.search_optimization.themes.history)} · ${selected.revision}` : t(($) => $.search_optimization.themes.history)} size="text">
          {revisions.isPending ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : revisions.isError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : selectedThemeRevisions.length ? <div className="space-y-2">{selectedThemeRevisions.map((revision) => <div key={`${revision.themeId}-${revision.revision}`} className="text-caption text-muted-foreground">#{revision.revision} · {revision.createdAt} · {revision.voided ? t(($) => $.search_optimization.themes.includeArchived) : ""}</div>)}</div> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.empty)}</span>}
        </SettingsRow>
        <SettingsRow label={selected?.voided ? t(($) => $.search_optimization.themes.includeArchived) : t(($) => $.search_optimization.save)}>
          <div className="flex flex-wrap items-center gap-3">
            {error ? <span role="alert" className="text-caption text-muted-foreground">{String(error.message).includes("409") ? t(($) => $.search_optimization.suggestions.errorConflict) : t(($) => $.search_optimization.saveFailed)}</span> : null}
            <Button disabled={pending || !draft.name.trim() || !themeHasQuestionOrKeyword(lines(draft.questionsText), lines(draft.keywordsText)) || !accountCompatible || (selectedId !== "" && (!selected || selectedBaseRevision === null)) || themes.isError || accountsLoading || accountsError || briefs.isError} onClick={save}>{selectedId ? t(($) => $.search_optimization.themes.revise) : t(($) => $.search_optimization.themes.create)}</Button>
            {selected && !selected.voided ? <Button variant="outline" disabled={pending} onClick={archive}>{t(($) => $.search_optimization.themes.archive)}</Button> : null}
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
