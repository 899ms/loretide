// The page's non-visual logic: what a form holds, what the server is sent, and
// which of the two a button should be looking at.
//
// It lives here rather than inside the component for the usual reason - these
// are the rules the product actually has (channels are a list, a decision keeps
// its reason only when it is a deferral or a drop, a brief is appended rather
// than edited), and rules that live in JSX cannot be asserted without a
// browser. The page holds text; this file decides what that text means.

import {
  type BriefRevision,
  type BriefRevisionInput,
  type TopicActionInput,
  type TopicCard,
  type TopicCardInput,
  type SetContentTopicSourcesInput,
} from "./contract";

/**
 * Quick reasons offered when a card is deferred or dropped. Codes, not
 * sentences: the label is a translation and the stored value must read the same
 * to every reader of the card, whatever language wrote it.
 *
 * The free-text note is separate and always optional - a quick reason answers
 * "which kind of no", the note answers "why this one".
 */
export const TOPIC_DECISION_REASONS = [
  "no_evidence",
  "no_timing",
  "duplicate",
  "low_fit",
  "no_capacity",
] as const;

export type TopicDecisionReason = (typeof TOPIC_DECISION_REASONS)[number];

/** The four actions, in the order the page offers them. */
export const TOPIC_ACTIONS = ["start", "save", "defer", "drop"] as const;
export type TopicAction = (typeof TOPIC_ACTIONS)[number];

/**
 * True when this action records a reason and a note. The server keeps both only
 * for a deferral or a drop and blanks them otherwise, so sending them for a
 * start or a save would quietly write nothing - the page asks here instead of
 * offering inputs whose content is discarded.
 */
export function actionKeepsReason(action: TopicAction): boolean {
  return action === "defer" || action === "drop";
}

/** Build the action request, dropping what this action does not keep. */
export function decisionInput(
  action: TopicAction,
  reason: string,
  note: string,
): TopicActionInput {
  if (!actionKeepsReason(action)) return { action };
  return { action, reason, note };
}

// Channels are stored as a list and typed as one line of text. Splitting on
// both the ASCII and the fullwidth comma matters: a Chinese keyboard produces
// the fullwidth one, and treating "知乎，公众号" as a single channel name is
// the kind of failure nobody reports, they just retype it.
const CHANNEL_SEPARATORS = /[,，、]/;

export function parseChannels(text: string): string[] {
  const seen = new Set<string>();
  const channels: string[] = [];
  for (const part of text.split(CHANNEL_SEPARATORS)) {
    const channel = part.trim();
    if (!channel || seen.has(channel)) continue;
    seen.add(channel);
    channels.push(channel);
  }
  return channels;
}

export function formatChannels(channels: readonly string[]): string {
  return channels.join(", ");
}

/**
 * The list filter value for "no account chosen yet".
 *
 * A filter has three answers and only two of them have an id: every card, one
 * account's cards, and the cards nobody has attached to an account. The third
 * gets a word no id can be — ids are hex — and the server agrees on it.
 */
export const TOPIC_ACCOUNT_FILTER_NONE = "none";

/** Every card, regardless of account. Empty so an absent filter is the default. */
export const TOPIC_ACCOUNT_FILTER_ALL = "";

/**
 * What a card's account selection means on the wire.
 *
 * The page holds one string because a select holds one string: the empty one
 * is "no account", and the server is told null rather than "" so the intent
 * survives a reader who treats "" as "unset".
 */
export function accountSelectionToWire(selection: string): string | null {
  const trimmed = selection.trim();
  return trimmed === "" ? null : trimmed;
}

/** What the select should show for a card, given what is stored on it. */
export function accountSelectionOf(accountId: string | null | undefined): string {
  return accountId ?? "";
}

/**
 * True when the selection would change the card. A link write is audited, so
 * re-confirming the account a card already has would add an event saying
 * nothing happened.
 */
export function accountSelectionChanged(
  selection: string,
  accountId: string | null | undefined,
): boolean {
  return accountSelectionToWire(selection) !== (accountId ?? null);
}

/** The two independent source-reference columns on a topic card (specs/030). */
export type TopicSourceField = "fitSourceIds" | "evidenceSourceIds";

/**
 * A partial edit of a card's source references.
 *
 * An omitted property deliberately means "leave that column alone". An empty
 * array means "clear that column". Keeping that distinction in the draft is
 * necessary because the endpoint applies it independently to each column.
 */
export interface TopicSourceDraft {
  fitSourceIds?: string[];
  evidenceSourceIds?: string[];
}

