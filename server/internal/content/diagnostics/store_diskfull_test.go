package diagnostics

import (
	"context"
	"testing"
)

// DIAG-03 "验证磁盘满模拟" (docs/development/diagnostics-acceptance-mapping.md).
//
// The bounded in-memory buffer is already covered by
// TestLogRegressionBufferCapacities. What had no test was the other end: the
// durable sink refusing writes because storage is exhausted. The card asks for
// three things at once — the failure is bounded, it is counted into dropped and
// sink_errors, and it does not take the caller's business down.
//
// Exhaustion is simulated inside the isolated schema by a trigger that raises
// SQLSTATE 53100, which is PostgreSQL's own disk_full condition. That drives the
// real INSERT path in Store.Technical and returns the error a full volume would,
// rather than standing in a fake at the Go boundary. No production code changes.

// exhaustStorage makes every insert into content_technical_log fail the way a
// full volume does. Returns a function that restores writes.
func exhaustStorage(t *testing.T, s *Store) func() {
	t.Helper()
	ctx := context.Background()
	const fn = `CREATE OR REPLACE FUNCTION diag_disk_full() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'could not extend file: No space left on device' USING ERRCODE = '53100';
END;
$$ LANGUAGE plpgsql;`
	if _, err := s.pool.Exec(ctx, fn); err != nil {
		t.Fatalf("install disk-full function: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`CREATE TRIGGER diag_disk_full_trigger BEFORE INSERT ON content_technical_log
		 FOR EACH ROW EXECUTE FUNCTION diag_disk_full();`); err != nil {
		t.Fatalf("install disk-full trigger: %v", err)
	}
	return func() {
		if _, err := s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS diag_disk_full_trigger ON content_technical_log`); err != nil {
			t.Fatalf("restore writes: %v", err)
		}
	}
}

func technicalRowCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), "SELECT count(*) FROM content_technical_log").Scan(&n); err != nil {
		t.Fatalf("count technical rows: %v", err)
	}
	return n
}

func TestTechnicalSinkOnFullStorageStaysBoundedVisibleAndNonFatal(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	run, err := Simulate(ctx, scope, "a", "normal", 1, "test", true)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	sample := run.Events[0]

	// A healthy write first, so a later assertion of "nothing landed" cannot
	// pass simply because writes never worked in this schema.
	healthy := sample
	healthy.ID = NewID()
	s.Technical(ctx, healthy)
	if got := technicalRowCount(t, s); got != 1 {
		t.Fatalf("baseline write did not land: %d rows", got)
	}

	restore := exhaustStorage(t, s)
	defer restore()

	const attempts = 25
	errorsBefore := s.Log.Errors.Load()
	droppedBefore := s.Log.Dropped.Load()
	bufferedBefore := len(s.Log.Events())
	rowsBefore := technicalRowCount(t, s)

	for i := 0; i < attempts; i++ {
		e := sample
		e.ID = NewID()
		// Technical returns nothing: a log sink cannot report failure to its
		// caller. Reaching the next line at all is part of the assertion.
		s.Technical(ctx, e)
	}

	// Visible: both counters move, and by the full amount. A sink that dropped
	// silently, or counted one failure for a burst, would fail here.
	if got := s.Log.Errors.Load() - errorsBefore; got != attempts {
		t.Errorf("sink_errors rose by %d, want %d: a full disk must be visible per failed write", got, attempts)
	}
	if got := s.Log.Dropped.Load() - droppedBefore; got != attempts {
		t.Errorf("dropped rose by %d, want %d", got, attempts)
	}

	// Bounded: nothing accumulates while the sink is refusing. The in-memory
	// buffer is only appended after a successful write, so a full disk must not
	// turn into unbounded memory growth.
	if got := len(s.Log.Events()) - bufferedBefore; got != 0 {
		t.Errorf("the in-memory buffer grew by %d during storage exhaustion", got)
	}
	if got := technicalRowCount(t, s); got != rowsBefore {
		t.Errorf("rows changed from %d to %d while every insert was failing", rowsBefore, got)
	}

	// Not fatal: business writes go to their own tables and must still commit
	// while the technical log is refusing. This is the DIAG-03 clause that a
	// counter assertion alone does not cover.
	if err := s.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatalf("a full technical log took a business write down with it: %v", err)
	}
	if rows, err := s.Runs(ctx, scope); err != nil || len(rows) != 1 {
		t.Fatalf("business read after sink failure: %d rows, err %v", len(rows), err)
	}
	// Reads of the surviving technical rows must also still work.
	if _, err := s.Query(ctx, scope, Filter{Limit: 10}); err != nil {
		t.Fatalf("query failed while the sink was full: %v", err)
	}

	// Recovery: once storage is available the sink resumes, and stops counting.
	restore()
	errorsAtRecovery := s.Log.Errors.Load()
	droppedAtRecovery := s.Log.Dropped.Load()
	recovered := sample
	recovered.ID = NewID()
	s.Technical(ctx, recovered)

	if got := technicalRowCount(t, s); got != rowsBefore+1 {
		t.Fatalf("write did not resume after storage recovered: %d rows, want %d", got, rowsBefore+1)
	}
	if s.Log.Errors.Load() != errorsAtRecovery || s.Log.Dropped.Load() != droppedAtRecovery {
		t.Error("counters kept rising after storage recovered")
	}
}

// The audit path is deliberately different: it writes inside the caller's
// transaction and fails it. A full disk must therefore reject the whole
// operation rather than be counted and swallowed, which is the distinction
// D13-V11 draws between an ordinary technical log and a critical audit.
func TestAuditOnFullStorageRejectsTheWriteInsteadOfCounting(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	restoreFn := func() {}
	if _, err := s.pool.Exec(ctx, `CREATE OR REPLACE FUNCTION diag_audit_full() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'could not extend file: No space left on device' USING ERRCODE = '53100';
END;
$$ LANGUAGE plpgsql;`); err != nil {
		t.Fatalf("install function: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`CREATE TRIGGER diag_audit_full_trigger BEFORE INSERT ON content_operation_audit
		 FOR EACH ROW EXECUTE FUNCTION diag_audit_full();`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}
	restoreFn = func() {
		_, _ = s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS diag_audit_full_trigger ON content_operation_audit`)
	}
	defer restoreFn()

	errorsBefore := s.Log.Errors.Load()
	event := Event{ID: NewID(), Workspace: "w", Actor: "u", ActorKind: "human",
		ObjectType: "diagnostics", Action: "export", Outcome: "success", Component: "diagnostics", Severity: "info"}

	if err := s.Audit(ctx, scope, event); err == nil {
		t.Fatal("a full disk let a critical audit report success")
	}
	// It failed loudly rather than being counted as a dropped log line.
	if got := s.Log.Errors.Load(); got != errorsBefore {
		t.Errorf("the audit failure was counted like a log drop (%d -> %d); it must be returned to the caller", errorsBefore, got)
	}
}
