"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  RANK_RESULT_KINDS, SEARCH_METRICS, RANK_SINGLE_OBSERVATION_RULE,
  rankResultDisplay, rankRuleDisplay, searchMetricDisplay, searchMetricValueDisplay, searchMetricValueState,
  useRankObservations, useRecordRankObservation, useRecordSearchMetric, useReviseRankObservation, useSearchMetrics,
  type RankObservation, type RankObservationInput, type RankObservationRevisionInput, type SearchMetricInput,
} from "@multica/core/content/feedback-learning";
import type { PublicationRecord } from "@multica/core/content/review-delivery";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@multica/ui/components/ui/select";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";

const PLATFORMS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;
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

export function SearchPerformanceSections({ wsId, publications: publicationRecords, accounts, themes: themeList, onConflict }: { wsId: string; publications: PublicationRecord[]; accounts: { id: string; label: string }[]; themes: { themeId: string; name: string; platform: string; accountId: string; voided: boolean }[]; onConflict?: () => void }) {
  const { t } = useT("common");
  const [publicationId, setPublicationId] = useState("");
  const [accountId, setAccountId] = useState("");
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
  const [includeVoided, setIncludeVoided] = useState(false);
  const selectedPublication = publicationRecords.find((item) => item.publicationRecordId === publicationId) ?? null;
  const selectedTheme = themeList.find((item) => item.themeId === themeId) ?? null;
  const metrics = useSearchMetrics(wsId, publicationId);
  const observations = useRankObservations(wsId, { themeId, publicationRecordId: observationPublication, includeVoided });
  const recordMetric = useRecordSearchMetric(wsId);
  const recordObservation = useRecordRankObservation(wsId);
  const reviseObservation = useReviseRankObservation(wsId);
  const records = publicationRecords;
  const metricRows = metrics.data?.metrics ?? [];
  const singleRule = rankRuleDisplay(RANK_SINGLE_OBSERVATION_RULE);
  const selectedPlatform = selectedPublication?.channel || selectedTheme?.platform || "";
  const channelMissing = Boolean(selectedPlatform && !PLATFORMS.includes(selectedPlatform as (typeof PLATFORMS)[number]));
  const error = recordMetric.error ?? recordObservation.error ?? reviseObservation.error;

  const metricItems = SEARCH_METRICS.map((item) => ({ value: item, label: displayMetric(t, item) }));
  const recordItems = [{ value: ALL, label: t(($) => $.search_optimization.none) }, ...records.map((record) => ({ value: record.publicationRecordId, label: publicationLabel(t, record) }))];
  const accountItems = [{ value: ALL, label: t(($) => $.search_optimization.themes.brandAccount) }, ...accounts.map((account) => ({ value: account.id, label: account.label }))];
  const themeItems = [{ value: ALL, label: t(($) => $.search_optimization.none) }, ...themeList.map((theme) => ({ value: theme.themeId, label: theme.name }))];
  const resultItems = RANK_RESULT_KINDS.map((item) => ({ value: item, label: displayResult(t, item) }));

  function onMetricSubmit() {
    if (!selectedPublication) return;
    const input: SearchMetricInput = {
      publication_record_id: selectedPublication.publicationRecordId, platform: selectedPublication.channel,
      account_id: accountId, metric, value: value.trim() === "" ? null : Number(value),
      unit, stat_window: window, sampled_at: new Date(sampledAt).toISOString(), evidence_note: metricEvidence,
    };
    recordMetric.mutate(input, { onError: refreshOnConflict, onSuccess: (result) => {
      if (!result) return;
      setValue(""); setUnit(""); setWindow(""); setMetricEvidence(""); setSampledAt(localDateTime());
    } });
  }
  function onObservationSubmit() {
    const input: RankObservationInput = {
      platform: selectedPlatform, account_id: accountId || selectedTheme?.accountId || "",
      query, theme_id: themeId, publication_record_id: observationPublication,
      observed_at: new Date(observedAt).toISOString(), conditions, result_kind: resultKind,
      position: resultKind === "position" ? Number(position) : null,
      scanned_depth: resultKind === "not_found" ? Number(scannedDepth) : null,
      evidence_note: observationEvidence,
    };
    recordObservation.mutate(input, { onError: refreshOnConflict, onSuccess: (result) => {
      if (!result) return;
      setQuery(""); setConditions(""); setPosition(""); setScannedDepth(""); setObservationEvidence(""); setObservedAt(localDateTime());
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
              const account = accounts.find((item) => item.id === publication?.platformAccount || item.label === publication?.platformAccount);
              setAccountId(account?.id ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.metrics.publication)}><SelectValue /></SelectTrigger><SelectContent>{recordItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.metrics.account)}>
            <Select items={accountItems} value={accountId || ALL} onValueChange={(next) => setAccountId(next === ALL ? "" : next ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.metrics.account)}><SelectValue /></SelectTrigger><SelectContent>{accountItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          {currentMetricsByKind.map(({ kind, status, samples }) => (
            <SettingsRow key={kind} label={displayMetric(t, kind)} description={status === "unrecorded" ? t(($) => $.search_optimization.metrics.unrecorded) : status === "unknown" ? t(($) => $.search_optimization.metrics.unknown) : undefined}>
              {samples.length ? <div className="space-y-1">{samples.map((sample) => <div key={sample.searchMetricId} className="text-caption text-muted-foreground">{metricAmountLabel(t, sample.value)} · {sample.unit} · {sample.statWindow} · {sample.sampledAt}</div>)}</div> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.metrics.unrecorded)}</span>}
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
          <SettingsRow label={t(($) => $.search_optimization.metrics.record)}><Button disabled={!selectedPublication || recordMetric.isPending || !window.trim() || !sampledAt || !metricEvidence.trim() || (value.trim() !== "" && (!Number.isInteger(Number(value)) || Number(value) < 0))} onClick={onMetricSubmit}>{t(($) => $.search_optimization.metrics.record)}</Button></SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.search_optimization.observations.title)} description={t(($) => $.search_optimization.observations.singleObservation)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.search_optimization.themes.selectTopic)}>
            <Select items={themeItems} value={themeId || ALL} onValueChange={(next) => {
              const id = next === ALL ? "" : next ?? "";
              setThemeId(id);
              setAccountId(themeList.find((item) => item.themeId === id)?.accountId ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.themes.selectTopic)}><SelectValue /></SelectTrigger><SelectContent>{themeItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.publication)}>
            <Select items={recordItems} value={observationPublication || ALL} onValueChange={(next) => {
              const id = next === ALL ? "" : next ?? "";
              setObservationPublication(id);
              const publication = records.find((item) => item.publicationRecordId === id);
              const account = accounts.find((item) => item.id === publication?.platformAccount || item.label === publication?.platformAccount);
              setAccountId(account?.id ?? "");
            }}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.publication)}><SelectValue /></SelectTrigger><SelectContent>{recordItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.account)}>
            <Select items={accountItems} value={accountId || ALL} onValueChange={(next) => setAccountId(next === ALL ? "" : next ?? "")}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.account)}><SelectValue /></SelectTrigger><SelectContent>{accountItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.query)} size="text"><Input value={query} onChange={(event) => setQuery(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.observedAt)} size="text"><Input type="datetime-local" value={observedAt} onChange={(event) => setObservedAt(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.conditions)} size="text"><Textarea rows={2} value={conditions} onChange={(event) => setConditions(event.target.value)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.result)}>
            <Select items={resultItems} value={resultKind} onValueChange={(next) => setResultKind((next as typeof resultKind) ?? "position")}><SelectTrigger aria-label={t(($) => $.search_optimization.observations.result)}><SelectValue /></SelectTrigger><SelectContent>{resultItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent></Select>
          </SettingsRow>
          {resultKind === "position" ? <SettingsRow label={t(($) => $.search_optimization.observations.position)} size="text"><Input type="number" min="1" step="1" value={position} onChange={(event) => setPosition(event.target.value)} /></SettingsRow> : <SettingsRow label={t(($) => $.search_optimization.observations.scannedDepth)} size="text"><Input type="number" min="1" step="1" value={scannedDepth} onChange={(event) => setScannedDepth(event.target.value)} /></SettingsRow>}
          <SettingsRow label={t(($) => $.search_optimization.observations.evidence)} size="text"><Textarea rows={2} value={observationEvidence} onChange={(event) => setObservationEvidence(event.target.value)} /></SettingsRow>
          {channelMissing ? <SettingsRow label={t(($) => $.search_optimization.observations.channelMissing)}><span className="text-caption text-muted-foreground" /></SettingsRow> : null}
          <SettingsRow label={t(($) => $.search_optimization.observations.singleObservation)}><span className="text-caption text-muted-foreground">{singleRule === "rank_single_observation" ? t(($) => $.search_optimization.values.rank_single_observation) : t(($) => $.search_optimization.unknown)}</span></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.includeVoided)}><Checkbox checked={includeVoided} onCheckedChange={(checked) => setIncludeVoided(checked === true)} /></SettingsRow>
          <SettingsRow label={t(($) => $.search_optimization.observations.record)}><Button disabled={channelMissing || !PLATFORMS.includes(selectedPlatform as (typeof PLATFORMS)[number]) || (!themeId && !observationPublication) || !query.trim() || !conditions.trim() || !observationEvidence.trim() || !observedAt || (resultKind === "position" ? !positive(position) : !positive(scannedDepth)) || recordObservation.isPending} onClick={onObservationSubmit}>{t(($) => $.search_optimization.observations.record)}</Button></SettingsRow>
          {(observations.data ?? []).length === 0 ? <SettingsRow label={t(($) => $.search_optimization.observations.empty)}><span className="text-caption text-muted-foreground" /></SettingsRow> : (observations.data ?? []).map((observation) => (
            <SettingsRow key={`${observation.observationId}-${observation.revision}`} label={`${observation.query} · ${displayResult(t, observation.resultKind)}`} description={`${observation.observedAt} · ${observation.platform}`}>
              <div className="flex flex-wrap items-center gap-3"><span className="text-caption text-muted-foreground">{observation.resultKind === "position" ? `${t(($) => $.search_optimization.observations.position)} ${observation.position ?? t(($) => $.search_optimization.unknown)}` : `${t(($) => $.search_optimization.observations.scannedDepth)} ${observation.scannedDepth ?? t(($) => $.search_optimization.unknown)}`} · {observation.conditions} · {observation.evidenceNote} · {observation.rule}</span>{!observation.voided ? <Button variant="outline" disabled={reviseObservation.isPending} onClick={() => voidObservation(observation)}>{t(($) => $.search_optimization.observations.void)}</Button> : <span className="text-caption text-muted-foreground">{t(($) => $.search_optimization.observations.voided)}</span>}</div>
            </SettingsRow>
          ))}
          {error ? <SettingsRow label={t(($) => $.search_optimization.saveFailed)}><span role="alert" className="text-caption text-muted-foreground">{error.message.includes("409") ? t(($) => $.search_optimization.suggestions.errorConflict) : t(($) => $.search_optimization.saveFailed)}</span></SettingsRow> : null}
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function positive(value: string): boolean {
  return value.trim() !== "" && Number.isInteger(Number(value)) && Number(value) > 0;
}

function publicationLabel(t: Translate, record: PublicationRecord): string {
  return `${record.channel || t(($) => $.search_optimization.unknown)} · ${record.publishedAt || record.publicationRecordId}`;
}
