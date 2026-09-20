"use client";

import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  SAVED,
  saveOutcome,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import {
  TOPIC_ACTIONS,
  TOPIC_DECISION_REASONS,
  briefDraftDiffers,
  briefDraftFromRevision,
  briefDraftToInput,
  canAppendBrief,
  decisionInput,
  emptyTopicCardDraft,
  formatChannels,
  isTopicCardDraftReady,
  latestRevision,
  sortedRevisions,
  topicCardDraftToInput,
  topicStatusKey,
  useActOnContentTopic,
  useAppendContentBrief,
  useContentBrief,
  useContentBriefs,
  useContentTopic,
  useContentTopics,
  useCreateContentTopic,
  type BriefDraft,
  type BriefRevision,
  type TopicAction,
  type TopicCard,
  type TopicCardDraft,
} from "@multica/core/content/topic-planning";
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
  SettingsContent,
  SettingsRow,
  SettingsSaveState,
  SettingsSection,
  SettingsTab,
  type SettingsSaveStatus,
} from "@multica/views/settings/layout";
import { PageHeader } from "@multica/views/layout/page-header";

// Topic cards and their frozen briefs (EP-04a, PR 2).
//
// The composition is the account settings page's, which is the composition of
// settings/components/workspace-tab.tsx: SettingsCard rows, one control per
// row, a save state beside the button. Nothing here is a new control and no
// style is tuned - this slice of the SOP work earns nothing from a second
// visual language, and the manual checklist asks for the opposite: that this
// page looks like the settings pages it sits beside.
//
// What the page does NOT do is as deliberate as what it does: no candidate
// topics are generated, no must-use/excluded material list exists, and no file
// is read. Those are later cards; the rows that name them say "not available
// yet" rather than pretending to be disabled features that might work.

type Translate = ReturnType<typeof useT<"common">>["t"];

function saveStatusOf(
  outcome: SaveOutcome | null,
  pending: boolean,
): SettingsSaveStatus {
  if (pending) return "saving";
  if (!outcome) return "idle";
  return outcome.kind === "saved" ? "saved" : "error";
}

function SaveFeedback({
  outcome,
  pending,
}: {
  outcome: SaveOutcome | null;
  pending: boolean;
}) {
  const { t } = useT("common");
  // The topic endpoints have no 409, so a conflict outcome cannot arrive here;
  // it would read as the same failure sentence if a future one did.
  const errorLabel =
    outcome?.kind === "failed" && outcome.detail.nextAction
      ? `${t(($) => $.contentTopics.failed)} · ${t(($) => $.contentTopics.nextAction, { action: outcome.detail.nextAction })}`
      : t(($) => $.contentTopics.failed);
  return (
    <SettingsSaveState
      status={saveStatusOf(outcome, pending)}
      savingLabel={t(($) => $.contentTopics.saving)}
      savedLabel={t(($) => $.contentTopics.saved)}
      errorLabel={errorLabel}
    />
  );
}

function statusLabel(t: Translate, status: string): string {
  switch (topicStatusKey(status)) {
    case "draft":
      return t(($) => $.contentTopics.status.draft);
    case "started":
      return t(($) => $.contentTopics.status.started);
    case "saved":
      return t(($) => $.contentTopics.status.saved);
    case "deferred":
      return t(($) => $.contentTopics.status.deferred);
    case "dropped":
      return t(($) => $.contentTopics.status.dropped);
    default:
      return t(($) => $.contentTopics.status.unknown);
  }
}

function actionLabel(t: Translate, action: TopicAction): string {
  switch (action) {
    case "start":
      return t(($) => $.contentTopics.actions.start);
    case "save":
      return t(($) => $.contentTopics.actions.save);
    case "defer":
      return t(($) => $.contentTopics.actions.defer);
    default:
      return t(($) => $.contentTopics.actions.drop);
  }
}

function reasonLabel(t: Translate, reason: string): string {
  switch (reason) {
    case "no_evidence":
      return t(($) => $.contentTopics.reasons.no_evidence);
    case "no_timing":
      return t(($) => $.contentTopics.reasons.no_timing);
    case "duplicate":
      return t(($) => $.contentTopics.reasons.duplicate);
    case "low_fit":
      return t(($) => $.contentTopics.reasons.low_fit);
    case "no_capacity":
      return t(($) => $.contentTopics.reasons.no_capacity);
    // A reason stored by a newer build still has to render as something, and
    // its stored code is a better label than a blank cell.
    default:
      return reason;
  }
}

