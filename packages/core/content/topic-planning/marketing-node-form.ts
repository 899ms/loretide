import type {
  LeadDays,
  MarketingNodeAccountInput,
  MarketingNodeInput,
  NodeRevision,
} from "./marketing-nodes";

// The marketing node form (specs/033 PR 3, contract §2.1 and §6).
//
// A revise submits the WHOLE revision: every revision row stores the full
// content, so a field the request leaves out is not "unchanged" but "empty in
// this revision". The form therefore starts from the current revision with
// every field carried over, and turns back into a complete input on submit.
//
// The lead time is held as the text of its input: "" is "not set", "0" is
// "no preparation needed". The two are never folded into one another.

export const MARKETING_NODE_KINDS = [
  "holiday",
  "industry",
  "brand_campaign",
  "marketing",
] as const;

export const MARKETING_NODE_DATE_CERTAINTIES = ["confirmed", "tentative"] as const;

// Server limits (server/internal/content/topic-planning/marketing_node.go).
export const MARKETING_NODE_LIMITS = {
  name: 200,
  goal: 2000,
  dateBasis: 1000,
  note: 2000,
  role: 200,
  accounts: 20,
  spanDays: 366,
  leadDays: 365,
  materials: 50,
} as const;

export interface MarketingNodeForm {
  name: string;
  kind: string;
  startsOn: string;
  endsOn: string;
  timezone: string;
  /** "" = not set; otherwise the typed number. */
  leadDays: string;
  accounts: MarketingNodeAccountInput[];
  goal: string;
  materialSourceIds: string[];
  dateCertainty: string;
  dateBasis: string;
  /** The note on this change; not part of the node's content. */
  note: string;
}

export type MarketingNodeFormField =
  | "name"
  | "kind"
  | "starts_on"
  | "ends_on"
  | "timezone"
  | "lead_days"
  | "accounts"
  | "goal"
  | "material_source_ids"
  | "date_certainty"
  | "date_basis"
  | "note";

/** A new node: the time zone is the brand's, the lead time is not set. */
export function emptyMarketingNodeForm(brandTimezone: string): MarketingNodeForm {
  return {
    name: "",
    kind: "marketing",
    startsOn: "",
    endsOn: "",
    timezone: brandTimezone,
    leadDays: "",
    accounts: [],
    goal: "",
    materialSourceIds: [],
    dateCertainty: "confirmed",
    dateBasis: "",
    note: "",
  };
}

function leadDaysText(leadDays: LeadDays): string {
  return leadDays.set ? String(leadDays.value) : "";
}

export function marketingNodeFormFromRevision(revision: NodeRevision): MarketingNodeForm {
  return {
    name: revision.name,
    kind: revision.kind,
    startsOn: revision.startsOn,
    endsOn: revision.endsOn,
    timezone: revision.timezone,
    leadDays: leadDaysText(revision.leadDays),
    accounts: revision.accounts.map((account) => ({ ...account })),
    goal: revision.goal,
    materialSourceIds: [...revision.materialSourceIds],
    dateCertainty: revision.dateCertainty,
    dateBasis: revision.dateBasis,
    note: "",
  };
}

/** "" is not set; anything else must already have passed marketingNodeFormErrors. */
function leadDaysOf(text: string): LeadDays {
  const trimmed = text.trim();
  return trimmed === "" ? { set: false } : { set: true, value: Number(trimmed) };
}

export function marketingNodeFormToInput(form: MarketingNodeForm): MarketingNodeInput {
  return {
    name: form.name.trim(),
    kind: form.kind,
    startsOn: form.startsOn.trim(),
    endsOn: form.endsOn.trim(),
    timezone: form.timezone.trim(),
    leadDays: leadDaysOf(form.leadDays),
    accounts: form.accounts.map((account) => ({
      accountId: account.accountId,
      role: account.role,
    })),
    goal: form.goal,
    materialSourceIds: [...form.materialSourceIds],
    dateCertainty: form.dateCertainty,
    dateBasis: form.dateBasis,
  };
}

/**
 * Whether the node's content differs from the revision. The server refuses a
 * revision identical to the current one, so saving is only offered when this
 * is true. The note alone is not a change.
 */
export function marketingNodeFormChanged(
  form: MarketingNodeForm,
  revision: NodeRevision,
): boolean {
  const next = marketingNodeFormToInput(form);
  const current = marketingNodeFormToInput(marketingNodeFormFromRevision(revision));
  return JSON.stringify(next) !== JSON.stringify(current);
}

