// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  SEARCH_DATA_ORIGINS,
  SEARCH_INTENTS,
  SEARCH_UNKNOWN_REASONS,
  THEME_ORIGINS,
  parseSearchTheme,
  parseSearchThemeList,
  parseSearchThemeRevisions,
  searchPath,
} from "./contract";

// specs/036 PR 1 (T027). The sets are held to the Go source, and every
// schema has a malformed-response case.

const goDir = join(__dirname, "../../../../../server/internal/content/topic-planning");
const goContract = readFileSync(join(goDir, "search_contract.go"), "utf8");

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`, "s").exec(goContract);
  if (!declaration) throw new Error(`${varName} not found in search_contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goContract);
    if (!value) throw new Error(`${name} has no literal`);
    return value[1]!;
  });
}

describe("controlled sets", () => {
  it("are the Go sets, in order", () => {
    expect([...SEARCH_INTENTS]).toEqual(goSetValues("SearchIntents"));
    expect([...THEME_ORIGINS]).toEqual(goSetValues("ThemeOrigins"));
    expect([...SEARCH_DATA_ORIGINS]).toEqual(goSetValues("DataOrigins"));
    expect([...SEARCH_UNKNOWN_REASONS]).toEqual(goSetValues("UnknownReasons"));
  });

  it("encodes each path segment", () => {
    expect(searchPath("themes", "a/b", "revisions")).toBe("themes/a%2Fb/revisions");
  });
});

const unknown = { status: "unknown", reason: "no_data_source" };

const themeWire = {
  theme_id: "t1", revision: 2, voided: false, name: "羊绒大衣怎么洗", platform: "xiaohongshu",
  account_id: "a1", business_goal: "门店到店预约", questions: ["羊绒大衣能机洗吗"], keywords: ["羊绒"],
  intent: "solve", origin: "customer_question", origin_note: "9 月私信", source_ids: ["s1"],
  topic_card_ids: ["c1"], brief_revision_ids: ["b1"], note: "", recorded_by: "u1",
  created_at: "2026-09-25T01:00:00Z", search_volume: unknown, competition: unknown, data_origin: "manual_only",
};

describe("parseSearchTheme", () => {
  it("reads a theme", () => {
    const theme = parseSearchTheme(themeWire);
    expect(theme).toMatchObject({
      themeId: "t1", revision: 2, voided: false, accountId: "a1", intent: "solve", origin: "customer_question",
      keywords: ["羊绒"], topicCardIds: ["c1"], briefRevisionIds: ["b1"], dataOrigin: "manual_only",
      searchVolume: { status: "unknown", reason: "no_data_source" },
      competition: { status: "unknown", reason: "no_data_source" },
    });
  });

  it("never shows a number where search volume or competition is unknown", () => {
    const theme = parseSearchTheme({ ...themeWire, search_volume: 12000, competition: { status: "low", reason: "x" } });
    expect(theme?.searchVolume).toEqual({ status: "unknown", reason: "unknown" });
    expect(theme?.competition).toEqual({ status: "unknown", reason: "unknown" });
  });

  it("reads a value outside a set as unknown", () => {
    const theme = parseSearchTheme({ ...themeWire, intent: "navigational", origin: "online_research", data_origin: "ai" });
    expect(theme?.intent).toBe("unknown");
    expect(theme?.origin).toBe("unknown");
    expect(theme?.dataOrigin).toBe("unknown");
  });

  it("returns null for a malformed response", () => {
    expect(parseSearchTheme({ ...themeWire, theme_id: 1 })).toBeNull();
    expect(parseSearchTheme({ ...themeWire, revision: "2" })).toBeNull();
    expect(parseSearchTheme("not json")).toBeNull();
  });
});

describe("parseSearchThemeList", () => {
  it("reads the list and treats null lists as empty", () => {
    const [theme] = parseSearchThemeList({ themes: [{ ...themeWire, questions: null, source_ids: null }] });
    expect(theme?.questions).toEqual([]);
    expect(theme?.sourceIds).toEqual([]);
  });

  it("returns [] for a malformed response", () => {
    expect(parseSearchThemeList({ themes: "x" })).toEqual([]);
    expect(parseSearchThemeList(null)).toEqual([]);
  });
});

describe("parseSearchThemeRevisions", () => {
  it("reads every revision", () => {
    const parsed = parseSearchThemeRevisions({ theme_id: "t1", revisions: [themeWire, { ...themeWire, revision: 1 }] });
    expect(parsed?.themeId).toBe("t1");
    expect(parsed?.revisions.map((theme) => theme.revision)).toEqual([2, 1]);
  });

  it("returns null for a malformed response", () => {
    expect(parseSearchThemeRevisions({ revisions: [] })).toBeNull();
  });
});
