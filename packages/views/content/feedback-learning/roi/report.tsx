"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_ATTRIBUTION_METHODS,
  ROI_CURRENCIES,
  ROI_SAVED,
  roiAmountView,
  roiMetricLabelKey,
  roiMetricView,
  roiMinorText,
  roiPath,
  roiWriteOutcome,
  useGenerateRoiReport,
  useRoiPreview,
  useRoiReports,
  useRoiReportVersion,
  useRoiReportVersions,
  type RoiBreakdownRow,
  type RoiMetric,
  type RoiReportVersion,
  type RoiResult,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";
import {
  ChoiceSelect,
  CurrencySelect,
  ReadOnlyList,
  SaveFeedback,
  StateRow,
  formatTime,
  joinList,
  labeled,
  methodLabel,
  notComputableLabel,
  optionName,
  recordKindLabel,
  type RoiFocus,
  type RoiOptions,
  type Translate,
} from "./shared";

// The report block (specs/034 PR 5, T091; U-21 to U-32, U-36).
//
// Every figure here is the server's. The page sends parameters, receives a
// result (a preview) or a stored version, and shows each metric's `display`
// string as written - or "not computable" with its reason, never 0, blank, ∞
// or NaN. Three ratios are three different things and are never called by
// each other's names: the business ROI (profit), the revenue-to-spend ratio
// and the ad ROAS (both revenue).
//
// A stored version is never recomputed on the page. Whether its inputs have
// moved on is the server's derivation, shown as a notice with the records
// that changed; the fix is a new version, and the old one stays.
//
// The AI explanation is always "pending": no model is called, and there is
// nothing to adopt.

interface RateDraft {
  from: string;
  rate: string;
  note: string;
}

interface ParamsDraft {
  title: string;
  start: string;
  end: string;
  currency: string;
  rates: RateDraft[];
  method: string;
  fromStage: string;
  toStage: string;
  bookingStage: string;
  accountIds: string[];
  workIds: string[];
  campaignLabels: string;
}

function monthBounds(): { start: string; end: string } {
  const now = new Date();
  const pad = (part: number) => String(part).padStart(2, "0");
  const last = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate();
  const month = `${now.getFullYear()}-${pad(now.getMonth() + 1)}`;
  return { start: `${month}-01`, end: `${month}-${pad(last)}` };
}

function emptyParams(): ParamsDraft {
  const { start, end } = monthBounds();
  return {
    title: "",
    start,
    end,
    currency: "CNY",
    rates: [],
    method: "even_split",
    fromStage: "",
    toStage: "",
    bookingStage: "",
    accountIds: [],
    workIds: [],
    campaignLabels: "",
  };
}

/** Contract §5.1. The server writes the time zone (the brand's), the
 *  generation time and who entered each rate. */
function paramsBody(draft: ParamsDraft): Record<string, unknown> {
  return {
    window: { start: draft.start, end: draft.end },
    report_currency: draft.currency,
    rates: draft.rates.map((rate) => ({
      from: rate.from,
      to: draft.currency,
      rate: rate.rate.trim(),
      note: rate.note,
    })),
    attribution_method: draft.method,
    conversion: { from_stage: draft.fromStage.trim(), to_stage: draft.toStage.trim() },
    booking_stage: draft.bookingStage.trim(),
    scope: {
      account_ids: draft.accountIds,
      work_ids: draft.workIds,
      campaign_labels: draft.campaignLabels
        .split(/[,，]/)
        .map((label) => label.trim())
        .filter((label) => label !== ""),
    },
  };
}

function paramsProblems(t: Translate, draft: ParamsDraft): string[] {
  const problems: string[] = [];
  if (!/^\d{4}-\d{2}-\d{2}$/.test(draft.start) || !/^\d{4}-\d{2}-\d{2}$/.test(draft.end)) {
    problems.push(t(($) => $.contentRoiReview.missingField, { field: t(($) => $.contentRoiReview.report.window) }));
  } else if (draft.end < draft.start) {
    problems.push(t(($) => $.contentRoiReview.report.windowBackwards));
  }
  if (draft.rates.some((rate) => !/^\d+(\.\d+)?$/.test(rate.rate.trim()) || rate.from === draft.currency)) {
    problems.push(t(($) => $.contentRoiReview.report.rateInvalid));
  }
  return problems;
}

// ---------------------------------------------------------------------------

