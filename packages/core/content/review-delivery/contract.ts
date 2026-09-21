import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Review requests, delivery tasks and publication records (SOP 8-9.3).
//
// The controlled sets are the server's; they are restated here as literal
// unions so a component can switch on them, and `contract.test.ts` holds them
// to the Go source rather than to my memory of it.
//
// There is deliberately NO controlled set for who declared a publication or for
// how it was verified: SOP 9.2 enumerates neither, so both are free text. The
// same test asserts no seventh set appears here later.
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md

/** SOP 8's first four channels. */
export const REVIEW_CHANNELS = ["xiaohongshu", "wechat_mp", "douyin", "shipinhao"] as const;
export type ReviewChannel = (typeof REVIEW_CHANNELS)[number];

/** SOP 7.1's review row. `pending` is the only start; the other four end it. */
export const REVIEW_STATUSES = [
  "pending", "changes_requested", "approved", "rejected", "cancelled",
] as const;
export type ReviewStatus = (typeof REVIEW_STATUSES)[number];

/** SOP 7.1's delivery row. */
export const DELIVERY_STATUSES = [
  "draft", "ready", "scheduled", "handed_off", "cancelled", "held",
] as const;
export type DeliveryStatus = (typeof DELIVERY_STATUSES)[number];

/** SOP 7.1's publication row. There is no state machine over it: each record is
 *  one independent observation, so a `removed` after a `verified_published` is
 *  the ordinary case of a platform taking the piece down. */
export const PUBLICATION_STATUSES = [
  "reported_published", "verified_published", "failed", "removed", "unknown",
] as const;
export type PublicationStatus = (typeof PUBLICATION_STATUSES)[number];

/** SOP 9.1's three handover actions. None of them means published. */
export const HANDOFF_METHODS = ["export", "copy", "handed_to_operator"] as const;
export type HandoffMethod = (typeof HANDOFF_METHODS)[number];

/** SOP 9.1's three version-match states. `unknown` is the SOP's own value for
 *  "the full text could not be retrieved" and is not a failure. */
export const VERSION_MATCHES = ["matched", "differs", "unknown"] as const;
export type VersionMatch = (typeof VERSION_MATCHES)[number];

export const TRANSITION_SUBJECTS = ["review_request", "delivery_task"] as const;
export type TransitionSubject = (typeof TRANSITION_SUBJECTS)[number];

/** The eight keys SOP 8 freezes. `attachments` is always empty until W-03. */
export interface DeliverySnapshot {
  channel: string;
  workId: string;
  artifactId: string;
  versionId: string;
  accountId: string;
  /** "" when the work was not started from a snapshot. A real state. */
  startSnapshotId: string;
  attachments: string[];
  deliveryConfig: Record<string, string>;
}

export interface ReviewRequest {
  reviewRequestId: string;
  workspaceId: string;
  workId: string;
  artifactId: string;
  versionId: string;
  accountId: string;
  channel: string;
  snapshot: DeliverySnapshot;
  status: string;
  requestedBy: string;
  requestedAt: string;
  /** "" and "" until someone decides. Not missing - undecided. */
  decidedBy: string;
  decidedAt: string;
  decisionNote: string;
  createdAt: string;
  updatedAt: string;
}

export interface ReviewTransition {
  transitionId: string;
  subjectKind: string;
  subjectId: string;
  /** "" means the subject was created. */
  fromStatus: string;
  toStatus: string;
  reason: string;
  actorId: string;
  createdAt: string;
}

export interface DeliveryTask {
  deliveryTaskId: string;
  workspaceId: string;
  workId: string;
  artifactId: string;
  /** "" while the task is still a draft. */
  reviewRequestId: string;
  channel: string;
  status: string;
  /** A time a PERSON reads. Nothing acts on it. */
  scheduledAt: string;
  handoffMethod: string;
  createdAt: string;
  updatedAt: string;
  /** Derived by the server on read: the planned time has passed and the task
   *  was not handed over. Not a stored column. */
  due: boolean;
  /** Derived by the server on read: handed over with nothing written down yet.
   *  SOP 9.2's 待登记 - it says nobody recorded anything, NOT that the system
   *  asked a platform. */
  pendingRegistration: boolean;
}

