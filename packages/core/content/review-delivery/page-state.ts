import type { DeliveryTask, PublicationRecord, ReviewRequest } from "./contract";
import { canTransitionDelivery, requiredFieldFor } from "./states";

// The review and delivery pages' rules, away from the pages.
//
// Everything here is a decision a page would otherwise take inside JSX: whether
// a button can do anything, which hint sits under a control, what the current
// publication state of a piece is. This repository writes no UI unit tests, so
// a rule left in a component is a rule nothing can check.
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md

/** Why "submit for review" cannot be pressed, or null when it can. The page
 *  shows the reason rather than a dead button with no explanation. */
export type SubmitBlocker = "notAChannelDraft" | "noVersion" | "noAccount" | null;

/**
 * Whether this document may be submitted, and if not, why.
 *
 * Only channel cuts: SOP 8's object of review is "具体渠道、具体文档版本及附件、
 * 具体交付配置的快照", and a body has no channel to be reviewed against. The
 * server refuses it too - this exists so the page can say so before the click
 * rather than surface a 400 after it.
 */
export function submitBlocker(
  kind: string,
  versionCount: number,
  accountId: string,
): SubmitBlocker {
  if (kind !== "channel_draft") return "notAChannelDraft";
  if (versionCount === 0) return "noVersion";
  if (!accountId) return "noAccount";
  return null;
}

/** A request whose disposition is already made cannot be changed. There is no
 *  endpoint for it either; this keeps the page from offering one. */
export function isDecided(status: string): boolean {
  return status !== "" && status !== "pending";
}

/**
 * The pending request for this version, if there is one.
 *
 * Per VERSION, not per document: SOP 8 says an approval belongs to the
 * combination it was granted for, so a newer version has no request until
 * somebody submits one.
 */
export function pendingRequestFor(
  requests: ReviewRequest[],
  versionId: string,
): ReviewRequest | null {
  return (
    requests.find(
      (request) => request.versionId === versionId && request.status === "pending",
    ) ?? null
  );
}

/** The newest request bound to this version, decided or not. */
export function latestRequestFor(
  requests: ReviewRequest[],
  versionId: string,
): ReviewRequest | null {
  return requests.find((request) => request.versionId === versionId) ?? null;
}

/**
 * SOP 8's personal mode: "审核通过并安排交接" is one click and TWO records.
 *
 * The steps are listed rather than written into the click handler so the page
 * cannot quietly collapse them into one call, and so the count is something a
 * test can check. Each step is its own request, in order; the second only runs
 * if the first succeeded.
 */
export const APPROVE_AND_SCHEDULE_STEPS = ["approve", "createDeliveryTask"] as const;

export type CombinedStep = (typeof APPROVE_AND_SCHEDULE_STEPS)[number];

/** Whether the combined action is offered. Only for a request still waiting:
 *  it starts with an approval, and an approval only follows `pending`. */
export function canApproveAndSchedule(status: string): boolean {
  return status === "pending";
}

/** What the current publication state of a piece is, for a summary line.
 *
 *  `pendingRegistration` is SOP 9.2's 待登记: the piece was handed over and
 *  nobody has written anything down. It says nothing about the platform - the
 *  SOP's own next clause is "不推断平台状态". */
export interface PublicationSummary {
  status: string;
  pendingRegistration: boolean;
  recordCount: number;
}

export function publicationSummary(
  records: PublicationRecord[],
  tasks: DeliveryTask[],
): PublicationSummary {
  const latest = records.length > 0 ? records[0] : undefined;
  return {
    status: latest?.status ?? "",
    // Derived from what the server already derived per task. A piece is
    // pending registration when any of its handovers is.
    pendingRegistration: records.length === 0 && tasks.some((task) => task.pendingRegistration),
    recordCount: records.length,
  };
}

/** Which delivery statuses this task can be moved to right now, each with the
 *  extra field that move needs. Illegal ones are returned too, disabled: hiding
 *  a control says the product does not have the move, which is a different and
 *  wrong message. */
export interface DeliveryMoveOption {
  status: string;
  allowed: boolean;
  requires: "scheduled_at" | "handoff_method" | "reason" | null;
}

export function deliveryMoveOptions(from: string, statuses: readonly string[]): DeliveryMoveOption[] {
  return statuses.map((status) => ({
    status,
    allowed: canTransitionDelivery(from, status),
    requires: requiredFieldFor(status),
  }));
}

/**
 * Whether the advance form has everything the chosen move needs.
 *
 * The same rule the server applies, asked before the request so the person is
 * told what is missing while they can still type it. The server checks again -
 * this is a courtesy, not the check.
 */
export function advanceReady(
  to: string,
  values: { scheduledAt?: string; handoffMethod?: string; reason?: string },
): boolean {
  switch (requiredFieldFor(to)) {
    case "scheduled_at":
      return !!values.scheduledAt;
    case "handoff_method":
      return !!values.handoffMethod;
    case "reason":
      return !!values.reason?.trim();
    default:
      return to !== "";
  }
}

/** Whether the publication form has what this status requires. Mirrors the
 *  server's conditional rules so the person sees the gap before the 400. */
export function recordReady(
  status: string,
  values: { pageUrlOrContentId?: string; verificationNote?: string; receiptNote?: string },
): boolean {
  switch (status) {
    case "reported_published":
      return !!values.pageUrlOrContentId?.trim();
    case "verified_published":
      return !!values.pageUrlOrContentId?.trim() && !!values.verificationNote?.trim();
    case "failed":
    case "removed":
      return !!values.receiptNote?.trim();
    case "unknown":
      return true;
    default:
      // A status this build has not heard of is not blocked on a requirement
      // it cannot name.
      return status !== "";
  }
}

/**
 * The model-backed entry points on these pages, with the reason each is off.
 *
 * Listed rather than written into the page so the page cannot quietly render
 * one of them, and so the count is something a reader can check. Constitution
 * IX keeps real executors disabled; EP-08 is what turns these on. Nothing here
 * fabricates a verification result.
 */
export const REVIEW_AI_ENTRY_POINTS = [
  { id: "autoSelfCheck" },
  { id: "autoVerifyPublication" },
] as const;

export type ReviewAIEntryPointId = (typeof REVIEW_AI_ENTRY_POINTS)[number]["id"];

/** Every entry point is unavailable in this phase. A function rather than a
 *  constant `false` so the day one of them turns on, the callers already ask. */
export function reviewAIEntryPointEnabled(_id: ReviewAIEntryPointId): boolean {
  return false;
}

/**
 * Whether the task is being shown as due.
 *
 * The server derives it; the page only reads it. Stated as a function so that a
 * page which starts computing its own "due" from the timestamp has to delete
 * this one first - and so this file is the single place that says the planned
 * time is never acted on.
 */
export function isShownAsDue(task: DeliveryTask): boolean {
  return task.due === true;
}
