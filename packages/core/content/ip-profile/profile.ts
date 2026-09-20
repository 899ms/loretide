import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// The account's expression profile (SOP §3.1), mirrored from Go.
//
// The source of truth is `server/internal/content/ip-profile/profile.go`. This
// file exists because EP-04's start screen has to answer "can this account
// start yet" without a round trip, and because the page has to render the same
// decision the server made.
//
// Two copies of a rule is how the two drift. `profile.test.ts` runs both
// against a committed decision matrix — `contracts/readiness-parity.json` —
// which the Go side asserts against too, so a change on either side that
// alters a decision turns one of them red.

/** Whether the creator has confirmed this item. Default is pending. */
export const FIELD_STATUSES = ["pending", "confirmed"] as const;
export type FieldStatus = (typeof FIELD_STATUSES)[number];

export interface TextField {
  value: string;
  status: FieldStatus;
}

export interface ListField {
  values: string[];
  status: FieldStatus;
}

export interface HoursField {
  value: number;
  status: FieldStatus;
}

/** The ten §3.1 fields plus the pasted style samples, in the doc's own order. */
export interface ExpressionProfile {
  audience: TextField;
  common_questions: TextField;
  experience: TextField;
  positioning: TextField;
  /** §3.1's "content direction" for the start condition (clarification Q3). */
  content_pillars: TextField;
  expression_style: TextField;
  forbidden_expressions: TextField;
  content_goals: TextField;
  primary_channels: ListField;
  weekly_hours: HoursField;
  style_samples: ListField;
}

export interface Readiness {
  can_start: boolean;
  missing: string[];
}

/** The keys Readiness names. They are field names so a page can point at the
 *  field rather than translating a sentence. */
export const MISSING_AUDIENCE = "audience";
export const MISSING_PILLARS = "content_pillars";
export const MISSING_CHANNELS = "primary_channels";
export const MISSING_WEEKLY_HOURS = "weekly_hours";

const textFieldSchema = z.object({
  value: z.string().catch(""),
  status: z.string().catch("pending"),
});
const listFieldSchema = z.object({
  values: z.array(z.string()).nullable().catch(null),
  status: z.string().catch("pending"),
});
const hoursFieldSchema = z.object({
  value: z.number().catch(0),
  status: z.string().catch("pending"),
});

// Lenient by design: an installed client talks to whatever backend is
// deployed, and an unexpected status must degrade to pending rather than
// blanking the page. A status this build does not recognise is NOT confirmed —
// failing towards "not yet confirmed" is the safe direction, because the only
// thing confirmation unlocks is "you may start".
const profileSchema = z.object({
  audience: textFieldSchema.optional(),
  common_questions: textFieldSchema.optional(),
  experience: textFieldSchema.optional(),
  positioning: textFieldSchema.optional(),
  content_pillars: textFieldSchema.optional(),
  expression_style: textFieldSchema.optional(),
  forbidden_expressions: textFieldSchema.optional(),
  content_goals: textFieldSchema.optional(),
  primary_channels: listFieldSchema.optional(),
  weekly_hours: hoursFieldSchema.optional(),
  style_samples: listFieldSchema.optional(),
});

export const profileReadSchema = z.object({
  revision_id: z.string().optional(),
  revision: z.number().optional(),
  profile: profileSchema.optional(),
  readiness: z
    .object({
      can_start: z.boolean().catch(false),
      missing: z.array(z.string()).nullable().catch(null),
    })
    .optional(),
  uses_neutral_expression: z.boolean().optional(),
});

export interface ProfileRead {
  revisionId: string;
  revision: number;
  profile: ExpressionProfile;
  /** What the SERVER decided. The page renders this; `profileReadiness` below
   *  is for deciding locally, e.g. before a save has landed. */
  readiness: Readiness;
  usesNeutralExpression: boolean;
}

function text(field?: { value: string; status: string }): TextField {
  return {
    value: field?.value ?? "",
    status: field?.status === "confirmed" ? "confirmed" : "pending",
  };
}

function list(field?: { values: string[] | null; status: string }): ListField {
  return {
    values: field?.values ?? [],
    status: field?.status === "confirmed" ? "confirmed" : "pending",
  };
}

