import {
  emptyOperatingRules,
  readCadence,
  readObservation,
  type ObservationSource,
  type OperatingRules,
} from "./operating-rules";

// What the operating-rules form holds, what it may submit, and how each value
// should read once it is stored.
//
// These are rules, not layout, so they live here with node tests rather than
// inside JSX. The page asks `canSaveDraft` whether the button is live,
// `draftProblem` which field to point at, and the two `*Display` functions what
// to put in a row.
//
// Every number in this form is held as a STRING. That is the whole point: ""
// is a box nobody filled in and "0" is a decision somebody made, and a
// `number` field would need a sentinel to tell them apart - which is exactly
// the sentinel that eventually gets read as a measurement. `Number("")` is 0
// in JavaScript, so anything that parsed on the way in would collapse the two
// before the request was even built.
//
// Contract: specs/029-operating-rules/contracts/operating-rules.md

/** The form's own state. Keys absent from a map are channels nobody has
 *  touched; a key mapped to "" is a box that was cleared. */
export interface RulesDraft {
  cadence: Record<string, string>;
  templates: Record<string, string>;
  observationDefault: string;
  observationByChannel: Record<string, string>;
}

export type DraftProblemReason = "not-a-number" | "negative" | "not-a-whole-number";

export interface RulesDraftProblem {
  field: string;
  reason: DraftProblemReason;
}

/** Reads a typed count. Blank is "nobody decided" and is not an error. */
export function parseCount(raw: string): { ok: true; value: number | null } | { ok: false; reason: DraftProblemReason } {
  const text = raw.trim();
  if (text === "") return { ok: true, value: null };
  const parsed = Number(text);
  if (!Number.isFinite(parsed)) return { ok: false, reason: "not-a-number" };
  if (!Number.isInteger(parsed)) return { ok: false, reason: "not-a-whole-number" };
  if (parsed < 0) return { ok: false, reason: "negative" };
  return { ok: true, value: parsed };
}

export function draftFromRules(rules: OperatingRules): RulesDraft {
  const cadence: Record<string, string> = {};
  for (const [platform, value] of Object.entries(rules.cadence)) {
    cadence[platform] = String(value);
  }
  const templates: Record<string, string> = {};
  for (const [platform, template] of Object.entries(rules.templates)) {
    templates[platform] = template.note;
  }
  const observationByChannel: Record<string, string> = {};
  for (const [platform, days] of Object.entries(rules.observation.byChannel)) {
    observationByChannel[platform] = String(days);
  }
  return {
    cadence,
    templates,
    // A brand with no window gets an empty box, never a "0" it did not choose.
    observationDefault:
      rules.observation.default === undefined ? "" : String(rules.observation.default),
    observationByChannel,
  };
}

/**
 * The draft as the endpoint takes it.
 *
 * A blank box drops the key entirely rather than sending 0: the server reads an
 * absent key as "nobody decided", and that is the state a cleared box is in.
 * An empty template note is dropped for the same reason - a channel with no
 * note and a channel with an empty note are the same thing, and keeping the
 * second would make the stored blob grow every time somebody opened the page.
 */
export function draftToRules(draft: RulesDraft, reviewRule: string): OperatingRules {
  const rules = emptyOperatingRules();
  rules.reviewRule = reviewRule;
  for (const [platform, raw] of Object.entries(draft.cadence)) {
    const parsed = parseCount(raw);
    if (parsed.ok && parsed.value !== null) rules.cadence[platform] = parsed.value;
  }
  for (const [platform, note] of Object.entries(draft.templates)) {
    if (note.trim() !== "") rules.templates[platform] = { note };
  }
  const globalWindow = parseCount(draft.observationDefault);
  if (globalWindow.ok && globalWindow.value !== null) {
    rules.observation.default = globalWindow.value;
  }
  for (const [platform, raw] of Object.entries(draft.observationByChannel)) {
    const parsed = parseCount(raw);
    if (parsed.ok && parsed.value !== null) rules.observation.byChannel[platform] = parsed.value;
  }
  return rules;
}

/** The first box that is not usable, or null. */
export function draftProblem(draft: RulesDraft): RulesDraftProblem | null {
  for (const [platform, raw] of Object.entries(draft.cadence)) {
    const parsed = parseCount(raw);
    if (!parsed.ok) return { field: `cadence.${platform}`, reason: parsed.reason };
  }
  const globalWindow = parseCount(draft.observationDefault);
  if (!globalWindow.ok) return { field: "observation.default", reason: globalWindow.reason };
  for (const [platform, raw] of Object.entries(draft.observationByChannel)) {
    const parsed = parseCount(raw);
    if (!parsed.ok) return { field: `observation.by_channel.${platform}`, reason: parsed.reason };
  }
  return null;
}

export function canSaveDraft(draft: RulesDraft): boolean {
  return draftProblem(draft) === null;
}

/**
 * How a stored cadence should read.
 *
 * THREE cases, and the first two are the reason this function exists: a
 * channel nobody has decided about and a channel deliberately set to zero look
 * identical if the page just prints the number. "Not set" is an invitation;
 * "nothing this week" is a plan.
 */
export type CadenceDisplay =
  | { kind: "unset" }
  | { kind: "paused" }
  | { kind: "count"; value: number };

export function cadenceDisplay(rules: OperatingRules, platform: string): CadenceDisplay {
  const { value, stored } = readCadence(rules, platform);
  if (!stored) return { kind: "unset" };
  if (value === 0) return { kind: "paused" };
  return { kind: "count", value };
}

/**
 * How a stored observation window should read.
 *
 * Carries where it came from, so a row can say "this channel is set to 7 days"
 * rather than showing 7 and leaving the reader to guess whether the brand-wide
 * value is in play.
 */
export type ObservationDisplay =
  | { kind: "unset" }
  | { kind: "days"; days: number; source: Exclude<ObservationSource, "none"> };

export function observationDisplay(rules: OperatingRules, platform: string): ObservationDisplay {
  const { days, source } = readObservation(rules, platform);
  if (source === "none") return { kind: "unset" };
  return { kind: "days", days, source };
}
