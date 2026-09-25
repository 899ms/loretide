"use client";

import { useId, useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  ROI_EVIDENCE_TYPES,
  ROI_SAVED,
  ROI_TOUCH_PLATFORMS,
  ROI_TOUCH_ROLES,
  roiPath,
  roiTouchForEvidence,
  roiTouchProblems,
  roiTouchRules,
  roiWriteOutcome,
  useRoiLead,
  useRoiLeads,
  useWriteRoiLead,
  useWriteRoiTouch,
  type RoiLead,
  type RoiTouch,
  type RoiWriteOutcome,
} from "@multica/core/content/feedback-learning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { SettingsCard, SettingsRow, SettingsSection } from "@multica/views/settings/layout";
import {
  ChoiceSelect,
  DuplicateRow,
  OptionSelect,
  ReadOnlyValue,
  SaveFeedback,
  StateRow,
  evidenceLabel,
  formatTime,
  joinList,
  nowLocalInput,
  optionName,
  platformLabel,
  roleLabel,
  sourceLabel,
  toInstant,
  toLocalInput,
  type RoiFocus,
  type RoiOptions,
  type Translate,
} from "./shared";

// The lead block (specs/034 PR 5, T088; U-08 to U-13).
//
// A lead carries a pseudonym, never a real name or contact detail; the page
// has no field for one. A touch is EVIDENCE of how the customer came - which
// of R-061's six kinds, and what it points at. Which touches a deal is
// credited to is a separate judgement, made on the deal (deals.tsx), and
// writing it changes no touch here.

function leadName(t: Translate, lead: RoiLead): string {
  return lead.customerRef.trim() || t(($) => $.contentRoiReview.leads.unnamed, { id: lead.leadId });
}

export function leadOptionLabel(t: Translate, leads: RoiLead[], leadId: string): string {
  if (leadId === "") return "";
  const lead = leads.find((item) => item.leadId === leadId);
  return lead ? leadName(t, lead) : leadId;
}

