package handler

import (
	"context"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"net/http/httptest"
	"strings"
	"testing"
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
