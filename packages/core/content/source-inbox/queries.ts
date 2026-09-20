import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseBulkResults,
  parseCreatedSource,
  parseDuplicates,
  parseSourceDetail,
  parseSourceRevisions,
  parseSources,
  type CreatedSource,
  type Source,
  type SourceBulkResult,
  type SourceDetail,
  type SourceRevision,
} from "./contract";

export const sourceInboxKeys = {
  all: (wsId: string) => ["contentSources", wsId] as const,
  list: (wsId: string, status: string, tag: string) =>
    ["contentSources", wsId, "list", status, tag] as const,
  detail: (wsId: string, sourceId: string) =>
    ["contentSources", wsId, "detail", sourceId] as const,
  revisions: (wsId: string, sourceId: string) =>
    ["contentSources", wsId, "revisions", sourceId] as const,
  duplicates: (wsId: string, contentHash: string) =>
    ["contentSources", wsId, "duplicates", contentHash] as const,
};

export function useContentSources(wsId: string, status = "", tag = "") {
  return useQuery<Source[]>({
    queryKey: sourceInboxKeys.list(wsId, status, tag),
    queryFn: async () => parseSources(await api.listContentSources(status, tag)),
  });
}

export function useContentSource(wsId: string, sourceId: string) {
  return useQuery<SourceDetail>({
    queryKey: sourceInboxKeys.detail(wsId, sourceId),
    enabled: !!sourceId,
    queryFn: async () => parseSourceDetail(await api.getContentSource(sourceId)),
  });
}

export function useContentSourceRevisions(wsId: string, sourceId: string) {
  return useQuery<SourceRevision[]>({
    queryKey: sourceInboxKeys.revisions(wsId, sourceId),
    enabled: !!sourceId,
    queryFn: async () => parseSourceRevisions(await api.listContentSourceRevisions(sourceId)),
  });
}

/**
 * The duplicate hint, asked before saving.
 *
 * A read with no side effects: it names what already holds this content and
 * changes nothing (SOP §4, R-011).
 */
export function useSourceDuplicates(wsId: string, contentHash: string) {
  return useQuery<string[]>({
    queryKey: sourceInboxKeys.duplicates(wsId, contentHash),
    enabled: !!contentHash,
    queryFn: async () => parseDuplicates(await api.contentSourceDuplicates(contentHash)),
  });
}

/**
 * Collecting. Not optimistic: the server assigns the id, the captured time and
 * the duplicate hint, so there is nothing a client could predict to render
 * early that would still be right.
 */
export function useCreateContentSource(wsId: string) {
  const client = useQueryClient();
  return useMutation<CreatedSource, Error, Record<string, unknown>>({
    mutationFn: async (body) => parseCreatedSource(await api.createContentSource(body)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: sourceInboxKeys.all(wsId) });
    },
  });
}

/**
 * Organising. Also not optimistic: the response carries the server's row and
 * a revision was appended, and both lists have to be refetched anyway.
 */
export function useOrganizeContentSource(wsId: string) {
  const client = useQueryClient();
  return useMutation<SourceDetail, Error, { sourceId: string; patch: Record<string, unknown> }>({
    mutationFn: async ({ sourceId, patch }) =>
      parseSourceDetail(await api.organizeContentSource(sourceId, patch)),
    onSuccess: (_result, variables) => {
      void client.invalidateQueries({ queryKey: sourceInboxKeys.all(wsId) });
      void client.invalidateQueries({
        queryKey: sourceInboxKeys.revisions(wsId, variables.sourceId),
      });
    },
  });
}

/**
 * Bulk tagging and archiving.
 *
 * The result is per item, so a caller can report which ones did not land. A
 * mutation that only said "ok" would hide exactly the case this shape exists
 * for.
 */
export function useBulkOrganizeContentSources(wsId: string) {
  const client = useQueryClient();
  return useMutation<SourceBulkResult[], Error, Record<string, unknown>>({
    mutationFn: async (body) => parseBulkResults(await api.bulkOrganizeContentSources(body)),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: sourceInboxKeys.all(wsId) });
    },
  });
}
