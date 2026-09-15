// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  discardDraft,
  editDraft,
  emptyDraftState,
  isDirty,
  selectDraft,
  type AccountServerValues,
} from "./form-state";

const serverA: AccountServerValues = {
  platform: "xiaohongshu",
  displayName: "A 号",
  personaPrompt: "A 的人设",
};
const serverB: AccountServerValues = {
  platform: "weibo",
  displayName: "B 号",
  personaPrompt: "B 的人设",
};

describe("account drafts", () => {
  // A1. The canonical assertion for "switching accounts does not carry unsaved
  // input over". Half-typed text in A must not appear when B is selected.
  it("shows B's server values while A holds an unsaved draft", () => {
    const typed = editDraft(emptyDraftState, "a", serverA, {
      personaPrompt: "半截草稿",
    });

    expect(selectDraft(typed, "b", serverB)).toEqual({
      platform: "weibo",
      displayName: "B 号",
      personaPrompt: "B 的人设",
    });
  });

  it("keeps A's draft while B is being viewed", () => {
    const typed = editDraft(emptyDraftState, "a", serverA, {
      personaPrompt: "半截草稿",
    });
    selectDraft(typed, "b", serverB);

    expect(selectDraft(typed, "a", serverA).personaPrompt).toBe("半截草稿");
  });

  it("edits one account without touching another's bucket", () => {
    const both = editDraft(
      editDraft(emptyDraftState, "a", serverA, { personaPrompt: "A 改" }),
      "b",
      serverB,
      { personaPrompt: "B 改" },
    );

    expect(selectDraft(both, "a", serverA).personaPrompt).toBe("A 改");
    expect(selectDraft(both, "b", serverB).personaPrompt).toBe("B 改");
  });

  // A2. After a save the bucket goes away, so the next read comes from the
  // server. Writing the saved value back into the bucket instead would leave a
  // local copy that quietly disagrees the moment anyone else saves.
  it("returns the server value again once a draft is discarded", () => {
    const typed = editDraft(emptyDraftState, "a", serverA, {
      personaPrompt: "半截草稿",
    });
    const saved = discardDraft(typed, "a");

    expect(selectDraft(saved, "a", { ...serverA, personaPrompt: "新的人设" }))
      .toEqual({
        platform: "xiaohongshu",
        displayName: "A 号",
        personaPrompt: "新的人设",
      });
  });

  it("discarding an account with no draft changes nothing", () => {
    expect(discardDraft(emptyDraftState, "a")).toBe(emptyDraftState);
  });

  // A blank persona prompt is a real value (LT-012), so typing text and then
  // clearing it is an edit, not a return to pristine.
  it("treats clearing the prompt as an unsaved edit", () => {
    const cleared = editDraft(emptyDraftState, "a", serverA, {
      personaPrompt: "",
    });

    expect(isDirty(cleared, "a", serverA)).toBe(true);
    expect(selectDraft(cleared, "a", serverA).personaPrompt).toBe("");
  });

  it("is not dirty before anything is typed", () => {
    expect(isDirty(emptyDraftState, "a", serverA)).toBe(false);
  });

  it("is not dirty when a draft matches the server exactly", () => {
    const retyped = editDraft(emptyDraftState, "a", serverA, {
      personaPrompt: serverA.personaPrompt,
    });

    expect(isDirty(retyped, "a", serverA)).toBe(false);
  });
});
