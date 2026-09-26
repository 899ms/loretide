import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { searchSuggestionAdoptionMutationOptions, searchSuggestionRetryMutationOptions } from "./suggestion-queries";

const wire = {
  suggestion_id: "s1", revision: 1, work_id: "w1", artifact_id: "a1", base_version_id: "v1",
  theme_id: "t1", theme_revision: 1, target_question: "问题", aspects: ["body"], rationale: "依据",
  evidence_source_ids: [], proposed_body: "新正文", author_kind: "human", recorded_by: "u1",
  created_at: "2026-09-26T00:00:00Z", state: "adopted", failure_code: "", base_is_current: false,
  theme_changed: false, decision: null, effects: [],
};

describe("search suggestion adoption mutation options", () => {
  afterEach(() => vi.restoreAllMocks());

  it("posts adoption and invalidates only after the authoritative response", async () => {
    const post = vi.fn(async () => wire);
    const invalidateQueries = vi.fn(async () => undefined);
    setApiInstance({ contentSearchPost: post } as unknown as ApiClient);
    const options = searchSuggestionAdoptionMutationOptions("ws-1", { invalidateQueries });
    await expect(options.mutationFn({ suggestionId: "s1", workId: "w1", artifactId: "a1", input: { decision: "adopt", revision: 1, note: "" } })).resolves.toMatchObject({ suggestionId: "s1" });
    expect(post).toHaveBeenCalledWith("suggestions/s1/decisions", { decision: "adopt", revision: 1, note: "" });
    expect(invalidateQueries).not.toHaveBeenCalled();
    options.onSuccess();
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["contentSearch", "ws-1", "suggestions"] });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["contentWorks", "ws-1"] });
  });

  it("retries only the stored decision and sends no body or revision", async () => {
    const post = vi.fn(async () => wire);
    setApiInstance({ contentSearchPost: post } as unknown as ApiClient);
    const options = searchSuggestionRetryMutationOptions("ws-1", { invalidateQueries: vi.fn(async () => undefined) });
    await options.mutationFn({ decisionId: "d1", workId: "w1", artifactId: "a1" });
    expect(post).toHaveBeenCalledWith("decisions/d1/retry", {});
  });
});
