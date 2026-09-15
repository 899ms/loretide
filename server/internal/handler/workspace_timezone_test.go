package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/middleware"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The brand workspace's timezone lives at settings["loretide.timezone"] - a key
// in the existing JSONB, not a column, so there is no migration (spec FR-007).
// The server is the authority on what is storable: the picker offers what Intl
// knows, but only what Go's time.LoadLocation resolves may be written.
//
// Contract: specs/004-lt009-brand-workspace/contracts/workspace-timezone.md

const timezoneKey = "loretide.timezone"

func workspaceWithOwner(t *testing.T, slug, name string) string {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": name, "slug": slug, "description": "timezone test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)
	return wsID
}

func settingsOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	settings, ok := body["settings"].(map[string]any)
	if !ok {
		t.Fatalf("response settings is %T, want an object: %v", body["settings"], body)
	}
	return settings
}

func TestCreateWorkspace_AcceptsAValidTimezone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-valid"
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})

	req := newRequest("POST", "/api/workspaces", map[string]any{
		"name": "Brand A", "slug": slug,
		"settings": map[string]any{timezoneKey: "America/Los_Angeles"},
	})
	var body map[string]any
	testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusCreated).JSON(&body)
	if got := settingsOf(t, body)[timezoneKey]; got != "America/Los_Angeles" {
		t.Errorf("created workspace timezone = %v, want America/Los_Angeles", got)
	}
}

// Every value here is rejected by time.LoadLocation, or must be: "" and "Local"
// DO resolve in Go - "" silently means UTC and "Local" means the server's own
// zone - so a validator that only asked LoadLocation would store both. Neither
// is a brand's timezone, and "" would read back as the default on the client
// while the server considered it set.
func TestCreateWorkspace_RejectsAnUnusableTimezone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	for _, tz := range []any{"Mars/Olympus", "+08:00", "", "Local", "asia/shanghai", 42, nil} {
		t.Run(nameFor(tz), func(t *testing.T) {
			slug := "handler-tests-tz-bad"
			_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
			})
			req := newRequest("POST", "/api/workspaces", map[string]any{
				"name": "Brand Bad", "slug": slug,
				"settings": map[string]any{timezoneKey: tz},
			})
			testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusBadRequest)

			var count int
			dbfx.QueryRow(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug).Scan(&count)
			if count != 0 {
				t.Errorf("a rejected timezone still created the workspace")
			}
		})
	}
}

func nameFor(v any) string {
	switch x := v.(type) {
	case string:
		if x == "" {
			return "empty"
		}
		return x
	case nil:
		return "null"
	default:
		return "non-string"
	}
}

func TestCreateWorkspace_WithoutATimezoneReadsAsTheDefault(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-absent"
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})

	req := newRequest("POST", "/api/workspaces", map[string]any{"name": "Brand C", "slug": slug})
	var body map[string]any
	testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusCreated).JSON(&body)
	if got := settingsOf(t, body)[timezoneKey]; got != "Asia/Shanghai" {
		t.Errorf("timezone = %v, want the Asia/Shanghai default (FR-002)", got)
	}
}

// A workspace that predates this feature has no such key. The response fills
// the default so the client never sees an absent value, but the stored row is
// left alone - filling it in on read would be a write nobody asked for.
func TestGetWorkspace_FillsTheDefaultWithoutWritingItBack(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-legacy"
	wsID := workspaceWithOwner(t, slug, "Legacy Brand")
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	dbfx.Exec(t, `UPDATE workspace SET settings = '{"co_authored_by_enabled": true}'::jsonb WHERE id = $1`, wsID)

	req := withURLParam(newRequest("GET", "/api/workspaces/"+wsID, nil), "id", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&body)
	if got := settingsOf(t, body)[timezoneKey]; got != "Asia/Shanghai" {
		t.Errorf("legacy workspace timezone = %v, want the default", got)
	}

	var stored string
	dbfx.QueryRow(t, `SELECT settings::text FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored != `{"co_authored_by_enabled": true}` {
		t.Errorf("reading the workspace rewrote its settings to %s", stored)
	}
}

func TestUpdateWorkspace_RejectsAnUnusableTimezoneAndKeepsTheOldOne(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-update-bad"
	wsID := workspaceWithOwner(t, slug, "Brand Update")
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	dbfx.Exec(t, `UPDATE workspace SET settings = '{"loretide.timezone": "Europe/London"}'::jsonb WHERE id = $1`, wsID)

	req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{timezoneKey: "Mars/Olympus"},
	}), "id", wsID)
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusBadRequest)

	var stored string
	dbfx.QueryRow(t, `SELECT settings->>'loretide.timezone' FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored != "Europe/London" {
		t.Errorf("a rejected update changed the stored timezone to %q", stored)
	}
}

// The one that protects the rest of the product: settings is replaced wholesale
// by this endpoint, so an update that does not mention the timezone must still
// leave every other key exactly as the caller sent it, and an update that does
// mention it must not disturb its siblings.
func TestUpdateWorkspace_PassesEveryOtherSettingsKeyThrough(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-passthrough"
	wsID := workspaceWithOwner(t, slug, "Brand Passthrough")
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})

	// An update with no timezone at all must be untouched by the new validation.
	req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{"co_authored_by_enabled": false, "other_key": "kept"},
	}), "id", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK).JSON(&body)
	settings := settingsOf(t, body)
	if settings["co_authored_by_enabled"] != false || settings["other_key"] != "kept" {
		t.Errorf("an update without a timezone altered the other settings: %v", settings)
	}

	// And an update that does carry one keeps its siblings.
	req = withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{
			"co_authored_by_enabled": false, "other_key": "kept", timezoneKey: "UTC",
		},
	}), "id", wsID)
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK).JSON(&body)
	settings = settingsOf(t, body)
	if settings[timezoneKey] != "UTC" || settings["other_key"] != "kept" {
		t.Errorf("setting the timezone disturbed the other settings: %v", settings)
	}
}

