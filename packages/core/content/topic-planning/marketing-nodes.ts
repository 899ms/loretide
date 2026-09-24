import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

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
