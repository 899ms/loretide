import {
  type ExpressionProfile,
  type FieldStatus,
  type HoursField,
  type ListField,
  type TextField,
} from "./profile";

// The editing model behind the §3.1 profile card.
//
// It lives here rather than in the page because everything interesting about
// the card is a rule, not a layout: which fields are confirmed together, what
// happens to a confirmed field when it is edited again, and how a list of
// channels survives a round trip through a text box. A rule inside JSX is a
// rule nobody can test, and this repository does not write UI unit tests.

/** Every field of the profile, addressed by its wire name. */
export type ProfileFieldKey = keyof ExpressionProfile;

/**
 * One editable group, in §3.1's order.
 *
 * Grouping is what makes confirmation usable: §3.1 asks eleven questions and
 * confirming them one at a time would be eleven revisions. Each group confirms
 * together, and one confirmation is one revision.
 */
export type ProfileGroupId = "audience" | "identity" | "direction" | "style" | "capacity";

export interface ProfileGroup {
  id: ProfileGroupId;
  fields: ProfileFieldKey[];
}

export const PROFILE_GROUPS: readonly ProfileGroup[] = [
  { id: "audience", fields: ["audience", "common_questions"] },
  { id: "identity", fields: ["experience", "positioning"] },
  { id: "direction", fields: ["content_pillars", "content_goals"] },
  {
    id: "style",
    fields: ["expression_style", "forbidden_expressions", "style_samples"],
  },
  { id: "capacity", fields: ["primary_channels", "weekly_hours"] },
];

/** Which fields carry a list of entries rather than one answer. */
export const LIST_FIELDS: readonly ProfileFieldKey[] = ["primary_channels", "style_samples"];

/** The one numeric field. */
export const HOURS_FIELD: ProfileFieldKey = "weekly_hours";

/**
 * The profile as text, one string per field.
 *
 * Lists are one entry per line and hours are a decimal string, because both
 * are edited in a text control and a draft that is already a string cannot
 * lose a half-typed number to a parse.
 */
export type ProfileDraft = Record<ProfileFieldKey, string>;

export function isListField(key: ProfileFieldKey): boolean {
  return LIST_FIELDS.includes(key);
}

function fieldText(profile: ExpressionProfile, key: ProfileFieldKey): string {
  if (isListField(key)) return (profile[key] as ListField).values.join("\n");
  if (key === HOURS_FIELD) {
    const hours = (profile[key] as HoursField).value;
    // A never-filled-in budget shows as blank, not as a confident "0". Zero is
    // a real answer ("no time this week") and must be typed to mean it.
    return hours === 0 ? "" : String(hours);
  }
  return (profile[key] as TextField).value;
}

export function profileToDraft(profile: ExpressionProfile): ProfileDraft {
  const draft = {} as ProfileDraft;
  for (const group of PROFILE_GROUPS) {
    for (const key of group.fields) draft[key] = fieldText(profile, key);
  }
  return draft;
}

export function splitEntries(text: string): string[] {
  return text
    .split("\n")
    .map((entry) => entry.trim())
    .filter((entry) => entry !== "");
}

export function parseHours(text: string): number {
  const hours = Number(text.trim());
  // NaN, infinities and negatives all mean "this is not a week's worth of
  // hours". They become zero, which readiness already treats as unmet, rather
  // than reaching the server as something it has to refuse.
  if (!Number.isFinite(hours) || hours < 0) return 0;
  return Math.floor(hours);
}

function statusFor(previous: FieldStatus, changed: boolean): FieldStatus {
  // Editing a confirmed answer takes the confirmation back. The alternative is
  // a field that reads 已确认 while showing text nobody has confirmed, which
  // would make the badge a lie and the readiness derived from it wrong.
  return changed ? "pending" : previous;
}

/**
 * Fold the draft back into a profile, keeping each field's status unless the
 * text changed. Nothing here invents content: a blank draft field produces a
 * blank value, never a guess.
 */
export function draftToProfile(base: ExpressionProfile, draft: ProfileDraft): ExpressionProfile {
  const next = { ...base } as ExpressionProfile;
  for (const group of PROFILE_GROUPS) {
    for (const key of group.fields) {
      const text = draft[key] ?? "";
      if (isListField(key)) {
        const field = base[key] as ListField;
        const values = splitEntries(text);
        const changed = values.join("\n") !== field.values.join("\n");
        (next[key] as ListField) = { values, status: statusFor(field.status, changed) };
      } else if (key === HOURS_FIELD) {
        const field = base[key] as HoursField;
        const value = parseHours(text);
        (next[key] as HoursField) = {
          value,
          status: statusFor(field.status, value !== field.value),
        };
      } else {
        const field = base[key] as TextField;
        (next[key] as TextField) = {
          value: text,
          status: statusFor(field.status, text !== field.value),
        };
      }
    }
  }
  return next;
}

function hasContent(profile: ExpressionProfile, key: ProfileFieldKey): boolean {
  if (isListField(key)) return (profile[key] as ListField).values.length > 0;
  if (key === HOURS_FIELD) return (profile[key] as HoursField).value > 0;
  return (profile[key] as TextField).value.trim() !== "";
}

/**
 * Confirm one group's fields.
 *
 * A blank field cannot be confirmed - there is nothing to say yes to - so it
 * stays pending. §3.1 is explicit that pending is an ordinary state and not an
 * error, so this is not a failure and the rest of the group still confirms.
 * Fields outside the group are returned untouched, which is what makes a group
 * confirmation a group confirmation rather than a whole-profile one.
 */
export function confirmGroup(profile: ExpressionProfile, group: ProfileGroup): ExpressionProfile {
  const next = { ...profile } as ExpressionProfile;
  for (const key of group.fields) {
    const status: FieldStatus = hasContent(profile, key) ? "confirmed" : "pending";
    if (isListField(key)) {
      (next[key] as ListField) = { ...(profile[key] as ListField), status };
    } else if (key === HOURS_FIELD) {
      (next[key] as HoursField) = { ...(profile[key] as HoursField), status };
    } else {
      (next[key] as TextField) = { ...(profile[key] as TextField), status };
    }
  }
  return next;
}

/** True when the draft says something the stored profile does not. */
export function draftDiffers(base: ExpressionProfile, draft: ProfileDraft): boolean {
  const stored = profileToDraft(base);
  return PROFILE_GROUPS.some((group) =>
    group.fields.some((key) => (draft[key] ?? "") !== stored[key]),
  );
}
