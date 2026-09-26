import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { searchPath } from "./contract";
import {
  parseSearchSuggestion, parseSearchSuggestionList, parseSearchSuggestionComparison,
  type SearchSuggestionInput, type SearchSuggestionRevisionInput, type SearchSuggestionAbandonInput,
  type SearchSuggestionState,
} from "./suggestion-contract";

export interface SearchSuggestionFilter {
  workId?: string;
  artifactId?: string;
  themeId?: string;
  state?: SearchSuggestionState;
}
export const searchSuggestionKeys = {
  all: (workspaceId: string) => ["contentSearch", workspaceId, "suggestions"] as const,
  list: (workspaceId: string, filter: SearchSuggestionFilter = {}) =>
    ["contentSearch", workspaceId, "suggestions", "list", filter] as const,
  detail: (workspaceId: string, suggestionId: string) =>
    ["contentSearch", workspaceId, "suggestions", "detail", suggestionId] as const,
  compare: (workspaceId: string, ids: readonly string[]) =>
    ["contentSearch", workspaceId, "suggestions", "compare", [...ids].sort()] as const,
};

export function useSearchSuggestions(workspaceId: string, filter: SearchSuggestionFilter = {}) {
  const query: Record<string, string> = {};
  if (filter.workId) query.work_id = filter.workId;
  if (filter.artifactId) query.artifact_id = filter.artifactId;
  if (filter.themeId) query.theme_id = filter.themeId;
  if (filter.state) query.state = filter.state;
  return useQuery({
    queryKey: searchSuggestionKeys.list(workspaceId, filter),
    queryFn: async () => parseSearchSuggestionList(await api.contentSearchGet("suggestions", query)),
  });
}
export function useSearchSuggestion(workspaceId: string, suggestionId: string) {
  return useQuery({
    queryKey: searchSuggestionKeys.detail(workspaceId, suggestionId),
    queryFn: async () => parseSearchSuggestion(await api.contentSearchGet(searchPath("suggestions", suggestionId))),
    enabled: suggestionId !== "",
  });
}
export function useCompareSearchSuggestions(workspaceId: string, ids: readonly string[]) {
  return useQuery({
    queryKey: searchSuggestionKeys.compare(workspaceId, ids),
    queryFn: async () => parseSearchSuggestionComparison(await api.contentSearchGet("suggestions/compare", { ids: [...ids].sort().join(",") })),
    enabled: ids.length >= 2 && ids.length <= 4 && new Set(ids).size === ids.length && ids.every(Boolean),
  });
}

// Writes never optimistically replace the body or derive a terminal state.
// Invalidate this brand's lists, details and comparisons together after success.
export function useCreateSearchSuggestion(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: SearchSuggestionInput) => parseSearchSuggestion(await api.contentSearchPost("suggestions", { ...input })),
    onSuccess: () => { void client.invalidateQueries({ queryKey: searchSuggestionKeys.all(workspaceId) }); },
  });
}
export function useReviseSearchSuggestion(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ suggestionId, input }: { suggestionId: string; input: SearchSuggestionRevisionInput }) =>
      parseSearchSuggestion(await api.contentSearchPost(searchPath("suggestions", suggestionId, "revisions"), { ...input })),
    onSuccess: () => { void client.invalidateQueries({ queryKey: searchSuggestionKeys.all(workspaceId) }); },
  });
}
export function useAbandonSearchSuggestion(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ suggestionId, input }: { suggestionId: string; input: SearchSuggestionAbandonInput }) =>
      parseSearchSuggestion(await api.contentSearchPost(searchPath("suggestions", suggestionId, "decisions"), { ...input })),
    onSuccess: () => { void client.invalidateQueries({ queryKey: searchSuggestionKeys.all(workspaceId) }); },
  });
}
