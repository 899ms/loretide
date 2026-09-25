"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_IMPORT_COLUMNS,
  ROI_IMPORT_KINDS,
  parsePastedRoiRows,
  roiImportAttempt,
  roiWriteOutcome,
  toRoiImportBody,
  useImportRoiRows,
  useRoiImport,
  useRoiImports,
  type RoiCsvProblem,
  type RoiImportAttempt,
  type RoiImportKind,
  type RoiImportResult,
  type RoiImportRow,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";
import {
  ChoiceSelect,
  ReadOnlyList,
  SaveFeedback,
  StateRow,
  formatTime,
  joinList,
  recordKindLabel,
  type Translate,
} from "./shared";

// The import block (specs/034 PR 5, T090; U-17 to U-20).
//
// A paste is read on the page (csv.ts): the first bad row stops it, naming
// the row and the column, and nothing is sent. A good paste is previewed with
// dry_run - the server says which rows look like duplicates, and nothing is
// stored. Rows that look like duplicates are held back unless the person
// says, row by row, that they are not. The import itself carries one
// Idempotency-Key per action, so pressing it twice writes once.

const IMPORT_COLUMN_NAMES = [
  "category", "pricing", "amount", "currency", "labor_minutes", "labor_rate", "incurred_at", "ad_spend",
  "account_id", "work_id", "campaign_label", "evidence_note", "note", "customer_ref", "stage", "qualified",
  "first_seen_at", "lead_id", "order_ref", "closed_at", "gross_basis", "gross_profit", "cogs",
] as const;

export function columnLabel(t: Translate, column: string): string {
  return (IMPORT_COLUMN_NAMES as readonly string[]).includes(column)
    ? t(($) => $.contentRoiReview.columns[column as (typeof IMPORT_COLUMN_NAMES)[number]])
    : column;
}

function csvProblemLabel(t: Translate, problem: RoiCsvProblem): string {
  const reason = t(($) => $.contentRoiReview.imports.csvReasons[problem.reason]);
  if (problem.row === 0) {
    return t(($) => $.contentRoiReview.imports.headerProblem, { column: problem.column, reason });
  }
  return t(($) => $.contentRoiReview.imports.rowProblem, {
    row: problem.row,
    column: columnLabel(t, problem.column),
    reason,
  });
}

function outcomeText(t: Translate, row: RoiImportRow, dryRun: boolean): string {
  switch (row.outcome) {
    case "written":
      return dryRun
        ? t(($) => $.contentRoiReview.imports.outcomes.willWrite)
        : t(($) => $.contentRoiReview.imports.outcomes.written, { id: row.recordId });
    case "duplicate":
      return t(($) => $.contentRoiReview.imports.outcomes.duplicate, {
        ids: joinList(t, [...row.duplicateOf, ...row.duplicateOfRows.map((line) => t(($) => $.contentRoiReview.imports.rowRef, { row: line }))]),
      });
    case "confirmed_not_duplicate":
      return dryRun
        ? t(($) => $.contentRoiReview.imports.outcomes.willWriteConfirmed)
        : t(($) => $.contentRoiReview.imports.outcomes.confirmed, { id: row.recordId });
    default:
      return t(($) => $.contentRoiReview.imports.outcomes.unknown);
  }
}