export function LeadsBlock({ wsId, options, focus }: { wsId: string; options: RoiOptions; focus: RoiFocus | null }) {
  const { t } = useT("common");
  const leads = useRoiLeads(wsId, { includeInactive: true });
  const [selectedId, setSelectedId] = useState(focus?.kind === "lead" ? focus.id : "");
  const list = [...(leads.data ?? [])].sort((a, b) => b.firstSeenAt.localeCompare(a.firstSeenAt));
  const stages = [...new Set(list.map((lead) => lead.stage.trim()).filter((stage) => stage !== ""))].sort();

  return (
    <>
      <SettingsSection title={t(($) => $.contentRoiReview.leads.listTitle)}>
        <SettingsCard>
          {leads.isLoading ? (
            <StateRow label={t(($) => $.contentRoiReview.loading)} />
          ) : leads.isError ? (
            <StateRow label={t(($) => $.contentRoiReview.loadFailed)} />
          ) : list.length === 0 ? (
            <StateRow label={t(($) => $.contentRoiReview.leads.listEmpty)} />
          ) : (
            list.map((lead) => (
              <SettingsRow
                key={lead.leadId}
                label={
                  <button
                    type="button"
                    onClick={() => setSelectedId(lead.leadId)}
                    data-active={lead.leadId === selectedId}
                    className="break-words text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {leadName(t, lead)}
                  </button>
                }
                description={[
                  lead.stage,
                  lead.qualified ? t(($) => $.contentRoiReview.leads.qualifiedShort) : "",
                  formatTime(lead.firstSeenAt),
                  lead.mergedInto
                    ? t(($) => $.contentRoiReview.leads.mergedInto, { lead: leadOptionLabel(t, list, lead.mergedInto) })
                    : "",
                  lead.voided ? t(($) => $.contentRoiReview.voided) : "",
                  t(($) => $.contentRoiReview.revisionNo, { revision: lead.revision }),
                ]
                  .filter((part) => part !== "")
                  .join(" · ")}
                size="select-wide"
              >
                <span className="text-caption text-muted-foreground">{lead.leadId}</span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      <LeadCreateSection wsId={wsId} stages={stages} onSaved={setSelectedId} />

      {selectedId !== "" ? (
        <LeadDetail
          key={selectedId}
          wsId={wsId}
          leadId={selectedId}
          leads={list}
          stages={stages}
          options={options}
          focusRevision={focus?.kind === "lead" && focus.id === selectedId ? focus.revision : null}
        />
      ) : null}
    </>
  );
}

// ---------------------------------------------------------------------------
// The lead form.

interface LeadDraft {
  customerRef: string;
  stage: string;
  qualified: boolean;
  firstSeenAt: string;
  note: string;
}

function emptyLeadDraft(): LeadDraft {
  return { customerRef: "", stage: "", qualified: false, firstSeenAt: nowLocalInput(), note: "" };
}

function draftFromLead(lead: RoiLead): LeadDraft {
  return {
    customerRef: lead.customerRef,
    stage: lead.stage,
    qualified: lead.qualified,
    firstSeenAt: toLocalInput(lead.firstSeenAt),
    note: "",
  };
}

function leadBody(draft: LeadDraft, notDuplicateOf: string[]): Record<string, unknown> {
  const body: Record<string, unknown> = {
    customer_ref: draft.customerRef.trim(),
    stage: draft.stage.trim(),
    qualified: draft.qualified,
    first_seen_at: toInstant(draft.firstSeenAt),
    note: draft.note,
  };
  if (notDuplicateOf.length > 0) body.not_duplicate_of = notDuplicateOf;
  return body;
}

function LeadFormRows({
  draft,
  onChange,
  disabled,
  stages,
}: {
  draft: LeadDraft;
  onChange: (next: LeadDraft) => void;
  disabled: boolean;
  stages: string[];
}) {
  const { t } = useT("common");
  const stageListId = useId();
  const edit = (patch: Partial<LeadDraft>) => onChange({ ...draft, ...patch });
  return (
    <>
      <SettingsRow
        label={t(($) => $.contentRoiReview.leads.customerRef)}
        description={t(($) => $.contentRoiReview.leads.customerRefHint)}
        size="text"
      >
        <Input
          value={draft.customerRef}
          onChange={(event) => edit({ customerRef: event.target.value })}
          placeholder={t(($) => $.contentRoiReview.leads.customerRefPlaceholder)}
          aria-label={t(($) => $.contentRoiReview.leads.customerRef)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow
        label={t(($) => $.contentRoiReview.leads.stage)}
        description={t(($) => $.contentRoiReview.leads.stageHint)}
        size="text"
      >
        <Input
          value={draft.stage}
          list={stageListId}
          onChange={(event) => edit({ stage: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.leads.stage)}
          disabled={disabled}
        />
        <datalist id={stageListId}>
          {stages.map((stage) => (
            <option key={stage} value={stage} />
          ))}
        </datalist>
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.leads.qualified)}>
        <Checkbox
          checked={draft.qualified}
          onCheckedChange={(value) => edit({ qualified: value === true })}
          aria-label={t(($) => $.contentRoiReview.leads.qualified)}
          disabled={disabled}
        />
      </SettingsRow>
      <SettingsRow label={t(($) => $.contentRoiReview.leads.firstSeenAt)} size="text">
        <Input
          type="datetime-local"
          value={draft.firstSeenAt}
          onChange={(event) => edit({ firstSeenAt: event.target.value })}
          aria-label={t(($) => $.contentRoiReview.leads.firstSeenAt)}
          disabled={disabled}
        />
      </SettingsRow>
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

function useSave<T>(write: { mutate: (vars: { path: string; body: Record<string, unknown> }, opts: { onSuccess: (value: T) => void; onError: (error: Error) => void }) => void }) {
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

function LeadCreateSection({
  wsId,
  stages,
  onSaved,
}: {
  wsId: string;
  stages: string[];
  onSaved: (leadId: string) => void;
}) {
  const { t } = useT("common");
  const write = useWriteRoiLead(wsId);
  const [draft, setDraft] = useState<LeadDraft>(emptyLeadDraft);
  const { outcome, confirmed, setConfirmed, duplicateOf, save } = useSave<RoiLead | null>(write);
  const missing = draft.firstSeenAt === "";
  const blocked = outcome?.kind === "duplicate" && !confirmed;

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.leads.createTitle)}
      description={t(($) => $.contentRoiReview.leads.createDescription)}
    >
      <SettingsCard>
        <LeadFormRows draft={draft} onChange={setDraft} disabled={write.isPending} stages={stages} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow
          label={t(($) => $.contentRoiReview.save)}
          description={
            missing
              ? t(($) => $.contentRoiReview.missingField, { field: t(($) => $.contentRoiReview.leads.firstSeenAt) })
              : undefined
          }
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button
              onClick={() =>
                save("leads", leadBody(draft, duplicateOf), (lead) => {
                  setDraft(emptyLeadDraft());
                  if (lead) onSaved(lead.leadId);
                })
              }
              disabled={write.isPending || missing || blocked}
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
// One lead: revisions, a new revision, void, merge, touches.

function LeadDetail({
  wsId,
  leadId,
  leads,
  stages,
  options,
  focusRevision,
}: {
  wsId: string;
  leadId: string;
  leads: RoiLead[];
  stages: string[];
  options: RoiOptions;
  focusRevision: number | null;
}) {
  const { t } = useT("common");
  const lead = useRoiLead(wsId, leadId);
  const detail = lead.data;

  if (lead.isLoading || !detail) {
    return (
      <SettingsSection title={t(($) => $.contentRoiReview.leads.detailTitle)}>
        <SettingsCard>
          <StateRow label={lead.isLoading ? t(($) => $.contentRoiReview.loading) : t(($) => $.contentRoiReview.notFound)} />
        </SettingsCard>
      </SettingsSection>
    );
  }
  const revisions = [...detail.revisions].sort((a, b) => b.revision - a.revision);

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentRoiReview.leads.detailTitle)}
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
                revision.qualified ? t(($) => $.contentRoiReview.leads.qualifiedShort) : "",
                formatTime(revision.firstSeenAt),
                revision.mergedInto
                  ? t(($) => $.contentRoiReview.leads.mergedInto, { lead: leadOptionLabel(t, leads, revision.mergedInto) })
                  : "",
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
                <ReadOnlyValue value={revision.customerRef} />
                <ReadOnlyValue value={revision.stage} muted />
              </div>
            </SettingsRow>
          ))}
        </SettingsCard>
      </SettingsSection>
      <LeadRevisionSection key={`revise-${detail.current.revision}`} wsId={wsId} current={detail.current} stages={stages} />
      <LeadMergeSection key={`merge-${detail.current.revision}`} wsId={wsId} current={detail.current} leads={leads} />
      <TouchesSection wsId={wsId} leadId={leadId} touches={detail.touches.map((touch) => touch.current)} options={options} />
    </>
  );
}

