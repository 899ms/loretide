package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Feature 009 (specs/009-diag-durable-outbox) FR / SC map:
//
//	FR-001  the interface is unchanged; a caller swaps implementations unedited
//	FR-003  no silent degradation to in-process behaviour without a database
//	FR-004  a transaction handle that is not a real transaction fails closed
//	FR-006  a rolled-back transaction leaves no row              (SC-002)
//	FR-007  the row is written through the caller's transaction
//	FR-008  a committed transaction dispatches its own batch in place
//	FR-010  a restart still dispatches                            (SC-001)
//	FR-011  and dispatches exactly once                           (SC-001)
//	FR-013  an empty idempotency key does not deduplicate
//	FR-014  the reverse of TestMemoryOutboxDoesNotSurviveTheProcess (SC-007)
//	FR-019  an oversized payload is dropped and counted, not written
//
// Every case here needs a real transaction, so they run only with
// LORETIDE_DIAG_TEST_DATABASE_URL set (see testStore). A skip is not a pass.

// durableFixture is a Store wired to a PostgresOutbox over the same pool, plus
// a recorder standing in for the receiver.
type durableFixture struct {
	store *Store
	out   *PostgresOutbox
	rec   *recorder
}

func newDurableFixture(t *testing.T) *durableFixture {
	t.Helper()
	s := testStore(t)
	rec := &recorder{}
	out := NewPostgresOutbox(s.Pool(), s.Log, OutboxLimits{MaxPayload: 1 << 20, MaxAttempts: 3})
	out.Deliver = rec.deliver
	return &durableFixture{store: s, out: out, rec: rec}
}

// inTx runs body inside one transaction and reports whether it committed, so a
// test can register a record the way a business write would.
func (f *durableFixture) inTx(t *testing.T, commit bool, body func(tx pgx.Tx)) any {
	t.Helper()
	ctx := context.Background()
	tx, err := f.store.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	body(tx)
	if commit {
		if err = tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	} else if err = tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	return tx
}

