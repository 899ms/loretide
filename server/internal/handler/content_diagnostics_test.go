package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
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
