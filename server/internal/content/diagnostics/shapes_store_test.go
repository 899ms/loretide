package diagnostics

import (
	"context"
	"testing"
)

// The shapes that only exist once they are in the database. Each of these
// asserts against what was stored, not against what Simulate returned in
// memory - for shape_not_run that distinction is the whole test.

func shapeService(t *testing.T) *Service {
	t.Helper()
	return NewService(testStore(t), "test", true)
}

// 006-W-7 read side. Producing 250 spans is not enough on its own: the run
// detail path is GetRun -> Query(Filter{Run:id, Limit:100}) and Store.Query
// clamps Limit at 100, while the waterfall collapses only above 200. With the
// clamp in place the panel receives 100 rows, never collapses, and the item
// stays unrunnable no matter how many spans the run has.
func TestGetRunReturnsEveryEventOfALargeRun(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	run, err := service.Run(ctx, scope, "", "shape_deep", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Events) <= 200 {
		t.Fatalf("the simulated run has %d events; it must exceed 200 before the read path matters", len(run.Events))
	}
	stored, err := service.Store.GetRun(ctx, scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Events) != len(run.Events) {
		t.Fatalf("GetRun returned %d of %d events. The waterfall collapses above 200, so a "+
			"truncated read means 006-W-7 and 006-W-8 cannot be verified however many spans the "+
			"run actually has.", len(stored.Events), len(run.Events))
	}
	seen := map[int64]bool{}
	for _, e := range stored.Events {
		if seen[e.Sequence] {
			t.Fatalf("sequence %d returned twice; the pages overlap", e.Sequence)
		}
		seen[e.Sequence] = true
	}
	for i := 1; i < len(stored.Events); i++ {
		if stored.Events[i].Sequence <= stored.Events[i-1].Sequence {
			t.Fatalf("events are out of order at index %d; paging lost the ordering", i)
		}
	}
}

// A run that fits in one page must not change behaviour or gain a second query.
func TestGetRunStillReturnsASmallRunWhole(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	run, err := service.Run(ctx, scope, "", "normal", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.Store.GetRun(ctx, scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Events) != len(run.Events) {
		t.Fatalf("GetRun returned %d of %d events for an ordinary run", len(stored.Events), len(run.Events))
	}
}

// 006-V-1. Regression starts at "not_run" in memory, so asserting on Simulate's
// return value would pass without a line of this feature being written. The
// only assertion that means anything is the one against the stored row.
func TestShapeNotRunReachesTheDatabaseUnevaluated(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	run, err := service.Run(ctx, scope, "", "shape_not_run", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.Store.GetRun(ctx, scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Regression != "not_run" {
		t.Fatalf("stored regression is %q, want not_run. Evaluate only ever writes passed or "+
			"failed, so anything else here means it was not skipped and the panel can never be "+
			"shown an unevaluated run.", stored.Regression)
	}
}

// Every other shape must still go through Evaluate: skipping is opt-in, and a
// shape that silently stopped being evaluated would show "not run" forever.
func TestOnlyTheNotRunShapeSkipsEvaluation(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	for _, sh := range shapes {
		if sh.SkipEvaluate {
			continue
		}
		t.Run(sh.ID, func(t *testing.T) {
			run, err := service.Run(ctx, scope, "", sh.ID, 42, "")
			if err != nil {
				t.Fatal(err)
			}
			stored, err := service.Store.GetRun(ctx, scope, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Regression != "passed" && stored.Regression != "failed" {
				t.Fatalf("stored regression is %q; Evaluate was not run", stored.Regression)
			}
		})
	}
}

// 006-L-4, the half that could not be verified before: build comes from an
// environment variable and is empty when it is unset, but module was hardcoded.
func TestShapeNoModuleStoresAnEmptyModule(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	run, err := service.Run(ctx, scope, "", "shape_no_module", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.Store.GetRun(ctx, scope, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Module != "" {
		t.Fatalf("stored module is %q, want empty", stored.Module)
	}
	for _, sh := range shapes {
		if sh.EmptyModule {
			continue
		}
		other, err := service.Run(ctx, scope, "", sh.ID, 42, "")
		if err != nil {
			t.Fatal(err)
		}
		if other.Module != "diagnostics" {
			t.Fatalf("%s reports module %q; only the no-module shape may be empty", sh.ID, other.Module)
		}
	}
}

// 002-V11-1, 002-V11-2, 002-V05-15. Before this shape the only routes to a
// non-zero counter were ~112 runs to fill the 1000-entry ring, or making a
// table unwritable - the destructive step that writes off the rest of the
// instance, which is why these three items could never be run in the same pass
// as the other overview checks.
func TestShapeSinkFailureRaisesBothCountersAndRecoversByItself(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()

	before, err := service.Overview(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Run(ctx, scope, "", "shape_sink_failure", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	during, err := service.Overview(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if during.Metrics.SinkErrors <= before.Metrics.SinkErrors {
		t.Fatalf("sink_errors stayed at %d; 002-V11-1 has no non-zero value to look at", during.Metrics.SinkErrors)
	}
	if during.Metrics.Dropped <= before.Metrics.Dropped {
		t.Fatalf("dropped stayed at %d; 002-V11-2 and 002-V05-15 have no non-zero value to look at", during.Metrics.Dropped)
	}

	// Recovery is the half that matters for the runbook: if the failure stuck,
	// every remaining acceptance item on this instance would run in a failed
	// state and the operator would have to rebuild it.
	if _, err = service.Run(ctx, scope, "", "normal", 42, ""); err != nil {
		t.Fatal(err)
	}
	after, err := service.Overview(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if after.Metrics.SinkErrors != during.Metrics.SinkErrors {
		t.Errorf("sink_errors moved from %d to %d during an ordinary run; the injected failure "+
			"outlived its own run and the instance would need rebuilding",
			during.Metrics.SinkErrors, after.Metrics.SinkErrors)
	}
	if after.Metrics.Dropped != during.Metrics.Dropped {
		t.Errorf("dropped moved from %d to %d during an ordinary run", during.Metrics.Dropped, after.Metrics.Dropped)
	}

	// What failed is the technical log, not the audit. "An audit write failure
	// rolls the whole thing back" is an invariant this feature must not touch,
	// so the run's audit row has to be there.
	audit, err := service.Store.Query(ctx, scope, Filter{Kind: "audit", Run: run.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Events) == 0 {
		t.Fatal("the run has no audit event: the sink shape reached the audit path, which it " +
			"must never do - audit write failure means the whole run rolls back")
	}
}

// Only the shape that declares FailSink may move the counters. Without this,
// a stray true would degrade every run on the instance and nothing would say so.
func TestOrdinaryRunsLeaveTheSinkCountersAlone(t *testing.T) {
	service := shapeService(t)
	ctx, scope := context.Background(), shapeScope()
	for _, sh := range shapes {
		if sh.FailSink {
			continue
		}
		t.Run(sh.ID, func(t *testing.T) {
			before, err := service.Overview(ctx, scope)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Run(ctx, scope, "", sh.ID, 42, ""); err != nil {
				t.Fatal(err)
			}
			after, err := service.Overview(ctx, scope)
			if err != nil {
				t.Fatal(err)
			}
			if after.Metrics.SinkErrors != before.Metrics.SinkErrors || after.Metrics.Dropped != before.Metrics.Dropped {
				t.Errorf("%s moved sink_errors %d->%d and dropped %d->%d; only the sink shape may",
					sh.ID, before.Metrics.SinkErrors, after.Metrics.SinkErrors,
					before.Metrics.Dropped, after.Metrics.Dropped)
			}
		})
	}
}
