// Live-stream state machine for the diagnostics page.
//
// The server bounds each stream connection to a window and closes it on
// purpose; the last page of a planned window is marked `rotate`. Reading that
// marker here is what separates a scheduled handover — which the user must not
// see — from a real disconnect, which the user must see. The machine is a pure
// function so the transitions can be tested without a DOM; `queries.ts` owns
// the fetch, the timers and the visibility listener.

export type StreamStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected"
  | "paused"
  | "denied";

export type StreamState = {
  status: StreamStatus;
  cursor: number;
  gap: boolean;
  notice: string;
  backoffMs: number;
};

export type StreamEvent =
  // A connection attempt is starting.
  | { type: "connect" }
  // The response arrived and is readable.
  | { type: "open" }
  // One NDJSON page was read.
  | { type: "page"; cursor: number; gap: boolean; rotate: boolean }
  // The connection closed without a rotate page.
  | { type: "end" }
  // The request failed. `status` is the HTTP status when there was one.
  | { type: "error"; status?: number; detail?: string }
  | { type: "pause" }
  | { type: "resume" }
  | { type: "workspace-change" }
  | { type: "filter-change" }
  | { type: "clear-gap" };

// reconnectInMs says what the caller should do next: null to stay put, 0 to
// connect immediately, a positive delay to wait first.
export type StreamTransition = { state: StreamState; reconnectInMs: number | null };

export const STREAM_BACKOFF_START_MS = 2000;
export const STREAM_BACKOFF_CEILING_MS = 30000;

export const initialStreamState: StreamState = {
  status: "paused",
  cursor: 0,
  gap: false,
  notice: "",
  backoffMs: 0,
};

const GAP_NOTICE = "实时流存在保留期缺口；已保留可读取的后续记录。";
const END_NOTICE = "实时流已断开，正在从上次游标重新连接。";
const ERROR_NOTICE = "实时流连接失败，正在从上次游标重新连接。";
const DENIED_NOTICE = "实时流授权失败，已停止重连。";
const DENIED_NOTICE_PREFIX = "实时流授权失败，已停止重连：";

// A failed attempt waits twice as long as the last one, up to the ceiling. Any
// successful page resets the caller's stored delay back to zero.
export function computeBackoff(previousMs: number): number {
  if (previousMs <= 0) return STREAM_BACKOFF_START_MS;
  return Math.min(previousMs * 2, STREAM_BACKOFF_CEILING_MS);
}

function isAuthorizationFailure(status: number | undefined): boolean {
  return status === 403 || status === 404;
}

function disconnect(state: StreamState, notice: string): StreamTransition {
  const backoffMs = computeBackoff(state.backoffMs);
  return {
    state: { ...state, status: "disconnected", notice, backoffMs },
    reconnectInMs: backoffMs,
  };
}

export function nextStreamState(state: StreamState, event: StreamEvent): StreamTransition {
  switch (event.type) {
    case "workspace-change":
      // A different workspace is a different event log: the cursor and the
      // recorded gap belong to the one being left.
      return {
        state: { ...initialStreamState, status: "connecting" },
        reconnectInMs: 0,
      };
    case "filter-change":
      // Same log, different question. The cursor restarts so the answer is
      // complete, but a gap already reported stays reported (FR-007).
      return {
        state: { ...state, status: "connecting", cursor: 0, notice: state.gap ? GAP_NOTICE : "", backoffMs: 0 },
        reconnectInMs: 0,
      };
    case "clear-gap":
      return { state: { ...state, gap: false, notice: state.notice === GAP_NOTICE ? "" : state.notice }, reconnectInMs: null };
    case "pause":
      return { state: { ...state, status: "paused", notice: "", backoffMs: 0 }, reconnectInMs: null };
    case "resume":
      // Resuming is the user asking, so it also releases a denied stream.
      if (state.status === "connected") return { state, reconnectInMs: null };
      return { state: { ...state, status: "connecting", notice: "", backoffMs: 0 }, reconnectInMs: 0 };
    default:
      break;
  }

  // Paused and denied are terminal until the user acts: nothing the transport
  // reports may restart them on its own (FR-004).
  if (state.status === "paused" || state.status === "denied") {
    return { state, reconnectInMs: null };
  }

  switch (event.type) {
    case "connect":
      // A rotate handover keeps the status the user sees, so the planned
      // window boundary is invisible (FR-001).
      if (state.status === "connected") return { state, reconnectInMs: null };
      return { state: { ...state, status: state.cursor > 0 ? "reconnecting" : "connecting" }, reconnectInMs: null };
    case "open":
      return { state: { ...state, status: "connected", notice: state.gap ? GAP_NOTICE : "" }, reconnectInMs: null };
    case "page": {
      const gap = state.gap || event.gap;
      return {
        state: {
          ...state,
          status: "connected",
          cursor: event.cursor,
          gap,
          notice: gap ? GAP_NOTICE : "",
          backoffMs: 0,
        },
        // A rotate page is the planned last one: resume at once, with no
        // change to what the user is looking at.
        reconnectInMs: event.rotate ? 0 : null,
      };
    }
    case "end":
      return disconnect(state, END_NOTICE);
    case "error":
      if (isAuthorizationFailure(event.status)) {
        return {
          state: { ...state, status: "denied", notice: event.detail ? `${DENIED_NOTICE_PREFIX}${event.detail}` : DENIED_NOTICE, backoffMs: 0 },
          reconnectInMs: null,
        };
      }
      return disconnect(state, ERROR_NOTICE);
    default:
      return { state, reconnectInMs: null };
  }
}
