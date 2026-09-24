import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";
import { parseTopicCard, type TopicCard } from "./contract";

// Marketing nodes (specs/033). Contract:
// specs/033-marketing-nodes/contracts/marketing-nodes.md §2, §3.2, §6.
//
// Two rules shape every parser here (constitution VI):
// - A value this client does not know (a new phase, change kind or import
//   outcome) is read as { kind: "unknown", raw } and kept, never dropped.
// - A malformed item in a list drops that item only; the rest of the list
//   still renders.

/** Lead time: "not set" and 0 ("no preparation needed") are different answers. */
export type LeadDays = { set: false } | { set: true; value: number };

export const NODE_PHASES = [
  "before_preparation",
  "preparing",
  "live",
  "ended",
  "before_start_unknown_lead",
] as const;
export type KnownNodePhase = (typeof NODE_PHASES)[number];
export type NodePhase = { kind: KnownNodePhase } | { kind: "unknown"; raw: string };

export const NODE_CHANGE_KINDS = [
  "create",
  "edit",
  "reschedule",
  "confirm",
  "cancel",
] as const;
export type KnownNodeChangeKind = (typeof NODE_CHANGE_KINDS)[number];
export type NodeChangeKind =
  | { kind: KnownNodeChangeKind }
  | { kind: "unknown"; raw: string };

export const IMPORT_OUTCOMES = ["created", "duplicate", "invalid"] as const;
export type KnownImportOutcome = (typeof IMPORT_OUTCOMES)[number];
export type ImportOutcome =
  | { kind: KnownImportOutcome }
  | { kind: "unknown"; raw: string };

function known<T extends string>(values: readonly T[]) {
  return (raw: string): { kind: T } | { kind: "unknown"; raw: string } =>
    (values as readonly string[]).includes(raw)
      ? { kind: raw as T }
      : { kind: "unknown", raw };
}

const toPhase = known(NODE_PHASES);
const toChangeKind = known(NODE_CHANGE_KINDS);
const toOutcome = known(IMPORT_OUTCOMES);

// Missing and null both mean "not set"; a number is a value, 0 included. It is
// never read as 0 when absent.
const leadDaysSchema = z
  .number()
  .int()
  .nullable()
  .optional()
  .transform((value): LeadDays =>
    value === null || value === undefined ? { set: false } : { set: true, value },
  );

const nodeAccountSchema = z
  .object({ account_id: z.string(), role: z.string().catch("") })
  .loose()
  .transform((account) => ({ accountId: account.account_id, role: account.role }));

export const nodeRevisionSchema = z
  .object({
    revision_id: z.string(),
    node_id: z.string(),
    workspace_id: z.string(),
    revision: z.number().int().positive(),
    change_kind: z.string(),
    status_after: z.string(),
    name: z.string(),
    kind: z.string(),
    starts_on: z.string(),
    ends_on: z.string(),
    timezone: z.string(),
    lead_days: leadDaysSchema,
    accounts: z.array(nodeAccountSchema).catch([]),
    goal: z.string(),
    material_source_ids: z.array(z.string()).catch([]),
    date_certainty: z.string(),
    date_basis: z.string(),
    note: z.string(),
    actor: z.string(),
    created_at: z.string(),
  })
  .loose()
  .transform((r) => ({
    revisionId: r.revision_id,
    nodeId: r.node_id,
    workspaceId: r.workspace_id,
    revision: r.revision,
    changeKind: toChangeKind(r.change_kind) as NodeChangeKind,
    statusAfter: r.status_after,
    name: r.name,
    kind: r.kind,
    startsOn: r.starts_on,
    endsOn: r.ends_on,
    timezone: r.timezone,
    leadDays: r.lead_days,
    accounts: r.accounts,
    goal: r.goal,
    materialSourceIds: r.material_source_ids,
    dateCertainty: r.date_certainty,
    dateBasis: r.date_basis,
    note: r.note,
    actor: r.actor,
    createdAt: r.created_at,
  }));