// FR-006 / User Story 3: a non-member is refused and the body carries no object
// fields - not the name, not the settings, not the timezone.
//
// This mounts the real middleware rather than calling the handler, because that
// is where the guard lives: router.go puts GET /api/workspaces/{id} inside a
// group with RequireWorkspaceMemberFromURL, and the handler itself does no
// membership check at all. Calling the handler directly returns 200 with the
// whole object - which looks alarming and is not, as long as nothing mounts it
// outside that group. The spec's Independent Test for this story described a
// direct handler call and would have "proved" a hole that does not exist; the
// spec is corrected accordingly.
func TestWorkspaceTimezone_IsNotReadableByANonMember(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-tz-nonmember"
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Someone Elses Brand", "slug": slug, "description": "non-member probe",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	dbfx.Exec(t, `UPDATE workspace SET settings = '{"loretide.timezone": "Europe/London"}'::jsonb WHERE id = $1`, wsID)

	router := chi.NewRouter()
	router.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceMemberFromURL(testHandler.Queries, "id"))
			r.Get("/", testHandler.GetWorkspace)
			r.Patch("/", testHandler.UpdateWorkspace)
		})
	})

	// testUserID is deliberately not a member of this workspace.
	for _, probe := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPatch, `{"settings":{"loretide.timezone":"UTC"}}`},
	} {
		var reader io.Reader
		if probe.body != "" {
			reader = strings.NewReader(probe.body)
		}
		req := httptest.NewRequest(probe.method, "/api/workspaces/"+wsID+"/", reader)
		req.Header.Set("X-User-ID", testUserID)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("non-member %s returned %d, want 403 or 404: %s", probe.method, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, leak := range []string{"Someone Elses Brand", "Europe/London", "loretide.timezone"} {
			if strings.Contains(body, leak) {
				t.Errorf("non-member %s refusal leaked %q: %s", probe.method, leak, body)
			}
		}
	}

	// And the refused write changed nothing.
	var stored string
	dbfx.QueryRow(t, `SELECT settings->>'loretide.timezone' FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored != "Europe/London" {
		t.Errorf("a refused non-member update changed the timezone to %q", stored)
	}
}

// Switch isolation, at the layer this feature owns: two workspaces hold two
// timezones, and reading one never returns the other's. User Story 2's browser
// checks cover the caches and the live stream; this covers the field the
// feature added, which those checks cannot see.
func TestWorkspaceTimezone_IsPerWorkspace(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	type brand struct{ slug, zone, id string }
	brands := []brand{
		{"handler-tests-tz-brand-a", "Asia/Shanghai", ""},
		{"handler-tests-tz-brand-b", "America/Los_Angeles", ""},
	}
	for i := range brands {
		brands[i].id = workspaceWithOwner(t, brands[i].slug, "Brand "+brands[i].slug)
		slug := brands[i].slug
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
		})
		req := withURLParam(newRequest("PATCH", "/api/workspaces/"+brands[i].id, map[string]any{
			"settings": map[string]any{timezoneKey: brands[i].zone},
		}), "id", brands[i].id)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)
	}

	// Read each back and assert it carries its own zone and not its neighbour's.
	for i, b := range brands {
		other := brands[1-i]
		req := withURLParam(newRequest("GET", "/api/workspaces/"+b.id, nil), "id", b.id)
		var body map[string]any
		testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&body)
		got := settingsOf(t, body)[timezoneKey]
		if got != b.zone {
			t.Errorf("%s returned timezone %v, want its own %s", b.slug, got, b.zone)
		}
		if got == other.zone {
			t.Errorf("%s returned the OTHER workspace's timezone %s", b.slug, other.zone)
		}
	}
}