function hours(field?: { value: number; status: string }): HoursField {
  return {
    value: field?.value ?? 0,
    status: field?.status === "confirmed" ? "confirmed" : "pending",
  };
}

/** An all-pending profile: what an account that has never confirmed reads as,
 *  and what a malformed response degrades to. Nothing is invented. */
export function emptyProfile(): ExpressionProfile {
  return {
    audience: text(),
    common_questions: text(),
    experience: text(),
    positioning: text(),
    content_pillars: text(),
    expression_style: text(),
    forbidden_expressions: text(),
    content_goals: text(),
    primary_channels: list(),
    weekly_hours: hours(),
    style_samples: list(),
  };
}

export function parseProfileRead(data: unknown): ProfileRead {
  const parsed = parseWithFallback(data, profileReadSchema, {} as z.infer<typeof profileReadSchema>, {
    endpoint: "content-accounts/profile",
  });
  const raw = parsed.profile;
  const profile: ExpressionProfile = {
    audience: text(raw?.audience),
    common_questions: text(raw?.common_questions),
    experience: text(raw?.experience),
    positioning: text(raw?.positioning),
    content_pillars: text(raw?.content_pillars),
    expression_style: text(raw?.expression_style),
    forbidden_expressions: text(raw?.forbidden_expressions),
    content_goals: text(raw?.content_goals),
    primary_channels: list(raw?.primary_channels),
    weekly_hours: hours(raw?.weekly_hours),
    style_samples: list(raw?.style_samples),
  };
  return {
    revisionId: parsed.revision_id ?? "",
    revision: parsed.revision ?? 0,
    profile,
    // The server's decision is preferred when it sent one; otherwise the same
    // rule is applied locally rather than claiming readiness nobody computed.
    readiness: parsed.readiness
      ? { can_start: parsed.readiness.can_start, missing: parsed.readiness.missing ?? [] }
      : profileReadiness(profile),
    usesNeutralExpression:
      parsed.uses_neutral_expression ?? usesNeutralExpression(profile),
  };
}

function confirmedText(field: TextField): boolean {
  return field.status === "confirmed" && field.value.trim() !== "";
}

function nonEmpty(values: string[]): string[] {
  return values.filter((value) => value.trim() !== "");
}

/**
 * SOP §3.1's minimum condition to start, and what is still missing.
 *
 * Only CONFIRMED fields count. A value the creator typed but has not confirmed
 * is exactly §3.1's "pending", and letting it satisfy a start condition would
 * make confirming decorative.
 *
 * Zero confirmed hours does not satisfy the time budget: confirming "0 hours a
 * week" says there is no time to spend. That reading is an interpretation, not
 * something §3.1 states outright; it is recorded in the contract so it can be
 * argued with, and the Go side reads it the same way.
 */
export function profileReadiness(profile: ExpressionProfile): Readiness {
  const missing: string[] = [];
  if (!confirmedText(profile.audience)) missing.push(MISSING_AUDIENCE);
  if (!confirmedText(profile.content_pillars)) missing.push(MISSING_PILLARS);
  if (
    profile.primary_channels.status !== "confirmed" ||
    nonEmpty(profile.primary_channels.values).length === 0
  ) {
    missing.push(MISSING_CHANNELS);
  }
  if (profile.weekly_hours.status !== "confirmed" || profile.weekly_hours.value <= 0) {
    missing.push(MISSING_WEEKLY_HOURS);
  }
  return { can_start: missing.length === 0, missing };
}

/**
 * §3.1's "use neutral expression and mark it when there is no style sample".
 *
 * Computed, never stored: a stored flag and the samples themselves would be
 * two answers to one question. It marks and does not block — §3.1 is explicit
 * that a missing sample must not stop material being recorded or writing being
 * done by hand — so it never appears in `profileReadiness`.
 */
export function usesNeutralExpression(profile: ExpressionProfile): boolean {
  return (
    profile.style_samples.status !== "confirmed" ||
    nonEmpty(profile.style_samples.values).length === 0
  );
}

/** True when this field has something the creator typed but has not confirmed. */
export function isPendingWithValue(field: TextField | ListField | HoursField): boolean {
  if (field.status === "confirmed") return false;
  if ("values" in field) return nonEmpty(field.values).length > 0;
  if (typeof field.value === "number") return field.value > 0;
  return field.value.trim() !== "";
}
