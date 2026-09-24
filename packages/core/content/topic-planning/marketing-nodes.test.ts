// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  EMPTY_MARKETING_NODE,
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
