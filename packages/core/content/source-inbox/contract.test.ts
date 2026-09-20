// @vitest-environment node
//
// Canonical home for the inbox's wire parsing. The component suite, when it
// lands, keeps the happy path and the wiring; the degradation matrix is here.

import { describe, expect, it } from "vitest";
import {
  isKnownKind,
  isKnownStatus,
  parseBulkResults,
  parseCreatedSource,
  parseDuplicates,
  parseSourceDetail,
  parseSourceRevisions,
  parseSources,
  SOURCE_KINDS,
  SOURCE_STATUSES,
} from "./contract";

const wireSource = {
  source_id: "src-1",
  workspace_id: "ws-1",
  kind: "pasted_text",
  url: "",
  captured_at: "2026-09-20T00:00:00Z",
  recorded_by: "user-1",
  historical_import: false,
  title: "a note",
  tags: ["x"],
  annotation: "where it came from",
  personal_judgement: "why I kept it",
  status: "inbox",
  updated_at: "2026-09-20T00:00:00Z",
};

describe("parseSources", () => {
  it("maps the wire shape to camelCase", () => {
    const [source] = parseSources({ sources: [wireSource] });
    expect(source).toMatchObject({
      sourceId: "src-1",
      historicalImport: false,
      personalJudgement: "why I kept it",
      tags: ["x"],
    });
  });

  // A null collection must not reach a caller: every reader would then have to
  // know which of the two shapes it received before mapping over it.
  it("normalises a null tag list to an empty one", () => {
    const [source] = parseSources({ sources: [{ ...wireSource, tags: null }] });
    expect(source?.tags).toEqual([]);
  });

  it("degrades a malformed body to an empty list rather than throwing", () => {
    expect(parseSources({ sources: "not an array" })).toEqual([]);
    expect(parseSources(null)).toEqual([]);
    expect(parseSources({})).toEqual([]);
  });

  // An older build receiving a value a newer server introduced still shows the
  // item: the note and its annotation are worth reading whatever the label is.
  it("keeps a row whose kind or status this build does not know", () => {
    const [source] = parseSources({
      sources: [{ ...wireSource, kind: "audio", status: "snoozed" }],
    });
    expect(source?.sourceId).toBe("src-1");
    expect(source?.kind).toBe("audio");
    expect(isKnownKind("audio")).toBe(false);
    expect(isKnownStatus("snoozed")).toBe(false);
  });
});

describe("parseSourceDetail", () => {
  it("returns a null snapshot for a url source", () => {
    const detail = parseSourceDetail({
      source: { ...wireSource, kind: "url", url: "https://example.com/a" },
      snapshot: null,
    });
    expect(detail.snapshot).toBeNull();
    expect(detail.source.url).toBe("https://example.com/a");
  });

  it("returns the snapshot when there is one", () => {
    const detail = parseSourceDetail({
      source: wireSource,
      snapshot: {
        snapshot_id: "snap-1",
        workspace_id: "ws-1",
        source_id: "src-1",
        content: "body",
        content_hash: "abc",
        captured_at: "2026-09-20T00:00:00Z",
      },
    });
    expect(detail.snapshot).toMatchObject({ snapshotId: "snap-1", contentHash: "abc" });
  });

  it("degrades a malformed body to an empty source", () => {
    expect(parseSourceDetail({ source: 42 }).source.sourceId).toBe("");
    expect(parseSourceDetail(undefined).snapshot).toBeNull();
  });
});

describe("parseCreatedSource", () => {
  it("carries the duplicate hint", () => {
    const created = parseCreatedSource({
      source: wireSource,
      snapshot: null,
      duplicates: ["src-0"],
    });
    expect(created.duplicates).toEqual(["src-0"]);
  });

  // The hint is a hint. A missing or null list is "none", never a failure.
  it("treats a missing duplicate list as none", () => {
    expect(parseCreatedSource({ source: wireSource }).duplicates).toEqual([]);
    expect(parseCreatedSource({ source: wireSource, duplicates: null }).duplicates).toEqual([]);
  });
});

describe("parseSourceRevisions", () => {
  it("normalises a null field list", () => {
    const [revision] = parseSourceRevisions({
      revisions: [
        {
          revision_id: "rev-1",
          workspace_id: "ws-1",
          source_id: "src-1",
          changed_fields: null,
          actor_id: "user-1",
          created_at: "2026-09-20T00:00:00Z",
        },
      ],
    });
    expect(revision?.changedFields).toEqual([]);
  });

  it("degrades a malformed body", () => {
    expect(parseSourceRevisions({ revisions: null })).toEqual([]);
  });
});

describe("parseDuplicates and parseBulkResults", () => {
  it("reads the duplicate ids", () => {
    expect(parseDuplicates({ source_ids: ["a", "b"] })).toEqual(["a", "b"]);
    expect(parseDuplicates({ source_ids: null })).toEqual([]);
    expect(parseDuplicates("nonsense")).toEqual([]);
  });

  // Per item, including the failures: a caller has to be able to say which
  // ones did not land.
  it("keeps each item's own outcome", () => {
    const results = parseBulkResults({
      results: [
        { source_id: "a", ok: true },
        { source_id: "b", ok: false, reason: "not_found" },
      ],
    });
    expect(results).toEqual([
      { sourceId: "a", ok: true, reason: "" },
      { sourceId: "b", ok: false, reason: "not_found" },
    ]);
  });

  // An explicit boolean check, not a truthy one: a server field that arrived
  // as the string "false" must not read as success.
  it("treats a non-boolean ok as not ok", () => {
    const results = parseBulkResults({ results: [{ source_id: "a", ok: "false" }] });
    expect(results).toEqual([]);
  });
});

describe("the controlled sets", () => {
  it("are the two the card defines", () => {
    expect(SOURCE_KINDS).toEqual(["pasted_text", "url"]);
    expect(SOURCE_STATUSES).toEqual(["inbox", "organized", "archived"]);
  });
});