function LeadRevisionSection({ wsId, current, stages }: { wsId: string; current: RoiLead; stages: string[] }) {
  const { t } = useT("common");
  const write = useWriteRoiLead(wsId);
  const [draft, setDraft] = useState<LeadDraft>(() => draftFromLead(current));
  const { outcome, confirmed, setConfirmed, duplicateOf, save } = useSave<RoiLead | null>(write);
  const path = roiPath("leads", current.leadId, "revisions");
  const blocked = outcome?.kind === "duplicate" && !confirmed;

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.leads.reviseTitle)}
      description={t(($) => $.contentRoiReview.reviseDescription, { revision: current.revision })}
    >
      <SettingsCard>
        <LeadFormRows draft={draft} onChange={setDraft} disabled={write.isPending} stages={stages} />
        {outcome?.kind === "duplicate" ? (
          <DuplicateRow matches={outcome.matches} confirmed={confirmed} onConfirm={setConfirmed} disabled={write.isPending} />
        ) : null}
        <SettingsRow label={t(($) => $.contentRoiReview.saveRevision)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            <Button
              variant="outline"
              onClick={() =>
                save(
                  path,
                  { ...leadBody(draftFromLead(current), []), base_revision: current.revision, voided: !current.voided },
                  () => undefined,
                )
              }
              disabled={write.isPending}
            >
              {current.voided ? t(($) => $.contentRoiReview.restore) : t(($) => $.contentRoiReview.void)}
            </Button>
            <Button
              onClick={() =>
                save(
                  path,
                  { ...leadBody(draft, duplicateOf), base_revision: current.revision, voided: current.voided },
                  () => undefined,
                )
              }
              disabled={write.isPending || draft.firstSeenAt === "" || blocked}
            >
              {t(($) => $.contentRoiReview.saveRevision)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function LeadMergeSection({ wsId, current, leads }: { wsId: string; current: RoiLead; leads: RoiLead[] }) {
  const { t } = useT("common");
  const write = useWriteRoiLead(wsId);
  const { outcome, save } = useSave<RoiLead | null>(write);
  const [target, setTarget] = useState("");
  const [note, setNote] = useState("");
  const candidates = leads.filter(
    (lead) => lead.leadId !== current.leadId && !lead.voided && lead.mergedInto === "",
  );
  const path = roiPath("leads", current.leadId, "merge");

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.leads.mergeTitle)}
      description={t(($) => $.contentRoiReview.leads.mergeDescription)}
    >
      <SettingsCard>
        {current.mergedInto !== "" ? (
          <SettingsRow
            label={t(($) => $.contentRoiReview.leads.mergedInto, { lead: leadOptionLabel(t, leads, current.mergedInto) })}
          >
            <div className="flex items-center gap-3">
              <SaveFeedback outcome={outcome} pending={write.isPending} />
              <Button
                variant="outline"
                onClick={() => save(path, { target_lead_id: "", note, base_revision: current.revision }, () => undefined)}
                disabled={write.isPending}
              >
                {t(($) => $.contentRoiReview.leads.unmerge)}
              </Button>
            </div>
          </SettingsRow>
        ) : (
          <>
            <SettingsRow label={t(($) => $.contentRoiReview.leads.mergeTarget)} size="select-wide">
              <ChoiceSelect
                items={[
                  { value: "__none__", label: t(($) => $.contentRoiReview.leads.chooseLead) },
                  ...candidates.map((lead) => ({ value: lead.leadId, label: leadName(t, lead) })),
                ]}
                value={target === "" ? "__none__" : target}
                onChange={(value) => setTarget(value === "__none__" ? "" : value)}
                label={t(($) => $.contentRoiReview.leads.mergeTarget)}
                disabled={write.isPending}
              />
            </SettingsRow>
            <SettingsRow label={t(($) => $.contentRoiReview.leads.mergeReason)} size="text" align="start">
              <Textarea
                value={note}
                onChange={(event) => setNote(event.target.value)}
                aria-label={t(($) => $.contentRoiReview.leads.mergeReason)}
                rows={2}
                disabled={write.isPending}
              />
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.contentRoiReview.leads.merge)}
              description={
                target === "" || note.trim() === ""
                  ? t(($) => $.contentRoiReview.leads.mergeNeeds)
                  : undefined
              }
            >
              <div className="flex items-center gap-3">
                <SaveFeedback outcome={outcome} pending={write.isPending} />
                <Button
                  onClick={() =>
                    save(path, { target_lead_id: target, note: note.trim(), base_revision: current.revision }, () => undefined)
                  }
                  disabled={write.isPending || target === "" || note.trim() === ""}
                >
                  {t(($) => $.contentRoiReview.leads.merge)}
                </Button>
              </div>
            </SettingsRow>
          </>
        )}
      </SettingsCard>
    </SettingsSection>
  );
}

