package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteSkillImportDuplicateConflictIncludesExistingSkill(t *testing.T) {
	w := httptest.NewRecorder()
	writeSkillImportDuplicateConflict(w, ExistingSkillIdentity{ID: "skill-123", Name: "review-helper"})

	if w.Code != 409 {
		t.Fatalf("status = %d, want 409: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "a skill with this name already exists" {
		t.Fatalf("error = %v", body["error"])
	}
	existing, ok := body["existing_skill"].(map[string]any)
	if !ok {
		t.Fatalf("existing_skill missing or wrong type: %#v", body["existing_skill"])
	}
	if existing["id"] != "skill-123" || existing["name"] != "review-helper" {
		t.Fatalf("existing_skill = %#v", existing)
	}
}

func withMockClawHubImport(t *testing.T, skillName string) string {
	t.Helper()
	slug := "review-helper"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/skills/" + slug:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"skill": map[string]any{
					"slug":        slug,
					"displayName": skillName,
					"summary":     "Imported test skill",
					"tags":        map[string]string{"latest": "1.0.0"},
				},
			})
		case "/api/v1/skills/" + slug + "/versions/1.0.0":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": map[string]any{
					"version": "1.0.0",
					"files": []map[string]any{
						{"path": "SKILL.md", "size": 16},
					},
				},
			})
		case "/api/v1/skills/" + slug + "/file":
			_, _ = w.Write([]byte("# Imported\n"))
		default:
			t.Fatalf("unexpected ClawHub path: %s", r.URL.String())
		}
	}))
	prev := clawHubAPIBase
	clawHubAPIBase = srv.URL + "/api/v1"
	t.Cleanup(func() {
		clawHubAPIBase = prev
		srv.Close()
	})
	return "https://clawhub.ai/acme/" + slug
}
