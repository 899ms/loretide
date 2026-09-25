"use client";

import type { ReactNode } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_CURRENCIES,
  roiCurrencyDigits,
  type RoiAmountProblem,
  type RoiReasonKey,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  SettingsRow,
  SettingsSaveState,
  type SettingsSaveStatus,
} from "@multica/views/settings/layout";

// Pieces every block of the ROI review page shares (specs/034 PR 5).
//
// Nothing here computes a figure. Amounts, ratios and "not computable" come
// from the server as strings; which label goes with them is chosen in
// @multica/core/content/feedback-learning (roi/display.ts, node-tested), and
// this file only turns those choices into text and existing components.

export type Translate = ReturnType<typeof useT<"common">>["t"];

/** An account or a work of this brand, supplied by the Web adapter:
 *  feedback-learning does not depend on ip-profile or work-editor. */
export interface RoiOption {
  id: string;
  name: string;
}

export interface RoiOptions {
  accounts: RoiOption[];
  accountsLoading?: boolean;
  accountsFailed?: boolean;
  works: RoiOption[];
  worksLoading?: boolean;
  worksFailed?: boolean;
}

/** A record the report points at: open its block, its detail, that revision. */
export interface RoiFocus {
  kind: "cost" | "lead" | "deal";
  id: string;
  revision: number | null;
}

export function optionName(options: RoiOption[], id: string): string {
  if (id === "") return "";
  return options.find((option) => option.id === id)?.name || id;
}

export function formatTime(value: string): string {
  if (value === "") return "";
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString();
}

/** A datetime-local value to the RFC 3339 instant the server takes, in this
 *  browser's time zone. "" stays "" (the server names the missing field). */
export function toInstant(local: string): string {
  if (local === "") return "";
  const time = new Date(local);
  return Number.isNaN(time.getTime()) ? "" : time.toISOString();
}

