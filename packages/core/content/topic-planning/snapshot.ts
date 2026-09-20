import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// One recorded start (EP-04b): the configuration that was in force when a
// brief revision was taken up for work.
//
// The sixteen aligned fields come from the server's diagnostics.Snapshot; the
// two extension keys are topic-planning's own and are NOT part of that shape.
// They are spelled out separately here for the same reason they are on the Go
// side: every one of the sixteen already means something else, and borrowing
// one would make "aligned" a half-truth.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

/** Where a run may draw material from. Mirrors ip-profile's controlled set. */
export const SOURCE_SCOPES = ["local", "web", "all"] as const;
export type SourceScope = (typeof SOURCE_SCOPES)[number];

export interface InputSnapshot {
  config_version: string;
  persona_ref: string;
  sop_version: string;
  skill_version: string;
  rule_version: string;
  executor: string;
  executor_version: string;
  /** The scope chosen FOR THIS START. */
  source_scope: string;
  /** The account preference as it was read at that moment. A different field. */
  saved_preference: string;
  required_sources: string[];
  excluded_sources: string[];
  grants: string[];
  file_hashes: Record<string, string>;
  temperature: number;
  budget: number;
  timeout_ms: number;
  /** topic-planning extension, not part of the aligned sixteen. */
  auto_precheck: boolean;
  /** topic-planning extension, not part of the aligned sixteen. */
  uses_neutral_expression: boolean;
}

export interface StartSnapshot {
  snapshotId: string;
  workspaceId: string;
  topicCardId: string;
  briefRevisionId: string;
  accountId: string;
  /** "" is a real state: a start with no project, not a missing value. */
  projectId: string;
  actorId: string;
  snapshot: InputSnapshot;
  createdAt: string;
}

// Lenient on purpose: an installed client talks to whatever backend is
// deployed, and a field this build has not heard of must not blank the page.
const snapshotBodySchema = z.object({
  config_version: z.string().catch(""),
  persona_ref: z.string().catch(""),
  sop_version: z.string().catch(""),
  skill_version: z.string().catch(""),
  rule_version: z.string().catch(""),
  executor: z.string().catch(""),
  executor_version: z.string().catch(""),
  source_scope: z.string().catch(""),
  saved_preference: z.string().catch(""),
  required_sources: z.array(z.string()).nullable().catch(null),
  excluded_sources: z.array(z.string()).nullable().catch(null),
  grants: z.array(z.string()).nullable().catch(null),
  file_hashes: z.record(z.string(), z.string()).nullable().catch(null),
  temperature: z.number().catch(0),
  budget: z.number().catch(0),
  timeout_ms: z.number().catch(0),
  auto_precheck: z.boolean().catch(true),
  uses_neutral_expression: z.boolean().catch(true),
});

export const startSnapshotSchema = z.object({
  snapshot_id: z.string(),
  workspace_id: z.string().optional(),
  topic_card_id: z.string().optional(),
  brief_revision_id: z.string().optional(),
  account_id: z.string().optional(),
  project_id: z.string().optional(),
  actor_id: z.string().optional(),
  snapshot: snapshotBodySchema.partial().optional(),
  created_at: z.string().optional(),
});

export const startSnapshotListSchema = z.object({
  start_snapshots: z.array(startSnapshotSchema).nullable().optional(),
});

/** An all-empty snapshot: what a malformed response degrades to. Nothing is
 *  invented, and no field reads as "not yet known" when it is really missing. */
export function emptyInputSnapshot(): InputSnapshot {
  return {
    config_version: "",
    persona_ref: "",
    sop_version: "",
    skill_version: "",
    rule_version: "",
    executor: "",
    executor_version: "",
    source_scope: "",
    saved_preference: "",
    required_sources: [],
    excluded_sources: [],
    grants: [],
    file_hashes: {},
    temperature: 0,
    budget: 0,
    timeout_ms: 0,
    // Failing towards "neutral, precheck on" is the safe direction: both say
    // "be more careful", and neither claims a permission nobody granted.
    auto_precheck: true,
    uses_neutral_expression: true,
  };
}

const EMPTY_SNAPSHOT: StartSnapshot = {
  snapshotId: "",
  workspaceId: "",
  topicCardId: "",
  briefRevisionId: "",
  accountId: "",
  projectId: "",
  actorId: "",
  snapshot: emptyInputSnapshot(),
  createdAt: "",
};

type SnapshotWire = z.infer<typeof startSnapshotSchema>;

function toSnapshot(wire: SnapshotWire): StartSnapshot {
  const body = wire.snapshot ?? {};
  const empty = emptyInputSnapshot();
  return {
    snapshotId: wire.snapshot_id,
    workspaceId: wire.workspace_id ?? "",
    topicCardId: wire.topic_card_id ?? "",
    briefRevisionId: wire.brief_revision_id ?? "",
    accountId: wire.account_id ?? "",
    projectId: wire.project_id ?? "",
    actorId: wire.actor_id ?? "",
    snapshot: {
      ...empty,
      ...body,
      // Null collections come back from a backend that marshalled a nil slice.
      // Defaulting them here means no consumer has to know which of null and
      // [] it received.
      required_sources: body.required_sources ?? empty.required_sources,
      excluded_sources: body.excluded_sources ?? empty.excluded_sources,
      grants: body.grants ?? empty.grants,
      file_hashes: body.file_hashes ?? empty.file_hashes,
    },
    createdAt: wire.created_at ?? "",
  };
}

export function parseStartSnapshot(data: unknown): StartSnapshot {
  const parsed = parseWithFallback(data, startSnapshotSchema, { snapshot_id: "" }, {
    endpoint: "content-topics/start-snapshot",
  });
  if (!parsed.snapshot_id) return EMPTY_SNAPSHOT;
  return toSnapshot(parsed);
}

export function parseStartSnapshots(data: unknown): StartSnapshot[] {
  const parsed = parseWithFallback(
    data,
    startSnapshotListSchema,
    { start_snapshots: [] as SnapshotWire[] },
    { endpoint: "content-topics/start-snapshots" },
  );
  return (parsed.start_snapshots ?? []).map(toSnapshot);
}

export interface StartInput {
  accountId: string;
  sourceScope: SourceScope;
  projectId?: string;
}

/** The request body. `project_id` is always sent, as "" when there is none:
 *  the server rejects unknown fields, and an omitted one would be indistinct
 *  from a client that predates the field. */
export function startInputToWire(input: StartInput) {
  return {
    account_id: input.accountId,
    source_scope: input.sourceScope,
    project_id: input.projectId ?? "",
  };
}

/** What a refused start says is still missing, in SOP 3.1's own field names.
 *  Never throws: the caller is already on a failure path. */
const startErrorSchema = z.object({
  missing: z.array(z.string()).nullable().catch(null),
});

export function missingStartFields(error: unknown): string[] {
  const body = (error as { body?: unknown } | null)?.body;
  const parsed = parseWithFallback<z.infer<typeof startErrorSchema>>(
    body,
    startErrorSchema,
    { missing: null },
    { endpoint: "content-topics/start-error" },
  );
  return parsed.missing ?? [];
}
