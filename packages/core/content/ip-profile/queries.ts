import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  NO_REVISION,
  parseAccount,
  parseAccountList,
  parseRevision,
  type ContentAccount,
  type PersonaRevision,
} from "./contract";

// Server state for the account settings page. TanStack Query owns all of it;
// the page keeps only which account is selected and the unsaved drafts.
//
// Every key carries wsId. Without it, switching brands would serve the previous
// brand's accounts out of cache until a refetch landed - which is the same
// class of bug as carrying a draft across accounts, one level up.

export const accountKeys = {
  all: (wsId: string) => ["contentAccounts", wsId] as const,
  list: (wsId: string) => ["contentAccounts", wsId, "list"] as const,
  persona: (wsId: string, accountId: string) =>
    ["contentAccounts", wsId, "persona", accountId] as const,
};

export function useContentAccounts(wsId: string) {
  return useQuery<ContentAccount[]>({
    queryKey: accountKeys.list(wsId),
    queryFn: async () => parseAccountList(await api.listContentAccounts()),
  });
}

/**
 * The account's current persona revision.
 *
 * A 404 means the account has no revision yet, which is an ordinary state and
 * not an error: the page shows an empty prompt either way. Failing the query
 * would put an error banner on a brand-new account.
 */
export function useAccountPersona(wsId: string, accountId: string) {
  return useQuery<PersonaRevision>({
    queryKey: accountKeys.persona(wsId, accountId),
    enabled: !!accountId,
    queryFn: async () => {
      try {
        return parseRevision(await api.getContentAccountPersona(accountId));
      } catch {
        return NO_REVISION;
      }
    },
  });
}

export function useCreateContentAccount(wsId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { platform: string; displayName: string }) =>
      parseAccount(
        await api.createContentAccount({
          platform: input.platform,
          display_name: input.displayName,
        }),
      ),
    // Creation navigates the page's selection to the new account, so it waits
    // for the server rather than guessing an id.
    onSuccess: () => client.invalidateQueries({ queryKey: accountKeys.all(wsId) }),
  });
}

export function useUpdateContentAccount(wsId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      accountId: string;
      platform: string;
      displayName: string;
    }) =>
      parseAccount(
        await api.updateContentAccount(input.accountId, {
          platform: input.platform,
          display_name: input.displayName,
        }),
      ),
    onSuccess: () => client.invalidateQueries({ queryKey: accountKeys.list(wsId) }),
  });
}

/**
 * Save a persona prompt.
 *
 * Not optimistic. The write appends a revision and can come back 409 when
 * someone else saved at the same moment; patching the cache first would show a
 * revision number that never existed and then have to take it back.
 */
export function useSetAccountPersona(wsId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { accountId: string; personaPrompt: string }) =>
      parseRevision(
        await api.setContentAccountPersona(input.accountId, input.personaPrompt),
      ),
    onSuccess: (_revision, input) =>
      client.invalidateQueries({
        queryKey: accountKeys.persona(wsId, input.accountId),
      }),
  });
}
