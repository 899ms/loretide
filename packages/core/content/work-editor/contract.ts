import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Works, documents and their append-only version history (SOP 7).
//
// The controlled sets are the server's; they are restated here as literal
// unions so a component can switch on them, and `contract.test.ts` holds them
// to the Go source rather than to my memory of it.
//
// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md

/** What sort of document this is. */
export const ARTIFACT_KINDS = ["body", "channel_draft"] as const;
export type ArtifactKind = (typeof ARTIFACT_KINDS)[number];

/** SOP 7.1's pair for the editing copy, and only that pair. `saved` means the
 *  editing copy is byte-for-byte the latest version. */
export const DRAFT_STATUSES = ["working", "saved"] as const;
export type DraftStatus = (typeof DRAFT_STATUSES)[number];

/** Where a version's content came from. Exactly three. */
export const VERSION_SOURCES = ["generated", "edited", "adopted"] as const;
export type VersionSource = (typeof VERSION_SOURCES)[number];

/** What was done, recorded beside the source. Restoring is HERE, not in the
 *  source set: what comes back is still what a person wrote - and so is
 *  `imported`, which is a piece pasted back in from where it was published
 *  (SOP 3.3). Its source is still `edited`; what differs is that nobody wrote
 *  it today. */
export const VERSION_ACTIONS = ["saved", "restored", "adopted", "imported"] as const;
export type VersionAction = (typeof VERSION_ACTIONS)[number];