const nodeTimingSchema = z
  .object({
    today: z.string(),
    timezone: z.string(),
    phase: z.string(),
    preparation_starts_on: z.string().nullable(),
  })
  .loose()
  .transform((timing) => ({
    today: timing.today,
    timezone: timing.timezone,
    phase: toPhase(timing.phase) as NodePhase,
    preparationStartsOn: timing.preparation_starts_on,
  }));

export const marketingNodeSchema = z
  .object({
    node_id: z.string(),
    workspace_id: z.string(),
    // Open-ended like topic card status: a newer state renders generically.
    status: z.string(),
    origin: z.string(),
    current_revision: z.number().int().positive(),
    current: nodeRevisionSchema,
    timing: nodeTimingSchema,
    created_at: z.string(),
    updated_at: z.string(),
  })
  .loose()
  .transform((node) => ({
    nodeId: node.node_id,
    workspaceId: node.workspace_id,
    status: node.status,
    origin: node.origin,
    currentRevision: node.current_revision,
    current: node.current,
    timing: node.timing,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
  }));

export const importRowResultSchema = z
  .object({
    row: z.number().int().positive(),
    outcome: z.string(),
    node_id: z.string().optional(),
    field: z.string().optional(),
  })
  .loose()
  .transform((row) => ({
    row: row.row,
    outcome: toOutcome(row.outcome) as ImportOutcome,
    nodeId: row.node_id ?? "",
    field: row.field ?? "",
  }));

export type NodeRevision = z.infer<typeof nodeRevisionSchema>;
export type MarketingNode = z.infer<typeof marketingNodeSchema>;
export type ImportRowResult = z.infer<typeof importRowResultSchema>;

export const EMPTY_NODE_REVISION: NodeRevision = {
  revisionId: "",
  nodeId: "",
  workspaceId: "",
  revision: 1,
  changeKind: { kind: "create" },
  statusAfter: "",
  name: "",
  kind: "",
  startsOn: "",
  endsOn: "",
  timezone: "",
  leadDays: { set: false },
  accounts: [],
  goal: "",
  materialSourceIds: [],
  dateCertainty: "",
  dateBasis: "",
  note: "",
  actor: "",
  createdAt: "",
};

export const EMPTY_MARKETING_NODE: MarketingNode = {
  nodeId: "",
  workspaceId: "",
  status: "",
  origin: "",
  currentRevision: 1,
  current: EMPTY_NODE_REVISION,
  timing: {
    today: "",
    timezone: "",
    phase: { kind: "unknown", raw: "" },
    preparationStartsOn: null,
  },
  createdAt: "",
  updatedAt: "",
};

/**
 * Parse each item on its own. The envelope must be an object with an array
 * under key; a malformed item is dropped (and logged by parseWithFallback), so
 * one bad node cannot empty the whole list.
 */
function parseItems<T>(
  data: unknown,
  key: string,
  item: z.ZodType,
  endpoint: string,
): T[] {
  const envelope = parseWithFallback<Record<string, unknown[]>>(
    data,
    z.object({ [key]: z.array(z.unknown()) }).loose(),
    { [key]: [] },
    { endpoint },
  );
  const items: T[] = [];
  for (const raw of envelope[key] ?? []) {
    const parsed = parseWithFallback<T | null>(raw, item, null, { endpoint });
    if (parsed !== null) items.push(parsed);
  }
  return items;
}

export function parseMarketingNode(data: unknown): MarketingNode {
  return parseWithFallback(data, marketingNodeSchema, EMPTY_MARKETING_NODE, {
    endpoint: "content-marketing-nodes/detail",
  });
}

export function parseMarketingNodes(data: unknown): MarketingNode[] {
  return parseItems<MarketingNode>(
    data,
    "marketing_nodes",
    marketingNodeSchema,
    "content-marketing-nodes/list",
  );
}

export function parseNodeRevisions(data: unknown): NodeRevision[] {
  return parseItems<NodeRevision>(
    data,
    "revisions",
    nodeRevisionSchema,
    "content-marketing-nodes/revisions",
  );
}

export function parseImportResult(data: unknown): ImportRowResult[] {
  return parseItems<ImportRowResult>(
    data,
    "results",
    importRowResultSchema,
    "content-marketing-nodes/import",
  );
}

