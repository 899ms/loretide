//go:build dbtest

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExistingSkillIdentityByNameReturnsIDAndName(t *testing.T) {
	namePrefix := "duplicate-import-identity"
	name := namePrefix + "-" + t.Name()
	skillID := insertHandlerTestSkill(t, namePrefix, "# Duplicate import identity")

	existing, ok, err := testHandler.existingSkillIdentityByName(context.Background(), parseUUID(testWorkspaceID), name)
	if err != nil {
		t.Fatalf("existingSkillIdentityByName: %v", err)
	}
	if !ok {
		t.Fatal("expected existing skill identity to be found")
	}
	if existing.ID != skillID || existing.Name != name {
		t.Fatalf("existing skill = %#v, want id %s name %s", existing, skillID, name)
	}
}

func TestImportSkillOnConflictSkipReturnsStructuredResult(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	namePrefix := "url-import-skip"
	skillName := namePrefix + "-" + t.Name()
	existingID := insertHandlerTestSkill(t, namePrefix, "# Existing")
	importURL := withMockClawHubImport(t, skillName)

	w := httptest.NewRecorder()
	req := newRequestAsUser(testUserID, http.MethodPost, "/api/skills/import", map[string]any{
		"url":         importURL,
		"on_conflict": "skip",
	})
	testHandler.ImportSkill(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body SkillImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "skipped" {
		t.Fatalf("status = %q", body.Status)
	}
	if body.ExistingSkill == nil || body.ExistingSkill.ID != existingID || body.ExistingSkill.Name != skillName {
		t.Fatalf("existing_skill = %#v", body.ExistingSkill)
	}
}

func TestImportSkillOnConflictRenameCreatesSuffixedSkill(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	namePrefix := "url-import-rename"
	skillName := namePrefix + "-" + t.Name()
	insertHandlerTestSkill(t, namePrefix, "# Existing")
	importURL := withMockClawHubImport(t, skillName)

	w := httptest.NewRecorder()
	req := newRequestAsUser(testUserID, http.MethodPost, "/api/skills/import", map[string]any{
		"url":         importURL,
		"on_conflict": "rename",
	})
	testHandler.ImportSkill(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	var body SkillImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "created" || body.Skill == nil {
		t.Fatalf("body = %#v", body)
	}
	if body.Skill.Name != skillName+"-2" {
		t.Fatalf("created skill name = %q, want %q", body.Skill.Name, skillName+"-2")
	}
}
