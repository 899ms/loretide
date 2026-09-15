package diagnostics

// Diagnostics self-check data shapes.
//
// These scenarios exist for one reason: a batch of manual acceptance items
// could not be run because the simulator could not produce the data shape they
// describe - concurrent spans, an orphan span, a single-span run, a run past
// the waterfall's collapse threshold, regression verdicts other than passed, an
// empty module, and non-zero sink counters. They are not business faults, and
// docs/13 section 7 does not enumerate them; Scenario.Kind keeps that
// distinction a fact in the struct rather than a sentence in a document.
//
// They are built here instead of inside Simulate's step loop on purpose. That
// loop makes every span the parent of the next one and lays the steps end to
// end in time, which is precisely why these shapes are unreachable through it.
// Adding branches to it would turn a loop that carries the deterministic
// semantics of 16 business faults into a general event generator, and the
// regression tests that pin those faults would start going red for changes to
// these shapes.
//
// Contract: specs/012-diag-simulator-shapes/contracts/simulator-shapes.md

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// shapeOrphanParentSpan is the parent of the orphan shape's dangling span. It
// is a fixed literal rather than a generated id for a reason that is not
// cosmetic: a generated id can collide with a real span in the same run, and
// the failure mode of a collision is not an error - the span quietly becomes a
// normal child and 006-W-5 verifies a shape it was never shown. A constant
// makes "it is not one of ours" an assertion instead of a probability.
//
// It must also stay a legal span id. Sanitize blanks anything that fails
// hexID, and a blanked parent renders as a plain top-level span with no anomaly
// - which on screen differs from an annotated orphan by one label.
const shapeOrphanParentSpan = "00000000dead0000"

// shapeDeepSpanCount sits above STREAM_EVENT_CAP (200) in
// packages/core/content/diagnostics/contract.ts, which is the waterfall's
// collapse threshold. Below it there is no "N more" affordance to verify.
const shapeDeepSpanCount = 250

// shapeBaseline is Simulate's virtual clock origin. Shapes share it so a
// reader comparing two runs is not also comparing two epochs.
var shapeBaseline = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// shape is the per-scenario description. It is deliberately not a field on Run:
// Run is serialized to the panel, so every field added to it is another outward
// shape change needing its own schema update and malformed-response test.
type shape struct {
	ID string
	// Items are the acceptance item numbers this shape makes runnable. A shape
	// that cannot name one has nobody asking for it.
	Items []string
	// SkipEvaluate leaves Regression at its "not_run" initial value so it
	// reaches the database. Evaluate only ever writes passed or failed, so
	// this is the only way not_run is observable outside memory (006-V-1).
	SkipEvaluate bool
	// FailSink forces this run's technical log writes to fail, which is what
	// raises sink_errors and dropped. Scoped to the run: the next run writes
	// normally, so recovery needs no action and no instance rebuild.
	FailSink bool
	// Status overrides Run.Status. Used by the undecidable shape, which needs a
	// status outside {completed, failed} while Regression reads passed.
	Status string
	// EmptyModule sets Run.Module to "". A plain Module string cannot express
	// this: "" would mean both "leave it alone" and "make it empty".
	EmptyModule bool
}

var shapes = []shape{
	{ID: "shape_concurrent", Items: []string{"006-W-3"}},
	{ID: "shape_orphan", Items: []string{"006-W-5"}},
	{ID: "shape_single_span", Items: []string{"006-W-6"}},
	{ID: "shape_deep", Items: []string{"006-W-7", "006-W-8", "008-O-3"}},
	{ID: "shape_not_run", Items: []string{"006-V-1"}, SkipEvaluate: true},
	{ID: "shape_regression_failed", Items: []string{"006-V-3"}},
	{ID: "shape_undecidable", Items: []string{"006-V-5"}, Status: "running"},
	{ID: "shape_no_module", Items: []string{"006-L-4"}, EmptyModule: true},
	{ID: "shape_sink_failure", Items: []string{"002-V11-1", "002-V11-2", "002-V05-15"}, FailSink: true},
}

func shapeByID(id string) (shape, bool) {
	for _, s := range shapes {
		if s.ID == id {
			return s, true
		}
	}
	return shape{}, false
}

// shapeSpan is one planned span. Times are milliseconds from shapeBaseline, so
// a plan reads as a picture of the waterfall it produces.
type shapeSpan struct {
	component string
	// parent indexes into the plan; -1 is a root span.
	parent int
	// parentSpan overrides the resolved parent id. Only the orphan shape sets
	// it, to point outside the run.
	parentSpan string
	startMs    int64
	durationMs int64
	code       string
}

