"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_ALLOCATION_METHODS,
  ROI_ALLOCATION_TARGETS,
  ROI_PRICINGS,
  ROI_SAVED,
  roiAmountProblem,
  roiCheckShares,
  roiCheckStoredShares,
  roiPath,
  roiReasonKey,
  roiWriteOutcome,
  useRoiCost,
  useRoiCosts,
  useWriteRoiCost,
  type RoiCost,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";
import {
  ChoiceSelect,
  CurrencySelect,
  DuplicateRow,
  OptionSelect,
  ReadOnlyList,
  ReadOnlyValue,
  SaveFeedback,
  StateRow,
  amountProblemLabel,
  formatTime,
  joinList,
  labeled,
  notComputableLabel,
  nowLocalInput,
  optionName,
  pricingLabel,
  sourceLabel,
  targetKindLabel,
  toInstant,
  toLocalInput,
  type RoiFocus,
  type RoiOptions,
  type Translate,
} from "./shared";

// The cost block (specs/034 PR 5, T087; U-02 to U-07).
//
// A cost is revised, never edited in place and never deleted: saving a change
// writes the next revision with base_revision, and "void" is a revision too.
// Amounts are typed and sent as strings; a labor cost without a rate is saved
// and shown as "not computable", never as 0.

interface ShareDraft {
  targetKind: string;
  targetId: string;
  weight: string;
  amount: string;
}

interface CostDraft {
  category: string;
  pricing: string;
  amount: string;
  currency: string;
  laborMinutes: string;
  laborRate: string;
  incurredAt: string;
  adSpend: boolean;
  accountId: string;
  workId: string;
  campaignLabel: string;
  evidenceNote: string;
  note: string;
  /** "none": not shared. */
  shareMethod: string;
  shares: ShareDraft[];
}

const NOT_SHARED = "none";

function emptyCostDraft(): CostDraft {
  return {
    category: "",
    pricing: "amount",
    amount: "",
    currency: "CNY",
    laborMinutes: "",
    laborRate: "",
    incurredAt: nowLocalInput(),
    adSpend: false,
    accountId: "",
    workId: "",
    campaignLabel: "",
    evidenceNote: "",
    note: "",
    shareMethod: NOT_SHARED,
    shares: [],
  };
}

function draftFromCost(cost: RoiCost): CostDraft {
  return {
    category: cost.category,
    pricing: cost.pricing,
    amount: cost.pricing === "amount" ? (cost.amount ?? "") : "",
    currency: cost.currency,
    laborMinutes: cost.laborMinutes === null ? "" : String(cost.laborMinutes),
    laborRate: cost.laborRate ?? "",
    incurredAt: toLocalInput(cost.incurredAt),
    adSpend: cost.adSpend,
    accountId: cost.accountId,
    workId: cost.workId,
    campaignLabel: cost.campaignLabel,
    evidenceNote: cost.evidenceNote,
    note: "",
    shareMethod: cost.allocations[0]?.method || NOT_SHARED,
    shares: cost.allocations.map((share) => ({
      targetKind: share.targetKind,
      targetId: share.targetId,
      weight: share.weight === null ? "" : String(share.weight),
      amount: share.allocated,
    })),
  };
}

type CostProblem =
  | { kind: "field"; field: "category" | "incurred_at" | "labor_minutes" | "allocations" }
  | { kind: "amount"; field: "amount" | "labor_rate"; problem: NonNullable<ReturnType<typeof roiAmountProblem>> }
  | { kind: "shares_gap"; total: string; original: string; gap: string };

