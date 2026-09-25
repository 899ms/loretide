"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_ADJUSTMENT_KINDS,
  ROI_GROSS_BASES,
  ROI_JUDGEMENTS,
  ROI_SAVED,
  grossDeltaShown,
  roiAmountProblem,
  roiDealGrossState,
  roiPath,
  roiWriteOutcome,
  signedGrossDelta,
  useRoiDeal,
  useRoiDeals,
  useRoiLead,
  useRoiLeads,
  useWriteRoiAdjustment,
  useWriteRoiAttribution,
  useWriteRoiDeal,
  type RoiAdjustment,
  type RoiAttribution,
  type RoiDeal,
  type RoiGrossDeltaDirection,
  type RoiLead,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";
import { leadOptionLabel, touchSummary } from "./leads";
import {
  ChoiceSelect,
  CurrencySelect,
  DuplicateRow,
  ReadOnlyList,
  ReadOnlyValue,
  SaveFeedback,
  StateRow,
  adjustmentKindLabel,
  amountProblemLabel,
  formatTime,
  grossBasisLabel,
  joinList,
  judgementLabel,
  labeled,
  notComputableLabel,
  nowLocalInput,
  sourceLabel,
  toInstant,
  toLocalInput,
  type RoiFocus,
  type RoiOptions,
  type Translate,
} from "./shared";

// The deal block (specs/034 PR 5, T089; U-12, U-14 to U-16).
//
// Three things live on a deal and are kept apart on the page as they are in
// storage: the deal itself (revised, never overwritten), its refunds and
// adjustments (each its own record), and the operator's attribution
// judgement - which touches the deal is credited to. The judgement is a
// judgement, labelled as one; it changes no touch.
//
// Signs, stated once so nobody has to guess:
//   - a refund's revenue change is a REDUCTION typed as a positive number;
//   - the gross-profit change is SIGNED and stored as typed: the person picks
//     "reduction" and types 300, the page sends "-300" (contract §1.6, §5.5),
//     and the stored value is shown back with its minus sign.

interface DealDraft {
  leadId: string;
  orderRef: string;
  amount: string;
  currency: string;
  closedAt: string;
  grossBasis: string;
  grossProfit: string;
  cogs: string;
  note: string;
}

function emptyDealDraft(): DealDraft {
  return {
    leadId: "",
    orderRef: "",
    amount: "",
    currency: "CNY",
    closedAt: nowLocalInput(),
    grossBasis: "none",
    grossProfit: "",
    cogs: "",
    note: "",
  };
}

function draftFromDeal(deal: RoiDeal): DealDraft {
  return {
    leadId: deal.leadId,
    orderRef: deal.orderRef,
    amount: deal.amount,
    currency: deal.currency,
    closedAt: toLocalInput(deal.closedAt),
    grossBasis: deal.grossBasis,
    grossProfit: deal.grossProfit ?? "",
    cogs: deal.cogs ?? "",
    note: "",
  };
}

function dealProblems(t: Translate, draft: DealDraft): string[] {
  const problems: string[] = [];
  const amount = roiAmountProblem(draft.amount, draft.currency);
  if (amount) problems.push(labeled(t, t(($) => $.contentRoiReview.deals.amount), amountProblemLabel(t, amount, draft.currency)));
  if (draft.closedAt === "") {
    problems.push(t(($) => $.contentRoiReview.missingField, { field: t(($) => $.contentRoiReview.deals.closedAt) }));
  }
  if (draft.grossBasis === "stated_gross_profit") {
    const problem = roiAmountProblem(draft.grossProfit, draft.currency, { signed: true });
    if (problem) problems.push(labeled(t, t(($) => $.contentRoiReview.deals.grossProfit), amountProblemLabel(t, problem, draft.currency)));
  }
  if (draft.grossBasis === "cogs") {
    const problem = roiAmountProblem(draft.cogs, draft.currency);
    if (problem) problems.push(labeled(t, t(($) => $.contentRoiReview.deals.cogs), amountProblemLabel(t, problem, draft.currency)));
  }
  return problems;
}

