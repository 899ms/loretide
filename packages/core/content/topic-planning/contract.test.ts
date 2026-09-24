// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_BRIEF_REVISION,
  EMPTY_TOPIC_CARD,
  parseBriefRevision,
  parseTopicCard,
  setContentTopicSourcesInputToWire,
  topicBodyPatchInputToWire,
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

  it("parses valid source reference ids and defaults missing ones to empty array", () => {
    const card = parseTopicCard({
      ...validTopic(),
      fit_source_ids: ["source-1", "source-2"],
      evidence_source_ids: ["source-3"],
    });
    expect(card.fitSourceIds).toEqual(["source-1", "source-2"]);
    expect(card.evidenceSourceIds).toEqual(["source-3"]);

    const missing = parseTopicCard(validTopic());
    expect(missing.fitSourceIds).toEqual([]);
    expect(missing.evidenceSourceIds).toEqual([]);
  });

  it("catches malformed source arrays to empty array without degrading whole card", () => {
    const card = parseTopicCard({
      ...validTopic(),
      fit_source_ids: "not-an-array",
      evidence_source_ids: [123, null],
    });
    expect(card.topicCardId).toBe("topic-1");
    expect(card.fitSourceIds).toEqual([]);
    expect(card.evidenceSourceIds).toEqual([]);
  });

  it("converts SetContentTopicSourcesInput to wire with absence != clear", () => {
    expect(
      setContentTopicSourcesInputToWire({
        fitSourceIds: ["s-1"],
      }),
    ).toEqual({
      fit_source_ids: ["s-1"],
    });

    expect(
      setContentTopicSourcesInputToWire({
        fitSourceIds: [],
        evidenceSourceIds: ["s-2"],
      }),
    ).toEqual({
      fit_source_ids: [],
      evidence_source_ids: ["s-2"],
    });
  });
});

// PATCH /api/content-topics/{id}/body (#240): omitted keys keep the stored
// answer, "" clears it. The wire body carries exactly the keys the operator
// changed, and never anything outside the five.
describe("topicBodyPatchInputToWire", () => {
  it("sends only the keys that are present", () => {
    expect(topicBodyPatchInputToWire({ timing: "本周写" })).toEqual({ timing: "本周写" });
  });

  it("keeps an empty string as a deliberate clear", () => {
    const wire = topicBodyPatchInputToWire({ ipFit: "" });
    expect(wire).toEqual({ ip_fit: "" });
    expect("ip_fit" in wire).toBe(true);
  });

  it("maps all five keys to the server's snake_case names", () => {
    expect(
      topicBodyPatchInputToWire({
        audienceProblemJudgment: "a",
        ipFit: "b",
        timing: "c",
        existingContentRelation: "d",
        evidenceGapsAndInvestment: "e",
      }),
    ).toEqual({
      audience_problem_judgment: "a",
      ip_fit: "b",
      timing: "c",
      existing_content_relation: "d",
      evidence_gaps_and_investment: "e",
    });
  });

  it("drops anything outside the five, even if a caller smuggles it in", () => {
    // The server answers an unknown key with 400; the transport should never
    // be the thing that sends one.
    const smuggled = {
      timing: "x",
      channels: ["zhihu"],
      status: "started",
      account_id: "acct",
    } as unknown as Parameters<typeof topicBodyPatchInputToWire>[0];
    expect(topicBodyPatchInputToWire(smuggled)).toEqual({ timing: "x" });
  });

  it("produces an empty object for an empty input, which callers must not send", () => {
    expect(topicBodyPatchInputToWire({})).toEqual({});
  });
});
