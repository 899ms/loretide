"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  RANK_RESULT_KINDS, SEARCH_METRICS, RANK_SINGLE_OBSERVATION_RULE,
  SEARCH_OBSERVATION_CHANNELS, buildRankObservationRequest, buildSearchMetricRequest,
  rankResultDisplay, rankRuleDisplay, searchMetricDisplay, searchMetricValueDisplay, searchMetricValueState,
  useRankObservations, useRecordRankObservation, useRecordSearchMetric, useReviseRankObservation, useSearchMetrics,
  type RankObservation, type RankObservationInput, type RankObservationRevisionInput,
} from "@multica/core/content/feedback-learning";
import type { PublicationRecord } from "@multica/core/content/review-delivery";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";

const ALL = "__all__";
type Translate = ReturnType<typeof useT<"common">>["t"];

function displayMetric(t: Translate, value: string) {
  const key = searchMetricDisplay(value);
  return t(($) => $.search_optimization.values.metric[key]);
}

function displayResult(t: Translate, value: string) {
  const key = rankResultDisplay(value);
  return t(($) => $.search_optimization.values.resultKind[key]);
}

function localDateTime() {
  const date = new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

function metricAmountLabel(t: Translate, value: number | null) {
  return searchMetricValueDisplay(value) === "unknown"
    ? t(($) => $.search_optimization.metrics.unknown)
    : value === 0 ? t(($) => $.search_optimization.metrics.zero) : String(value);
}

function observationInput(observation: RankObservation): RankObservationInput {
  return {
    platform: observation.platform, account_id: observation.accountId, query: observation.query,
    theme_id: observation.themeId, publication_record_id: observation.publicationRecordId,
    observed_at: observation.observedAt, conditions: observation.conditions,
    result_kind: observation.resultKind === "unknown" ? "not_found" : observation.resultKind,
    position: observation.position, scanned_depth: observation.scannedDepth, evidence_note: observation.evidenceNote,
  };
}

export function SearchPerformanceSections({ wsId, publications: publicationRecords, accounts, themes: themeList, onConflict, publicationsLoading = false, publicationsError = false, accountsLoading = false, accountsError = false, themesLoading = false, themesError = false }: { wsId: string; publications: PublicationRecord[]; accounts: { id: string; label: string; platform: string }[]; themes: { themeId: string; name: string; platform: string; accountId: string; voided: boolean }[]; onConflict?: () => void; publicationsLoading?: boolean; publicationsError?: boolean; accountsLoading?: boolean; accountsError?: boolean; themesLoading?: boolean; themesError?: boolean }) {
  const { t } = useT("common");
  const [publicationId, setPublicationId] = useState("");
  const [metricAccountId, setMetricAccountId] = useState("");
  const [metric, setMetric] = useState<(typeof SEARCH_METRICS)[number]>(SEARCH_METRICS[0]);
  const [value, setValue] = useState("");
  const [unit, setUnit] = useState("");
  const [window, setWindow] = useState("");
  const [sampledAt, setSampledAt] = useState(localDateTime());
  const [metricEvidence, setMetricEvidence] = useState("");
  const [themeId, setThemeId] = useState("");
  const [observationPublication, setObservationPublication] = useState("");
  const [query, setQuery] = useState("");
  const [observedAt, setObservedAt] = useState(localDateTime());
  const [conditions, setConditions] = useState("");
  const [resultKind, setResultKind] = useState<(typeof RANK_RESULT_KINDS)[number]>("position");
  const [position, setPosition] = useState("");
  const [scannedDepth, setScannedDepth] = useState("");
  const [observationEvidence, setObservationEvidence] = useState("");
  const [observationAccountId, setObservationAccountId] = useState("");
  const [editingObservationId, setEditingObservationId] = useState("");
  const [observationBaseRevision, setObservationBaseRevision] = useState<number | null>(null);
  const [includeVoided, setIncludeVoided] = useState(false);
  const selectedPublication = publicationRecords.find((item) => item.publicationRecordId === publicationId) ?? null;
  const selectedObservationPublication = publicationRecords.find((item) => item.publicationRecordId === observationPublication) ?? null;
  const selectedTheme = themeList.find((item) => item.themeId === themeId) ?? null;
  const metrics = useSearchMetrics(wsId, publicationId);
  const observations = useRankObservations(wsId, { themeId, publicationRecordId: observationPublication, includeVoided });
  const recordMetric = useRecordSearchMetric(wsId);
  const recordObservation = useRecordRankObservation(wsId);
  const reviseObservation = useReviseRankObservation(wsId);
  const records = publicationRecords;
  const metricRows = metrics.data?.metrics ?? [];
  const singleRule = rankRuleDisplay(RANK_SINGLE_OBSERVATION_RULE);
  const observationPlatform = selectedObservationPublication?.channel || selectedTheme?.platform || "";
  const observationPlatformMismatch = Boolean(selectedObservationPublication && selectedTheme && selectedObservationPublication.channel !== selectedTheme.platform);
  const channelMissing = Boolean(observationPlatform && !SEARCH_OBSERVATION_CHANNELS.includes(observationPlatform as (typeof SEARCH_OBSERVATION_CHANNELS)[number]));
  const error = recordMetric.error ?? recordObservation.error ?? reviseObservation.error;

  const metricItems = SEARCH_METRICS.map((item) => ({ value: item, label: displayMetric(t, item) }));
  const recordItems = [{ value: ALL, label: t(($) => $.search_optimization.none) }, ...records.map((record) => ({ value: record.publicationRecordId, label: publicationLabel(t, record) }))];
  const metricAccountItems = [{ value: ALL, label: t(($) => $.search_optimization.themes.brandAccount) }, ...accounts.filter((account) => account.platform === selectedPublication?.channel).map((account) => ({ value: account.id, label: account.label }))];
  const observationAccountItems = [{ value: ALL, label: t(($) => $.search_optimization.themes.brandAccount) }, ...accounts.filter((account) => account.platform === observationPlatform).map((account) => ({ value: account.id, label: account.label }))];
  const themeItems = [{ value: ALL, label: t(($) => $.search_optimization.none) }, ...themeList.filter((theme) => !theme.voided).map((theme) => ({ value: theme.themeId, label: theme.name }))];
  const resultItems = RANK_RESULT_KINDS.map((item) => ({ value: item, label: displayResult(t, item) }));

  const metricRequest = buildSearchMetricRequest({
    publication: selectedPublication ? { publicationRecordId: selectedPublication.publicationRecordId, channel: selectedPublication.channel, platformAccount: selectedPublication.platformAccount } : null,
    accountId: metricAccountId, accounts, metric, value, unit, statWindow: window, sampledAt, evidenceNote: metricEvidence,
  });
  const observationRequest = buildRankObservationRequest({
    theme: selectedTheme ? { themeId: selectedTheme.themeId, platform: selectedTheme.platform, accountId: selectedTheme.accountId, voided: selectedTheme.voided } : null,
    publication: selectedObservationPublication ? { publicationRecordId: selectedObservationPublication.publicationRecordId, channel: selectedObservationPublication.channel, platformAccount: selectedObservationPublication.platformAccount } : null,
    accountId: observationAccountId, accounts, query, observedAt, conditions,
    resultKind, position, scannedDepth, evidenceNote: observationEvidence,
  });

  function resetObservationEditor() {
    setEditingObservationId(""); setObservationBaseRevision(null); setQuery(""); setConditions(""); setPosition(""); setScannedDepth(""); setObservationEvidence(""); setObservedAt(localDateTime());
  }
  function onMetricSubmit() {
    if (!metricRequest.ok) return;
    recordMetric.mutate(metricRequest.input, { onError: refreshOnConflict, onSuccess: (result) => {
      if (!result) return;
      setValue(""); setUnit(""); setWindow(""); setMetricEvidence(""); setSampledAt(localDateTime());
    } });
  }
  function onObservationSubmit() {
    if (!observationRequest.ok) return;
    if (editingObservationId && observationBaseRevision !== null) {
      const input: RankObservationRevisionInput = { ...observationRequest.input, base_revision: observationBaseRevision, voided: false };
      reviseObservation.mutate({ observationId: editingObservationId, input }, { onError: refreshOnConflict, onSuccess: (result) => { if (result) resetObservationEditor(); } });
      return;
    }
    recordObservation.mutate(observationRequest.input, { onError: refreshOnConflict, onSuccess: (result) => {
      if (!result) return;
      resetObservationEditor();
    } });
  }
  function refreshOnConflict(reason: Error) {
    if (!reason.message.includes("409")) return;
    onConflict?.();
  }
  function voidObservation(observation: RankObservation) {
    const input: RankObservationRevisionInput = {
      ...observationInput(observation), base_revision: observation.revision, voided: true,
    };
    reviseObservation.mutate({ observationId: observation.observationId, input }, { onError: refreshOnConflict });
  }
  function editObservation(observation: RankObservation) {
    setEditingObservationId(observation.observationId); setObservationBaseRevision(observation.revision);
    setThemeId(observation.themeId); setObservationPublication(observation.publicationRecordId); setObservationAccountId(observation.accountId);
    setQuery(observation.query); setObservedAt(new Date(new Date(observation.observedAt).getTime() - new Date(observation.observedAt).getTimezoneOffset() * 60_000).toISOString().slice(0, 16));
    setConditions(observation.conditions); setResultKind(observation.resultKind === "unknown" ? "not_found" : observation.resultKind);
    setPosition(observation.position === null ? "" : String(observation.position)); setScannedDepth(observation.scannedDepth === null ? "" : String(observation.scannedDepth)); setObservationEvidence(observation.evidenceNote);
  }

  const currentMetricsByKind = SEARCH_METRICS.map((kind) => ({
    kind,
    status: searchMetricValueState(metricRows, kind),
    samples: metricRows.filter((row) => row.metric === kind),
  }));

  return (
    <>
      <SettingsSection title={t(($) => $.search_optimization.metrics.title)} description={t(($) => $.search_optimization.metrics.description)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.search_optimization.metrics.publication)}>
            <Select items={recordItems} value={publicationId || ALL} onValueChange={(next) => {
              const id = next === ALL ? "" : next ?? "";
              setPublicationId(id);
              const publication = records.find((item) => item.publicationRecordId === id);
              const account = accounts.find((item) => item.platform === publication?.channel && (item.id === publication?.platformAccount || item.label === publication?.platformAccount));
              setMetricAccountId(account?.id ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.metrics.publication)}><SelectValue /></SelectTrigger><SelectContent>{recordItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.account)}>
            <Select items={metricAccountItems} value={metricAccountId || ALL} onValueChange={(next) => setMetricAccountId(next === ALL ? "" : next ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.metrics.account)}><SelectValue /></SelectTrigger><SelectContent>{metricAccountItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          {publicationsLoading || accountsLoading || (publicationId !== "" && metrics.isPending) ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
          {publicationsError || accountsError || metrics.isError ? <SettingsRow label={t(($) => $.search_optimization.metrics.title)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
          {currentMetricsByKind.map(({ kind, status, samples }) => (
            <SettingsRow key={kind} label={displayMetric(t, kind)} description={publicationId && metrics.isSuccess ? status === "unrecorded" ? t(($) => $.search_optimization.metrics.unrecorded) : status === "unknown" ? t(($) => $.search_optimization.metrics.unknown) : undefined : undefined}>
              {!publicationId ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.select)}</span> : metrics.isPending ? <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loading)}</span> : metrics.isError ? <span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span> : samples.length ? <div className="space-y-1">{samples.map((sample) => <div key={sample.searchMetricId} className="text-caption text-muted-foreground">{metricAmountLabel(t, sample.value)} · {sample.unit} · {sample.statWindow} · {sample.sampledAt}</div>)}</div> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.metrics.unrecorded)}</span>}
            </SettingsRow>
          ))}
          <SettingsRow label={t(($) => $.search_optimization.metrics.metric)}>
            <Select items={metricItems} value={metric} onValueChange={(next) => setMetric((next as typeof metric) ?? SEARCH_METRICS[0])}><SelectTrigger aria-label={t(($) => $.search_optimization.metrics.metric)}><SelectValue /></SelectTrigger><SelectContent>{metricItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.value)} size="text"><Input type="number" min="0" step="1" value={value} onChange={(event) => setValue(event.target.value)} placeholder={t(($) => $.search_optimization.metrics.unknown)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.unit)} size="text"><Input value={unit} onChange={(event) => setUnit(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.window)} size="text"><Input value={window} onChange={(event) => setWindow(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.sampledAt)} size="text"><Input type="datetime-local" value={sampledAt} onChange={(event) => setSampledAt(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.evidence)} size="text"><Textarea rows={2} value={metricEvidence} onChange={(event) => setMetricEvidence(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.record)}><Button disabled={!metricRequest.ok || publicationsLoading || publicationsError || accountsLoading || accountsError || metrics.isError || recordMetric.isPending} onClick={onMetricSubmit}>{t(($) => $.search_optimization.metrics.record)}</Button></SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.search_optimization.observations.title)} description={t(($) => $.search_optimization.observations.singleObservation)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.search_optimization.themes.selectTopic)}>
              <Select disabled={themesLoading || themesError} items={themeItems} value={themeId || ALL} onValueChange={(next) => {
              const id = next === ALL ? "" : next ?? "";
              setThemeId(id);
              setObservationAccountId(themeList.find((item) => item.themeId === id)?.accountId ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.themes.selectTopic)}><SelectValue /></SelectTrigger><SelectContent>{themeItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.publication)}>
            <Select items={recordItems} value={observationPublication || ALL} onValueChange={(next) => {
              const id = next === ALL ? "" : next ?? "";
              setObservationPublication(id);
              const publication = records.find((item) => item.publicationRecordId === id);
              const account = accounts.find((item) => item.platform === publication?.channel && (item.id === publication?.platformAccount || item.label === publication?.platformAccount));
              setObservationAccountId(account?.id ?? selectedTheme?.accountId ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.publication)}><SelectValue /></SelectTrigger><SelectContent>{recordItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.account)}>
            <Select disabled={accountsLoading || accountsError} items={observationAccountItems} value={observationAccountId || ALL} onValueChange={(next) => setObservationAccountId(next === ALL ? "" : next ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.account)}><SelectValue /></SelectTrigger><SelectContent>{observationAccountItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          {publicationsLoading || accountsLoading || themesLoading || ((themeId !== "" || observationPublication !== "") && observations.isPending) ? <SettingsRow label={t(($) => $.search_optimization.loading)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
          {publicationsError || accountsError || themesError || observations.isError ? <SettingsRow label={t(($) => $.search_optimization.observations.title)}><span role="alert" className="text-caption text-muted-foreground">{t(($) => $.search_optimization.loadFailed)}</span></SettingsRow> : null}
          <SettingsRow label={t(($) => $.search_optimization.observations.query)} size="text"><Input value={query} onChange={(event) => setQuery(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.observedAt)} size="text"><Input type="datetime-local" value={observedAt} onChange={(event) => setObservedAt(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.conditions)} size="text"><Textarea rows={2} value={conditions} onChange={(event) => setConditions(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.result)}>
            <Select items={resultItems} value={resultKind} onValueChange={(next) => setResultKind((next as typeof resultKind) ?? "position")}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.result)}><SelectValue /></SelectTrigger><SelectContent>{resultItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          {resultKind === "position" ? <SettingsRow label={t(($) => $.search_optimization.observations.position)} size="text"><Input type="number" min="1" step="1" value={position} onChange={(event) => setPosition(event.target.value)} /></SettingsRow> : <SettingsRow label={t(($) => $.search_optimization.observations.scannedDepth)} size="text"><Input type="number" min="1" step="1" value={scannedDepth} onChange={(event) => setScannedDepth(event.target.value)} /></SettingsRow>}
          <SettingsRow label={t(($) => $.search_optimization.observations.evidence)} size="text"><Textarea rows={2} value={observationEvidence} onChange={(event) => setObservationEvidence(event.target.value)} /></SettingsRow>
          {channelMissing ? <SettingsRow label={t(($) => $.search_optimization.observations.channelMissing)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
          {observationPlatformMismatch ? <SettingsRow label={t(($) => $.search_optimization.observations.title)}><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.invalidContext)}</span></SettingsRow> : null}
          <SettingsRow label={t(($) => $.search_optimization.observations.singleObservation)}><span className="text-caption text-muted-foreground">{singleRule === "rank_single_observation" ? t(($) => $.search_optimization.values.rank_single_observation) : t(($) => $.search_optimization.unknown)}</span></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.includeVoided)}><Checkbox checked={includeVoided} onCheckedChange={(checked) => setIncludeVoided(checked === true)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.record)}><div className="flex flex-wrap gap-3">{editingObservationId ? <Button variant="outline" disabled={reviseObservation.isPending} onClick={resetObservationEditor}>{t(($) => $.cancel)}</Button> : null}<Button disabled={!observationRequest.ok || observationPlatformMismatch || channelMissing || publicationsLoading || publicationsError || accountsLoading || accountsError || themesLoading || themesError || recordObservation.isPending || reviseObservation.isPending} onClick={onObservationSubmit}>{editingObservationId ? t(($) => $.search_optimization.save) : t(($) => $.search_optimization.observations.record)}</Button></div></SettingsRow>
          {!themeId && !observationPublication ? <SettingsRow label={t(($) => $.search_optimization.observations.empty)}><span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.select)}</span></SettingsRow> : observations.isSuccess && (observations.data ?? []).length === 0 ? <SettingsRow label={t(($) => $.search_optimization.observations.empty)}><span className="text-caption text-muted-foreground" /></SettingsRow> : observations.isSuccess ? (observations.data ?? []).map((observation) => (
            <SettingsRow key={`${observation.observationId}-${observation.revision}`} label={`${observation.query} · ${displayResult(t, observation.resultKind)}`} description={`${observation.observedAt} · ${observation.platform}`}>
              <div className="flex flex-wrap items-center gap-3"><span className="text-caption text-muted-foreground">{observation.resultKind === "position" ? `${t(($) => $.search_optimization.observations.position)} ${observation.position ?? t(($) => $.search_optimization.unknown)}` : `${t(($) => $.search_optimization.observations.scannedDepth)} ${observation.scannedDepth ?? t(($) => $.search_optimization.unknown)}`} · {observation.conditions} · {observation.evidenceNote} · {observation.rule}</span>{!observation.voided ? <><Button variant="outline" disabled={reviseObservation.isPending} onClick={() => editObservation(observation)}>{t(($) => $.search_optimization.observations.revise)}</Button><Button variant="outline" disabled={reviseObservation.isPending} onClick={() => voidObservation(observation)}>{t(($) => $.search_optimization.observations.void)}</Button></> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.observations.voided)}</span>}</div>
            </SettingsRow>
          )) : null}
          {error ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{error.message.includes("409") ? t(($) => $.search_optimization.suggestions.errorConflict) : t(($) => $.search_optimization.saveFailed)}</span></SettingsRow> : null}
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function publicationLabel(t: Translate, record: PublicationRecord): string {
  return `${record.channel || t(($) => $.search_optimization.unknown)} · ${record.publishedAt || record.publicationRecordId}`;
}