export interface PublicationRecord {
  publicationRecordId: string;
  workspaceId: string;
  workId: string;
  artifactId: string;
  deliveryTaskId: string;
  channel: string;
  status: string;
  /** Who typed it, from the session. */
  actorId: string;
  /** Who said it. Free text - SOP 9.2 gives no enumeration. */
  declaredBy: string;
  pageUrlOrContentId: string;
  receiptNote: string;
  /** How it was checked. Free text, for the same reason. */
  verificationNote: string;
  publishedAt: string;
  platformAccount: string;
  platformEdited: boolean;
  editNote: string;
  versionMatch: string;
  /** SOP 3.3's 发布后快照: the version this publication is of, stated
   *  directly. "" when it was not stated and the version has to be found
   *  through the delivery task instead. */
  versionId: string;
  /** SOP 3.3's 历史导入标识: this record is for something published before
   *  this system was in use. Carried on the record itself so a later
   *  aggregate can tell historical numbers from new ones without a join. */
  historicalImport: boolean;
  createdAt: string;
}

// Lenient by design: an installed client talks to whatever backend is deployed,
// and a status this build has not heard of must not blank the page. Every
// controlled value stays `z.string()` for that reason - what the page may WRITE
// is the union above; what it may READ is wider.
const snapshotSchema = z.object({
  channel: z.string().optional(),
  work_id: z.string().optional(),
  artifact_id: z.string().optional(),
  version_id: z.string().optional(),
  account_id: z.string().optional(),
  start_snapshot_id: z.string().optional(),
  attachments: z.array(z.string()).nullable().optional(),
  delivery_config: z.record(z.string(), z.string()).nullable().optional(),
});

const reviewSchema = z.object({
  review_request_id: z.string(),
  workspace_id: z.string().optional(),
  work_id: z.string().optional(),
  artifact_id: z.string().optional(),
  version_id: z.string().optional(),
  account_id: z.string().optional(),
  channel: z.string().optional(),
  snapshot: snapshotSchema.nullable().optional(),
  status: z.string().optional(),
  requested_by: z.string().optional(),
  requested_at: z.string().optional(),
  decided_by: z.string().optional(),
  decided_at: z.string().nullable().optional(),
  decision_note: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
});

const transitionSchema = z.object({
  transition_id: z.string(),
  subject_kind: z.string().optional(),
  subject_id: z.string().optional(),
  from_status: z.string().optional(),
  to_status: z.string().optional(),
  reason: z.string().optional(),
  actor_id: z.string().optional(),
  created_at: z.string().optional(),
});

const deliverySchema = z.object({
  delivery_task_id: z.string(),
  workspace_id: z.string().optional(),
  work_id: z.string().optional(),
  artifact_id: z.string().optional(),
  review_request_id: z.string().optional(),
  channel: z.string().optional(),
  status: z.string().optional(),
  scheduled_at: z.string().nullable().optional(),
  handoff_method: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
  due: z.boolean().optional(),
  pending_registration: z.boolean().optional(),
});

const publicationSchema = z.object({
  publication_record_id: z.string(),
  workspace_id: z.string().optional(),
  work_id: z.string().optional(),
  artifact_id: z.string().optional(),
  delivery_task_id: z.string().optional(),
  channel: z.string().optional(),
  status: z.string().optional(),
  actor_id: z.string().optional(),
  declared_by: z.string().optional(),
  page_url_or_content_id: z.string().optional(),
  receipt_note: z.string().optional(),
  verification_note: z.string().optional(),
  published_at: z.string().nullable().optional(),
  platform_account: z.string().optional(),
  platform_edited: z.boolean().optional(),
  edit_note: z.string().optional(),
  version_match: z.string().optional(),
  version_id: z.string().optional(),
  historical_import: z.boolean().optional(),
  created_at: z.string().optional(),
});