function costProblems(draft: CostDraft): CostProblem[] {
  const problems: CostProblem[] = [];
  if (draft.category.trim() === "") problems.push({ kind: "field", field: "category" });
  if (draft.pricing === "amount") {
    const problem = roiAmountProblem(draft.amount, draft.currency);
    if (problem) problems.push({ kind: "amount", field: "amount", problem });
  } else {
    if (!/^\d+$/.test(draft.laborMinutes.trim())) problems.push({ kind: "field", field: "labor_minutes" });
    // An empty rate is allowed: the cost is saved as "not computable".
    if (draft.laborRate.trim() !== "") {
      const problem = roiAmountProblem(draft.laborRate, draft.currency);
      if (problem) problems.push({ kind: "amount", field: "labor_rate", problem });
    }
  }
  if (draft.incurredAt === "") problems.push({ kind: "field", field: "incurred_at" });
  if (draft.shareMethod !== NOT_SHARED) {
    const incomplete =
      draft.shares.length === 0 ||
      draft.shares.some(
        (share) =>
          share.targetId.trim() === "" ||
          (draft.shareMethod === "weights"
            ? !/^[1-9]\d*$/.test(share.weight.trim())
            : roiAmountProblem(share.amount, draft.currency) !== null),
      );
    if (incomplete) {
      problems.push({ kind: "field", field: "allocations" });
    } else if (draft.shareMethod === "amounts" && draft.pricing === "amount") {
      const check = roiCheckShares(draft.shares.map((share) => share.amount), draft.amount, draft.currency);
      if (check && !check.matches) problems.push({ kind: "shares_gap", ...check });
    }
  }
  return problems;
}

function fieldName(t: Translate, field: string): string {
  switch (field) {
    case "category":
      return t(($) => $.contentRoiReview.costs.category);
    case "incurred_at":
      return t(($) => $.contentRoiReview.costs.incurredAt);
    case "labor_minutes":
      return t(($) => $.contentRoiReview.costs.laborMinutes);
    case "labor_rate":
      return t(($) => $.contentRoiReview.costs.laborRate);
    case "amount":
      return t(($) => $.contentRoiReview.costs.amount);
    default:
      return t(($) => $.contentRoiReview.costs.shares);
  }
}

function problemLabel(t: Translate, problem: CostProblem, currency: string): string {
  switch (problem.kind) {
    case "field":
      return t(($) => $.contentRoiReview.missingField, { field: fieldName(t, problem.field) });
    case "amount":
      return labeled(t, fieldName(t, problem.field), amountProblemLabel(t, problem.problem, currency));
    default:
      return t(($) => $.contentRoiReview.costs.sharesGap, {
        total: problem.total,
        original: problem.original,
        gap: problem.gap,
      });
  }
}

function costBody(draft: CostDraft, options: { editing: boolean; hadShares: boolean; notDuplicateOf: string[] }) {
  const body: Record<string, unknown> = {
    category: draft.category.trim(),
    pricing: draft.pricing,
    currency: draft.currency,
    incurred_at: toInstant(draft.incurredAt),
    ad_spend: draft.adSpend,
    account_id: draft.accountId,
    work_id: draft.workId,
    campaign_label: draft.campaignLabel.trim(),
    evidence_note: draft.evidenceNote,
    note: draft.note,
  };
  if (draft.pricing === "amount") body.amount = draft.amount.trim();
  else {
    // A count of minutes, not money.
    body.labor_minutes = Number(draft.laborMinutes.trim());
    if (draft.laborRate.trim() !== "") body.labor_rate = draft.laborRate.trim();
  }
  if (draft.shareMethod !== NOT_SHARED) {
    body.allocations = draft.shares.map((share) =>
      draft.shareMethod === "weights"
        ? { target_kind: share.targetKind, target_id: share.targetId.trim(), weight: Number(share.weight.trim()) }
        : { target_kind: share.targetKind, target_id: share.targetId.trim(), amount: share.amount.trim() },
    );
  } else if (options.editing && options.hadShares) {
    // [] ends the split; leaving it out would keep the previous one.
    body.allocations = [];
  }
  if (options.notDuplicateOf.length > 0) body.not_duplicate_of = options.notDuplicateOf;
  return body;
}

/** A cost's amount as shown: the server's figure, or "not computable" with
 *  its reason in the secondary text colour. */
