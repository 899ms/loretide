package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The brand's automatic-precheck switch lives at
// settings["loretide.auto_precheck"] - a key in the existing JSONB, not a
// column, so there is no migration (spec FR-001). This file covers the server
// half: what is storable, what a read fills in, and that a read fills it in
// without writing it.
//
// This card is configuration only. Nothing here runs a precheck; that is EP-06.
//
// Contract: specs/019-lt015-brand-precheck-switch/contracts/auto-precheck.md

const autoPrecheckKey = "loretide.auto_precheck"

func autoPrecheckWorkspace(t *testing.T, slug, name string) string {
	t.Helper()
	wsID := workspaceWithOwner(t, slug, name)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	return wsID
}

// FR-002. A brand that never chose reads as on - including every brand created
// before this feature existed, which is the case that has no key at all.
func TestGetWorkspace_AutoPrecheckDefaultsToOn(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-default", "Brand Default")

	req := withURLParam(newRequest("GET", "/api/workspaces/"+wsID, nil), "id", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&body)

	got, ok := settingsOf(t, body)[autoPrecheckKey].(bool)
	if !ok || !got {
		t.Fatalf("a brand with no stored value reads as %v; the SOP says automatic precheck is on by default", settingsOf(t, body)[autoPrecheckKey])
	}
}

// FR-003. Reading must not write. Without this a single GET would turn "never
// chose" into "chose on" permanently, and the settings page could no longer
// tell the two apart.
func TestGetWorkspace_AutoPrecheckDefaultIsNotWrittenBack(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-nowrite", "Brand No Write")

	req := withURLParam(newRequest("GET", "/api/workspaces/"+wsID, nil), "id", wsID)
	testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK)

	var stored *bool
	dbfx.QueryRow(t, `SELECT (settings->>'loretide.auto_precheck')::boolean FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored != nil {
		t.Errorf("reading the workspace stored %v; filling the default is a response-only step", *stored)
	}
}

// FR-004, and the mistake this feature is most likely to make. false is the
// boolean zero value: any implementation that decides "is it set" from the
// value's truthiness reports a switched-off brand as never-chosen, and the
// default then turns it back on. A brand that switched the precheck off and
// found it on again next morning would have no way to tell what happened.
func TestWorkspaceAutoPrecheck_StoredFalseSurvivesAReadAsFalse(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-false", "Brand Off")

	req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{autoPrecheckKey: false},
	}), "id", wsID)
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)

	read := withURLParam(newRequest("GET", "/api/workspaces/"+wsID, nil), "id", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.GetWorkspace, read).Want(http.StatusOK).JSON(&body)

	got, ok := settingsOf(t, body)[autoPrecheckKey].(bool)
	if !ok || got {
		t.Fatalf("a brand that switched automatic precheck off reads back as %v: false was treated as unset and the default overwrote it", settingsOf(t, body)[autoPrecheckKey])
	}
}

// FR-005. Anything that is not a boolean is refused rather than stored: a
// settings blob carrying "false" as a string is worse than a rejected edit,
// because every reader then has to guess what it meant.
func TestUpdateWorkspace_RejectsANonBooleanAutoPrecheck(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-bad", "Brand Bad")
	dbfx.Exec(t, `UPDATE workspace SET settings = '{"loretide.auto_precheck": false}'::jsonb WHERE id = $1`, wsID)

	for name, value := range map[string]any{
		"string": "false", "number": 0, "null": nil,
		"object": map[string]any{"enabled": true}, "array": []any{true},
	} {
		t.Run(name, func(t *testing.T) {
			req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
				"settings": map[string]any{autoPrecheckKey: value},
			}), "id", wsID)
			testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusBadRequest)
		})
	}

	var stored bool
	dbfx.QueryRow(t, `SELECT (settings->>'loretide.auto_precheck')::boolean FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored {
		t.Error("a rejected update changed the stored value")
	}
}

