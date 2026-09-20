// @vitest-environment node
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

import {
  ARTIFACT_KINDS,
  DRAFT_STATUSES,
  VERSION_ACTIONS,
  VERSION_SOURCES,
  describeVersion,
  parseArtifact,
  parseArtifactVersion,
  parseArtifactVersions,
  parseWork,
  parseWorks,
} from "./contract";

// The controlled sets are the server's. Restating them here is unavoidable - a
// component has to switch on them - so they are compared against the Go source
// rather than against my memory of it, the same way platforms.ts and scope.ts
// are.
const goSource = readFileSync(
  join(__dirname, "../../../../server/internal/content/work-editor/contract.go"),
  "utf8",
);

function goSetValues(varName: string): string[] {
  const declaration = new RegExp(`var ${varName} = \\[\\][A-Za-z]+\\{([^}]*)\\}`).exec(goSource);
  if (!declaration) throw new Error(`${varName} not found in contract.go`);
  const constants = declaration[1]!.split(",").map((name) => name.trim()).filter(Boolean);
  return constants.map((name) => {
    const value = new RegExp(`${name}\\s+[A-Za-z]+ = "([^"]+)"`).exec(goSource);
    if (!value) throw new Error(`${name} has no literal in contract.go`);
    return value[1]!;
  });
}

describe("the controlled sets match the Go source", () => {
  it.each([
    ["Kinds", ARTIFACT_KINDS],
    ["DraftStatuses", DRAFT_STATUSES],
    ["Sources", VERSION_SOURCES],
    ["Actions", VERSION_ACTIONS],
  ])("%s", (goName, tsValues) => {
    expect([...tsValues]).toEqual(goSetValues(goName));
  });

  it("does not carry restored as a version source", () => {
    // Restoring is an action. Its content is still what a person wrote, so the
    // source stays edited.
    expect(VERSION_SOURCES).not.toContain("restored");
    expect(VERSION_ACTIONS).toContain("restored");
  });

  it("does not carry imported as a version source either", () => {
    // Same shape of claim for SOP 3.3's historical import, and the same
    // reason: a body pasted back in from a platform is still something a
    // person wrote. The source set stays at exactly SOP 7.1's three.
    expect(VERSION_SOURCES).not.toContain("imported");
    expect(VERSION_ACTIONS).toContain("imported");
    expect(VERSION_SOURCES).toHaveLength(3);
  });
});

const workWire = {
  work_id: "work-1",
  workspace_id: "ws-1",
  topic_card_id: "card-1",
  snapshot_id: "",
  title: "手写稿",
  created_at: "2026-09-21T00:00:00Z",
  updated_at: "2026-09-21T00:00:00Z",
};

describe("parseWork", () => {
  it("reads an empty snapshot id as a work that was never started, not as missing", () => {
    expect(parseWork(workWire).snapshotId).toBe("");
    expect(parseWork(workWire).workId).toBe("work-1");
  });

  it("degrades a malformed response instead of throwing", () => {
    expect(parseWork({ nonsense: true }).workId).toBe("");
    expect(parseWorks({ works: null })).toEqual([]);
    expect(parseWorks("not an object")).toEqual([]);
  });

  it("reads an empty topic card id as a work that belongs to no card", () => {
    // SOP 3.3. Before the historical import this could not happen, so the
    // field being empty is new information rather than a missing value.
    const imported = parseWork({ ...workWire, topic_card_id: "", historical_import: true });
    expect(imported.topicCardId).toBe("");
    expect(imported.historicalImport).toBe(true);
  });

  it("treats an absent historical_import as not an import", () => {
    // A backend deployed without migration 532 sends nothing. Absent must
    // read as false, and a non-boolean must not be coerced into true: a work
    // wrongly labelled historical would be filtered out of the by-card lists
    // it belongs in.
    expect(parseWork(workWire).historicalImport).toBe(false);
    expect(parseWork({ ...workWire, historical_import: "yes" }).historicalImport).toBe(false);
    expect(parseWork({ ...workWire, historical_import: 1 }).historicalImport).toBe(false);
  });
});

describe("parseArtifact", () => {
  it("keeps the editing copy and its status", () => {
    const artifact = parseArtifact({
      artifact_id: "art-1", work_id: "work-1", kind: "body",
      title: "正文", position: 1, draft_body: "写了一点", draft_status: "working",
    });
    expect(artifact.draftBody).toBe("写了一点");
    expect(artifact.draftStatus).toBe("working");
  });

  it("degrades an unrecognised draft status to working, not to saved", () => {
    // Claiming everything is saved when it might not be is the direction that
    // loses work.
    expect(parseArtifact({ artifact_id: "a", draft_status: "finished" }).draftStatus).toBe("working");
    expect(parseArtifact({ artifact_id: "a" }).draftStatus).toBe("working");
  });

  it("keeps a kind this build has not heard of rather than blanking the row", () => {
    // What the page may WRITE is the controlled set; what it may READ is wider.
    expect(parseArtifact({ artifact_id: "a", kind: "newsletter" }).kind).toBe("newsletter");
  });
});

describe("parseArtifactVersion", () => {
  it("keeps source and action apart", () => {
    const version = parseArtifactVersion({
      version_id: "v-2", revision: 2, source: "edited", action: "restored",
      body: "旧文", restored_from: "v-1",
    });
    expect(version.source).toBe("edited");
    expect(version.action).toBe("restored");
    expect(version.restoredFrom).toBe("v-1");
    expect(version.adoptedFrom).toBe("");
  });

  it("reads an empty history as [] rather than an error", () => {
    expect(parseArtifactVersions({ versions: null })).toEqual([]);
    expect(parseArtifactVersions({})).toEqual([]);
  });
});

describe("describeVersion", () => {
  it("returns both facts and the id the version came from", () => {
    expect(
      describeVersion(parseArtifactVersion({
        version_id: "v-3", source: "adopted", action: "adopted", adopted_from: "v-2",
      })),
    ).toEqual({ source: "adopted", action: "adopted", from: "v-2" });

    expect(
      describeVersion(parseArtifactVersion({
        version_id: "v-1", source: "edited", action: "saved",
      })),
    ).toEqual({ source: "edited", action: "saved", from: "" });
  });
});
