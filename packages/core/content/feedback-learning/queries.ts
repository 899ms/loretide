import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseFeedbackExcerpt,
  parseFeedbackList,
  parseManualMetric,
  parseManualMetrics,
  parsePendingFeedback,
  type FeedbackExcerpt,
  type FeedbackList,
  type ManualMetric,
  type PendingFeedback,
} from "./contract";
import { toImportBody, type CsvRow } from "./csv";

// Server state for manual metrics and feedback excerpts. TanStack Query owns
// it.
//
// Nothing here is optimistic. Every one of these writes is a fact somebody is
// asserting about the world - a number they read off a platform, a thing
// somebody said - and none of it is locally predictable: the server decides
// whether the publication record is still there and whether every row of a
// paste is usable. Guessing the outcome would show numbers that were refused.
//
// Every key carries workspaceId. Without it, switching brands would serve the
// previous brand's numbers out of cache until a refetch landed.

export const feedbackKeys = {
  all: (workspaceId: string) => ["contentFeedback", workspaceId] as const,
  metrics: (workspaceId: string, publicationRecordId = "", metric = "") =>
    ["contentMetrics", workspaceId, "list", publicationRecordId, metric] as const,
  excerpts: (workspaceId: string, publicationRecordId = "") =>
    ["contentFeedback", workspaceId, "list", publicationRecordId] as const,
  pending: (workspaceId: string) => ["contentFeedback", workspaceId, "pending"] as const,
};

export function useContentMetrics(workspaceId: string, publicationRecordId = "", metric = "") {
  return useQuery<ManualMetric[]>({
    queryKey: feedbackKeys.metrics(workspaceId, publicationRecordId, metric),
    queryFn: async () =>
      parseManualMetrics(await api.listContentMetrics(publicationRecordId, metric)),
  });
}

export function useContentFeedback(workspaceId: string, publicationRecordId = "") {
  return useQuery<FeedbackList>({
    queryKey: feedbackKeys.excerpts(workspaceId, publicationRecordId),
    queryFn: async () => parseFeedbackList(await api.listContentFeedback(publicationRecordId)),
  });
}

/** SOP 2's fifth workbench item. */
export function usePendingFeedback(workspaceId: string) {
  return useQuery<PendingFeedback[]>({
    queryKey: feedbackKeys.pending(workspaceId),
    queryFn: async () => parsePendingFeedback(await api.listContentFeedbackPending()),
  });
}

export interface RecordMetricInput {
  publicationRecordId: string;
  platform: string;
  accountId?: string;
  metric: string;
  /** null means the platform does not show this number. Callers pass null,
   *  never 0, when they do not know. */
  value: number | null;
  unit?: string;
  statWindow?: string;
  sampledAt: string;
  evidenceNote?: string;
}

/**
 * Recording one observation from the form.
 *
 * There is no sourceType here and there must not be: which endpoint is called
 * is what the server records as the origin, and a caller that could declare
 * its own origin is not reporting one.
 */
export function useRecordContentMetric(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<ManualMetric, Error, RecordMetricInput>({
    mutationFn: async (input) =>
      parseManualMetric(
        await api.recordContentMetric({
          publication_record_id: input.publicationRecordId,
          platform: input.platform,
          account_id: input.accountId ?? "",
          metric: input.metric,
          // Passed straight through. A `?? 0` here would turn "I do not know"
          // into an observation nobody made.
          value: input.value,
          unit: input.unit ?? "",
          stat_window: input.statWindow ?? "",
          sampled_at: input.sampledAt,
          evidence_note: input.evidenceNote ?? "",
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentMetrics", workspaceId] });
      // Recording a number takes the piece off the pending list.
      void client.invalidateQueries({ queryKey: feedbackKeys.pending(workspaceId) });
    },
  });
}

/**
 * Importing a pasted block, all or nothing.
 *
 * The server refuses the whole batch on the first bad row and says which one.
 * Nothing partial ever lands, so there is no "3 of 40 got in" state for a
 * caller to have to explain.
 */
export function useImportContentMetrics(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<ManualMetric[], Error, { publicationRecordId: string; rows: CsvRow[] }>({
    mutationFn: async ({ publicationRecordId, rows }) =>
      parseManualMetrics(await api.importContentMetrics(toImportBody(publicationRecordId, rows))),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentMetrics", workspaceId] });
      void client.invalidateQueries({ queryKey: feedbackKeys.pending(workspaceId) });
    },
  });
}

export interface RecordExcerptInput {
  publicationRecordId: string;
  sourceType: string;
  /** Redacted by the person writing it. Nothing redacts for them. */
  redactedExcerpt?: string;
  /** The operator's own reading, kept apart from the quote. */
  interpretation?: string;
  tags?: string[];
  occurredAt: string;
}

export function useRecordContentFeedback(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<FeedbackExcerpt, Error, RecordExcerptInput>({
    mutationFn: async (input) =>
      parseFeedbackExcerpt(
        await api.recordContentFeedback({
          publication_record_id: input.publicationRecordId,
          source_type: input.sourceType,
          redacted_excerpt: input.redactedExcerpt ?? "",
          interpretation: input.interpretation ?? "",
          tags: input.tags ?? [],
          occurred_at: input.occurredAt,
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentFeedback", workspaceId] });
    },
  });
}
