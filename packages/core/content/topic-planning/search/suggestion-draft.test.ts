import { describe, expect, it } from "vitest";
import {
  beginSuggestionCreate, beginSuggestionRevision, suggestionCreateRequest, suggestionDraftContextMatches,
  suggestionRevisionRequest, updateSuggestionDraft,
} from "./suggestion-draft";
import type { SearchSuggestion, SearchSuggestionContentInput } from "./suggestion-contract";

const draft: SearchSuggestionContentInput = {
  target_question: "How do I wash wool?", aspects: ["body"], rationale: "Answer care steps",
  evidence_source_ids: ["source-1"], proposed_body: "Hand wash carefully.",
};
const suggestion = {
  suggestionId: "suggestion-1", workId: "work-1", artifactId: "artifact-1", themeId: "theme-1",
  baseVersionId: "version-7", revision: 3,
} as SearchSuggestion;

describe("search suggestion edit session", () => {
  it("pins create identity and base version at session start", () => {
    const session = beginSuggestionCreate({ workspaceId: "ws-1", workId: "work-1", artifactId: "artifact-1" }, "theme-1", "version-7", draft);
    expect(suggestionCreateRequest(session)).toEqual({ ...draft, work_id: "work-1", artifact_id: "artifact-1", theme_id: "theme-1", base_version_id: "version-7" });
    expect(suggestionDraftContextMatches(session, { workspaceId: "ws-1", workId: "work-1", artifactId: "artifact-2" })).toBe(false);
    const edited = updateSuggestionDraft(session, { ...draft, proposed_body: "Edited after opening" });
    expect(suggestionCreateRequest(edited)?.base_version_id).toBe("version-7");
  });

  it("pins a selected suggestion revision and preserves edits across refreshed server objects", () => {
    const session = beginSuggestionRevision("ws-1", suggestion, draft);
    const edited = updateSuggestionDraft(session, { ...draft, proposed_body: "Unsubmitted local draft" });
    expect(suggestionRevisionRequest(edited)).toEqual({
      suggestionId: "suggestion-1",
      input: { ...draft, proposed_body: "Unsubmitted local draft", base_revision: 3 },
    });
    expect(suggestionRevisionRequest(edited)?.input.base_revision).toBe(3);
  });

  it("does not open create sessions without fixed context and baseline", () => {
    expect(beginSuggestionCreate({ workspaceId: "ws-1", workId: "work-1", artifactId: "artifact-1" }, "theme-1", "", draft).mode).toBe("idle");
  });
});
