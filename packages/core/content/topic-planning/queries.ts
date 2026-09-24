import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  briefRevisionInputToWire,
  parseBriefRevision,
  parseBriefRevisions,
  parseTopicActionResult,
  parseTopicCard,
  parseTopicCards,
  setContentTopicSourcesInputToWire,
  topicActionInputToWire,
  topicBodyPatchInputToWire,
  topicCardInputToWire,
  type BriefRevision,
  type BriefRevisionInput,
  type SetContentTopicSourcesInput,
  type TopicActionInput,
  type TopicBodyPatchInput,
  type TopicCard,
  type TopicCardInput,
} from "./contract";
import {
  parseStartSnapshot,
  parseStartSnapshots,
  startInputToWire,
  type StartInput,
  type StartSnapshot,
} from "./snapshot";

export const topicPlanningKeys = {
  all: (workspaceId: string) => ["contentTopics", workspaceId] as const,
  // The filter is part of the key: two filters are two lists, and a shared key
  // would serve one of them the other's cards.
  list: (workspaceId: string, accountFilter = "") =>
    ["contentTopics", workspaceId, "list", accountFilter] as const,
  detail: (workspaceId: string, topicCardId: string) =>
    ["contentTopics", workspaceId, "detail", topicCardId] as const,
  briefs: (workspaceId: string, topicCardId: string) =>
    ["contentTopics", workspaceId, "briefs", topicCardId] as const,
  brief: (workspaceId: string, topicCardId: string, revisionId: string) =>
    [
      "contentTopics",
      workspaceId,
      "briefs",
      topicCardId,
      "detail",
      revisionId,
    ] as const,
  starts: (workspaceId: string, topicCardId: string, revisionId: string) =>
    [
      "contentTopics",
      workspaceId,
      "briefs",
      topicCardId,
      "starts",
      revisionId,
    ] as const,
  snapshot: (workspaceId: string, topicCardId: string, snapshotId: string) =>
    ["contentTopics", workspaceId, "snapshots", topicCardId, snapshotId] as const,
};

/** Every start of one brief revision, newest first. */
export function useBriefStarts(
  workspaceId: string,
  topicCardId: string,
  revisionId: string,
) {
  return useQuery<StartSnapshot[]>({
    queryKey: topicPlanningKeys.starts(workspaceId, topicCardId, revisionId),
    enabled: !!topicCardId && !!revisionId,
    queryFn: async () =>
      parseStartSnapshots(await api.listContentBriefStarts(topicCardId, revisionId)),
  });
}

/** One start by its stable key. */
export function useStartSnapshot(
  workspaceId: string,
  topicCardId: string,
  snapshotId: string,
) {
  return useQuery<StartSnapshot>({
    queryKey: topicPlanningKeys.snapshot(workspaceId, topicCardId, snapshotId),
    enabled: !!topicCardId && !!snapshotId,
    queryFn: async () =>
      parseStartSnapshot(await api.getContentStartSnapshot(topicCardId, snapshotId)),
  });
}

/**
 * Start a brief revision.
 *
 * Not optimistic, and not treated as idempotent: the write can be refused for
 * a readiness gap, and starting the same revision again is a legitimate second
 * start rather than a retry of the first. Callers disable the control while it
 * is in flight; they must not suppress a second start on the grounds that one
 * already exists.
 */
export function useStartBrief(workspaceId: string, topicCardId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: StartInput & { revisionId: string }) =>
      parseStartSnapshot(
        await api.startContentBrief(topicCardId, input.revisionId, startInputToWire(input)),
      ),
    onSuccess: (_snapshot, input) =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.starts(workspaceId, topicCardId, input.revisionId),
      }),
  });
}

export function useContentTopics(workspaceId: string, accountFilter = "") {
  return useQuery<TopicCard[]>({
    queryKey: topicPlanningKeys.list(workspaceId, accountFilter),
    queryFn: async () =>
      parseTopicCards(await api.listContentTopics(accountFilter)),
  });
}

export function useContentTopic(workspaceId: string, topicCardId: string) {
  return useQuery<TopicCard>({
    queryKey: topicPlanningKeys.detail(workspaceId, topicCardId),
    enabled: !!topicCardId,
    queryFn: async () => parseTopicCard(await api.getContentTopic(topicCardId)),
  });
}

export function useContentBriefs(workspaceId: string, topicCardId: string) {
  return useQuery<BriefRevision[]>({
    queryKey: topicPlanningKeys.briefs(workspaceId, topicCardId),
    enabled: !!topicCardId,
    queryFn: async () =>
      parseBriefRevisions(await api.listContentBriefs(topicCardId)),
  });
}

export function useContentBrief(
  workspaceId: string,
  topicCardId: string,
  revisionId: string,
) {
  return useQuery<BriefRevision>({
    queryKey: topicPlanningKeys.brief(workspaceId, topicCardId, revisionId),
    enabled: !!topicCardId && !!revisionId,
    queryFn: async () =>
      parseBriefRevision(await api.getContentBrief(topicCardId, revisionId)),
  });
}

export function useCreateContentTopic(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: TopicCardInput) =>
      parseTopicCard(await api.createContentTopic(topicCardInputToWire(input))),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.all(workspaceId),
      }),
  });
}

export function useActOnContentTopic(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      topicCardId: string;
      body: TopicActionInput;
    }) =>
      parseTopicActionResult(
        await api.actOnContentTopic(
          input.topicCardId,
          topicActionInputToWire(input.body),
        ),
      ),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.all(workspaceId),
      }),
  });
}

// Changing which account a card belongs to invalidates the whole workspace
// prefix: the card's own detail, and every filtered list — the one it left and
// the one it joined.
export function useSetContentTopicAccount(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      topicCardId: string;
      accountId: string | null;
    }) =>
      parseTopicCard(
        await api.setContentTopicAccount(input.topicCardId, input.accountId),
      ),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.all(workspaceId),
      }),
  });
}

// Whole-column replacement of the card's referenced material sources
// (specs/030).
export function useSetContentTopicSources(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      topicCardId: string;
      sources: SetContentTopicSourcesInput;
    }) =>
      parseTopicCard(
        await api.setContentTopicSources(
          input.topicCardId,
          setContentTopicSourcesInputToWire(input.sources),
        ),
      ),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.all(workspaceId),
      }),
  });
}

export function usePatchContentTopicBody(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      topicCardId: string;
      body: TopicBodyPatchInput;
    }) =>
      parseTopicCard(
        await api.patchContentTopicBody(
          input.topicCardId,
          topicBodyPatchInputToWire(input.body),
        ),
      ),
    onSuccess: () =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.all(workspaceId),
      }),
  });
}

export function useAppendContentBrief(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      topicCardId: string;
      body: BriefRevisionInput;
    }) =>
      parseBriefRevision(
        await api.appendContentBrief(
          input.topicCardId,
          briefRevisionInputToWire(input.body),
        ),
      ),
    onSuccess: (_revision, input) =>
      client.invalidateQueries({
        queryKey: topicPlanningKeys.briefs(workspaceId, input.topicCardId),
      }),
  });
}
