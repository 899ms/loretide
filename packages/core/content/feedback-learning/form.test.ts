// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  MAX_NOTE_RUNES,
  MAX_SHORT_RUNES,
  MAX_TAGS,
  canSubmitExcerptDraft,
  canSubmitMetricDraft,
  countRunes,
  csvPreview,
  emptyExcerptDraft,
  emptyMetricDraft,
  excerptDraftProblem,
  excerptDraftToInput,
  isTimestamp,
  metricDraftProblem,
  metricDraftToInput,
  parseTagInput,
  parseValueInput,
  type ExcerptDraft,
  type MetricDraft,
} from "./form";

// What the page may submit, decided here rather than in JSX.
//
// The point of most of these is that the button is dead BEFORE the request:
// letting somebody press submit on a row the server will refuse teaches them
// nothing except that the product is unreliable.

const RECORD = "pub-1";

function metricDraft(overrides: Partial<MetricDraft> = {}): MetricDraft {
  return {
    ...emptyMetricDraft(RECORD),
    platform: "xiaohongshu",
    metric: "read",
    sampledAt: "2026-09-20T10:00:00Z",
    ...overrides,
  };
}

function excerptDraft(overrides: Partial<ExcerptDraft> = {}): ExcerptDraft {
  return {
    ...emptyExcerptDraft(RECORD),
    sourceType: "comment",
    redactedExcerpt: "看完就去买了",
    occurredAt: "2026-09-20T10:00:00Z",
    ...overrides,
  };
}

describe("the limits match the Go source", () => {
  // Three numbers restated on this side of the wire. Compared against the Go
  // rather than against my memory of it: a form that allows 21000 runes where
  // the server allows 20000 turns a typed paragraph into a 400.
  const goSource = readFileSync(
    join(__dirname, "../../../../server/internal/content/feedback-learning/contract.go"),
    "utf8",
  );

  function goConst(name: string): number {
    const found = new RegExp(`const ${name} = (\\d+)`).exec(goSource);
    if (!found) throw new Error(`${name} not found in contract.go`);
    return Number(found[1]);
  }

  it.each([
    ["MaxNoteRunes", MAX_NOTE_RUNES],
    ["MaxShortRunes", MAX_SHORT_RUNES],
    ["MaxTags", MAX_TAGS],
  ])("%s", (goName, tsValue) => {
    expect(tsValue).toBe(goConst(goName));
  });
});

describe("countRunes", () => {
  // FR-018: runes, not bytes. Counting bytes would give Chinese a third of the
  // room English gets under the same limit.
  it("counts a Chinese character as one", () => {
    expect(countRunes("看完就去买了")).toBe(6);
  });

  it("counts an astral character as one, unlike String.length", () => {
    expect("👍".length).toBe(2);
    expect(countRunes("👍")).toBe(1);
  });
});

describe("parseValueInput - blank is not zero", () => {
  // The single silent-corruption risk on this card. `Number("")` is 0 in
  // JavaScript, so a blank cell reaching a naive parse becomes a measurement
  // nobody made.
  it("reads a blank as null", () => {
    expect(parseValueInput("")).toEqual({ ok: true, value: null });
    expect(parseValueInput("   ")).toEqual({ ok: true, value: null });
  });

  it("reads a typed zero as zero", () => {
    expect(parseValueInput("0")).toEqual({ ok: true, value: 0 });
  });

  it("keeps the two apart", () => {
    const blank = parseValueInput("");
    const zero = parseValueInput("0");
    expect(blank).not.toEqual(zero);
  });

  it("accepts a thousands separator", () => {
    expect(parseValueInput("12,400")).toEqual({ ok: true, value: 12400 });
  });

  it("refuses something that is not a number", () => {
    expect(parseValueInput("many")).toEqual({ ok: false });
  });
});

