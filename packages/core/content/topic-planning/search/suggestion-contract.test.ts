// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  SUGGESTION_ASPECTS, SUGGESTION_AUTHOR_KINDS, SUGGESTION_DECISIONS, SUGGESTION_EFFECT_OUTCOMES,
  SUGGESTION_EFFECT_FAILURES, SUGGESTION_STATES,
  parseSearchSuggestion, parseSearchSuggestionList, parseSearchSuggestionComparison,
} from "./suggestion-contract";

const wire = {
  suggestion_id: "s1", revision: 2, work_id: "w1", artifact_id: "a1", base_version_id: "v1",
  theme_id: "t1", theme_revision: 3, target_question: "如何清洗？", aspects: ["body"],
  rationale: "回应客户提问", evidence_source_ids: ["e1"], proposed_body: "保留空行\n\n",
  author_kind: "human", recorded_by: "u1", created_at: "2026-09-25T01:00:00Z",
  state: "open", base_is_current: true, theme_changed: false, decision: null, effects: null,
};
const decision = { decision_id: "d1", suggestion_id: "s1", suggestion_revision: 2,
  decision: "adopt", note: "", decided_by: "u1", created_at: wire.created_at };
const effect = { effect_id: "f1", decision_id: "d1", outcome: "failed", version_id: "",
  failure_code: "storage", created_at: wire.created_at };

describe("suggestion response contract", () => {
  it("matches all Go controlled sets", () => {
    const go = readFileSync(join(__dirname, "../../../../../server/internal/content/topic-planning/search_contract.go"), "utf8");
    for (const [name, values] of Object.entries({ SuggestionAspects: SUGGESTION_ASPECTS,
      AuthorKinds: SUGGESTION_AUTHOR_KINDS, SuggestionDecisions: SUGGESTION_DECISIONS,
      EffectOutcomes: SUGGESTION_EFFECT_OUTCOMES, EffectFailures: SUGGESTION_EFFECT_FAILURES,
      SuggestionStates: SUGGESTION_STATES })) {
      const names = new RegExp(`var ${name} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(go)?.[1];
      expect(names, name).toBeDefined();
      const literals = names!.split(",").map((n) => n.trim()).filter(Boolean).map((n) =>
        new RegExp(`${n}\\s+[A-Za-z]+ = "([^"]+)"`).exec(go)?.[1]);
      expect([...values], name).toEqual(literals);
    }
  });
  it.each(SUGGESTION_STATES)("retains server state %s without inference", (state) => {
    expect(parseSearchSuggestion({ ...wire, state })?.state).toBe(state);
  });
  it.each(SUGGESTION_EFFECT_FAILURES)("retains failure %s", (failure_code) => {
    expect(parseSearchSuggestion({ ...wire, state: "adopt_failed", failure_code, decision,
      effects: [{ ...effect, failure_code }] })).toMatchObject({ failureCode: failure_code,
      decision: { decisionId: "d1", suggestionRevision: 2 }, effects: [{ failureCode: failure_code }] });
  });
  it("does not interpret absent effects as successful adoption", () => {
    expect(parseSearchSuggestion({ ...wire, state: "adopt_unrecorded", decision })).toMatchObject({
      state: "adopt_unrecorded", effects: [], failureCode: "", diff: null,
    });
  });
  it("preserves the body and ordered diff text, including blank lines", () => {
    const ops = [{ op: "equal", text: "第一行\n" }, { op: "delete", text: "旧\n" },
      { op: "insert", text: "新\n\n" }];
    expect(parseSearchSuggestion({ ...wire, diff: { ops, inserted_lines: 2, deleted_lines: 1 } }))
      .toMatchObject({ proposedBody: wire.proposed_body, diff: { ops, insertedLines: 2, deletedLines: 1 } });
  });
  it("degrades unknown controlled values without discarding the record", () => {
    expect(parseSearchSuggestion({ ...wire, state: "future", failure_code: "future", author_kind: "robot",
      aspects: ["future"], decision: { ...decision, decision: "future" },
      effects: [{ ...effect, outcome: "future", failure_code: "future" }] })).toMatchObject({
      state: "unknown", failureCode: "unknown", authorKind: "unknown", aspects: ["unknown"],
      decision: { decision: "unknown" }, effects: [{ outcome: "unknown", failureCode: "unknown" }],
    });
  });
  it.each([null, {}, { ...wire, revision: 0 }, { ...wire, revision: 1.5 },
    { ...wire, proposed_body: 12 }, { ...wire, base_is_current: "true" },
    { ...wire, effects: [{}] }, { ...wire, decision: {} },
    { ...wire, diff: { ops: [], inserted_lines: -1, deleted_lines: 0 } }])("rejects malformed detail %#", (data) => {
    expect(parseSearchSuggestion(data)).toBeNull();
  });
  it("accepts empty lists and refuses malformed list entries", () => {
    expect(parseSearchSuggestionList({ suggestions: null })).toEqual([]);
    expect(parseSearchSuggestionList({ suggestions: [wire] })).toHaveLength(1);
    expect(parseSearchSuggestionList({ suggestions: [wire, {}] })).toEqual([]);
    expect(parseSearchSuggestionList(null)).toEqual([]);
  });
  it("preserves comparison order and same-base fact", () => {
    const result = parseSearchSuggestionComparison({ suggestions: [{ ...wire, suggestion_id: "s2" }, wire], same_base: false });
    expect(result?.suggestions.map((s) => s.suggestionId)).toEqual(["s2", "s1"]);
    expect(result?.sameBase).toBe(false);
    expect(parseSearchSuggestionComparison({ suggestions: [wire], same_base: "false" })).toBeNull();
    expect(parseSearchSuggestionComparison({ suggestions: [{}], same_base: true })).toBeNull();
  });
});
