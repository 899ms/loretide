import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseRankObservation,
  parseRankObservationList,
  parseSearchMetric,
  parseSearchMetricList,
  searchObservationPath,
  type RankObservation,
  type RankObservationInput,
  type RankObservationRevisionInput,
  type SearchMetricInput,
  type SearchMetricRecord,
} from "./contract";

// Server state for search metrics and ranking observations (specs/036 PR 4).
// TanStack Query owns it.
//
// Nothing here is optimistic: a write may be refused (a publication record
// that is not this brand's, a stale base_revision, an observation from the
// future), and what the server stored - the normalized query, the revision
// number - is what the page shows. Writes invalidate on success.
//
// Every key carries workspaceId, so switching brands never serves the
// previous brand's numbers from cache.

export interface RankObservationFilter {
  themeId?: string;
  publicationRecordId?: string;
  query?: string;
  includeVoided?: boolean;
}

export const searchObservationKeys = {
  all: (workspaceId: string) => ["contentSearch", workspaceId, "observations"] as const,
  metrics: (workspaceId: string, publicationRecordId: string) =>
    ["contentSearch", workspaceId, "observations", "metrics", publicationRecordId] as const,
  observations: (workspaceId: string, filter: RankObservationFilter = {}) =>
    ["contentSearch", workspaceId, "observations", "rank", filter] as const,
};

/** GET metrics?publication_record_id=: every sample of one record. */
export function useSearchMetrics(workspaceId: string, publicationRecordId: string) {
  return useQuery<{ publicationRecordId: string; metrics: SearchMetricRecord[] } | null>({
    queryKey: searchObservationKeys.metrics(workspaceId, publicationRecordId),
    queryFn: async () => parseSearchMetricList(
      await api.contentSearchGet("metrics", { publication_record_id: publicationRecordId }),
    ),
    enabled: publicationRecordId !== "",
  });
}

/** GET rank-observations: at least one of theme, record or query. */
export function useRankObservations(workspaceId: string, filter: RankObservationFilter) {
  const query: Record<string, string> = {};
  if (filter.themeId) query.theme_id = filter.themeId;
  if (filter.publicationRecordId) query.publication_record_id = filter.publicationRecordId;
  if (filter.query) query.query = filter.query;
  if (filter.includeVoided) query.include_voided = "true";
  return useQuery<RankObservation[]>({
    queryKey: searchObservationKeys.observations(workspaceId, filter),
    queryFn: async () => parseRankObservationList(await api.contentSearchGet("rank-observations", query)),
    enabled: Boolean(filter.themeId || filter.publicationRecordId || filter.query),
  });
}

/** POST metrics: one sample. */
export function useRecordSearchMetric(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<SearchMetricRecord | null, Error, SearchMetricInput>({
    mutationFn: async (input) => parseSearchMetric(await api.contentSearchPost("metrics", { ...input })),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: searchObservationKeys.all(workspaceId) });
    },
  });
}

/** POST rank-observations: revision 1 of one observation. */
export function useRecordRankObservation(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<RankObservation | null, Error, RankObservationInput>({
    mutationFn: async (input) => parseRankObservation(await api.contentSearchPost("rank-observations", { ...input })),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: searchObservationKeys.all(workspaceId) });
    },
  });
}

/** POST rank-observations/{observationId}/revisions: a correction, or voided = true. */
export function useReviseRankObservation(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<RankObservation | null, Error, { observationId: string; input: RankObservationRevisionInput }>({
    mutationFn: async ({ observationId, input }) => parseRankObservation(
      await api.contentSearchPost(searchObservationPath("rank-observations", observationId, "revisions"), { ...input }),
    ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: searchObservationKeys.all(workspaceId) });
    },
  });
}
