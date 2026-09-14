package middleware

import (
	"context"
	"crypto/rand"
	"net/http"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Trace propagation for every API route.
//
// Propagation and recording are separate concerns. This middleware only makes
// sure a request runs under a trace and that the caller is told which one;
// which routes write a technical event is still decided by the diagnostics
// route group (handler.DiagnosticTrace). Mounting this globally must not widen
// what gets recorded.
//
// An inbound traceparent is not trusted. Anyone can send one, and adopting it
// would let a caller choose which trace their events land on. The boundary
// therefore always mints this instance's own trace and keeps a checked inbound
// value only as a correlation attribute. The one exception is a request that
// has already authenticated as this instance's daemon, which AdoptDaemonTrace
// handles — after authentication, never from a header the caller controls.
//
// This file deliberately does not import the diagnostics package: a file
// outside the content roots that imports one would have to be registered as an
// adapter in scripts/content-boundaries.json. Everything crosses through
// context.Context instead.

// DiagnosticTraceHeader names the response header carrying the trace id. The
// value shape predates this middleware; only the set of routes that carry it
// changed.
const DiagnosticTraceHeader = "X-Diagnostic-Trace"

type ctxKeyUpstreamTrace struct{}

// UpstreamTraceFromContext returns the inbound trace id that was checked but
// deliberately not adopted as this request's parent, or "" when there was
// none or it was adopted. Callers may record it for correlation. It is not an
// identity: never use it as a query key, across workspaces, or in an
// authorization decision.
func UpstreamTraceFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUpstreamTrace{}).(string)
	return v
}

// extractUpstream reads an inbound traceparent under the same rules
// diagnostics.Unpack applies to a queue envelope: parse it, then require the
// result to be valid. An unparsable or all-zero value yields an invalid span
// context and is dropped rather than stored.
func extractUpstream(r *http.Request) trace.SpanContext {
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
	return trace.SpanContextFromContext(ctx)
}

// newLocalSpanContext mints a span that belongs to this instance. parent is
// the zero value for a fresh trace, or an adopted span context whose trace id
// this segment continues. Parentage is carried the way diagnostics.Child
// carries it — by the event's parent_span_id, not inside the span context,
// which has no field for it.
func newLocalSpanContext(parent trace.SpanContext) trace.SpanContext {
	cfg := trace.SpanContextConfig{TraceFlags: trace.FlagsSampled}
	if parent.IsValid() {
		cfg.TraceID = parent.TraceID()
	} else {
		_, _ = rand.Read(cfg.TraceID[:])
	}
	_, _ = rand.Read(cfg.SpanID[:])
	return trace.NewSpanContext(cfg)
}

// Trace gives every request a trace of this instance's own and reports it back
// on DiagnosticTraceHeader. A checked inbound traceparent is parked as a
// correlation attribute, never as the parent.
func Trace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := newLocalSpanContext(trace.SpanContext{})
		ctx := trace.ContextWithSpanContext(r.Context(), span)
		if upstream := extractUpstream(r); upstream.IsValid() {
			ctx = context.WithValue(ctx, ctxKeyUpstreamTrace{}, upstream.TraceID().String())
		}
		w.Header().Set(DiagnosticTraceHeader, span.TraceID().String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdoptDaemonTrace continues an inbound trace for a request that has already
// authenticated as this instance's daemon. Mount it after DaemonAuth: the
// decision reads DaemonAuthPathFromContext, which only a completed
// authentication sets, so nothing the caller sends can reach it.
//
// On adoption the correlation attribute is cleared — the inbound value is the
// trace now, so recording it separately would only duplicate it — and the
// response header is restated, because the id the caller is told must be the
// id the request actually ran under.
func AdoptDaemonTrace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if DaemonAuthPathFromContext(r.Context()) == "" {
			next.ServeHTTP(w, r)
			return
		}
		upstream := extractUpstream(r)
		if !upstream.IsValid() {
			next.ServeHTTP(w, r)
			return
		}
		span := newLocalSpanContext(upstream)
		ctx := trace.ContextWithSpanContext(r.Context(), span)
		ctx = context.WithValue(ctx, ctxKeyUpstreamTrace{}, "")
		w.Header().Set(DiagnosticTraceHeader, span.TraceID().String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
