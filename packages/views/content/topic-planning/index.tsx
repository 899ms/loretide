"use client";

import type { ReactNode } from "react";
import { useState } from "react";
import { useT } from "@multica/views/i18n";
import {
  SAVED,
  saveOutcome,
  useContentAccounts,
  type SaveOutcome,
} from "@multica/core/content/ip-profile";
import {
  TOPIC_ACCOUNT_FILTER_NONE,
  TOPIC_ACTIONS,
  TOPIC_DECISION_REASONS,
  accountSelectionChanged,
  accountSelectionOf,
  accountSelectionToWire,
  addTopicSource,
  briefDraftDiffers,
  briefDraftFromRevision,
  briefDraftToInput,
  canAppendBrief,
  decisionInput,
  emptyTopicCardDraft,
  formatChannels,
  isTopicCardDraftReady,
  latestRevision,
  removeTopicSource,
  sortedRevisions,
  topicCardDraftToInput,
  topicSourceDraftDiffers,
  topicSourceDraftToInput,
  topicSourceSelectionOf,
  topicStatusKey,
  useActOnContentTopic,
  useAppendContentBrief,
  useContentBrief,
  useContentBriefs,
  useContentTopic,
  useContentTopics,
  useCreateContentTopic,
  editTopicBodyDraft,
  topicBodyDraftChanged,
  topicBodyFieldValue,
  usePatchContentTopicBody,
  useSetContentTopicAccount,
  useSetContentTopicSources,
  type BriefDraft,
  type BriefRevision,
  type TopicAction,
  type TopicBodyKey,
  type TopicBodyPatchInput,
  type TopicCard,
  type TopicCardDraft,
  type TopicSourceDraft,
  type TopicSourceField,
} from "@multica/core/content/topic-planning";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
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
import { StartRunSection } from "./start";

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

/** A source-inbox row supplied by the Web composition adapter. */
export interface TopicSourceCandidate {
  sourceId: string;
  title: string;
  url: string;
  status: string;
}

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

// The accounts this brand publishes under, as select options. An account a
// card points at but that no longer lists is still shown by its id rather than
// as an empty box: the card says something, and hiding it would read as "no
// account chosen".
function useAccountOptions(wsId: string) {
  const accounts = useContentAccounts(wsId);
  return (accounts.data ?? []).map((account) => ({
    value: account.account_id,
    label: account.display_name || account.account_id,
  }));
}

function accountName(
  options: { value: string; label: string }[],
  accountId: string,
): string {
  return (
    options.find((option) => option.value === accountId)?.label ?? accountId
  );
}

