// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  DELIVERABLE_PLATFORMS,
  REVIEW_RULES,
  RULE_PLATFORMS,
  emptyOperatingRules,
  isDeliverable,
  operatingRulesToRequest,
  parseOperatingRules,
  readCadence,
  readObservation,
  templateNoteFor,
} from "./operating-rules";
import { isWebLink, readHomepage } from "./homepage";

// The controlled sets are the server's. Restating them here is unavoidable - a
// component has to switch on them - so they are compared against the Go source
// rather than against my memory of it, the same way 025's and 027's are.
const goSource = readFileSync(
  join(__dirname, "../../../server/internal/content/workspace-core/operating_rules.go"),
  "utf8",
);

function goSlice(name: string): string[] {
  const declaration = new RegExp(`var ${name} = \\[\\]string\\{([^}]*)\\}`, "s").exec(goSource);
  if (!declaration) throw new Error(`${name} not found in operating_rules.go`);
  return [...declaration[1]!.matchAll(/"([a-z_]+)"/g)].map((match) => match[1]!);
}

describe("the controlled sets match the Go source", () => {
  it("has ip-profile's eight channels, not review-delivery's four", () => {
    expect([...RULE_PLATFORMS]).toEqual(goSlice("Platforms"));
    expect(RULE_PLATFORMS).toHaveLength(8);
  });

  it("has exactly one review rule", () => {
    expect([...REVIEW_RULES]).toEqual(goSlice("ReviewRules"));
    expect(REVIEW_RULES).toHaveLength(1);
  });

  it("has no escape hatch in either set", () => {
    for (const forbidden of ["other", "misc", "custom", "team"]) {
      expect(RULE_PLATFORMS as readonly string[]).not.toContain(forbidden);
      expect(REVIEW_RULES as readonly string[]).not.toContain(forbidden);
    }
  });
});

describe("defaults", () => {
  it("fills the review rule and nothing else", () => {
    const rules = emptyOperatingRules();
    expect(rules.reviewRule).toBe("self");
    // No invented cadence and no invented window (FR-024).
    expect(rules.cadence).toEqual({});
    expect(rules.observation.default).toBeUndefined();
  });
});

describe("unset is not zero", () => {
  // The single silent-corruption risk on this card. A page that reads the
  // number and ignores `stored` cannot tell "nothing goes out on this channel"
  // from "nobody has decided", and those are different pages.
  it("tells a stored 0 apart from an absent channel", () => {
    const rules = emptyOperatingRules();
    rules.cadence.wechat_mp = 0;

    expect(readCadence(rules, "wechat_mp")).toEqual({ value: 0, stored: true });
    expect(readCadence(rules, "douyin")).toEqual({ value: 0, stored: false });
    expect(readCadence(rules, "wechat_mp")).not.toEqual(readCadence(rules, "douyin"));
  });

  it("does not turn a missing window into zero days", () => {
    const rules = emptyOperatingRules();
    expect(readObservation(rules, "xiaohongshu").source).toBe("none");

    rules.observation.default = 0;
    // A stored 0 IS a window: look the same day.
    expect(readObservation(rules, "xiaohongshu")).toEqual({ days: 0, source: "global" });
  });
});

describe("readObservation", () => {
  it("says where the window came from", () => {
    const rules = emptyOperatingRules();
    rules.observation.default = 14;
    rules.observation.byChannel.douyin = 7;

    expect(readObservation(rules, "douyin")).toEqual({ days: 7, source: "channel" });
    expect(readObservation(rules, "xiaohongshu")).toEqual({ days: 14, source: "global" });
  });
});

