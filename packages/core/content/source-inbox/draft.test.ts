// @vitest-environment node
//
// Canonical home for the inbox page's rules. The page renders what these
// return and holds no judgement of its own.

import { describe, expect, it } from "vitest";
import {
  bulkSummary,
  canSubmitBulk,
  canSubmitDraft,
  draftProblem,
  draftToRequest,
  duplicateHint,
  emptyDraft,
  isWebURL,
  organizeActions,
  parseTagInput,
  type SourceDraft,
} from "./draft";

function draft(overrides: Partial<SourceDraft> = {}): SourceDraft {
  return { ...emptyDraft(), ...overrides };
}

describe("draftProblem", () => {
  it("accepts a pasted text with a body", () => {
    expect(draftProblem(draft({ content: "a note" }))).toBe("");
  });

  it("accepts a url with a link", () => {
    expect(draftProblem(draft({ kind: "url", url: "https://example.com/a" }))).toBe("");
  });

  it.each([
    ["pasted text with no body", draft({ content: "   " }), "content"],
    ["pasted text carrying a link", draft({ content: "x", url: "https://example.com" }), "url"],
    ["url with no link", draft({ kind: "url" }), "url"],
    ["url that is not a url", draft({ kind: "url", url: "not a url" }), "url"],
    // A url source has no body: nothing read the page, so there is no text the
    // link vouches for.
    ["url carrying a body", draft({ kind: "url", url: "https://example.com", content: "x" }), "content"],
    ["an unknown kind", draft({ kind: "file", content: "x" }), "kind"],
  ])("names the field for %s", (_name, input, field) => {
    expect(draftProblem(input)).toBe(field);
  });
});

describe("canSubmitDraft", () => {
  it("is false while a save is in flight, even on a good draft", () => {
    expect(canSubmitDraft(draft({ content: "a note" }), true)).toBe(false);
    expect(canSubmitDraft(draft({ content: "a note" }), false)).toBe(true);
  });
});

describe("isWebURL", () => {
  it.each(["https://example.com/a", "http://example.com"])("accepts %s", (value) => {
    expect(isWebURL(value)).toBe(true);
  });

  // Not a source a person can go back and read, and storing one invites a page
  // to follow it.
  it.each(["javascript:alert(1)", "file:///etc/passwd", "data:text/plain,x", "", "   ", "example.com"])(
    "rejects %s",
    (value) => {
      expect(isWebURL(value)).toBe(false);
    },
  );
});

describe("parseTagInput", () => {
  it("splits on commas, full-width commas and whitespace", () => {
    expect(parseTagInput("a, b，c  d")).toEqual(["a", "b", "c", "d"]);
  });

  it("drops blanks and repeats rather than rejecting them", () => {
    expect(parseTagInput(" , a ,, a ")).toEqual(["a"]);
    expect(parseTagInput("   ")).toEqual([]);
  });
});

describe("draftToRequest", () => {
  it("sends a body for a pasted text and no url", () => {
    const body = draftToRequest(draft({ content: "a note", tags: "x, y" }));
    expect(body).toMatchObject({ kind: "pasted_text", content: "a note", tags: ["x", "y"] });
    expect(body.url).toBeUndefined();
  });

  it("sends a url and no body for a link", () => {
    const body = draftToRequest(draft({ kind: "url", url: "https://example.com/a" }));
    expect(body).toMatchObject({ kind: "url", url: "https://example.com/a" });
    expect(body.content).toBeUndefined();
  });

  it("carries the historical import marker", () => {
    expect(draftToRequest(draft({ content: "x", historicalImport: true })).historical_import).toBe(true);
  });
});

describe("duplicateHint", () => {
  // The whole point. §4 asks the system to point out the repeat and stop;
  // R-011 says both collections keep their own annotation.
  it("never blocks, whatever it found", () => {
    expect(duplicateHint([]).blocking).toBe(false);
    expect(duplicateHint(["a", "b"]).blocking).toBe(false);
  });

  it("counts what it found", () => {
    expect(duplicateHint(["a", "b"])).toEqual({ count: 2, sourceIds: ["a", "b"], blocking: false });
  });
});

describe("organizeActions", () => {
  it("never offers the status it already has", () => {
    expect(organizeActions("inbox")).toEqual(["organized", "archived"]);
    expect(organizeActions("archived")).toEqual(["inbox", "organized"]);
  });

  // Archiving is a judgement, and judgements can be revisited. A one-way
  // pipeline would make "I was wrong about this" impossible without a delete,
  // and there is no delete.
  it("offers a way back from archived", () => {
    expect(organizeActions("archived")).toContain("inbox");
  });

  it("offers every status for a value this build does not know", () => {
    expect(organizeActions("snoozed")).toEqual(["inbox", "organized", "archived"]);
  });
});

describe("bulkSummary", () => {
  it("separates what landed from what did not", () => {
    const summary = bulkSummary([
      { sourceId: "a", ok: true, reason: "" },
      { sourceId: "b", ok: false, reason: "not_found" },
      { sourceId: "c", ok: true, reason: "" },
    ]);
    expect(summary).toEqual({ ok: 2, failed: 1, failedIds: ["b"] });
  });

  it("reports an empty batch as nothing rather than as success", () => {
    expect(bulkSummary([])).toEqual({ ok: 0, failed: 0, failedIds: [] });
  });
});

describe("canSubmitBulk", () => {
  it("needs both a selection and something to apply", () => {
    expect(canSubmitBulk([], ["x"], "")).toBe(false);
    expect(canSubmitBulk(["a"], [], "")).toBe(false);
    expect(canSubmitBulk(["a"], ["x"], "")).toBe(true);
    expect(canSubmitBulk(["a"], [], "archived")).toBe(true);
  });
});
