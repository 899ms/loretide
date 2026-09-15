package diagnostics

import (
	"context"
	"testing"
)

func shapeScope() Scope { return Scope{Workspace: "w", Actor: "u"} }

func simulateShapeRun(t *testing.T, id string) Run {
	t.Helper()
	run, err := Simulate(context.Background(), shapeScope(), "", id, 42, "test", true)
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return run
}

// Milliseconds from the run's own baseline, so a plan reads the way the
// waterfall draws it.
func startMs(e Event) int64 { return e.Occurred.Sub(shapeBaseline).Milliseconds() }

// SC-003: a shape that cannot name an acceptance item has nobody asking for it.
func TestEveryShapeIsRegisteredAndNamesItsItems(t *testing.T) {
	registered := map[string]bool{}
	for _, s := range Scenarios {
		if s.Kind == "shape" {
			registered[s.ID] = true
		}
	}
	for _, sh := range shapes {
		if !registered[sh.ID] {
			t.Errorf("shape %s is in the shape table but not in Scenarios, so the panel cannot offer it", sh.ID)
		}
		if len(sh.Items) == 0 {
			t.Errorf("shape %s names no acceptance item; nothing asked for it and nothing will notice if it breaks", sh.ID)
		}
		delete(registered, sh.ID)
	}
	for id := range registered {
		t.Errorf("scenario %s is kind %q but has no entry in the shape table, so Simulate would run it down the fault path", id, "shape")
	}
}

func TestEveryShapeScenarioProducesARun(t *testing.T) {
	for _, sh := range shapes {
		t.Run(sh.ID, func(t *testing.T) {
			run := simulateShapeRun(t, sh.ID)
			if len(run.Events) == 0 {
				t.Fatal("no events")
			}
			if run.Scenario != sh.ID {
				t.Errorf("run records scenario %q, want %q", run.Scenario, sh.ID)
			}
		})
	}
}

// Fixed plans, so two runs of one shape agree on time and duration. Ids are
// random here exactly as they are on the fault path.
func TestShapeRunsAreDeterministicInTime(t *testing.T) {
	for _, sh := range shapes {
		t.Run(sh.ID, func(t *testing.T) {
			a, b := simulateShapeRun(t, sh.ID), simulateShapeRun(t, sh.ID)
			if len(a.Events) != len(b.Events) {
				t.Fatalf("event count differs between runs: %d vs %d", len(a.Events), len(b.Events))
			}
			for i := range a.Events {
				if !a.Events[i].Occurred.Equal(b.Events[i].Occurred) || a.Events[i].Duration != b.Events[i].Duration {
					t.Fatalf("event %d is not reproducible", i)
				}
			}
		})
	}
}

// 006-W-3. The assertion pins the amount of overlap, not the fact of it: an
// overlap of a few milliseconds is invisible on screen, and the item would be
// recorded as a rendering defect that does not exist.
func TestShapeConcurrentOverlapsTwoSiblingsVisibly(t *testing.T) {
	run := simulateShapeRun(t, "shape_concurrent")
	bySpan := map[string]Event{}
	for _, e := range run.Events {
		bySpan[e.Span] = e
	}
	best := int64(0)
	var left, right Event
	for i, a := range run.Events {
		for _, b := range run.Events[i+1:] {
			if a.Parent == "" || a.Parent != b.Parent {
				continue // siblings only
			}
			if a.Span == b.Parent || b.Span == a.Parent {
				continue
			}
			overlap := min64(startMs(a)+a.Duration, startMs(b)+b.Duration) - max64(startMs(a), startMs(b))
			if overlap > best {
				best, left, right = overlap, a, b
			}
		}
	}
	if best <= 0 {
		t.Fatal("no two sibling spans overlap in time, so 006-W-3 has nothing to look at")
	}
	shorter := min64(left.Duration, right.Duration)
	if best < 100 || best < shorter/4 {
		t.Errorf("%s and %s overlap by %dms (shorter bar %dms). 006-W-3 asks for an overlap a "+
			"reader can see; under 100ms or under a quarter of the bar it reads as two adjacent bars",
			left.Step, right.Step, best, shorter)
	}
	// Neither may start before its parent: that is the clock-skew rule, which
	// this shape is not testing and must not trip.
	for _, e := range []Event{left, right} {
		if p, ok := bySpan[e.Parent]; ok && startMs(e) < startMs(p) {
			t.Errorf("%s starts before its parent %s and would be flagged as clock skew", e.Step, p.Step)
		}
	}
}

