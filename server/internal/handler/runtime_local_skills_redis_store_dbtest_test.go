//go:build dbtest

package handler

import (
	"context"
	"testing"
	"time"
)

func TestRedisLocalSkillImportStore_CompletePreservesFiles(t *testing.T) {
	rdb := newRedisTestClient(t)
	ctx := context.Background()
	store := NewRedisLocalSkillImportStore(rdb)

	req, err := store.Create(ctx, LocalSkillImportRequestInput{
		RuntimeID: "runtime-1",
		CreatorID: "user-1",
		SkillKey:  "review-helper",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	completedAt := time.Now().UTC().Format(time.RFC3339)
	skill := SkillWithFilesResponse{
		SkillResponse: SkillResponse{
			ID:          "skill-1",
			WorkspaceID: testWorkspaceID,
			Name:        "Review Helper",
			Description: "Review PRs",
			Content:     "# Review Helper",
			CreatedAt:   completedAt,
			UpdatedAt:   completedAt,
		},
		Files: []SkillFileResponse{
			{
				ID:        "file-1",
				SkillID:   "skill-1",
				Path:      "rules.md",
				Content:   "Use the review checklist.",
				CreatedAt: completedAt,
				UpdatedAt: completedAt,
			},
		},
	}
	if err := store.Complete(ctx, req.ID, skill); err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, err := store.Get(ctx, req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != RuntimeLocalSkillCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	if got.Skill == nil {
		t.Fatal("completed skill lost round trip")
	}
	if len(got.Skill.Files) != 1 {
		t.Fatalf("files lost round trip: %+v", got.Skill.Files)
	}
	if got.Skill.Files[0].Path != "rules.md" || got.Skill.Files[0].Content != "Use the review checklist." {
		t.Fatalf("file corrupted round trip: %+v", got.Skill.Files[0])
	}
}