export interface MarketingNodeAccountInput {
  accountId: string;
  role: string;
}

/**
 * The full content of a node revision. Editing submits the whole revision, so
 * a form must carry every field over from the current one - an omitted
 * leadDays would mean "not set in this revision" (contract §2.1).
 */
export interface MarketingNodeInput {
  name: string;
  kind: string;
  startsOn: string;
  endsOn: string;
  timezone: string;
  leadDays: LeadDays;
  accounts: MarketingNodeAccountInput[];
  goal: string;
  materialSourceIds: string[];
  dateCertainty: string;
  dateBasis: string;
}

export function marketingNodeInputToWire(
  input: MarketingNodeInput,
): Record<string, unknown> {
  return {
    name: input.name,
    kind: input.kind,
    starts_on: input.startsOn,
    ends_on: input.endsOn,
    timezone: input.timezone,
    lead_days: input.leadDays.set ? input.leadDays.value : null,
    accounts: input.accounts.map((account) => ({
      account_id: account.accountId,
      role: account.role,
    })),
    goal: input.goal,
    material_source_ids: input.materialSourceIds,
    date_certainty: input.dateCertainty,
    date_basis: input.dateBasis,
  };
}

export function createMarketingNodeToWire(
  input: MarketingNodeInput,
  note = "",
): Record<string, unknown> {
  return { ...marketingNodeInputToWire(input), note };
}

export function reviseMarketingNodeToWire(
  baseRevision: number,
  input: MarketingNodeInput,
  note = "",
): Record<string, unknown> {
  return { ...marketingNodeInputToWire(input), base_revision: baseRevision, note };
}

export function importMarketingNodesToWire(
  rows: MarketingNodeInput[],
): Record<string, unknown> {
  return { rows: rows.map(marketingNodeInputToWire) };
}

export function nodeTransitionToWire(
  baseRevision: number,
  note = "",
): Record<string, unknown> {
  return { base_revision: baseRevision, note };
}

// ---------------------------------------------------------------------------
// Candidates and schedule impact (specs/033 PR 2, contract §4, §5).
//
// Everything below the stored fields is computed by the server on each read
// from what people filled in; nothing in a candidate is generated text.

export const MATERIAL_GAP_KINDS = ["none", "archived", "missing"] as const;
export type KnownMaterialGapKind = (typeof MATERIAL_GAP_KINDS)[number];
export type MaterialGapKind =
  | { kind: KnownMaterialGapKind }
  | { kind: "unknown"; raw: string };

export const DUPLICATE_REASONS = ["name_match", "adopted_from_node"] as const;
export type KnownDuplicateReason = (typeof DUPLICATE_REASONS)[number];
export type DuplicateReason =
  | { kind: KnownDuplicateReason }
  | { kind: "unknown"; raw: string };

const toGapKind = known(MATERIAL_GAP_KINDS);
const toDuplicateReason = known(DUPLICATE_REASONS);

/**
 * Whether the time left is shorter than the lead time. "unknown" when no lead
 * time was set: that is no answer, and must not render as "enough".
 */
export type LeadShort = boolean | "unknown";

// Only a literal true is short (constitution VI): a string "true" or 1 from a
// confused server is not read as a warning, and null is "unknown".
const leadShortSchema = z
  .unknown()
  .transform((value): LeadShort =>
    value === null || value === undefined ? "unknown" : value === true,
  );

const optionalRevision = z.number().int().positive().nullable().catch(null);

const profileFieldSchema = z
  .object({ value: z.string().catch(""), status: z.string().catch("pending") })
  .loose()
  .transform((field) => ({ value: field.value, status: field.status }));

const accountRelationSchema = z
  .object({
    audience: profileFieldSchema,
    content_pillars: profileFieldSchema,
    content_goals: profileFieldSchema,
  })
  .loose()
  .transform((account) => ({
    audience: account.audience,
    contentPillars: account.content_pillars,
    contentGoals: account.content_goals,
  }));

