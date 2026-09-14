package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestContentDiagnosticsAuthAndFaultGate(t *testing.T) {
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", false)
	for _, tc := range []struct {
		name, actor, user, workspace, query string
		status                              int
	}{{"unauthenticated", "", "", testWorkspaceID, "", 401}, {"machine", "task_token", testUserID, testWorkspaceID, "", 403}, {"cross-workspace", "", testUserID, "11111111-1111-1111-1111-111111111111", "", 404}, {"account", "", testUserID, testWorkspaceID, "?account_id=ungranted", 403}, {"owner", "", testUserID, testWorkspaceID, "", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/content-diagnostics/overview"+tc.query, nil)
			r.Header.Set("X-User-ID", tc.user)
			r.Header.Set("X-Workspace-ID", tc.workspace)
			r.Header.Set("X-Actor-Source", tc.actor)
			w := httptest.NewRecorder()
			h.ContentDiagnosticOverview(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
	r := httptest.NewRequest("POST", "/api/content-diagnostics/simulate", strings.NewReader(`{"scenario":"normal","seed":42}`))
	r.Header.Set("X-User-ID", testUserID)
	r.Header.Set("X-Workspace-ID", testWorkspaceID)
	w := httptest.NewRecorder()
	h.ContentDiagnosticSimulate(w, r)
	if w.Code != 403 {
		t.Fatalf("non-test gate: %d %s", w.Code, w.Body.String())
	}
}
func TestContentDiagnosticsRejectsMalformedCursorAndSensitiveInput(t *testing.T) {
	for _, q := range []string{"after=-1", "after=bogus", "from=no-date", "limit=100000"} {
		if _, err := diagnosticFilter(httptest.NewRequest("GET", "/?"+q, nil)); err == nil {
			t.Fatal(q)
		}
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	r := httptest.NewRequest("POST", "/api/content-diagnostics/simulate", strings.NewReader(`{"scenario":"normal","prompt":"secret"}`))
	r.Header.Set("X-User-ID", testUserID)
	r.Header.Set("X-Workspace-ID", testWorkspaceID)
	w := httptest.NewRecorder()
	h.ContentDiagnosticSimulate(w, r)
	if w.Code != 409 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Code, w.Body.String())
	}
	_, _ = context.Background(), h
}

// The tests below close the acceptance gaps in
// specs/002-diag-package-stream-recovery. FR map:
//
//	FR-001 TestContentDiagnosticStreamClosesPlannedWindowWithRotate
//	FR-001 TestContentDiagnosticStreamEndsWithoutRotateWhenMembershipIsRevoked
//	FR-002 TestContentDiagnosticStreamAppliesRequestedFilter
//	FR-005 TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved
//	FR-006 TestContentDiagnosticExportRefusesUngrantedAccount

// newDiagnosticWorkspace gives one test its own workspace so seeded diagnostic
// events cannot be read by — or leak into — the rest of the suite.
func newDiagnosticWorkspace(t *testing.T) (Handler, string) {
	t.Helper()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	slug := fmt.Sprintf("diag-stream-%d", time.Now().UnixNano())
	ws := dbfx.Workspace(t, "Diagnostics Stream", slug)
	dbfx.Member(t, ws, testUserID, "owner")
	dbfx.Cleanup(t, "DELETE FROM content_technical_log WHERE workspace_id=$1", ws)
	dbfx.Cleanup(t, "DELETE FROM content_operation_audit WHERE workspace_id=$1", ws)
	dbfx.Cleanup(t, "DELETE FROM content_diagnostic_run WHERE workspace_id=$1", ws)
	return h, ws
}

func seedTechnicalEvent(t *testing.T, h Handler, ws, severity, code string) {
	t.Helper()
	h.ContentDiagnostics.Store.Technical(context.Background(), diagnostics.Event{
		ID: diagnostics.NewID(), Workspace: ws, Actor: testUserID, ActorKind: "human",
		ObjectType: "diagnostics", Action: "query", Outcome: "success", Code: code,
		Trace: diagnostics.NewID(), Operation: diagnostics.NewID(),
		Occurred: time.Now().UTC(), Received: time.Now().UTC(),
		Component: "api", Severity: severity, Build: "test", Test: true,
	})
}

// streamLines runs the stream handler to completion and returns the NDJSON
// pages it wrote. The handler streams, so the response cannot be decoded as a
// single JSON document the way testutil's .JSON() helper decodes one.
func streamLines(t *testing.T, h Handler, ws, query string) []diagnostics.Page {
	t.Helper()
	req := testutil.WithHeaders(httptest.NewRequest("GET", "/api/content-diagnostics/stream?"+query, nil),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	res := testutil.Call(t, h.ContentDiagnosticStream, req).Want(200)
	pages := []diagnostics.Page{}
	for _, line := range strings.Split(strings.TrimSuffix(res.Body.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var page diagnostics.Page
		if err := json.Unmarshal([]byte(line), &page); err != nil {
			t.Fatalf("stream line is not a page: %v: %s", err, line)
		}
		pages = append(pages, page)
	}
	return pages
}

func TestContentDiagnosticStreamClosesPlannedWindowWithRotate(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	seedTechnicalEvent(t, h, ws, "info", "TIMEOUT")
	restore := diagnosticStreamWindow
	diagnosticStreamWindow = 50 * time.Millisecond
	t.Cleanup(func() { diagnosticStreamWindow = restore })

	pages := streamLines(t, h, ws, "after=0")
	if len(pages) < 2 {
		t.Fatalf("planned window wrote %d pages, want a first page and a closing page", len(pages))
	}
	for i, page := range pages[:len(pages)-1] {
		if page.Rotate {
			t.Fatalf("page %d is marked rotate before the window closed", i)
		}
	}
	last := pages[len(pages)-1]
	if !last.Rotate {
		t.Fatal("the planned window closed without a rotate page, so the client reports a disconnect")
	}
	if last.Cursor < pages[0].Cursor {
		t.Fatalf("closing cursor %d went backwards from %d", last.Cursor, pages[0].Cursor)
	}
}

func TestContentDiagnosticStreamAppliesRequestedFilter(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	seedTechnicalEvent(t, h, ws, "info", "TIMEOUT")
	seedTechnicalEvent(t, h, ws, "error", "SEARCH_FAILED")
	restore := diagnosticStreamWindow
	diagnosticStreamWindow = 50 * time.Millisecond
	t.Cleanup(func() { diagnosticStreamWindow = restore })

	seen := 0
	for _, page := range streamLines(t, h, ws, "after=0&kind=technical&severity=error&error_code=SEARCH_FAILED") {
		for _, e := range page.Events {
			seen++
			if e.Severity != "error" || e.Code != "SEARCH_FAILED" {
				t.Fatalf("stream ignored the filter: severity=%q code=%q", e.Severity, e.Code)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("filtered stream returned %d events, want the 1 matching event", seen)
	}
}

func TestContentDiagnosticStreamEndsWithoutRotateWhenMembershipIsRevoked(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	seedTechnicalEvent(t, h, ws, "info", "TIMEOUT")
	restore := diagnosticStreamWindow
	diagnosticStreamWindow = 10 * time.Second
	t.Cleanup(func() { diagnosticStreamWindow = restore })
	// Demote well before the first one-second batch boundary, so the recheck
	// at that boundary is what ends the stream, not the planned window.
	revoke := time.AfterFunc(100*time.Millisecond, func() {
		_, _ = testPool.Exec(context.Background(), "UPDATE member SET role='member' WHERE workspace_id=$1 AND user_id=$2", ws, testUserID)
	})
	t.Cleanup(func() { revoke.Stop() })

	start := time.Now()
	pages := streamLines(t, h, ws, "after=0")
	if elapsed := time.Since(start); elapsed >= diagnosticStreamWindow {
		t.Fatalf("revoked stream stayed open for %s; the membership recheck did not close it", elapsed)
	}
	for i, page := range pages {
		if page.Rotate {
			t.Fatalf("page %d is marked rotate; revocation is not a planned rotation", i)
		}
	}
}

func TestContentDiagnosticExportDownloadNamesTheFileItWantsSaved(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	sim := testutil.Call(t, h.ContentDiagnosticSimulate, testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/simulate", `{"scenario":"normal","seed":42}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)).Want(201)
	var run struct {
		ID string `json:"run_id"`
	}
	sim.JSON(&run)

	res := testutil.Call(t, h.ContentDiagnosticExport, testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/export?run_id="+run.ID, `{}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)).Want(200)
	if disposition := res.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment;") || !strings.Contains(disposition, "filename=") {
		t.Fatalf("download response does not name a file: %q", disposition)
	}
	var bundle map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("download body is not a bundle: %v", err)
	}
	if bundle["redacted"] != true {
		t.Fatalf("download bundle is not marked redacted: %v", bundle["redacted"])
	}
	// The bundle is read next to the server's own logs, so it keeps the wire
	// key names rather than anything the browser would rewrite them to.
	if _, ok := bundle["manifest"]; !ok {
		t.Fatalf("download bundle lost its wire key names: %v", bundle)
	}
}

func TestContentDiagnosticExportRefusesUngrantedAccount(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	res := testutil.Call(t, h.ContentDiagnosticExport, testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/export?run_id=missing&account_id=ungranted", `{}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)).Want(403)
	if res.Header().Get("Content-Disposition") != "" {
		t.Fatal("a refused export still told the browser to save a file")
	}
	var body struct {
		Code string `json:"code"`
		Next string `json:"next_action"`
	}
	res.JSON(&body)
	if body.Code != "AUTHORIZATION_DENIED" || body.Next == "" {
		t.Fatalf("refusal is not a diagnostic error object: %+v", body)
	}
}

// Feature 005 (specs/005-diag-trace-and-sanitize) FR map:
//
//	FR-002  TestContentDiagnosticTraceRunsThroughQueueAndDaemon
//	FR-003  TestContentDiagnosticBoundaryReportsTheTraceItRanUnder
//	FR-004  TestContentDiagnosticKeepsAUserTraceparentAsCorrelationOnly
//	FR-004a TestContentDiagnosticDropsAMalformedTraceparent
//	FR-015  TestTracePropagatesWithoutRecordingOutsideDiagnostics

// technicalEventCount counts rows a workspace has, so a test can assert that a
// request added one — or, for FR-015, that it added none.
func technicalEventCount(t *testing.T, ws string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		"SELECT count(*) FROM content_technical_log WHERE workspace_id=$1", ws).Scan(&n); err != nil {
		t.Fatalf("count technical log: %v", err)
	}
	return n
}

// latestTechnicalEvent returns the newest event a workspace recorded.
func latestTechnicalEvent(t *testing.T, ws string) diagnostics.Event {
	t.Helper()
	var payload []byte
	if err := testPool.QueryRow(context.Background(),
		"SELECT payload FROM content_technical_log WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1", ws).Scan(&payload); err != nil {
		t.Fatalf("read newest technical event: %v", err)
	}
	var e diagnostics.Event
	if err := json.Unmarshal(payload, &e); err != nil {
		t.Fatalf("decode technical event: %v", err)
	}
	return e
}

// callTraced runs a handler behind the same middleware chain the router mounts,
// so the test exercises the boundary rather than the handler in isolation.
func callTraced(t *testing.T, h Handler, next http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	middleware.Trace(h.DiagnosticTrace(next)).ServeHTTP(rec, req)
	return rec
}

func TestContentDiagnosticTraceRunsThroughQueueAndDaemon(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/simulate", `{"scenario":"normal","seed":42}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)

	rec := callTraced(t, h, h.ContentDiagnosticSimulate, req)
	if rec.Code != 201 {
		t.Fatalf("simulate: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	traceID := rec.Header().Get(middleware.DiagnosticTraceHeader)
	if traceID == "" {
		t.Fatal("no trace id was returned, so nothing downstream can be correlated to this request")
	}

	var run struct {
		Events []struct {
			Trace     string `json:"trace_id"`
			Component string `json:"component"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if len(run.Events) == 0 {
		t.Fatal("the run produced no events")
	}
	// The run walks web -> api -> database -> queue -> daemon -> executor ->
	// tool -> result -> database, and every hop goes through Pack/Unpack. If the
	// HTTP boundary is wired, all of them carry the trace the request ran under.
	seen := map[string]bool{}
	for _, e := range run.Events {
		seen[e.Component] = true
		if e.Trace != traceID {
			t.Fatalf("component %q ran under trace %s, but the request was %s: the chain is broken at this hop",
				e.Component, e.Trace, traceID)
		}
	}
	for _, want := range []string{"queue", "daemon", "result"} {
		if !seen[want] {
			t.Fatalf("the run never reached %q, so this test did not prove continuity through it", want)
		}
	}
}

func TestContentDiagnosticBoundaryReportsTheTraceItRanUnder(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/client", `{"code":"UI_ERROR"}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)

	before := technicalEventCount(t, ws)
	rec := callTraced(t, h, h.ContentDiagnosticClient, req)
	if rec.Code != 201 {
		t.Fatalf("client error: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := technicalEventCount(t, ws); got <= before {
		t.Fatalf("a recorded route wrote no technical event (%d -> %d)", before, got)
	}
	header := rec.Header().Get(middleware.DiagnosticTraceHeader)
	if got := latestTechnicalEvent(t, ws).Trace; got != header {
		t.Fatalf("%s = %s but the event was recorded under %s; the caller was told the wrong id",
			middleware.DiagnosticTraceHeader, header, got)
	}
}

func TestContentDiagnosticKeepsAUserTraceparentAsCorrelationOnly(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	const inbound = "44444444444444444444444444444444"
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/client", `{"code":"UI_ERROR"}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws,
		"traceparent", "00-"+inbound+"-5555555555555555-01")

	rec := callTraced(t, h, h.ContentDiagnosticClient, req)
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	event := latestTechnicalEvent(t, ws)
	if event.Trace == inbound {
		t.Fatal("a user-credential request chose its own trace id; the boundary must mint its own")
	}
	if event.Upstream != inbound {
		t.Fatalf("upstream_trace = %q, want %q kept as a correlation attribute", event.Upstream, inbound)
	}
}

func TestContentDiagnosticDropsAMalformedTraceparent(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/client", `{"code":"UI_ERROR"}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws,
		"traceparent", "not-a-traceparent")

	rec := callTraced(t, h, h.ContentDiagnosticClient, req)
	// A bad header from the caller is not the caller's request failing.
	if rec.Code != 201 {
		t.Fatalf("a malformed traceparent failed the request: %d %s", rec.Code, rec.Body.String())
	}
	if event := latestTechnicalEvent(t, ws); event.Upstream != "" {
		t.Fatalf("upstream_trace = %q, want empty: an unparsable value is dropped, not stored", event.Upstream)
	}
}

// FR-015: propagation is global, recording is not. This is the only automated
// protection for the second half of that rule. If anyone mounts DiagnosticTrace
// on the shared stack, or teaches the propagation middleware to record, this
// test goes red — which is the whole reason it exists.
func TestTracePropagatesWithoutRecordingOutsideDiagnostics(t *testing.T) {
	_, ws := newDiagnosticWorkspace(t)

	// A route outside the diagnostics group: propagation reaches it, the
	// recording middleware does not.
	businessRoute := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"workspaces":[]}`))
	})

	before := technicalEventCount(t, ws)

	req := testutil.WithHeaders(httptest.NewRequest("GET", "/api/workspaces", nil),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	rec := httptest.NewRecorder()
	middleware.Trace(businessRoute).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("propagation changed a business route's response: %d", rec.Code)
	}
	if got := rec.Header().Get(middleware.DiagnosticTraceHeader); got == "" {
		t.Fatalf("%s missing on a business route; propagation is supposed to be global", middleware.DiagnosticTraceHeader)
	}
	if after := technicalEventCount(t, ws); after != before {
		t.Fatalf("a business route wrote %d technical event(s); recording must stay inside the diagnostics group",
			after-before)
	}
}
