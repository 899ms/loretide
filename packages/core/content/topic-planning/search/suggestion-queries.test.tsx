/**
 * @vitest-environment jsdom
 */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { useAdoptSearchSuggestion, useRetrySearchSuggestionDecision } from "./suggestion-queries";

const wire = {
  suggestion_id: "s1", revision: 1, work_id: "w1", artifact_id: "a1", base_version_id: "v1",
  theme_id: "t1", theme_revision: 1, target_question: "问题", aspects: ["body"], rationale: "依据",
  evidence_source_ids: [], proposed_body: "新正文", author_kind: "human", recorded_by: "u1",
  created_at: "2026-09-26T00:00:00Z", state: "adopted", failure_code: "", base_is_current: false,
  theme_changed: false, decision: { decision_id: "d1", suggestion_id: "s1", suggestion_revision: 1,
    decision: "adopt", note: "", decided_by: "u1", created_at: "2026-09-26T00:00:00Z" },
  effects: [{ effect_id: "e1", decision_id: "d1", outcome: "done", version_id: "v2", failure_code: "", created_at: "2026-09-26T00:00:00Z" }],
};

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

function client() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
}

describe("search suggestion adoption mutations", () => {
  afterEach(() => vi.restoreAllMocks());

  it("adopts without an optimistic cache write and invalidates search and work state", async () => {
    const post = vi.fn(async () => wire);
    setApiInstance({ contentSearchPost: post } as unknown as ApiClient);
    const qc = client();
    const invalidated: unknown[] = [];
    vi.spyOn(qc, "invalidateQueries").mockImplementation(async (filters) => { invalidated.push(filters?.queryKey); });
    const { result } = renderHook(() => useAdoptSearchSuggestion("ws-1"), { wrapper: wrapper(qc) });
    await act(async () => {
      await result.current.mutateAsync({ suggestionId: "s1", workId: "w1", artifactId: "a1", input: { decision: "adopt", revision: 1, note: "" } });
    });
    expect(post).toHaveBeenCalledWith("suggestions/s1/decisions", { decision: "adopt", revision: 1, note: "" });
    expect(invalidated).toEqual([["contentSearch", "ws-1", "suggestions"], ["contentWorks", "ws-1"]]);
    qc.clear();
  });

  it("retries a stored decision without accepting a new body or revision", async () => {
    const post = vi.fn(async () => wire);
    setApiInstance({ contentSearchPost: post } as unknown as ApiClient);
    const qc = client();
    const { result } = renderHook(() => useRetrySearchSuggestionDecision("ws-1"), { wrapper: wrapper(qc) });
    await act(async () => {
      await result.current.mutateAsync({ decisionId: "d1", workId: "w1", artifactId: "a1" });
    });
    expect(post).toHaveBeenCalledWith("decisions/d1/retry", {});
    qc.clear();
  });
});
