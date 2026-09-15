package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Diagnostics shipped its refusal behaviour before the shared helper existed
// and 002-V05-11 verified it. Extracting the decision must not move any of it,
// so these pin the three combinations BEFORE the handler changes, and must
// still pass after. If one of them goes red, the extraction regressed - the fix
// is the handler, never this file.
//
// A8 / A9 in specs/014-lt010-workspace-authorization/contracts.

func diagnosticsWorkspace(t *testing.T, slug string) string {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Diag Authz " + slug, "slug": slug, "description": "authz probe",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	return wsID
}

func diagnosticsRequest(t *testing.T, wsID string) *http.Request {
	t.Helper()
	return testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-diagnostics/overview", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
}

// A8, first combination: a non-member gets 404, not 403. The status is part of
// the contract because 403 would concede the workspace exists.
func TestDiagnosticsRefusesANonMemberWith404(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	wsID := diagnosticsWorkspace(t, "diag-authz-nonmember")

	rec := testutil.Call(t, h.ContentDiagnosticOverview, diagnosticsRequest(t, wsID)).
		Want(http.StatusNotFound)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refusal: %v: %s", err, rec.Body.String())
	}
	if body["code"] != "AUTHORIZATION_DENIED" {
		t.Errorf("code = %v, want AUTHORIZATION_DENIED", body["code"])
	}
	if body["next_action"] != "check_authorization" {
		t.Errorf("next_action = %v, want check_authorization", body["next_action"])
	}
	for _, leak := range []string{"Diag Authz", "diag-authz-nonmember", "settings"} {
		if containsField(rec.Body.String(), leak) {
			t.Errorf("refusal leaked %q: %s", leak, rec.Body.String())
		}
	}
}

// A8, second combination: a member whose role is too low gets 403 - and a 403
// here is safe precisely because the caller has already proven membership.
func TestDiagnosticsRefusesAnInsufficientRoleWith403(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	wsID := diagnosticsWorkspace(t, "diag-authz-role")
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		wsID, testUserID)

	rec := testutil.Call(t, h.ContentDiagnosticOverview, diagnosticsRequest(t, wsID)).
		Want(http.StatusForbidden)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refusal: %v: %s", err, rec.Body.String())
	}
	if body["code"] != "AUTHORIZATION_DENIED" {
		t.Errorf("code = %v, want AUTHORIZATION_DENIED", body["code"])
	}
	// The sanitized diagnostic error object, field for field.
	for _, key := range []string{"error", "code", "trace_id", "component", "retryable", "next_action"} {
		if _, ok := body[key]; !ok {
			t.Errorf("refusal body is missing %q: %v", key, body)
		}
	}
	if len(body) != 6 {
		t.Errorf("refusal body has %d fields, want exactly 6: %v", len(body), body)
	}
}

// A8, third combination: an account id outside the allowed set gets 403.
func TestDiagnosticsRefusesAnUngrantedAccountWith403(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	wsID := diagnosticsWorkspace(t, "diag-authz-account")
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-diagnostics/overview?account_id=someone-else", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	rec := testutil.Call(t, h.ContentDiagnosticOverview, req).Want(http.StatusForbidden)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "AUTHORIZATION_DENIED" {
		t.Errorf("code = %v, want AUTHORIZATION_DENIED", body["code"])
	}
}

// A9: the refusal must not reveal which workspace ids are real. A workspace
// that exists but is not yours, and one that does not exist at all, have to
// come back identical - byte for byte, trace id aside.
func TestDiagnosticsRefusalDoesNotRevealWhetherTheWorkspaceExists(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	real := diagnosticsWorkspace(t, "diag-authz-someone-elses")

	hidden := testutil.Call(t, h.ContentDiagnosticOverview, diagnosticsRequest(t, real)).
		Want(http.StatusNotFound).Body.String()
	missing := testutil.Call(t, h.ContentDiagnosticOverview,
		diagnosticsRequest(t, "44444444-4444-4444-8444-444444444444")).
		Want(http.StatusNotFound).Body.String()

	if hidden != missing {
		t.Errorf("a real-but-invisible workspace and a missing one answer differently, "+
			"which tells a caller which ids exist:\n  invisible: %s\n  missing:   %s",
			hidden, missing)
	}
}

func containsField(body, needle string) bool {
	return len(needle) > 0 && len(body) > 0 && json.Valid([]byte(body)) &&
		stringContains(body, needle)
}

func stringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
