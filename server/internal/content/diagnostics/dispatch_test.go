package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// Feature 005 (specs/005-diag-trace-and-sanitize) FR map:
//
//	FR-009  a rollback dispatches nothing; a commit dispatches
//	FR-010  a failed dispatch is visible and does not take the caller down
//	FR-010a a process exit loses undispatched items — asserted as a limit
//	FR-011  the queue is bounded and an overflow is counted, not silent
//	FR-017  the signature does not assume the in-process implementation

func item(id string) DispatchItem {
	return DispatchItem{ID: id, Kind: "audit", Payload: []byte(`{"x":1}`), IdempotencyKey: id}
}

// recorder stands in for whatever eventually receives a dispatched record.
type recorder struct {
	mu   sync.Mutex
	got  []string
	fail error
}

func (r *recorder) deliver(_ context.Context, d DispatchItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.got = append(r.got, d.ID)
	return nil
}

func (r *recorder) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.got...)
}

func newOutbox(rec *recorder) *MemoryOutbox {
	o := NewMemoryOutbox(NewLogBuffer(10), 4)
	o.Deliver = rec.deliver
	return o
}

func TestOutboxRollbackDispatchesNothing(t *testing.T) {
	rec := &recorder{}
	o := newOutbox(rec)
	tx := new(int)

	if err := o.Register(context.Background(), tx, item("a")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if n := o.Settle(context.Background(), tx, false); n != 0 {
		t.Fatalf("rollback dispatched %d items, want 0", n)
	}
	if got := rec.ids(); len(got) != 0 {
		t.Fatalf("rollback delivered %v; a record from a rolled-back transaction must not exist", got)
	}
	// And it must not resurface later.
	if n := o.Settle(context.Background(), tx, true); n != 0 {
		t.Fatalf("a discarded record came back on a later settle: %d", n)
	}
}

func TestOutboxCommitDispatchesExactlyOnce(t *testing.T) {
	rec := &recorder{}
	o := newOutbox(rec)
	tx := new(int)

	for _, id := range []string{"a", "b"} {
		if err := o.Register(context.Background(), tx, item(id)); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	if n := o.Settle(context.Background(), tx, true); n != 2 {
		t.Fatalf("commit dispatched %d items, want 2", n)
	}
	if got := fmt.Sprint(rec.ids()); got != "[a b]" {
		t.Fatalf("delivered %s, want [a b]", got)
	}
	// Settling the same transaction again must not double-deliver.
	if n := o.Settle(context.Background(), tx, true); n != 0 {
		t.Fatalf("a second settle dispatched %d more items", n)
	}
}

func TestOutboxDeduplicatesOnIdempotencyKey(t *testing.T) {
	rec := &recorder{}
	o := newOutbox(rec)

	first, second := new(int), new(int)
	_ = o.Register(context.Background(), first, item("a"))
	o.Settle(context.Background(), first, true)
	// Same key, different transaction: a retry of the same logical record.
	_ = o.Register(context.Background(), second, item("a"))
	o.Settle(context.Background(), second, true)

	if got := rec.ids(); len(got) != 1 {
		t.Fatalf("delivered %v, want one delivery for a repeated idempotency key", got)
	}
}

func TestOutboxDeliveryFailureIsVisibleAndDoesNotPropagate(t *testing.T) {
	rec := &recorder{fail: errors.New("receiver unavailable")}
	log := NewLogBuffer(10)
	o := NewMemoryOutbox(log, 4)
	o.Deliver = rec.deliver
	tx := new(int)

	_ = o.Register(context.Background(), tx, item("a"))
	// Settle reports what it dispatched; a failure is not an error the caller
	// has to handle, the same way Store.Technical does not fail a request.
	if n := o.Settle(context.Background(), tx, true); n != 0 {
		t.Fatalf("a failed delivery was counted as dispatched: %d", n)
	}
	if got := log.Errors.Load(); got == 0 {
		t.Fatal("a failed delivery left no trace in the error counter; it must be visible")
	}
}

func TestOutboxIsBoundedAndCountsWhatItDrops(t *testing.T) {
	rec := &recorder{}
	log := NewLogBuffer(10)
	o := NewMemoryOutbox(log, 2)
	o.Deliver = rec.deliver
	tx := new(int)

	for i := 0; i < 5; i++ {
		_ = o.Register(context.Background(), tx, item(fmt.Sprintf("i%d", i)))
	}
	dispatched := o.Settle(context.Background(), tx, true)
	if dispatched > 2 {
		t.Fatalf("dispatched %d items past a capacity of 2", dispatched)
	}
	if log.Dropped.Load() == 0 {
		t.Fatal("items were dropped without being counted; an overflow must be visible")
	}
}

// FR-010a. The limit is asserted rather than described, so that replacing the
// in-process implementation with a durable one has a test to turn green.
func TestOutboxDoesNotSurviveTheProcess(t *testing.T) {
	rec := &recorder{}
	o := newOutbox(rec)
	tx := new(int)
	_ = o.Register(context.Background(), tx, item("a"))

	// A new outbox is what a restarted process gets. The staged record is gone:
	// nothing outside this process ever knew about it.
	restarted := newOutbox(rec)
	if n := restarted.Settle(context.Background(), tx, true); n != 0 {
		t.Fatalf("a restarted process dispatched %d items; the in-process outbox cannot know about them", n)
	}
	if got := rec.ids(); len(got) != 0 {
		t.Fatalf("delivered %v after a restart", got)
	}
}

// FR-017. A durable implementation must be able to replace this one without the
// caller changing, so the caller is written against the interface here.
type stubOutbox struct {
	registered []DispatchItem
	settled    []bool
}

func (s *stubOutbox) Register(_ context.Context, _ any, d DispatchItem) error {
	s.registered = append(s.registered, d)
	return nil
}

func (s *stubOutbox) Settle(_ context.Context, _ any, committed bool) int {
	s.settled = append(s.settled, committed)
	if committed {
		return len(s.registered)
	}
	return 0
}

func TestOutboxCallersDependOnTheInterfaceOnly(t *testing.T) {
	var _ Outbox = (*MemoryOutbox)(nil)

	// The same caller body, run against a stub that stores nothing in process.
	record := func(o Outbox) int {
		tx := new(int)
		_ = o.Register(context.Background(), tx, item("a"))
		return o.Settle(context.Background(), tx, true)
	}

	stub := &stubOutbox{}
	if n := record(stub); n != 1 {
		t.Fatalf("stub dispatched %d, want 1", n)
	}
	if len(stub.registered) != 1 || stub.registered[0].IdempotencyKey != "a" {
		t.Fatalf("the four-part record did not reach the implementation: %+v", stub.registered)
	}
	if n := record(newOutbox(&recorder{})); n != 1 {
		t.Fatalf("in-process outbox dispatched %d, want 1", n)
	}
}
