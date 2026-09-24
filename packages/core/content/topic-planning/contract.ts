import { z } from "zod";
import { parseWithFallback } from "@multica/core/api/schema";

// Status is deliberately open-ended on reads. An older installed client can
// receive a state introduced by a newer server and render the generic label
// instead of replacing the whole card with the fallback.
export const topicStatusSchema = z.string();

export const topicCardSchema = z
  .object({
    topic_card_id: z.string(),
    workspace_id: z.string(),
    account_id: z.string().nullable(),
    audience_problem_judgment: z.string(),
    ip_fit: z.string(),
    timing: z.string(),
    existing_content_relation: z.string(),
    evidence_gaps_and_investment: z.string(),
    channels: z.array(z.string()),
    recommended_action: z.string(),
    fit_source_ids: z.array(z.string()).catch([]),
    evidence_source_ids: z.array(z.string()).catch([]),
    status: topicStatusSchema,
    decision_reason: z.string(),
    decision_note: z.string(),
    started_brief_revision_id: z.string().nullable(),
    created_at: z.string(),
    updated_at: z.string(),
  })
  .loose()
  .transform((card) => ({
    topicCardId: card.topic_card_id,
    workspaceId: card.workspace_id,
    accountId: card.account_id,
    audienceProblemJudgment: card.audience_problem_judgment,
    ipFit: card.ip_fit,
    timing: card.timing,
    existingContentRelation: card.existing_content_relation,
    evidenceGapsAndInvestment: card.evidence_gaps_and_investment,
    channels: card.channels,
    recommendedAction: card.recommended_action,
    fitSourceIds: card.fit_source_ids,
    evidenceSourceIds: card.evidence_source_ids,
    status: card.status,
    decisionReason: card.decision_reason,
    decisionNote: card.decision_note,
    startedBriefRevisionId: card.started_brief_revision_id,
    createdAt: card.created_at,
    updatedAt: card.updated_at,
  }));

export const briefRevisionSchema = z
  .object({
    brief_revision_id: z.string(),
    topic_card_id: z.string(),
    workspace_id: z.string(),
    revision: z.number().int().positive(),
    audience: z.string(),
    core_problem: z.string(),
    claim_and_boundaries: z.string(),
    channels: z.array(z.string()),
    format: z.string(),
    structure: z.string(),
    citation_requirements: z.string(),
    source_scope: z.string(),
    deliverable: z.string(),
    time_limit: z.string(),
    cost_limit: z.string(),
    created_at: z.string(),
  })
  .loose()
  .transform((brief) => ({
    briefRevisionId: brief.brief_revision_id,
    topicCardId: brief.topic_card_id,
    workspaceId: brief.workspace_id,
    revision: brief.revision,
    audience: brief.audience,
    coreProblem: brief.core_problem,
    claimAndBoundaries: brief.claim_and_boundaries,
    channels: brief.channels,
    format: brief.format,
    structure: brief.structure,
    citationRequirements: brief.citation_requirements,
    sourceScope: brief.source_scope,
    deliverable: brief.deliverable,
    timeLimit: brief.time_limit,
    costLimit: brief.cost_limit,
    createdAt: brief.created_at,
  }));

export const topicCardListSchema = z
  .object({ topic_cards: z.array(topicCardSchema) })
  .loose()
  .transform((list) => ({ topicCards: list.topic_cards }));

export const briefRevisionListSchema = z
  .object({ brief_revisions: z.array(briefRevisionSchema) })
  .loose()
  .transform((list) => ({ briefRevisions: list.brief_revisions }));

export const topicActionResultSchema = z
  .object({
    topic_card: topicCardSchema,
    brief_revision: briefRevisionSchema.optional(),
  })
  .loose()
  .transform((result) => ({
    topicCard: result.topic_card,
    briefRevision: result.brief_revision,
  }));

export type TopicStatus = z.infer<typeof topicStatusSchema>;
export type TopicCard = z.infer<typeof topicCardSchema>;
export type BriefRevision = z.infer<typeof briefRevisionSchema>;
export type TopicActionResult = z.infer<typeof topicActionResultSchema>;

export interface TopicCardInput {
  accountId?: string | null;
  audienceProblemJudgment: string;
  ipFit: string;
  timing: string;
  existingContentRelation: string;
  evidenceGapsAndInvestment: string;
  channels: string[];
  recommendedAction: string;
  fitSourceIds?: string[];
  evidenceSourceIds?: string[];
}

export interface SetContentTopicSourcesInput {
  fitSourceIds?: string[];
  evidenceSourceIds?: string[];
}

// Omission preserves the stored answer. An empty string is deliberate: it
// clears one answer without changing the other four.
export interface TopicBodyPatchInput {
  audienceProblemJudgment?: string;
  ipFit?: string;
  timing?: string;
  existingContentRelation?: string;
  evidenceGapsAndInvestment?: string;
}

export interface BriefRevisionInput {
  audience: string;
  coreProblem: string;
  claimAndBoundaries: string;
  channels: string[];
  format: string;
  structure: string;
  citationRequirements: string;
  sourceScope: string;
  deliverable: string;
  timeLimit: string;
  costLimit: string;
}

export interface TopicActionInput {
  action: "start" | "save" | "defer" | "drop";
  reason?: string;
  note?: string;
  brief?: BriefRevisionInput;
}