const reviewDetailSchema = z.object({
  review: reviewSchema.nullable().optional(),
  transitions: z.array(transitionSchema).nullable().optional(),
});
const reviewListSchema = z.object({ reviews: z.array(reviewSchema).nullable().optional() });
const deliveryListSchema = z.object({ deliveries: z.array(deliverySchema).nullable().optional() });
const publicationListSchema = z.object({
  publications: z.array(publicationSchema).nullable().optional(),
});

const EMPTY_SNAPSHOT: DeliverySnapshot = {
  channel: "", workId: "", artifactId: "", versionId: "", accountId: "",
  startSnapshotId: "", attachments: [], deliveryConfig: {},
};

const EMPTY_REVIEW: ReviewRequest = {
  reviewRequestId: "", workspaceId: "", workId: "", artifactId: "", versionId: "",
  accountId: "", channel: "", snapshot: EMPTY_SNAPSHOT, status: "", requestedBy: "",
  requestedAt: "", decidedBy: "", decidedAt: "", decisionNote: "",
  createdAt: "", updatedAt: "",
};

function toSnapshot(wire: z.infer<typeof snapshotSchema> | null | undefined): DeliverySnapshot {
  if (!wire) return EMPTY_SNAPSHOT;
  return {
    channel: wire.channel ?? "",
    workId: wire.work_id ?? "",
    artifactId: wire.artifact_id ?? "",
    versionId: wire.version_id ?? "",
    accountId: wire.account_id ?? "",
    startSnapshotId: wire.start_snapshot_id ?? "",
    // null and absent both become []. A caller should not have to know which
    // of the two it received to decide whether to map over it.
    attachments: wire.attachments ?? [],
    deliveryConfig: wire.delivery_config ?? {},
  };
}

function toReview(wire: z.infer<typeof reviewSchema>): ReviewRequest {
  return {
    reviewRequestId: wire.review_request_id,
    workspaceId: wire.workspace_id ?? "",
    workId: wire.work_id ?? "",
    artifactId: wire.artifact_id ?? "",
    versionId: wire.version_id ?? "",
    accountId: wire.account_id ?? "",
    channel: wire.channel ?? "",
    snapshot: toSnapshot(wire.snapshot),
    status: wire.status ?? "",
    requestedBy: wire.requested_by ?? "",
    requestedAt: wire.requested_at ?? "",
    decidedBy: wire.decided_by ?? "",
    decidedAt: wire.decided_at ?? "",
    decisionNote: wire.decision_note ?? "",
    createdAt: wire.created_at ?? "",
    updatedAt: wire.updated_at ?? "",
  };
}

