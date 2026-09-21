package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
)

// The topic write paths share the workspace delete/write protocol the
// diagnostics module and chat session creation follow: the write transaction
// takes LockWorkspaceForContentDiagnosticWrite before it touches a row, so a
// delete either waits for the write and sweeps it, or commits first and leaves
// the write with no workspace to attach to. content_topic_card and
// content_brief_revision carry a text workspace id and no foreign key, so
// nothing else would stop an orphan row from being written after the delete
// commits (Issue #104).
func TestContentTopicWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	store := h.topicPlanningStore()

	slug := fmt.Sprintf("topic-fence-%d", time.Now().UnixNano())
	var workspaceID string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`,
		slug, slug).Scan(&workspaceID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_brief_revision WHERE workspace_id = $1`,
			`DELETE FROM content_topic_card WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, workspaceID)
		}
	})

	card := topicplanning.TopicCard{
		WorkspaceID:               workspaceID,
		AudienceProblemJudgment:   "audience/problem/judgment",
		IPFit:                     "fit",
		Timing:                    "没有时效依据",
		ExistingContentRelation:   "没有",
		EvidenceGapsAndInvestment: "没有现成证据",
		Channels:                  []string{"zhihu"},
		RecommendedAction:         "开始",
	}
	created, err := store.Create(ctx, testUserID, card)
	if err != nil {
		t.Fatalf("create while the workspace exists: %v", err)
	}
	if _, err = store.Act(ctx, workspaceID, testUserID, created.TopicCardID,
		topicplanning.ActionRequest{Action: topicplanning.ActionStart}); err != nil {
		t.Fatalf("start while the workspace exists: %v", err)
	}

	count := func(table string) int {
		var total int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}
	cardsBefore, briefsBefore, auditBefore := count("content_topic_card"),
		count("content_brief_revision"), count("content_operation_audit")
	if cardsBefore != 1 || briefsBefore != 1 {
		t.Fatalf("setup wrote %d cards and %d briefs, want 1 and 1", cardsBefore, briefsBefore)
	}

	// The delete commits on its own connection, exactly as a workspace deletion
	// that finished just before the next request arrives.
	if _, err = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	for _, write := range []struct {
		name string
		call func() error
	}{
		{"create", func() error {
			_, err := store.Create(ctx, testUserID, card)
			return err
		}},
		{"start", func() error {
			_, err := store.Act(ctx, workspaceID, testUserID, created.TopicCardID,
				topicplanning.ActionRequest{Action: topicplanning.ActionStart})
			return err
		}},
		{"save", func() error {
			_, err := store.Act(ctx, workspaceID, testUserID, created.TopicCardID,
				topicplanning.ActionRequest{Action: topicplanning.ActionSave})
			return err
		}},
		{"defer", func() error {
			_, err := store.Act(ctx, workspaceID, testUserID, created.TopicCardID,
				topicplanning.ActionRequest{Action: topicplanning.ActionDefer, Reason: "等素材"})
			return err
		}},
		{"drop", func() error {
			_, err := store.Act(ctx, workspaceID, testUserID, created.TopicCardID,
				topicplanning.ActionRequest{Action: topicplanning.ActionDrop, Reason: "重复"})
			return err
		}},
		{"append-brief", func() error {
			_, err := store.AppendBrief(ctx, workspaceID, testUserID, created.TopicCardID,
				topicplanning.BriefRevision{Audience: "audience"})
			return err
		}},
		{"set-account", func() error {
			_, err := store.SetAccount(ctx, workspaceID, testUserID, created.TopicCardID, nil)
			return err
		}},
		{"set-sources", func() error {
			_, err := store.SetSources(ctx, workspaceID, testUserID, created.TopicCardID, nil, nil)
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, topicplanning.ErrNotFound) {
				t.Fatalf("%s after the delete committed = %v, want ErrNotFound", write.name, err)
			}
		})
	}

	// Refused is not enough on its own: the point of the fence is that nothing
	// reached the tables, audit rows included.
	if got := count("content_topic_card"); got != cardsBefore {
		t.Errorf("content_topic_card = %d after the refused writes, want %d", got, cardsBefore)
	}
	if got := count("content_brief_revision"); got != briefsBefore {
		t.Errorf("content_brief_revision = %d after the refused writes, want %d", got, briefsBefore)
	}
	if got := count("content_operation_audit"); got != auditBefore {
		t.Errorf("content_operation_audit = %d after the refused writes, want %d", got, auditBefore)
	}
}
