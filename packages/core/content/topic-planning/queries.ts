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
  list: (workspaceId: string) => ["contentTopics", workspaceId, "list"] as const,
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

export function useContentTopics(workspaceId: string) {
  return useQuery<TopicCard[]>({
    queryKey: topicPlanningKeys.list(workspaceId),
    queryFn: async () => parseTopicCards(await api.listContentTopics()),
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
      parseBriefRevision(
        await api.getContentBrief(topicCardId, revisionId),
      ),
  });
}

export function useCreateContentTopic(workspaceId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: TopicCardInput) =>
      parseTopicCard(await api.createContentTopic(topicCardInputToWire(input))),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: topicPlanningKeys.all(workspaceId) }),
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
