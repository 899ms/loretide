import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// The material inbox's wire contract (specs/028, SOP §4).
//
// kind and status are open-ended strings on reads, deliberately. An older
// installed client receiving a value a newer server introduced renders the raw
// value rather than replacing the whole row with a fallback - the item and its
// annotation are still worth showing.

export const SOURCE_KINDS = ["pasted_text", "url"] as const;
export type SourceKind = (typeof SOURCE_KINDS)[number];

export const SOURCE_STATUSES = ["inbox", "organized", "archived"] as const;
export type SourceStatus = (typeof SOURCE_STATUSES)[number];

const sourceSchema = z
  .object({
    source_id: z.string(),
    workspace_id: z.string(),
    kind: z.string(),
    url: z.string(),
    captured_at: z.string(),
    recorded_by: z.string(),
    historical_import: z.boolean(),
    title: z.string(),
    tags: z.array(z.string()).nullable(),
    annotation: z.string(),
    personal_judgement: z.string(),
    status: z.string(),
    updated_at: z.string(),
  })
  .loose()
  .transform((source) => ({
    sourceId: source.source_id,
    workspaceId: source.workspace_id,
    kind: source.kind,
    url: source.url,
    capturedAt: source.captured_at,
    recordedBy: source.recorded_by,
    historicalImport: source.historical_import,
    title: source.title,
    // A null collection is normalised to an empty one so every reader can
    // map over it without knowing which of the two it received.
    tags: source.tags ?? [],
    annotation: source.annotation,
    personalJudgement: source.personal_judgement,
    status: source.status,
    updatedAt: source.updated_at,
  }));

export type Source = z.infer<typeof sourceSchema>;

const snapshotSchema = z
  .object({
    snapshot_id: z.string(),
    workspace_id: z.string(),
    source_id: z.string(),
    content: z.string(),
    content_hash: z.string(),
    captured_at: z.string(),
  })
  .loose()
  .transform((snapshot) => ({
    snapshotId: snapshot.snapshot_id,
    workspaceId: snapshot.workspace_id,
    sourceId: snapshot.source_id,
    content: snapshot.content,
    contentHash: snapshot.content_hash,
    capturedAt: snapshot.captured_at,
  }));

export type SourceSnapshot = z.infer<typeof snapshotSchema>;

const revisionSchema = z
  .object({
    revision_id: z.string(),
    workspace_id: z.string(),
    source_id: z.string(),
    changed_fields: z.array(z.string()).nullable(),
    actor_id: z.string(),
    created_at: z.string(),
  })
  .loose()
  .transform((revision) => ({
    revisionId: revision.revision_id,
    workspaceId: revision.workspace_id,
    sourceId: revision.source_id,
    changedFields: revision.changed_fields ?? [],
    actorId: revision.actor_id,
    createdAt: revision.created_at,
  }));

export type SourceRevision = z.infer<typeof revisionSchema>;

const bulkResultSchema = z
  .object({
    source_id: z.string(),
    ok: z.boolean(),
    reason: z.string().optional(),
  })
  .loose()
  .transform((result) => ({
    sourceId: result.source_id,
    ok: result.ok === true,
    reason: result.reason ?? "",
  }));

export type SourceBulkResult = z.infer<typeof bulkResultSchema>;

const EMPTY_SOURCE: Source = {
  sourceId: "",
  workspaceId: "",
  kind: "pasted_text",
  url: "",
  capturedAt: "",
  recordedBy: "",
  historicalImport: false,
  title: "",
  tags: [],
  annotation: "",
  personalJudgement: "",
  status: "inbox",
  updatedAt: "",
};

const sourceListSchema = z
  .object({ sources: z.array(sourceSchema) })
  .loose()
  .transform((list) => list.sources);

const revisionListSchema = z
  .object({ revisions: z.array(revisionSchema) })
  .loose()
  .transform((list) => list.revisions);

const duplicatesSchema = z
  .object({ source_ids: z.array(z.string()).nullable() })
  .loose()
  .transform((body) => body.source_ids ?? []);

const bulkSchema = z
  .object({ results: z.array(bulkResultSchema) })
  .loose()
  .transform((body) => body.results);

const detailSchema = z
  .object({
    source: sourceSchema,
    snapshot: snapshotSchema.nullable().optional(),
  })
  .loose()
  .transform((body) => ({ source: body.source, snapshot: body.snapshot ?? null }));

/**
 * What creating answers with.
 *
 * `duplicates` is a hint and nothing else: the item was created, and neither it
 * nor the ones it names were changed (R-011). A caller that treats a non-empty
 * list as a failure would be inventing a rule the SOP does not have.
 */
const createdSchema = z
  .object({
    source: sourceSchema,
    snapshot: snapshotSchema.nullable().optional(),
    duplicates: z.array(z.string()).nullable(),
  })
  .loose()
  .transform((body) => ({
    source: body.source,
    snapshot: body.snapshot ?? null,
    duplicates: body.duplicates ?? [],
  }));

export type SourceDetail = { source: Source; snapshot: SourceSnapshot | null };
export type CreatedSource = SourceDetail & { duplicates: string[] };

export function parseSources(data: unknown): Source[] {
  return parseWithFallback(data, sourceListSchema, [] as Source[], {
    endpoint: "GET /api/content-sources",
  });
}

export function parseSourceDetail(data: unknown): SourceDetail {
  return parseWithFallback(data, detailSchema, { source: EMPTY_SOURCE, snapshot: null }, {
    endpoint: "GET /api/content-sources/{id}",
  });
}

export function parseCreatedSource(data: unknown): CreatedSource {
  return parseWithFallback(
    data,
    createdSchema,
    { source: EMPTY_SOURCE, snapshot: null, duplicates: [] },
    { endpoint: "POST /api/content-sources" },
  );
}

export function parseSourceRevisions(data: unknown): SourceRevision[] {
  return parseWithFallback(data, revisionListSchema, [] as SourceRevision[], {
    endpoint: "GET /api/content-sources/{id}/revisions",
  });
}

export function parseDuplicates(data: unknown): string[] {
  return parseWithFallback(data, duplicatesSchema, [] as string[], {
    endpoint: "GET /api/content-sources/duplicates",
  });
}

export function parseBulkResults(data: unknown): SourceBulkResult[] {
  return parseWithFallback(data, bulkSchema, [] as SourceBulkResult[], {
    endpoint: "POST /api/content-sources/bulk",
  });
}

/** Whether a status is one this build knows how to label. */
export function isKnownStatus(status: string): status is SourceStatus {
  return (SOURCE_STATUSES as readonly string[]).includes(status);
}

export function isKnownKind(kind: string): kind is SourceKind {
  return (SOURCE_KINDS as readonly string[]).includes(kind);
}
