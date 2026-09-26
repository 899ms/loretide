package workeditor

import (
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/idempotency"
)

// ApplyBody is a real PostgreSQL contract. It is deliberately skipped unless
// the isolated work-editor database is configured; local development must not
// select or contact a database implicitly.
func TestApplyBodyAppendsOneSuggestionVersionAndReplays(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)
	baseBody := "旧的正文"
	typeInto(t, fx, workID, artifactID, baseBody)
	base, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	request, err := idempotency.NewRequest("apply-search-suggestion", artifactID, "suggestion-1", struct {
		BaseVersionID string `json:"base_version_id"`
		Body          string `json:"body"`
	}{base.VersionID, "新的正文"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: base.VersionID, Body: "新的正文"}, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionID == base.VersionID || first.Revision != base.Revision+1 || first.Source != SourceEdited ||
		first.Action != ActionSuggestionApplied || first.Body != "新的正文" {
		t.Fatalf("applied version = %+v", first)
	}
	artifact, err := fx.store.GetArtifact(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil || artifact.DraftBody != first.Body || artifact.DraftStatus != DraftSaved {
		t.Fatalf("artifact after apply = %+v, %v", artifact, err)
	}
	replay, err := fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: base.VersionID, Body: "新的正文"}, request)
	if err != nil || replay.VersionID != first.VersionID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	versions, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions = %d, %v; want base and exactly one applied version", len(versions), err)
	}
	different, err := idempotency.NewRequest("apply-search-suggestion", artifactID, "suggestion-1", struct {
		BaseVersionID string `json:"base_version_id"`
		Body          string `json:"body"`
	}{base.VersionID, "另一份正文"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: base.VersionID, Body: "另一份正文"}, different)
	if !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("different request with same key = %v, want ErrConflict", err)
	}
}

func TestApplyBodyRejectsMovedBaseUnsavedDraftAndNoChange(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)
	typeInto(t, fx, workID, artifactID, "正文")
	base, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: "not-the-latest", Body: "新正文"}); !errors.Is(err, ErrBaseMoved) {
		t.Fatalf("moved base = %v, want ErrBaseMoved", err)
	}
	typeInto(t, fx, workID, artifactID, "未保存的正文")
	if _, err = fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: base.VersionID, Body: "新正文"}); !errors.Is(err, ErrDraftUnsaved) {
		t.Fatalf("working draft = %v, want ErrDraftUnsaved", err)
	}
	typeInto(t, fx, workID, artifactID, base.Body)
	if _, err = fx.store.ApplyBody(ctx, testWorkspace, testActor, workID, artifactID,
		BodyApplication{BaseVersionID: base.VersionID, Body: base.Body}); !errors.Is(err, ErrNoChange) {
		t.Fatalf("unchanged body = %v, want ErrNoChange", err)
	}
}
