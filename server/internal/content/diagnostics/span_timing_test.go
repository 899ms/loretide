package diagnostics

import (
	"context"
	"testing"
	"time"
)

// occurred_at is the START of the step the event represents; a step occupies
// [occurred_at, occurred_at + duration_ms).
//
// That sentence had never been written down. The simulator advanced its clock
// BEFORE stamping Occurred, so it emitted the END of each step, while the
// waterfall reads Occurred as a start and draws [occurred, occurred+duration].
// Both ends were self-consistent; together they shifted every bar right by its
// own duration and invented gaps between steps that are actually contiguous.
// Run 93edf840 showed 5ms and 9ms of waiting that never happened, and put
// executor visually inside daemon when it actually runs after it.
//
// Contract: specs/011-diag-panel-time-and-jump/contracts/span-timing.md

func simulateForTiming(t *testing.T, scenario string) Run {
	t.Helper()
	run, err := Simulate(context.Background(), Scope{Workspace: "w", Actor: "u"}, "", scenario, 7, "test", true)
	if err != nil {
		t.Fatalf("simulate %s: %v", scenario, err)
	}
	return run
}

// T1. Adjacent steps meet end to start, with zero tolerance.
//
// clock_skew is excluded deliberately, and the exclusion is the point: that
// scenario exists to violate this rule. A T1 that covered it would force the
// clock-skew fix to be reverted. See contracts/span-timing.md T3.
func TestSimulatedStepsAreContiguousInTime(t *testing.T) {
	for _, scenario := range Scenarios {
		if scenario.ID == "clock_skew" {
			continue // the one scenario whose purpose is out-of-order time
		}
		if scenario.Kind != "fault" {
			// Self-check data shapes (specs/012) are branching trees, not the
			// linear step pipeline this invariant describes: shape_concurrent
			// overlaps two siblings on purpose and shape_deep nests. They stay
			// inside TestOnlyClockSkewProducesOutOfOrderTime below, which is
			// the guarantee that does apply to them.
			continue
		}
		t.Run(scenario.ID, func(t *testing.T) {
			run := simulateForTiming(t, scenario.ID)
			if len(run.Events) < 2 {
				return // a single-step run has no adjacent pair; not a failure
			}
			for i := 0; i+1 < len(run.Events); i++ {
				cur, next := run.Events[i], run.Events[i+1]
				want := cur.Occurred.Add(time.Duration(cur.Duration) * time.Millisecond)
				if !next.Occurred.Equal(want) {
					t.Errorf("step %s -> %s: next starts at %s, want %s (prev start %s + duration %dms). "+
						"A mismatch here is drawn as a gap or an overlap in the waterfall.",
						cur.Step, next.Step,
						next.Occurred.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano),
						cur.Occurred.Format(time.RFC3339Nano), cur.Duration)
				}
			}
		})
	}
}

// The first event must start at the run's own baseline, not one duration in.
// Without this, every bar could still be contiguous while the whole chart sat
// shifted to the right.
func TestFirstSimulatedStepStartsAtTheRunBaseline(t *testing.T) {
	run := simulateForTiming(t, "normal")
	if len(run.Events) == 0 {
		t.Fatal("normal scenario produced no events")
	}
	first := run.Events[0]
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !first.Occurred.Equal(base) {
		t.Errorf("first step starts at %s, want the virtual-time baseline %s; "+
			"a non-zero offset means Occurred is still being stamped after the clock advances",
			first.Occurred.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano))
	}
}

// C1 and C4. clock_skew must produce genuinely out-of-order time, and no other
// scenario may. Before this feature the scenario only set an error code, so the
// waterfall's clockSkew rule - which fires when a child starts before its
// parent - could never match it, and text093 never appeared.
func TestClockSkewScenarioProducesOutOfOrderTime(t *testing.T) {
	run := simulateForTiming(t, "clock_skew")
	if len(run.Events) < 2 {
		t.Fatal("clock_skew produced too few events to be out of order")
	}
	bySpan := map[string]Event{}
	for _, e := range run.Events {
		bySpan[e.Span] = e
	}
	skewed := 0
	for _, e := range run.Events {
		parent, ok := bySpan[e.Parent]
		if !ok {
			continue
		}
		if e.Occurred.Before(parent.Occurred) {
			skewed++
		}
	}
	if skewed == 0 {
		t.Error("no event starts before its parent, so the waterfall's clockSkew rule cannot fire " +
			"and text093 will never be shown - this is exactly the defect 006-W-4 reported")
	}
}

func TestOnlyClockSkewProducesOutOfOrderTime(t *testing.T) {
	for _, scenario := range Scenarios {
		if scenario.ID == "clock_skew" {
			continue
		}
		t.Run(scenario.ID, func(t *testing.T) {
			run := simulateForTiming(t, scenario.ID)
			bySpan := map[string]Event{}
			for _, e := range run.Events {
				bySpan[e.Span] = e
			}
			for _, e := range run.Events {
				parent, ok := bySpan[e.Parent]
				if ok && e.Occurred.Before(parent.Occurred) {
					t.Errorf("step %s starts before its parent %s: only clock_skew may do that",
						e.Step, parent.Step)
				}
			}
		})
	}
}