describe("isTimestamp", () => {
  it("accepts what the server accepts", () => {
    expect(isTimestamp("2026-09-20T10:00:00Z")).toBe(true);
    expect(isTimestamp("2026-09-20T10:00:00+08:00")).toBe(true);
    expect(isTimestamp("2026-09-20T10:00:00.5Z")).toBe(true);
  });

  // Date.parse takes all three. time.Parse(time.RFC3339, ...) takes none, so
  // accepting them here would only move the refusal to after the click.
  it("refuses what Date.parse would have let through", () => {
    expect(isTimestamp("2026-09-20")).toBe(false);
    expect(isTimestamp("September 20 2026")).toBe(false);
    expect(isTimestamp("2026-09-20 10:00:00")).toBe(false);
  });

  it("refuses a well-shaped impossible date", () => {
    expect(isTimestamp("2026-13-45T10:00:00Z")).toBe(false);
  });
});

describe("metricDraftProblem", () => {
  it("passes a complete draft", () => {
    expect(metricDraftProblem(metricDraft())).toBeNull();
    expect(canSubmitMetricDraft(metricDraft())).toBe(true);
  });

  // A blank value is the ordinary case, not an incomplete form: the platform
  // does not show that number.
  it("passes with the value left blank", () => {
    expect(metricDraftProblem(metricDraft({ value: "" }))).toBeNull();
  });

  it.each([
    ["publication_record_id", { publicationRecordId: "" }, "missing"],
    ["platform", { platform: "" }, "missing"],
    ["platform", { platform: "twitter" }, "outside-the-set"],
    ["metric", { metric: "" }, "missing"],
    ["metric", { metric: "engagement" }, "outside-the-set"],
    ["sampled_at", { sampledAt: "" }, "missing"],
    ["sampled_at", { sampledAt: "2026-09-20" }, "not-a-timestamp"],
    ["value", { value: "many" }, "not-a-number"],
  ])("points at %s", (field, overrides, reason) => {
    expect(metricDraftProblem(metricDraft(overrides))).toEqual({ field, reason });
  });

  it("refuses a unit past the short limit", () => {
    const draft = metricDraft({ unit: "字".repeat(MAX_SHORT_RUNES + 1) });
    expect(metricDraftProblem(draft)).toEqual({ field: "unit", reason: "too-long" });
  });

  it("accepts a unit exactly at the limit", () => {
    expect(metricDraftProblem(metricDraft({ unit: "字".repeat(MAX_SHORT_RUNES) }))).toBeNull();
  });
});

describe("metricDraftToInput", () => {
  it("sends a blank value as null, never as 0", () => {
    const input = metricDraftToInput(metricDraft({ value: "" }));
    expect(input.value).toBeNull();
    expect(input.value).not.toBe(0);
  });

  it("sends a typed zero as 0", () => {
    expect(metricDraftToInput(metricDraft({ value: "0" })).value).toBe(0);
  });

  // FR-008 / SC-004: which endpoint is called is what the server records as
  // the origin. A caller that could declare its own origin is not reporting
  // one, so the draft has no field for it to declare.
  it("carries no source_type and no recorded_by", () => {
    const input = metricDraftToInput(metricDraft()) as unknown as Record<string, unknown>;
    expect(input).not.toHaveProperty("sourceType");
    expect(input).not.toHaveProperty("source_type");
    expect(input).not.toHaveProperty("recordedBy");
  });
});

describe("parseTagInput", () => {
  it("splits on both comma widths and trims", () => {
    expect(parseTagInput("转化, 第三段，对比")).toEqual(["转化", "第三段", "对比"]);
  });

  it("drops blanks and repeats but keeps the typed order", () => {
    expect(parseTagInput("b,,a, b ,c")).toEqual(["b", "a", "c"]);
  });

  it("reads an empty box as no tags", () => {
    expect(parseTagInput("   ")).toEqual([]);
  });
});

