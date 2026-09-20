// How a page should read the expression-profile query.
//
// This exists because "the profile is empty" and "the profile could not be
// read" produce the same all-pending shape, and a page that cannot tell them
// apart reports a failed request as four missing fields. Keeping the mapping
// here rather than in a component means every surface answers the question the
// same way, and that the answer can be tested without a DOM.

import type { ProfileRead, Readiness } from "./profile";

/** What a page is looking at right now. */
export type ProfileReadState = "idle" | "loading" | "failed" | "ready";

/**
 * The parts of a query result this decision needs.
 *
 * `enabled` is separate from `isPending` on purpose: a query that is turned
 * off (no account selected yet) never resolves, so on its own `isPending`
 * would read as a spinner that never stops.
 */
export interface ProfileQueryLike {
  enabled: boolean;
  isPending: boolean;
  isError: boolean;
  data?: ProfileRead;
}

export function profileReadState(query: ProfileQueryLike): ProfileReadState {
  if (!query.enabled) return "idle";
  if (query.isError) return "failed";
  if (query.data) return "ready";
  if (query.isPending) return "loading";
  // Not pending, not an error, and no data: nothing the page can show. Calling
  // it ready would render the empty profile as if it were the account's.
  return "failed";
}

/**
 * The account's readiness, or the reason there isn't one.
 *
 * A failed read deliberately does NOT come back as `can_start: false` with an
 * empty `missing` list - that reads as "ready except for nothing", which is
 * how a blocked start ends up with no explanation.
 */
export type ReadinessView =
  | { kind: "ready"; readiness: Readiness }
  | { kind: "failed" }
  | { kind: "loading" }
  | { kind: "idle" };

export function readinessView(query: ProfileQueryLike): ReadinessView {
  const state = profileReadState(query);
  if (state === "ready" && query.data) {
    return { kind: "ready", readiness: query.data.readiness };
  }
  return { kind: state === "ready" ? "failed" : state };
}

/** Whether a page may present `data` as the account's own profile. */
export function profileIsTrustworthy(query: ProfileQueryLike): boolean {
  return profileReadState(query) === "ready";
}