const candidateTimingSchema = z
  .object({
    today: z.string(),
    timezone: z.string(),
    phase: z.string(),
    preparation_starts_on: z.string().nullable(),
    days_until_start: z.number().int(),
    lead_days: leadDaysSchema,
    lead_short: leadShortSchema,
  })
  .loose()
  .transform((timing) => ({
    today: timing.today,
    timezone: timing.timezone,
    phase: toPhase(timing.phase) as NodePhase,
    preparationStartsOn: timing.preparation_starts_on,
    daysUntilStart: timing.days_until_start,
    leadDays: timing.lead_days,
    leadShort: timing.lead_short,
  }));

const collisionSchema = z
  .object({
    node_id: z.string(),
    name: z.string(),
    starts_on: z.string(),
    ends_on: z.string(),
  })
  .loose()
  .transform((c) => ({
    nodeId: c.node_id,
    name: c.name,
    startsOn: c.starts_on,
    endsOn: c.ends_on,
  }));

const materialGapSchema = z
  .object({ kind: z.string(), source_id: z.string().optional() })
  .loose()
  .transform((gap) => ({
    kind: toGapKind(gap.kind) as MaterialGapKind,
    sourceId: gap.source_id ?? "",
  }));

const duplicateRiskSchema = z
  .object({ topic_card_id: z.string(), reason: z.string() })
  .loose()
  .transform((risk) => ({
    topicCardId: risk.topic_card_id,
    reason: toDuplicateReason(risk.reason) as DuplicateReason,
  }));

/** A malformed entry in a candidate's list drops that entry, not the candidate. */
function lenientArray<T extends z.ZodType>(item: T) {
  return z
    .array(z.unknown())
    .catch([])
    .transform((values) =>
      values.flatMap((value) => {
        const parsed = item.safeParse(value);
        return parsed.success ? [parsed.data as z.output<T>] : [];
      }),
    );
}

export const nodeCandidateSchema = z
  .object({
    candidate_id: z.string(),
    node_id: z.string(),
    account_id: z.string(),
    angle: z.string(),
    // Open-ended: a newer status renders as its raw value.
    status: z.string(),
    dismiss_reason: z.string().catch(""),
    topic_card_id: z.string().catch(""),
    adopted_revision: optionalRevision,
    impact_decision: z.string().catch(""),
    impact_decision_note: z.string().catch(""),
    impact_decision_revision: optionalRevision,
    impact_decided_by: z.string().catch(""),
    in_scope: z.boolean(),
    timing: candidateTimingSchema,
    relation: z
      .object({
        goal: z.string(),
        role: z.string(),
        account: accountRelationSchema.nullable().catch(null),
      })
      .loose(),
    collisions: lenientArray(collisionSchema),
    material_gaps: lenientArray(materialGapSchema),
    duplicate_risks: lenientArray(duplicateRiskSchema),
    origin: z.string(),
    date_certainty: z.string(),
    date_basis: z.string(),
  })
  .loose()
  .transform((c) => ({
    candidateId: c.candidate_id,
    nodeId: c.node_id,
    accountId: c.account_id,
    angle: c.angle,
    status: c.status,
    dismissReason: c.dismiss_reason,
    topicCardId: c.topic_card_id,
    adoptedRevision: c.adopted_revision,
    impactDecision: c.impact_decision,
    impactDecisionNote: c.impact_decision_note,
    impactDecisionRevision: c.impact_decision_revision,
    impactDecidedBy: c.impact_decided_by,
    inScope: c.in_scope,
    timing: c.timing,
    relation: {
      goal: c.relation.goal,
      role: c.relation.role,
      account: c.relation.account,
    },
    collisions: c.collisions,
    materialGaps: c.material_gaps,
    duplicateRisks: c.duplicate_risks,
    origin: c.origin,
    dateCertainty: c.date_certainty,
    dateBasis: c.date_basis,
  }));

const impactDatesSchema = z
  .object({
    starts_on: z.string(),
    ends_on: z.string(),
    timezone: z.string(),
    lead_days: leadDaysSchema,
  })
  .loose()
  .transform((dates) => ({
    startsOn: dates.starts_on,
    endsOn: dates.ends_on,
    timezone: dates.timezone,
    leadDays: dates.lead_days,
  }));