func (f *durableFixture) rows(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	if err := f.store.Pool().QueryRow(context.Background(), "SELECT count(*) FROM content_dispatch_outbox WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// FR-007, FR-008. The row is written through the caller's transaction and the
// batch dispatches once that transaction commits.
func TestPostgresOutboxRegistersInTheCallersTransaction(t *testing.T) {
	f := newDurableFixture(t)
	tx := f.inTx(t, true, func(tx pgx.Tx) {
		if err := f.out.Register(context.Background(), tx, item("commit-1")); err != nil {
			t.Fatalf("register: %v", err)
		}
		// Still inside the transaction: nothing may have been dispatched yet.
		if got := f.rec.ids(); len(got) != 0 {
			t.Fatalf("dispatched %v before commit", got)
		}
	})
	if n := f.out.Settle(context.Background(), tx, true); n != 1 {
		t.Fatalf("settle dispatched %d, want 1", n)
	}
	if got := f.rec.ids(); len(got) != 1 || got[0] != "commit-1" {
		t.Fatalf("delivered %v, want [commit-1]", got)
	}
	if n := f.rows(t, "item_id = $1 AND delivered_at IS NOT NULL", "commit-1"); n != 1 {
		t.Fatalf("delivered_at not recorded (%d rows marked)", n)
	}
}

// FR-006, SC-002. A rolled-back transaction leaves no row at all, so no later
// drain can resurrect it.
func TestPostgresOutboxRollbackLeavesNoRow(t *testing.T) {
	f := newDurableFixture(t)
	tx := f.inTx(t, false, func(tx pgx.Tx) {
		if err := f.out.Register(context.Background(), tx, item("rolled-back")); err != nil {
			t.Fatalf("register: %v", err)
		}
	})
	if n := f.out.Settle(context.Background(), tx, false); n != 0 {
		t.Fatalf("settle dispatched %d after a rollback", n)
	}
	if n := f.rows(t, "item_id = $1", "rolled-back"); n != 0 {
		t.Fatalf("%d rows survived the rollback", n)
	}
	if got := f.rec.ids(); len(got) != 0 {
		t.Fatalf("delivered %v after a rollback", got)
	}
}

// FR-010, FR-011, FR-014, SC-001, SC-007. The reverse of
// TestMemoryOutboxDoesNotSurviveTheProcess: a record committed but never
// dispatched by this process is dispatched after a restart, exactly once.
func TestPostgresOutboxSurvivesTheProcess(t *testing.T) {
	f := newDurableFixture(t)
	// Commit without ever calling Settle: the process died between commit and
	// dispatch, which is precisely the window the memory implementation loses.
	f.inTx(t, true, func(tx pgx.Tx) {
		if err := f.out.Register(context.Background(), tx, item("survivor")); err != nil {
			t.Fatalf("register: %v", err)
		}
	})
	if got := f.rec.ids(); len(got) != 0 {
		t.Fatalf("dispatched %v without a settle", got)
	}

	// A restarted process: a brand-new outbox and drainer over the same table.
	restarted := NewPostgresOutbox(f.store.Pool(), f.store.Log, OutboxLimits{MaxPayload: 1 << 20, MaxAttempts: 3})
	restarted.Deliver = f.rec.deliver
	d := NewDrainer(restarted, DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour})

	if n := d.DrainOnce(context.Background()); n != 1 {
		t.Fatalf("a restarted process dispatched %d items, want 1", n)
	}
	if got := f.rec.ids(); len(got) != 1 || got[0] != "survivor" {
		t.Fatalf("delivered %v after a restart, want [survivor]", got)
	}
	// Exactly once: a second sweep must find nothing.
	if n := d.DrainOnce(context.Background()); n != 0 {
		t.Fatalf("a second drain dispatched %d items; the record was already delivered", n)
	}
	if got := f.rec.ids(); len(got) != 1 {
		t.Fatalf("delivered %v across two drains, want exactly one delivery", got)
	}
}

// FR-004. A handle that is not a real transaction fails closed. Staging it or
// accepting it silently would report a record as durable when nothing was
// written, which is worse than the caller's transaction failing.
func TestPostgresOutboxRejectsANonTransactionHandle(t *testing.T) {
	f := newDurableFixture(t)
	if err := f.out.Register(context.Background(), new(int), item("not-a-tx")); err == nil {
		t.Fatal("Register accepted a handle that is not a transaction")
	}
	if n := f.rows(t, "item_id = $1", "not-a-tx"); n != 0 {
		t.Fatalf("%d rows written for a handle that is not a transaction", n)
	}
}

// FR-013. A non-empty key deduplicates; an empty key does not — the same answer
// MemoryOutbox gives, so swapping implementations cannot change behaviour.
func TestPostgresOutboxIdempotencyKeyRules(t *testing.T) {
	f := newDurableFixture(t)
	dup := DispatchItem{ID: "dup", Kind: "audit", Payload: []byte("{}"), IdempotencyKey: "same-key"}
	for range 2 {
		f.inTx(t, true, func(tx pgx.Tx) {
			if err := f.out.Register(context.Background(), tx, dup); err != nil {
				t.Fatalf("register: %v", err)
			}
		})
	}
	if n := f.rows(t, "idempotency_key = $1", "same-key"); n != 1 {
		t.Fatalf("%d rows for one idempotency key, want 1", n)
	}

	blank := DispatchItem{ID: "blank", Kind: "audit", Payload: []byte("{}")}
	for range 2 {
		f.inTx(t, true, func(tx pgx.Tx) {
			if err := f.out.Register(context.Background(), tx, blank); err != nil {
				t.Fatalf("register: %v", err)
			}
		})
	}
	if n := f.rows(t, "item_id = $1", "blank"); n != 2 {
		t.Fatalf("%d rows for two empty-key registrations, want 2 (an empty key must not deduplicate)", n)
	}
}

// FR-019, FR-017. An oversized payload is dropped and counted rather than
// written — and Register still returns nil, because losing one diagnostic
// record must not take down the caller's business transaction.
func TestPostgresOutboxDropsAnOversizedPayload(t *testing.T) {
	f := newDurableFixture(t)
	small := NewPostgresOutbox(f.store.Pool(), f.store.Log, OutboxLimits{MaxPayload: 8, MaxAttempts: 3})
	before := f.store.Log.Dropped.Load()
	f.inTx(t, true, func(tx pgx.Tx) {
		big := DispatchItem{ID: "oversized", Kind: "audit", Payload: make([]byte, 64), IdempotencyKey: "oversized"}
		if err := small.Register(context.Background(), tx, big); err != nil {
			t.Fatalf("Register returned %v; an oversized payload must be dropped, not fail the caller's transaction", err)
		}
	})
	if n := f.rows(t, "item_id = $1", "oversized"); n != 0 {
		t.Fatalf("%d oversized rows written", n)
	}
	if got := f.store.Log.Dropped.Load() - before; got != 1 {
		t.Fatalf("Dropped grew by %d, want 1", got)
	}
}

// FR-001. The durable implementation satisfies the same interface, and the
// existing TestOutboxCallersDependOnTheInterfaceOnly is the standing assertion
// that a caller written against it needs no edit.
func TestPostgresOutboxSatisfiesTheOutboxInterface(t *testing.T) {
	var _ Outbox = (*PostgresOutbox)(nil)
	var _ Outbox = (*MemoryOutbox)(nil)
}

// FR-003. Without a pool the durable implementation fails; it never quietly
// becomes an in-process queue, which is the very property this feature removes.
func TestPostgresOutboxWithoutAPoolFailsRatherThanDegrading(t *testing.T) {
	o := NewPostgresOutbox(nil, NewLogBuffer(10), OutboxLimits{MaxPayload: 1 << 20, MaxAttempts: 3})
	if err := o.Register(context.Background(), new(int), item("no-pool")); err == nil {
		t.Fatal("Register succeeded without a pool")
	}
	if n := o.Settle(context.Background(), new(int), true); n != 0 {
		t.Fatalf("Settle dispatched %d without a pool", n)
	}
}

// FR-005, FR-005a, SC-011. The run row, its audit event and the dispatch record
// share one transaction: commit and all three exist, roll back and none do.
// Every table it touches belongs to this module.
func TestCommitRunWithDispatchSharesOneTransaction(t *testing.T) {
	f := newDurableFixture(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}
	run, err := Simulate(ctx, scope, "a", "normal", 1, "test", true)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}

	rec := DispatchItem{ID: run.ID, Kind: "run_committed", Payload: []byte(`{}`), IdempotencyKey: "commit-" + run.ID}
	if err = f.store.CommitRunWithDispatch(ctx, scope, run, f.out, rec); err != nil {
		t.Fatalf("CommitRunWithDispatch: %v", err)
	}

	if got := f.rec.ids(); len(got) != 1 || got[0] != run.ID {
		t.Fatalf("delivered %v, want [%s]", got, run.ID)
	}
	if n := f.rows(t, "item_id = $1 AND delivered_at IS NOT NULL", run.ID); n != 1 {
		t.Fatalf("%d dispatch rows marked delivered, want 1", n)
	}
	// The workspace comes from the run, not from the caller's item.
	if n := f.rows(t, "item_id = $1 AND workspace_id = $2", run.ID, "w"); n != 1 {
		t.Fatal("the dispatch record was not scoped to the run's workspace")
	}
	// The business write is really there alongside it.
	var runRows int
	if err = f.store.Pool().QueryRow(ctx, "SELECT count(*) FROM content_diagnostic_run WHERE run_id = $1", run.ID).Scan(&runRows); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runRows != 1 {
		t.Fatalf("%d run rows, want 1", runRows)
	}

	// A denied scope writes nothing at all, on either side.
	other := Scope{Workspace: "other", Actor: "u", Accounts: []string{"a"}}
	if err = f.store.CommitRunWithDispatch(ctx, other, run, f.out, rec); err == nil {
		t.Fatal("CommitRunWithDispatch accepted a scope that does not allow the run")
	}
	if n := f.rows(t, "item_id = $1", run.ID); n != 1 {
		t.Fatalf("%d dispatch rows after a denied commit, want the original 1", n)
	}
}

// FR-006 through the real entry point: when the transaction fails, neither the
// run nor the dispatch record exists, and nothing is dispatched.
func TestCommitRunWithDispatchRollsBackBothWrites(t *testing.T) {
	f := newDurableFixture(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}
	run, err := Simulate(ctx, scope, "a", "normal", 2, "test", true)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if err = f.store.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	// Committing the same run id again violates the run table's unique index,
	// so the transaction fails after the dispatch record was registered.
	rec := DispatchItem{ID: run.ID, Kind: "run_committed", Payload: []byte(`{}`), IdempotencyKey: "rollback-" + run.ID}
	if err = f.store.CommitRunWithDispatch(ctx, scope, run, f.out, rec); err == nil {
		t.Fatal("a duplicate run committed twice")
	}
	if n := f.rows(t, "idempotency_key = $1", "rollback-"+run.ID); n != 0 {
		t.Fatalf("%d dispatch rows survived a failed transaction", n)
	}
	if got := f.rec.ids(); len(got) != 0 {
		t.Fatalf("delivered %v after a failed transaction", got)
	}
}