function normalizeSourceIds(ids: readonly string[] | undefined): string[] {
  const seen = new Set<string>();
  const normalized: string[] = [];
  for (const value of ids ?? []) {
    const id = value.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    normalized.push(id);
  }
  return normalized;
}

function sourceIdsEqual(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((id, index) => id === right[index]);
}

/** The selected ids for one field, falling back to the card until it is edited. */
export function topicSourceSelectionOf(
  draft: TopicSourceDraft,
  field: TopicSourceField,
  currentIds: readonly string[],
): string[] {
  return normalizeSourceIds(draft[field] ?? currentIds);
}

/** Adds one source to a column without changing the other column. */
export function addTopicSource(
  draft: TopicSourceDraft,
  field: TopicSourceField,
  sourceId: string,
  currentIds: readonly string[],
): TopicSourceDraft {
  return {
    ...draft,
    [field]: normalizeSourceIds([
      ...topicSourceSelectionOf(draft, field, currentIds),
      sourceId,
    ]),
  };
}

/** Removes one source. Removing the last selected source is an explicit clear. */
export function removeTopicSource(
  draft: TopicSourceDraft,
  field: TopicSourceField,
  sourceId: string,
  currentIds: readonly string[],
): TopicSourceDraft {
  const id = sourceId.trim();
  return {
    ...draft,
    [field]: topicSourceSelectionOf(draft, field, currentIds).filter(
      (current) => current !== id,
    ),
  };
}

/** Converts only edited fields to the wire shape; omission is not a clear. */
export function topicSourceDraftToInput(
  draft: TopicSourceDraft,
): SetContentTopicSourcesInput {
  const input: SetContentTopicSourcesInput = {};
  if (draft.fitSourceIds !== undefined) {
    input.fitSourceIds = normalizeSourceIds(draft.fitSourceIds);
  }
  if (draft.evidenceSourceIds !== undefined) {
    input.evidenceSourceIds = normalizeSourceIds(draft.evidenceSourceIds);
  }
  return input;
}

/** Whether an edited draft would change either stored source-reference column. */
export function topicSourceDraftDiffers(
  draft: TopicSourceDraft,
  card: Pick<TopicCard, "fitSourceIds" | "evidenceSourceIds">,
): boolean {
  const input = topicSourceDraftToInput(draft);
  return (
    (input.fitSourceIds !== undefined &&
      !sourceIdsEqual(input.fitSourceIds, normalizeSourceIds(card.fitSourceIds))) ||
    (input.evidenceSourceIds !== undefined &&
      !sourceIdsEqual(input.evidenceSourceIds, normalizeSourceIds(card.evidenceSourceIds)))
  );
}

/** The seven items of docs/01 §5.2, as the form holds them. */
export interface TopicCardDraft extends TopicSourceDraft {
  /** The account this card is written for; empty means none was chosen. */
  accountId: string;
  audienceProblemJudgment: string;
  ipFit: string;
  timing: string;
  existingContentRelation: string;
  evidenceGapsAndInvestment: string;
  channels: string;
  recommendedAction: string;
}

export const emptyTopicCardDraft: TopicCardDraft = {
  accountId: "",
  audienceProblemJudgment: "",
  ipFit: "",
  timing: "",
  existingContentRelation: "",
  evidenceGapsAndInvestment: "",
  channels: "",
  recommendedAction: "",
};

export function topicCardDraftToInput(draft: TopicCardDraft): TopicCardInput {
  return {
    accountId: accountSelectionToWire(draft.accountId),
    audienceProblemJudgment: draft.audienceProblemJudgment.trim(),
    ipFit: draft.ipFit.trim(),
    timing: draft.timing.trim(),
    existingContentRelation: draft.existingContentRelation.trim(),
    evidenceGapsAndInvestment: draft.evidenceGapsAndInvestment.trim(),
    channels: parseChannels(draft.channels),
    recommendedAction: draft.recommendedAction.trim(),
    ...topicSourceDraftToInput(draft),
  };
}

/**
 * Whether the card can be created.
 *
 * Every item must say something, and "没有" is one of the things it may say
 * (§5.2: no timing basis means write that down). That is the opposite of
 * forcing invented content - what it refuses is a blank, which reads as "not
 * considered" and is indistinguishable from "considered, and there is none".
 */
export function isTopicCardDraftReady(draft: TopicCardDraft): boolean {
  const items = [
    draft.audienceProblemJudgment,
    draft.ipFit,
    draft.timing,
    draft.existingContentRelation,
    draft.evidenceGapsAndInvestment,
    draft.recommendedAction,
  ];
  return items.every((item) => item.trim() !== "") && parseChannels(draft.channels).length > 0;
}