export function topicCardInputToWire(
  input: TopicCardInput,
): Record<string, unknown> {
  const wire: Record<string, unknown> = {
    account_id: input.accountId ?? null,
    audience_problem_judgment: input.audienceProblemJudgment,
    ip_fit: input.ipFit,
    timing: input.timing,
    existing_content_relation: input.existingContentRelation,
    evidence_gaps_and_investment: input.evidenceGapsAndInvestment,
    channels: input.channels,
    recommended_action: input.recommendedAction,
  };
  if (input.fitSourceIds !== undefined) {
    wire.fit_source_ids = input.fitSourceIds;
  }
  if (input.evidenceSourceIds !== undefined) {
    wire.evidence_source_ids = input.evidenceSourceIds;
  }
  return wire;
}

export function setContentTopicSourcesInputToWire(
  input: SetContentTopicSourcesInput,
): Record<string, unknown> {
  const wire: Record<string, unknown> = {};
  if (input.fitSourceIds !== undefined) {
    wire.fit_source_ids = input.fitSourceIds;
  }
  if (input.evidenceSourceIds !== undefined) {
    wire.evidence_source_ids = input.evidenceSourceIds;
  }
  return wire;
}

export function topicBodyPatchInputToWire(
  input: TopicBodyPatchInput,
): Record<string, unknown> {
  const wire: Record<string, unknown> = {};
  if (input.audienceProblemJudgment !== undefined) {
    wire.audience_problem_judgment = input.audienceProblemJudgment;
  }
  if (input.ipFit !== undefined) {
    wire.ip_fit = input.ipFit;
  }
  if (input.timing !== undefined) {
    wire.timing = input.timing;
  }
  if (input.existingContentRelation !== undefined) {
    wire.existing_content_relation = input.existingContentRelation;
  }
  if (input.evidenceGapsAndInvestment !== undefined) {
    wire.evidence_gaps_and_investment = input.evidenceGapsAndInvestment;
  }
  return wire;
}

export function briefRevisionInputToWire(
  input: BriefRevisionInput,
): Record<string, unknown> {
  return {
    audience: input.audience,
    core_problem: input.coreProblem,
    claim_and_boundaries: input.claimAndBoundaries,
    channels: input.channels,
    format: input.format,
    structure: input.structure,
    citation_requirements: input.citationRequirements,
    source_scope: input.sourceScope,
    deliverable: input.deliverable,
    time_limit: input.timeLimit,
    cost_limit: input.costLimit,
  };
}

export function topicActionInputToWire(
  input: TopicActionInput,
): Record<string, unknown> {
  return {
    action: input.action,
    reason: input.reason ?? "",
    note: input.note ?? "",
    brief: input.brief ? briefRevisionInputToWire(input.brief) : {},
  };
}

export const EMPTY_TOPIC_CARD: TopicCard = {
  topicCardId: "",
  workspaceId: "",
  accountId: null,
  audienceProblemJudgment: "",
  ipFit: "",
  timing: "",
  existingContentRelation: "",
  evidenceGapsAndInvestment: "",
  channels: [],
  recommendedAction: "",
  fitSourceIds: [],
  evidenceSourceIds: [],
  status: "draft",
  decisionReason: "",
  decisionNote: "",
  startedBriefRevisionId: null,
  createdAt: "",
  updatedAt: "",
};

export const EMPTY_BRIEF_REVISION: BriefRevision = {
  briefRevisionId: "",
  topicCardId: "",
  workspaceId: "",
  revision: 1,
  audience: "",
  coreProblem: "",
  claimAndBoundaries: "",
  channels: [],
  format: "",
  structure: "",
  citationRequirements: "",
  sourceScope: "",
  deliverable: "",
  timeLimit: "",
  costLimit: "",
  createdAt: "",
};

export function parseTopicCard(data: unknown): TopicCard {
  return parseWithFallback(data, topicCardSchema, EMPTY_TOPIC_CARD, {
    endpoint: "content-topics/detail",
  });
}

export function parseTopicCards(data: unknown): TopicCard[] {
  return parseWithFallback(
    data,
    topicCardListSchema,
    { topicCards: [] as TopicCard[] },
    { endpoint: "content-topics/list" },
  ).topicCards;
}

export function parseBriefRevision(data: unknown): BriefRevision {
  return parseWithFallback(data, briefRevisionSchema, EMPTY_BRIEF_REVISION, {
    endpoint: "content-topics/brief",
  });
}

export function parseBriefRevisions(data: unknown): BriefRevision[] {
  return parseWithFallback(
    data,
    briefRevisionListSchema,
    { briefRevisions: [] as BriefRevision[] },
    { endpoint: "content-topics/briefs" },
  ).briefRevisions;
}

export function parseTopicActionResult(data: unknown): TopicActionResult {
  return parseWithFallback<TopicActionResult>(
    data,
    topicActionResultSchema,
    { topicCard: EMPTY_TOPIC_CARD, briefRevision: undefined },
    { endpoint: "content-topics/action" },
  );
}

export function topicStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return "Draft";
    case "started":
      return "Started";
    case "saved":
      return "Saved";
    case "deferred":
      return "Deferred";
    case "dropped":
      return "Dropped";
    default:
      return "Unknown";
  }
}
