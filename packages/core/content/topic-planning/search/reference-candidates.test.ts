import { describe, expect, it } from "vitest";
import { validateSearchSuggestionEvidence, validateSearchThemeReferences } from "./reference-candidates";

describe("search optimization reference candidates", () => {
  it("accepts theme references only when each source, topic and brief is a current candidate", () => {
    const input = {
      sourceIds: ["source-1"], availableSourceIds: ["source-1", "source-2"],
      topicCardIds: ["topic-1"], availableTopicCardIds: ["topic-1", "topic-2"],
      briefRevisionIds: ["brief-1"],
      availableBriefs: [{ topicCardId: "topic-1", briefRevisionId: "brief-1" }, { topicCardId: "topic-2", briefRevisionId: "brief-2" }],
    };
    expect(validateSearchThemeReferences(input)).toEqual({ valid: true, missingSourceIds: [], missingTopicCardIds: [], missingBriefRevisionIds: [] });
    expect(validateSearchThemeReferences({ ...input, topicCardIds: ["topic-1", "topic-removed"], briefRevisionIds: ["brief-2"] })).toEqual({
      valid: false, missingSourceIds: [], missingTopicCardIds: ["topic-removed"], missingBriefRevisionIds: ["brief-2"],
    });
  });

  it("rejects evidence IDs removed from the latest candidate set while retaining their identities", () => {
    expect(validateSearchSuggestionEvidence(["source-1", "source-removed"], ["source-1"])).toEqual({
      valid: false, missingEvidenceSourceIds: ["source-removed"],
    });
    expect(validateSearchSuggestionEvidence([], [])).toEqual({ valid: true, missingEvidenceSourceIds: [] });
  });
});
