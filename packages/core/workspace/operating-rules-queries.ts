import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  operatingRulesToRequest,
  parseOperatingRules,
  type OperatingRules,
} from "./operating-rules";

// Server state for the brand's operating rules. TanStack Query owns it.
//
// Nothing here is optimistic. Saving a cadence or an observation window
// changes what everybody else in the brand sees, and the server is the one
// that decides whether a channel name is real - guessing the outcome would
// show a saved state for a request that was refused.
//
// Every key carries workspaceId. Without it, switching brands would serve the
// previous brand's rules out of cache until a refetch landed.

export const operatingRulesKeys = {
  all: (workspaceId: string) => ["operatingRules", workspaceId] as const,
};

export function useOperatingRules(workspaceId: string) {
  return useQuery<OperatingRules>({
    queryKey: operatingRulesKeys.all(workspaceId),
    queryFn: async () => parseOperatingRules(await api.contentOperatingRules()),
  });
}

/**
 * Saving the whole set.
 *
 * The whole set goes up because that is how it is edited - one form, one
 * sitting - but only this card's settings key is written. There is no merge
 * helper on this side: the server merges, so a caller cannot forget to, and
 * two people saving different sections a second apart do not erase each other.
 */
export function useSaveOperatingRules(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<OperatingRules, Error, OperatingRules>({
    mutationFn: async (rules) =>
      parseOperatingRules(await api.setContentOperatingRules(operatingRulesToRequest(rules))),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: operatingRulesKeys.all(workspaceId) });
    },
  });
}

/** The account's public page link. Its own endpoint, and its own cache: the
 *  link lives on the account row, not with the brand's rules. */
export function useSaveAccountHomepage(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<unknown, Error, { accountId: string; homepage: string }>({
    mutationFn: async ({ accountId, homepage }) =>
      api.setContentAccountHomepage(accountId, homepage),
    onSettled: () => {
      // The accounts list carries the settings blob the link lives in. The key
      // mirrors ip-profile's accountKeys.all rather than importing it: this
      // file sits outside packages/core/content, and reaching into a content
      // module from here would be a dependency the registry never declared.
      // A drift here shows up as a stale link, which is why the page reads the
      // account list rather than caching the link separately.
      void client.invalidateQueries({ queryKey: ["contentAccounts", workspaceId] });
    },
  });
}
