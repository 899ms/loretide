//go:build dbtest

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newSkillArchiveImportRequest(userID string, archive []byte, filename, onConflict string) *http.Request {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(archive)
	if onConflict != "" {
		_ = writer.WriteField("on_conflict", onConflict)
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/skills/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	return req
}

func TestImportSkill_ArchiveUploadCreatesSkill(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	name := "archive-create-" + t.Name()
	archive := buildTestZip(t, map[string]string{
		name + "/SKILL.md":       skillMdWithName(name, "From archive"),
		name + "/scripts/run.sh": "echo hi",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM skill WHERE workspace_id = $1 AND name = $2`, testWorkspaceID, name)
	})

	w := httptest.NewRecorder()
	testHandler.ImportSkill(w, newSkillArchiveImportRequest(testUserID, archive, name+".skill", "fail"))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	var body SkillImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "created" || body.Skill == nil {
		t.Fatalf("body = %#v", body)
	}
	if body.Skill.Name != name {
		t.Errorf("name = %q, want %q", body.Skill.Name, name)
	}
	found := false
	for _, f := range body.Skill.Files {
		if f.Path == "scripts/run.sh" {
			found = true
		}
	}
	if !found {
		t.Errorf("scripts/run.sh missing from imported files: %#v", body.Skill.Files)
	}
}

func TestImportSkill_ArchiveUploadConflictSkip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	namePrefix := "archive-skip"
	skillName := namePrefix + "-" + t.Name()
	existingID := insertHandlerTestSkill(t, namePrefix, "# Existing")
	archive := buildTestZip(t, map[string]string{
		skillName + "/SKILL.md": skillMdWithName(skillName, "From archive"),
	})

	w := httptest.NewRecorder()
	testHandler.ImportSkill(w, newSkillArchiveImportRequest(testUserID, archive, skillName+".skill", "skip"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body SkillImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", body.Status)
	}
	if body.ExistingSkill == nil || body.ExistingSkill.ID != existingID {
		t.Fatalf("existing_skill = %#v", body.ExistingSkill)
	}
}

func TestImportSkill_ArchiveUploadRejectsNonZip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("handler test DB not configured")
	}
	w := httptest.NewRecorder()
	testHandler.ImportSkill(w, newSkillArchiveImportRequest(testUserID, []byte("not a zip at all"), "bad.skill", "fail"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}
