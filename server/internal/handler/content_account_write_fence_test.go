package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"github.com/multica-ai/multica/server/internal/util"
)

// The account half of the workspace delete/write protocol (Issue #104).
//
// content_account carries a text workspace id and no foreign key, exactly like
// content_account_revision, so creating, patching or re-scoping an account
// outside the fence leaves the same orphan. These cases drive the real fence
// against the real database: the property is about transaction ordering, and a
// fake cannot have an opinion about that.

// Delete commits first: every account write finds no workspace row, is refused,
// and leaves nothing behind.
func TestAccountWritesAreRefusedAfterTheWorkspaceIsDeleted(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "account-fence-deleted-first", "owner")
	created := createAccount(t, ws, "zhihu", "先删后写")
	accountID := created["account_id"].(string)

	if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, ws); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	service := testHandler.contentAccountService()
	display := "改名"
	for _, write := range []struct {
		name string
		call func() error
	}{
		{"create", func() error {
			_, err := service.Create(context.Background(), ws, testUserID, "weibo", "新号", nil)
			return err
		}},
		{"update", func() error {
			_, err := service.Update(context.Background(), ws, testUserID, accountID,
				ipprofile.Patch{DisplayName: &display})
			return err
		}},
		{"set scope", func() error {
			_, err := service.SetScope(context.Background(), ws, testUserID, accountID,
				string(ipprofile.ScopeWeb))
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, ipprofile.ErrWorkspaceGone) {
				t.Fatalf("%s got %v, want ErrWorkspaceGone", write.name, err)
			}
		})
	}

	// The point of the refusal: the deleted workspace gained no account, and
	// the account row the delete left behind was not edited either.
	var accounts int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account WHERE workspace_id = $1`, ws).Scan(&accounts)
	if accounts != 1 {
		t.Errorf("content_account holds %d rows for the deleted workspace, want the 1 the delete left", accounts)
	}
	var name, settings string
	dbfx.QueryRow(t, `SELECT display_name, settings::text FROM content_account WHERE account_id = $1`,
		accountID).Scan(&name, &settings)
	if name != "先删后写" {
		t.Errorf("display name = %q, want the value from before the delete", name)
	}
	if settings != "{}" {
		t.Errorf("settings = %s, want {} — the refused scope write must not have landed", settings)
	}
}

// The write holds the fence first: the delete waits for it, and once both have
// committed the account is gone with its workspace rather than orphaned.
func TestADeleteWaitsForAnInFlightAccountWriteAndThenSweepsIt(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "account-fence-write-first", "owner")
	workspaceUUID, err := util.ParseUUID(ws)
	if err != nil {
		t.Fatalf("parse workspace id: %v", err)
	}
	ctx := context.Background()

	// Hold the fence exactly as an account write does, and insert inside it.
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
		INSERT INTO content_account (account_id, workspace_id, platform, display_name, settings)
		VALUES ($1, $2, 'zhihu', '写入中', '{}'::jsonb)`, "account-fence-inflight-"+ws, ws); err != nil {
		t.Fatalf("insert account inside the fence: %v", err)
	}

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
		for _, statement := range []string{
			`DELETE FROM content_account_revision WHERE workspace_id = $1`,
			`DELETE FROM content_account WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			if _, err := deleteTx.Exec(deleteCtx, statement, ws); err != nil {
				deleteReturned <- err
				return
			}
		}
		deleteReturned <- deleteTx.Commit(deleteCtx)
	}()

	<-deleteStarted
	// Long enough for the delete to reach the lock and block on it. Without the
	// fence it would finish here instead, and the account inserted above would
	// outlive its workspace.
	select {
	case err := <-deleteReturned:
		t.Fatalf("the delete completed while an account write held the fence: %v", err)
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

	var orphans int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM content_account WHERE workspace_id = $1`, ws).Scan(&orphans); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d accounts survived the workspace they belong to", orphans)
	}
}

// An account write refused because its workspace is gone answers 404, and the
// body is the one a missing account produces.
func TestADeletedWorkspaceAnswersLikeAMissingAccountOnAccountWrites(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	gone := httptest.NewRecorder()
	testHandler.accountWriteError(gone, ipprofile.ErrWorkspaceGone)
	if gone.Code != http.StatusNotFound {
		t.Errorf("a deleted workspace answered %d, want 404", gone.Code)
	}
	missing := httptest.NewRecorder()
	testHandler.accountWriteError(missing, ipprofile.ErrNotFound)
	if gone.Body.String() != missing.Body.String() {
		t.Errorf("a deleted workspace is distinguishable from a missing account:\n%s\n%s",
			gone.Body.String(), missing.Body.String())
	}
}

// The account fence itself: an id that cannot name a workspace has no row to
// hold, and is answered like one that is gone.
func TestTheAccountFenceRefusesAnIdThatIsNotAWorkspace(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	fence := contentRevisionFence{h: testHandler}
	err := fence.WithAccountFence(context.Background(), "not-a-uuid", func(ipprofile.AccountTx) error {
		t.Error("the callback ran for an id that cannot name a workspace")
		return nil
	})
	if !errors.Is(err, ipprofile.ErrWorkspaceGone) {
		t.Errorf("got %v, want ErrWorkspaceGone", err)
	}
}
