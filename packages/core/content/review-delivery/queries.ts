import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  parseDelivery,
  parseDeliveries,
  parsePublication,
  parsePublications,
  parseReview,
  parseReviewDetail,
  parseReviews,
  type DeliveryTask,
  type PublicationRecord,
  type ReviewDetail,
  type ReviewRequest,
} from "./contract";

// Server state for review, delivery and publication. TanStack Query owns it.
//
// Nothing here is optimistic. Every one of these mutations changes what other
// people see - a disposition, a handover, a statement that something is live -
// and none of them is locally predictable: the server decides whether the move
// is legal, whether the review behind it is approved, and whether a required
// field is missing. Guessing the outcome would show an approval that the server
// then refuses.
//
// Every key carries workspaceId. Without it, switching brands would serve the
// previous brand's review queue out of cache until a refetch landed.

export const reviewDeliveryKeys = {
  all: (workspaceId: string) => ["contentReviews", workspaceId] as const,
  reviews: (workspaceId: string, artifactId = "", status = "") =>
    ["contentReviews", workspaceId, "list", artifactId, status] as const,
  review: (workspaceId: string, reviewId: string) =>
    ["contentReviews", workspaceId, "detail", reviewId] as const,
  deliveries: (workspaceId: string, artifactId = "", status = "") =>
    ["contentDeliveries", workspaceId, "list", artifactId, status] as const,
  publications: (workspaceId: string, artifactId = "") =>
    ["contentPublications", workspaceId, "list", artifactId] as const,
};

export function useContentReviews(workspaceId: string, artifactId = "", status = "") {
  return useQuery<ReviewRequest[]>({
    queryKey: reviewDeliveryKeys.reviews(workspaceId, artifactId, status),
    queryFn: async () => parseReviews(await api.listContentReviews(artifactId, status)),
  });
}

export function useContentReview(workspaceId: string, reviewId: string) {
  return useQuery<ReviewDetail>({
    queryKey: reviewDeliveryKeys.review(workspaceId, reviewId),
    enabled: !!reviewId,
    queryFn: async () => parseReviewDetail(await api.getContentReview(reviewId)),
  });
}

export function useContentDeliveries(workspaceId: string, artifactId = "", status = "") {
  return useQuery<DeliveryTask[]>({
    queryKey: reviewDeliveryKeys.deliveries(workspaceId, artifactId, status),
    queryFn: async () => parseDeliveries(await api.listContentDeliveries(artifactId, status)),
  });
}

export function useContentPublications(workspaceId: string, artifactId = "") {
  return useQuery<PublicationRecord[]>({
    queryKey: reviewDeliveryKeys.publications(workspaceId, artifactId),
    queryFn: async () => parsePublications(await api.listContentPublications(artifactId)),
  });
}

export interface SubmitReviewInput {
  artifactId: string;
  versionId: string;
  accountId: string;
  channel: string;
  startSnapshotId?: string;
}

export function useSubmitContentReview(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<ReviewRequest, Error, SubmitReviewInput>({
    mutationFn: async (input) =>
      parseReview(
        await api.submitContentReview({
          artifact_id: input.artifactId,
          version_id: input.versionId,
          account_id: input.accountId,
          channel: input.channel,
          start_snapshot_id: input.startSnapshotId ?? "",
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: reviewDeliveryKeys.all(workspaceId) });
    },
  });
}

export function useDecideContentReview(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<ReviewRequest, Error, { reviewId: string; status: string; note: string }>({
    mutationFn: async ({ reviewId, status, note }) =>
      parseReview(await api.decideContentReview(reviewId, { status, note })),
    onSettled: (_data, _error, variables) => {
      void client.invalidateQueries({ queryKey: reviewDeliveryKeys.all(workspaceId) });
      void client.invalidateQueries({
        queryKey: reviewDeliveryKeys.review(workspaceId, variables.reviewId),
      });
      // A disposition changes whether a task may move past draft, so the
      // delivery list is uncertain too.
      void client.invalidateQueries({ queryKey: ["contentDeliveries", workspaceId] });
    },
  });
}

export function useCreateContentDelivery(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<
    DeliveryTask,
    Error,
    { artifactId: string; channel: string; reviewRequestId: string }
  >({
    mutationFn: async (input) =>
      parseDelivery(
        await api.createContentDelivery({
          artifact_id: input.artifactId,
          channel: input.channel,
          review_request_id: input.reviewRequestId,
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentDeliveries", workspaceId] });
    },
  });
}

export interface AdvanceDeliveryInput {
  deliveryId: string;
  status?: string;
  scheduledAt?: string;
  handoffMethod?: string;
  reason?: string;
  /** SOP 9.3's deliberate "deliver the approved snapshot anyway". */
  keepApprovedSnapshot?: boolean;
}

export function useAdvanceContentDelivery(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<DeliveryTask, Error, AdvanceDeliveryInput>({
    mutationFn: async (input) =>
      parseDelivery(
        await api.advanceContentDelivery(input.deliveryId, {
          status: input.status ?? "",
          scheduled_at: input.scheduledAt ?? "",
          handoff_method: input.handoffMethod ?? "",
          reason: input.reason ?? "",
          keep_approved_snapshot: input.keepApprovedSnapshot === true,
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentDeliveries", workspaceId] });
    },
  });
}

export interface RecordPublicationInput {
  artifactId: string;
  deliveryTaskId?: string;
  channel: string;
  status: string;
  declaredBy?: string;
  pageUrlOrContentId?: string;
  receiptNote?: string;
  verificationNote?: string;
  publishedAt?: string;
  platformAccount?: string;
  platformEdited?: boolean;
  editNote?: string;
  versionMatch?: string;
}

/**
 * Writing down what a person says happened on a platform.
 *
 * There is no actorId here and there must not be: who typed it comes from the
 * session, because it is the one field on the row that must not be forgeable.
 * Who SAID it is declaredBy, free text.
 */
export function useRecordContentPublication(workspaceId: string) {
  const client = useQueryClient();
  return useMutation<PublicationRecord, Error, RecordPublicationInput>({
    mutationFn: async (input) =>
      parsePublication(
        await api.recordContentPublication({
          artifact_id: input.artifactId,
          delivery_task_id: input.deliveryTaskId ?? "",
          channel: input.channel,
          status: input.status,
          declared_by: input.declaredBy ?? "",
          page_url_or_content_id: input.pageUrlOrContentId ?? "",
          receipt_note: input.receiptNote ?? "",
          verification_note: input.verificationNote ?? "",
          published_at: input.publishedAt ?? "",
          platform_account: input.platformAccount ?? "",
          platform_edited: input.platformEdited === true,
          edit_note: input.editNote ?? "",
          version_match: input.versionMatch ?? "unknown",
        }),
      ),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["contentPublications", workspaceId] });
      // Recording one clears the derived 待登记 flag on the task.
      void client.invalidateQueries({ queryKey: ["contentDeliveries", workspaceId] });
    },
  });
}