/** The eleven items of §5.3, as the form holds them. */
export interface BriefDraft {
  audience: string;
  coreProblem: string;
  claimAndBoundaries: string;
  channels: string;
  format: string;
  structure: string;
  citationRequirements: string;
  sourceScope: string;
  deliverable: string;
  timeLimit: string;
  costLimit: string;
}

export const emptyBriefDraft: BriefDraft = {
  audience: "",
  coreProblem: "",
  claimAndBoundaries: "",
  channels: "",
  format: "",
  structure: "",
  citationRequirements: "",
  sourceScope: "",
  deliverable: "",
  timeLimit: "",
  costLimit: "",
};

/**
 * Seed the editing form from a version. Editing a brief means appending the
 * next version, so the form starts as a copy of the one being changed rather
 * than blank - otherwise every edit would silently drop the ten items the
 * author did not retype.
 */
export function briefDraftFromRevision(revision: BriefRevision | undefined): BriefDraft {
  if (!revision) return emptyBriefDraft;
  return {
    audience: revision.audience,
    coreProblem: revision.coreProblem,
    claimAndBoundaries: revision.claimAndBoundaries,
    channels: formatChannels(revision.channels),
    format: revision.format,
    structure: revision.structure,
    citationRequirements: revision.citationRequirements,
    sourceScope: revision.sourceScope,
    deliverable: revision.deliverable,
    timeLimit: revision.timeLimit,
    costLimit: revision.costLimit,
  };
}

export function briefDraftToInput(draft: BriefDraft): BriefRevisionInput {
  return {
    audience: draft.audience.trim(),
    coreProblem: draft.coreProblem.trim(),
    claimAndBoundaries: draft.claimAndBoundaries.trim(),
    channels: parseChannels(draft.channels),
    format: draft.format.trim(),
    structure: draft.structure.trim(),
    citationRequirements: draft.citationRequirements.trim(),
    sourceScope: draft.sourceScope.trim(),
    deliverable: draft.deliverable.trim(),
    timeLimit: draft.timeLimit.trim(),
    costLimit: draft.costLimit.trim(),
  };
}

/**
 * True when the draft would store something different from the version it was
 * seeded from. Versions are append-only and permanent, so a save that changed
 * nothing would still add one to the history for a reader to work out later.
 */
export function briefDraftDiffers(
  draft: BriefDraft,
  revision: BriefRevision | undefined,
): boolean {
  const next = briefDraftToInput(draft);
  // With no version to compare against, anything typed is a difference.
  const current = briefDraftToInput(briefDraftFromRevision(revision));
  return (
    current.audience !== next.audience ||
    current.coreProblem !== next.coreProblem ||
    current.claimAndBoundaries !== next.claimAndBoundaries ||
    formatChannels(current.channels) !== formatChannels(next.channels) ||
    current.format !== next.format ||
    current.structure !== next.structure ||
    current.citationRequirements !== next.citationRequirements ||
    current.sourceScope !== next.sourceScope ||
    current.deliverable !== next.deliverable ||
    current.timeLimit !== next.timeLimit ||
    current.costLimit !== next.costLimit
  );
}

/**
 * Versions oldest first. The server already orders them, but a list that is
 * read for "which one is current" must not depend on that promise holding for
 * every future endpoint that returns the same shape.
 */
export function sortedRevisions(revisions: readonly BriefRevision[]): BriefRevision[] {
  return [...revisions].sort((left, right) => left.revision - right.revision);
}

export function latestRevision(
  revisions: readonly BriefRevision[],
): BriefRevision | undefined {
  const sorted = sortedRevisions(revisions);
  return sorted[sorted.length - 1];
}

/**
 * A card only accepts appended versions once it has been started: the first
 * version is created by the start action, and appending before that is refused
 * by the server. Asking here keeps the page from offering a button whose only
 * outcome is an error.
 */
export function canAppendBrief(card: TopicCard | undefined): boolean {
  return !!card && !!card.startedBriefRevisionId;
}

/**
 * The translation key for a status, with a default branch: the server is the
 * authority on this enum and an installed client can meet a value it has never
 * heard of. It renders as "unknown" rather than blank or as a crash.
 */
export function topicStatusKey(status: string): string {
  switch (status) {
    case "draft":
    case "started":
    case "saved":
    case "deferred":
    case "dropped":
      return status;
    default:
      return "unknown";
  }
}
