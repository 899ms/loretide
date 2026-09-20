// @vitest-environment node
import { describe, expect, it } from "vitest";

import { emptyOperatingRules, readCadence, readObservation } from "./operating-rules";
import {
  canSaveDraft,
  cadenceDisplay,
  draftFromRules,
  draftProblem,
  draftToRules,
  observationDisplay,
  parseCount,
  type RulesDraft,
} from "./rules-form";

// What the form may submit and how a stored value should read, decided here
// rather than in JSX.

function emptyDraft(): RulesDraft {
  return { cadence: {}, templates: {}, observationDefault: "", observationByChannel: {} };
}

describe("parseCount", () => {
  // The single silent-corruption risk on this card. Number("") is 0 in
  // JavaScript, so a blank box reaching a naive parse becomes a decision
  // nobody made.
  it("reads a blank as null, not as zero", () => {
    expect(parseCount("")).toEqual({ ok: true, value: null });
    expect(parseCount("   ")).toEqual({ ok: true, value: null });
  });

  it("reads a typed zero as zero", () => {
    expect(parseCount("0")).toEqual({ ok: true, value: 0 });
  });

  it("keeps the two apart", () => {
    expect(parseCount("")).not.toEqual(parseCount("0"));
  });

  it.each([
    ["many", "not-a-number"],
    ["1.5", "not-a-whole-number"],
    ["-1", "negative"],
  ])("refuses %s", (raw, reason) => {
    expect(parseCount(raw)).toEqual({ ok: false, reason });
  });
});

describe("draftFromRules", () => {
  it("gives a brand that set nothing empty boxes, not zeroes", () => {
    const draft = draftFromRules(emptyOperatingRules());
    expect(draft.observationDefault).toBe("");
    expect(draft.observationDefault).not.toBe("0");
    expect(draft.cadence).toEqual({});
  });

  it("shows a stored zero as a typed zero", () => {
    const rules = emptyOperatingRules();
    rules.cadence.wechat_mp = 0;
    rules.observation.default = 0;
    const draft = draftFromRules(rules);
    expect(draft.cadence.wechat_mp).toBe("0");
    expect(draft.observationDefault).toBe("0");
  });
});

describe("draftToRules", () => {
  it("drops a cleared box rather than sending zero", () => {
    const draft = emptyDraft();
    draft.cadence.xiaohongshu = "";
    draft.observationDefault = "";

    const rules = draftToRules(draft, "self");
    // Absent, not 0: the server reads an absent key as "nobody decided", and
    // that is what a cleared box means.
    expect(readCadence(rules, "xiaohongshu").stored).toBe(false);
    expect(rules.observation.default).toBeUndefined();
    expect(rules.observation.default).not.toBe(0);
  });

  it("sends a typed zero", () => {
    const draft = emptyDraft();
    draft.cadence.wechat_mp = "0";
    draft.observationDefault = "0";

    const rules = draftToRules(draft, "self");
    expect(readCadence(rules, "wechat_mp")).toEqual({ value: 0, stored: true });
    expect(readObservation(rules, "wechat_mp")).toEqual({ days: 0, source: "global" });
  });

  it("drops an empty note so the stored blob does not grow on every visit", () => {
    const draft = emptyDraft();
    draft.templates.zhihu = "   ";
    draft.templates.douyin = "封面 16:9";
    const rules = draftToRules(draft, "self");
    expect(rules.templates.zhihu).toBeUndefined();
    expect(rules.templates.douyin).toEqual({ note: "封面 16:9" });
  });

  it("survives a round trip without changing what was stored", () => {
    const rules = emptyOperatingRules();
    rules.cadence.xiaohongshu = 3;
    rules.cadence.wechat_mp = 0;
    rules.templates.xiaohongshu = { note: "标题 20 字内" };
    rules.observation.default = 14;
    rules.observation.byChannel.douyin = 7;

    expect(draftToRules(draftFromRules(rules), rules.reviewRule)).toEqual(rules);
  });
});

describe("draftProblem", () => {
  it("passes an untouched form", () => {
    expect(draftProblem(emptyDraft())).toBeNull();
    expect(canSaveDraft(emptyDraft())).toBe(true);
  });

  it.each([
    ["cadence.xiaohongshu", (d: RulesDraft) => { d.cadence.xiaohongshu = "-1"; }, "negative"],
    ["cadence.douyin", (d: RulesDraft) => { d.cadence.douyin = "many"; }, "not-a-number"],
    ["observation.default", (d: RulesDraft) => { d.observationDefault = "1.5"; }, "not-a-whole-number"],
    ["observation.by_channel.zhihu", (d: RulesDraft) => { d.observationByChannel.zhihu = "-2"; }, "negative"],
  ])("points at %s", (field, mutate, reason) => {
    const draft = emptyDraft();
    mutate(draft);
    expect(draftProblem(draft)).toEqual({ field, reason });
    expect(canSaveDraft(draft)).toBe(false);
  });

  // The field names match what the server names in its 400, so a refusal that
  // does get through points at the same box the local check would have.
  it("uses the server's field names", () => {
    const draft = emptyDraft();
    draft.observationByChannel.douyin = "-1";
    expect(draftProblem(draft)?.field).toBe("observation.by_channel.douyin");
  });
});

describe("cadenceDisplay", () => {
  // Three cases, and the first two are why this function exists: a channel
  // nobody decided about and a channel deliberately set to zero print the same
  // number.
  it("separates never-set from deliberately zero", () => {
    const rules = emptyOperatingRules();
    rules.cadence.wechat_mp = 0;
    rules.cadence.xiaohongshu = 3;

    expect(cadenceDisplay(rules, "douyin")).toEqual({ kind: "unset" });
    expect(cadenceDisplay(rules, "wechat_mp")).toEqual({ kind: "paused" });
    expect(cadenceDisplay(rules, "xiaohongshu")).toEqual({ kind: "count", value: 3 });
  });

  it("never reports an unset channel as a count", () => {
    const display = cadenceDisplay(emptyOperatingRules(), "zhihu");
    expect(display.kind).not.toBe("count");
    expect(display.kind).not.toBe("paused");
  });
});

describe("observationDisplay", () => {
  it("says where the window came from", () => {
    const rules = emptyOperatingRules();
    rules.observation.default = 14;
    rules.observation.byChannel.douyin = 7;

    expect(observationDisplay(rules, "douyin")).toEqual({ kind: "days", days: 7, source: "channel" });
    expect(observationDisplay(rules, "xiaohongshu")).toEqual({ kind: "days", days: 14, source: "global" });
    expect(observationDisplay(emptyOperatingRules(), "xiaohongshu")).toEqual({ kind: "unset" });
  });

  it("reports a stored zero as a real window, not as unset", () => {
    const rules = emptyOperatingRules();
    rules.observation.default = 0;
    expect(observationDisplay(rules, "xiaohongshu")).toEqual({ kind: "days", days: 0, source: "global" });
  });
});