function CostAmount({ cost }: { cost: RoiCost }) {
  const { t } = useT("common");
  if (cost.amount === null) {
    return (
      <span className="text-body text-muted-foreground">
        {labeled(t, t(($) => $.contentRoiReview.costs.amount), notComputableLabel(t, roiReasonKey(cost.amountStatus)))}
      </span>
    );
  }
  return (
    <span className="text-body">
      {cost.amount} {cost.currency}
    </span>
  );
}

// ---------------------------------------------------------------------------

export function CostsBlock({ wsId, options, focus }: { wsId: string; options: RoiOptions; focus: RoiFocus | null }) {
  const { t } = useT("common");
  const costs = useRoiCosts(wsId, { includeInactive: true });
  const [selectedId, setSelectedId] = useState(focus?.kind === "cost" ? focus.id : "");
  const list = [...(costs.data ?? [])].sort((a, b) => b.incurredAt.localeCompare(a.incurredAt));

  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.costs.listTitle)}>
        <SettingsCard>
          {costs.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : costs.isError ? (
            <StateRow label={t(($) => $.contentRoiReview.loadFailed)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.costs.listEmpty)} />
          ) : (
            list.map((cost) => (
              <SettingsRow
                key={cost.costId}
                label={
                  <button
                    type="button"
                    onClick={() => setSelectedId(cost.costId)}
                    data-active={cost.costId === selectedId}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {cost.category || cost.costId}
                  </button>
                }
                description={[
                  pricingLabel(t, cost.pricing),
                  formatTime(cost.incurredAt),
                  cost.adSpend ? t(($) => $.contentRoiReview.costs.adSpendShort) : "",
                  optionName(options.accounts, cost.accountId),
                  optionName(options.works, cost.workId),
                  cost.voided ? t(($) => $.contentRoiReview.voided) : "",
                  t(($) => $.contentRoiReview.revisionNo, { revision: cost.revision }),
                ]
                  .filter((part) => part !== "")
                  .join(" · ")}
                size="select-wide"
              >
                <CostAmount cost={cost} />
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      <CostFormSection wsId={wsId} options={options} onSaved={setSelectedId} />

      {selectedId !== "" ? (
        <CostDetail
          key={selectedId}
          wsId={wsId}
          costId={selectedId}
          options={options}
          focusRevision={focus?.kind === "cost" && focus.id === selectedId ? focus.revision : null}
        />
      ) : null}
    </>
  );
}

// ---------------------------------------------------------------------------
// The form, shared by a new cost and a new revision.

function CostFormRows({
  draft,
  onChange,
  disabled,
  options,
}: {
  draft: CostDraft;
  onChange: (next: CostDraft) => void;
  disabled: boolean;
  options: RoiOptions;
}) {
  const { t } = useT("common");
  const edit = (patch: Partial<CostDraft>) => onChange({ ...draft, ...patch });
  const pricingItems = ROI_PRICINGS.map((value) => ({ value, label: pricingLabel(t, value) }));
  const methodItems = [
    { value: NOT_SHARED, label: t(($) => $.contentRoiReview.costs.notShared) },
    ...ROI_ALLOCATION_METHODS.map((value) => ({
      value,
      label: t(($) => $.contentRoiReview.allocationMethods[value]),
    })),
  ];
  const targetItems = ROI_ALLOCATION_TARGETS.map((value) => ({ value, label: targetKindLabel(t, value) }));
  const setShare = (index: number, patch: Partial<ShareDraft>) =>
    edit({ shares: draft.shares.map((share, at) => (at === index ? { ...share, ...patch } : share)) });
  const shareCheck =
    draft.shareMethod === "amounts" && draft.pricing === "amount"
      ? roiCheckShares(draft.shares.map((share) => share.amount), draft.amount, draft.currency)
      : null;

  return (
    <>
      <SettingsRow
        label={t(($) => $.contentRoiReview.costs.category)}
        description={t(($) => $.contentRoiReview.costs.categoryHint)}
        size="text"
      >
        <Input
          value={draft.category}
          onChange={(event) => edit({ category: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.costs.category)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.costs.pricing)} size="select">
        <ChoiceSelect
          items={pricingItems}
          value={draft.pricing}
          onChange={(pricing) => edit({ pricing })}
          label={t(($) => $.contentRoiReview.costs.pricing)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.currency)} size="select">
        <CurrencySelect
          value={draft.currency}
          onChange={(currency) => edit({ currency })}
          label={t(($) => $.contentRoiReview.currency)}
          disabled={disabled}
        />
      </SettingsRow>
      {draft.pricing === "amount" ? (
        <SettingsRow
          label={t(($) => $.contentRoiReview.costs.amount)}
          description={t(($) => $.contentRoiReview.amountHint)}
          size="text"
        >
          <Input
            value={draft.amount}
            inputMode="decimal"
            onChange={(event) => edit({ amount: event.target.value })}
            placeholder="3000.00"
            aria-label={t(($) => $.contentRoiReview.costs.amount)}
            disabled={disabled}
          />
        </SettingsRow>
      ) : (
        <>
          <SettingsRow label={t(($) => $.contentRoiReview.costs.laborMinutes)} size="text">
            <Input
              value={draft.laborMinutes}
              inputMode="numeric"
              onChange={(event) => edit({ laborMinutes: event.target.value })}
              placeholder="360"
              aria-label={t(($) => $.contentRoiReview.costs.laborMinutes)}
              disabled={disabled}
            />
          </SettingsRow>
          <SettingsRow
            label={t(($) => $.contentRoiReview.costs.laborRate)}
            description={t(($) => $.contentRoiReview.costs.laborRateHint)}
            size="text"
          >
            <Input
              value={draft.laborRate}
              inputMode="decimal"
              onChange={(event) => edit({ laborRate: event.target.value })}
              placeholder="150.00"
              aria-label={t(($) => $.contentRoiReview.costs.laborRate)}
              disabled={disabled}
            />
          </SettingsRow>
        </>
      )}
      <SettingsRow label={t(($) => $.contentRoiReview.costs.incurredAt)} size="text">
        <Input
          type="datetime-local"
          value={draft.incurredAt}
          onChange={(event) => edit({ incurredAt: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.costs.incurredAt)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentRoiReview.costs.adSpend)}
        description={t(($) => $.contentRoiReview.costs.adSpendHint)}
      >
        <Checkbox
          checked={draft.adSpend}
          onCheckedChange={(value) => edit({ adSpend: value === true })}
          aria-label={t(($) => $.contentRoiReview.costs.adSpend)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.account)} size="select-wide">
        <OptionSelect
          options={options.accounts}
          value={draft.accountId}
          onChange={(accountId) => edit({ accountId })}
          label={t(($) => $.contentRoiReview.account)}
          loading={options.accountsLoading}
          failed={options.accountsFailed}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.work)} size="select-wide">
        <OptionSelect
          options={options.works}
          value={draft.workId}
          onChange={(workId) => edit({ workId })}
          label={t(($) => $.contentRoiReview.work)}
          loading={options.worksLoading}
          failed={options.worksFailed}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.costs.campaignLabel)} size="text">
        <Input
          value={draft.campaignLabel}
          onChange={(event) => edit({ campaignLabel: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.costs.campaignLabel)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.evidenceNote)} size="text" align="start">
        <Textarea
          value={draft.evidenceNote}
          onChange={(event) => edit({ evidenceNote: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.evidenceNote)}
          rows={2}
          disabled={disabled}
        />
      </SettingsRow>

      <SettingsRow
        label={t(($) => $.contentRoiReview.costs.shares)}
        description={t(($) => $.contentRoiReview.costs.sharesHint)}
        size="select-wide"
      >
        <ChoiceSelect
          items={methodItems}
          value={draft.shareMethod}
          onChange={(shareMethod) =>
            edit({
              shareMethod,
              shares:
                shareMethod === NOT_SHARED
                  ? []
                  : draft.shares.length > 0
                    ? draft.shares
                    : [{ targetKind: "work", targetId: "", weight: "1", amount: "" }],
            })
          }
          label={t(($) => $.contentRoiReview.costs.shares)}
          disabled={disabled}
        />
      </SettingsRow>
      {draft.shareMethod !== NOT_SHARED
        ? draft.shares.map((share, index) => (
            <SettingsRow
              key={index}
              label={t(($) => $.contentRoiReview.costs.shareNo, { number: index + 1 })}
              size="text"
              align="start"
            >
              <div className="space-y-2">
                <ChoiceSelect
                  items={targetItems}
                  value={share.targetKind}
                  onChange={(targetKind) => setShare(index, { targetKind, targetId: "" })}
                  label={t(($) => $.contentRoiReview.costs.shareTarget)}
                  disabled={disabled}
                />
                {share.targetKind === "work" ? (
                  <OptionSelect
                    options={options.works}
                    value={share.targetId}
                    onChange={(targetId) => setShare(index, { targetId })}
                    label={t(($) => $.contentRoiReview.work)}
                    loading={options.worksLoading}
                    failed={options.worksFailed}
                    disabled={disabled}
                  />
                ) : share.targetKind === "account" ? (
                  <OptionSelect
                    options={options.accounts}
                    value={share.targetId}
                    onChange={(targetId) => setShare(index, { targetId })}
                    label={t(($) => $.contentRoiReview.account)}
                    loading={options.accountsLoading}
                    failed={options.accountsFailed}
                    disabled={disabled}
                  />
                ) : (
                  <Input
                    type={share.targetKind === "period" ? "month" : "text"}
                    value={share.targetId}
                    onChange={(event) => setShare(index, { targetId: event.target.value })}
                    aria-label={targetKindLabel(t, share.targetKind)}
                    disabled={disabled}
                  />
                )}
                {draft.shareMethod === "weights" ? (
                  <Input
                    value={share.weight}
                    inputMode="numeric"
                    onChange={(event) => setShare(index, { weight: event.target.value })}
                    aria-label={t(($) => $.contentRoiReview.costs.shareWeight)}
                    placeholder={t(($) => $.contentRoiReview.costs.shareWeight)}
                    disabled={disabled}
                  />
                ) : (
                  <Input
                    value={share.amount}
                    inputMode="decimal"
                    onChange={(event) => setShare(index, { amount: event.target.value })}
                    aria-label={t(($) => $.contentRoiReview.costs.shareAmount)}
                    placeholder={t(($) => $.contentRoiReview.costs.shareAmount)}
                    disabled={disabled}
                  />
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => edit({ shares: draft.shares.filter((_, at) => at !== index) })}
                  disabled={disabled}
                >
                  {t(($) => $.contentRoiReview.remove)}
                </Button>
              </div>
            </SettingsRow>
          ))
        : null}
      {draft.shareMethod !== NOT_SHARED ? (
        <SettingsRow
          label={t(($) => $.contentRoiReview.costs.addShare)}
          description={
            shareCheck
              ? shareCheck.matches
                ? t(($) => $.contentRoiReview.costs.sharesMatch, { total: shareCheck.total })
                : t(($) => $.contentRoiReview.costs.sharesGap, { total: shareCheck.total, original: shareCheck.original, gap: shareCheck.gap })
              : undefined
          }
        >
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              edit({ shares: [...draft.shares, { targetKind: "work", targetId: "", weight: "1", amount: "" }] })
            }
            disabled={disabled}
          >
            {t(($) => $.contentRoiReview.costs.addShare)}
          </Button>
        </SettingsRow>
      ) : null}

      <SettingsRow label={t(($) => $.contentRoiReview.note)} size="text" align="start">
        <Textarea
          value={draft.note}
          onChange={(event) => edit({ note: event.target.value })}
          placeholder={t(($) => $.contentRoiReview.notePlaceholder)}
          aria-label={t(($) => $.contentRoiReview.note)}
          rows={2}
          disabled={disabled}
        />
      </SettingsRow>
    </>
  );
}