function dealBody(draft: DealDraft, notDuplicateOf: string[]): Record<string, unknown> {
  const body: Record<string, unknown> = {
    lead_id: draft.leadId,
    order_ref: draft.orderRef.trim(),
    amount: draft.amount.trim(),
    currency: draft.currency,
    closed_at: toInstant(draft.closedAt),
    gross_basis: draft.grossBasis,
    note: draft.note,
  };
  if (draft.grossBasis === "stated_gross_profit") body.gross_profit = draft.grossProfit.trim();
  if (draft.grossBasis === "cogs") body.cogs = draft.cogs.trim();
  if (notDuplicateOf.length > 0) body.not_duplicate_of = notDuplicateOf;
  return body;
}

function useSave<T>(write: {
  mutate: (
    vars: { path: string; body: Record<string, unknown> },
    opts: { onSuccess: (value: T) => void; onError: (error: Error) => void },
  ) => void;
}) {
  const [outcome, setOutcome] = useState<RoiWriteOutcome | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const duplicateOf = outcome?.kind === "duplicate" && confirmed ? outcome.matches : [];
  const save = (path: string, body: Record<string, unknown>, onSaved: (value: T) => void) =>
    write.mutate(
      { path, body },
      {
        onSuccess: (value) => {
          setOutcome(ROI_SAVED);
          setConfirmed(false);
          onSaved(value);
        },
        onError: (error) => {
          setOutcome(roiWriteOutcome(error));
          setConfirmed(false);
        },
      },
    );
  return { outcome, confirmed, setConfirmed, duplicateOf, save };
}