/** A value the person typed, shown back read-only. */
function ReadOnlyValue({ value }: { value: string }) {
  const { t } = useT("common");
  return (
    <span className="block whitespace-pre-wrap break-words text-body text-muted-foreground">
      {value.trim() === "" ? t(($) => $.contentTopics.blank) : value}
    </span>
  );
}

export interface TopicPlanningPageProps {
  wsId: string;
}

export function TopicPlanningPage({ wsId }: TopicPlanningPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">
          {t(($) => $.contentTopics.title)}
        </span>
      </PageHeader>
      <TopicPlanningContent wsId={wsId} />
    </>
  );
}

function TopicPlanningContent({ wsId }: TopicPlanningPageProps) {
  const { t } = useT("common");
  const topics = useContentTopics(wsId);
  const [selectedId, setSelectedId] = useState("");

  const list = topics.data ?? [];
  // An id that is no longer in the list - another brand, or a card removed
  // elsewhere - must not stay selected, or the panel would edit something that
  // is not there.
  const selected = list.find((card) => card.topicCardId === selectedId) ?? null;

  return (
    <SettingsContent>
      <SettingsTab
        title={t(($) => $.contentTopics.title)}
        description={t(($) => $.contentTopics.description)}
      >
        <SettingsSection title={t(($) => $.contentTopics.listTitle)}>
          <SettingsCard>
            {topics.isError ? (
              <SettingsRow label={t(($) => $.contentTopics.loadFailed)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : list.length === 0 ? (
              <SettingsRow label={t(($) => $.contentTopics.empty)}>
                <span className="text-caption text-muted-foreground" />
              </SettingsRow>
            ) : (
              list.map((card) => (
                <SettingsRow
                  key={card.topicCardId}
                  label={
                    <button
                      type="button"
                      onClick={() => setSelectedId(card.topicCardId)}
                      data-active={card.topicCardId === selectedId}
                      // Weight carries the selection, so a hovered selected row
                      // still reads as selected.
                      className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                    >
                      {card.audienceProblemJudgment || card.topicCardId}
                    </button>
                  }
                  description={`${statusLabel(t, card.status)} · ${formatChannels(card.channels)}`}
                >
                  <span className="text-caption text-muted-foreground" />
                </SettingsRow>
              ))
            )}
          </SettingsCard>
        </SettingsSection>

        <CreateTopicSection wsId={wsId} onCreated={setSelectedId} />

        <NotAvailableYetSection />

        {selected ? (
          <TopicCardPanel
            key={selected.topicCardId}
            wsId={wsId}
            card={selected}
          />
        ) : null}
      </SettingsTab>
    </SettingsContent>
  );
}

// The three inputs EP-04a deliberately does not build. Named rather than
// hidden: someone reading this page should be able to tell "not built yet"
// from "built and broken", and a missing row says neither.
function NotAvailableYetSection() {
  const { t } = useT("common");
  const rows = [
    t(($) => $.contentTopics.unavailable.candidates),
    t(($) => $.contentTopics.unavailable.mustUse),
    t(($) => $.contentTopics.unavailable.files),
  ];
  return (
    <SettingsSection
      title={t(($) => $.contentTopics.unavailable.title)}
      description={t(($) => $.contentTopics.unavailable.description)}
    >
      <SettingsCard>
        {rows.map((label) => (
          <SettingsRow key={label} label={label}>
            <span className="text-caption text-muted-foreground">
              {t(($) => $.contentTopics.unavailable.badge)}
            </span>
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function CreateTopicSection({
  wsId,
  onCreated,
}: {
  wsId: string;
  onCreated: (topicCardId: string) => void;
}) {
  const { t } = useT("common");
  const create = useCreateContentTopic(wsId);
  const [draft, setDraft] = useState<TopicCardDraft>(emptyTopicCardDraft);
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const edit = (patch: Partial<TopicCardDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const submit = () => {
    setOutcome(null);
    create.mutate(topicCardDraftToInput(draft), {
      onSuccess: (card) => {
        setOutcome(SAVED);
        setDraft(emptyTopicCardDraft);
        // Waits for the server rather than guessing an id: the selection moves
        // to a real card or not at all.
        if (card.topicCardId) onCreated(card.topicCardId);
      },
      onError: (error) => setOutcome(saveOutcome(error)),
    });
  };

  const items: {
    key: keyof TopicCardDraft;
    label: string;
    placeholder: string;
    rows?: number;
  }[] = [
    {
      key: "audienceProblemJudgment",
      label: t(($) => $.contentTopics.fields.audienceProblemJudgment),
      placeholder: t(
        ($) => $.contentTopics.placeholders.audienceProblemJudgment,
      ),
      rows: 3,
    },
    {
      key: "ipFit",
      label: t(($) => $.contentTopics.fields.ipFit),
      placeholder: t(($) => $.contentTopics.placeholders.ipFit),
      rows: 2,
    },
    {
      key: "timing",
      label: t(($) => $.contentTopics.fields.timing),
      placeholder: t(($) => $.contentTopics.placeholders.timing),
      rows: 2,
    },
    {
      key: "existingContentRelation",
      label: t(($) => $.contentTopics.fields.existingContentRelation),
      placeholder: t(
        ($) => $.contentTopics.placeholders.existingContentRelation,
      ),
      rows: 2,
    },
    {
      key: "evidenceGapsAndInvestment",
      label: t(($) => $.contentTopics.fields.evidenceGapsAndInvestment),
      placeholder: t(
        ($) => $.contentTopics.placeholders.evidenceGapsAndInvestment,
      ),
      rows: 2,
    },
  ];

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.createTitle)}
      description={t(($) => $.contentTopics.createDescription)}
    >
      <SettingsCard>
        {items.map((item) => (
          <SettingsRow
            key={item.key}
            label={item.label}
            size="text"
            align="start"
          >
            <Textarea
              value={draft[item.key]}
              onChange={(event) => edit({ [item.key]: event.target.value })}
              placeholder={item.placeholder}
              rows={item.rows}
            />
          </SettingsRow>
        ))}
        <SettingsRow
          label={t(($) => $.contentTopics.fields.channels)}
          description={t(($) => $.contentTopics.channelsHint)}
          size="text"
        >
          <Input
            value={draft.channels}
            onChange={(event) => edit({ channels: event.target.value })}
            placeholder={t(($) => $.contentTopics.placeholders.channels)}
          />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentTopics.fields.recommendedAction)}
          size="text"
        >
          <Input
            value={draft.recommendedAction}
            onChange={(event) =>
              edit({ recommendedAction: event.target.value })
            }
            placeholder={t(
              ($) => $.contentTopics.placeholders.recommendedAction,
            )}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentTopics.create)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={create.isPending} />
            <Button
              onClick={submit}
              disabled={create.isPending || !isTopicCardDraftReady(draft)}
            >
              {t(($) => $.contentTopics.create)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function TopicCardPanel({ wsId, card }: { wsId: string; card: TopicCard }) {
  // The detail query is what the four actions refresh, so the panel reads it
  // rather than the row it was selected from.
  const detail = useContentTopic(wsId, card.topicCardId);
  const current = detail.data ?? card;
  return (
    <>
      <TopicCardDetail card={current} />
      <DecisionSection wsId={wsId} card={current} />
      {canAppendBrief(current) ? (
        <BriefSection wsId={wsId} card={current} />
      ) : null}
    </>
  );
}

function TopicCardDetail({ card }: { card: TopicCard }) {
  const { t } = useT("common");
  const items: { label: string; value: string }[] = [
    {
      label: t(($) => $.contentTopics.fields.audienceProblemJudgment),
      value: card.audienceProblemJudgment,
    },
    { label: t(($) => $.contentTopics.fields.ipFit), value: card.ipFit },
    { label: t(($) => $.contentTopics.fields.timing), value: card.timing },
    {
      label: t(($) => $.contentTopics.fields.existingContentRelation),
      value: card.existingContentRelation,
    },
    {
      label: t(($) => $.contentTopics.fields.evidenceGapsAndInvestment),
      value: card.evidenceGapsAndInvestment,
    },
    {
      label: t(($) => $.contentTopics.fields.channels),
      value: formatChannels(card.channels),
    },
    {
      label: t(($) => $.contentTopics.fields.recommendedAction),
      value: card.recommendedAction,
    },
  ];
  return (
    <SettingsSection title={t(($) => $.contentTopics.detailTitle)}>
      <SettingsCard>
        <SettingsRow label={t(($) => $.contentTopics.statusLabel)}>
          <span className="text-body text-muted-foreground">
            {statusLabel(t, card.status)}
          </span>
        </SettingsRow>
        {items.map((item) => (
          <SettingsRow
            key={item.label}
            label={item.label}
            size="text"
            align="start"
          >
            <ReadOnlyValue value={item.value} />
          </SettingsRow>
        ))}
        {card.decisionReason || card.decisionNote ? (
          <>
            <SettingsRow
              label={t(($) => $.contentTopics.decisionReason)}
              size="text"
              align="start"
            >
              <ReadOnlyValue
                value={
                  card.decisionReason ? reasonLabel(t, card.decisionReason) : ""
                }
              />
            </SettingsRow>
            <SettingsRow
              label={t(($) => $.contentTopics.decisionNote)}
              size="text"
              align="start"
            >
              <ReadOnlyValue value={card.decisionNote} />
            </SettingsRow>
          </>
        ) : null}
      </SettingsCard>
    </SettingsSection>
  );
}

function DecisionSection({ wsId, card }: { wsId: string; card: TopicCard }) {
  const { t } = useT("common");
  const act = useActOnContentTopic(wsId);
  const [reason, setReason] = useState("");
  const [note, setNote] = useState("");
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const run = (action: TopicAction) => {
    setOutcome(null);
    act.mutate(
      {
        topicCardId: card.topicCardId,
        body: decisionInput(action, reason, note),
      },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  const reasonItems = TOPIC_DECISION_REASONS.map((value) => ({
    value,
    label: reasonLabel(t, value),
  }));

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.decisionTitle)}
      description={t(($) => $.contentTopics.decisionDescription)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentTopics.decisionReason)}
          description={t(($) => $.contentTopics.decisionReasonHint)}
          size="select-wide"
        >
          <Select
            items={reasonItems}
            value={reason}
            onValueChange={(value) => setReason(value ?? "")}
          >
            <SelectTrigger
              aria-label={t(($) => $.contentTopics.decisionReason)}
            >
              <SelectValue
                placeholder={t(
                  ($) => $.contentTopics.decisionReasonPlaceholder,
                )}
              />
            </SelectTrigger>
            <SelectContent>
              {TOPIC_DECISION_REASONS.map((value) => (
                <SelectItem key={value} value={value}>
                  {reasonLabel(t, value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentTopics.decisionNote)} size="text">
          <Input
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder={t(($) => $.contentTopics.decisionNotePlaceholder)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentTopics.decisionRun)}>
          <div className="flex flex-wrap items-center gap-3">
            <SaveFeedback outcome={outcome} pending={act.isPending} />
            {TOPIC_ACTIONS.map((action) => (
              <Button
                key={action}
                variant={action === "start" ? "default" : "outline"}
                onClick={() => run(action)}
                disabled={act.isPending}
              >
                {actionLabel(t, action)}
              </Button>
            ))}
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function BriefSection({ wsId, card }: { wsId: string; card: TopicCard }) {
  const { t } = useT("common");
  const briefs = useContentBriefs(wsId, card.topicCardId);
  const revisions = sortedRevisions(briefs.data ?? []);
  const newest = latestRevision(revisions);
  const [viewingId, setViewingId] = useState("");
  // The version being read comes from its own endpoint, by the stable id: the
  // list is a list, and what an older version says is read from the server
  // rather than reconstructed from it.
  const viewing = useContentBrief(wsId, card.topicCardId, viewingId);

  return (
    <>
      <SettingsSection
        title={t(($) => $.contentTopics.briefTitle)}
        description={t(($) => $.contentTopics.briefDescription)}
      >
        <SettingsCard>
          {briefs.isError ? (
            <SettingsRow label={t(($) => $.contentTopics.briefLoadFailed)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : revisions.length === 0 ? (
            <SettingsRow label={t(($) => $.contentTopics.briefEmpty)}>
              <span className="text-caption text-muted-foreground" />
            </SettingsRow>
          ) : (
            revisions.map((revision) => (
              <SettingsRow
                key={revision.briefRevisionId}
                label={
                  <button
                    type="button"
                    onClick={() => setViewingId(revision.briefRevisionId)}
                    data-active={revision.briefRevisionId === viewingId}
                    className="text-left text-body data-[active=true]:font-semibold hover:text-foreground"
                  >
                    {t(($) => $.contentTopics.revision, {
                      number: revision.revision,
                    })}
                  </button>
                }
                description={
                  revision.briefRevisionId === card.startedBriefRevisionId
                    ? t(($) => $.contentTopics.firstRevision)
                    : undefined
                }
              >
                <span className="text-caption text-muted-foreground">
                  {revision.createdAt}
                </span>
              </SettingsRow>
            ))
          )}
        </SettingsCard>
      </SettingsSection>

      {viewingId && viewing.data ? (
        <BriefReadOnly revision={viewing.data} />
      ) : null}

      <AppendBriefSection
        key={newest?.briefRevisionId ?? "none"}
        wsId={wsId}
        card={card}
        newest={newest}
      />
    </>
  );
}

function briefItems(
  t: Translate,
  values: BriefDraft,
): { key: keyof BriefDraft; label: string; value: string }[] {
  return [
    {
      key: "audience",
      label: t(($) => $.contentTopics.brief.audience),
      value: values.audience,
    },
    {
      key: "coreProblem",
      label: t(($) => $.contentTopics.brief.coreProblem),
      value: values.coreProblem,
    },
    {
      key: "claimAndBoundaries",
      label: t(($) => $.contentTopics.brief.claimAndBoundaries),
      value: values.claimAndBoundaries,
    },
    {
      key: "channels",
      label: t(($) => $.contentTopics.brief.channels),
      value: values.channels,
    },
    {
      key: "format",
      label: t(($) => $.contentTopics.brief.format),
      value: values.format,
    },
    {
      key: "structure",
      label: t(($) => $.contentTopics.brief.structure),
      value: values.structure,
    },
    {
      key: "citationRequirements",
      label: t(($) => $.contentTopics.brief.citationRequirements),
      value: values.citationRequirements,
    },
    {
      key: "sourceScope",
      label: t(($) => $.contentTopics.brief.sourceScope),
      value: values.sourceScope,
    },
    {
      key: "deliverable",
      label: t(($) => $.contentTopics.brief.deliverable),
      value: values.deliverable,
    },
    {
      key: "timeLimit",
      label: t(($) => $.contentTopics.brief.timeLimit),
      value: values.timeLimit,
    },
    {
      key: "costLimit",
      label: t(($) => $.contentTopics.brief.costLimit),
      value: values.costLimit,
    },
  ];
}

function BriefReadOnly({ revision }: { revision: BriefRevision }) {
  const { t } = useT("common");
  const values = briefDraftFromRevision(revision);
  return (
    <SettingsSection
      title={t(($) => $.contentTopics.revision, { number: revision.revision })}
      description={t(($) => $.contentTopics.briefReadOnly)}
    >
      <SettingsCard>
        {briefItems(t, values).map((item) => (
          <SettingsRow
            key={item.key}
            label={item.label}
            size="text"
            align="start"
          >
            <ReadOnlyValue value={item.value} />
          </SettingsRow>
        ))}
      </SettingsCard>
    </SettingsSection>
  );
}

function AppendBriefSection({
  wsId,
  card,
  newest,
}: {
  wsId: string;
  card: TopicCard;
  newest: BriefRevision | undefined;
}) {
  const { t } = useT("common");
  const append = useAppendContentBrief(wsId);
  const [draft, setDraft] = useState<BriefDraft>(() =>
    briefDraftFromRevision(newest),
  );
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const edit = (patch: Partial<BriefDraft>) =>
    setDraft((current) => ({ ...current, ...patch }));

  const submit = () => {
    setOutcome(null);
    append.mutate(
      { topicCardId: card.topicCardId, body: briefDraftToInput(draft) },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.appendTitle)}
      description={t(($) => $.contentTopics.appendDescription)}
    >
      <SettingsCard>
        {briefItems(t, draft).map((item) => (
          <SettingsRow
            key={item.key}
            label={item.label}
            description={
              item.key === "channels"
                ? t(($) => $.contentTopics.channelsHint)
                : undefined
            }
            size="text"
            align="start"
          >
            {item.key === "channels" ||
            item.key === "timeLimit" ||
            item.key === "costLimit" ||
            item.key === "format" ? (
              <Input
                value={item.value}
                onChange={(event) => edit({ [item.key]: event.target.value })}
              />
            ) : (
              <Textarea
                value={item.value}
                onChange={(event) => edit({ [item.key]: event.target.value })}
                rows={2}
              />
            )}
          </SettingsRow>
        ))}
        <SettingsRow
          label={t(($) => $.contentTopics.appendRun)}
          description={t(($) => $.contentTopics.appendHint)}
        >
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={append.isPending} />
            {/* Appending an identical version would add a permanent entry to a
                history that says nothing changed. */}
            <Button
              onClick={submit}
              disabled={append.isPending || !briefDraftDiffers(draft, newest)}
            >
              {t(($) => $.contentTopics.appendRun)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}
