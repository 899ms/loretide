import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseSearchTheme,
  parseSearchThemeList,
  parseSearchThemeRevisions,
  searchPath,
  type SearchTheme,
  type SearchThemeInput,
  type SearchThemeRevisionInput,
} from "./contract";

// Server state for search themes (specs/036 PR 1). TanStack Query owns it.
//
// Nothing here is optimistic: a write may be refused (a stale base_revision,
// a reference that is not this brand's), and what the server stored -
// normalized keywords, the revision number - is what the page shows. Writes
// invalidate on success and the reads refetch.
//
// Every key carries workspaceId, so switching brands never serves the
// previous brand's themes from cache.

export interface SearchThemeFilter {
  platform?: string;
  accountId?: string;
  topicCardId?: string;
  includeArchived?: boolean;
}

export const searchThemeKeys = {
  all: (workspaceId: string) => ["contentSearch", workspaceId, "themes"] as const,
  list: (workspaceId: string, filter: SearchThemeFilter = {}) =>
    ["contentSearch", workspaceId, "themes", "list", filter] as const,
  detail: (workspaceId: string, themeId: string) =>
    ["contentSearch", workspaceId, "themes", "detail", themeId] as const,
  revisions: (workspaceId: string, themeId: string) =>
    ["contentSearch", workspaceId, "themes", "revisions", themeId] as const,
};

/** GET themes, filtered. Archived themes only when asked. */
export function useSearchThemes(workspaceId: string, filter: SearchThemeFilter = {}) {
  const query: Record<string, string> = {};
  if (filter.platform) query.platform = filter.platform;
  if (filter.accountId) query.account_id = filter.accountId;
  if (filter.topicCardId) query.topic_card_id = filter.topicCardId;
  if (filter.includeArchived) query.include_archived = "true";
  return useQuery<SearchTheme[]>({
    queryKey: searchThemeKeys.list(workspaceId, filter),
    queryFn: async () => parseSearchThemeList(await api.contentSearchGet("themes", query)),
  });
}

/** GET themes/{themeId}: the current revision. */
export function useSearchTheme(workspaceId: string, themeId: string) {
  return useQuery<SearchTheme | null>({
    queryKey: searchThemeKeys.detail(workspaceId, themeId),
    queryFn: async () => parseSearchTheme(await api.contentSearchGet(searchPath("themes", themeId))),
    enabled: themeId !== "",
  });
}

/** GET themes/{themeId}/revisions: every revision, newest first. */
export function useSearchThemeRevisions(workspaceId: string, themeId: string) {
  return useQuery<{ themeId: string; revisions: SearchTheme[] } | null>({
    queryKey: searchThemeKeys.revisions(workspaceId, themeId),
    queryFn: async () => parseSearchThemeRevisions(
      await api.contentSearchGet(searchPath("themes", themeId, "revisions")),
    ),
    enabled: themeId !== "",
  });
}

/** POST themes: revision 1 of a new theme. */
export function useCreateSearchTheme(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<SearchTheme | null, Error, SearchThemeInput>({
    mutationFn: async (input) => parseSearchTheme(await api.contentSearchPost("themes", { ...input })),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: searchThemeKeys.all(workspaceId) });
    },
  });
}

/** POST themes/{themeId}/revisions: the next revision; voided = true archives. */
export function useReviseSearchTheme(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<SearchTheme | null, Error, { themeId: string; input: SearchThemeRevisionInput }>({
    mutationFn: async ({ themeId, input }) => parseSearchTheme(
      await api.contentSearchPost(searchPath("themes", themeId, "revisions"), { ...input }),
    ),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: searchThemeKeys.all(workspaceId) });
    },
  });
}