function toTransition(wire: z.infer<typeof transitionSchema>): ReviewTransition {
  return {
    transitionId: wire.transition_id,
    subjectKind: wire.subject_kind ?? "",
    subjectId: wire.subject_id ?? "",
    fromStatus: wire.from_status ?? "",
    toStatus: wire.to_status ?? "",
    reason: wire.reason ?? "",
    actorId: wire.actor_id ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function toDelivery(wire: z.infer<typeof deliverySchema>): DeliveryTask {
  return {
    deliveryTaskId: wire.delivery_task_id,
    workspaceId: wire.workspace_id ?? "",
    workId: wire.work_id ?? "",
    artifactId: wire.artifact_id ?? "",
    reviewRequestId: wire.review_request_id ?? "",
    channel: wire.channel ?? "",
    status: wire.status ?? "",
    scheduledAt: wire.scheduled_at ?? "",
    handoffMethod: wire.handoff_method ?? "",
    createdAt: wire.created_at ?? "",
    updatedAt: wire.updated_at ?? "",
    // Explicit === true rather than truthy: a backend that stops sending these
    // must read as "not due" and "nothing pending", never as an undefined that
    // a later `!` turns into the opposite.
    due: wire.due === true,
    pendingRegistration: wire.pending_registration === true,
  };
}

function toPublication(wire: z.infer<typeof publicationSchema>): PublicationRecord {
  return {
    publicationRecordId: wire.publication_record_id,
    workspaceId: wire.workspace_id ?? "",
    workId: wire.work_id ?? "",
    artifactId: wire.artifact_id ?? "",
    deliveryTaskId: wire.delivery_task_id ?? "",
    channel: wire.channel ?? "",
    status: wire.status ?? "",
    actorId: wire.actor_id ?? "",
    declaredBy: wire.declared_by ?? "",
    pageUrlOrContentId: wire.page_url_or_content_id ?? "",
    receiptNote: wire.receipt_note ?? "",
    verificationNote: wire.verification_note ?? "",
    publishedAt: wire.published_at ?? "",
    platformAccount: wire.platform_account ?? "",
    platformEdited: wire.platform_edited === true,
    editNote: wire.edit_note ?? "",
    versionMatch: wire.version_match ?? "",
    versionId: wire.version_id ?? "",
    // === true, not truthiness: a backend deployed without migration 533
    // sends nothing, and "absent" means "not an import".
    historicalImport: wire.historical_import === true,
    createdAt: wire.created_at ?? "",
  };
}

export function parseReview(data: unknown): ReviewRequest {
  const parsed = parseWithFallback(data, reviewSchema, { review_request_id: "" }, {
    endpoint: "content-reviews/detail",
  });
  return parsed.review_request_id ? toReview(parsed) : EMPTY_REVIEW;
}

export interface ReviewDetail {
  review: ReviewRequest;
  transitions: ReviewTransition[];
}

export function parseReviewDetail(data: unknown): ReviewDetail {
  const parsed = parseWithFallback<z.infer<typeof reviewDetailSchema>>(
    data,
    reviewDetailSchema,
    { review: null, transitions: [] },
    { endpoint: "content-reviews/detail" },
  );
  return {
    review: parsed.review?.review_request_id ? toReview(parsed.review) : EMPTY_REVIEW,
    transitions: (parsed.transitions ?? []).map(toTransition),
  };
}

export function parseReviews(data: unknown): ReviewRequest[] {
  const parsed = parseWithFallback(data, reviewListSchema, { reviews: [] }, {
    endpoint: "content-reviews/list",
  });
  return (parsed.reviews ?? []).map(toReview);
}

export function parseDeliveries(data: unknown): DeliveryTask[] {
  const parsed = parseWithFallback(data, deliveryListSchema, { deliveries: [] }, {
    endpoint: "content-deliveries/list",
  });
  return (parsed.deliveries ?? []).map(toDelivery);
}

export function parseDelivery(data: unknown): DeliveryTask {
  const parsed = parseWithFallback(data, deliverySchema, { delivery_task_id: "" }, {
    endpoint: "content-deliveries/detail",
  });
  return toDelivery(parsed);
}

export function parsePublications(data: unknown): PublicationRecord[] {
  const parsed = parseWithFallback(data, publicationListSchema, { publications: [] }, {
    endpoint: "content-publications/list",
  });
  return (parsed.publications ?? []).map(toPublication);
}

export function parsePublication(data: unknown): PublicationRecord {
  const parsed = parseWithFallback(data, publicationSchema, { publication_record_id: "" }, {
    endpoint: "content-publications/detail",
  });
  return toPublication(parsed);
}

/**
 * The current publication status is the latest record, not a stored field.
 *
 * The list arrives newest first; this is the whole of "what is it doing out
 * there now". A page that kept its own copy would be a second truth.
 */
export function currentPublication(records: PublicationRecord[]): PublicationRecord | null {
  return records.length > 0 ? (records[0] ?? null) : null;
}

/**
 * Whether a handover method means the piece was published. Always false.
 *
 * SOP 9.1: "导出成功、复制完成或交接给他人都不自动等于发布成功". It is a function
 * rather than an absence so that a future reader who wants handover to count as
 * publication has to change something that says why it does not.
 */
export function handoffMeansPublished(_method: string): boolean {
  return false;
}
