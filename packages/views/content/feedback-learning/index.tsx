"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  EXCERPT_SOURCES,
  METRIC_NAMES,
  METRIC_PLATFORMS,
  canSubmitExcerptDraft,
  canSubmitMetricDraft,
  csvPreview,
  describeMetricValue,
  emptyExcerptDraft,
  emptyMetricDraft,
  excerptDraftProblem,
  excerptDraftToInput,
  metricDraftProblem,
  metricDraftToInput,
  useContentFeedback,
  useContentMetrics,
  useImportContentMetrics,
  usePendingFeedback,
  useRecordContentFeedback,
  useRecordContentMetric,
  type DraftProblem,
  type FeedbackExcerpt,
  type ManualMetric,
  type PendingFeedback,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  SettingsCard,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
} from "@multica/views/settings/layout";

// Manual metrics and feedback excerpts (specs/027, SOP 10.1), sitting under the
// publication records they belong to - which is where the thing being measured
// is.
//
// Every rule lives in @multica/core/content/feedback-learning: form.ts decides
// what may be submitted, csv.ts reads a pasted block, pending.ts derives the
// workbench item. All three have node tests. This file is wiring: which hook,
// which string, which row.
//
// Three things this page has to keep saying out loud:
//
//   - A blank number is unknown, not zero. It renders as "未知", never as 0 and
//     never as an empty cell somebody will read as zero.
//   - Nothing here goes and looks at a platform. Every number was typed in.
//   - Redacting is the person's job. The system stores what it is given.

type Translate = ReturnType<typeof useT<"common">>["t"];

/** One publication record the numbers can hang on. Passed in by the adapter:
 *  the block is rendered inside review-delivery's own section, which already
 *  holds the list. */
export interface FeedbackPublicationOption {
  publicationRecordId: string;
  channel: string;
  status: string;
  publishedAt: string;
  createdAt: string;
}

export interface FeedbackLearningSectionsProps {
  wsId: string;
  records: FeedbackPublicationOption[];
}

