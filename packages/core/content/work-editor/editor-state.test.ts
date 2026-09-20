// @vitest-environment node
import { describe, expect, it } from "vitest";

import type { Artifact, ArtifactVersion } from "./contract";
import {
  AI_ENTRY_POINTS,
  aiEntryPointEnabled,
  canSaveVersion,
  comparisonPair,
  defaultComparisonTarget,
  draftBadge,
  draftIsUnsent,
  nextArtifactPosition,
} from "./editor-state";

function artifact(over: Partial<Artifact> = {}): Artifact {
  return {
    artifactId: "art-1", workId: "work-1", workspaceId: "ws-1", kind: "body",
    title: "正文", position: 1, draftBody: "", draftStatus: "saved",
    draftSavedAt: "", createdAt: "", updatedAt: "",
    ...over,
  };
}

function version(revision: number, over: Partial<ArtifactVersion> = {}): ArtifactVersion {
  return {
    versionId: `v-${revision}`, artifactId: "art-1", workId: "work-1",
    workspaceId: "ws-1", revision, source: "edited", action: "saved",
    body: `第${revision}版`, restoredFrom: "", adoptedFrom: "", actorId: "u",
    createdAt: "", ...over,
  };
}

describe("draftIsUnsent", () => {
  it("compares against the server's copy of the draft, not its status", () => {
    // Between a keystroke and the next autosave the two say different things,
    // and the editor is the one that knows.
    const stored = artifact({ draftBody: "写了一点", draftStatus: "working" });
    expect(draftIsUnsent("写了一点", stored)).toBe(false);
    expect(draftIsUnsent("写了一点点", stored)).toBe(true);
  });

  it("has nothing to send when no document is open", () => {
    expect(draftIsUnsent("anything", undefined)).toBe(false);
  });
});

describe("draftBadge", () => {
  it.each([
    ["unsent keystrokes", "新的", artifact({ draftBody: "旧的", draftStatus: "saved" }), "working"],
    ["stored draft past the last version", "旧的", artifact({ draftBody: "旧的", draftStatus: "working" }), "working"],
    ["everything flushed and saved", "旧的", artifact({ draftBody: "旧的", draftStatus: "saved" }), "saved"],
    ["no document open", "", undefined, "working"],
  ])("%s reads as %s", (_name, local, stored, want) => {
    expect(draftBadge(local, stored as Artifact | undefined)).toBe(want);
  });

  it("never claims saved while anything is outstanding", () => {
    // Telling somebody their work is saved when it is not is the failure that
    // costs them the work, so every uncertain case falls to working.
    const cases: Array<[string, Artifact]> = [
      ["新的", artifact({ draftBody: "旧的", draftStatus: "saved" })],
      ["旧的", artifact({ draftBody: "旧的", draftStatus: "working" })],
    ];
    for (const [local, stored] of cases) {
      expect(draftBadge(local, stored)).toBe("working");
    }
  });
});

describe("canSaveVersion", () => {
  it("is false for an untouched document with a version already", () => {
    // Saving an unchanged document again adds a version identical to the last,
    // which is noise in a history whose job is to be readable.
    expect(canSaveVersion("第1版", artifact({ draftBody: "第1版" }), [version(1)])).toBe(false);
  });

  it("is true once anything is outstanding", () => {
    expect(canSaveVersion("改了", artifact({ draftBody: "第1版" }), [version(1)])).toBe(true);
    expect(
      canSaveVersion("第1版", artifact({ draftBody: "第1版", draftStatus: "working" }), [version(1)]),
    ).toBe(true);
  });

  it("is true for a first version with something typed, false for a blank one", () => {
    expect(canSaveVersion("开头", artifact({ draftBody: "开头" }), [])).toBe(true);
    expect(canSaveVersion("", artifact({ draftBody: "" }), [])).toBe(false);
  });

  it("is false when no document is open", () => {
    expect(canSaveVersion("x", undefined, [])).toBe(false);
  });
});

describe("comparisonPair", () => {
  // Versions arrive newest first.
  const versions = [version(3), version(2), version(1)];

  it("puts the older version on the left whichever order it was picked", () => {
    expect(comparisonPair(versions, "v-1", "v-3")?.left.revision).toBe(1);
    expect(comparisonPair(versions, "v-3", "v-1")?.left.revision).toBe(1);
    expect(comparisonPair(versions, "v-3", "v-1")?.right.revision).toBe(3);
  });

  it("returns nothing rather than comparing a version with itself", () => {
    expect(comparisonPair(versions, "v-2", "v-2")).toBeNull();
  });

  it("returns nothing when either side is unknown", () => {
    expect(comparisonPair(versions, "v-2", "v-9")).toBeNull();
    expect(comparisonPair(versions, "", "v-2")).toBeNull();
    expect(comparisonPair([], "v-1", "v-2")).toBeNull();
  });
});

describe("defaultComparisonTarget", () => {
  const versions = [version(3), version(2), version(1)];

  it("defaults to the version before the one picked", () => {
    // "What changed in this version" is the question somebody clicking a
    // history row is asking.
    expect(defaultComparisonTarget(versions, "v-3")).toBe("v-2");
    expect(defaultComparisonTarget(versions, "v-2")).toBe("v-1");
  });

  it("has nothing to compare the first version against", () => {
    expect(defaultComparisonTarget(versions, "v-1")).toBe("");
    expect(defaultComparisonTarget(versions, "v-9")).toBe("");
  });
});

describe("nextArtifactPosition", () => {
  it("puts a new document after the last one, and starts at 1", () => {
    expect(nextArtifactPosition([])).toBe(1);
    expect(nextArtifactPosition([artifact({ position: 1 }), artifact({ position: 4 })])).toBe(5);
  });
});

describe("the model-backed entry points", () => {
  it("are exactly three, and every one is unavailable in this phase", () => {
    // Listed rather than written into the page, so the page cannot quietly
    // render two of them and the count is something a reader can check.
    expect(AI_ENTRY_POINTS.map((entry) => entry.id)).toEqual([
      "rewriteSelection", "polishWhole", "candidateVersions",
    ]);
    for (const entry of AI_ENTRY_POINTS) {
      expect(aiEntryPointEnabled(entry.id)).toBe(false);
    }
  });
});
