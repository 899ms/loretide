// @vitest-environment node
import { describe, expect, it } from "vitest";
import { EMPTY_TOPIC_CARD } from "./contract";
import {
  EMPTY_MARKETING_NODE,
  EMPTY_NODE_CANDIDATE,
  adoptToWire,
  candidatePatchToWire,
  impactDecisionToWire,
  parseAdoptResult,
  parseImpactItems,
  parseNodeCandidate,
  parseNodeCandidates,
  marketingNodeInputToWire,
  parseImportResult,
  parseMarketingNode,
  parseMarketingNodes,
  parseNodeRevisions,
  type MarketingNodeInput,
} from "./marketing-nodes";

function validRevision(overrides: Record<string, unknown> = {}) {
  return {
    revision_id: "rev-1",
    node_id: "node-1",
    workspace_id: "ws-1",
    revision: 1,
    change_kind: "create",
    status_after: "active",
    name: "双十一",
    kind: "marketing",
    starts_on: "2026-11-11",
    ends_on: "2026-11-11",
    timezone: "Asia/Shanghai",
    lead_days: 14,
    accounts: [{ account_id: "a1", role: "主推" }],
    goal: "清库存",
    material_source_ids: ["s1"],
    date_certainty: "confirmed",
    date_basis: "",
    note: "",
    actor: "user-1",
    created_at: "2026-09-25T00:00:00Z",
    ...overrides,
  };
}

function validNode(overrides: Record<string, unknown> = {}) {
  return {
    node_id: "node-1",
    workspace_id: "ws-1",
    status: "active",
    origin: "manual",
    current_revision: 1,
    current: validRevision(),
    timing: {
      today: "2026-11-11",
      timezone: "Asia/Shanghai",
      phase: "live",
      preparation_starts_on: "2026-10-28",
    },
    created_at: "2026-09-25T00:00:00Z",
    updated_at: "2026-09-25T00:00:00Z",
    ...overrides,
  };
}

function validImportRow(overrides: Record<string, unknown> = {}) {
  return { row: 1, outcome: "created", node_id: "node-1", ...overrides };
}

describe("marketing node parsing", () => {
  it("reads a well-formed node", () => {
    const node = parseMarketingNode(validNode());
    expect(node.nodeId).toBe("node-1");
    expect(node.timing.phase).toEqual({ kind: "live" });
    expect(node.current.leadDays).toEqual({ set: true, value: 14 });
    expect(node.current.accounts).toEqual([{ accountId: "a1", role: "主推" }]);
  });

  it("falls back when a node field is missing, has the wrong type, or the node is not an object", () => {
    const { timing: _missing, ...noTiming } = validNode();
    expect(parseMarketingNode(noTiming)).toEqual(EMPTY_MARKETING_NODE);
    expect(parseMarketingNode(validNode({ current_revision: "1" }))).toEqual(
      EMPTY_MARKETING_NODE,
    );
    expect(parseMarketingNode("node-1")).toEqual(EMPTY_MARKETING_NODE);
  });

  it("drops only the malformed node from a list", () => {
    const { node_id: _missing, ...noId } = validNode({ node_id: "x" });
    const nodes = parseMarketingNodes({
      marketing_nodes: [
        validNode(),
        noId,
        validNode({ node_id: "node-2", status: 7 }),
        42,
        validNode({ node_id: "node-3" }),
      ],
    });
    expect(nodes.map((node) => node.nodeId)).toEqual(["node-1", "node-3"]);
    expect(parseMarketingNodes({ marketing_nodes: "none" })).toEqual([]);
    expect(parseMarketingNodes(null)).toEqual([]);
  });

  it("keeps an unknown phase and change kind with their raw value", () => {
    const node = parseMarketingNode(
      validNode({
        timing: {
          today: "2026-11-11",
          timezone: "UTC",
          phase: "cooling_down",
          preparation_starts_on: null,
        },
        current: validRevision({ change_kind: "merge" }),
      }),
    );
    expect(node.nodeId).toBe("node-1");
    expect(node.timing.phase).toEqual({ kind: "unknown", raw: "cooling_down" });
    expect(node.current.changeKind).toEqual({ kind: "unknown", raw: "merge" });
  });

  it("never reads a missing lead time as zero", () => {
    const { lead_days: _missing, ...noLead } = validRevision();
    const [missing, nullLead, zero] = parseNodeRevisions({
      revisions: [noLead, validRevision({ lead_days: null }), validRevision({ lead_days: 0 })],
    });
    expect(missing?.leadDays).toEqual({ set: false });
    expect(nullLead?.leadDays).toEqual({ set: false });
    expect(zero?.leadDays).toEqual({ set: true, value: 0 });
  });

  it("drops a revision that is missing a field, has the wrong type, or is not an object", () => {
    const { actor: _missing, ...noActor } = validRevision({ revision: 2 });
    const revisions = parseNodeRevisions({
      revisions: [
        validRevision(),
        noActor,
        validRevision({ revision: 3, lead_days: "14" }),
        "rev",
        validRevision({ revision: 4 }),
      ],
    });
    expect(revisions.map((revision) => revision.revision)).toEqual([1, 4]);
  });

  it("reads import outcomes, keeps unknown ones, and drops malformed rows", () => {
    const { row: _missing, ...noRow } = validImportRow();
    const results = parseImportResult({
      results: [
        validImportRow(),
        validImportRow({ row: 2, outcome: "invalid", node_id: undefined, field: "kind" }),
        validImportRow({ row: 3, outcome: "merged" }),
        noRow,
        validImportRow({ row: "5" }),
        null,
      ],
    });
    expect(results).toEqual([
      { row: 1, outcome: { kind: "created" }, nodeId: "node-1", field: "" },
      { row: 2, outcome: { kind: "invalid" }, nodeId: "", field: "kind" },
      { row: 3, outcome: { kind: "unknown", raw: "merged" }, nodeId: "node-1", field: "" },
    ]);
  });
});

