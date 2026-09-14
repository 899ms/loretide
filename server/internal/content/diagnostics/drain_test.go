package diagnostics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Feature 009 (specs/009-diag-durable-outbox) FR / SC map:
//
//	FR-009  a sweep dispatches records this process never registered
//	FR-009a an expired lease lets another drainer take the record  (SC-010)
//	FR-009b the interval is injected, so a test does not wait for the default
//	        (SC-012)
//	FR-012  two drainers never deliver the same record twice       (SC-003)
//	FR-016  a failing receiver does not affect the committed business write
//	FR-017  failures land in the two counters that already exist   (SC-004)
//	FR-018  attempts are bounded and a dead letter is observable
//	FR-020  finished records are pruned on the same wake-up
//
// Like dispatch_postgres_test.go these need a real transaction, so they run
// only with LORETIDE_DIAG_TEST_DATABASE_URL set. A skip is not a pass.

// commitOne registers one record in its own committed transaction, the way a
// business write would, and returns without dispatching anything.
func (f *durableFixture) commitOne(t *testing.T, d DispatchItem) {
	t.Helper()
	f.inTx(t, true, func(tx pgx.Tx) {
		if err := f.out.Register(context.Background(), tx, d); err != nil {
			t.Fatalf("register %s: %v", d.ID, err)
		}
	})
}

func (f *durableFixture) drainer(opts DrainerOptions) *Drainer { return NewDrainer(f.out, opts) }

// FR-012, SC-003. Two drainers sweeping the same batch at the same time deliver
// each record once in total, not once each.
func TestDrainerConcurrentSweepsDeliverEachRecordOnce(t *testing.T) {
	f := newDurableFixture(t)
	const records = 12
	for i := range records {
		f.commitOne(t, item("concurrent-"+NewID()+"-"+string(rune('a'+i))))
	}

	a := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: records, Retention: time.Hour})
	b := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: records, Retention: time.Hour})

	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i, d := range []*Drainer{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counts[i] = d.DrainOnce(context.Background())
		}()
	}
	wg.Wait()

	if total := counts[0] + counts[1]; total != records {
		t.Fatalf("two drainers delivered %d in total (%d + %d), want %d", total, counts[0], counts[1], records)
	}
	// The receiver is the real check: a record delivered twice shows up here
	// even if the counts happen to add up.
	seen := map[string]int{}
	for _, id := range f.rec.ids() {
		seen[id]++
	}
	if len(seen) != records {
		t.Fatalf("receiver saw %d distinct records, want %d", len(seen), records)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("record %s delivered %d times, want exactly 1", id, n)
		}
	}
}

// FR-009a, SC-010. A drainer that claims a record and then disappears must not
// strand it: once the lease expires another drainer takes it, and the record is
// still delivered only once in total.
func TestDrainerReclaimsARecordWhoseLeaseExpired(t *testing.T) {
	f := newDurableFixture(t)
	f.commitOne(t, item("stranded"))

	// Claim with a lease measured in milliseconds and then walk away without
	// delivering — a drainer whose process died holding the claim.
	claimed, err := f.out.claimDue(context.Background(), 10, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d records, want 1", len(claimed))
	}
	// While the lease holds, nobody else may take it.
	if n := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour}).DrainOnce(context.Background()); n != 0 {
		t.Fatalf("a second drainer delivered %d records while the lease was held", n)
	}

	time.Sleep(120 * time.Millisecond)

	if n := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour}).DrainOnce(context.Background()); n != 1 {
		t.Fatalf("after the lease expired the record was delivered %d times, want 1", n)
	}
	if got := f.rec.ids(); len(got) != 1 || got[0] != "stranded" {
		t.Fatalf("receiver saw %v, want exactly [stranded]", got)
	}
}