function calendarDay(value: string): number | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const [y, m, d] = value.split("-").map(Number) as [number, number, number];
  const time = Date.UTC(y, m - 1, d);
  const date = new Date(time);
  if (
    date.getUTCFullYear() !== y ||
    date.getUTCMonth() !== m - 1 ||
    date.getUTCDate() !== d
  ) {
    return null;
  }
  return time / 86_400_000;
}

// The same rule the server applies with time.LoadLocation: a real IANA name,
// never "" or "Local", and no fixed offset (Intl accepts "+08:00", Go does not).
function validTimezone(name: string): boolean {
  const trimmed = name.trim();
  if (trimmed === "" || trimmed === "Local" || /^[+-]/.test(trimmed)) return false;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: trimmed });
    return true;
  } catch {
    return false;
  }
}

function tooLong(value: string, limit: number): boolean {
  return [...value].length > limit;
}

/**
 * The fields the server would refuse, checked before saving so the page can
 * name them. An empty list means the form may be submitted.
 */
export function marketingNodeFormErrors(form: MarketingNodeForm): MarketingNodeFormField[] {
  const errors: MarketingNodeFormField[] = [];
  const name = form.name.trim();
  if (name === "" || tooLong(name, MARKETING_NODE_LIMITS.name)) errors.push("name");
  if (!(MARKETING_NODE_KINDS as readonly string[]).includes(form.kind)) errors.push("kind");
  const starts = calendarDay(form.startsOn.trim());
  const ends = calendarDay(form.endsOn.trim());
  if (starts === null) errors.push("starts_on");
  if (ends === null) {
    errors.push("ends_on");
  } else if (
    starts !== null &&
    (ends < starts || ends - starts + 1 > MARKETING_NODE_LIMITS.spanDays)
  ) {
    errors.push("ends_on");
  }
  if (!validTimezone(form.timezone)) errors.push("timezone");
  const lead = form.leadDays.trim();
  if (
    lead !== "" &&
    (!/^\d+$/.test(lead) || Number(lead) > MARKETING_NODE_LIMITS.leadDays)
  ) {
    errors.push("lead_days");
  }
  if (
    form.accounts.length > MARKETING_NODE_LIMITS.accounts ||
    form.accounts.some((account) => tooLong(account.role, MARKETING_NODE_LIMITS.role))
  ) {
    errors.push("accounts");
  }
  if (tooLong(form.goal, MARKETING_NODE_LIMITS.goal)) errors.push("goal");
  if (form.materialSourceIds.length > MARKETING_NODE_LIMITS.materials) {
    errors.push("material_source_ids");
  }
  if (
    !(MARKETING_NODE_DATE_CERTAINTIES as readonly string[]).includes(form.dateCertainty)
  ) {
    errors.push("date_certainty");
  }
  if (tooLong(form.dateBasis, MARKETING_NODE_LIMITS.dateBasis)) errors.push("date_basis");
  if (tooLong(form.note, MARKETING_NODE_LIMITS.note)) errors.push("note");
  return errors;
}

/** Add or remove an applicable account; the other accounts keep their roles. */
export function setMarketingNodeAccount(
  form: MarketingNodeForm,
  accountId: string,
  included: boolean,
): MarketingNodeForm {
  const present = form.accounts.some((account) => account.accountId === accountId);
  if (included) {
    return present
      ? form
      : { ...form, accounts: [...form.accounts, { accountId, role: "" }] };
  }
  return {
    ...form,
    accounts: form.accounts.filter((account) => account.accountId !== accountId),
  };
}

export function setMarketingNodeAccountRole(
  form: MarketingNodeForm,
  accountId: string,
  role: string,
): MarketingNodeForm {
  return {
    ...form,
    accounts: form.accounts.map((account) =>
      account.accountId === accountId ? { ...account, role } : account,
    ),
  };
}

export function setMarketingNodeMaterial(
  form: MarketingNodeForm,
  sourceId: string,
  included: boolean,
): MarketingNodeForm {
  const present = form.materialSourceIds.includes(sourceId);
  if (included) {
    return present
      ? form
      : { ...form, materialSourceIds: [...form.materialSourceIds, sourceId] };
  }
  return {
    ...form,
    materialSourceIds: form.materialSourceIds.filter((id) => id !== sourceId),
  };
}
