// The inbox page's rules, kept out of the page.
//
// Two reasons these are here rather than in the component: a second surface
// (desktop) should reuse the judgement rather than the markup, and a rule with
// a test beside it is a rule someone can change on purpose. The page renders
// what these return.

import { SOURCE_KINDS, SOURCE_STATUSES, type SourceBulkResult, type SourceStatus } from "./contract";

/** What the collect form holds while someone is filling it in. */
export interface SourceDraft {
  kind: string;
  url: string;
  content: string;
  title: string;
  tags: string;
  annotation: string;
  personalJudgement: string;
  historicalImport: boolean;
}

export function emptyDraft(): SourceDraft {
  return {
    kind: "pasted_text",
    url: "",
    content: "",
    title: "",
    tags: "",
    annotation: "",
    personalJudgement: "",
    historicalImport: false,
  };
}

/**
 * The field a draft is wrong about, or "" when it is ready.
 *
 * This mirrors the server's rule rather than replacing it: the server decides,
 * and a save still has to survive its answer. What this buys is telling someone
 * which box to fix before they press the button, instead of after.
 */
export function draftProblem(draft: SourceDraft): string {
  if (!(SOURCE_KINDS as readonly string[]).includes(draft.kind)) return "kind";
  if (draft.kind === "url") {
    if (!isWebURL(draft.url)) return "url";
    // A url source has no body: nothing has read the page, so there is no text
    // the link vouches for. What the person knows goes in the annotation.
    if (draft.content.trim() !== "") return "content";
    return "";
  }
  if (draft.content.trim() === "") return "content";
  if (draft.url.trim() !== "") return "url";
  return "";
}

export function canSubmitDraft(draft: SourceDraft, pending: boolean): boolean {
  return !pending && draftProblem(draft) === "";
}

/**
 * Whether a link is one a person can go back and read.
 *
 * http and https only. A `javascript:` or `file:` link is not a source, and
 * storing one invites a page to follow it.
 */
export function isWebURL(value: string): boolean {
  if (value.trim() === "") return false;
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    return false;
  }
  if (parsed.host === "") return false;
  return parsed.protocol === "http:" || parsed.protocol === "https:";
}

/** Tags as typed, split and trimmed. Blank entries are dropped, not rejected. */
export function parseTagInput(value: string): string[] {
  const seen = new Set<string>();
  for (const raw of value.split(/[,，\s]+/)) {
    const tag = raw.trim();
    if (tag !== "") seen.add(tag);
  }
  return [...seen];
}

/** The request body a draft becomes. Absent fields are left out, not sent empty. */
export function draftToRequest(draft: SourceDraft): Record<string, unknown> {
  const body: Record<string, unknown> = {
    kind: draft.kind,
    title: draft.title,
    tags: parseTagInput(draft.tags),
    annotation: draft.annotation,
    personal_judgement: draft.personalJudgement,
    historical_import: draft.historicalImport,
  };
  if (draft.kind === "url") body.url = draft.url;
  else body.content = draft.content;
  return body;
}

/**
 * What the duplicate hint says.
 *
 * `blocking` is always false. SOP §4 asks the system to point out the repeat
 * and stop; R-011 says the two collections keep their own annotations. A page
 * that refused the save, or offered to merge, would be doing the thing both
 * sentences rule out.
 */
export interface DuplicateHint {
  count: number;
  sourceIds: string[];
  blocking: false;
}

export function duplicateHint(sourceIds: string[]): DuplicateHint {
  return { count: sourceIds.length, sourceIds, blocking: false };
}

/**
 * The organising moves offered from a status.
 *
 * Not a linear pipeline: something archived can come back to the inbox,
 * because deciding it was not useful is a judgement that can be revisited.
 * What is never offered is the status it already has.
 */
export function organizeActions(status: string): SourceStatus[] {
  return SOURCE_STATUSES.filter((candidate) => candidate !== status);
}

/** How a bulk result reads: how many landed, and which did not. */
export interface BulkSummary {
  ok: number;
  failed: number;
  failedIds: string[];
}

export function bulkSummary(results: SourceBulkResult[]): BulkSummary {
  const failedIds = results.filter((result) => !result.ok).map((result) => result.sourceId);
  return { ok: results.length - failedIds.length, failed: failedIds.length, failedIds };
}

/**
 * Whether a bulk request is worth sending.
 *
 * Nothing selected, or nothing to apply, is not a request - it would come back
 * as a 400 that says what the button could have said.
 */
export function canSubmitBulk(selected: string[], addTags: string[], status: string): boolean {
  if (selected.length === 0) return false;
  return addTags.length > 0 || status !== "";
}
