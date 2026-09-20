import {
  DELIVERY_STATUSES,
  REVIEW_STATUSES,
  type DeliveryStatus,
  type ReviewStatus,
} from "./contract";

// The two state machines, restated for the page so a button can be disabled
// rather than hidden, and so the reason can be shown.
//
// They are the server's rules; `states.test.ts` compares this table against the
// Go source. The server still decides - this copy exists to keep the page from
// offering a move that will be refused, not to replace the check.
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md

const REVIEW_TRANSITIONS: Record<ReviewStatus, readonly ReviewStatus[]> = {
  pending: ["changes_requested", "approved", "rejected", "cancelled"],
  // Terminal. Re-submitting is a NEW request, not a revival of this one.
  changes_requested: [],
  approved: [],
  rejected: [],
  cancelled: [],
};

const DELIVERY_TRANSITIONS: Record<DeliveryStatus, readonly DeliveryStatus[]> = {
  draft: ["ready", "cancelled"],
  ready: ["scheduled", "handed_off", "held", "cancelled"],
  scheduled: ["handed_off", "held", "cancelled"],
  held: ["ready", "cancelled"],
  handed_off: [],
  cancelled: [],
};

export function canTransitionReview(from: string, to: string): boolean {
  const allowed = REVIEW_TRANSITIONS[from as ReviewStatus];
  return allowed !== undefined && allowed.includes(to as ReviewStatus);
}

export function canTransitionDelivery(from: string, to: string): boolean {
  const allowed = DELIVERY_TRANSITIONS[from as DeliveryStatus];
  return allowed !== undefined && allowed.includes(to as DeliveryStatus);
}

/** Every delivery status, with whether it can be reached from `from`. The page
 *  shows all six and disables the ones that cannot: hiding a control says the
 *  product does not have it, which is a different and wrong message. */
export function deliveryMoves(from: string): { status: DeliveryStatus; allowed: boolean }[] {
  return DELIVERY_STATUSES.map((status) => ({
    status,
    allowed: canTransitionDelivery(from, status),
  }));
}

export function reviewMoves(from: string): { status: ReviewStatus; allowed: boolean }[] {
  return REVIEW_STATUSES.filter((status) => status !== "pending").map((status) => ({
    status,
    allowed: canTransitionReview(from, status),
  }));
}

/** Which extra field a move needs, or null. The page asks for it BEFORE the
 *  request rather than surfacing a 400 afterwards; the server checks again. */
export function requiredFieldFor(to: string): "scheduled_at" | "handoff_method" | "reason" | null {
  switch (to) {
    case "scheduled":
      return "scheduled_at";
    case "handed_off":
      return "handoff_method";
    case "held":
    case "cancelled":
      // SOP 9.2: "运营者可标记实际发布失败、延后或取消，并注明原因".
      return "reason";
    default:
      return null;
  }
}

/** Which extra field a publication status needs, or null. */
export function publicationRequiredFields(status: string): string[] {
  switch (status) {
    case "reported_published":
      return ["page_url_or_content_id"];
    case "verified_published":
      return ["page_url_or_content_id", "verification_note"];
    case "failed":
    case "removed":
      return ["receipt_note"];
    default:
      // A server-driven enum switch needs a default branch: a status this
      // build has not heard of asks for nothing extra rather than blocking the
      // form on a requirement it cannot name.
      return [];
  }
}

/** Past `draft`, a task needs an approved review behind it. */
export function requiresApprovedReview(status: string): boolean {
  return status !== "draft";
}