describe("marketing node request bodies", () => {
  const input: MarketingNodeInput = {
    name: "双十一",
    kind: "marketing",
    startsOn: "2026-11-11",
    endsOn: "2026-11-11",
    timezone: "Asia/Shanghai",
    leadDays: { set: false },
    accounts: [{ accountId: "a1", role: "主推" }],
    goal: "",
    materialSourceIds: [],
    dateCertainty: "confirmed",
    dateBasis: "",
  };

  it("sends null for an unset lead time and the number for zero", () => {
    expect(marketingNodeInputToWire(input).lead_days).toBeNull();
    expect(
      marketingNodeInputToWire({ ...input, leadDays: { set: true, value: 0 } }).lead_days,
    ).toBe(0);
    expect(marketingNodeInputToWire(input).accounts).toEqual([
      { account_id: "a1", role: "主推" },
    ]);
  });
});

// specs/033 PR 2: candidate, impact item and adoption result shapes (T047,
// SC-015). Each shape gets the three malformed cases - a field missing, a
// field of the wrong type, not an object - and unknown values are kept.

function validCandidate(overrides: Record<string, unknown> = {}) {
  return {
    candidate_id: "c1",
    workspace_id: "ws-1",
    node_id: "node-1",
    account_id: "a1",
    angle: "",
    status: "open",
    dismiss_reason: "",
    topic_card_id: "",
    adopted_revision: null,
    impact_decision: "",
    impact_decision_note: "",
    impact_decision_revision: null,
    impact_decided_by: "",
    created_at: "2026-09-25T00:00:00Z",
    updated_at: "2026-09-25T00:00:00Z",
    in_scope: true,
    timing: {
      today: "2026-11-01",
      timezone: "Asia/Shanghai",
      phase: "preparing",
      preparation_starts_on: "2026-10-28",
      days_until_start: 10,
      lead_days: 14,
      lead_short: true,
    },
    relation: {
      goal: "清库存 + 拉新",
      role: "主推",
      account: {
        audience: { value: "", status: "pending" },
        content_pillars: { value: "选品", status: "confirmed" },
        content_goals: { value: "拉新", status: "confirmed" },
      },
    },
    collisions: [{ node_id: "n2", name: "预热周", starts_on: "2026-11-05", ends_on: "2026-11-06" }],
    material_gaps: [{ kind: "archived", source_id: "s2" }],
    duplicate_risks: [{ topic_card_id: "t9", reason: "name_match" }],
    origin: "manual",
    date_certainty: "confirmed",
    date_basis: "",
    ...overrides,
  };
}

