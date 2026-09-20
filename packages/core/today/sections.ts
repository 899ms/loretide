// The Today dashboard's derived views, as pure functions.
//
// Every rule about what belongs in a section lives here rather than in the
// page, so that a second surface (desktop) reuses the judgment instead of the
// markup. The page's only job is to render what these return.

import type {
  AccountGapEntry,
  AccountLike,
  ArtifactLike,
  DeliveryEntry,
  DeliveryTaskLike,
  EstimatedEffort,
  ProfileLike,
  ReviewEntry,
  ReviewRequestLike,
  Section,
  TopicCardLike,
  TopicEntry,
  WorkEntry,
  WorkLike,
} from "./types";

/** How many rows a section shows before it defers to the object's own page. */
export const SECTION_LIMIT = 10;

/** The only topic status the section lists (ruling Q2 = A). */
export const TOPIC_STATUS_WORTH_WRITING = "draft";

/** Review states that still need a person (FR-006). */
export const REVIEW_STATUSES_NEEDING_ATTENTION = ["pending", "changes_requested"];

/** The draft state that means a document is still being written (FR-008). */
export const ARTIFACT_STATUS_IN_PROGRESS = "working";

/** ip-profile's confirmed field status. Anything else is not confirmed. */
const FIELD_CONFIRMED = "confirmed";

/**
 * Cap a section and report what was cut.
 *
 * `total` is the length before truncation, which is what makes "10 of 37"
 * possible. Taking it afterwards would report 10 of 10.
 */
export function capSection<T>(items: T[], limit: number = SECTION_LIMIT): Section<T> {
  const total = items.length;
  const shown = items.slice(0, limit);
  return { shown, total, hidden: total - shown.length };
}

/** A one-line reason a card is worth writing, per SOP §11. */
export function topicReason(card: TopicCardLike): string {
  const judgment = card.audienceProblemJudgment.trim();
  if (judgment) return judgment;
  return card.recommendedAction.trim();
}

/**
 * How much time this card's account has per week.
 *
 * Pending hours are reported as unconfirmed rather than as their value: the
 * number exists, but treating an unfinished setup step as a plan is the thing
 * FR-010b forbids.
 */
export function estimatedEffort(
  accountId: string | null,
  profile: ProfileLike | undefined,
): EstimatedEffort {
  if (!accountId) return { kind: "no-account" };
  if (!profile) return { kind: "unconfirmed" };
  if (profile.weekly_hours.status !== FIELD_CONFIRMED) return { kind: "unconfirmed" };
  return { kind: "hours", weeklyHours: profile.weekly_hours.value };
}

/**
 * Section 1 - topic cards worth writing.
 *
 * Only `draft`. `started` is already being written and belongs to section 2;
 * `saved`, `deferred` and `dropped` are decided. `deferred` is excluded by the
 * ruling even though "come back to it later" reads like a candidate.
 */
export function worthWritingTopics(
  cards: TopicCardLike[],
  profiles: Map<string, ProfileLike>,
  failedAccountIds: ReadonlySet<string> = new Set(),
): Section<TopicEntry> {
  const entries = cards
    .filter((card) => card.status === TOPIC_STATUS_WORTH_WRITING)
    // Newest first: a card made today is the one the person just thought of.
    .slice()
    .sort((a, b) => compareDesc(a.createdAt, b.createdAt))
    .map((card) => ({
      topicCardId: card.topicCardId,
      accountId: card.accountId,
      reason: topicReason(card),
      effort: estimatedEffort(
        card.accountId,
        card.accountId ? profiles.get(card.accountId) : undefined,
      ),
      channels: card.channels,
      failed: card.accountId !== null && failedAccountIds.has(card.accountId),
    }));
  return capSection(entries);
}

/**
 * Section 2 - works with at least one document still being written.
 *
 * A work with no documents is not "in progress": there is nothing to continue.
 */