function useCostSave(wsId: string) {
  const write = useWriteRoiCost(wsId);
  const [outcome, setOutcome] = useState<RoiWriteOutcome | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const duplicateOf = outcome?.kind === "duplicate" && confirmed ? outcome.matches : [];
  const save = (path: string, body: Record<string, unknown>, onSaved: (cost: RoiCost | null) => void) => {
    write.mutate(
      { path, body },
      {
        onSuccess: (cost) => {
          setOutcome(ROI_SAVED);
          setConfirmed(false);
          onSaved(cost);
        },
        onError: (error) => {
          setOutcome(roiWriteOutcome(error));
          setConfirmed(false);
        },
      },
    );
  };
  return { write, outcome, confirmed, setConfirmed, duplicateOf, save };
}

function CostFormSection({
  wsId,
  options,
  onSaved,
}: {
  wsId: string;
  options: RoiOptions;
  onSaved: (costId: string) => void;
}) {
  const { t } = useT("common");
  const [draft, setDraft] = useState<CostDraft>(emptyCostDraft);
  const { write, outcome, confirmed, setConfirmed, duplicateOf, save } = useCostSave(wsId);
  const problems = costProblems(draft);
  const blockedByDuplicate = outcome?.kind === "duplicate" && !confirmed;

  const submit = () =>
    save("costs", costBody(draft, { editing: false, hadShares: false, notDuplicateOf: duplicateOf }), (cost) => {
      setDraft(emptyCostDraft());
      if (cost) onSaved(cost.costId);
    });

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.costs.createTitle)}
      description={t(($) => $.contentRoiReview.costs.createDescription)}
    >
      <SettingsCard>
        <CostFormRows draft={draft} onChange={setDraft} disabled={write.isPending} options={options} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow
          label={t(($) => $.contentRoiReview.save)}
          description={problems.length > 0 ? problems.map((problem) => problemLabel(t, problem, draft.currency)).join(" · ") : undefined}
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button onClick={submit} disabled={write.isPending || problems.length > 0 || blockedByDuplicate}>
              {t(($) => $.contentRoiReview.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// One cost: every revision, a new revision, void.

function SharesSummary({ cost, options }: { cost: RoiCost; options: RoiOptions }) {
  const { t } = useT("common");
  const { accounts, works } = options;
  if (cost.allocations.length === 0) return <ReadOnlyValue value={t(($) => $.contentRoiReview.costs.notShared)} muted />;
  const check = roiCheckStoredShares(
    cost.allocations.map((share) => share.allocatedMinor),
    cost.amountMinor,
    cost.currency,
  );
  const target = (kind: string, id: string) =>
    kind === "work" ? optionName(works, id) : kind === "account" ? optionName(accounts, id) : id;
  return (
    <ReadOnlyList
      items={[
        ...cost.allocations.map((share) =>
          labeled(t, `${targetKindLabel(t, share.targetKind)} ${target(share.targetKind, share.targetId)}`, share.allocated),
        ),
        ...(check
          ? [
              check.matches
                ? t(($) => $.contentRoiReview.costs.sharesMatch, { total: check.total })
                : t(($) => $.contentRoiReview.costs.sharesGap, { total: check.total, original: check.original, gap: check.gap }),
            ]
          : []),
      ]}
      empty=""
    />
  );
}

function CostDetail({
  wsId,
  costId,
  options,
  focusRevision,
}: {
  wsId: string;
  costId: string;
  options: RoiOptions;
  focusRevision: number | null;
}) {
  const { t } = useT("common");
  const cost = useRoiCost(wsId, costId);
  const history = cost.data;

  if (cost.isLoading) {
    return (
      <SettingsSection title={t(($) => $.contentRoiReview.costs.detailTitle)}>
        <SettingsCard>
          <StateRow label={t(($) => $.contentRoiReview.loading)} />
        </SettingsCard>
      </SettingsSection>
    );
  }
  if (!history) {
    return (
      <SettingsSection title={t(($) => $.contentRoiReview.costs.detailTitle)}>
        <SettingsCard>
          <StateRow label={t(($) => $.contentRoiReview.notFound)} />
        </SettingsCard>
      </SettingsSection>
    );
  }
  const revisions = [...history.revisions].sort((a, b) => b.revision - a.revision);

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentRoiReview.costs.detailTitle)}
        description={t(($) => $.contentRoiReview.revisionsHint)}
      >
        <SettingsCard>
          {revisions.map((revision) => (
            <SettingsRow
              key={revision.revision}
              label={
                <span className={revision.revision === focusRevision ? "font-semibold" : undefined}>
                  {t(($) => $.contentRoiReview.revisionNo, { revision: revision.revision })}
                  {revision.voided ? ` · ${t(($) => $.contentRoiReview.voided)}` : ""}
                </span>
              }
              description={[
                revision.category,
                pricingLabel(t, revision.pricing),
                revision.laborMinutes !== null
                  ? t(($) => $.contentRoiReview.costs.laborSummary, {
                      minutes: revision.laborMinutes,
                      rate: revision.laborRate ?? t(($) => $.contentRoiReview.costs.noRate),
                    })
                  : "",
                formatTime(revision.incurredAt),
                revision.adSpend ? t(($) => $.contentRoiReview.costs.adSpendShort) : "",
                optionName(options.accounts, revision.accountId),
                optionName(options.works, revision.workId),
                revision.campaignLabel,
                sourceLabel(t, revision.sourceType),
                revision.notDuplicateOf.length > 0
                  ? t(($) => $.contentRoiReview.confirmedNotDuplicateOf, { ids: joinList(t, revision.notDuplicateOf) })
                  : "",
                revision.note,
                t(($) => $.contentRoiReview.recordedBy, { who: revision.recordedBy, at: formatTime(revision.createdAt) }),
              ]
                .filter((part) => part !== "")
                .join(" · ")}
              size="text"
              align="start"
            >
              <div className="space-y-1">
                <CostAmount cost={revision} />
                <SharesSummary cost={revision} options={options} />
              </div>
            </SettingsRow>
          ))}
        </SettingsCard>
      </SettingsSection>
      <CostRevisionSection key={history.current.revision} wsId={wsId} current={history.current} options={options} />
    </>
  );
}