func TestCreateWorkspace_RejectsANonBooleanAutoPrecheck(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-precheck-create-bad"
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})

	req := newRequest("POST", "/api/workspaces", map[string]any{
		"name": "Brand Create Bad", "slug": slug,
		"settings": map[string]any{autoPrecheckKey: "true"},
	})
	testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusBadRequest)

	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM workspace WHERE slug = $1`, slug).Scan(&count)
	if count != 0 {
		t.Errorf("a rejected creation left %d workspace rows behind", count)
	}
}

func TestCreateWorkspace_AcceptsAnExplicitAutoPrecheckValue(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	const slug = "handler-tests-precheck-create-ok"
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})

	req := newRequest("POST", "/api/workspaces", map[string]any{
		"name": "Brand Create OK", "slug": slug,
		"settings": map[string]any{autoPrecheckKey: false},
	})
	var body map[string]any
	testutil.Call(t, testHandler.CreateWorkspace, req).Want(http.StatusCreated).JSON(&body)

	if got, ok := settingsOf(t, body)[autoPrecheckKey].(bool); !ok || got {
		t.Errorf("created workspace reports %v, want false", settingsOf(t, body)[autoPrecheckKey])
	}
}

// An update that says nothing about this key must not be forced to carry it.
// Renaming a brand is not a statement about its precheck settings.
func TestUpdateWorkspace_AcceptsAnUpdateWithoutTheAutoPrecheckKey(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-absent", "Brand Absent")

	req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{timezoneKey: "Europe/London"},
	}), "id", wsID)
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)

	var stored *bool
	dbfx.QueryRow(t, `SELECT (settings->>'loretide.auto_precheck')::boolean FROM workspace WHERE id = $1`, wsID).Scan(&stored)
	if stored != nil {
		t.Errorf("an update that never mentioned the key stored %v", *stored)
	}
}

// SC-005. Two brands, two switches. If one brand's value could be read through
// another's, "this brand runs a precheck" would stop being a true statement
// about that brand.
func TestWorkspaceAutoPrecheck_IsIsolatedBetweenBrands(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	off := autoPrecheckWorkspace(t, "handler-tests-precheck-brand-off", "Brand Off")
	on := autoPrecheckWorkspace(t, "handler-tests-precheck-brand-on", "Brand On")

	for id, enabled := range map[string]bool{off: false, on: true} {
		req := withURLParam(newRequest("PATCH", "/api/workspaces/"+id, map[string]any{
			"settings": map[string]any{autoPrecheckKey: enabled},
		}), "id", id)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK)
	}
	for id, want := range map[string]bool{off: false, on: true} {
		req := withURLParam(newRequest("GET", "/api/workspaces/"+id, nil), "id", id)
		var body map[string]any
		testutil.Call(t, testHandler.GetWorkspace, req).Want(http.StatusOK).JSON(&body)
		if got, ok := settingsOf(t, body)[autoPrecheckKey].(bool); !ok || got != want {
			t.Errorf("brand %s reports %v, want %v", id, settingsOf(t, body)[autoPrecheckKey], want)
		}
	}
}

// The key sits in the same blob as the timezone and the endpoint replaces
// settings wholesale, so the two have to survive each other.
func TestUpdateWorkspace_AutoPrecheckAndTimezoneCoexist(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := autoPrecheckWorkspace(t, "handler-tests-precheck-coexist", "Brand Both")

	req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{
		"settings": map[string]any{timezoneKey: "Asia/Tokyo", autoPrecheckKey: false},
	}), "id", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.UpdateWorkspace, req).Want(http.StatusOK).JSON(&body)

	settings := settingsOf(t, body)
	if settings[timezoneKey] != "Asia/Tokyo" {
		t.Errorf("timezone is %v after writing the precheck key", settings[timezoneKey])
	}
	if got, ok := settings[autoPrecheckKey].(bool); !ok || got {
		t.Errorf("precheck is %v after writing the timezone", settings[autoPrecheckKey])
	}
}