function newKey(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `roi-import-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export function ImportsBlock({ wsId }: { wsId: string }) {
  const { t } = useT("common");
  const importRows = useImportRoiRows(wsId);
  const [kind, setKind] = useState<RoiImportKind>("cost");
  const [text, setText] = useState("");
  const [preview, setPreview] = useState<{ text: string; kind: RoiImportKind; result: RoiImportResult } | null>(null);
  const [confirmations, setConfirmations] = useState<Record<number, string[]>>({});
  const [attempt, setAttempt] = useState<RoiImportAttempt | null>(null);
  const [outcome, setOutcome] = useState<RoiWriteOutcome | null>(null);
  const [done, setDone] = useState<RoiImportResult | null>(null);
  const [openBatch, setOpenBatch] = useState("");

  const parsed = parsePastedRoiRows(kind, text);
  const localProblem = parsed.ok ? null : parsed.problem;
  const previewCurrent = preview !== null && preview.text === text && preview.kind === kind;
  const kindItems = ROI_IMPORT_KINDS.map((value) => ({ value, label: recordKindLabel(t, value) }));

  const reset = () => {
    setPreview(null);
    setConfirmations({});
    setOutcome(null);
    setDone(null);
  };

  const runPreview = () => {
    if (!parsed.ok || parsed.rows.length === 0) return;
    setOutcome(null);
    setDone(null);
    importRows.mutate(
      { body: toRoiImportBody(kind, parsed.rows, { dryRun: true, confirmations }) },
      {
        onSuccess: (result) => {
          if (result) setPreview({ text, kind, result });
          else setOutcome({ kind: "failed", nextAction: "", traceId: "" });
        },
        onError: (error) => setOutcome(roiWriteOutcome(error)),
      },
    );
  };

  const runImport = () => {
    if (!parsed.ok || parsed.rows.length === 0 || !previewCurrent) return;
    const body = toRoiImportBody(kind, parsed.rows, { confirmations });
    const next = roiImportAttempt(attempt, body, newKey);
    setAttempt(next);
    setOutcome(null);
    importRows.mutate(
      { body, idempotencyKey: next.key },
      {
        onSuccess: (result) => {
          setDone(result);
          setOutcome(result ? { kind: "saved" } : { kind: "failed", nextAction: "", traceId: "" });
          if (result) setOpenBatch(result.importBatchId);
        },
        onError: (error) => setOutcome(roiWriteOutcome(error)),
      },
    );
  };

  const toggleConfirm = (row: RoiImportRow) => {
    const next = { ...confirmations };
    if (next[row.row]) delete next[row.row];
    else next[row.row] = row.duplicateOf;
    setConfirmations(next);
  };

  const previewRows = previewCurrent ? preview.result.rows : [];

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentRoiReview.imports.title)}
        description={t(($) => $.contentRoiReview.imports.description)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentRoiReview.imports.kind)} size="select">
            <ChoiceSelect
              items={kindItems}
              value={kind}
              onChange={(value) => {
                setKind(value as RoiImportKind);
                reset();
              }}
              label={t(($) => $.contentRoiReview.imports.kind)}
              disabled={importRows.isPending}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentRoiReview.imports.paste)}
            description={t(($) => $.contentRoiReview.imports.columnsHint, {
              columns: ROI_IMPORT_COLUMNS[kind].join(","),
            })}
            size="text"
            align="start"
          >
            <Textarea
              value={text}
              onChange={(event) => {
                setText(event.target.value);
                setConfirmations({});
                setDone(null);
              }}
              aria-label={t(($) => $.contentRoiReview.imports.paste)}
              rows={6}
              disabled={importRows.isPending}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentRoiReview.imports.preview)}
            description={
              localProblem
                ? csvProblemLabel(t, localProblem)
                : parsed.ok && parsed.rows.length === 0
                  ? t(($) => $.contentRoiReview.imports.empty)
                  : t(($) => $.contentRoiReview.imports.previewHint)
            }
          >
            <Button
              variant="outline"
              onClick={runPreview}
              disabled={importRows.isPending || !parsed.ok || parsed.rows.length === 0}
            >
              {t(($) => $.contentRoiReview.imports.preview)}
            </Button>
          </SettingsRow>
          {previewCurrent ? (
            <>
              {previewRows.map((row) => (
                <SettingsRow key={row.row} label={t(($) => $.contentRoiReview.imports.rowRef, { row: row.row })} description={outcomeText(t, row, true)}>
                  {row.outcome === "duplicate" || confirmations[row.row] ? (
                    <Button
                      variant={confirmations[row.row] ? "brand" : "outline"}
                      size="sm"
                      onClick={() => toggleConfirm(row)}
                      disabled={importRows.isPending || row.duplicateOf.length === 0}
                    >
                      {t(($) => $.contentRoiReview.notDuplicate)}
                    </Button>
                  ) : (
                    <span className="text-caption text-muted-foreground" />
                  )}
                </SettingsRow>
              ))}
              <SettingsRow
                label={t(($) => $.contentRoiReview.imports.submit)}
                description={t(($) => $.contentRoiReview.imports.submitHint)}
              >
                <div className="flex items-center gap-3">
                  <SaveFeedback outcome={outcome} pending={importRows.isPending} />
                  <Button onClick={runImport} disabled={importRows.isPending || !parsed.ok}>
                    {t(($) => $.contentRoiReview.imports.submit)}
                  </Button>
                </div>
              </SettingsRow>
            </>
          ) : outcome ? (
            <SettingsRow label={t(($) => $.contentRoiReview.imports.preview)}>
              <SaveFeedback outcome={outcome} pending={importRows.isPending} />
            </SettingsRow>
          ) : null}
          {done ? (
            <SettingsRow label={t(($) => $.contentRoiReview.imports.doneTitle)} size="text" align="start">
              <ReadOnlyList
                items={[
                  t(($) => $.contentRoiReview.imports.batchCounts, {
                    rows: done.rowCount,
                    written: done.writtenCount,
                    skipped: done.skippedCount,
                  }),
                  ...done.rows.map((row) =>
                    `${t(($) => $.contentRoiReview.imports.rowRef, { row: row.row })} · ${outcomeText(t, row, false)}`,
                  ),
                ]}
                empty=""
              />
            </SettingsRow>
          ) : null}
        </SettingsCard>
      </SettingsSection>

      <BatchList wsId={wsId} openBatch={openBatch} onOpen={setOpenBatch} />
    </>
  );
}

function BatchList({ wsId, openBatch, onOpen }: { wsId: string; openBatch: string; onOpen: (id: string) => void }) {
  const { t } = useT("common");
  const batches = useRoiImports(wsId);
  const list = batches.data ?? [];
  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.imports.batchesTitle)}>
        <SettingsCard>
          {batches.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : batches.isError ? (
            <StateRow label={t(($) => $.contentRoiReview.loadFailed)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.imports.batchesEmpty)} />
          ) : (
            list.map((batch) => (
              <SettingsRow
                key={batch.importBatchId}
                label={
                  <button
                    type="button"
                    onClick={() => onOpen(batch.importBatchId)}
                    data-active={batch.importBatchId === openBatch}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {recordKindLabel(t, batch.recordKind)} · {formatTime(batch.createdAt)}
                  </button>
                }
                description={[
                  t(($) => $.contentRoiReview.imports.batchCounts, {
                    rows: batch.rowCount,
                    written: batch.writtenCount,
                    skipped: batch.skippedCount,
                  }),
                  t(($) => $.contentRoiReview.imports.batchBy, { who: batch.recordedBy }),
                ].join(" · ")}
                size="select-wide"
              >
                <span className="text-caption text-muted-foreground">{batch.importBatchId}</span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>
      {openBatch !== "" ? <BatchDetail key={openBatch} wsId={wsId} batchId={openBatch} /> : null}
    </>
  );
}

function BatchDetail({ wsId, batchId }: { wsId: string; batchId: string }) {
  const { t } = useT("common");
  const batch = useRoiImport(wsId, batchId);
  const data = batch.data;
  return (
    <SettingsSection title={t(($) => $.contentRoiReview.imports.batchDetailTitle)}>
      <SettingsCard>
        {batch.isLoading ? (
          <StateRow label={t(($) => $.contentRoiReview.loading)} />
        ) : !data ? (
          <StateRow label={t(($) => $.contentRoiReview.notFound)} />
        ) : (
          data.rows.map((row) => (
            <SettingsRow
              key={row.row}
              label={t(($) => $.contentRoiReview.imports.rowRef, { row: row.row })}
              description={outcomeText(t, row, false)}
            >
              <span className="text-caption text-muted-foreground">{row.recordId}</span>
            </SettingsRow>
          ))
        )}
      </SettingsCard>
    </SettingsSection>
  );
}
