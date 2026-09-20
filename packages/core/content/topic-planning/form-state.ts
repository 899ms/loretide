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

/** The seven items of docs/01 §5.2, as the form holds them. */
export interface TopicCardDraft {
  audienceProblemJudgment: string;
  ipFit: string;
  timing: string;
  existingContentRelation: string;
  evidenceGapsAndInvestment: string;
  channels: string;
  recommendedAction: string;
}

export const emptyTopicCardDraft: TopicCardDraft = {
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
    accountId: null,
    audienceProblemJudgment: draft.audienceProblemJudgment.trim(),
    ipFit: draft.ipFit.trim(),
    timing: draft.timing.trim(),
    existingContentRelation: draft.existingContentRelation.trim(),
    evidenceGapsAndInvestment: draft.evidenceGapsAndInvestment.trim(),
    channels: parseChannels(draft.channels),
    recommendedAction: draft.recommendedAction.trim(),
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
