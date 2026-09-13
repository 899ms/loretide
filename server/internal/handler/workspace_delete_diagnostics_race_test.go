package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

type blockingContentDiagnosticsGuard struct {
	inner   diagnostics.WorkspaceWriteGuard
	locked  chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *blockingContentDiagnosticsGuard) LockForContentDiagnosticWrite(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	if err := g.inner.LockForContentDiagnosticWrite(ctx, tx, workspaceID); err != nil {
		return err
	}
	g.once.Do(func() { close(g.locked) })
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type observingContentDiagnosticsGuard struct {
	inner diagnostics.WorkspaceWriteGuard
	pid   chan int32
	once  sync.Once
}

func (g *observingContentDiagnosticsGuard) LockForContentDiagnosticWrite(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	var pid int32
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		return diagnostics.ErrUnavailable
	}
	g.once.Do(func() { g.pid <- pid })
	return g.inner.LockForContentDiagnosticWrite(ctx, tx, workspaceID)
}

type contentDiagnosticsRaceFixture struct {
	workspaceID string
	neighbourID string
}

func seedContentDiagnosticsRaceFixture(t *testing.T, name string) contentDiagnosticsRaceFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	createWorkspace := func(slug string) string {
		var id string
		if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`, slug, slug).Scan(&id); err != nil {
			t.Fatalf("create workspace %s: %v", slug, err)
		}
		return id
	}
	f := contentDiagnosticsRaceFixture{
		workspaceID: createWorkspace("handler-content-diag-race-target-" + suffix),
		neighbourID: createWorkspace("handler-content-diag-race-neighbour-" + suffix),
	}
	t.Cleanup(func() {
		for _, workspaceID := range []string{f.workspaceID, f.neighbourID} {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_technical_log WHERE workspace_id = $1`, workspaceID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_operation_audit WHERE workspace_id = $1`, workspaceID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM content_diagnostic_run WHERE workspace_id = $1`, workspaceID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID)
		}
	})
	if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, f.workspaceID, testUserID); err != nil {
		t.Fatalf("add target owner: %v", err)
	}
	seedContentDiagnosticRows(t, f.neighbourID, "neighbour-"+suffix)
	return f
}

func seedContentDiagnosticRows(t *testing.T, workspaceID, suffix string) {
	t.Helper()
	ctx := context.Background()
	for _, entry := range []struct {
		table string
		key   string
		id    string
	}{
		{table: "content_diagnostic_run", key: "run_id", id: "run-" + suffix},
		{table: "content_operation_audit", key: "event_id", id: "audit-" + suffix},
		{table: "content_technical_log", key: "event_id", id: "log-" + suffix},
	} {
		if _, err := testPool.Exec(ctx, `INSERT INTO `+entry.table+` (`+entry.key+`, workspace_id, payload) VALUES ($1, $2, '{}'::jsonb)`, entry.id, workspaceID); err != nil {
			t.Fatalf("seed %s for %s: %v", entry.table, workspaceID, err)
		}
	}
}

func assertContentDiagnosticRows(t *testing.T, workspaceID string, want int) {
	t.Helper()
	for _, table := range []string{"content_diagnostic_run", "content_operation_audit", "content_technical_log"} {
		var got int
		if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&got); err != nil {
			t.Fatalf("count %s for %s: %v", table, workspaceID, err)
		}
		if got != want {
			t.Fatalf("%s workspace %s count=%d, want %d", table, workspaceID, got, want)
		}
	}
}

func startContentDiagnosticWorkspaceDelete(workspaceID string) (<-chan int, <-chan struct{}) {
	status := make(chan int, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		response := httptest.NewRecorder()
		request := withURLParam(newRequest(http.MethodDelete, "/api/workspaces/"+workspaceID, nil), "id", workspaceID)
		testHandler.DeleteWorkspace(response, request)
		status <- response.Code
	}()
	return status, done
}

func awaitContentDiagnosticSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for %s", description)
	}
}

func awaitContentDiagnosticPID(t *testing.T, pids <-chan int32) int32 {
	t.Helper()
	select {
	case pid := <-pids:
		return pid
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for diagnostic writer backend")
		return 0
	}
}

func awaitContentDiagnosticError(t *testing.T, result <-chan error, description string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for %s", description)
		return context.DeadlineExceeded
	}
}

func releaseContentDiagnosticGuard(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}

func newContentDiagnosticRun(t *testing.T, workspaceID string) (diagnostics.Scope, diagnostics.Run) {
	t.Helper()
	scope := diagnostics.Scope{Workspace: workspaceID, Actor: testUserID}
	run, err := diagnostics.Simulate(context.Background(), scope, "", "normal", 42, "test", true)
	if err != nil {
		t.Fatalf("build diagnostic run: %v", err)
	}
	return scope, run
}