export function worksInProgress(
  works: WorkLike[],
  artifacts: Map<string, ArtifactLike[]>,
  failedWorkIds: ReadonlySet<string> = new Set(),
): Section<WorkEntry> {
  const entries: WorkEntry[] = [];
  // Most recently touched first, decided before mapping so the entry list is
  // already in order.
  const ordered = works.slice().sort((a, b) => compareDesc(a.updatedAt, b.updatedAt));
  for (const work of ordered) {
    const list = artifacts.get(work.workId) ?? [];
    const working = list.filter((item) => item.draftStatus === ARTIFACT_STATUS_IN_PROGRESS);
    if (working.length === 0) {
      // A work whose documents could not be read is still reported, so that
      // "nothing in progress" and "could not tell" do not look the same.
      if (failedWorkIds.has(work.workId)) {
        entries.push({
          workId: work.workId,
          workTitle: work.title,
          artifactId: "",
          artifactTitle: "",
          failed: true,
        });
      }
      continue;
    }
    const latest = working.reduce((newest, item) =>
      compareDesc(item.updatedAt, newest.updatedAt) < 0 ? item : newest,
    );
    entries.push({
      workId: work.workId,
      workTitle: work.title,
      artifactId: latest.artifactId,
      artifactTitle: latest.title,
      failed: false,
    });
  }
  return capSection(entries);
}

/** Section 3 - reviews still waiting on a person. */
export function reviewsNeedingAttention(reviews: ReviewRequestLike[]): Section<ReviewEntry> {
  const entries = reviews
    .filter((review) => REVIEW_STATUSES_NEEDING_ATTENTION.includes(review.status))
    // Oldest first: the one that has been waiting longest is the one to do.
    .slice()
    .sort((a, b) => compareAsc(a.requestedAt, b.requestedAt))
    .map((review) => ({
      reviewRequestId: review.reviewRequestId,
      channel: review.channel,
      status: review.status,
    }));
  return capSection(entries);
}

/**
 * Section 4 - handovers that came due and handovers nobody wrote down.
 *
 * Both flags come from the server, which derives them on read. Recomputing
 * "has the scheduled time passed" here would be a second definition of due,
 * and the server's is the one SOP 9.1 describes.
 */
export function deliveriesNeedingAction(tasks: DeliveryTaskLike[]): Section<DeliveryEntry> {
  const entries: DeliveryEntry[] = [];
  for (const task of tasks) {
    if (task.due) {
      entries.push({ deliveryTaskId: task.deliveryTaskId, channel: task.channel, reason: "due" });
      continue;
    }
    if (task.pendingRegistration) {
      entries.push({
        deliveryTaskId: task.deliveryTaskId,
        channel: task.channel,
        reason: "pending_registration",
      });
    }
  }
  // Overdue handovers before unrecorded ones: one is late, the other is only
  // unwritten.
  entries.sort((a, b) => reasonRank(a.reason) - reasonRank(b.reason));
  return capSection(entries);
}

function reasonRank(reason: DeliveryEntry["reason"]): number {
  return reason === "due" ? 0 : 1;
}

/** Section 6 - accounts whose setup does not yet meet the start condition. */
export function accountsMissingConfig(
  accounts: AccountLike[],
  profiles: Map<string, ProfileLike>,
  failedAccountIds: ReadonlySet<string> = new Set(),
): Section<AccountGapEntry> {
  const entries: AccountGapEntry[] = [];
  for (const account of accounts) {
    const profile = profiles.get(account.account_id);
    if (!profile) {
      if (failedAccountIds.has(account.account_id)) {
        entries.push({
          accountId: account.account_id,
          displayName: account.display_name,
          missing: [],
          failed: true,
        });
      }
      continue;
    }
    if (profile.readiness.can_start) continue;
    entries.push({
      accountId: account.account_id,
      displayName: account.display_name,
      // Field names are kept verbatim: the page points at the field rather
      // than translating a sentence.
      missing: profile.readiness.missing,
      failed: false,
    });
  }
  entries.sort((a, b) => a.displayName.localeCompare(b.displayName));
  return capSection(entries);
}

function compareDesc(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? 1 : -1;
}

function compareAsc(a: string, b: string): number {
  if (a === b) return 0;
  return a < b ? -1 : 1;
}