// FR-016, FR-017, FR-018, SC-004. A receiver that keeps failing costs the
// business write nothing, is counted where an operator already looks, and the
// record eventually stops being retried instead of spinning forever.
func TestDrainerBoundsRetriesAndDeadLetters(t *testing.T) {
	f := newDurableFixture(t)
	bounded := NewPostgresOutbox(f.store.Pool(), f.store.Log, OutboxLimits{MaxPayload: 1 << 20, MaxAttempts: 2})
	bounded.Deliver = func(context.Context, DispatchItem) error { return errors.New("receiver down") }
	f.out.Deliver = bounded.Deliver

	// The business write itself succeeds and stays committed.
	f.commitOne(t, item("doomed"))
	if n := f.rows(t, "item_id = $1", "doomed"); n != 1 {
		t.Fatalf("the committed record is not in the table (%d rows)", n)
	}

	d := NewDrainer(bounded, DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour})
	errsBefore := f.store.Log.Errors.Load()

	// First attempt fails and backs the record off.
	if n := d.DrainOnce(context.Background()); n != 0 {
		t.Fatalf("a failing receiver reported %d deliveries", n)
	}
	if f.rows(t, "item_id = $1 AND attempt_count = 1 AND dead_lettered_at IS NULL", "doomed") != 1 {
		t.Fatal("after one failure the record should be backed off, not dead lettered")
	}
	// Backoff is in the future, so an immediate sweep must find nothing.
	if n := d.DrainOnce(context.Background()); n != 0 {
		t.Fatalf("a backed-off record was retried immediately (%d deliveries)", n)
	}

	// Pull the next attempt forward instead of sleeping through the backoff.
	if _, err := f.store.Pool().Exec(context.Background(),
		"UPDATE content_dispatch_outbox SET next_attempt_at = now() - interval '1 second' WHERE item_id = $1", "doomed"); err != nil {
		t.Fatalf("advance backoff: %v", err)
	}
	if n := d.DrainOnce(context.Background()); n != 0 {
		t.Fatalf("second attempt reported %d deliveries", n)
	}
	if f.rows(t, "item_id = $1 AND dead_lettered_at IS NOT NULL", "doomed") != 1 {
		t.Fatal("the record should be dead lettered after its attempts ran out")
	}
	// Dead lettered means dead: no further sweep may pick it up.
	if _, err := f.store.Pool().Exec(context.Background(),
		"UPDATE content_dispatch_outbox SET next_attempt_at = now() - interval '1 second' WHERE item_id = $1", "doomed"); err != nil {
		t.Fatalf("advance backoff: %v", err)
	}
	if n := d.DrainOnce(context.Background()); n != 0 {
		t.Fatalf("a dead-lettered record was retried (%d deliveries)", n)
	}

	if grew := f.store.Log.Errors.Load() - errsBefore; grew < 2 {
		t.Fatalf("Errors grew by %d across two failed deliveries, want at least 2", grew)
	}
	// Dead lettering is reported through the technical log, not a new counter.
	// Sanitize is an allowlist, so Step is the discriminator that survives it -
	// an invented ObjectType would arrive as "unknown".
	found := false
	for _, e := range f.store.Log.Events() {
		if e.Step == dispatchDeadLetterStep && e.Outcome == "failed" && e.ObjectID == "doomed" {
			found = true
		}
	}
	if !found {
		t.Fatal("dead lettering produced no technical log event; it would only be visible in a column nobody reads")
	}
}

// FR-020. Finished records do not accumulate: the same wake-up that drains also
// deletes what has outlived the retention window, and leaves live records alone.
func TestDrainerPrunesFinishedRecordsOnTheSameWakeUp(t *testing.T) {
	f := newDurableFixture(t)
	f.commitOne(t, item("to-prune"))
	d := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour})
	if n := d.DrainOnce(context.Background()); n != 1 {
		t.Fatalf("delivered %d, want 1", n)
	}
	if f.rows(t, "item_id = $1", "to-prune") != 1 {
		t.Fatal("a freshly delivered record must not be pruned")
	}

	// Age it past the window instead of waiting a week.
	if _, err := f.store.Pool().Exec(context.Background(),
		"UPDATE content_dispatch_outbox SET delivered_at = now() - interval '2 hours' WHERE item_id = $1", "to-prune"); err != nil {
		t.Fatalf("age record: %v", err)
	}
	f.commitOne(t, item("still-live"))

	d.DrainOnce(context.Background())

	if n := f.rows(t, "item_id = $1", "to-prune"); n != 0 {
		t.Fatalf("%d records survived past the retention window", n)
	}
	if n := f.rows(t, "item_id = $1", "still-live"); n != 1 {
		t.Fatalf("prune removed a record that was not finished (%d left)", n)
	}
}

// FR-009b, SC-012. The interval is a parameter, so a sweep loop can be observed
// in milliseconds rather than at the production cadence.
func TestDrainerIntervalIsInjectedAndStopsWithItsContext(t *testing.T) {
	f := newDurableFixture(t)
	f.commitOne(t, item("swept-by-loop"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.drainer(DrainerOptions{Interval: 10 * time.Millisecond, Lease: time.Minute, Batch: 10, Retention: time.Hour}).Start(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(f.rec.ids()) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := f.rec.ids(); len(got) != 1 {
		t.Fatalf("the sweep loop delivered %v within its own interval, want one record", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after its context was cancelled")
	}
}

// FR-012. The lease is what keeps two drainers from delivering the same record;
// SKIP LOCKED is what keeps the second one from *waiting* for the first. They
// buy different things, and only this case can tell them apart: hold a row lock
// open in another transaction and a sweep must come back empty rather than
// block until that transaction ends.
//
// Without SKIP LOCKED this test hangs until the context deadline and fails.
func TestDrainerDoesNotBlockOnARowAnotherTransactionHolds(t *testing.T) {
	f := newDurableFixture(t)
	f.commitOne(t, item("locked-row"))

	holder, err := f.store.Pool().Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	var held int64
	if err = holder.QueryRow(context.Background(),
		"SELECT sequence FROM content_dispatch_outbox WHERE item_id = $1 FOR UPDATE", "locked-row").Scan(&held); err != nil {
		t.Fatalf("hold row: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan int, 1)
	go func() {
		done <- f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour}).DrainOnce(ctx)
	}()
	select {
	case n := <-done:
		if n != 0 {
			t.Fatalf("a sweep delivered %d records that another transaction had locked", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the sweep blocked on a row another transaction held; SKIP LOCKED is what stops drainers serialising behind each other")
	}

	// Once the holder lets go, the record is delivered normally.
	if err = holder.Rollback(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	if n := f.drainer(DrainerOptions{Interval: time.Hour, Lease: time.Minute, Batch: 10, Retention: time.Hour}).DrainOnce(context.Background()); n != 1 {
		t.Fatalf("after the lock was released the sweep delivered %d, want 1", n)
	}
}
