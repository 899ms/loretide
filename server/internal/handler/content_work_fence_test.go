package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
)

// The work editor's write paths follow the workspace delete/write protocol the
// diagnostics module, topic planning and chat session creation follow: the
// write transaction takes LockWorkspaceForContentDiagnosticWrite before it
// touches a row, so a delete either waits for the write and sweeps it, or
// commits first and leaves the write with no workspace to attach to.
//
// content_work, content_artifact and content_artifact_version carry a text
// workspace id and no foreign key, so nothing else would stop an orphan from
// being written after the delete commits (Issue #104).
//
// This is also what the module's own fixture stands in for: it runs in an
// isolated schema with no workspace table, so the fence is proven here.
func TestWorkEditorWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	store := h.workEditorStore()

	slug := fmt.Sprintf("work-fence-%d", time.Now().UnixNano())
	var workspaceID string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`,
		slug, slug).Scan(&workspaceID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_artifact_version WHERE workspace_id = $1`,
			`DELETE FROM content_artifact WHERE workspace_id = $1`,
			`DELETE FROM content_work WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, workspaceID)
		}
	})

	work, err := store.CreateWork(ctx, workspaceID, testUserID, workeditor.Work{
		TopicCardID: "card-fence", Title: "手写稿",
	})
	if err != nil {
		t.Fatalf("create work while the workspace exists: %v", err)
	}
	artifact, err := store.CreateArtifact(ctx, workspaceID, testUserID, work.WorkID, workeditor.Artifact{
		Kind: workeditor.KindBody, Title: "正文", Position: 1,
	})
	if err != nil {
		t.Fatalf("create artifact while the workspace exists: %v", err)
	}
	body := "写了一点"
	if _, err = store.PatchArtifact(ctx, workspaceID, testUserID, work.WorkID, artifact.ArtifactID,
		workeditor.ArtifactPatch{DraftBody: &body}); err != nil {
		t.Fatalf("autosave while the workspace exists: %v", err)
	}
	version, err := store.SaveVersion(ctx, workspaceID, testUserID, work.WorkID, artifact.ArtifactID)
	if err != nil {
		t.Fatalf("save version while the workspace exists: %v", err)
	}

	count := func(table string) int {
		var total int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}
	worksBefore, artifactsBefore, versionsBefore := count("content_work"),
		count("content_artifact"), count("content_artifact_version")
	if worksBefore != 1 || artifactsBefore != 1 || versionsBefore != 1 {
		t.Fatalf("setup wrote %d/%d/%d rows, want 1/1/1", worksBefore, artifactsBefore, versionsBefore)
	}

	// The delete commits on its own connection, exactly as a workspace deletion
	// that finished just before the next request arrived.
	if _, err = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	for _, write := range []struct {
		name string
		call func() error
	}{
		{"create-work", func() error {
			_, err := store.CreateWork(ctx, workspaceID, testUserID, workeditor.Work{
				TopicCardID: "card-fence", Title: "第二篇",
			})
			return err
		}},
		{"rename-work", func() error {
			_, err := store.RenameWork(ctx, workspaceID, testUserID, work.WorkID, "改名")
			return err
		}},
		{"create-artifact", func() error {
			_, err := store.CreateArtifact(ctx, workspaceID, testUserID, work.WorkID, workeditor.Artifact{
				Kind: workeditor.KindChannelDraft, Title: "渠道稿", Position: 2,
			})
			return err
		}},
		{"autosave", func() error {
			later := "又写了一点"
			_, err := store.PatchArtifact(ctx, workspaceID, testUserID, work.WorkID,
				artifact.ArtifactID, workeditor.ArtifactPatch{DraftBody: &later})
			return err
		}},
		{"save-version", func() error {
			_, err := store.SaveVersion(ctx, workspaceID, testUserID, work.WorkID, artifact.ArtifactID)
			return err
		}},
		{"restore-version", func() error {
			_, err := store.RestoreVersion(ctx, workspaceID, testUserID, work.WorkID,
				artifact.ArtifactID, version.VersionID)
			return err
		}},
		{"adopt-version", func() error {
			_, err := store.AdoptVersion(ctx, workspaceID, testUserID, work.WorkID,
				artifact.ArtifactID, version.VersionID)
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, workeditor.ErrNotFound) {
				t.Fatalf("%s after the delete committed = %v, want ErrNotFound", write.name, err)
			}
		})
	}

	// Nothing landed. A single extra row here is an orphan nothing will ever
	// clean up, because the workspace it belonged to is gone.
	if got := count("content_work"); got != worksBefore {
		t.Errorf("content_work has %d rows, want %d", got, worksBefore)
	}
	if got := count("content_artifact"); got != artifactsBefore {
		t.Errorf("content_artifact has %d rows, want %d", got, artifactsBefore)
	}
	if got := count("content_artifact_version"); got != versionsBefore {
		t.Errorf("content_artifact_version has %d rows, want %d", got, versionsBefore)
	}
}
