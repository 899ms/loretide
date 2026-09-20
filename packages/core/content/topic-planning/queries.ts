import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  briefRevisionInputToWire,
  parseBriefRevision,
  parseBriefRevisions,
  parseTopicActionResult,
  parseTopicCard,
  parseTopicCards,
  topicActionInputToWire,
  topicCardInputToWire,
  type BriefRevision,
  type BriefRevisionInput,
  type TopicActionInput,
  type TopicCard,
  type TopicCardInput,
} from "./contract";

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
};

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