/** A stored instant back into a datetime-local value, for editing. */
export function toLocalInput(instant: string): string {
  const time = new Date(instant);
  if (instant === "" || Number.isNaN(time.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${time.getFullYear()}-${pad(time.getMonth() + 1)}-${pad(time.getDate())}T${pad(time.getHours())}:${pad(time.getMinutes())}`;
}

export function nowLocalInput(): string {
  return toLocalInput(new Date().toISOString());
}

// ---------------------------------------------------------------------------
// Labels for the controlled sets. A value this build does not know renders
// as its raw text.

export function reasonLabel(t: Translate, reason: RoiReasonKey): string {
  return t(($) => $.contentRoiReview.reasons[reason]);
}

/** "不可计算：<reason>" - one string, so it is never split into a blank. */
export function notComputableLabel(t: Translate, reason: RoiReasonKey): string {
  return t(($) => $.contentRoiReview.notComputable, { reason: reasonLabel(t, reason) });
}

export function amountProblemLabel(t: Translate, problem: RoiAmountProblem, currency: string): string {
  const digits = roiCurrencyDigits(currency) ?? 0;
  return t(($) => $.contentRoiReview.amountProblems[problem], { currency, digits });
}

function known<T extends string>(values: readonly T[], value: string): value is T {
  return (values as readonly string[]).includes(value);
}

const EVIDENCE = [
  "platform_linked_content", "content_comment", "customer_statement",
  "dedicated_channel", "account_only", "unknown",
] as const;
export function evidenceLabel(t: Translate, value: string): string {
  return known(EVIDENCE, value) ? t(($) => $.contentRoiReview.evidence[value]) : value;
}

const ROLES = ["first_touch", "pre_booking", "other"] as const;
export function roleLabel(t: Translate, value: string): string {
  return known(ROLES, value) ? t(($) => $.contentRoiReview.roles[value]) : value;
}

const GROSS_BASES = ["none", "stated_gross_profit", "cogs"] as const;
export function grossBasisLabel(t: Translate, value: string): string {
  return known(GROSS_BASES, value) ? t(($) => $.contentRoiReview.grossBases[value]) : value;
}

const JUDGEMENTS = ["confirmed", "operator_judgement", "multi_touch", "unknown"] as const;
export function judgementLabel(t: Translate, value: string): string {
  return known(JUDGEMENTS, value) ? t(($) => $.contentRoiReview.judgements[value]) : value;
}

const METHODS = ["first_touch", "last_touch", "even_split", "judgement_weights"] as const;
export function methodLabel(t: Translate, value: string): string {
  return known(METHODS, value) ? t(($) => $.contentRoiReview.methods[value]) : value;
}

const PRICINGS = ["amount", "labor_time"] as const;
export function pricingLabel(t: Translate, value: string): string {
  return known(PRICINGS, value) ? t(($) => $.contentRoiReview.pricings[value]) : value;
}

const ADJUSTMENT_KINDS = ["refund", "adjustment"] as const;
export function adjustmentKindLabel(t: Translate, value: string): string {
  return known(ADJUSTMENT_KINDS, value) ? t(($) => $.contentRoiReview.adjustmentKinds[value]) : value;
}

const SOURCES = ["manual", "import"] as const;
export function sourceLabel(t: Translate, value: string): string {
  return known(SOURCES, value) ? t(($) => $.contentRoiReview.sources[value]) : value;
}

const TARGET_KINDS = ["work", "account", "campaign_label", "period"] as const;
export function targetKindLabel(t: Translate, value: string): string {
  return known(TARGET_KINDS, value) ? t(($) => $.contentRoiReview.targetKinds[value]) : value;
}

const PLATFORMS = [
  "xiaohongshu", "douyin", "wechat_mp", "bilibili", "zhihu", "weibo", "kuaishou", "shipinhao",
] as const;
export function platformLabel(t: Translate, value: string): string {
  if (value === "") return t(($) => $.contentRoiReview.blank);
  return known(PLATFORMS, value) ? t(($) => $.contentRoiReview.platforms[value]) : value;
}

const RECORD_KINDS = ["cost", "lead", "touch", "deal", "adjustment", "attribution"] as const;
export function recordKindLabel(t: Translate, value: string): string {
  return known(RECORD_KINDS, value) ? t(($) => $.contentRoiReview.recordKinds[value]) : value;
}

export function labeled(t: Translate, label: string, value: string): string {
  return t(($) => $.contentRoiReview.labeled, { label, value });
}

export function joinList(t: Translate, items: string[]): string {
  return items.join(t(($) => $.contentRoiReview.listSeparator));
}

// ---------------------------------------------------------------------------
// Save feedback.

export function outcomeLabel(t: Translate, outcome: RoiWriteOutcome): string {
  switch (outcome.kind) {
    case "saved":
      return t(($) => $.contentRoiReview.saved);
    case "stale":
      return t(($) => $.contentRoiReview.stale);
    case "duplicate":
      return t(($) => $.contentRoiReview.duplicate, { ids: joinList(t, outcome.matches) });
    case "idempotency":
      return t(($) => $.contentRoiReview.idempotency);
    case "invalid":
      return outcome.row
        ? t(($) => $.contentRoiReview.invalidRow, { row: outcome.row, field: outcome.field, reason: outcome.reason })
        : t(($) => $.contentRoiReview.invalidField, { field: outcome.field, reason: outcome.reason });
    case "not_found":
      return t(($) => $.contentRoiReview.notFound);
    default:
      return outcome.nextAction
        ? `${t(($) => $.contentRoiReview.failed)} · ${t(($) => $.contentRoiReview.nextAction, { action: outcome.nextAction })}`
        : t(($) => $.contentRoiReview.failed);
  }
}

function saveStatusOf(outcome: RoiWriteOutcome | null, pending: boolean): SettingsSaveStatus {
  if (pending) return "saving";
  if (!outcome) return "idle";
  return outcome.kind === "saved" ? "saved" : "error";
}

export function SaveFeedback({ outcome, pending }: { outcome: RoiWriteOutcome | null; pending: boolean }) {
  const { t } = useT("common");
  return (
    <SettingsSaveState
      status={saveStatusOf(outcome, pending)}
      savingLabel={t(($) => $.contentRoiReview.saving)}
      savedLabel={t(($) => $.contentRoiReview.saved)}
      errorLabel={outcome && outcome.kind !== "saved" ? outcomeLabel(t, outcome) : t(($) => $.contentRoiReview.failed)}
    />
  );
}

/** A possible-duplicate refusal: the matching ids, and the one choice the
 *  person has - say it is not a duplicate - before saving again. */
export function DuplicateRow({
  matches,
  confirmed,
  onConfirm,
  disabled,
}: {
  matches: string[];
  confirmed: boolean;
  onConfirm: (confirmed: boolean) => void;
  disabled?: boolean;
}) {
  const { t } = useT("common");
  return (
    <SettingsRow
      label={t(($) => $.contentRoiReview.duplicate, { ids: joinList(t, matches) })}
      description={t(($) => $.contentRoiReview.duplicateHint)}
    >
      <Button variant={confirmed ? "brand" : "outline"} size="sm" onClick={() => onConfirm(!confirmed)} disabled={disabled}>
        {t(($) => $.contentRoiReview.notDuplicate)}
      </Button>
    </SettingsRow>
  );
}

// ---------------------------------------------------------------------------
// Read-only pieces.

/** A value shown back read-only; an empty one says "not filled in", never blank. */
export function ReadOnlyValue({ value, muted }: { value: string; muted?: boolean }) {
  const { t } = useT("common");
  const empty = value.trim() === "";
  return (
    <span
      className={
        empty || muted
          ? "block whitespace-pre-wrap break-words text-body text-muted-foreground"
          : "block whitespace-pre-wrap break-words text-body"
      }
    >
      {empty ? t(($) => $.contentRoiReview.blank) : value}
    </span>
  );
}

export function ReadOnlyList({ items, empty }: { items: ReactNode[]; empty: string }) {
  if (items.length === 0) {
    return <span className="text-body text-muted-foreground">{empty}</span>;
  }
  return (
    <ul className="space-y-1">
      {items.map((item, index) => (
        <li key={index} className="whitespace-pre-wrap break-words text-body text-muted-foreground">
          {item}
        </li>
      ))}
    </ul>
  );
}

export function StateRow({ label }: { label: string }) {
  return (
    <SettingsRow label={label}>
      <span className="text-caption text-muted-foreground" />
    </SettingsRow>
  );
}

// ---------------------------------------------------------------------------
// Controls.

export function ChoiceSelect({
  items,
  value,
  onChange,
  label,
  disabled,
}: {
  items: { value: string; label: string }[];
  value: string;
  onChange: (value: string) => void;
  label: string;
  disabled?: boolean;
}) {
  return (
    <Select items={items} value={value} onValueChange={(next) => onChange(next ?? value)} disabled={disabled}>
      <SelectTrigger aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function CurrencySelect({
  value,
  onChange,
  label,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  disabled?: boolean;
}) {
  const items = ROI_CURRENCIES.map((currency) => ({ value: currency.code, label: currency.code }));
  return <ChoiceSelect items={items} value={value} onChange={onChange} label={label} disabled={disabled} />;
}

// A select item may not be "", so "not linked" has a value no id can take.
const NOT_LINKED = "__not_linked__";

/** An account or work picker. "" is "not linked", a real answer. An id the
 *  list does not have (another page's, or not loaded) stays selectable. */
export function OptionSelect({
  options,
  value,
  onChange,
  label,
  loading,
  failed,
  disabled,
}: {
  options: RoiOption[];
  value: string;
  onChange: (value: string) => void;
  label: string;
  loading?: boolean;
  failed?: boolean;
  disabled?: boolean;
}) {
  const { t } = useT("common");
  const noneLabel = loading
    ? t(($) => $.contentRoiReview.loading)
    : failed
      ? t(($) => $.contentRoiReview.optionsFailed)
      : t(($) => $.contentRoiReview.notLinked);
  const listed = options.map((option) => ({ value: option.id, label: option.name || option.id }));
  const items = [
    { value: NOT_LINKED, label: noneLabel },
    ...(value !== "" && !options.some((option) => option.id === value) ? [{ value, label: value }] : []),
    ...listed,
  ];
  return (
    <ChoiceSelect
      items={items}
      value={value === "" ? NOT_LINKED : value}
      onChange={(next) => onChange(next === NOT_LINKED ? "" : next)}
      label={label}
      disabled={disabled}
    />
  );
}