// 006-W-5, and the place this feature is most likely to fail quietly. An
// illegal parent id is blanked by Sanitize; a blanked parent renders as a plain
// top-level span with no anomaly, which differs from an annotated orphan by one
// label on screen and would be recorded as a pass.
func TestShapeOrphanPointsOutsideTheRunAndSurvivesSanitize(t *testing.T) {
	run := simulateShapeRun(t, "shape_orphan")
	spans := map[string]bool{}
	for _, e := range run.Events {
		spans[e.Span] = true
	}
	if spans[shapeOrphanParentSpan] {
		t.Fatalf("%s collided with a real span in this run; the orphan is a normal child and "+
			"006-W-5 would verify a shape it was never shown", shapeOrphanParentSpan)
	}
	orphans := 0
	for _, e := range run.Events {
		if e.Parent == "" || spans[e.Parent] {
			continue
		}
		orphans++
		if e.Parent != shapeOrphanParentSpan {
			t.Errorf("%s has an unexpected dangling parent %q", e.Step, e.Parent)
		}
	}
	if orphans != 1 {
		t.Fatalf("got %d orphan spans, want exactly 1", orphans)
	}
	// The run has already been through Sanitize. Re-running it is the assertion
	// that matters: the parent must still be there afterwards.
	for _, e := range run.Events {
		if e.Parent == shapeOrphanParentSpan && Sanitize(e).Parent != shapeOrphanParentSpan {
			t.Fatal("Sanitize blanked the orphan's parent; the span renders as a plain top-level " +
				"node with no anomaly and the item cannot be verified")
		}
	}
}

// 006-W-6
func TestShapeSingleSpanProducesExactlyOneEvent(t *testing.T) {
	run := simulateShapeRun(t, "shape_single_span")
	if len(run.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(run.Events))
	}
	if run.Events[0].Parent != "" {
		t.Errorf("the only span has parent %q; it must be a root", run.Events[0].Parent)
	}
}

// 006-W-7 / 006-W-8 / 008-O-3. The count has to clear STREAM_EVENT_CAP (200 in
// packages/core/content/diagnostics/contract.ts), and the failing span has to
// sit under an unbroken ancestor chain, because what W-7 verifies is that both
// stay visible while the rest is collapsed.
func TestShapeDeepClearsTheCollapseThresholdWithAnIntactAncestorChain(t *testing.T) {
	run := simulateShapeRun(t, "shape_deep")
	if len(run.Events) <= 200 {
		t.Fatalf("got %d events; the waterfall collapses only above 200, so 006-W-7 has no "+
			"\"N more\" affordance to look at", len(run.Events))
	}
	bySpan := map[string]Event{}
	for _, e := range run.Events {
		bySpan[e.Span] = e
	}
	failing := []Event{}
	for _, e := range run.Events {
		if e.Code != "" {
			failing = append(failing, e)
		}
	}
	if len(failing) == 0 {
		t.Fatal("no span carries an error code, so there is no failing step to keep visible")
	}
	for _, e := range failing {
		depth := 0
		for cursor := e; cursor.Parent != ""; depth++ {
			parent, ok := bySpan[cursor.Parent]
			if !ok {
				t.Fatalf("%s: ancestor chain breaks at %q", e.Step, cursor.Parent)
			}
			if depth > len(run.Events) {
				t.Fatalf("%s: ancestor chain does not terminate", e.Step)
			}
			cursor = parent
		}
		if depth < 2 {
			t.Errorf("%s sits at depth %d; 006-W-7 needs the failure below a chain of ancestors, "+
				"not at the top where keeping it visible is trivial", e.Step, depth)
		}
	}
	// 008-O-3: one object across several pages of the technical log, which the
	// panel fetches 25 at a time.
	for _, e := range run.Events {
		if e.ObjectID != run.ID {
			t.Fatalf("event %s belongs to object %q, not the run; the version card would not "+
				"span pages", e.Step, e.ObjectID)
		}
	}
	if len(run.Events) <= 25 {
		t.Errorf("got %d events for one object; the technical log pages at 25, so 008-O-3 needs more", len(run.Events))
	}
}

// 006-V-3. The verdict comes out of Evaluate, not out of an assignment: writing
// "failed" directly would reduce the item to "the panel can display a string we
// put there".
func TestShapeRegressionFailedIsDecidedByEvaluate(t *testing.T) {
	run := simulateShapeRun(t, "shape_regression_failed")
	if run.Expected == run.Actual {
		t.Fatalf("expected %q equals actual %q, so Evaluate will write passed", run.Expected, run.Actual)
	}
	if run.Regression != "not_run" {
		t.Fatalf("regression is %q before evaluation", run.Regression)
	}
	Evaluate(&run)
	if run.Regression != "failed" {
		t.Fatalf("Evaluate wrote %q, want failed", run.Regression)
	}
}

// 006-V-5. The panel decides "undecidable" from regression=passed on a status
// outside {completed, failed}; describeRegressionVerdict is the ruler and this
// feature does not bend it.
func TestShapeUndecidableIsPassedOnAnUnfinishedStatus(t *testing.T) {
	run := simulateShapeRun(t, "shape_undecidable")
	Evaluate(&run)
	if run.Regression != "passed" {
		t.Fatalf("regression is %q, want passed", run.Regression)
	}
	if run.Status == "completed" || run.Status == "failed" {
		t.Fatalf("status %q is a finished state, so the panel reads the run as passed, not undecidable", run.Status)
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