export function ReportBlock({
  wsId,
  brandTimezone,
  options,
  onFocus,
}: {
  wsId: string;
  brandTimezone: string;
  options: RoiOptions;
  onFocus: (focus: RoiFocus) => void;
}) {
  const { t } = useT("common");
  const [draft, setDraft] = useState<ParamsDraft>(emptyParams);
  const preview = useRoiPreview();
  const generate = useGenerateRoiReport(wsId);
  const [previewed, setPreviewed] = useState<{ result: RoiResult; body: Record<string, unknown> } | null>(null);
  const [outcome, setOutcome] = useState<RoiWriteOutcome | null>(null);
  const [openReport, setOpenReport] = useState<{ reportId: string; versionNo: number } | null>(null);
  const problems = paramsProblems(t, draft);
  const pending = preview.isPending || generate.isPending;

  const runPreview = () => {
    const body = paramsBody(draft);
    setOutcome(null);
    preview.mutate(body, {
      onSuccess: (result) => {
        if (result) {
          setPreviewed({ result, body });
          setOutcome(ROI_SAVED);
        } else {
          setOutcome({ kind: "failed", nextAction: "", traceId: "" });
        }
      },
      onError: (error) => setOutcome(roiWriteOutcome(error)),
    });
  };

  const runGenerate = () => {
    setOutcome(null);
    generate.mutate(
      { path: "reports", body: { title: draft.title.trim(), params: paramsBody(draft) } },
      {
        onSuccess: (version) => {
          setOutcome(ROI_SAVED);
          setPreviewed(null);
          if (version) setOpenReport({ reportId: version.reportId, versionNo: version.versionNo });
        },
        onError: (error) => setOutcome(roiWriteOutcome(error)),
      },
    );
  };

  return (
    <>
      <ParamsSection
        draft={draft}
        onChange={setDraft}
        disabled={pending}
        brandTimezone={brandTimezone}
        options={options}
      />
      <SettingsSection>
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentRoiReview.report.run)}
            description={problems.length > 0 ? problems.join(" · ") : t(($) => $.contentRoiReview.report.runHint)}
          >
            <div className="flex items-center gap-3">
              <SaveFeedback outcome={outcome} pending={pending} />
              <Button variant="outline" onClick={runPreview} disabled={pending || problems.length > 0}>
                {t(($) => $.contentRoiReview.report.preview)}
              </Button>
              <Button onClick={runGenerate} disabled={pending || problems.length > 0}>
                {t(($) => $.contentRoiReview.report.generate)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {previewed ? (
        <ResultView
          title={t(($) => $.contentRoiReview.report.previewTitle)}
          description={t(($) => $.contentRoiReview.report.previewNotSaved)}
          result={previewed.result}
          rates={previewed.body.rates}
          params={previewed.body}
          options={options}
          onFocus={onFocus}
        />
      ) : null}

      <ReportList wsId={wsId} open={openReport} onOpen={setOpenReport} options={options} onFocus={onFocus} />
    </>
  );
}