export function DealsBlock({ wsId, options, focus }: { wsId: string; options: RoiOptions; focus: RoiFocus | null }) {
  const { t } = useT("common");
  const deals = useRoiDeals(wsId, { includeInactive: true });
  const leads = useRoiLeads(wsId, { includeInactive: true });
  const [selectedId, setSelectedId] = useState(focus?.kind === "deal" ? focus.id : "");
  const list = [...(deals.data ?? [])].sort((a, b) => b.closedAt.localeCompare(a.closedAt));
  const leadList = leads.data ?? [];

  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.deals.listTitle)}>
        <SettingsCard>
          {deals.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : deals.isError ? (
            <StateRow label={t(($) => $.contentRoiReview.loadFailed)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.deals.listEmpty)} />
          ) : (
            list.map((deal) => (
              <SettingsRow
                key={deal.dealId}
                label={
                  <button
                    type="button"
                    onClick={() => setSelectedId(deal.dealId)}
                    data-active={deal.dealId === selectedId}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {deal.orderRef || deal.dealId}
                  </button>
                }
                description={[
                  leadOptionLabel(t, leadList, deal.leadId),
                  formatTime(deal.closedAt),
                  grossBasisLabel(t, deal.grossBasis),
                  deal.voided ? t(($) => $.contentRoiReview.voided) : "",
                  t(($) => $.contentRoiReview.revisionNo, { revision: deal.revision }),
                ]
                  .filter((part) => part !== "")
                  .join(" · ")}
                size="select-wide"
              >
                <span className="text-body">
                  {deal.amount} {deal.currency}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      <DealCreateSection wsId={wsId} leads={leadList} onSaved={setSelectedId} />

      {selectedId !== "" ? (
        <DealDetail
          key={selectedId}
          wsId={wsId}
          dealId={selectedId}
          leads={leadList}
          options={options}
          focusRevision={focus?.kind === "deal" && focus.id === selectedId ? focus.revision : null}
        />
      ) : null}
    </>
  );
}

function DealFormRows({
  draft,
  onChange,
  disabled,
  leads,
}: {
  draft: DealDraft;
  onChange: (next: DealDraft) => void;
  disabled: boolean;
  leads: RoiLead[];
}) {
  const { t } = useT("common");
  const edit = (patch: Partial<DealDraft>) => onChange({ ...draft, ...patch });
  const basisItems = ROI_GROSS_BASES.map((value) => ({ value, label: grossBasisLabel(t, value) }));
  const leadItems = [
    { value: "__none__", label: t(($) => $.contentRoiReview.notLinked) },
    ...(draft.leadId !== "" && !leads.some((lead) => lead.leadId === draft.leadId)
      ? [{ value: draft.leadId, label: draft.leadId }]
      : []),
    ...leads
      .filter((lead) => !lead.voided && lead.mergedInto === "")
      .map((lead) => ({ value: lead.leadId, label: leadOptionLabel(t, leads, lead.leadId) })),
  ];
  return (
    <>
      <SettingsRow label={t(($) => $.contentRoiReview.deals.lead)} size="select-wide">
        <ChoiceSelect
          items={leadItems}
          value={draft.leadId === "" ? "__none__" : draft.leadId}
          onChange={(value) => edit({ leadId: value === "__none__" ? "" : value })}
          label={t(($) => $.contentRoiReview.deals.lead)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentRoiReview.deals.orderRef)}
        description={t(($) => $.contentRoiReview.deals.orderRefHint)}
        size="text"
      >
        <Input
          value={draft.orderRef}
          onChange={(event) => edit({ orderRef: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.deals.orderRef)}
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
      <SettingsRow
        label={t(($) => $.contentRoiReview.deals.amount)}
        description={t(($) => $.contentRoiReview.amountHint)}
        size="text"
      >
        <Input
          value={draft.amount}
          inputMode="decimal"
          onChange={(event) => edit({ amount: event.target.value })}
          placeholder="10000.00"
          aria-label={t(($) => $.contentRoiReview.deals.amount)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.deals.closedAt)} size="text">
        <Input
          type="datetime-local"
          value={draft.closedAt}
          onChange={(event) => edit({ closedAt: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.deals.closedAt)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentRoiReview.deals.grossBasis)}
        description={t(($) => $.contentRoiReview.deals.grossBasisHint)}
        size="select-wide"
      >
        <ChoiceSelect
          items={basisItems}
          value={draft.grossBasis}
          onChange={(grossBasis) => edit({ grossBasis })}
          label={t(($) => $.contentRoiReview.deals.grossBasis)}
          disabled={disabled}
        />
      </SettingsRow>
      {draft.grossBasis === "stated_gross_profit" ? (
        <SettingsRow label={t(($) => $.contentRoiReview.deals.grossProfit)} size="text">
          <Input
            value={draft.grossProfit}
            inputMode="decimal"
            onChange={(event) => edit({ grossProfit: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.deals.grossProfit)}
            disabled={disabled}
          />
        </SettingsRow>
      ) : null}
      {draft.grossBasis === "cogs" ? (
        <SettingsRow label={t(($) => $.contentRoiReview.deals.cogs)} size="text">
          <Input
            value={draft.cogs}
            inputMode="decimal"
            onChange={(event) => edit({ cogs: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.deals.cogs)}
            disabled={disabled}
          />
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

function DealCreateSection({
  wsId,
  leads,
  onSaved,
}: {
  wsId: string;
  leads: RoiLead[];
  onSaved: (dealId: string) => void;
}) {
  const { t } = useT("common");
  const write = useWriteRoiDeal(wsId);
  const [draft, setDraft] = useState<DealDraft>(emptyDealDraft);
  const { outcome, confirmed, setConfirmed, duplicateOf, save } = useSave<RoiDeal | null>(write);
  const problems = dealProblems(t, draft);
  const blocked = outcome?.kind === "duplicate" && !confirmed;

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.deals.createTitle)}
      description={t(($) => $.contentRoiReview.deals.createDescription)}
    >
      <SettingsCard>
        <DealFormRows draft={draft} onChange={setDraft} disabled={write.isPending} leads={leads} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow label={t(($) => $.contentRoiReview.save)} description={problems.length > 0 ? problems.join(" · ") : undefined}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button
              onClick={() =>
                save("deals", dealBody(draft, duplicateOf), (deal) => {
                  setDraft(emptyDealDraft());
                  if (deal) onSaved(deal.dealId);
                })
              }
              disabled={write.isPending || problems.length > 0 || blocked}
            >
              {t(($) => $.contentRoiReview.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// One deal.

function grossDeltaText(t: Translate, adjustment: RoiAdjustment): string {
  const shown = grossDeltaShown(adjustment.grossDelta);
  if (shown === "not_given") return t(($) => $.contentRoiReview.deals.grossDeltaNotGiven);
  return t(($) => $.contentRoiReview.deals.grossDeltaStored, {
    value: adjustment.grossDelta ?? "",
    currency: adjustment.currency,
    direction: t(($) => $.contentRoiReview.deals.grossDeltaShown[shown]),
  });
}

function DealGrossLine({ deal, adjustments }: { deal: RoiDeal; adjustments: RoiAdjustment[] }) {
  const { t } = useT("common");
  const state = roiDealGrossState(deal, adjustments);
  switch (state.kind) {
    case "no_basis":
      return <ReadOnlyValue value={t(($) => $.contentRoiReview.deals.noGrossBasis)} muted />;
    case "refund_without_gross_delta":
      return (
        <ReadOnlyValue
          value={labeled(t, t(($) => $.contentRoiReview.deals.grossProfit), notComputableLabel(t, "refund_without_gross_delta"))}
          muted
        />
      );
    case "stated":
      return <ReadOnlyValue value={`${state.grossProfit} ${deal.currency}`} />;
    default:
      return (
        <ReadOnlyValue
          value={
            deal.grossBasis === "cogs"
              ? t(($) => $.contentRoiReview.deals.grossFromCogs, {
                  amount: deal.amount,
                  cogs: deal.cogs ?? "",
                  currency: deal.currency,
                })
              : t(($) => $.contentRoiReview.deals.grossInReport)
          }
          muted
        />
      );
  }
}

function DealDetail({
  wsId,
  dealId,
  leads,
  options,
  focusRevision,
}: {
  wsId: string;
  dealId: string;
  leads: RoiLead[];
  options: RoiOptions;
  focusRevision: number | null;
}) {
  const { t } = useT("common");
  const deal = useRoiDeal(wsId, dealId);
  const detail = deal.data;

  if (deal.isLoading || !detail) {
    return (
      <SettingsSection title={t(($) => $.contentRoiReview.deals.detailTitle)}>
        <SettingsCard>
          <StateRow label={deal.isLoading ? t(($) => $.contentRoiReview.loading) : t(($) => $.contentRoiReview.notFound)} />
        </SettingsCard>
      </SettingsSection>
    );
  }
  const revisions = [...detail.revisions].sort((a, b) => b.revision - a.revision);
  const adjustments = detail.adjustments.map((item) => item.current);

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentRoiReview.deals.detailTitle)}
        description={t(($) => $.contentRoiReview.revisionsHint)}
      >
        <SettingsCard>
          <SettingsRow label={t(($) => $.contentRoiReview.deals.grossProfit)} size="text" align="start">
            <DealGrossLine deal={detail.current} adjustments={adjustments} />
          </SettingsRow>
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
                revision.orderRef,
                leadOptionLabel(t, leads, revision.leadId),
                formatTime(revision.closedAt),
                grossBasisLabel(t, revision.grossBasis),
                revision.grossProfit !== null
                  ? labeled(t, t(($) => $.contentRoiReview.deals.grossProfit), revision.grossProfit)
                  : "",
                revision.cogs !== null ? labeled(t, t(($) => $.contentRoiReview.deals.cogs), revision.cogs) : "",
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
              <ReadOnlyValue value={`${revision.amount} ${revision.currency}`} />
            </SettingsRow>
          ))}
        </SettingsCard>
      </SettingsSection>
      <DealRevisionSection key={`revise-${detail.current.revision}`} wsId={wsId} current={detail.current} leads={leads} />
      <AdjustmentsSection wsId={wsId} deal={detail.current} adjustments={adjustments} />
      <AttributionSection
        key={`judge-${detail.attributions.length}`}
        wsId={wsId}
        deal={detail.current}
        attributions={detail.attributions}
        options={options}
      />
    </>
  );
}

function DealRevisionSection({ wsId, current, leads }: { wsId: string; current: RoiDeal; leads: RoiLead[] }) {
  const { t } = useT("common");
  const write = useWriteRoiDeal(wsId);
  const [draft, setDraft] = useState<DealDraft>(() => draftFromDeal(current));
  const { outcome, confirmed, setConfirmed, duplicateOf, save } = useSave<RoiDeal | null>(write);
  const problems = dealProblems(t, draft);
  const blocked = outcome?.kind === "duplicate" && !confirmed;
  const path = roiPath("deals", current.dealId, "revisions");

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.deals.reviseTitle)}
      description={t(($) => $.contentRoiReview.reviseDescription, { revision: current.revision })}
    >
      <SettingsCard>
        <DealFormRows draft={draft} onChange={setDraft} disabled={write.isPending} leads={leads} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow label={t(($) => $.contentRoiReview.saveRevision)} description={problems.length > 0 ? problems.join(" · ") : undefined}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button
              variant="outline"
              onClick={() =>
                save(
                  path,
                  { ...dealBody(draftFromDeal(current), []), base_revision: current.revision, voided: !current.voided },
                  () => undefined,
                )
              }
              disabled={write.isPending}
            >
              {current.voided ? t(($) => $.contentRoiReview.restore) : t(($) => $.contentRoiReview.void)}
            </Button>
            <Button
              onClick={() =>
                save(path, { ...dealBody(draft, duplicateOf), base_revision: current.revision, voided: current.voided }, () => undefined)
              }
              disabled={write.isPending || problems.length > 0 || blocked}
            >
              {t(($) => $.contentRoiReview.saveRevision)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// Refunds and adjustments.

interface AdjustmentDraft {
  kind: string;
  amount: string;
  grossDirection: RoiGrossDeltaDirection;
  grossAmount: string;
  occurredAt: string;
  note: string;
}

function emptyAdjustmentDraft(): AdjustmentDraft {
  return { kind: "refund", amount: "", grossDirection: "not_given", grossAmount: "", occurredAt: nowLocalInput(), note: "" };
}

function draftFromAdjustment(adjustment: RoiAdjustment): AdjustmentDraft {
  const shown = grossDeltaShown(adjustment.grossDelta);
  const value = adjustment.grossDelta ?? "";
  return {
    kind: adjustment.kind,
    amount: adjustment.revenueDelta,
    grossDirection: shown === "not_given" ? "not_given" : shown === "reduction" ? "reduction" : "increase",
    grossAmount: shown === "reduction" ? value.replace(/^-/, "") : value,
    occurredAt: toLocalInput(adjustment.occurredAt),
    note: "",
  };
}

function adjustmentBody(draft: AdjustmentDraft, currency: string): Record<string, unknown> {
  const body: Record<string, unknown> = {
    kind: draft.kind,
    amount: draft.amount.trim(),
    currency,
    occurred_at: toInstant(draft.occurredAt),
    note: draft.note,
  };
  const gross = signedGrossDelta(draft.grossDirection, draft.grossAmount);
  if (gross !== null) body.gross_delta = gross;
  return body;
}

function AdjustmentsSection({ wsId, deal, adjustments }: { wsId: string; deal: RoiDeal; adjustments: RoiAdjustment[] }) {
  const { t } = useT("common");
  const write = useWriteRoiAdjustment(wsId);
  const { outcome, save } = useSave<RoiAdjustment | null>(write);
  const [editing, setEditing] = useState<RoiAdjustment | null>(null);
  const [draft, setDraft] = useState<AdjustmentDraft>(emptyAdjustmentDraft);
  const edit = (patch: Partial<AdjustmentDraft>) => setDraft({ ...draft, ...patch });
  const kindItems = ROI_ADJUSTMENT_KINDS.map((value) => ({ value, label: adjustmentKindLabel(t, value) }));
  const directionItems = (["not_given", "reduction", "increase"] as const).map((value) => ({
    value,
    label: t(($) => $.contentRoiReview.deals.grossDirections[value]),
  }));
  const amountProblem = roiAmountProblem(draft.amount, deal.currency, { signed: draft.kind === "adjustment" });
  const grossProblem =
    draft.grossDirection === "not_given" ? null : roiAmountProblem(draft.grossAmount, deal.currency);
  const signed = signedGrossDelta(draft.grossDirection, draft.grossAmount);
  const problems = [
    amountProblem
      ? labeled(t, t(($) => $.contentRoiReview.deals.revenueDelta), amountProblemLabel(t, amountProblem, deal.currency))
      : "",
    grossProblem
      ? labeled(t, t(($) => $.contentRoiReview.deals.grossDelta), amountProblemLabel(t, grossProblem, deal.currency))
      : "",
    draft.occurredAt === ""
      ? t(($) => $.contentRoiReview.missingField, { field: t(($) => $.contentRoiReview.deals.occurredAt) })
      : "",
  ].filter((problem) => problem !== "");

  const reset = () => {
    setEditing(null);
    setDraft(emptyAdjustmentDraft());
  };
  const submit = () => {
    const body = adjustmentBody(draft, deal.currency);
    if (editing) {
      save(
        roiPath("deals", deal.dealId, "adjustments", editing.adjustmentId, "revisions"),
        { ...body, base_revision: editing.revision, voided: editing.voided },
        reset,
      );
    } else {
      save(roiPath("deals", deal.dealId, "adjustments"), body, reset);
    }
  };
  const toggleVoid = (adjustment: RoiAdjustment) =>
    save(
      roiPath("deals", deal.dealId, "adjustments", adjustment.adjustmentId, "revisions"),
      {
        ...adjustmentBody(draftFromAdjustment(adjustment), deal.currency),
        base_revision: adjustment.revision,
        voided: !adjustment.voided,
      },
      () => undefined,
    );

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.deals.adjustmentsTitle)}
      description={t(($) => $.contentRoiReview.deals.adjustmentsDescription)}
    >
      <SettingsCard>
        {adjustments.length === 0 ? (
          <StateRow label={t(($) => $.contentRoiReview.deals.adjustmentsEmpty)} />
        ) : (
          adjustments.map((adjustment) => (
            <SettingsRow
              key={adjustment.adjustmentId}
              label={
                <span>
                  {adjustmentKindLabel(t, adjustment.kind)}
                  {adjustment.voided ? ` · ${t(($) => $.contentRoiReview.voided)}` : ""}
                </span>
              }
              description={[
                t(($) => $.contentRoiReview.deals.revenueDeltaStored, {
                  value: adjustment.revenueDelta,
                  currency: adjustment.currency,
                }),
                grossDeltaText(t, adjustment),
                formatTime(adjustment.occurredAt),
                adjustment.note,
                t(($) => $.contentRoiReview.revisionNo, { revision: adjustment.revision }),
              ]
                .filter((part) => part !== "")
                .join(" · ")}
              size="select-wide"
            >
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setEditing(adjustment);
                    setDraft(draftFromAdjustment(adjustment));
                  }}
                  disabled={write.isPending}
                >
                  {t(($) => $.contentRoiReview.edit)}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => toggleVoid(adjustment)} disabled={write.isPending}>
                  {adjustment.voided ? t(($) => $.contentRoiReview.restore) : t(($) => $.contentRoiReview.void)}
                </Button>
              </div>
            </SettingsRow>
          ))
        )}

        <SettingsRow
          label={
            editing
              ? t(($) => $.contentRoiReview.deals.editAdjustment, { revision: editing.revision })
              : t(($) => $.contentRoiReview.deals.addAdjustment)
          }
          size="select"
        >
          <ChoiceSelect
            items={kindItems}
            value={draft.kind}
            onChange={(kind) => edit({ kind })}
            label={t(($) => $.contentRoiReview.deals.adjustmentKind)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.deals.revenueDelta)}
          description={t(($) => $.contentRoiReview.deals.revenueDeltaHint, { currency: deal.currency })}
          size="text"
        >
          <Input
            value={draft.amount}
            inputMode="decimal"
            onChange={(event) => edit({ amount: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.deals.revenueDelta)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.deals.grossDelta)}
          description={t(($) => $.contentRoiReview.deals.grossDeltaHint)}
          size="select-wide"
        >
          <ChoiceSelect
            items={directionItems}
            value={draft.grossDirection}
            onChange={(value) => edit({ grossDirection: value as RoiGrossDeltaDirection })}
            label={t(($) => $.contentRoiReview.deals.grossDelta)}
            disabled={write.isPending}
          />
        </SettingsRow>
        {draft.grossDirection !== "not_given" ? (
          <SettingsRow
            label={t(($) => $.contentRoiReview.deals.grossDeltaAmount)}
            description={
              signed !== null && grossProblem === null
                ? t(($) => $.contentRoiReview.deals.grossDeltaWillStore, { value: signed, currency: deal.currency })
                : undefined
            }
            size="text"
          >
            <Input
              value={draft.grossAmount}
              inputMode="decimal"
              onChange={(event) => edit({ grossAmount: event.target.value })}
              aria-label={t(($) => $.contentRoiReview.deals.grossDeltaAmount)}
              disabled={write.isPending}
            />
          </SettingsRow>
        ) : null}
        <SettingsRow label={t(($) => $.contentRoiReview.deals.occurredAt)} size="text">
          <Input
            type="datetime-local"
            value={draft.occurredAt}
            onChange={(event) => edit({ occurredAt: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.deals.occurredAt)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.note)} size="text" align="start">
          <Textarea
            value={draft.note}
            onChange={(event) => edit({ note: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.note)}
            rows={2}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.save)} description={problems.length > 0 ? problems.join(" · ") : undefined}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            {editing ? (
              <Button variant="ghost" onClick={reset} disabled={write.isPending}>
                {t(($) => $.contentRoiReview.cancel)}
              </Button>
            ) : null}
            <Button onClick={submit} disabled={write.isPending || problems.length > 0}>
              {t(($) => $.contentRoiReview.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// The attribution judgement.

function AttributionSection({
  wsId,
  deal,
  attributions,
  options,
}: {
  wsId: string;
  deal: RoiDeal;
  attributions: RoiAttribution[];
  options: RoiOptions;
}) {
  const { t } = useT("common");
  const write = useWriteRoiAttribution(wsId);
  const { outcome, save } = useSave<RoiAttribution | null>(write);
  const lead = useRoiLead(wsId, deal.leadId);
  const history = [...attributions].sort((a, b) => b.revision - a.revision);
  const latest = history[0] ?? null;
  const [judgement, setJudgement] = useState(latest?.judgement ?? "operator_judgement");
  const [chosen, setChosen] = useState<string[]>(latest?.touchIds ?? []);
  const [weights, setWeights] = useState<Record<string, string>>(() =>
    Object.fromEntries((latest?.touchIds ?? []).map((id, index) => [id, latest?.weights[index] !== undefined ? String(latest.weights[index]) : ""])),
  );
  const [note, setNote] = useState("");
  const touches = (lead.data?.touches ?? []).map((touch) => touch.current).filter((touch) => !touch.voided);
  const judgementItems = ROI_JUDGEMENTS.map((value) => ({ value, label: judgementLabel(t, value) }));
  const unknown = judgement === "unknown";
  const weightValues = chosen.map((id) => (weights[id] ?? "").trim());
  const anyWeight = weightValues.some((value) => value !== "");
  const weightsValid = !anyWeight || weightValues.every((value) => /^[1-9]\d*$/.test(value));

  const submit = () => {
    const touchIds = unknown ? [] : chosen;
    save(
      roiPath("deals", deal.dealId, "attribution"),
      {
        judgement,
        touch_ids: touchIds,
        // Weights are counts, not money; all or none.
        weights: !unknown && anyWeight ? weightValues.map(Number) : [],
        note,
        base_revision: latest?.revision ?? 0,
      },
      () => setNote(""),
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.deals.attributionTitle)}
      description={t(($) => $.contentRoiReview.deals.attributionDescription)}
    >
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentRoiReview.deals.attributionHistory)} size="text" align="start">
          <ReadOnlyList
            items={history.map((item) =>
              [
                t(($) => $.contentRoiReview.revisionNo, { revision: item.revision }),
                judgementLabel(t, item.judgement),
                item.touchIds.length > 0
                  ? t(($) => $.contentRoiReview.deals.acceptedTouches, { ids: joinList(t, item.touchIds) })
                  : t(($) => $.contentRoiReview.deals.noAcceptedTouch),
                item.weights.length > 0 ? t(($) => $.contentRoiReview.deals.weightsShown, { weights: item.weights.join(":") }) : "",
                item.note,
                t(($) => $.contentRoiReview.recordedBy, { who: item.recordedBy, at: formatTime(item.createdAt) }),
              ]
                .filter((part) => part !== "")
                .join(" · "),
            )}
            empty={t(($) => $.contentRoiReview.deals.noJudgement)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.deals.judgement)} size="select-wide">
          <ChoiceSelect
            items={judgementItems}
            value={judgement}
            onChange={setJudgement}
            label={t(($) => $.contentRoiReview.deals.judgement)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.deals.acceptTouches)}
          description={
            deal.leadId === ""
              ? t(($) => $.contentRoiReview.deals.noLeadNoTouches)
              : t(($) => $.contentRoiReview.deals.weightsHint)
          }
          size="text"
          align="start"
        >
          <div className="space-y-2">
            {lead.isLoading && deal.leadId !== "" ? (
              <span className="text-body text-muted-foreground">{t(($) => $.contentRoiReview.loading)}</span>
            ) : touches.length === 0 ? (
              <span className="text-body text-muted-foreground">{t(($) => $.contentRoiReview.leads.touchesEmpty)}</span>
            ) : (
              touches.map((touch) => {
                const checked = chosen.includes(touch.touchId);
                return (
                  <div key={touch.touchId} className="space-y-1">
                    <label className="flex items-start gap-3 text-body text-muted-foreground">
                      <Checkbox
                        checked={checked && !unknown}
                        disabled={write.isPending || unknown}
                        onCheckedChange={(value) =>
                          setChosen(
                            value === true
                              ? [...chosen, touch.touchId]
                              : chosen.filter((id) => id !== touch.touchId),
                          )
                        }
                        aria-label={touch.touchId}
                      />
                      <span className="break-words">{touchSummary(t, touch, options)}</span>
                    </label>
                    {checked && !unknown ? (
                      <Input
                        value={weights[touch.touchId] ?? ""}
                        inputMode="numeric"
                        onChange={(event) => setWeights({ ...weights, [touch.touchId]: event.target.value })}
                        placeholder={t(($) => $.contentRoiReview.deals.weightPlaceholder)}
                        aria-label={t(($) => $.contentRoiReview.deals.weightPlaceholder)}
                        disabled={write.isPending}
                      />
                    ) : null}
                  </div>
                );
              })
            )}
          </div>
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.note)} size="text" align="start">
          <Textarea
            value={note}
            onChange={(event) => setNote(event.target.value)}
            aria-label={t(($) => $.contentRoiReview.note)}
            rows={2}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.deals.saveJudgement)}
          description={
            !weightsValid
              ? t(($) => $.contentRoiReview.deals.weightsInvalid)
              : t(($) => $.contentRoiReview.deals.judgementNotEvidence)
          }
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button onClick={submit} disabled={write.isPending || !weightsValid}>
              {t(($) => $.contentRoiReview.deals.saveJudgement)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
