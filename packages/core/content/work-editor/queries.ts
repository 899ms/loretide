import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseArtifact,
  parseArtifactVersion,
  parseArtifactVersions,
  parseArtifacts,
  parseWork,
  parseWorks,
  type Artifact,
  type ArtifactKind,
  type ArtifactVersion,
  type Work,
} from "./contract";

// Server state for the work editor. TanStack Query owns all of it.
//
// The text somebody is currently typing is NOT here: that is client state and
// belongs to the page, which flushes it through useAutosaveArtifact. Putting a
// live editor buffer in the query cache would make every keystroke a cache
// write and every refetch a chance to overwrite what the person is typing.
//
// Every key carries workspaceId. Without it, switching brands would serve the
// previous brand's works out of cache until a refetch landed.

export const workEditorKeys = {
  all: (workspaceId: string) => ["contentWorks", workspaceId] as const,
  list: (workspaceId: string, topicCardId = "") =>
    ["contentWorks", workspaceId, "list", topicCardId] as const,
  detail: (workspaceId: string, workId: string) =>
    ["contentWorks", workspaceId, "detail", workId] as const,
  artifacts: (workspaceId: string, workId: string) =>
    ["contentWorks", workspaceId, "artifacts", workId] as const,
  versions: (workspaceId: string, workId: string, artifactId: string) =>
    ["contentWorks", workspaceId, "versions", workId, artifactId] as const,
  version: (workspaceId: string, workId: string, artifactId: string, versionId: string) =>
    ["contentWorks", workspaceId, "versions", workId, artifactId, versionId] as const,
};

export function useContentWorks(workspaceId: string, topicCardId = "") {
  return useQuery<Work[]>({
    queryKey: workEditorKeys.list(workspaceId, topicCardId),
    queryFn: async () => parseWorks(await api.listContentWorks(topicCardId)),
  });
}

export function useContentWork(workspaceId: string, workId: string) {
  return useQuery<Work>({
    queryKey: workEditorKeys.detail(workspaceId, workId),
    enabled: !!workId,
    queryFn: async () => parseWork(await api.getContentWork(workId)),
  });
}

export function useContentArtifacts(workspaceId: string, workId: string) {
  return useQuery<Artifact[]>({
    queryKey: workEditorKeys.artifacts(workspaceId, workId),
    enabled: !!workId,
    queryFn: async () => parseArtifacts(await api.listContentArtifacts(workId)),
  });
}

/** A document's history, newest first. An empty one is an ordinary state. */
export function useArtifactVersions(workspaceId: string, workId: string, artifactId: string) {
  return useQuery<ArtifactVersion[]>({
    queryKey: workEditorKeys.versions(workspaceId, workId, artifactId),
    enabled: !!workId && !!artifactId,
    queryFn: async () =>
      parseArtifactVersions(await api.listContentArtifactVersions(workId, artifactId)),
  });
}

export function useArtifactVersion(
  workspaceId: string,
  workId: string,
  artifactId: string,
  versionId: string,
) {
  return useQuery<ArtifactVersion>({
    queryKey: workEditorKeys.version(workspaceId, workId, artifactId, versionId),
    enabled: !!workId && !!artifactId && !!versionId,
    queryFn: async () =>
      parseArtifactVersion(await api.getContentArtifactVersion(workId, artifactId, versionId)),
  });
}

export function useCreateContentWork(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { topicCardId: string; snapshotId?: string; title: string }) =>
      parseWork(
        await api.createContentWork({
          topic_card_id: input.topicCardId,
          // Always sent, as "" when the work was not started from a snapshot:
          // the server rejects unknown fields and would read an omitted one as
          // a client that predates the field.
          snapshot_id: input.snapshotId ?? "",
          title: input.title,
        }),
      ),
    // Creation navigates the page to the new work, so it waits for the server
    // rather than guessing an id.
    onSuccess: () => client.invalidateQueries({ queryKey: workEditorKeys.all(workspaceId) }),
  });
}

export function useCreateContentArtifact(workspaceId: string, workId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { kind: ArtifactKind; title: string; position: number }) =>
      parseArtifact(
        await api.createContentArtifact(workId, {
          kind: input.kind,
          title: input.title,
          position: input.position,
        }),
      ),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: workEditorKeys.artifacts(workspaceId, workId) }),
  });
}

/**
 * Autosave. Writes the editing copy and produces no version.
 *
 * Deliberately does not invalidate the version list: there is nothing new to
 * fetch, and refetching on every keystroke would be a request storm for a list
 * that did not change. It does patch the document cache with what the server
 * returned, so the saved/working badge follows the server's answer rather than
 * a guess the page made.
 */
export function useAutosaveArtifact(workspaceId: string, workId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      artifactId: string;
      draftBody?: string;
      title?: string;
      position?: number;
    }) => {
      const body: { draft_body?: string; title?: string; position?: number } = {};
      if (input.draftBody !== undefined) body.draft_body = input.draftBody;
      if (input.title !== undefined) body.title = input.title;
      if (input.position !== undefined) body.position = input.position;
      return parseArtifact(await api.patchContentArtifact(workId, input.artifactId, body));
    },
    onSuccess: (artifact) =>
      client.setQueryData<Artifact[]>(
        workEditorKeys.artifacts(workspaceId, workId),
        (current) =>
          current?.map((entry) =>
            entry.artifactId === artifact.artifactId ? artifact : entry,
          ),
      ),
  });
}

/**
 * Save the editing copy as a version, restore an older one, or adopt one as the
 * next step's baseline.
 *
 * All three APPEND, so all three invalidate the same two caches: the history
 * gained a row, and the document's saved/working status moved. None is
 * optimistic - a save can lose a race for its revision number and come back a
 * conflict, and showing a revision that never existed would have to be taken
 * back.
 */
export function useArtifactVersionAction(
  workspaceId: string,
  workId: string,
  artifactId: string,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { action: "save" | "restore" | "adopt"; versionId?: string }) => {
      if (input.action === "save") {
        return parseArtifactVersion(await api.saveContentArtifactVersion(workId, artifactId));
      }
      if (!input.versionId) throw new Error("restore and adopt need a version id");
      if (input.action === "restore") {
        return parseArtifactVersion(
          await api.restoreContentArtifactVersion(workId, artifactId, input.versionId),
        );
      }
      return parseArtifactVersion(
        await api.adoptContentArtifactVersion(workId, artifactId, input.versionId),
      );
    },
    onSuccess: async () => {
      await client.invalidateQueries({
        queryKey: workEditorKeys.versions(workspaceId, workId, artifactId),
      });
      await client.invalidateQueries({
        queryKey: workEditorKeys.artifacts(workspaceId, workId),
      });
    },
  });
}
