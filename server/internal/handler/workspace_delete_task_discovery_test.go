package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"
)

// workspaceDeletePathFixture builds the ownership shapes teardown has to handle:
// one task reachable from the victim workspace only through agent_id, one only
// through issue_id, one only through runtime_id, and one that belongs entirely to
// a neighbour workspace and must survive.
//
// The tokens cover the three explicit task_token paths: one owned by the victim
// workspace, one whose workspace_id points at the neighbour while its TASK is the
// victim's, one whose workspace_id points at the neighbour while its AGENT is the
// victim's, and one that is entirely the neighbour's.
type workspaceDeletePathFixture struct {
	victimID    string
	neighbourID string

	victimAgent   string
	victimIssue   string
	victimRuntime string

	neighbourAgent   string
	neighbourIssue   string
	neighbourRuntime string

	taskViaAgent   string
	taskViaIssue   string
	taskViaRuntime string
	neighbourTask  string

	victimToken        string
	crossTaskToken     string
	crossAgentToken    string
	neighbourOnlyToken string
}

// TestSweepTasksForOwner_FailsClosedWhenTasksKeepArriving pins the behaviour that
// makes the sweep safe without an application-wide enqueue protocol: if tasks keep
// appearing for an owner that has already been swept, teardown refuses to commit
// instead of leaving a half-cleaned tenant behind.
//
// The page function stands in for a broken fence — it keeps producing a task the
// sweep cannot get rid of.
func TestSweepTasksForOwner_FailsClosedWhenTasksKeepArriving(t *testing.T) {
	ctx := context.Background()
	owner := parseUUID("11111111-2222-3333-4444-555555555555")
	stubTask := parseUUID("66666666-7777-8888-9999-aaaaaaaaaaaa")

	// Faithful model of a broken fence: each pass finds one page of tasks and
	// then reports the owner clean, so the next pass starts from the beginning
	// and finds another one. Honours the page query's `id > cursor` contract, so
	// the only thing that can stop this is the pass cap.
	pages := 0
	firstPage := func(_ context.Context, _ pgtype.UUID, _ int32) ([]pgtype.UUID, error) {
		pages++
		if pages > 100 {
			t.Fatal("sweep did not stop; the pass cap is not working")
		}
		return []pgtype.UUID{stubTask}, nil
	}
	nextPage := func(context.Context, pgtype.UUID, pgtype.UUID, int32) ([]pgtype.UUID, error) {
		return nil, nil
	}
	noop := func(context.Context, []pgtype.UUID) error { return nil }

	err := sweepTasksForOwner(ctx, "agent", owner, firstPage, nextPage, noop, noop)
	if err == nil {
		t.Fatal("sweep succeeded while tasks kept arriving; teardown would commit a partial cleanup")
	}
	if !strings.Contains(err.Error(), "refusing to commit a partial teardown") {
		t.Errorf("error = %v, want a fail-closed partial-teardown error", err)
	}
}