func holdContentDiagnosticRunForDelete(t *testing.T, workspaceID string) pgx.Tx {
	t.Helper()
	ctx := context.Background()
	runID := "block-delete-" + diagnostics.NewID()
	if _, err := testPool.Exec(ctx, `INSERT INTO content_diagnostic_run (run_id, workspace_id, payload) VALUES ($1, $2, '{}'::jsonb)`, runID, workspaceID); err != nil {
		t.Fatalf("seed delete blocker: %v", err)
	}
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin delete blocker: %v", err)
	}
	if _, err := tx.Exec(ctx, `SELECT run_id FROM content_diagnostic_run WHERE run_id = $1 FOR UPDATE`, runID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("lock delete blocker: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func TestContentDiagnosticWritesCoordinateWithWorkspaceDelete(t *testing.T) {
	if testPool == nil || testHandler == nil {
		t.Skip("database not available")
	}

	t.Run("run and audit commit before delete sweep", func(t *testing.T) {
		f := seedContentDiagnosticsRaceFixture(t, "writer-first-run")
		inner := newContentDiagnosticsWorkspaceWriteGuard(testHandler.Queries)
		guard := &blockingContentDiagnosticsGuard{inner: inner, locked: make(chan struct{}), release: make(chan struct{})}
		t.Cleanup(func() { releaseContentDiagnosticGuard(guard.release) })
		store := diagnostics.NewStore(testPool, guard)
		scope, run := newContentDiagnosticRun(t, f.workspaceID)
		writerErr := make(chan error, 1)
		go func() { writerErr <- store.CommitRun(context.Background(), scope, run, false) }()
		awaitContentDiagnosticSignal(t, guard.locked, "run writer to take workspace guard")

		status, deleteDone := startContentDiagnosticWorkspaceDelete(f.workspaceID)
		if !waitForBlockedBackend(t, deleteDone) {
			t.Fatal("workspace delete did not wait for the guarded run writer")
		}
		releaseContentDiagnosticGuard(guard.release)
		if err := awaitContentDiagnosticError(t, writerErr, "run writer"); err != nil {
			t.Fatalf("commit run before delete: %v", err)
		}
		awaitContentDiagnosticSignal(t, deleteDone, "workspace delete")
		if got := <-status; got != http.StatusNoContent {
			t.Fatalf("delete status=%d, want %d", got, http.StatusNoContent)
		}
		assertContentDiagnosticRows(t, f.workspaceID, 0)
		assertContentDiagnosticRows(t, f.neighbourID, 1)
	})

	t.Run("technical commit before delete sweep", func(t *testing.T) {
		f := seedContentDiagnosticsRaceFixture(t, "writer-first-technical")
		inner := newContentDiagnosticsWorkspaceWriteGuard(testHandler.Queries)
		guard := &blockingContentDiagnosticsGuard{inner: inner, locked: make(chan struct{}), release: make(chan struct{})}
		t.Cleanup(func() { releaseContentDiagnosticGuard(guard.release) })
		store := diagnostics.NewStore(testPool, guard)
		event := diagnostics.Event{ID: diagnostics.NewID(), Workspace: f.workspaceID, Actor: testUserID, ActorKind: "human", ObjectType: "diagnostics", Action: "execute", Outcome: "success", Component: "api", Severity: "info"}
		writerDone := make(chan struct{})
		go func() { defer close(writerDone); store.Technical(context.Background(), event) }()
		awaitContentDiagnosticSignal(t, guard.locked, "technical writer to take workspace guard")

		status, deleteDone := startContentDiagnosticWorkspaceDelete(f.workspaceID)
		if !waitForBlockedBackend(t, deleteDone) {
			t.Fatal("workspace delete did not wait for the guarded technical writer")
		}
		releaseContentDiagnosticGuard(guard.release)
		awaitContentDiagnosticSignal(t, writerDone, "technical writer")
		awaitContentDiagnosticSignal(t, deleteDone, "workspace delete")
		if got := <-status; got != http.StatusNoContent {
			t.Fatalf("delete status=%d, want %d", got, http.StatusNoContent)
		}
		assertContentDiagnosticRows(t, f.workspaceID, 0)
		assertContentDiagnosticRows(t, f.neighbourID, 1)
	})

	for _, tc := range []struct {
		name  string
		write func(*diagnostics.Store, diagnostics.Scope, diagnostics.Run) error
	}{
		{
			name: "run and audit are rejected after delete commits",
			write: func(store *diagnostics.Store, scope diagnostics.Scope, run diagnostics.Run) error {
				return store.CommitRun(context.Background(), scope, run, false)
			},
		},
		{
			name: "standalone audit is rejected after delete commits",
			write: func(store *diagnostics.Store, scope diagnostics.Scope, _ diagnostics.Run) error {
				return store.Audit(context.Background(), scope, diagnostics.Event{ID: diagnostics.NewID(), Workspace: scope.Workspace, Actor: scope.Actor, ActorKind: "human", ObjectType: "diagnostics", Action: "export", Outcome: "success", Component: "diagnostics", Severity: "info"})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := seedContentDiagnosticsRaceFixture(t, "delete-first-write")
			blocker := holdContentDiagnosticRunForDelete(t, f.workspaceID)
			status, deleteDone := startContentDiagnosticWorkspaceDelete(f.workspaceID)
			if !waitForBlockedBackend(t, deleteDone) {
				t.Fatal("workspace delete did not reach the held diagnostic row")
			}

			inner := newContentDiagnosticsWorkspaceWriteGuard(testHandler.Queries)
			guard := &observingContentDiagnosticsGuard{inner: inner, pid: make(chan int32, 1)}
			store := diagnostics.NewStore(testPool, guard)
			scope, run := newContentDiagnosticRun(t, f.workspaceID)
			writerErr := make(chan error, 1)
			go func() { writerErr <- tc.write(store, scope, run) }()
			pid := awaitContentDiagnosticPID(t, guard.pid)
			waitForBlockedWriter(t, context.Background(), pid)

			if err := blocker.Rollback(context.Background()); err != nil {
				t.Fatalf("release delete blocker: %v", err)
			}
			awaitContentDiagnosticSignal(t, deleteDone, "workspace delete")
			if got := <-status; got != http.StatusNoContent {
				t.Fatalf("delete status=%d, want %d", got, http.StatusNoContent)
			}
			if err := awaitContentDiagnosticError(t, writerErr, "rejected diagnostic writer"); !errors.Is(err, diagnostics.ErrDenied) {
				t.Fatalf("writer error=%v, want ErrDenied", err)
			}
			assertContentDiagnosticRows(t, f.workspaceID, 0)
			assertContentDiagnosticRows(t, f.neighbourID, 1)
		})
	}

	t.Run("technical is dropped after delete commits", func(t *testing.T) {
		f := seedContentDiagnosticsRaceFixture(t, "delete-first-technical")
		blocker := holdContentDiagnosticRunForDelete(t, f.workspaceID)
		status, deleteDone := startContentDiagnosticWorkspaceDelete(f.workspaceID)
		if !waitForBlockedBackend(t, deleteDone) {
			t.Fatal("workspace delete did not reach the held diagnostic row")
		}

		inner := newContentDiagnosticsWorkspaceWriteGuard(testHandler.Queries)
		guard := &observingContentDiagnosticsGuard{inner: inner, pid: make(chan int32, 1)}
		store := diagnostics.NewStore(testPool, guard)
		event := diagnostics.Event{ID: diagnostics.NewID(), Workspace: f.workspaceID, Actor: testUserID, ActorKind: "human", ObjectType: "diagnostics", Action: "execute", Outcome: "success", Component: "api", Severity: "info"}
		beforeErrors := store.Log.Errors.Load()
		beforeDropped := store.Log.Dropped.Load()
		writerDone := make(chan struct{})
		go func() { defer close(writerDone); store.Technical(context.Background(), event) }()
		pid := awaitContentDiagnosticPID(t, guard.pid)
		waitForBlockedWriter(t, context.Background(), pid)

		if err := blocker.Rollback(context.Background()); err != nil {
			t.Fatalf("release delete blocker: %v", err)
		}
		awaitContentDiagnosticSignal(t, deleteDone, "workspace delete")
		awaitContentDiagnosticSignal(t, writerDone, "technical writer")
		if got := <-status; got != http.StatusNoContent {
			t.Fatalf("delete status=%d, want %d", got, http.StatusNoContent)
		}
		if got := store.Log.Errors.Load(); got != beforeErrors+1 {
			t.Fatalf("sink errors=%d, want %d", got, beforeErrors+1)
		}
		if got := store.Log.Dropped.Load(); got != beforeDropped+1 {
			t.Fatalf("dropped=%d, want %d", got, beforeDropped+1)
		}
		for _, got := range store.Log.Events() {
			if got.ID == event.ID {
				t.Fatal("rejected technical event remained in the in-memory buffer")
			}
		}
		assertContentDiagnosticRows(t, f.workspaceID, 0)
		assertContentDiagnosticRows(t, f.neighbourID, 1)
	})
}
