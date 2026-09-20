package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"github.com/multica-ai/multica/server/internal/util"
)

// The workspace delete/write protocol for ip-profile (Issue #104).
//
// content_account_revision carries a text workspace id and no foreign key, so
// nothing in the database stops a revision being written for a workspace that
// is being deleted. Both cases below drive the real fence against the real
// database, because the property being tested is about transaction ordering
// and a fake cannot have an opinion about that.

// Delete commits first: the write finds no workspace row and is refused, and
// nothing is left behind.
func TestAWriteIsRefusedAfterTheWorkspaceIsDeleted(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "fence-deleted-first", "owner")
	accountID := createAccount(t, ws, "zhihu", "先删后写")["account_id"].(string)

	// A real delete of the workspace row, committed before the write starts.
	// Deleting the row is what the fence reads; the rest of DeleteWorkspace's
	// sweep is not what this case is about.
	if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	service := testHandler.contentAccountService()
	_, err := service.SetPersonaPrompt(context.Background(), ws, testUserID, accountID, "写给已删除的空间")
	if !errors.Is(err, ipprofile.ErrWorkspaceGone) {
		t.Fatalf("SetPersonaPrompt got %v, want ErrWorkspaceGone", err)
	}

	_, err = service.SetProfile(context.Background(), ws, testUserID, accountID, ipprofile.ExpressionProfile{})
	if !errors.Is(err, ipprofile.ErrWorkspaceGone) {
		t.Fatalf("SetProfile got %v, want ErrWorkspaceGone", err)
	}

	// The point of the refusal: no orphan row.
	var revisions int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE workspace_id = $1`, ws).Scan(&revisions)
	if revisions != 0 {
		t.Errorf("%d revisions were written for a deleted workspace", revisions)
	}
}

// The write holds the fence first: the delete waits for it, and once both have
// committed the revision is gone with its workspace rather than orphaned.
//
// The two orders are the only ones the protocol permits, and this is the half
// the previous code could not honour: it read the current revision outside any
// transaction, so a delete could commit in the gap.
func TestADeleteWaitsForAnInFlightRevisionWriteAndThenSweepsIt(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "fence-write-first", "owner")
	accountID := createAccount(t, ws, "weibo", "先写后删")["account_id"].(string)
	workspaceUUID, err := util.ParseUUID(ws)
	if err != nil {
		t.Fatalf("parse workspace id: %v", err)
	}

	ctx := context.Background()

	// Hold the fence exactly as a revision write does, and insert inside it.
	writeTx, err := testHandler.TxStarter.Begin(ctx)
	if err != nil {
		t.Fatalf("begin write: %v", err)
	}
	writeDone := false
	defer func() {
		if !writeDone {
			_ = writeTx.Rollback(ctx)
		}
	}()

	if _, err := testHandler.Queries.WithTx(writeTx).LockWorkspaceForContentDiagnosticWrite(ctx, workspaceUUID); err != nil {
		t.Fatalf("take the fence: %v", err)
	}
	if _, err := writeTx.Exec(ctx, `
		INSERT INTO content_account_revision
			(revision_id, account_id, workspace_id, revision, persona_prompt, profile)
		VALUES ($1, $2, $3, 1, $4, '{}'::jsonb)
	`, "fence-revision-"+accountID, accountID, ws, "写入中"); err != nil {
		t.Fatalf("insert revision inside the fence: %v", err)
	}

	// A delete that wants FOR UPDATE on the same row. It must block while the
	// write holds FOR KEY SHARE.
	deleteStarted := make(chan struct{})
	deleteReturned := make(chan error, 1)
	var once sync.Once
	go func() {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		deleteTx, err := testHandler.TxStarter.Begin(deleteCtx)
		if err != nil {
			deleteReturned <- err
			return
		}
		defer func() { _ = deleteTx.Rollback(deleteCtx) }()
		once.Do(func() { close(deleteStarted) })
		// The same lock DeleteWorkspace takes before its sweep.
		if _, err := deleteTx.Exec(deleteCtx, `SELECT id FROM workspace WHERE id = $1 FOR UPDATE`, ws); err != nil {
			deleteReturned <- err
			return
		}
		if _, err := deleteTx.Exec(deleteCtx, `DELETE FROM content_account_revision WHERE workspace_id = $1`, ws); err != nil {
			deleteReturned <- err
			return
		}
		if _, err := deleteTx.Exec(deleteCtx, `DELETE FROM content_account WHERE workspace_id = $1`, ws); err != nil {
			deleteReturned <- err
			return
		}
		if _, err := deleteTx.Exec(deleteCtx, `DELETE FROM workspace WHERE id = $1`, ws); err != nil {
			deleteReturned <- err
			return
		}
		deleteReturned <- deleteTx.Commit(deleteCtx)
	}()

	<-deleteStarted
	// Give the delete long enough to reach the lock and block on it. If the
	// fence did not work, it would finish here instead.
	select {
	case err := <-deleteReturned:
		t.Fatalf("the delete completed while a revision write held the fence: %v", err)
	case <-time.After(750 * time.Millisecond):
	}

	if err := writeTx.Commit(ctx); err != nil {
		t.Fatalf("commit the fenced write: %v", err)
	}
	writeDone = true

	select {
	case err := <-deleteReturned:
		if err != nil {
			t.Fatalf("the delete failed after the write committed: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the delete never completed after the write committed")
	}

	// Committed in that order, the revision is swept with its workspace.
	var orphans int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM content_account_revision WHERE workspace_id = $1`, ws).Scan(&orphans); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d revisions survived the workspace they belong to", orphans)
	}
}

// The fence itself, rather than the service on top of it: an id that is not a
// workspace has no row to hold, and is answered the same way as one that is
// gone.
func TestTheFenceRefusesAnIdThatIsNotAWorkspace(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	fence := contentRevisionFence{h: testHandler}

	err := fence.WithWorkspaceFence(context.Background(), "not-a-uuid", func(ipprofile.RevisionTx) error {
		t.Error("the callback ran for an id that cannot name a workspace")
		return nil
	})
	if !errors.Is(err, ipprofile.ErrWorkspaceGone) {
		t.Errorf("got %v, want ErrWorkspaceGone", err)
	}
}

// A revision write refused because its workspace is gone answers 404, and the
// body is the same one a missing account produces — a refusal must not reveal
// that a workspace once existed.
func TestADeletedWorkspaceAnswersLikeAMissingAccount(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	_ = pgx.ErrNoRows // the fence maps this; asserted through the service above

	ws := accountWorkspace(t, "fence-404-shape", "owner")
	accountID := createAccount(t, ws, "douyin", "删后形状")["account_id"].(string)
	if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	service := testHandler.contentAccountService()
	_, err := service.SetPersonaPrompt(context.Background(), ws, testUserID, accountID, "x")
	if !errors.Is(err, ipprofile.ErrWorkspaceGone) {
		t.Fatalf("got %v, want ErrWorkspaceGone", err)
	}

	// personaWriteError maps it; this is the status the endpoint returns.
	recorder := httptest.NewRecorder()
	testHandler.personaWriteError(recorder, err)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("a deleted workspace answered %d, want 404", recorder.Code)
	}

	// And byte-identical to the body a missing account produces.
	missing := httptest.NewRecorder()
	testHandler.personaWriteError(missing, ipprofile.ErrNotFound)
	if recorder.Body.String() != missing.Body.String() {
		t.Errorf("a deleted workspace is distinguishable from a missing account:\n%s\n%s",
			recorder.Body.String(), missing.Body.String())
	}
}
