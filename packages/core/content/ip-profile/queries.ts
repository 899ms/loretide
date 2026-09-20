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
import {
  emptyProfile,
  parseProfileRead,
  profileReadiness,
  usesNeutralExpression,
  type ExpressionProfile,
  type ProfileRead,
} from "./profile";

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
  profile: (wsId: string, accountId: string) =>
    ["contentAccounts", wsId, "profile", accountId] as const,
};

/** What an account with no revision yet looks like: every field pending, and
 *  the same two decisions derived from that. */
export function emptyProfileRead(): ProfileRead {
  const profile = emptyProfile();
  return {
    revisionId: "",
    revision: 0,
    profile,
    readiness: profileReadiness(profile),
    usesNeutralExpression: usesNeutralExpression(profile),
  };
}

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

/**
 * The account's stored expression profile plus the readiness and neutral-
 * expression decisions the server derived from it.
 *
 * Like the persona query, a failure degrades to the empty profile rather than
 * an error banner: an account that has never been confirmed reads as
 * all-pending, which is exactly what the page shows anyway.
 */
export function useAccountProfile(wsId: string, accountId: string) {
  return useQuery<ProfileRead>({
    queryKey: accountKeys.profile(wsId, accountId),
    enabled: !!accountId,
    queryFn: async () => {
      try {
        return parseProfileRead(await api.getContentAccountProfile(accountId));
      } catch {
        return emptyProfileRead();
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
/**
 * Confirm the profile by appending a revision.
 *
 * Not optimistic, for the same reason the persona write is not: the write can
 * come back 409, and the readiness the page shows afterwards is the server's
 * decision about a snapshot that either exists or does not. The persona query
 * is invalidated too - one revision carries both, so a profile write moves the
 * revision number the persona card displays.
 */
export function useSetAccountProfile(wsId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { accountId: string; profile: ExpressionProfile }) =>
      parseRevision(await api.setContentAccountProfile(input.accountId, input.profile)),
    onSuccess: async (_revision, input) => {
      await client.invalidateQueries({
        queryKey: accountKeys.profile(wsId, input.accountId),
      });
      await client.invalidateQueries({
        queryKey: accountKeys.persona(wsId, input.accountId),
      });
    },
  });
}

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
