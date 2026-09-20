package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type runtimeLocalSkillPendingWorkRecorder struct {
	hints []string
}

func (r *runtimeLocalSkillPendingWorkRecorder) NotifyPendingWork(runtimeID, kind string) {
	r.hints = append(r.hints, runtimeID+":"+kind)
}

func TestInMemoryLocalSkillListStore_PreservesSummaries(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryLocalSkillListStore()
	req, err := store.Create(ctx, "runtime-xyz")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	body := map[string]any{
		"status":    "completed",
		"supported": true,
		"skills": []map[string]any{
			{
				"key":         "paper-desktop:review-helper",
				"name":        "paper-desktop:review-helper",
				"description": "Review PRs",
				"source_path": "~/.claude/plugins/cache/paper/skills/review-helper",
				"provider":    "claude",
				"root":        "plugin",
				"plugin":      "paper-desktop@paper",
				"can_disable": true,
				"file_count":  2,
			},
		},
	}
	raw, _ := json.Marshal(body)

	var parsed struct {
		Skills []RuntimeLocalSkillSummary `json:"skills"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal report body: %v", err)
	}

	if err := store.Complete(ctx, req.ID, parsed.Skills, true, nil, false); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := store.Get(ctx, req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected stored result")
	}
	if len(got.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(got.Skills))
	}
	if got.Skills[0].SourcePath != "~/.claude/plugins/cache/paper/skills/review-helper" {
		t.Fatalf("source_path = %q", got.Skills[0].SourcePath)
	}
	if got.Skills[0].Root != "plugin" || got.Skills[0].Plugin != "paper-desktop@paper" {
		t.Fatalf("plugin origin = %#v", got.Skills[0])
	}
	if got.Skills[0].FileCount != 2 {
		t.Fatalf("file_count = %d", got.Skills[0].FileCount)
	}
	if !got.Skills[0].CanDisable {
		t.Fatal("can_disable capability was not preserved")
	}
}

func TestInMemoryLocalSkillListStore_TimesOutRunningRequests(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryLocalSkillListStore()
	req, err := store.Create(ctx, "runtime-xyz")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req.Status = RuntimeLocalSkillRunning
	startedAt := time.Now().Add(-61 * time.Second)
	req.RunStartedAt = &startedAt

	got, err := store.Get(ctx, req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected stored request")
	}
	if got.Status != RuntimeLocalSkillTimeout {
		t.Fatalf("expected timeout, got %s", got.Status)
	}
	if got.Error == "" {
		t.Fatal("expected timeout error")
	}
}

func TestInMemoryLocalSkillImportStore_TimesOutRunningRequests(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryLocalSkillImportStore()
	req, err := store.Create(ctx, LocalSkillImportRequestInput{
		RuntimeID: "runtime-xyz",
		CreatorID: "user-1",
		SkillKey:  "review-helper",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req.Status = RuntimeLocalSkillRunning
	startedAt := time.Now().Add(-61 * time.Second)
	req.RunStartedAt = &startedAt

	got, err := store.Get(ctx, req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected stored request")
	}
	if got.Status != RuntimeLocalSkillTimeout {
		t.Fatalf("expected timeout, got %s", got.Status)
	}
	if got.Error == "" {
		t.Fatal("expected timeout error")
	}
}

func TestCleanOptionalString(t *testing.T) {
	if got := cleanOptionalString(nil); got != nil {
		t.Fatalf("expected nil, got %q", *got)
	}

	raw := "  "
	if got := cleanOptionalString(&raw); got != nil {
		t.Fatalf("expected nil for whitespace-only value, got %q", *got)
	}

	value := "  Review Helper  "
	got := cleanOptionalString(&value)
	if got == nil || *got != "Review Helper" {
		t.Fatalf("expected trimmed value, got %#v", got)
	}
}

func ptr[T any](value T) *T {
	return &value
}