export interface Work {
  workId: string;
  workspaceId: string;
  topicCardId: string;
  /** "" when the work was not started from a snapshot. A real state: SOP 6.2
   *  requires writing to work with nothing else in place. */
  snapshotId: string;
  title: string;
  /** SOP 3.3's 历史导入标识: this work was pasted in from something already
   *  published elsewhere. Decided when the work is created and never changed.
   *
   *  A work with this set has no topicCardId, which is why by-card lists have
   *  to exclude it explicitly - see `worksInProgress` in core/today. */
  historicalImport: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Artifact {
  artifactId: string;
  workId: string;
  workspaceId: string;
  kind: string;
  title: string;
  position: number;
  draftBody: string;
  draftStatus: DraftStatus;
  draftSavedAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface ArtifactVersion {
  versionId: string;
  artifactId: string;
  workId: string;
  workspaceId: string;
  revision: number;
  source: string;
  action: string;
  body: string;
  /** "" unless the matching action produced this version. */
  restoredFrom: string;
  adoptedFrom: string;
  actorId: string;
  createdAt: string;
}

// Lenient by design: an installed client talks to whatever backend is
// deployed, and a value this build has not heard of must not blank the page.
// kind / source / action stay `z.string()` for that reason - what the page may
// WRITE is the controlled set above; what it may READ is wider.
const workSchema = z.object({
  work_id: z.string(),
  workspace_id: z.string().optional(),
  topic_card_id: z.string().optional(),
  snapshot_id: z.string().optional(),
  title: z.string().optional(),
  historical_import: z.boolean().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
});

const artifactSchema = z.object({
  artifact_id: z.string(),
  work_id: z.string().optional(),
  workspace_id: z.string().optional(),
  kind: z.string().optional(),
  title: z.string().optional(),
  position: z.number().optional(),
  draft_body: z.string().optional(),
  draft_status: z.string().optional(),
  draft_saved_at: z.string().optional(),
  created_at: z.string().optional(),
  updated_at: z.string().optional(),
});

const versionSchema = z.object({
  version_id: z.string(),
  artifact_id: z.string().optional(),
  work_id: z.string().optional(),
  workspace_id: z.string().optional(),
  revision: z.number().optional(),
  source: z.string().optional(),
  action: z.string().optional(),
  body: z.string().optional(),
  restored_from: z.string().optional(),
  adopted_from: z.string().optional(),
  actor_id: z.string().optional(),
  created_at: z.string().optional(),
});

const workListSchema = z.object({ works: z.array(workSchema).nullable().optional() });
const artifactListSchema = z.object({ artifacts: z.array(artifactSchema).nullable().optional() });
const versionListSchema = z.object({ versions: z.array(versionSchema).nullable().optional() });

const EMPTY_WORK: Work = {
  workId: "", workspaceId: "", topicCardId: "", snapshotId: "",
  title: "", historicalImport: false, createdAt: "", updatedAt: "",
};

const EMPTY_ARTIFACT: Artifact = {
  artifactId: "", workId: "", workspaceId: "", kind: "", title: "", position: 0,
  draftBody: "",
  // A build that cannot tell degrades to `working`: claiming everything is
  // saved when it might not be is the direction that loses work.
  draftStatus: "working",
  draftSavedAt: "", createdAt: "", updatedAt: "",
};

const EMPTY_VERSION: ArtifactVersion = {
  versionId: "", artifactId: "", workId: "", workspaceId: "", revision: 0,
  source: "", action: "", body: "", restoredFrom: "", adoptedFrom: "",
  actorId: "", createdAt: "",
};

function toWork(wire: z.infer<typeof workSchema>): Work {
  return {
    workId: wire.work_id,
    workspaceId: wire.workspace_id ?? "",
    topicCardId: wire.topic_card_id ?? "",
    snapshotId: wire.snapshot_id ?? "",
    title: wire.title ?? "",
    // === true, not truthiness. A backend that has not been deployed with
    // migration 532 sends nothing, and "absent" must read as "not an import"
    // rather than as a value to be coerced.
    historicalImport: wire.historical_import === true,
    createdAt: wire.created_at ?? "",
    updatedAt: wire.updated_at ?? "",
  };
}

function toArtifact(wire: z.infer<typeof artifactSchema>): Artifact {
  return {
    artifactId: wire.artifact_id,
    workId: wire.work_id ?? "",
    workspaceId: wire.workspace_id ?? "",
    kind: wire.kind ?? "",
    title: wire.title ?? "",
    position: wire.position ?? 0,
    draftBody: wire.draft_body ?? "",
    draftStatus: isDraftStatus(wire.draft_status) ? wire.draft_status : "working",
    draftSavedAt: wire.draft_saved_at ?? "",
    createdAt: wire.created_at ?? "",
    updatedAt: wire.updated_at ?? "",
  };
}

function toVersion(wire: z.infer<typeof versionSchema>): ArtifactVersion {
  return {
    versionId: wire.version_id,
    artifactId: wire.artifact_id ?? "",
    workId: wire.work_id ?? "",
    workspaceId: wire.workspace_id ?? "",
    revision: wire.revision ?? 0,
    source: wire.source ?? "",
    action: wire.action ?? "",
    body: wire.body ?? "",
    restoredFrom: wire.restored_from ?? "",
    adoptedFrom: wire.adopted_from ?? "",
    actorId: wire.actor_id ?? "",
    createdAt: wire.created_at ?? "",
  };
}

function isDraftStatus(value: unknown): value is DraftStatus {
  return value === "working" || value === "saved";
}

export function parseWork(data: unknown): Work {
  const parsed = parseWithFallback(data, workSchema, { work_id: "" }, {
    endpoint: "content-works/detail",
  });
  return parsed.work_id ? toWork(parsed) : EMPTY_WORK;
}

export function parseWorks(data: unknown): Work[] {
  const parsed = parseWithFallback(data, workListSchema, { works: [] }, {
    endpoint: "content-works/list",
  });
  return (parsed.works ?? []).map(toWork);
}

export function parseArtifact(data: unknown): Artifact {
  const parsed = parseWithFallback(data, artifactSchema, { artifact_id: "" }, {
    endpoint: "content-works/artifact",
  });
  return parsed.artifact_id ? toArtifact(parsed) : EMPTY_ARTIFACT;
}

export function parseArtifacts(data: unknown): Artifact[] {
  const parsed = parseWithFallback(data, artifactListSchema, { artifacts: [] }, {
    endpoint: "content-works/artifacts",
  });
  return (parsed.artifacts ?? []).map(toArtifact);
}

export function parseArtifactVersion(data: unknown): ArtifactVersion {
  const parsed = parseWithFallback(data, versionSchema, { version_id: "" }, {
    endpoint: "content-works/version",
  });
  return parsed.version_id ? toVersion(parsed) : EMPTY_VERSION;
}

export function parseArtifactVersions(data: unknown): ArtifactVersion[] {
  const parsed = parseWithFallback(data, versionListSchema, { versions: [] }, {
    endpoint: "content-works/versions",
  });
  return (parsed.versions ?? []).map(toVersion);
}

/**
 * What the history sidebar shows for one version: two facts, not one.
 *
 * SOP 7.1 asks for "来源与动作记录". Folding them would make "restored" look
 * like an answer to "who wrote this", which it is not - a restored version's
 * content is still what a person wrote.
 */
export function describeVersion(version: ArtifactVersion): {
  source: string;
  action: string;
  from: string;
} {
  return {
    source: version.source,
    action: version.action,
    from: version.restoredFrom || version.adoptedFrom || "",
  };
}