/** The account select, shared by the create form, the detail panel and the filter. */
function AccountSelect({
  value,
  options,
  onChange,
  label,
  noneLabel,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  label: string;
  noneLabel: string;
}) {
  // The empty option is a real choice, not a placeholder: "this card is not
  // written for one account" is something a person means.
  const items = [{ value: NO_ACCOUNT, label: noneLabel }, ...options];
  return (
    <Select
      items={items}
      value={value === "" ? NO_ACCOUNT : value}
      onValueChange={(next) =>
        onChange(next === NO_ACCOUNT || next == null ? "" : next)
      }
    >
      <SelectTrigger aria-label={label}>
        <SelectValue placeholder={noneLabel} />
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

// Base UI's Select treats the empty string as "nothing selected", so the
// "no account" choice needs a value of its own to be selectable at all. It
// never leaves this file: accountSelectionToWire is what the server sees.
const NO_ACCOUNT = "__none__";

// Same reason as NO_ACCOUNT, for the list filter's "every card" row.
const FILTER_ALL = "__all__";

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
  /**
   * Source candidates are supplied by the composition adapter. topic-planning
   * deliberately has no dependency on source-inbox in the content graph.
   */
  sourceCandidates?: TopicSourceCandidate[];
  sourceCandidatesLoading?: boolean;
  sourceCandidatesFailed?: boolean;
  /**
   * Slot rendered at the end of a card's detail view. The work editor is a
   * separate content module that the registry places downstream of this one,
   * so the page it lives on may not import it; the web adapter composes the
   * two and passes the section in here.
   */
  renderCardExtras?: (topicCardId: string) => ReactNode;
}

export function TopicPlanningPage({
  wsId,
  sourceCandidates = [],
  sourceCandidatesLoading = false,
  sourceCandidatesFailed = false,
  renderCardExtras,
}: TopicPlanningPageProps) {
  const { t } = useT("common");
  return (
    <>
      <PageHeader className="xl:hidden">
        <span className="text-body font-medium">
          {t(($) => $.contentTopics.title)}
        </span>
      </PageHeader>
      <TopicPlanningContent
        wsId={wsId}
        sourceCandidates={sourceCandidates}
        sourceCandidatesLoading={sourceCandidatesLoading}
        sourceCandidatesFailed={sourceCandidatesFailed}
        renderCardExtras={renderCardExtras}
      />
    </>
  );
}

function TopicPlanningContent({
  wsId,
  sourceCandidates = [],
  sourceCandidatesLoading = false,
  sourceCandidatesFailed = false,
  renderCardExtras,
}: TopicPlanningPageProps) {
  const { t } = useT("common");
  const accountOptions = useAccountOptions(wsId);
  // "" is every card; the sentinel is the cards no account was chosen for.
  const [accountFilter, setAccountFilter] = useState("");
  const topics = useContentTopics(wsId, accountFilter);
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
            <SettingsRow
              label={t(($) => $.contentTopics.filterByAccount)}
              description={t(($) => $.contentTopics.filterByAccountHint)}
              size="select-wide"
            >
              <Select
                items={[
                  {
                    value: FILTER_ALL,
                    label: t(($) => $.contentTopics.filterAll),
                  },
                  {
                    value: TOPIC_ACCOUNT_FILTER_NONE,
                    label: t(($) => $.contentTopics.noAccount),
                  },
                  ...accountOptions,
                ]}
                value={accountFilter === "" ? FILTER_ALL : accountFilter}
                onValueChange={(value) =>
                  setAccountFilter(
                    value === FILTER_ALL || value == null ? "" : value,
                  )
                }
              >
                <SelectTrigger
                  aria-label={t(($) => $.contentTopics.filterByAccount)}
                >
                  <SelectValue
                    placeholder={t(($) => $.contentTopics.filterAll)}
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={FILTER_ALL}>
                    {t(($) => $.contentTopics.filterAll)}
                  </SelectItem>
                  <SelectItem value={TOPIC_ACCOUNT_FILTER_NONE}>
                    {t(($) => $.contentTopics.noAccount)}
                  </SelectItem>
                  {accountOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </SettingsRow>
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
                  description={`${statusLabel(t, card.status)} · ${
                    card.accountId
                      ? accountName(accountOptions, card.accountId)
                      : t(($) => $.contentTopics.noAccount)
                  } · ${formatChannels(card.channels)}`}
                >
                  <span className="text-caption text-muted-foreground" />
                </SettingsRow>
              ))
            )}
          </SettingsCard>
        </SettingsSection>

        <CreateTopicSection
          wsId={wsId}
          onCreated={setSelectedId}
          accountOptions={accountOptions}
          sources={sourceCandidates}
          sourcesLoading={sourceCandidatesLoading}
          sourcesFailed={sourceCandidatesFailed}
        />

        <NotAvailableYetSection />

        {selected ? (
          <TopicCardPanel
            key={selected.topicCardId}
            wsId={wsId}
            card={selected}
            accountOptions={accountOptions}
            sources={sourceCandidates}
            sourcesLoading={sourceCandidatesLoading}
            sourcesFailed={sourceCandidatesFailed}
            renderCardExtras={renderCardExtras}
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
  accountOptions,
  sources,
  sourcesLoading,
  sourcesFailed,
}: {
  wsId: string;
  onCreated: (topicCardId: string) => void;
  accountOptions: { value: string; label: string }[];
  sources: TopicSourceCandidate[];
  sourcesLoading: boolean;
  sourcesFailed: boolean;
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
        <SettingsRow
          label={t(($) => $.contentTopics.fields.account)}
          description={t(($) => $.contentTopics.accountHint)}
          size="select-wide"
        >
          <AccountSelect
            value={draft.accountId}
            options={accountOptions}
            onChange={(value) => edit({ accountId: value })}
            label={t(($) => $.contentTopics.fields.account)}
            noneLabel={t(($) => $.contentTopics.noAccount)}
          />
        </SettingsRow>
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
        <TopicSourceSelectorRows
          draft={draft}
          sources={sources}
          sourcesLoading={sourcesLoading}
          sourcesFailed={sourcesFailed}
          disabled={create.isPending}
          currentFitSourceIds={[]}
          currentEvidenceSourceIds={[]}
          onChange={edit}
        />
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

function TopicCardPanel({
  wsId,
  card,
  accountOptions,
  sources,
  sourcesLoading,
  sourcesFailed,
  renderCardExtras,
}: {
  wsId: string;
  card: TopicCard;
  accountOptions: { value: string; label: string }[];
  sources: TopicSourceCandidate[];
  sourcesLoading: boolean;
  sourcesFailed: boolean;
  renderCardExtras?: (topicCardId: string) => ReactNode;
}) {
  // The detail query is what the four actions refresh, so the panel reads it
  // rather than the row it was selected from.
  const detail = useContentTopic(wsId, card.topicCardId);
  const current = detail.data ?? card;
  return (
    <>
      <TopicCardDetail card={current} accountOptions={accountOptions} />
      <TopicBodyEditSection
        key={current.topicCardId}
        wsId={wsId}
        card={current}
      />
      <TopicSourceReferenceSection
        key={`${current.topicCardId}:${current.updatedAt}`}
        wsId={wsId}
        card={current}
        sources={sources}
        sourcesLoading={sourcesLoading}
        sourcesFailed={sourcesFailed}
      />
      <AccountLinkSection
        wsId={wsId}
        card={current}
        accountOptions={accountOptions}
      />
      <DecisionSection wsId={wsId} card={current} />
      {canAppendBrief(current) ? (
        <BriefSection wsId={wsId} card={current} />
      ) : null}
      {/* EP-04b sits after the decision actions and the brief versions: the
          "start" action above accepts the topic and freezes the first brief,
          "start a run" below takes one of those revisions up for work. Two
          verbs on one card, adjacent and separately labelled. Shown only once
          there is a revision to start. */}
      {canAppendBrief(current) ? (
        <StartRunSection
          wsId={wsId}
          card={current}
          accountOptions={accountOptions}
        />
      ) : null}
      {/* 024's work sections sit after the start block, which is where the
          ruling put their entry point. Rendered whatever the card's state:
          SOP 6.2 requires that writing works with nothing else in place, so a
          work does not wait on a brief or a start. */}
      {renderCardExtras ? renderCardExtras(current.topicCardId) : null}
    </>
  );
}

function TopicCardDetail({
  card,
  accountOptions,
}: {
  card: TopicCard;
  accountOptions: { value: string; label: string }[];
}) {
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
        <SettingsRow label={t(($) => $.contentTopics.fields.account)}>
          <span className="text-body text-muted-foreground">
            {card.accountId
              ? accountName(accountOptions, card.accountId)
              : t(($) => $.contentTopics.noAccount)}
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

const TOPIC_BODY_FIELDS: {
  key: TopicBodyKey;
  label: (t: Translate) => string;
  placeholder: (t: Translate) => string;
}[] = [
  {
    key: "audienceProblemJudgment",
    label: (t) => t(($) => $.contentTopics.fields.audienceProblemJudgment),
    placeholder: (t) => t(($) => $.contentTopics.placeholders.audienceProblemJudgment),
  },
  {
    key: "ipFit",
    label: (t) => t(($) => $.contentTopics.fields.ipFit),
    placeholder: (t) => t(($) => $.contentTopics.placeholders.ipFit),
  },
  {
    key: "timing",
    label: (t) => t(($) => $.contentTopics.fields.timing),
    placeholder: (t) => t(($) => $.contentTopics.placeholders.timing),
  },
  {
    key: "existingContentRelation",
    label: (t) => t(($) => $.contentTopics.fields.existingContentRelation),
    placeholder: (t) => t(($) => $.contentTopics.placeholders.existingContentRelation),
  },
  {
    key: "evidenceGapsAndInvestment",
    label: (t) => t(($) => $.contentTopics.fields.evidenceGapsAndInvestment),
    placeholder: (t) => t(($) => $.contentTopics.placeholders.evidenceGapsAndInvestment),
  },
];

// The five body answers, editable in place (#244).
//
// Mounted with key={topicCardId}, so each card gets its own draft and its own
// mutation: switching cards starts from an empty draft rather than carrying
// one card's unsaved text onto another. The draft/diff rules themselves live
// in core (editTopicBodyDraft and friends, form-state.test.ts).
function TopicBodyEditSection({
  wsId,
  card,
}: {
  wsId: string;
  card: TopicCard;
}) {
  const { t } = useT("common");
  const patchBody = usePatchContentTopicBody(wsId);
  const [draft, setDraft] = useState<TopicBodyPatchInput>({});
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const changed = topicBodyDraftChanged(draft);

  const edit = (key: TopicBodyKey, value: string) => {
    setDraft((current) => editTopicBodyDraft(current, card, key, value));
  };

  const save = () => {
    // The server answers an empty body with 400; the button is disabled in
    // that state, and this keeps a stray call from reaching it anyway.
    if (!changed || patchBody.isPending) return;
    setOutcome(null);
    patchBody.mutate(
      { topicCardId: card.topicCardId, body: draft },
      {
        onSuccess: () => {
          setOutcome(SAVED);
          setDraft({});
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.bodyEdit.title)}
      description={t(($) => $.contentTopics.bodyEdit.description)}
    >
      <SettingsCard>
        {TOPIC_BODY_FIELDS.map((field) => (
          <SettingsRow
            key={field.key}
            label={field.label(t)}
            size="text"
            align="start"
          >
            <Textarea
              value={topicBodyFieldValue(draft, card, field.key)}
              onChange={(event) => edit(field.key, event.target.value)}
              placeholder={field.placeholder(t)}
              rows={2}
              disabled={patchBody.isPending}
            />
          </SettingsRow>
        ))}
        <SettingsRow label={t(($) => $.contentTopics.bodyEdit.save)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={patchBody.isPending} />
            <Button disabled={!changed || patchBody.isPending} onClick={save}>
              {t(($) => $.contentTopics.bodyEdit.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

function sourceStatusLabel(t: Translate, status: string): string {
  switch (status) {
    case "inbox":
      return t(($) => $.contentTopics.sourceReferences.statuses.inbox);
    case "organized":
      return t(($) => $.contentTopics.sourceReferences.statuses.organized);
    case "archived":
      return t(($) => $.contentTopics.sourceReferences.statuses.archived);
    default:
      return status || t(($) => $.contentTopics.sourceReferences.statuses.unknown);
  }
}

function sourceLabel(
  source: TopicSourceCandidate | undefined,
  sourceId: string,
): string {
  return source?.title.trim() || source?.url.trim() || sourceId;
}

function SourceReferenceValue({
  sourceIds,
  sources,
}: {
  sourceIds: readonly string[];
  sources: TopicSourceCandidate[];
}) {
  const { t } = useT("common");
  if (sourceIds.length === 0) {
    return (
      <span className="text-body text-muted-foreground">
        {t(($) => $.contentTopics.sourceReferences.empty)}
      </span>
    );
  }
  return (
    <ul className="space-y-1">
      {sourceIds.map((sourceId) => {
        const source = sources.find((item) => item.sourceId === sourceId);
        return (
          <li key={sourceId} className="break-words text-body text-muted-foreground">
            {sourceLabel(source, sourceId)} · {sourceStatusLabel(t, source?.status ?? "")}
          </li>
        );
      })}
    </ul>
  );
}

function TopicSourceSelectorRows({
  draft,
  sources,
  sourcesLoading,
  sourcesFailed,
  disabled,
  currentFitSourceIds,
  currentEvidenceSourceIds,
  onChange,
}: {
  draft: TopicSourceDraft;
  sources: TopicSourceCandidate[];
  sourcesLoading: boolean;
  sourcesFailed: boolean;
  disabled: boolean;
  currentFitSourceIds: readonly string[];
  currentEvidenceSourceIds: readonly string[];
  onChange: (patch: TopicSourceDraft) => void;
}) {
  const { t } = useT("common");
  const candidates = sources.filter((source) => source.status !== "archived");
  const fields: {
    field: TopicSourceField;
    label: string;
    currentIds: readonly string[];
  }[] = [
    {
      field: "fitSourceIds",
      label: t(($) => $.contentTopics.sourceReferences.fit),
      currentIds: currentFitSourceIds,
    },
    {
      field: "evidenceSourceIds",
      label: t(($) => $.contentTopics.sourceReferences.evidence),
      currentIds: currentEvidenceSourceIds,
    },
  ];

  return (
    <>
      {fields.map(({ field, label, currentIds }) => {
        const selectedIds = topicSourceSelectionOf(draft, field, currentIds);
        const retainedIds = selectedIds.filter(
          (sourceId) => !candidates.some((source) => source.sourceId === sourceId),
        );
        return (
          <SettingsRow
            key={field}
            label={label}
            description={t(($) => $.contentTopics.sourceReferences.selectorHint)}
            size="text"
            align="start"
          >
            <div className="space-y-2">
              {retainedIds.map((sourceId) => {
                const source = sources.find((item) => item.sourceId === sourceId);
                return (
                  <label
                    key={sourceId}
                    className="flex items-start gap-3 text-body text-muted-foreground"
                  >
                    <Checkbox
                      checked
                      disabled={disabled}
                      onCheckedChange={(value) => {
                        if (value !== false) return;
                        onChange(
                          removeTopicSource(draft, field, sourceId, currentIds),
                        );
                      }}
                      aria-label={t(($) => $.contentTopics.sourceReferences.select, {
                        source: sourceLabel(source, sourceId),
                      })}
                    />
                    <span className="break-words">
                      {sourceLabel(source, sourceId)} · {sourceStatusLabel(t, source?.status ?? "")}
                    </span>
                  </label>
                );
              })}
              {sourcesLoading ? (
                <span className="text-body text-muted-foreground">
                  {t(($) => $.contentTopics.sourceReferences.loading)}
                </span>
              ) : sourcesFailed ? (
                <span className="text-body text-muted-foreground">
                  {t(($) => $.contentTopics.sourceReferences.loadFailed)}
                </span>
              ) : candidates.length === 0 ? (
                <span className="text-body text-muted-foreground">
                  {t(($) => $.contentTopics.sourceReferences.candidatesEmpty)}
                </span>
              ) : (
                <div className="space-y-2">
                  {candidates.map((source) => {
                    const checked = selectedIds.includes(source.sourceId);
                    return (
                      <label
                        key={source.sourceId}
                        className="flex items-start gap-3 text-body text-muted-foreground"
                      >
                        <Checkbox
                          checked={checked}
                          disabled={disabled}
                          onCheckedChange={(value) =>
                            onChange(
                              value === true
                                ? addTopicSource(draft, field, source.sourceId, currentIds)
                                : removeTopicSource(draft, field, source.sourceId, currentIds),
                            )
                          }
                          aria-label={t(($) => $.contentTopics.sourceReferences.select, {
                            source: sourceLabel(source, source.sourceId),
                          })}
                        />
                        <span className="break-words">
                          {sourceLabel(source, source.sourceId)} · {sourceStatusLabel(t, source.status)}
                        </span>
                      </label>
                    );
                  })}
                </div>
              )}
            </div>
          </SettingsRow>
        );
      })}
    </>
  );
}

function TopicSourceReferenceSection({
  wsId,
  card,
  sources,
  sourcesLoading,
  sourcesFailed,
}: {
  wsId: string;
  card: TopicCard;
  sources: TopicSourceCandidate[];
  sourcesLoading: boolean;
  sourcesFailed: boolean;
}) {
  const { t } = useT("common");
  const setSources = useSetContentTopicSources(wsId);
  const [draft, setDraft] = useState<TopicSourceDraft>({});
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const changed = topicSourceDraftDiffers(draft, card);

  const save = () => {
    setOutcome(null);
    setSources.mutate(
      { topicCardId: card.topicCardId, sources: topicSourceDraftToInput(draft) },
      {
        onSuccess: () => {
          setOutcome(SAVED);
          setDraft({});
        },
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.sourceReferences.title)}
      description={t(($) => $.contentTopics.sourceReferences.description)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentTopics.sourceReferences.fit)}
          size="text"
          align="start"
        >
          <SourceReferenceValue sourceIds={card.fitSourceIds} sources={sources} />
        </SettingsRow>
        <SettingsRow
          label={t(($) => $.contentTopics.sourceReferences.evidence)}
          size="text"
          align="start"
        >
          <SourceReferenceValue sourceIds={card.evidenceSourceIds} sources={sources} />
        </SettingsRow>
        <TopicSourceSelectorRows
          draft={draft}
          sources={sources}
          sourcesLoading={sourcesLoading}
          sourcesFailed={sourcesFailed}
          disabled={setSources.isPending}
          currentFitSourceIds={card.fitSourceIds}
          currentEvidenceSourceIds={card.evidenceSourceIds}
          onChange={setDraft}
        />
        <SettingsRow label={t(($) => $.contentTopics.sourceReferences.save)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={setSources.isPending} />
            <Button disabled={!changed || setSources.isPending} onClick={save}>
              {t(($) => $.contentTopics.sourceReferences.save)}
            </Button>
          </div>
        </SettingsRow>
      </SettingsCard>
    </SettingsSection>
  );
}

// Changing which account a card is written for. Its own section rather than a
// field in the decision row: it is a different question ("who is this for")
// from the four actions ("what happens to it"), and the server audits it as
// its own step.
function AccountLinkSection({
  wsId,
  card,
  accountOptions,
}: {
  wsId: string;
  card: TopicCard;
  accountOptions: { value: string; label: string }[];
}) {
  const { t } = useT("common");
  const link = useSetContentTopicAccount(wsId);
  const [selection, setSelection] = useState(() =>
    accountSelectionOf(card.accountId),
  );
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);

  const submit = () => {
    setOutcome(null);
    link.mutate(
      {
        topicCardId: card.topicCardId,
        accountId: accountSelectionToWire(selection),
      },
      {
        onSuccess: () => setOutcome(SAVED),
        onError: (error) => setOutcome(saveOutcome(error)),
      },
    );
  };

  return (
    <SettingsSection
      title={t(($) => $.contentTopics.accountTitle)}
      description={t(($) => $.contentTopics.accountDescription)}
    >
      <SettingsCard>
        <SettingsRow
          label={t(($) => $.contentTopics.fields.account)}
          size="select-wide"
        >
          <AccountSelect
            value={selection}
            options={accountOptions}
            onChange={setSelection}
            label={t(($) => $.contentTopics.fields.account)}
            noneLabel={t(($) => $.contentTopics.noAccount)}
          />
        </SettingsRow>
        <SettingsRow label={t(($) => $.contentTopics.accountSave)}>
          <div className="flex items-center gap-3">
            <SaveFeedback outcome={outcome} pending={link.isPending} />
            {/* Saving the account the card already has would write an audit
                event that records nothing happening. */}
            <Button
              onClick={submit}
              disabled={
                link.isPending ||
                !accountSelectionChanged(selection, card.accountId)
              }
            >
              {t(($) => $.contentTopics.accountSave)}
            </Button>
          </div>
        </SettingsRow>
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

// Marketing nodes (specs/033 PR 3): a page of its own, exported from the
// module it belongs to.
export {
  MarketingNodesPage,
  type MarketingNodeAccountOption,
  type MarketingNodeSourceOption,
  type MarketingNodesPageProps,
} from "./marketing-nodes";
