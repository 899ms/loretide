// Structural inputs for the Today dashboard's derived views.
//
// These types are declared here rather than imported from the content modules
// on purpose. `packages/core/today/` sits outside `packages/core/content/`, so
// importing a content module's index from here would be rejected by
// check-content-boundaries as a private import from a non-adapter file. It
// also could not live inside a content module: the dashboard reads
// topic-planning, work-editor, review-delivery and ip-profile, and no
// registered module depends on all four.
//
// Structural typing is what keeps these honest. The adapter page passes the
// real parsed objects into these functions, so a field that is renamed or
// retyped upstream fails typecheck there. That page is the proof, not a
// duplicated schema here.
//
// Note the casing split: ip-profile speaks snake_case (`can_start`,
// `weekly_hours`), the other three speak camelCase. That is upstream's shape,
// not a choice made here.

/** A topic card, as topic-planning's contract returns it. */
export interface TopicCardLike {
  topicCardId: string;
  accountId: string | null;
  status: string;
  audienceProblemJudgment: string;
  recommendedAction: string;
  channels: string[];
  createdAt: string;
}

/** A work container, as work-editor's contract returns it. */
export interface WorkLike {
  workId: string;
  topicCardId: string;
  title: string;
  updatedAt: string;
}

/** A document's editing copy, as work-editor's contract returns it. */
export interface ArtifactLike {
  artifactId: string;
  workId: string;
  title: string;
  draftStatus: string;
  updatedAt: string;
}

/** A review request, as review-delivery's contract returns it. */
export interface ReviewRequestLike {
  reviewRequestId: string;
  artifactId: string;
  channel: string;
  status: string;
  requestedAt: string;
}

/**
 * A delivery task, as review-delivery's contract returns it.
 *
 * `due` and `pendingRegistration` are DERIVED BY THE SERVER on read. Nothing
 * here recomputes them: the server's definition is the one SOP 9.1/9.2
 * describe, and a second computation would be a second truth.
 */
export interface DeliveryTaskLike {
  deliveryTaskId: string;
  artifactId: string;
  channel: string;
  status: string;
  scheduledAt: string;
  due: boolean;
  pendingRegistration: boolean;
}

/** A content account, as ip-profile's contract returns it (snake_case). */
export interface AccountLike {
  account_id: string;
  display_name: string;
  platform: string;
}

/** An expression profile's readiness and weekly hours (snake_case). */
export interface ProfileLike {
  readiness: { can_start: boolean; missing: string[] };
  weekly_hours: { value: number; status: string };
}

/**
 * How much time the person has to spend on a card, per SOP §11's "预计投入".
 *
 * Three cases, not two. A profile whose weekly hours are still pending holds a
 * number, but it is a number nobody has confirmed - showing it as the estimate
 * would turn an unfinished setup step into a commitment.
 */
export type EstimatedEffort =
  | { kind: "hours"; weeklyHours: number }
  | { kind: "unconfirmed" }
  | { kind: "no-account" };

export interface TopicEntry {
  topicCardId: string;
  accountId: string | null;
  /** Why this card is worth writing, per SOP §11. */
  reason: string;
  effort: EstimatedEffort;
  channels: string[];
  /** True when this card's account profile could not be read (FR-017). */
  failed: boolean;
}

export interface WorkEntry {
  workId: string;
  workTitle: string;
  artifactId: string;
  artifactTitle: string;
  failed: boolean;
}

export interface ReviewEntry {
  reviewRequestId: string;
  channel: string;
  status: string;
}

export interface DeliveryEntry {
  deliveryTaskId: string;
  channel: string;
  /** Which of SOP 9.1/9.2's two situations put this here. */
  reason: "due" | "pending_registration";
}

export interface AccountGapEntry {
  accountId: string;
  displayName: string;
  /** Field names, not a sentence - the page points at the field (FR-009). */
  missing: string[];
  failed: boolean;
}

/**
 * One section's visible slice plus what was cut.
 *
 * `total` is counted BEFORE the cap. A total taken after truncation would read
 * "10 of 10" on a list of 37, which is worse than showing no total at all
 * because it looks correct.
 */
export interface Section<T> {
  shown: T[];
  total: number;
  hidden: number;
}