function ParamsSection({
  draft,
  onChange,
  disabled,
  brandTimezone,
  options,
}: {
  draft: ParamsDraft;
  onChange: (next: ParamsDraft) => void;
  disabled: boolean;
  brandTimezone: string;
  options: RoiOptions;
}) {
  const { t } = useT("common");
  const edit = (patch: Partial<ParamsDraft>) => onChange({ ...draft, ...patch });
  const methodItems = ROI_ATTRIBUTION_METHODS.map((value) => ({ value, label: methodLabel(t, value) }));
  const otherCurrencies = ROI_CURRENCIES.map((item) => item.code).filter((code) => code !== draft.currency);
  const setRate = (index: number, patch: Partial<RateDraft>) =>
    edit({ rates: draft.rates.map((rate, at) => (at === index ? { ...rate, ...patch } : rate)) });
  const toggle = (list: string[], id: string, on: boolean) => (on ? [...list, id] : list.filter((item) => item !== id));

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.report.paramsTitle)}
      description={t(($) => $.contentRoiReview.report.paramsDescription)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentRoiReview.report.reportTitle)} size="text">
          <Input
            value={draft.title}
            onChange={(event) => edit({ title: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.reportTitle)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.windowStart)}
          description={t(($) => $.contentRoiReview.report.timezone, { timezone: brandTimezone })}
          size="text"
        >
          <Input
            type="date"
            value={draft.start}
            onChange={(event) => edit({ start: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.windowStart)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.report.windowEnd)} size="text">
          <Input
            type="date"
            value={draft.end}
            onChange={(event) => edit({ end: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.windowEnd)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.report.reportCurrency)} size="select">
          <CurrencySelect
            value={draft.currency}
            onChange={(currency) => edit({ currency, rates: draft.rates.filter((rate) => rate.from !== currency) })}
            label={t(($) => $.contentRoiReview.report.reportCurrency)}
            disabled={disabled}
          />
        </SettingsRow>
        {draft.rates.map((rate, index) => (
          <SettingsRow
            key={index}
            label={t(($) => $.contentRoiReview.report.rateNo, { number: index + 1 })}
            description={t(($) => $.contentRoiReview.report.rateMeaning, {
              from: rate.from,
              to: draft.currency,
              rate: rate.rate || "?",
            })}
            size="text"
            align="start"
          >
            <div className="space-y-2">
              <ChoiceSelect
                items={otherCurrencies.map((code) => ({ value: code, label: code }))}
                value={rate.from}
                onChange={(from) => setRate(index, { from })}
                label={t(($) => $.contentRoiReview.report.rateFrom)}
                disabled={disabled}
              />
              <Input
                value={rate.rate}
                inputMode="decimal"
                onChange={(event) => setRate(index, { rate: event.target.value })}
                placeholder="7.1234"
                aria-label={t(($) => $.contentRoiReview.report.rate)}
                disabled={disabled}
              />
              <Input
                value={rate.note}
                onChange={(event) => setRate(index, { note: event.target.value })}
                placeholder={t(($) => $.contentRoiReview.report.rateNote)}
                aria-label={t(($) => $.contentRoiReview.report.rateNote)}
                disabled={disabled}
              />
              <Button
                variant="ghost"
                size="sm"
                onClick={() => edit({ rates: draft.rates.filter((_, at) => at !== index) })}
                disabled={disabled}
              >
                {t(($) => $.contentRoiReview.remove)}
              </Button>
            </div>
          </SettingsRow>
        ))}
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.addRate)}
          description={t(($) => $.contentRoiReview.report.ratesHint)}
        >
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const unused = otherCurrencies.find((code) => !draft.rates.some((rate) => rate.from === code));
              if (unused) edit({ rates: [...draft.rates, { from: unused, rate: "", note: "" }] });
            }}
            disabled={disabled || draft.rates.length >= otherCurrencies.length}
          >
            {t(($) => $.contentRoiReview.report.addRate)}
          </Button>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.method)}
          description={t(($) => $.contentRoiReview.report.methodHint)}
          size="select-wide"
        >
          <ChoiceSelect
            items={methodItems}
            value={draft.method}
            onChange={(method) => edit({ method })}
            label={t(($) => $.contentRoiReview.report.method)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.report.fromStage)} size="text">
          <Input
            value={draft.fromStage}
            onChange={(event) => edit({ fromStage: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.fromStage)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.toStage)}
          description={t(($) => $.contentRoiReview.report.stageLimitation)}
          size="text"
        >
          <Input
            value={draft.toStage}
            onChange={(event) => edit({ toStage: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.toStage)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.report.bookingStage)} size="text">
          <Input
            value={draft.bookingStage}
            onChange={(event) => edit({ bookingStage: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.bookingStage)}
            disabled={disabled}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.scopeAccounts)}
          description={t(($) => $.contentRoiReview.report.scopeHint)}
          size="text"
          align="start"
        >
          <div className="space-y-2">
            {options.accounts.length === 0 ? (
              <span className="text-body text-muted-foreground">{t(($) => $.contentRoiReview.report.scopeNone)}</span>
            ) : (
              options.accounts.map((account) => (
                <label key={account.id} className="flex items-start gap-3 text-body text-muted-foreground">
                  <Checkbox
                    checked={draft.accountIds.includes(account.id)}
                    onCheckedChange={(value) => edit({ accountIds: toggle(draft.accountIds, account.id, value === true) })}
                    aria-label={account.name || account.id}
                    disabled={disabled}
                  />
                  <span className="break-words">{account.name || account.id}</span>
                </label>
              ))
            )}
          </div>
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.report.scopeWorks)} size="text" align="start">
          <div className="space-y-2">
            {options.works.length === 0 ? (
              <span className="text-body text-muted-foreground">{t(($) => $.contentRoiReview.report.scopeNone)}</span>
            ) : (
              options.works.map((work) => (
                <label key={work.id} className="flex items-start gap-3 text-body text-muted-foreground">
                  <Checkbox
                    checked={draft.workIds.includes(work.id)}
                    onCheckedChange={(value) => edit({ workIds: toggle(draft.workIds, work.id, value === true) })}
                    aria-label={work.name || work.id}
                    disabled={disabled}
                  />
                  <span className="break-words">{work.name || work.id}</span>
                </label>
              ))
            )}
          </div>
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.report.scopeLabels)}
          description={t(($) => $.contentRoiReview.report.scopeLabelsHint)}
          size="text"
        >
          <Input
            value={draft.campaignLabels}
            onChange={(event) => edit({ campaignLabels: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.report.scopeLabels)}
            disabled={disabled}
          />
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// Stored reports and their versions.

function ReportList({
  wsId,
  open,
  onOpen,
  options,
  onFocus,
}: {
  wsId: string;
  open: { reportId: string; versionNo: number } | null;
  onOpen: (open: { reportId: string; versionNo: number } | null) => void;
  options: RoiOptions;
  onFocus: (focus: RoiFocus) => void;
}) {
  const { t } = useT("common");
  const reports = useRoiReports(wsId);
  const list = reports.data ?? [];
  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.report.listTitle)}>
        <SettingsCard>
          {reports.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : reports.isError ? (
            <StateRow label={t(($) => $.contentRoiReview.loadFailed)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.report.listEmpty)} />
          ) : (
            list.map((report) => (
              <SettingsRow
                key={report.reportId}
                label={
                  <button
                    type="button"
                    onClick={() => onOpen({ reportId: report.reportId, versionNo: report.versionNo })}
                    data-active={report.reportId === open?.reportId}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {report.title || report.reportId}
                  </button>
                }
                description={[
                  t(($) => $.contentRoiReview.report.versionNo, { version: report.versionNo }),
                  report.calcVersion,
                  t(($) => $.contentRoiReview.recordedBy, { who: report.createdBy, at: formatTime(report.createdAt) }),
                ].join(" · ")}
                size="select-wide"
              >
                <span className="text-caption text-muted-foreground">{report.reportId}</span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>
      {open ? (
        <ReportVersions
          key={open.reportId}
          wsId={wsId}
          reportId={open.reportId}
          versionNo={open.versionNo}
          onVersion={(versionNo) => onOpen({ reportId: open.reportId, versionNo })}
          options={options}
          onFocus={onFocus}
        />
      ) : null}
    </>
  );
}

function ReportVersions({
  wsId,
  reportId,
  versionNo,
  onVersion,
  options,
  onFocus,
}: {
  wsId: string;
  reportId: string;
  versionNo: number;
  onVersion: (versionNo: number) => void;
  options: RoiOptions;
  onFocus: (focus: RoiFocus) => void;
}) {
  const { t } = useT("common");
  const versions = useRoiReportVersions(wsId, reportId);
  const version = useRoiReportVersion(wsId, reportId, versionNo);
  const generate = useGenerateRoiReport(wsId);
  const [outcome, setOutcome] = useState<RoiWriteOutcome | null>(null);
  const list = [...(versions.data?.versions ?? [])].sort((a, b) => b.versionNo - a.versionNo);

  const regenerate = () => {
    setOutcome(null);
    generate.mutate(
      { path: roiPath("reports", reportId, "versions"), body: {} },
      {
        onSuccess: (next) => {
          setOutcome(ROI_SAVED);
          if (next) onVersion(next.versionNo);
        },
        onError: (error) => setOutcome(roiWriteOutcome(error)),
      },
    );
  };

  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.report.versionsTitle)}>
        <SettingsCard>
          {versions.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.notFound)} />
          ) : (
            list.map((item) => (
              <SettingsRow
                key={item.versionNo}
                label={
                  <button
                    type="button"
                    onClick={() => onVersion(item.versionNo)}
                    data-active={item.versionNo === versionNo}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {t(($) => $.contentRoiReview.report.versionNo, { version: item.versionNo })}
                  </button>
                }
                description={[
                  item.title,
                  item.calcVersion,
                  t(($) => $.contentRoiReview.recordedBy, { who: item.createdBy, at: formatTime(item.createdAt) }),
                ]
                  .filter((part) => part !== "")
                  .join(" · ")}
              >
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ))
          )}
          <SettingsRow
            label={t(($) => $.contentRoiReview.report.regenerate)}
            description={t(($) => $.contentRoiReview.report.regenerateHint)}
          >
            <div className="flex items-center gap-3">
              <SaveFeedback outcome={outcome} pending={generate.isPending} />
              <Button variant="outline" onClick={regenerate} disabled={generate.isPending}>
                {t(($) => $.contentRoiReview.report.regenerate)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
      {version.isLoading ? null : version.data ? (
        <VersionView version={version.data} options={options} onFocus={onFocus} onRegenerate={regenerate} pending={generate.isPending} />
      ) : (
        <SettingsSection>
          <SettingsCard>
            <StateRow label={t(($) => $.contentRoiReview.notFound)} />
          </SettingsCard>
        </SettingsSection>
      )}
    </>
  );
}