function validImpactItem(overrides: Record<string, unknown> = {}) {
  return {
    candidate_id: "c1",
    topic_card_id: "t1",
    account_id: "a1",
    card_status: "draft",
    adopted_revision: 1,
    current_revision: 3,
    before: { starts_on: "2026-11-11", ends_on: "2026-11-11", timezone: "Asia/Shanghai", lead_days: 14 },
    after: { starts_on: "2026-11-18", ends_on: "2026-11-18", timezone: "Asia/Shanghai", lead_days: null },
    cancelled: false,
    ...overrides,
  };
}

describe("marketing node candidate parsing", () => {
  it("reads a well-formed candidate", () => {
    const candidate = parseNodeCandidate(validCandidate());
    expect(candidate.candidateId).toBe("c1");
    expect(candidate.timing.leadShort).toBe(true);
    expect(candidate.timing.leadDays).toEqual({ set: true, value: 14 });
    expect(candidate.timing.phase).toEqual({ kind: "preparing" });
    expect(candidate.relation.account?.audience).toEqual({ value: "", status: "pending" });
    expect(candidate.materialGaps).toEqual([{ kind: { kind: "archived" }, sourceId: "s2" }]);
    expect(candidate.duplicateRisks).toEqual([
      { topicCardId: "t9", reason: { kind: "name_match" } },
    ]);
    expect(candidate.adoptedRevision).toBeNull();
  });

  it("falls back when a candidate field is missing, has the wrong type, or it is not an object", () => {
    const { in_scope: _missing, ...noScope } = validCandidate();
    expect(parseNodeCandidate(noScope)).toEqual(EMPTY_NODE_CANDIDATE);
    expect(parseNodeCandidate(validCandidate({ angle: 3 }))).toEqual(EMPTY_NODE_CANDIDATE);
    expect(parseNodeCandidate(["c1"])).toEqual(EMPTY_NODE_CANDIDATE);
  });

  it("reads a missing lead_short as unknown, never as enough", () => {
    const unset = validCandidate({
      timing: { ...validCandidate().timing, lead_days: null, lead_short: null },
    });
    const candidate = parseNodeCandidate(unset);
    expect(candidate.timing.leadShort).toBe("unknown");
    expect(candidate.timing.leadDays).toEqual({ set: false });
    // Only a literal true is a warning.
    const odd = parseNodeCandidate(
      validCandidate({ timing: { ...validCandidate().timing, lead_short: "true" } }),
    );
    expect(odd.timing.leadShort).toBe(false);
  });

  it("keeps unknown gap kinds, duplicate reasons and phases with their raw value", () => {
    const candidate = parseNodeCandidate(
      validCandidate({
        material_gaps: [{ kind: "restricted", source_id: "s3" }, { kind: "none" }],
        duplicate_risks: [{ topic_card_id: "t2", reason: "same_week" }],
        timing: { ...validCandidate().timing, phase: "paused" },
      }),
    );
    expect(candidate.materialGaps).toEqual([
      { kind: { kind: "unknown", raw: "restricted" }, sourceId: "s3" },
      { kind: { kind: "none" }, sourceId: "" },
    ]);
    expect(candidate.duplicateRisks[0]?.reason).toEqual({ kind: "unknown", raw: "same_week" });
    expect(candidate.timing.phase).toEqual({ kind: "unknown", raw: "paused" });
  });

  it("drops a malformed entry inside a candidate, not the candidate", () => {
    const candidate = parseNodeCandidate(
      validCandidate({
        collisions: [{ node_id: "n2" }, validCandidate().collisions[0]],
        material_gaps: "none",
        relation: { goal: "g", role: "", account: { audience: 1 } },
      }),
    );
    expect(candidate.candidateId).toBe("c1");
    expect(candidate.collisions).toHaveLength(1);
    expect(candidate.materialGaps).toEqual([]);
    expect(candidate.relation.account).toBeNull();
  });

  it("drops one malformed candidate and keeps the rest of the list", () => {
    const list = parseNodeCandidates({
      candidates: [validCandidate(), { candidate_id: 7 }, validCandidate({ candidate_id: "c2" })],
    });
    expect(list.map((c) => c.candidateId)).toEqual(["c1", "c2"]);
    expect(parseNodeCandidates({ candidates: "none" })).toEqual([]);
    expect(parseNodeCandidates(null)).toEqual([]);
  });
});