export const impactItemSchema = z
  .object({
    candidate_id: z.string(),
    topic_card_id: z.string(),
    account_id: z.string(),
    // The card's own status, or "missing"; open-ended.
    card_status: z.string(),
    adopted_revision: z.number().int().positive(),
    current_revision: z.number().int().positive(),
    before: impactDatesSchema,
    after: impactDatesSchema,
    cancelled: z.boolean(),
  })
  .loose()
  .transform((item) => ({
    candidateId: item.candidate_id,
    topicCardId: item.topic_card_id,
    accountId: item.account_id,
    cardStatus: item.card_status,
    adoptedRevision: item.adopted_revision,
    currentRevision: item.current_revision,
    before: item.before,
    after: item.after,
    cancelled: item.cancelled === true,
  }));

export type NodeCandidate = z.infer<typeof nodeCandidateSchema>;
export type ImpactItem = z.infer<typeof impactItemSchema>;

export const EMPTY_NODE_CANDIDATE: NodeCandidate = {
  candidateId: "",
  nodeId: "",
  accountId: "",
  angle: "",
  status: "",
  dismissReason: "",
  topicCardId: "",
  adoptedRevision: null,
  impactDecision: "",
  impactDecisionNote: "",
  impactDecisionRevision: null,
  impactDecidedBy: "",
  inScope: false,
  timing: {
    today: "",
    timezone: "",
    phase: { kind: "unknown", raw: "" },
    preparationStartsOn: null,
    daysUntilStart: 0,
    leadDays: { set: false },
    leadShort: "unknown",
  },
  relation: { goal: "", role: "", account: null },
  collisions: [],
  materialGaps: [],
  duplicateRisks: [],
  origin: "",
  dateCertainty: "",
  dateBasis: "",
};

export function parseNodeCandidate(data: unknown): NodeCandidate {
  return parseWithFallback(data, nodeCandidateSchema, EMPTY_NODE_CANDIDATE, {
    endpoint: "content-marketing-nodes/candidate",
  });
}

export function parseNodeCandidates(data: unknown): NodeCandidate[] {
  return parseItems<NodeCandidate>(
    data,
    "candidates",
    nodeCandidateSchema,
    "content-marketing-nodes/candidates",
  );
}

export function parseImpactItems(data: unknown): ImpactItem[] {
  return parseItems<ImpactItem>(
    data,
    "impact",
    impactItemSchema,
    "content-marketing-nodes/impact",
  );
}

export interface AdoptResult {
  candidate: NodeCandidate;
  topicCard: TopicCard;
}

/** The candidate and the card it now points at; each half falls back alone. */
export function parseAdoptResult(data: unknown): AdoptResult {
  const envelope = parseWithFallback<{ candidate: unknown; topic_card: unknown }>(
    data,
    z.object({ candidate: z.unknown(), topic_card: z.unknown() }).loose(),
    { candidate: null, topic_card: null },
    { endpoint: "content-marketing-nodes/adopt" },
  );
  return {
    candidate: parseNodeCandidate(envelope.candidate),
    topicCard: parseTopicCard(envelope.topic_card),
  };
}

/** Each key is optional; a missing key leaves that field as it is (§2.1). */
export interface CandidatePatchInput {
  angle?: string;
  status?: "open" | "dismissed";
  dismissReason?: string;
}

export function candidatePatchToWire(
  input: CandidatePatchInput,
): Record<string, unknown> {
  const wire: Record<string, unknown> = {};
  if (input.angle !== undefined) wire.angle = input.angle;
  if (input.status !== undefined) wire.status = input.status;
  if (input.dismissReason !== undefined) wire.dismiss_reason = input.dismissReason;
  return wire;
}

export type AdoptInput = { mode: "create" } | { mode: "link"; topicCardId: string };

export function adoptToWire(input: AdoptInput): Record<string, unknown> {
  return input.mode === "link"
    ? { mode: "link", topic_card_id: input.topicCardId }
    : { mode: "create" };
}

export function impactDecisionToWire(
  decision: "kept" | "handled",
  note = "",
): Record<string, unknown> {
  return { decision, note };
}