describe("excerptDraftProblem", () => {
  it("passes with only the quote", () => {
    expect(excerptDraftProblem(excerptDraft({ interpretation: "" }))).toBeNull();
  });

  // R-045 keeps the two apart, which means either one alone is a real row: an
  // observation worth writing down that quotes nobody is still an observation.
  it("passes with only the interpretation", () => {
    const draft = excerptDraft({ redactedExcerpt: "", interpretation: "转化点在第三段" });
    expect(excerptDraftProblem(draft)).toBeNull();
  });

  it("refuses a row that says nothing at all", () => {
    const draft = excerptDraft({ redactedExcerpt: "  ", interpretation: "" });
    expect(excerptDraftProblem(draft)).toEqual({ field: "redacted_excerpt", reason: "missing" });
    expect(canSubmitExcerptDraft(draft)).toBe(false);
  });

  it.each([
    ["source_type", { sourceType: "" }, "missing"],
    ["source_type", { sourceType: "review" }, "outside-the-set"],
    ["occurred_at", { occurredAt: "" }, "missing"],
    ["occurred_at", { occurredAt: "yesterday" }, "not-a-timestamp"],
  ])("points at %s", (field, overrides, reason) => {
    expect(excerptDraftProblem(excerptDraft(overrides))).toEqual({ field, reason });
  });

  it("refuses more tags than the server takes", () => {
    const tagInput = Array.from({ length: MAX_TAGS + 1 }, (_, index) => `t${index}`).join(",");
    expect(excerptDraftProblem(excerptDraft({ tagInput }))).toEqual({
      field: "tags",
      reason: "too-many",
    });
  });

  it("measures the quote in runes, not UTF-16 units", () => {
    // MAX_NOTE_RUNES astral characters are 2 * MAX_NOTE_RUNES in
    // String.length. A byte or unit count would refuse this; the server does
    // not, so neither does the form.
    expect(excerptDraftProblem(excerptDraft({ redactedExcerpt: "👍".repeat(MAX_NOTE_RUNES) }))).toBeNull();
    expect(excerptDraftProblem(excerptDraft({ redactedExcerpt: "字".repeat(MAX_NOTE_RUNES + 1) }))).toEqual({
      field: "redacted_excerpt",
      reason: "too-long",
    });
  });
});

describe("excerptDraftToInput", () => {
  it("keeps the quote and the reading in their own fields", () => {
    const input = excerptDraftToInput(
      excerptDraft({ redactedExcerpt: "看完就去买了", interpretation: "转化点在第三段" }),
    );
    expect(input.redactedExcerpt).toBe("看完就去买了");
    expect(input.interpretation).toBe("转化点在第三段");
  });

  it("splits the tag box", () => {
    expect(excerptDraftToInput(excerptDraft({ tagInput: "转化,对比" })).tags).toEqual([
      "转化",
      "对比",
    ]);
  });
});

describe("csvPreview", () => {
  const header = "platform,account_id,metric,value,unit,stat_window,sampled_at";

  it("reads an empty box as nothing yet, not as an error", () => {
    const preview = csvPreview("   ");
    expect(preview.empty).toBe(true);
    expect(preview.problem).toBeNull();
    expect(preview.canImport).toBe(false);
  });

  it("counts the blanks before the import, not after", () => {
    const preview = csvPreview(
      [
        header,
        "xiaohongshu,acc,read,1200,次,14 天,2026-09-20T10:00:00Z",
        "xiaohongshu,acc,conversion,,次,14 天,2026-09-20T10:00:00Z",
        "xiaohongshu,acc,like,0,次,14 天,2026-09-20T10:00:00Z",
      ].join("\n"),
    );
    expect(preview.rows).toHaveLength(3);
    // One unknown, not two: the row that says 0 was measured.
    expect(preview.unknown).toBe(1);
    expect(preview.rows[1]!.value).toBeNull();
    expect(preview.rows[2]!.value).toBe(0);
    expect(preview.canImport).toBe(true);
  });

  // FR-011a: the paste is all or nothing, and the refusal names the line. "One
  // of your forty rows is wrong" is not something anybody can act on.
  it("stops at the first bad row and names it", () => {
    const preview = csvPreview(
      [
        header,
        "xiaohongshu,acc,read,1200,次,14 天,2026-09-20T10:00:00Z",
        "xiaohongshu,acc,engagement,3,次,14 天,2026-09-20T10:00:00Z",
        "xiaohongshu,acc,like,5,次,14 天,2026-09-20T10:00:00Z",
      ].join("\n"),
    );
    expect(preview.problem).toEqual({ row: 2, column: "metric", reason: "outside-the-set" });
    // Nothing is offered for import, including the row that was fine.
    expect(preview.rows).toHaveLength(0);
    expect(preview.canImport).toBe(false);
  });
});