describe("marketing node impact parsing", () => {
  it("reads a well-formed impact item, unset lead kept apart from 0", () => {
    const [item] = parseImpactItems({ impact: [validImpactItem()] });
    expect(item?.candidateId).toBe("c1");
    expect(item?.before.leadDays).toEqual({ set: true, value: 14 });
    expect(item?.after.leadDays).toEqual({ set: false });
    expect(item?.cardStatus).toBe("draft");
  });

  it("drops an item with a missing field, a wrong type, or that is not an object", () => {
    const { after: _missing, ...noAfter } = validImpactItem();
    const list = parseImpactItems({
      impact: [noAfter, validImpactItem({ cancelled: "yes" }), 42, validImpactItem({ candidate_id: "c2" })],
    });
    expect(list.map((item) => item.candidateId)).toEqual(["c2"]);
  });

  it("keeps an unknown card status as it came", () => {
    const [item] = parseImpactItems({ impact: [validImpactItem({ card_status: "archived_v2" })] });
    expect(item?.cardStatus).toBe("archived_v2");
  });
});

describe("marketing node adoption result parsing", () => {
  const card = {
    topic_card_id: "t1",
    workspace_id: "ws-1",
    account_id: "a1",
    audience_problem_judgment: "",
    ip_fit: "角度",
    timing: "双十一｜2026-11-11–2026-11-11（Asia/Shanghai）｜准备期自 2026-10-28",
    existing_content_relation: "",
    evidence_gaps_and_investment: "",
    channels: [],
    fit_source_ids: ["s1"],
    evidence_source_ids: [],
    recommended_action: "",
    status: "draft",
    decision_reason: "",
    decision_note: "",
    started_brief_revision_id: null,
    created_at: "2026-09-25T00:00:00Z",
    updated_at: "2026-09-25T00:00:00Z",
  };

  it("reads the candidate and the card", () => {
    const result = parseAdoptResult({
      candidate: validCandidate({ status: "adopted", topic_card_id: "t1", adopted_revision: 2 }),
      topic_card: card,
    });
    expect(result.candidate.adoptedRevision).toBe(2);
    expect(result.topicCard.topicCardId).toBe("t1");
  });

  it("falls back half by half when a part is missing, mistyped, or the result is not an object", () => {
    const noCard = parseAdoptResult({ candidate: validCandidate() });
    expect(noCard.candidate.candidateId).toBe("c1");
    expect(noCard.topicCard).toEqual(EMPTY_TOPIC_CARD);
    const badCandidate = parseAdoptResult({ candidate: "c1", topic_card: card });
    expect(badCandidate.candidate).toEqual(EMPTY_NODE_CANDIDATE);
    expect(badCandidate.topicCard.topicCardId).toBe("t1");
    const neither = parseAdoptResult("adopted");
    expect(neither.candidate).toEqual(EMPTY_NODE_CANDIDATE);
    expect(neither.topicCard).toEqual(EMPTY_TOPIC_CARD);
  });
});

describe("marketing node candidate requests", () => {
  it("sends only the keys that were given", () => {
    expect(candidatePatchToWire({ angle: "" })).toEqual({ angle: "" });
    expect(candidatePatchToWire({ status: "dismissed", dismissReason: "档期满" })).toEqual({
      status: "dismissed",
      dismiss_reason: "档期满",
    });
    expect(adoptToWire({ mode: "create" })).toEqual({ mode: "create" });
    expect(adoptToWire({ mode: "link", topicCardId: "t1" })).toEqual({
      mode: "link",
      topic_card_id: "t1",
    });
    expect(impactDecisionToWire("kept")).toEqual({ decision: "kept", note: "" });
  });
});
