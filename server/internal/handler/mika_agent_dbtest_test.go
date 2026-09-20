//go:build dbtest

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
)

func createMika(t *testing.T, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := withChatTestWorkspaceCtx(t, newRequest("POST", "/api/agents/mika", body))
	w := httptest.NewRecorder()
	testHandler.CreateMikaAgent(w, req)
	return w
}

func cleanupMika(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		testPool.Exec(context.Background(),
			`DELETE FROM agent WHERE workspace_id = $1 AND system_key = $2`,
			testWorkspaceID, service.MikaSystemKey)
	})
}

// TestCreateMikaAgent_ServerOwnsTheDefinition is the point of moving creation
// server-side: the caller sends only a runtime and a language, and everything
// that makes Mika Mika is decided here.
func TestCreateMikaAgent_ServerOwnsTheDefinition(t *testing.T) {
	cleanupMika(t)

	w := createMika(t, map[string]any{
		"runtime_id": handlerTestRuntimeID(t),
		"language":   "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeAgent(t, w)

	if resp.SystemKey != service.MikaSystemKey {
		t.Fatalf("system_key = %q, want %q", resp.SystemKey, service.MikaSystemKey)
	}
	if resp.Name != service.MikaDefaultName {
		t.Fatalf("name = %q, want %q", resp.Name, service.MikaDefaultName)
	}
	if resp.PermissionMode != mikaAgentPermissionMode {
		t.Fatalf("permission_mode = %q, want %q", resp.PermissionMode, mikaAgentPermissionMode)
	}
	// The workspace half starts empty — the product half is never written to
	// the row, which is what keeps a release from overwriting workspace notes.
	if resp.Instructions != "" {
		t.Fatalf("instructions must start empty, got %q", resp.Instructions)
	}
	if !strings.Contains(resp.SystemInstructions, "You are Mika") {
		t.Fatalf("system_instructions should carry the product prompt, got %q", resp.SystemInstructions)
	}

	// kind stays 'user' so Mika keeps appearing in agent lists and assignment
	// surfaces, and survives runtime teardown.
	var kind string
	if err := testPool.QueryRow(context.Background(),
		`SELECT kind FROM agent WHERE id = $1`, resp.ID).Scan(&kind); err != nil {
		t.Fatalf("load agent kind: %v", err)
	}
	if kind != "user" {
		t.Fatalf("kind = %q, want \"user\" — 'system' hides the row and deletes it with its runtime", kind)
	}
}

func TestCreateMikaAgent_IsIdempotentPerWorkspace(t *testing.T) {
	cleanupMika(t)
	runtimeID := handlerTestRuntimeID(t)

	first := createMika(t, map[string]any{"runtime_id": runtimeID, "language": "en"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first call: expected 201, got %d: %s", first.Code, first.Body.String())
	}
	second := createMika(t, map[string]any{"runtime_id": runtimeID, "language": "zh"})
	if second.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d: %s", second.Code, second.Body.String())
	}
	if a, b := decodeAgent(t, first).ID, decodeAgent(t, second).ID; a != b {
		t.Fatalf("expected the same agent back, got %s then %s", a, b)
	}

	var count int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent WHERE workspace_id = $1 AND system_key = $2`,
		testWorkspaceID, service.MikaSystemKey).Scan(&count); err != nil {
		t.Fatalf("count mika agents: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 Mika in the workspace, got %d", count)
	}
}

func TestCreateMikaAgent_RejectsUnsupportedLanguage(t *testing.T) {
	cleanupMika(t)
	w := createMika(t, map[string]any{"runtime_id": handlerTestRuntimeID(t), "language": "fr"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestArchiveMikaIsRejected: archiving would hide the workspace's entry point
// while leaving the row in place, which also strands the bootstrap endpoint —
// its lookup skips archived rows but the unique index does not.
func TestArchiveMikaIsRejected(t *testing.T) {
	cleanupMika(t)
	w := createMika(t, map[string]any{
		"runtime_id": handlerTestRuntimeID(t),
		"language":   "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	agentID := decodeAgent(t, w).ID

	req := withChatTestWorkspaceCtx(t, withURLParam(
		newRequest("POST", "/api/agents/"+agentID+"/archive", nil), "id", agentID,
	))
	archiveW := httptest.NewRecorder()
	testHandler.ArchiveAgent(archiveW, req)
	if archiveW.Code != http.StatusBadRequest {
		t.Fatalf("archiving a system agent: expected 400, got %d: %s", archiveW.Code, archiveW.Body.String())
	}

	var archivedAt *string
	if err := testPool.QueryRow(context.Background(),
		`SELECT archived_at::text FROM agent WHERE id = $1`, agentID).Scan(&archivedAt); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if archivedAt != nil {
		t.Fatalf("agent must remain active, got archived_at = %v", *archivedAt)
	}
}