// ---------------------------------------------------------------------------
// Touches: evidence.

interface TouchDraft {
  evidenceType: string;
  platform: string;
  accountId: string;
  workId: string;
  publicationRecordId: string;
  role: string;
  paid: boolean;
  occurredAt: string;
  evidenceNote: string;
  note: string;
}

function emptyTouchDraft(): TouchDraft {
  return {
    evidenceType: "customer_statement",
    platform: "",
    accountId: "",
    workId: "",
    publicationRecordId: "",
    role: "first_touch",
    paid: false,
    occurredAt: nowLocalInput(),
    evidenceNote: "",
    note: "",
  };
}

function draftFromTouch(touch: RoiTouch): TouchDraft {
  return {
    evidenceType: touch.evidenceType,
    platform: touch.platform,
    accountId: touch.accountId,
    workId: touch.workId,
    publicationRecordId: touch.publicationRecordId,
    role: touch.role,
    paid: touch.paid,
    occurredAt: toLocalInput(touch.occurredAt),
    evidenceNote: touch.evidenceNote,
    note: "",
  };
}

function touchBody(draft: TouchDraft): Record<string, unknown> {
  return {
    evidence_type: draft.evidenceType,
    platform: draft.platform,
    account_id: draft.accountId,
    work_id: draft.workId,
    publication_record_id: draft.publicationRecordId.trim(),
    role: draft.role,
    paid: draft.paid,
    occurred_at: toInstant(draft.occurredAt),
    evidence_note: draft.evidenceNote,
    note: draft.note,
  };
}