export function FeedbackLearningSections({ wsId, records }: FeedbackLearningSectionsProps) {
  const { t } = useT("common");
  // The newest record is the default, but it is a choice: a piece that was
  // published, removed and published again has several, and a number belongs to
  // exactly one of them.
  const [recordId, setRecordId] = useState("");
  const selected = records.find((record) => record.publicationRecordId === recordId)
    ?? records[0]
    ?? null;
  const activeId = selected?.publicationRecordId ?? "";

  if (records.length === 0) {
    return (
      <SettingsSection
        title={t(($) => $.contentMetrics.sectionTitle)}
        description={t(($) => $.contentMetrics.sectionDescription)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentMetrics.noRecords)}
            description={t(($) => $.contentMetrics.noRecordsHint)}
          >
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>
    );
  }

  const recordItems = records.map((record) => ({
    value: record.publicationRecordId,
    label: recordLabel(t, record),
  }));

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentMetrics.sectionTitle)}
        // SOP 10.1: the numbers are copied in by a person. Nothing fetches them.
        description={t(($) => $.contentMetrics.sectionDescription)}
      >
        <SettingsCard>
          <SettingsRow
            label={t(($) => $.contentMetrics.recordLabel)}
            description={t(($) => $.contentMetrics.recordHint)}
            size="select-wide"
          >
            <Select
              items={recordItems}
              value={activeId}
              onValueChange={(value) => setRecordId(value ?? "")}
            >
              <SelectTrigger aria-label={t(($) => $.contentMetrics.recordLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {recordItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <MetricListSection wsId={wsId} publicationRecordId={activeId} />
      <MetricFormSection wsId={wsId} publicationRecordId={activeId} />
      <CsvPasteSection wsId={wsId} publicationRecordId={activeId} />
      <ExcerptSection wsId={wsId} publicationRecordId={activeId} />
      <PendingSection
        wsId={wsId}
        activeId={activeId}
        // Only the records this piece has can be picked here. A pending row
        // from another piece would otherwise fall back to this piece's newest
        // record and quietly point the form somewhere else.
        pickable={new Set(records.map((record) => record.publicationRecordId))}
        onPick={setRecordId}
      />
    </>
  );
}

// ------------------------------------------------------------------ metrics

function MetricListSection({
  wsId,
  publicationRecordId,
}: {
  wsId: string;
  publicationRecordId: string;
}) {
  const { t } = useT("common");
  const metrics = useContentMetrics(wsId, publicationRecordId);
  const list = metrics.data ?? [];

  return (
    <SettingsSection
      title={t(($) => $.contentMetrics.listTitle)}
      // 10.1: 阅读 and 播放 are kept apart. There is no total and no ranking
      // here for the same reason there is no endpoint for one.
      description={t(($) => $.contentMetrics.listHint)}
    >
      <SettingsCard>
        {metrics.isPending ? (
          <SettingsRow label={t(($) => $.contentMetrics.loading)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : metrics.isError ? (
          <SettingsRow label={t(($) => $.contentMetrics.loadFailed)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : list.length === 0 ? (
          <SettingsRow label={t(($) => $.contentMetrics.listEmpty)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : (
          list.map((metric) => <MetricRow key={metric.manualMetricId} metric={metric} />)
        )}
      </SettingsCard>
    </SettingsSection>
  );
}

function MetricRow({ metric }: { metric: ManualMetric }) {
  const { t } = useT("common");
  // The one rendering decision that matters on this page. An unknown value says
  // so; it is not a 0 and it is not a blank cell that the next person along
  // reads as 0.
  const described = describeMetricValue(metric.value);
  const shown = described.known
    ? `${described.text}${metric.unit ? ` ${metric.unit}` : ""}`
    : t(($) => $.contentMetrics.valueUnknown);

  const parts = [platformLabel(t, metric.platform)];
  if (metric.statWindow) parts.push(metric.statWindow);
  if (metric.sampledAt) parts.push(metric.sampledAt);
  parts.push(metricSourceLabel(t, metric.sourceType));
  parts.push(
    metric.versionId
      ? `${t(($) => $.contentMetrics.versionLabel)}: ${metric.versionId}`
      : t(($) => $.contentMetrics.versionUnknown),
  );

  return (
    <SettingsRow label={metricNameLabel(t, metric.metric)} description={parts.join(" · ")}>
      <span
        className={
          described.known
            ? "text-caption text-foreground"
            : // Muted, not a warning: an unknown is an ordinary answer, not a
              // fault the operator has to fix.
              "text-caption text-muted-foreground"
        }
      >
        {shown}
      </span>
    </SettingsRow>
  );
}

function MetricFormSection({
  wsId,
  publicationRecordId,
}: {
  wsId: string;
  publicationRecordId: string;
}) {
  const { t } = useT("common");
  const [draft, setDraft] = useState(() => emptyMetricDraft());
  const record = useRecordContentMetric(wsId);

  const full = { ...draft, publicationRecordId };
  const problem = metricDraftProblem(full);
  const ready = canSubmitMetricDraft(full);

  const platformItems = METRIC_PLATFORMS.map((value) => ({
    value,
    label: platformLabel(t, value),
  }));
  const metricItems = METRIC_NAMES.map((value) => ({
    value,
    label: metricNameLabel(t, value),
  }));

  const submit = () => {
    if (!ready) return;
    record.mutate(metricDraftToInput(full), {
      onSuccess: () => setDraft(emptyMetricDraft()),
    });
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMetrics.formTitle)}
      description={t(($) => $.contentMetrics.formHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentMetrics.platformLabel)} size="select-wide">
          <Select
            items={platformItems}
            value={draft.platform}
            onValueChange={(value) => setDraft({ ...draft, platform: value ?? "" })}
          >
            <SelectTrigger aria-label={t(($) => $.contentMetrics.platformLabel)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {platformItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>

        <SettingsRow label={t(($) => $.contentMetrics.accountLabel)} size="text">
          <Input
            value={draft.accountId}
            onChange={(event) => setDraft({ ...draft, accountId: event.target.value })}
            placeholder={t(($) => $.contentMetrics.accountPlaceholder)}
            aria-label={t(($) => $.contentMetrics.accountLabel)}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentMetrics.metricLabel)}
          // Eleven names and no "other": the SOP named these and stopped.
          description={t(($) => $.contentMetrics.metricHint)}
          size="select-wide"
        >
          <Select
            items={metricItems}
            value={draft.metric}
            onValueChange={(value) => setDraft({ ...draft, metric: value ?? "" })}
          >
            <SelectTrigger aria-label={t(($) => $.contentMetrics.metricLabel)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {metricItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>

        {/* The row this whole card exists to get right. The hint is not a
            nicety: without it, a blank box and a box holding 0 look like the
            same thing to whoever fills this in. */}
        <SettingsRow
          label={t(($) => $.contentMetrics.valueLabel)}
          description={t(($) => $.contentMetrics.valueHint)}
          size="text"
        >
          <Input
            value={draft.value}
            onChange={(event) => setDraft({ ...draft, value: event.target.value })}
            placeholder={t(($) => $.contentMetrics.valuePlaceholder)}
            aria-label={t(($) => $.contentMetrics.valueLabel)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentMetrics.valuePreviewLabel)}>
          <span className="text-caption text-muted-foreground">
            {draft.value.trim() === ""
              ? t(($) => $.contentMetrics.valueUnknown)
              : draft.value.trim()}
          </span>
        </SettingsRow>

        <SettingsRow label={t(($) => $.contentMetrics.unitLabel)} size="text">
          <Input
            value={draft.unit}
            onChange={(event) => setDraft({ ...draft, unit: event.target.value })}
            placeholder={t(($) => $.contentMetrics.unitPlaceholder)}
            aria-label={t(($) => $.contentMetrics.unitLabel)}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentMetrics.windowLabel)}
          // Free text on purpose: "发布后 14 天累计" cannot be written as a
          // start and an end, and forcing two timestamps would make people
          // invent boundaries they do not know.
          description={t(($) => $.contentMetrics.windowHint)}
          size="text"
        >
          <Input
            value={draft.statWindow}
            onChange={(event) => setDraft({ ...draft, statWindow: event.target.value })}
            placeholder={t(($) => $.contentMetrics.windowPlaceholder)}
            aria-label={t(($) => $.contentMetrics.windowLabel)}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentMetrics.sampledAtLabel)}
          description={t(($) => $.contentMetrics.sampledAtHint)}
          size="text"
        >
          <Input
            value={draft.sampledAt}
            onChange={(event) => setDraft({ ...draft, sampledAt: event.target.value })}
            placeholder={t(($) => $.contentMetrics.sampledAtPlaceholder)}
            aria-label={t(($) => $.contentMetrics.sampledAtLabel)}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentMetrics.evidenceLabel)}
          description={t(($) => $.contentMetrics.evidenceHint)}
          size="text"
          align="start"
        >
          <Textarea
            value={draft.evidenceNote}
            onChange={(event) => setDraft({ ...draft, evidenceNote: event.target.value })}
            placeholder={t(($) => $.contentMetrics.evidencePlaceholder)}
            aria-label={t(($) => $.contentMetrics.evidenceLabel)}
            rows={3}
          />
        </SettingsRow>

        <SettingsRow
          label={t(($) => $.contentMetrics.submitLabel)}
          description={problem ? problemLabel(t, problem) : t(($) => $.contentMetrics.submitHint)}
        >
          <div className="flex items-center gap-2">
            <ActionState pending={record.isPending} failed={record.isError} />
            <Button onClick={submit} disabled={!ready || record.isPending}>
              {t(($) => $.contentMetrics.submit)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------- csv paste

function CsvPasteSection({
  wsId,
  publicationRecordId,
}: {
  wsId: string;
  publicationRecordId: string;
}) {
  const { t } = useT("common");
  const [text, setText] = useState("");
  const importer = useImportContentMetrics(wsId);
  const preview = csvPreview(text);

  const submit = () => {
    if (!preview.canImport || !publicationRecordId) return;
    importer.mutate(
      { publicationRecordId, rows: preview.rows },
      { onSuccess: () => setText("") },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentMetrics.csvTitle)}
      // A paste, not an upload. File upload arrives with W-03 and saying so is
      // better than a disabled button nobody can explain.
      description={t(($) => $.contentMetrics.csvHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentMetrics.csvNoUpload)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentMetrics.csvColumnsLabel)}
          description={t(($) => $.contentMetrics.csvColumnsHint)}
        >
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentMetrics.csvTextLabel)}
          size="text"
          align="start"
        >
          <Textarea
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder={t(($) => $.contentMetrics.csvPlaceholder)}
            aria-label={t(($) => $.contentMetrics.csvTextLabel)}
            rows={6}
          />
        </SettingsRow>

        {/* The preview is where a person can still tell us we read their
            blanks wrong. Finding that out after forty rows landed is finding
            it out too late. */}
        <SettingsRow
          label={t(($) => $.contentMetrics.csvPreviewLabel)}
          description={
            preview.problem
              ? t(($) => $.contentMetrics.csvAllOrNothing)
              : preview.empty
                ? undefined
                : t(($) => $.contentMetrics.csvUnknownCount, { unknown: preview.unknown })
          }
        >
          <span className="text-caption text-muted-foreground">
            {preview.problem
              ? t(($) => $.contentMetrics.csvProblem, {
                  row: preview.problem.row,
                  column: csvColumnLabel(t, preview.problem.column),
                  reason: csvReasonLabel(t, preview.problem.reason),
                })
              : preview.empty
                ? t(($) => $.contentMetrics.csvNothingYet)
                : t(($) => $.contentMetrics.csvRowCount, { rows: preview.rows.length })}
          </span>
        </SettingsRow>

        {preview.rows.slice(0, 5).map((row, index) => (
          <SettingsRow
            key={`${row.metric}-${index}`}
            label={`${index + 1}. ${metricNameLabel(t, row.metric)}`}
            description={[platformLabel(t, row.platform), row.statWindow, row.sampledAt]
              .filter(Boolean)
              .join(" · ")}
          >
            <span
              className={
                row.value === null
                  ? "text-caption text-muted-foreground"
                  : "text-caption text-foreground"
              }
            >
              {row.value === null
                ? t(($) => $.contentMetrics.valueUnknown)
                : `${row.value}${row.unit ? ` ${row.unit}` : ""}`}
            </span>
          </SettingsRow>
        ))}

        <SettingsRow
          label={t(($) => $.contentMetrics.csvSubmitLabel)}
          description={t(($) => $.contentMetrics.csvSubmitHint)}
        >
          <div className="flex items-center gap-2">
            <ActionState pending={importer.isPending} failed={importer.isError} />
            <Button onClick={submit} disabled={!preview.canImport || importer.isPending}>
              {t(($) => $.contentMetrics.csvSubmit)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ----------------------------------------------------------------- excerpts

function ExcerptSection({
  wsId,
  publicationRecordId,
}: {
  wsId: string;
  publicationRecordId: string;
}) {
  const { t } = useT("common");
  const feedback = useContentFeedback(wsId, publicationRecordId);
  const [draft, setDraft] = useState(() => emptyExcerptDraft());
  const record = useRecordContentFeedback(wsId);

  const full = { ...draft, publicationRecordId };
  const problem = excerptDraftProblem(full);
  const ready = canSubmitExcerptDraft(full);
  const list = feedback.data?.excerpts ?? [];

  const sourceItems = EXCERPT_SOURCES.map((value) => ({
    value,
    label: excerptSourceLabel(t, value),
  }));

  const submit = () => {
    if (!ready) return;
    record.mutate(excerptDraftToInput(full), {
      onSuccess: () => setDraft(emptyExcerptDraft()),
    });
  };

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentFeedback.formTitle)}
        // R-045: the quote and the reading of it are two fields, because a
        // later reader has to be able to tell the evidence from the judgement.
        description={t(($) => $.contentFeedback.formHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentFeedback.sourceLabel)} size="select-wide">
            <Select
              items={sourceItems}
              value={draft.sourceType}
              onValueChange={(value) => setDraft({ ...draft, sourceType: value ?? "" })}
            >
              <SelectTrigger aria-label={t(($) => $.contentFeedback.sourceLabel)}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {sourceItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingsRow>

          {/* Said in the interface, not only in the spec: nothing redacts for
              the person typing. SOP 10.1 "支持脱敏摘录" supports them doing it. */}
          <SettingsRow
            label={t(($) => $.contentFeedback.excerptLabel)}
            description={t(($) => $.contentFeedback.redactYourself)}
            size="text"
            align="start"
          >
            <Textarea
              value={draft.redactedExcerpt}
              onChange={(event) => setDraft({ ...draft, redactedExcerpt: event.target.value })}
              placeholder={t(($) => $.contentFeedback.excerptPlaceholder)}
              aria-label={t(($) => $.contentFeedback.excerptLabel)}
              rows={3}
            />
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentFeedback.interpretationLabel)}
            description={t(($) => $.contentFeedback.interpretationHint)}
            size="text"
            align="start"
          >
            <Textarea
              value={draft.interpretation}
              onChange={(event) => setDraft({ ...draft, interpretation: event.target.value })}
              placeholder={t(($) => $.contentFeedback.interpretationPlaceholder)}
              aria-label={t(($) => $.contentFeedback.interpretationLabel)}
              rows={3}
            />
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentFeedback.tagsLabel)}
            description={t(($) => $.contentFeedback.tagsHint)}
            size="text"
          >
            <Input
              value={draft.tagInput}
              onChange={(event) => setDraft({ ...draft, tagInput: event.target.value })}
              placeholder={t(($) => $.contentFeedback.tagsPlaceholder)}
              aria-label={t(($) => $.contentFeedback.tagsLabel)}
            />
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentFeedback.occurredAtLabel)}
            description={t(($) => $.contentFeedback.occurredAtHint)}
            size="text"
          >
            <Input
              value={draft.occurredAt}
              onChange={(event) => setDraft({ ...draft, occurredAt: event.target.value })}
              placeholder={t(($) => $.contentFeedback.occurredAtPlaceholder)}
              aria-label={t(($) => $.contentFeedback.occurredAtLabel)}
            />
          </SettingsRow>

          <SettingsRow
            label={t(($) => $.contentFeedback.submitLabel)}
            description={
              problem ? problemLabel(t, problem) : t(($) => $.contentFeedback.submitHint)
            }
          >
            <div className="flex items-center gap-2">
              <ActionState pending={record.isPending} failed={record.isError} />
              <Button onClick={submit} disabled={!ready || record.isPending}>
                {t(($) => $.contentFeedback.submit)}
              </Button>
            </div>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection
        title={t(($) => $.contentFeedback.listTitle)}
        // No count, no sentiment, no grouping: 10.2 is where any of that would
        // belong, and it has not landed.
        description={t(($) => $.contentFeedback.listHint)}
      >
        <SettingsCard>
          {feedback.isPending ? (
            <SettingsRow label={t(($) => $.contentFeedback.loading)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : feedback.isError ? (
            <SettingsRow label={t(($) => $.contentFeedback.loadFailed)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : list.length === 0 ? (
            <SettingsRow label={t(($) => $.contentFeedback.listEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            list.map((excerpt) => <ExcerptRow key={excerpt.feedbackExcerptId} excerpt={excerpt} />)
          )}
        </SettingsCard>
      </SettingsSection>

      <ReviewPlaceholder state={feedback.data?.reviewState ?? "pending_data"} />
    </>
  );
}

function ExcerptRow({ excerpt }: { excerpt: FeedbackExcerpt }) {
  const { t } = useT("common");
  const parts = [excerptSourceLabel(t, excerpt.sourceType)];
  if (excerpt.occurredAt) parts.push(excerpt.occurredAt);
  if (excerpt.tags.length > 0) parts.push(excerpt.tags.join(t(($) => $.contentFeedback.tagSeparator)));
  return (
    <>
      <SettingsRow
        label={excerpt.redactedExcerpt || t(($) => $.contentFeedback.noExcerpt)}
        description={parts.join(" · ")}
      >
        <span className="text-caption text-muted-foreground">{excerpt.createdAt}</span>
      </SettingsRow>
      {/* Its own row, not appended to the quote. Joining them is exactly what
          R-045 forbids - after that nobody can tell which half is evidence. */}
      <SettingsRow
        label={t(($) => $.contentFeedback.interpretationLabel)}
        description={excerpt.interpretation || t(($) => $.contentFeedback.noInterpretation)}
      >
        <span className="text-caption text-muted-foreground" />
      </SettingsRow>
    </>
  );
}

// ------------------------------------------------------------------- ai ----

/**
 * SOP 7.1's AI review row, present and not running.
 *
 * Hiding it would say the product does not intend to do this, a blank would
 * read as unfinished, and a spinner as "any moment now". Same treatment 024 and
 * 025 gave their entry points. Nothing here fabricates a conclusion.
 */
function ReviewPlaceholder({ state }: { state: string }) {
  const { t } = useT("common");
  return (
    <SettingsSection title={t(($) => $.contentFeedback.aiTitle)}>
      <SettingsCard>
        <SettingsRow
          label={reviewStateLabel(t, state)}
          description={t(($) => $.contentFeedback.aiUnavailable)}
        >
          <Button variant="outline" disabled>
            {t(($) => $.contentFeedback.aiRun)}
          </Button>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// -------------------------------------------------------------- pending ----

function PendingSection({
  wsId,
  activeId,
  pickable,
  onPick,
}: {
  wsId: string;
  activeId: string;
  pickable: ReadonlySet<string>;
  onPick: (publicationRecordId: string) => void;
}) {
  const { t } = useT("common");
  const pending = usePendingFeedback(wsId);
  const list = pending.data ?? [];

  return (
    <SettingsSection
      title={t(($) => $.contentFeedback.pendingTitle)}
      // The whole rule, stated: it went out and nobody has recorded a number.
      // Not "overdue" - there is no observation time to be past.
      description={t(($) => $.contentFeedback.pendingHint)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentFeedback.pendingScope)}>
          <span className="text-caption text-muted-foreground" />
        </SettingsRow>
        {pending.isPending ? (
          <SettingsRow label={t(($) => $.contentFeedback.loading)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : pending.isError ? (
          <SettingsRow label={t(($) => $.contentFeedback.loadFailed)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : list.length === 0 ? (
          <SettingsRow label={t(($) => $.contentFeedback.pendingEmpty)}>
            <span className="text-caption text-muted-foreground" />
          </SettingsRow>
        ) : (
          list.map((item) => (
            <PendingRow
              key={item.publicationRecordId}
              item={item}
              active={item.publicationRecordId === activeId}
              pickable={pickable.has(item.publicationRecordId)}
              onPick={onPick}
            />
          ))
        )}
      </SettingsCard>
    </SettingsSection>
  );
}

function PendingRow({
  item,
  active,
  pickable,
  onPick,
}: {
  item: PendingFeedback;
  active: boolean;
  pickable: boolean;
  onPick: (publicationRecordId: string) => void;
}) {
  const { t } = useT("common");
  const parts = [channelLabel(t, item.channel)];
  if (item.publishedAt) parts.push(item.publishedAt);
  return (
    <SettingsRow
      // Weight, not colour: hovering a picked row must not visually downgrade
      // it to a plain hover.
      label={active ? t(($) => $.contentFeedback.pendingActive) : parts.join(" · ")}
      description={
        active
          ? parts.join(" · ")
          : pickable
            ? undefined
            : // Saying where it is beats a button that would land somewhere
              // else. The list is brand-wide; this block is one document's.
              t(($) => $.contentFeedback.pendingElsewhere)
      }
    >
      {pickable ? (
        <Button
          variant="outline"
          data-active={active}
          className="data-[active=true]:font-semibold"
          onClick={() => onPick(item.publicationRecordId)}
        >
          {t(($) => $.contentFeedback.pendingOpen)}
        </Button>
      ) : (
        <span className="text-caption text-muted-foreground" />
      )}
    </SettingsRow>
  );
}

// ------------------------------------------------------------------- shared

function ActionState({ pending, failed }: { pending: boolean; failed: boolean }) {
  const { t } = useT("common");
  return (
    <SettingsSaveState
      status={pending ? "saving" : failed ? "error" : "idle"}
      savingLabel={t(($) => $.contentMetrics.saving)}
      savedLabel={t(($) => $.contentMetrics.saved)}
      errorLabel={t(($) => $.contentMetrics.failed)}
    />
  );
}

function recordLabel(t: Translate, record: FeedbackPublicationOption): string {
  const parts = [channelLabel(t, record.channel), publicationStatusLabel(t, record.status)];
  if (record.publishedAt) parts.push(record.publishedAt);
  else if (record.createdAt) parts.push(record.createdAt);
  return parts.join(" · ");
}

function problemLabel(t: Translate, problem: DraftProblem): string {
  const field = fieldLabel(t, problem.field);
  switch (problem.reason) {
    case "missing":
      return `${field}: ${t(($) => $.contentMetrics.problems.missing)}`;
    case "outside-the-set":
      return `${field}: ${t(($) => $.contentMetrics.problems.outsideTheSet)}`;
    case "not-a-number":
      return `${field}: ${t(($) => $.contentMetrics.problems.notANumber)}`;
    case "not-a-timestamp":
      return `${field}: ${t(($) => $.contentMetrics.problems.notATimestamp)}`;
    case "too-long":
      return `${field}: ${t(($) => $.contentMetrics.problems.tooLong)}`;
    case "too-many":
      return `${field}: ${t(($) => $.contentMetrics.problems.tooMany)}`;
    default:
      // A reason a newer core added. Naming the field beats saying nothing.
      return field;
  }
}

function fieldLabel(t: Translate, field: string): string {
  switch (field) {
    case "publication_record_id":
      return t(($) => $.contentMetrics.recordLabel);
    case "platform":
      return t(($) => $.contentMetrics.platformLabel);
    case "account_id":
      return t(($) => $.contentMetrics.accountLabel);
    case "metric":
      return t(($) => $.contentMetrics.metricLabel);
    case "value":
      return t(($) => $.contentMetrics.valueLabel);
    case "unit":
      return t(($) => $.contentMetrics.unitLabel);
    case "stat_window":
      return t(($) => $.contentMetrics.windowLabel);
    case "sampled_at":
      return t(($) => $.contentMetrics.sampledAtLabel);
    case "evidence_note":
      return t(($) => $.contentMetrics.evidenceLabel);
    case "source_type":
      return t(($) => $.contentFeedback.sourceLabel);
    case "redacted_excerpt":
      return t(($) => $.contentFeedback.excerptLabel);
    case "interpretation":
      return t(($) => $.contentFeedback.interpretationLabel);
    case "tags":
      return t(($) => $.contentFeedback.tagsLabel);
    case "occurred_at":
      return t(($) => $.contentFeedback.occurredAtLabel);
    default:
      return field;
  }
}

function csvColumnLabel(t: Translate, column: string): string {
  return fieldLabel(t, column);
}

function csvReasonLabel(t: Translate, reason: string): string {
  switch (reason) {
    case "missing":
      return t(($) => $.contentMetrics.problems.missing);
    case "not-a-number":
      return t(($) => $.contentMetrics.problems.notANumber);
    case "outside-the-set":
      return t(($) => $.contentMetrics.problems.outsideTheSet);
    case "unknown-column":
      return t(($) => $.contentMetrics.problems.unknownColumn);
    default:
      return reason;
  }
}

// A value this build has not heard of still has to render as something. The raw
// value is a worse label than a translated one and a better one than a blank.

function platformLabel(t: Translate, platform: string): string {
  return channelLabel(t, platform);
}

function channelLabel(t: Translate, channel: string): string {
  switch (channel) {
    case "xiaohongshu":
      return t(($) => $.contentMetrics.platforms.xiaohongshu);
    case "wechat_mp":
      return t(($) => $.contentMetrics.platforms.wechat_mp);
    case "douyin":
      return t(($) => $.contentMetrics.platforms.douyin);
    case "shipinhao":
      return t(($) => $.contentMetrics.platforms.shipinhao);
    default:
      return channel;
  }
}

function metricNameLabel(t: Translate, metric: string): string {
  switch (metric) {
    case "impression":
      return t(($) => $.contentMetrics.names.impression);
    // 阅读 and 播放 have their own labels and stay that way. 10.1:
    // "不同平台的阅读和播放分别保留，不直接合并排名".
    case "read":
      return t(($) => $.contentMetrics.names.read);
    case "play":
      return t(($) => $.contentMetrics.names.play);
    case "completion":
      return t(($) => $.contentMetrics.names.completion);
    case "like":
      return t(($) => $.contentMetrics.names.like);
    case "comment":
      return t(($) => $.contentMetrics.names.comment);
    case "favorite":
      return t(($) => $.contentMetrics.names.favorite);
    case "share":
      return t(($) => $.contentMetrics.names.share);
    case "follow":
      return t(($) => $.contentMetrics.names.follow);
    case "direct_message":
      return t(($) => $.contentMetrics.names.direct_message);
    case "conversion":
      return t(($) => $.contentMetrics.names.conversion);
    default:
      return metric;
  }
}

function metricSourceLabel(t: Translate, source: string): string {
  switch (source) {
    case "manual":
      return t(($) => $.contentMetrics.sources.manual);
    case "csv_import":
      return t(($) => $.contentMetrics.sources.csv_import);
    default:
      return source;
  }
}

function excerptSourceLabel(t: Translate, source: string): string {
  switch (source) {
    case "comment":
      return t(($) => $.contentFeedback.sources.comment);
    case "private_message":
      return t(($) => $.contentFeedback.sources.private_message);
    case "lead":
      return t(($) => $.contentFeedback.sources.lead);
    default:
      return source;
  }
}

function reviewStateLabel(t: Translate, state: string): string {
  switch (state) {
    case "pending_data":
      return t(($) => $.contentFeedback.reviewStates.pending_data);
    case "queued":
      return t(($) => $.contentFeedback.reviewStates.queued);
    case "generating":
      return t(($) => $.contentFeedback.reviewStates.generating);
    case "generated":
      return t(($) => $.contentFeedback.reviewStates.generated);
    case "failed":
      return t(($) => $.contentFeedback.reviewStates.failed);
    case "edited":
      return t(($) => $.contentFeedback.reviewStates.edited);
    case "superseded":
      return t(($) => $.contentFeedback.reviewStates.superseded);
    default:
      return state;
  }
}

function publicationStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "reported_published":
      return t(($) => $.contentMetrics.publicationStatuses.reported_published);
    case "verified_published":
      return t(($) => $.contentMetrics.publicationStatuses.verified_published);
    case "failed":
      return t(($) => $.contentMetrics.publicationStatuses.failed);
    case "removed":
      return t(($) => $.contentMetrics.publicationStatuses.removed);
    case "unknown":
      return t(($) => $.contentMetrics.publicationStatuses.unknown);
    default:
      return status;
  }
}