describe("parseOperatingRules", () => {
  it("reads what the server sends", () => {
    const rules = parseOperatingRules({
      cadence: { xiaohongshu: 3, wechat_mp: 0 },
      templates: { xiaohongshu: { note: "标题 20 字内" } },
      review_rule: "self",
      observation: { default: 14, by_channel: { douyin: 7 } },
    });
    expect(readCadence(rules, "wechat_mp")).toEqual({ value: 0, stored: true });
    expect(readCadence(rules, "douyin").stored).toBe(false);
    expect(templateNoteFor(rules, "xiaohongshu")).toBe("标题 20 字内");
    expect(readObservation(rules, "douyin")).toEqual({ days: 7, source: "channel" });
  });

  it("degrades a malformed response to defaults without inventing a window", () => {
    // Each of these is reachable from a real response: an older backend, a
    // null column, a field this build has not heard of.
    for (const wire of [null, undefined, "nope", 7, {}, { observation: null }, { cadence: null }]) {
      const rules = parseOperatingRules(wire);
      expect(rules.reviewRule).toBe("self");
      expect(rules.observation.default).toBeUndefined();
      expect(readCadence(rules, "xiaohongshu").stored).toBe(false);
    }
  });

  it("keeps a channel this build has not heard of rather than dropping it", () => {
    const rules = parseOperatingRules({ cadence: { newplatform: 2 }, observation: {} });
    expect(readCadence(rules, "newplatform")).toEqual({ value: 2, stored: true });
  });

  it("never turns an absent window into 0", () => {
    const rules = parseOperatingRules({ observation: { default: null } });
    expect(rules.observation.default).toBeUndefined();
    expect(rules.observation.default).not.toBe(0);
  });
});

describe("operatingRulesToRequest", () => {
  it("omits the window rather than sending null", () => {
    const body = operatingRulesToRequest(emptyOperatingRules());
    expect(body.observation).toEqual({ by_channel: {} });
    expect(JSON.stringify(body)).not.toContain("null");
  });

  it("sends a stored zero", () => {
    const rules = emptyOperatingRules();
    rules.cadence.wechat_mp = 0;
    rules.observation.default = 0;
    const body = operatingRulesToRequest(rules) as {
      cadence: Record<string, number>;
      observation: { default?: number };
    };
    expect(body.cadence.wechat_mp).toBe(0);
    expect(body.observation.default).toBe(0);
  });
});

describe("templateNoteFor", () => {
  // SOP 3.2: "模型上下文仅接收必要的渠道说明". Only the note, and a channel
  // with no template yields "" rather than undefined for a caller to handle.
  it("returns the note alone", () => {
    const rules = emptyOperatingRules();
    rules.cadence.xiaohongshu = 3;
    rules.templates.xiaohongshu = { note: "标题 20 字内" };
    rules.observation.default = 14;

    expect(templateNoteFor(rules, "xiaohongshu")).toBe("标题 20 字内");
    expect(templateNoteFor(rules, "zhihu")).toBe("");
  });
});

describe("isDeliverable", () => {
  // Eight channels can hold a template; four can be delivered to. Not a bug -
  // FR-012a - but the page has to say it, so there is a function to ask.
  it("separates the four from the eight", () => {
    expect(DELIVERABLE_PLATFORMS).toHaveLength(4);
    expect(isDeliverable("xiaohongshu")).toBe(true);
    expect(isDeliverable("zhihu")).toBe(false);
    for (const platform of DELIVERABLE_PLATFORMS) {
      expect(RULE_PLATFORMS as readonly string[]).toContain(platform);
    }
  });
});

describe("homepage", () => {
  it("accepts a web link", () => {
    expect(isWebLink("https://www.xiaohongshu.com/user/profile/x")).toBe(true);
    expect(isWebLink("http://example.com/me")).toBe(true);
  });

  it("refuses anything that is not a page a person can open", () => {
    for (const link of ["", "javascript:alert(1)", "file:///etc/passwd", "data:text/html,x", "www.example.com"]) {
      expect(isWebLink(link)).toBe(false);
    }
  });

  it("tells a cleared link apart from one nobody entered", () => {
    expect(readHomepage({ "loretide.homepage": "" })).toEqual({ link: "", stored: true });
    expect(readHomepage({})).toEqual({ link: "", stored: false });
    expect(readHomepage(null)).toEqual({ link: "", stored: false });
    expect(readHomepage({ "loretide.homepage": 7 })).toEqual({ link: "", stored: false });
  });

  it("leaves the account's other settings out of its business", () => {
    // LT-014's scope lives on the same blob. Reading a homepage must not need
    // to know that, and must not disturb it.
    const settings = { "loretide.scope": "all", "loretide.homepage": "https://example.com" };
    expect(readHomepage(settings).link).toBe("https://example.com");
    expect(settings["loretide.scope"]).toBe("all");
  });
});