function touchFieldLabel(t: Translate, field: "work_id" | "account_id" | "platform"): string {
  switch (field) {
    case "work_id":
      return t(($) => $.contentRoiReview.leads.touchContent);
    case "account_id":
      return t(($) => $.contentRoiReview.account);
    default:
      return t(($) => $.contentRoiReview.leads.platform);
  }
}

export function touchSummary(t: Translate, touch: RoiTouch, options: RoiOptions): string {
  return [
    evidenceLabel(t, touch.evidenceType),
    touch.platform ? platformLabel(t, touch.platform) : "",
    optionName(options.accounts, touch.accountId),
    optionName(options.works, touch.workId),
    touch.publicationRecordId,
    roleLabel(t, touch.role),
    touch.paid ? t(($) => $.contentRoiReview.leads.paidShort) : "",
    formatTime(touch.occurredAt),
  ]
    .filter((part) => part !== "")
    .join(" · ");
}

function TouchesSection({
  wsId,
  leadId,
  touches,
  options,
}: {
  wsId: string;
  leadId: string;
  touches: RoiTouch[];
  options: RoiOptions;
}) {
  const { t } = useT("common");
  const write = useWriteRoiTouch(wsId);
  const { outcome, save } = useSave<RoiTouch | null>(write);
  const [editing, setEditing] = useState<RoiTouch | null>(null);
  const [draft, setDraft] = useState<TouchDraft>(emptyTouchDraft);
  const problems = roiTouchProblems(draft);
  const rules = roiTouchRules(draft.evidenceType);
  const edit = (patch: Partial<TouchDraft>) => setDraft({ ...draft, ...patch });
  const evidenceItems = ROI_EVIDENCE_TYPES.map((value) => ({ value, label: evidenceLabel(t, value) }));
  const roleItems = ROI_TOUCH_ROLES.map((value) => ({ value, label: roleLabel(t, value) }));
  const platformItems = [
    { value: "__none__", label: t(($) => $.contentRoiReview.blank) },
    ...ROI_TOUCH_PLATFORMS.map((value) => ({ value, label: platformLabel(t, value) })),
  ];
  const sorted = [...touches].sort((a, b) => a.occurredAt.localeCompare(b.occurredAt));

  const submit = () => {
    const body = touchBody(draft);
    if (editing) {
      save(
        roiPath("leads", leadId, "touches", editing.touchId, "revisions"),
        { ...body, base_revision: editing.revision, voided: editing.voided },
        () => {
          setEditing(null);
          setDraft(emptyTouchDraft());
        },
      );
    } else {
      save(roiPath("leads", leadId, "touches"), body, () => setDraft(emptyTouchDraft()));
    }
  };

  const toggleVoid = (touch: RoiTouch) =>
    save(
      roiPath("leads", leadId, "touches", touch.touchId, "revisions"),
      { ...touchBody(draftFromTouch(touch)), base_revision: touch.revision, voided: !touch.voided },
      () => undefined,
    );

  return (
    <SettingsSection
      title={t(($) => $.contentRoiReview.leads.touchesTitle)}
      description={t(($) => $.contentRoiReview.leads.touchesDescription)}
    >
      <SettingsCard>
        {sorted.length === 0 ? (
          <StateRow label={t(($) => $.contentRoiReview.leads.touchesEmpty)} />
        ) : (
          sorted.map((touch) => (
            <SettingsRow
              key={touch.touchId}
              label={
                <span>
                  {touchSummary(t, touch, options)}
                  {touch.voided ? ` · ${t(($) => $.contentRoiReview.voided)}` : ""}
                </span>
              }
              description={[
                touch.leadId !== leadId ? t(($) => $.contentRoiReview.leads.fromMerged, { lead: touch.leadId }) : "",
                touch.evidenceNote,
                t(($) => $.contentRoiReview.revisionNo, { revision: touch.revision }),
                touch.touchId,
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
                    setEditing(touch);
                    setDraft(draftFromTouch(touch));
                  }}
                  disabled={write.isPending || touch.leadId !== leadId}
                >
                  {t(($) => $.contentRoiReview.edit)}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => toggleVoid(touch)}
                  disabled={write.isPending || touch.leadId !== leadId}
                >
                  {touch.voided ? t(($) => $.contentRoiReview.restore) : t(($) => $.contentRoiReview.void)}
                </Button>
              </div>
            </SettingsRow>
          ))
        )}

        <SettingsRow
          label={
            editing
              ? t(($) => $.contentRoiReview.leads.editTouch, { revision: editing.revision })
              : t(($) => $.contentRoiReview.leads.addTouch)
          }
          description={t(($) => $.contentRoiReview.leads.evidenceHint)}
          size="select-wide"
        >
          <ChoiceSelect
            items={evidenceItems}
            value={draft.evidenceType}
            onChange={(evidenceType) => setDraft(roiTouchForEvidence(draft, evidenceType))}
            label={t(($) => $.contentRoiReview.leads.evidenceType)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.leads.platform)} size="select">
          <ChoiceSelect
            items={platformItems}
            value={draft.platform === "" ? "__none__" : draft.platform}
            onChange={(value) => edit({ platform: value === "__none__" ? "" : value })}
            label={t(($) => $.contentRoiReview.leads.platform)}
            disabled={write.isPending || rules.platform === "forbidden"}
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
            disabled={write.isPending || rules.account === "forbidden"}
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
            disabled={write.isPending || rules.content === "forbidden"}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.leads.publicationRecord)} size="text">
          <Input
            value={draft.publicationRecordId}
            onChange={(event) => edit({ publicationRecordId: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.leads.publicationRecord)}
            disabled={write.isPending || rules.content === "forbidden"}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.leads.role)} size="select">
          <ChoiceSelect
            items={roleItems}
            value={draft.role}
            onChange={(role) => edit({ role })}
            label={t(($) => $.contentRoiReview.leads.role)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.leads.paid)} description={t(($) => $.contentRoiReview.leads.paidHint)}>
          <Checkbox
            checked={draft.paid}
            onCheckedChange={(value) => edit({ paid: value === true })}
            aria-label={t(($) => $.contentRoiReview.leads.paid)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.leads.occurredAt)} size="text">
          <Input
            type="datetime-local"
            value={draft.occurredAt}
            onChange={(event) => edit({ occurredAt: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.leads.occurredAt)}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentRoiReview.evidenceNote)} size="text" align="start">
          <Textarea
            value={draft.evidenceNote}
            onChange={(event) => edit({ evidenceNote: event.target.value })}
            aria-label={t(($) => $.contentRoiReview.evidenceNote)}
            rows={2}
            disabled={write.isPending}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentRoiReview.save)}
          description={
            problems.length > 0
              ? t(($) => $.contentRoiReview.leads.touchProblems, {
                  fields: joinList(t, problems.map((field) => touchFieldLabel(t, field))),
                })
              : draft.occurredAt === ""
                ? t(($) => $.contentRoiReview.missingField, { field: t(($) => $.contentRoiReview.leads.occurredAt) })
                : undefined
          }
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={write.isPending} />
            {editing ? (
              <Button
                variant="ghost"
                onClick={() => {
                  setEditing(null);
                  setDraft(emptyTouchDraft());
                }}
                disabled={write.isPending}
              >
                {t(($) => $.contentRoiReview.cancel)}
              </Button>
            ) : null}
            <Button onClick={submit} disabled={write.isPending || problems.length > 0 || draft.occurredAt === ""}>
              {t(($) => $.contentRoiReview.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
