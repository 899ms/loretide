/**
 * @vitest-environment jsdom
 */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "../../api";
import type { ApiClient } from "../../api/client";
import { accountKeys, useAccountProfile, useSetAccountProfile } from "./queries";
import { emptyProfile } from "./profile";

// What this file covers is the wiring the page depends on and cannot see: what
// the hooks make of a response, what they do when there is none, and which
// caches a confirmation invalidates. The readiness rule itself is tested
// against the parity matrix in profile.test.ts and is not re-run here.

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

function client() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

describe("useAccountProfile", () => {
  afterEach(() => vi.restoreAllMocks());

  it("parses a stored profile and keeps the server's own decisions", async () => {
    setApiInstance({
      getContentAccountProfile: vi.fn(async () => ({
        revision_id: "rev-1",
        revision: 4,
        profile: {
          audience: { value: "new managers", status: "confirmed" },
          content_pillars: { value: "one-on-ones", status: "confirmed" },
          primary_channels: { values: ["xiaohongshu"], status: "confirmed" },
          weekly_hours: { value: 6, status: "confirmed" },
          style_samples: { values: [], status: "pending" },
        },
        readiness: { can_start: true, missing: null },
        uses_neutral_expression: true,
      })),
    } as unknown as ApiClient);

    const qc = client();
    const { result } = renderHook(() => useAccountProfile("ws-1", "acct-1"), {
      wrapper: wrapper(qc),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.revisionId).toBe("rev-1");
    expect(result.current.data?.revision).toBe(4);
    expect(result.current.data?.profile.audience.value).toBe("new managers");
    expect(result.current.data?.readiness).toEqual({ can_start: true, missing: [] });
    expect(result.current.data?.usesNeutralExpression).toBe(true);
    // A field the server did not send is pending and empty, not absent: the
    // page reads every one of the ten without optional chaining.
    expect(result.current.data?.profile.positioning).toEqual({ value: "", status: "pending" });
    qc.clear();
  });

  // This used to assert the opposite: a failure came back as the empty
  // profile. The two states are not the same, and the endpoint answers 200
  // with an all-pending profile for an account that has no revisions, so the
  // fallback only ever hid a real error - as "all four fields are missing".
  it("reports a failed request as a failure, not as an empty profile", async () => {
    setApiInstance({
      getContentAccountProfile: vi.fn(async () => {
        throw new Error("boom");
      }),
    } as unknown as ApiClient);

    const qc = client();
    const { result } = renderHook(() => useAccountProfile("ws-1", "acct-1"), {
      wrapper: wrapper(qc),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
    qc.clear();
  });

  it("does not fetch before an account is selected", async () => {
    const get = vi.fn(async () => ({}));
    setApiInstance({ getContentAccountProfile: get } as unknown as ApiClient);

    const qc = client();
    renderHook(() => useAccountProfile("ws-1", ""), { wrapper: wrapper(qc) });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(get).not.toHaveBeenCalled();
    qc.clear();
  });
});

describe("useSetAccountProfile", () => {
  afterEach(() => vi.restoreAllMocks());

  it("sends the profile and invalidates both the profile and the persona", async () => {
    const set = vi.fn(async () => ({
      revision_id: "rev-2",
      account_id: "acct-1",
      revision: 5,
      persona_prompt: "",
    }));
    setApiInstance({ setContentAccountProfile: set } as unknown as ApiClient);

    const qc = client();
    const invalidated: unknown[] = [];
    vi.spyOn(qc, "invalidateQueries").mockImplementation(async (filters) => {
      invalidated.push(filters?.queryKey);
    });

    const { result } = renderHook(() => useSetAccountProfile("ws-1"), {
      wrapper: wrapper(qc),
    });
    const profile = emptyProfile();
    profile.audience = { value: "new managers", status: "confirmed" };
    const revision = await result.current.mutateAsync({ accountId: "acct-1", profile });

    expect(set).toHaveBeenCalledWith("acct-1", profile);
    expect(revision.revision).toBe(5);
    // One revision carries the prompt and the profile, so confirming the
    // profile moves the revision number the persona card shows.
    expect(invalidated).toEqual([
      accountKeys.profile("ws-1", "acct-1"),
      accountKeys.persona("ws-1", "acct-1"),
    ]);
    qc.clear();
  });
});
