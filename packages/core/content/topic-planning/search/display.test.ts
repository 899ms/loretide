import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  authorKindDisplay, searchDataOriginDisplay, searchIntentDisplay, searchOriginDisplay,
  suggestionAspectDisplay, suggestionFailureDisplay, suggestionStateDisplay, themeUnknownReasonDisplay,
} from "./display";

describe("search optimization display values", () => {
  it.each(["learn", "solve", "compare", "buy", "find", "unclassified"])("keeps intent %s", (value) => {
    expect(searchIntentDisplay(value)).toBe(value);
  });
  it.each(["manual_keyword", "customer_question", "authorized_material"])("keeps origin %s", (value) => {
    expect(searchOriginDisplay(value)).toBe(value);
  });
  it.each(["open", "adopted", "adopt_failed", "adopt_unrecorded", "abandoned"])("keeps state %s", (value) => {
    expect(suggestionStateDisplay(value)).toBe(value);
  });
  it.each(["base_moved", "draft_unsaved", "no_change", "target_not_found", "storage"])("keeps failure %s", (value) => {
    expect(suggestionFailureDisplay(value)).toBe(value);
  });
  it.each(["title", "body", "topics", "description"])("keeps aspect %s", (value) => {
    expect(suggestionAspectDisplay(value)).toBe(value);
  });
  it("keeps origins, author kinds and unknown reason safe", () => {
    expect(authorKindDisplay("human")).toBe("human");
    expect(authorKindDisplay("future")).toBe("unknown");
    expect(searchDataOriginDisplay("manual_only")).toBe("manual_only");
    expect(searchDataOriginDisplay("future")).toBe("unknown");
    expect(searchIntentDisplay("future-intent")).toBe("unknown");
    expect(searchOriginDisplay("future-origin")).toBe("unknown");
    expect(suggestionStateDisplay("future-state")).toBe("unknown");
    expect(suggestionFailureDisplay("future-failure")).toBe("unknown");
    expect(suggestionAspectDisplay("future-aspect")).toBe("unknown");
    expect(themeUnknownReasonDisplay({ searchVolume: { status: "unknown", reason: "no_data_source" }, competition: { status: "unknown", reason: "no_data_source" } } as never)).toBe("no_data_source");
    expect(themeUnknownReasonDisplay({ searchVolume: { status: "unknown", reason: "unknown" }, competition: { status: "unknown", reason: "no_data_source" } } as never)).toBe("unknown");
  });
});

describe("search optimization locale copy", () => {
  const baseDir = resolve(dirname(fileURLToPath(import.meta.url)), "../../../../../packages/views/locales");
  const locales = ["en", "zh-Hans", "ja", "ko"];
  const riskyPromises = /保证排名|上首页|提升排名|排名第一|\bguarantee(?:d)?\b|\bboost ranking\b|\btop rank\b/i;

  function strings(value: unknown): string[] {
    if (typeof value === "string") return [value];
    if (Array.isArray(value)) return value.flatMap(strings);
    if (value && typeof value === "object") return Object.values(value).flatMap(strings);
    return [];
  }

  it.each(locales)("contains no ranking promises outside the approved disclaimer (%s)", (locale) => {
    const bundle = JSON.parse(readFileSync(resolve(baseDir, locale, "common.json"), "utf8")) as Record<string, unknown>;
    const namespace = bundle.search_optimization as Record<string, unknown>;
    const disclaimer = namespace["optimization.no_ranking_promise"];
    expect(typeof disclaimer).toBe("string");
    const copy = { ...namespace };
    delete copy["optimization.no_ranking_promise"];
    expect(strings(copy).filter((value) => riskyPromises.test(value))).toEqual([]);
    expect((disclaimer as string).trim()).not.toBe("");
  });
});