function changedInputText(t: Translate, change: RoiReportVersion["changedInputs"][number]): string {
  const record = `${recordKindLabel(t, change.kind)} ${change.id}`;
  if (change.reportRevision === null) return t(($) => $.contentRoiReview.report.changeNew, { record });
  if (change.currentRevision === null) {
    return t(($) => $.contentRoiReview.report.changeGone, { record, revision: change.reportRevision });
  }
  return t(($) => $.contentRoiReview.report.changeRevised, {
    record,
    from: change.reportRevision,
    to: change.currentRevision,
  });
}

function VersionView({
  version,
  options,
  onFocus,
  onRegenerate,
  pending,
}: {
  version: RoiReportVersion;
  options: RoiOptions;
  onFocus: (focus: RoiFocus) => void;
  onRegenerate: () => void;
  pending: boolean;
}) {
  const { t } = useT("common");
  return (
    <>
      {version.inputsChanged ? (
        <SettingsSection>
          <SettingsCard>
            <SettingsRow
              label={t(($) => $.contentRoiReview.report.inputsChanged, { changed: version.changedInputs.length })}
              description={t(($) => $.contentRoiReview.report.inputsChangedHint)}
              size="text"
              align="start"
            >
              <div className="space-y-2">
                <ReadOnlyList items={version.changedInputs.map((change) => changedInputText(t, change))} empty="" />
                <Button variant="outline" size="sm" onClick={onRegenerate} disabled={pending}>
                  {t(($) => $.contentRoiReview.report.regenerate)}
                </Button>
              </div>
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}
      <ResultView
        title={t(($) => $.contentRoiReview.report.versionTitle, {
          title: version.title || version.reportId,
          version: version.versionNo,
        })}
        description={t(($) => $.contentRoiReview.recordedBy, { who: version.createdBy, at: formatTime(version.createdAt) })}
        result={version.result}
        rates={version.params.rates}
        params={version.params}
        options={options}
        onFocus={onFocus}
      />
      <SettingsSection title={t(($) => $.contentRoiReview.report.aiTitle)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentRoiReview.report.aiState)}>
            <span className="text-body text-muted-foreground">{t(($) => $.contentRoiReview.report.aiPending)}</span>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

// ---------------------------------------------------------------------------
// One result: header, metrics with their provenance, breakdown, rules.

/** The metrics in the order the page shows them, grouped. */
const METRIC_GROUPS = [
  { key: "spend", ids: ["spend_total", "ad_spend_total"] },
  { key: "funnel", ids: ["qualified_leads", "bookings", "deals", "conversion_rate"] },
  { key: "revenue", ids: ["net_revenue", "attributed_net_revenue", "attributed_gross_profit"] },
  { key: "coverage", ids: ["attribution_coverage_count", "attribution_coverage_amount"] },
  { key: "unit", ids: ["cost_per_qualified_lead", "cost_per_deal"] },
  { key: "returns", ids: ["business_roi", "revenue_to_spend", "ad_roas"] },
] as const;

function textOf(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function ratesText(rates: unknown): string[] {
  if (!Array.isArray(rates)) return [];
  return rates.map((rate) => {
    const item = (rate ?? {}) as Record<string, unknown>;
    return `${textOf(item.from)} → ${textOf(item.to)} ${textOf(item.rate)}${textOf(item.note) ? ` (${textOf(item.note)})` : ""}`;
  });
}

function ResultView({
  title,
  description,
  result,
  rates,
  params,
  options,
  onFocus,
}: {
  title: string;
  description: string;
  result: RoiResult;
  rates: unknown;
  params: Record<string, unknown>;
  options: RoiOptions;
  onFocus: (focus: RoiFocus) => void;
}) {
  const { t } = useT("common");
  const conversion = (params.conversion ?? {}) as Record<string, unknown>;
  const rateLines = ratesText(rates);
  const breakdown = result.breakdown;
  return (
    <>
      <SettingsSection title={title} description={description}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentRoiReview.report.window)}>
            <span className="text-body">
              {t(($) => $.contentRoiReview.report.windowShown, {
                start: result.window.start,
                end: result.window.end,
                timezone: result.window.timezone,
              })}
            </span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentRoiReview.report.reportCurrency)}>
            <span className="text-body">{result.reportCurrency}</span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentRoiReview.report.method)}>
            <span className="text-body">{methodLabel(t, result.attributionMethod)}</span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentRoiReview.report.ratesUsed)} size="text" align="start">
            <ReadOnlyList items={rateLines} empty={t(($) => $.contentRoiReview.report.noRates)} />
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentRoiReview.report.calcVersion)}>
            <span className="text-body">{result.calcVersion}</span>
          </SettingsRow>
          <SettingsRow label={t(($) => $.contentRoiReview.report.generatedAt)}>
            <span className="text-body">{formatTime(result.generatedAt)}</span>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {METRIC_GROUPS.map((group) => (
        <SettingsSection key={group.key} title={t(($) => $.contentRoiReview.report.groups[group.key])}>
          <SettingsCard>
            {group.ids.map((id) => (
              <MetricRow
                key={id}
                id={id}
                metric={result.metrics[id]}
                calcVersion={result.calcVersion}
                reportCurrency={result.reportCurrency}
                fromStage={textOf(conversion.from_stage)}
                toStage={textOf(conversion.to_stage)}
                onFocus={onFocus}
              />
            ))}
          </SettingsCard>
        </SettingsSection>
      ))}

      <SettingsSection
        title={t(($) => $.contentRoiReview.report.breakdownTitle)}
        description={t(($) => $.contentRoiReview.report.breakdownNotAdditive)}
      >
        <SettingsCard>
          {breakdown.byWork.map((row) => (
            <BreakdownLine key={`work-${row.id}`} label={labeled(t, t(($) => $.contentRoiReview.work), optionName(options.works, row.id))} row={row} />
          ))}
          {breakdown.byAccount.map((row) => (
            <BreakdownLine key={`account-${row.id}`} label={labeled(t, t(($) => $.contentRoiReview.account), optionName(options.accounts, row.id))} row={row} />
          ))}
          {breakdown.accountLevelUnknownWork ? (
            <BreakdownLine label={t(($) => $.contentRoiReview.report.accountLevel)} row={breakdown.accountLevelUnknownWork} muted />
          ) : null}
          {breakdown.brandLevelUnknownAccount ? (
            <BreakdownLine label={t(($) => $.contentRoiReview.report.brandLevel)} row={breakdown.brandLevelUnknownAccount} muted />
          ) : null}
          {breakdown.unattributed ? (
            <BreakdownLine label={t(($) => $.contentRoiReview.report.unattributed)} row={breakdown.unattributed} muted />
          ) : null}
          {breakdown.byWork.length === 0 && breakdown.byAccount.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.report.breakdownEmpty)} />
          ) : null}
          {breakdown.evenSplitFallback.length > 0 ? (
            <SettingsRow
              label={t(($) => $.contentRoiReview.report.evenSplitFallback)}
              description={joinList(t, breakdown.evenSplitFallback)}
            >
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : null}
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.contentRoiReview.report.rulesTitle)}>
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentRoiReview.report.rulesLabel)} size="text" align="start">
            <ReadOnlyList items={result.rules} empty={t(($) => $.contentRoiReview.blank)} />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    </>
  );
}

