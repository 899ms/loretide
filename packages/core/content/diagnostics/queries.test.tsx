/**
 * @vitest-environment jsdom
 */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "../../api";
import type { ApiClient } from "../../api/client";
import { useDiagnosticStream } from "./queries";

const event = { event_id: "a".repeat(32), sequence: 1, occurred_at: "2026-01-01T00:00:00Z", received_at: "2026-01-01T00:00:00Z", actor_kind: "system", actor_id: "", workspace_id: "ws-1", account_id: "", object_type: "diagnostics", object_id: "run", object_version: "1", action: "query", outcome: "success", error_code: "", operation_id: "b".repeat(32), trace_id: "c".repeat(32), span_id: "d".repeat(16), parent_span_id: "", run_id: "run", attempt: 1, step: "api", component: "api", severity: "info", duration_ms: 1, safe_message: "Completed", retryable: false, next_action: "inspect_trace", build: "test", is_test: true };
function wrapper(qc: QueryClient) { return function Wrapper({ children }: { children: ReactNode }) { return <QueryClientProvider client={qc}>{children}</QueryClientProvider>; }; }
function closedStream(page: unknown) { const bytes = new TextEncoder().encode(`${JSON.stringify(page)}\n`); return new Response(new ReadableStream({ start(controller) { controller.enqueue(bytes); controller.close(); } })); }

describe("useDiagnosticStream", () => {
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });
  it("keeps its cursor, de-duplicates resumed events, and never reconnects after pause", async () => {
    const calls: string[] = [];
    const page = { events: [event], cursor: 1, gap: true, has_more: false };
    setApiInstance({ contentDiagnosticStream: vi.fn((query: string) => { calls.push(query); return Promise.resolve(closedStream(page)); }) } as unknown as ApiClient);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result, rerender, unmount } = renderHook(({ enabled }) => useDiagnosticStream("ws-1", enabled), { initialProps: { enabled: true }, wrapper: wrapper(qc) });
    await waitFor(() => expect(result.current.status).toBe("disconnected"));
    expect(result.current.events).toHaveLength(1);
    expect(result.current.notice).toContain("断开");
    expect(result.current.gap).toBe(true);
    await new Promise((resolve) => setTimeout(resolve, 2100));
    await waitFor(() => expect(calls).toEqual(["after=0", "after=1"]));
    await waitFor(() => expect(result.current.events).toHaveLength(1));
    rerender({ enabled: false });
    unmount();
    await new Promise((resolve) => setTimeout(resolve, 2100));
    expect(calls).toHaveLength(2);

    qc.clear();
  }, 10000);
});
