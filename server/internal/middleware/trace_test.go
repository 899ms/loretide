package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// Feature 005 (specs/005-diag-trace-and-sanitize) FR map:
//
//	FR-001  the boundary always mints this instance's trace
//	FR-003  X-Diagnostic-Trace carries that trace id on every propagated route
//	FR-004  only an authenticated daemon path adopts an inbound traceparent
//	FR-004a a malformed inbound value is neither adopted nor kept
//	FR-015  propagation is global; recording is not this middleware's job
//	FR-016  the adoption decision reads the authentication result, not a header
//	SC-002  the returned id is the one the request ran under
//	SC-003  a forged value is never adopted and never fails the request

const (
	upstreamTraceID  = "11111111111111111111111111111111"
	upstreamSpanID   = "2222222222222222"
	validTraceparent = "00-" + upstreamTraceID + "-" + upstreamSpanID + "-01"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// seen captures what the wrapped handler observed, which is the only thing
// downstream code can act on.
type seen struct {
	span     trace.SpanContext
	upstream string
}

func runTrace(t *testing.T, mw func(http.Handler) http.Handler, req *http.Request) (*httptest.ResponseRecorder, seen) {
	t.Helper()
	var got seen
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.span = trace.SpanContextFromContext(r.Context())
		got.upstream = UpstreamTraceFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("body"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, got
}

func TestTracePropagationAtTheBoundary(t *testing.T) {
	for _, tc := range []struct {
		name        string
		traceparent string
		// wantUpstream is the correlation attribute the handler should see.
		wantUpstream string
	}{
		{name: "no inbound header", traceparent: "", wantUpstream: ""},
		{name: "valid inbound header is not adopted for a user request", traceparent: validTraceparent, wantUpstream: upstreamTraceID},
		{name: "unsampled inbound header still leaves a usable trace", traceparent: "00-" + upstreamTraceID + "-" + upstreamSpanID + "-00", wantUpstream: upstreamTraceID},
		{name: "malformed header is neither adopted nor kept", traceparent: "garbage", wantUpstream: ""},
		{name: "all-zero trace id is rejected like any other invalid value", traceparent: "00-" + "00000000000000000000000000000000" + "-" + upstreamSpanID + "-01", wantUpstream: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/anything", nil)
			if tc.traceparent != "" {
				req.Header.Set("traceparent", tc.traceparent)
			}
			rec, got := runTrace(t, Trace, req)

			// FR-001: a usable trace exists no matter what arrived.
			if !got.span.IsValid() {
				t.Fatal("handler ran without a valid span context; nothing downstream can correlate")
			}
			// FR-004/FR-004a: the inbound value is never the parent here.
			if got.span.TraceID().String() == upstreamTraceID {
				t.Fatalf("inbound traceparent was adopted on a user request: %s", got.span.TraceID())
			}
			if got.upstream != tc.wantUpstream {
				t.Fatalf("upstream correlation attribute = %q, want %q", got.upstream, tc.wantUpstream)
			}
			// FR-003/SC-002: the response names the trace the request ran under.
			header := rec.Header().Get("X-Diagnostic-Trace")
			if !hex32.MatchString(header) {
				t.Fatalf("X-Diagnostic-Trace = %q, want 32 hex characters", header)
			}
			if header != got.span.TraceID().String() {
				t.Fatalf("X-Diagnostic-Trace = %s but the request ran under %s", header, got.span.TraceID())
			}
			// FR-015/SC-003: the middleware carries the request, it does not change it.
			if rec.Code != http.StatusTeapot {
				t.Fatalf("status = %d, want %d: the middleware must not change the response", rec.Code, http.StatusTeapot)
			}
			if rec.Body.String() != "body" {
				t.Fatalf("body = %q, want %q", rec.Body.String(), "body")
			}
		})
	}
}

func TestTraceMintsADistinctTracePerRequest(t *testing.T) {
	first, _ := runTrace(t, Trace, httptest.NewRequest("GET", "/api/anything", nil))
	second, _ := runTrace(t, Trace, httptest.NewRequest("GET", "/api/anything", nil))
	if a, b := first.Header().Get("X-Diagnostic-Trace"), second.Header().Get("X-Diagnostic-Trace"); a == b {
		t.Fatalf("two requests shared trace id %s; each request needs its own", a)
	}
}

func TestTraceSpanIsChildOfNothingVisibleToTheCaller(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/anything", nil)
	req.Header.Set("traceparent", validTraceparent)
	_, got := runTrace(t, Trace, req)
	if got.span.SpanID().String() == upstreamSpanID {
		t.Fatal("this segment reused the caller's span id")
	}
}

func TestUpstreamTraceFromContextIsEmptyWithoutTheMiddleware(t *testing.T) {
	if v := UpstreamTraceFromContext(context.Background()); v != "" {
		t.Fatalf("bare context reported an upstream trace %q", v)
	}
}

// FR-004 / FR-016: adoption is gated on the authentication result that
// DaemonAuth records in the context, never on anything the caller can send.
func TestAdoptDaemonTraceOnlyAfterDaemonAuthentication(t *testing.T) {
	chain := func(next http.Handler) http.Handler { return Trace(AdoptDaemonTrace(next)) }

	t.Run("authenticated daemon continues the inbound trace", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/daemon/anything", nil)
		req.Header.Set("traceparent", validTraceparent)
		req = req.WithContext(WithDaemonContext(req.Context(), "ws-1", "daemon-1"))

		rec, got := runTrace(t, chain, req)

		if got.span.TraceID().String() != upstreamTraceID {
			t.Fatalf("trace id = %s, want the inbound %s: the daemon leg must continue the caller's trace",
				got.span.TraceID(), upstreamTraceID)
		}
		if got.span.SpanID().String() == upstreamSpanID {
			t.Fatal("this segment reused the caller's span id instead of opening its own")
		}
		if got.upstream != "" {
			t.Fatalf("upstream attribute = %q, want empty once adopted: the trace id already is that value", got.upstream)
		}
		if h := rec.Header().Get("X-Diagnostic-Trace"); h != upstreamTraceID {
			t.Fatalf("X-Diagnostic-Trace = %s, want %s: the caller must be told the id the request ran under", h, upstreamTraceID)
		}
	})

	t.Run("a caller claiming to be a daemon by header is not adopted", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/daemon/anything", nil)
		req.Header.Set("traceparent", validTraceparent)
		// Both of these are server-set headers that DaemonAuth strips. Setting
		// them here is exactly the forgery the rule exists to defeat.
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Daemon-ID", "daemon-1")

		_, got := runTrace(t, chain, req)

		if got.span.TraceID().String() == upstreamTraceID {
			t.Fatal("a self-declared daemon got its trace adopted; the decision must come from authentication")
		}
		if got.upstream != upstreamTraceID {
			t.Fatalf("upstream attribute = %q, want %q recorded as correlation only", got.upstream, upstreamTraceID)
		}
	})

	t.Run("authenticated daemon without an inbound trace still gets one", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/daemon/anything", nil)
		req = req.WithContext(WithDaemonContext(req.Context(), "ws-1", "daemon-1"))

		rec, got := runTrace(t, chain, req)

		if !got.span.IsValid() {
			t.Fatal("daemon request ran without a valid span context")
		}
		if h := rec.Header().Get("X-Diagnostic-Trace"); h != got.span.TraceID().String() {
			t.Fatalf("X-Diagnostic-Trace = %s but the request ran under %s", h, got.span.TraceID())
		}
	})

	t.Run("authenticated daemon sending a malformed trace is not adopted", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/daemon/anything", nil)
		req.Header.Set("traceparent", "garbage")
		req = req.WithContext(WithDaemonContext(req.Context(), "ws-1", "daemon-1"))

		_, got := runTrace(t, chain, req)

		if !got.span.IsValid() {
			t.Fatal("malformed inbound value left the request without a trace")
		}
		if got.upstream != "" {
			t.Fatalf("malformed value was kept as %q; it must be dropped, not stored", got.upstream)
		}
	})
}
