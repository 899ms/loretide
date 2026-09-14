package diagnostics

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Feature 010 (specs/010-diag-evidence-gaps) FR / SC map:
//
//	FR-008  QueueWait sums the queue's event durations and nothing else
//	FR-009  P95 is null below the sample threshold and set at it   (SC-005)
//
// These close the two DIAG-07 rows of the acceptance mapping. Both assertions
// need a real transaction after all: Overview reads its events back through
// Store.Query, so there is no way to feed it a sample set without a database.
// The specification assumed otherwise; see the delivery record.

// seedTechnical writes n events through the same path production uses, so what
// Overview reads back has been through Sanitize exactly once.
func seedTechnical(t *testing.T, s *Store, scope Scope, component string, duration int64, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		s.Technical(ctx, Event{
			ID: NewID(), Workspace: scope.Workspace, Account: "a", Actor: scope.Actor,
			ActorKind: "system", ObjectType: "diagnostics", Action: "execute",
			Outcome: "success", Component: component, Severity: "info",
			Duration: duration, Occurred: time.Now().UTC(), Received: time.Now().UTC(),
			Step: fmt.Sprintf("%02d", i),
		})
	}
}

func overviewFor(t *testing.T, s *Store, scope Scope) Overview {
	t.Helper()
	svc := NewService(s, "test", true)
	o, err := svc.Overview(context.Background(), scope)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	return o
}

// FR-008. QueueWait is the queue's waiting time, not the total time of
// everything that happened. The second half of this test is the part that
// matters: with only the first, deleting the component check and summing every
// event would still pass.
func TestOverviewQueueWaitCountsOnlyQueueEvents(t *testing.T) {
	s := testStore(t)
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	seedTechnical(t, s, scope, "queue", 40, 3)
	if got := overviewFor(t, s, scope).Metrics.QueueWait; got != 120 {
		t.Fatalf("QueueWait = %d after 3 queue events of 40ms, want 120", got)
	}

	// Non-queue work takes time too. None of it is queue wait.
	seedTechnical(t, s, scope, "executor", 500, 4)
	o := overviewFor(t, s, scope)
	if o.Metrics.QueueWait != 120 {
		t.Fatalf("QueueWait = %d after adding 4 non-queue events of 500ms, want it unchanged at 120", o.Metrics.QueueWait)
	}
	// The non-queue events did land — otherwise the assertion above would hold
	// for the boring reason that nothing was written.
	if o.Metrics.Count != 7 {
		t.Fatalf("sample count = %d, want 7; the non-queue events never reached the read model", o.Metrics.Count)
	}
}

// FR-009, SC-005. P95 is nullable because a percentile over a handful of
// samples is a number with no meaning. Both sides of the threshold are asserted:
// with only the null side, raising the threshold to 1000 still passes; with only
// the set side, returning a value unconditionally still passes.
func TestOverviewP95IsNullUntilThereAreEnoughSamples(t *testing.T) {
	const threshold = 20
	s := testStore(t)
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	seedTechnical(t, s, scope, "executor", 10, threshold-1)
	below := overviewFor(t, s, scope)
	if below.Metrics.Count != threshold-1 {
		t.Fatalf("sample count = %d, want %d", below.Metrics.Count, threshold-1)
	}
	if below.Metrics.P95 != nil {
		t.Fatalf("P95 = %d with %d samples; below the threshold it must stay null", *below.Metrics.P95, threshold-1)
	}

	seedTechnical(t, s, scope, "executor", 10, 1)
	at := overviewFor(t, s, scope)
	if at.Metrics.Count != threshold {
		t.Fatalf("sample count = %d, want %d", at.Metrics.Count, threshold)
	}
	if at.Metrics.P95 == nil {
		t.Fatalf("P95 is still null at %d samples; the threshold is supposed to be reached here", threshold)
	}
}