function CostRevisionSection({ wsId, current, options }: { wsId: string; current: RoiCost; options: RoiOptions }) {
  const { t } = useT("common");
  const [draft, setDraft] = useState<CostDraft>(() => draftFromCost(current));
  const { write, outcome, confirmed, setConfirmed, duplicateOf, save } = useCostSave(wsId);
  const problems = costProblems(draft);
  const blockedByDuplicate = outcome?.kind === "duplicate" && !confirmed;
  const path = roiPath("costs", current.costId, "revisions");

  const submit = () =>
    save(
      path,
      {
        ...costBody(draft, { editing: true, hadShares: current.allocations.length > 0, notDuplicateOf: duplicateOf }),
        base_revision: current.revision,
        voided: current.voided,
      },
      () => undefined,
    );

  // Void or restore: the same fields as the current revision, flag flipped.
  // Allocations are left out, so the split carries over unchanged.
  const toggleVoid = () => {
    const body = costBody(draftFromCost(current), { editing: false, hadShares: false, notDuplicateOf: [] });
    delete body.allocations;
    save(path, { ...body, base_revision: current.revision, voided: !current.voided }, () => undefined);
  };

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.costs.reviseTitle)}
      description={t(($) => $.contentRoiReview.reviseDescription, { revision: current.revision })}
    >
      <SettingsCard>
        <CostFormRows draft={draft} onChange={setDraft} disabled={write.isPending} options={options} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow
          label={t(($) => $.contentRoiReview.saveRevision)}
          description={problems.length > 0 ? problems.map((problem) => problemLabel(t, problem, draft.currency)).join(" · ") : undefined}
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button variant="outline" onClick={toggleVoid} disabled={write.isPending}>
              {current.voided ? t(($) => $.contentRoiReview.restore) : t(($) => $.contentRoiReview.void)}
            </Button>
            <Button onClick={submit} disabled={write.isPending || problems.length > 0 || blockedByDuplicate}>
              {t(($) => $.contentRoiReview.saveRevision)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