function BreakdownLine({ label, row, muted }: { label: string; row: RoiBreakdownRow; muted?: boolean }) {
  const { t } = useT("common");
  const amount = (value: RoiBreakdownRow["attributedNetRevenue"]) => {
    const view = roiAmountView(value);
    return view.kind === "value" ? view.display : notComputableLabel(t, view.reason);
  };
  return (
    <SettingsRow
      label={<span className={muted ? "text-muted-foreground" : undefined}>{label}</span>}
      description={t(($) => $.contentRoiReview.report.dealsTouched, { deals: row.dealsTouched })}
      size="text"
      align="start"
    >
      <ReadOnlyList
        items={[
          labeled(t, t(($) => $.contentRoiReview.metrics.attributed_net_revenue), amount(row.attributedNetRevenue)),
          labeled(t, t(($) => $.contentRoiReview.metrics.attributed_gross_profit), amount(row.attributedGrossProfit)),
        ]}
        empty=""
      />
    </SettingsRow>
  );
}

function MetricRow({
  id,
  metric,
  calcVersion,
  fromStage,
  toStage,
  onFocus,
  reportCurrency,
}: {
  id: string;
  metric: RoiMetric | undefined;
  reportCurrency: string;
  calcVersion: string;
  fromStage: string;
  toStage: string;
  onFocus: (focus: RoiFocus) => void;
}) {
  const { t } = useT("common");
  const [open, setOpen] = useState(false);
  const view = roiMetricView(metric);
  const labelKey = roiMetricLabelKey(id);
  const formula = labelKey === "default" ? "" : t(($) => $.contentRoiReview.formulas[labelKey]);
  const stages =
    id === "conversion_rate"
      ? t(($) => $.contentRoiReview.report.conversionStages, {
          from: fromStage || t(($) => $.contentRoiReview.report.stageNotSet),
          to: toStage || t(($) => $.contentRoiReview.report.stageNotSet),
        })
      : "";
  const records = metric?.records ?? [];

  return (
    <>
      <SettingsRow
        label={
          <button
            type="button"
            onClick={() => setOpen(!open)}
            className="break-words text-left text-body hover:text-foreground"
            aria-expanded={open}
          >
            {t(($) => $.contentRoiReview.metrics[labelKey])}
          </button>
        }
        description={[formula, stages, id === "conversion_rate" ? t(($) => $.contentRoiReview.report.stageLimitation) : ""]
          .filter((part) => part !== "")
          .join(" · ")}
        size="select-wide"
      >
        {view.kind === "value" ? (
          <span className={view.negative ? "text-body text-muted-foreground" : "text-body font-medium"}>{view.display}</span>
        ) : (
          <span className="text-body text-muted-foreground">{notComputableLabel(t, view.reason)}</span>
        )}
      </SettingsRow>
      {open ? (
        <SettingsRow label={t(($) => $.contentRoiReview.report.provenance)} size="text" align="start">
          <div className="space-y-2">
            <span className="block text-caption text-muted-foreground">
              {t(($) => $.contentRoiReview.report.formulaId, {
                formula: metric?.formula || "-",
                calc: calcVersion,
              })}
            </span>
            {metric && metric.numerator !== "" ? (
              <span className="block text-caption text-muted-foreground">
                {t(($) => $.contentRoiReview.report.fraction, {
                  numerator: metric.numerator,
                  denominator: metric.denominator || "-",
                })}
              </span>
            ) : null}
            {records.length === 0 ? (
              <span className="block text-body text-muted-foreground">{t(($) => $.contentRoiReview.report.noRecords)}</span>
            ) : (
              <ul className="space-y-1">
                {records.map((record, index) => {
                  const text = [
                    `${recordKindLabel(t, record.kind)} ${record.id}`,
                    t(($) => $.contentRoiReview.revisionNo, { revision: record.revision }),
                    record.amountMinor !== null
                      ? t(($) => $.contentRoiReview.report.recordAmount, {
                          amount: roiMinorText(record.amountMinor, record.currency),
                          currency: record.currency || "-",
                        })
                      : "",
                    record.convertedMinor !== null
                      ? t(($) => $.contentRoiReview.report.recordConverted, {
                          amount: roiMinorText(record.convertedMinor, reportCurrency),
                          currency: reportCurrency,
                        })
                      : record.amountMinor !== null
                        ? t(($) => $.contentRoiReview.report.recordNotConverted)
                        : "",
                  ]
                    .filter((part) => part !== "")
                    .join(" · ");
                  const linkable = record.kind === "cost" || record.kind === "lead" || record.kind === "deal";
                  return (
                    <li key={`${record.kind}-${record.id}-${index}`} className="text-body text-muted-foreground">
                      {linkable ? (
                        <button
                          type="button"
                          className="break-words text-left hover:text-foreground"
                          onClick={() =>
                            onFocus({ kind: record.kind as RoiFocus["kind"], id: record.id, revision: record.revision })
                          }
                        >
                          {text}
                        </button>
                      ) : (
                        <span className="break-words">{text}</span>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </SettingsRow>
      ) : null}
    </>
  );
}