func shapePlan(id string) []shapeSpan {
	switch id {
	case "shape_concurrent":
		// executor [150,550) and tool [350,750) are siblings under api and
		// overlap by 200ms - half of each bar. 006-W-3 asks for an overlap a
		// reader can see, so the assertion pins the amount, not the fact.
		// Both start after their parent, so this shape does not also trip the
		// clock-skew rule it is not testing.
		return []shapeSpan{
			{component: "web", parent: -1, startMs: 0, durationMs: 100},
			{component: "api", parent: 0, startMs: 100, durationMs: 700},
			{component: "executor", parent: 1, startMs: 150, durationMs: 400},
			{component: "tool", parent: 1, startMs: 350, durationMs: 400},
		}
	case "shape_orphan":
		return []shapeSpan{
			{component: "web", parent: -1, startMs: 0, durationMs: 50},
			{component: "api", parent: 0, startMs: 50, durationMs: 50},
			{component: "daemon", parent: -1, parentSpan: shapeOrphanParentSpan, startMs: 100, durationMs: 50},
		}
	case "shape_single_span":
		return []shapeSpan{{component: "web", parent: -1, startMs: 0, durationMs: 25}}
	case "shape_deep":
		// A four-deep spine carrying the failure, then leaves under api to get
		// past the collapse threshold. 006-W-7 verifies that the failing span
		// and every one of its ancestors stay visible while collapsed, so the
		// failure has to sit below an intact chain rather than at the top.
		plan := []shapeSpan{
			{component: "web", parent: -1, startMs: 0, durationMs: 3000},
			{component: "api", parent: 0, startMs: 10, durationMs: 2900},
			{component: "daemon", parent: 1, startMs: 20, durationMs: 2800},
			{component: "executor", parent: 2, startMs: 30, durationMs: 2700, code: "TIMEOUT"},
		}
		for i := len(plan); i < shapeDeepSpanCount; i++ {
			plan = append(plan, shapeSpan{component: "tool", parent: 1, startMs: int64(100 + i*10), durationMs: 5})
		}
		return plan
	default:
		// The verdict and metadata shapes need no particular topology; a short
		// ordinary chain keeps the waterfall out of the way of what they are
		// there to show.
		return []shapeSpan{
			{component: "web", parent: -1, startMs: 0, durationMs: 20},
			{component: "api", parent: 0, startMs: 20, durationMs: 30},
		}
	}
}

// simulateShape builds a self-check run. It is reached only through Simulate,
// so it inherits that function's isolation gate: there is no second switch, no
// environment variable of its own, and no assembly-time parameter. Nothing here
// can be injected from a production path.
func simulateShape(ctx context.Context, scope Scope, account string, sh shape, expected string, seed int64, build string) (Run, error) {
	snapshot := Snapshot{ConfigVersion: "fixture-v1", PersonaRef: "fixture:persona", SOPVersion: "fixture-v1", SkillVersion: "fixture-v1", RuleVersion: "fixture-v1", Executor: "simulator", ExecutorVersion: "1", Scope: "all", Preference: "all", Required: []string{"fixture-a"}, Excluded: []string{}, Grants: []string{"fixture-a"}, Hashes: map[string]string{"fixture-a": "sha256:fixture-v1"}, Temperature: 0, Budget: 0, Timeout: 30000}
	run := Run{ID: NewID(), Workspace: scope.Workspace, Account: account, Actor: scope.Actor, Scenario: sh.ID, Seed: seed, Created: time.Now().UTC(), Snapshot: CloneSnapshot(snapshot), Events: []Event{}, Gaps: []string{}, Status: "completed", Expected: expected, Regression: "not_run", Module: "diagnostics", Build: safeToken(build), Test: true}
	if sh.EmptyModule {
		run.Module = ""
	}

	plan := shapePlan(sh.ID)
	operation := NewID()
	root, _ := Child(ctx)

	// Span ids come from the same Child helper the fault path uses, so every
	// span in the run shares one trace id and the ids are the shape the rest of
	// the system already expects.
	spanCtx := make([]context.Context, len(plan))
	spanID := make([]string, len(plan))
	for i, p := range plan {
		from := root
		if p.parent >= 0 {
			from = spanCtx[p.parent]
		}
		child, _ := Child(from)
		spanCtx[i] = child
		spanID[i] = trace.SpanContextFromContext(child).SpanID().String()
	}

	code := ""
	for i, p := range plan {
		parent := p.parentSpan
		if parent == "" && p.parent >= 0 {
			parent = spanID[p.parent]
		}
		if p.code != "" {
			code = p.code
		}
		actorKind := "system"
		switch {
		case i == 0:
			actorKind = "human"
		case p.component == "executor":
			actorKind = "agent"
		}
		e := Event{
			ID: NewID(), Sequence: int64(i + 1),
			Occurred: shapeBaseline.Add(time.Duration(p.startMs) * time.Millisecond), Received: run.Created,
			Workspace: scope.Workspace, Account: account, Actor: scope.Actor, ActorKind: actorKind,
			ObjectType: "simulation", ObjectID: run.ID, Version: "1",
			Action: "simulate", Outcome: "success", Code: p.code,
			Operation: operation,
			Trace:     trace.SpanContextFromContext(spanCtx[i]).TraceID().String(),
			Span:      spanID[i], Parent: parent,
			Run: run.ID, Attempt: 1,
			Step: fmt.Sprintf("%02d-%s", i, p.component), Component: p.component,
			Severity: "info", Duration: p.durationMs, Build: run.Build, Test: true,
		}
		if p.code != "" {
			e.Outcome = "failed"
			e.Severity = "error"
		}
		run.Events = append(run.Events, Sanitize(e))
	}

	run.Actual = code
	if code != "" {
		run.Status = "failed"
	}
	if sh.Status != "" {
		run.Status = sh.Status
	}
	return run, nil
}
