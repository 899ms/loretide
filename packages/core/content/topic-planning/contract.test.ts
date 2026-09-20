// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_BRIEF_REVISION,
  EMPTY_TOPIC_CARD,
  parseBriefRevision,
  parseTopicCard,
  topicStatusLabel,
} from "./contract";

function validTopic() {
  return {
    topic_card_id: "topic-1",
    workspace_id: "workspace-1",
    account_id: null,
    audience_problem_judgment: "audience/problem/judgment",
    ip_fit: "fit",
    timing: "没有时效依据",
    existing_content_relation: "没有",
    evidence_gaps_and_investment: "gap",
    channels: ["zhihu"],
    recommended_action: "start",
    status: "draft",
    decision_reason: "",
    decision_note: "",
    started_brief_revision_id: null,
    created_at: "2026-09-19T00:00:00Z",
    updated_at: "2026-09-19T00:00:00Z",
  };
}

function validBrief() {
  return {
    brief_revision_id: "brief-1",
    topic_card_id: "topic-1",
    workspace_id: "workspace-1",
    revision: 2,
    audience: "audience",
    core_problem: "problem",
    claim_and_boundaries: "boundary",
    channels: ["zhihu"],
    format: "article",
    structure: "problem-evidence-action",
    citation_requirements: "public sources",
    source_scope: "provided URLs",
    deliverable: "draft",
    time_limit: "two hours",
    cost_limit: "zero",
    created_at: "2026-09-19T00:00:00Z",
  };
}

describe("topic planning response parsing", () => {
  it("falls back when a required field is missing", () => {
    const { recommended_action: _missing, ...malformed } = validTopic();
    expect(parseTopicCard(malformed)).toEqual(EMPTY_TOPIC_CARD);
  });

  it("falls back when a field has the wrong type", () => {
    expect(parseTopicCard({ ...validTopic(), channels: "zhihu" })).toEqual(
      EMPTY_TOPIC_CARD,
    );
  });

  it("preserves a future server status without blanking the card", () => {
    expect(
      parseTopicCard({ ...validTopic(), status: "archived" }),
    ).toMatchObject({
      topicCardId: "topic-1",
      status: "archived",
    });
  });

  it("keeps the default branch for a future server status", () => {
    expect(topicStatusLabel("future-status")).toBe("Unknown");
  });

  it("rejects malformed brief versions instead of casting network JSON", () => {
    expect(parseBriefRevision({ ...validBrief(), revision: "2" })).toEqual(
      EMPTY_BRIEF_REVISION,
    );
  });
});
