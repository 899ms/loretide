import type { PendingFeedback } from "./contract";

// SOP 2's fifth workbench item: 待补录的反馈.
//
// This file exists because specs/026's dashboard needs the same derivation the
// server does, and 026 was written when this module did not exist - its
// FR-005b said the block shows "暂不可用". The ruling on Q6 made updating that
// part of this card's storage PR, and this is the function the dashboard uses.
//
// The time logic is the server's, and it stays there. SOP 3.2's brand-level
// 反馈观察时点 landed in specs/029, so "has it been long enough" now has an
// answer - read out of the brand's settings on every call. This file never
// computes it: a number of days living in the client would be a second rule,
// out of step with the one the operator can actually see and change.
//
// What reaches here is the server's verdict per row, and all this file does is
// decide which sentence goes with it.

/** What the dashboard's fifth block shows. */
export interface PendingFeedbackSummary {
  /** How many published pieces nobody has recorded numbers for. */
  count: number;
  /** The first few, so the block can name them rather than showing a number
   *  on its own - 026's FR-002: every row says which thing it is. */
  items: PendingFeedback[];
  /** True when there is nothing to do, which is a result rather than an empty
   *  state to apologise for. */
  empty: boolean;
}

/** How many rows the dashboard block shows before "and N more". */
export const DASHBOARD_PREVIEW = 5;

export function pendingFeedbackSummary(
  pending: PendingFeedback[],
  preview = DASHBOARD_PREVIEW,
): PendingFeedbackSummary {
  return {
    count: pending.length,
    items: pending.slice(0, preview),
    empty: pending.length === 0,
  };
}

/**
 * A one-line label for a pending row.
 *
 * It names the channel and the publication record rather than showing an id on
 * its own; an operator reading the workbench has to be able to tell which piece
 * this is without opening it.
 */
export function describePending(item: PendingFeedback): string {
  const parts = [item.channel || "unknown channel"];
  if (item.publishedAt) parts.push(item.publishedAt);
  return parts.join(" · ");
}

/**
 * Whether the dashboard block still has to say the feature is unavailable.
 *
 * 026's FR-005b required that wording while this module did not exist. It is a
 * function rather than a deleted line so the dashboard has something to call,
 * and so the day the answer changes there is one place it changes.
 */
export function feedbackBlockIsAvailable(): boolean {
  return true;
}

/**
 * Which sentence a pending row gets, from the server's `due`.
 *
 * Three answers, and keeping the last two apart is the whole point. "Nobody
 * set a window" is not "the window elapsed": the first is a setting waiting to
 * be filled in, the second is work waiting to be done, and a page that printed
 * the same line for both would be reporting a measurement it never took.
 *
 * "not_yet" is not among them because such a record is not on this list; if a
 * backend ever sent one it would land in "unstated" rather than being dressed
 * up as either of the other two.
 */
export type PendingDueNote = "passed" | "unknown" | "unstated";

export function pendingDueNote(due: string): PendingDueNote {
  switch (due) {
    case "passed":
      return "passed";
    case "unknown":
      return "unknown";
    default:
      // Includes "" from a backend that predates 029 and anything this build
      // has not heard of. Saying nothing beats saying the wrong one of two.
      return "unstated";
  }
}
