package diagnostics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Isolation negatives for the self-check shapes.
//
// One assertion per shape, not one for the group. A single "shapes are gated"
// test proves that today's nine are blocked; it would stay green on the day
// someone adds a tenth through a path that is not gated, which is the failure
// this feature has to be unable to have.
//
// The rule these pin is the isolation one: a task, an executor, its own
// database and ports (constitution "Development Workflow" and
// docs/development/ai-collaboration.md), alongside principle IX, real executors
// stay disabled. What this feature adds is the ability to produce abnormal
// data - a switch that could forge a "passed" verdict on a production path
// would be worse than the acceptance items it exists to unblock.

func TestNoShapeIsReachableWithTestModeOff(t *testing.T) {
	for _, sh := range shapes {
		t.Run(sh.ID, func(t *testing.T) {
			run, err := Simulate(context.Background(), shapeScope(), "", sh.ID, 42, "test", false)
			if err != ErrDenied {
				t.Fatalf("got err %v, want ErrDenied: %s is reachable with simulation disabled", err, sh.ID)
			}
			if len(run.Events) != 0 || run.ID != "" {
				t.Fatalf("%s produced a run while denied: %d events, id %q", sh.ID, len(run.Events), run.ID)
			}
		})
	}
}

func TestNoShapeIsReachableOutsideTheCallersScope(t *testing.T) {
	denied := []struct {
		name  string
		scope Scope
		acct  string
	}{
		{"other workspace", Scope{Workspace: "", Actor: "u"}, ""},
		{"no actor", Scope{Workspace: "w", Actor: ""}, ""},
		{"account not granted", Scope{Workspace: "w", Actor: "u"}, "ungranted"},
	}
	for _, sh := range shapes {
		for _, d := range denied {
			t.Run(sh.ID+"/"+d.name, func(t *testing.T) {
				run, err := Simulate(context.Background(), d.scope, d.acct, sh.ID, 42, "test", true)
				if err != ErrDenied {
					t.Fatalf("got err %v, want ErrDenied", err)
				}
				if len(run.Events) != 0 || run.ID != "" {
					t.Fatalf("%s produced a run while denied: %d events, id %q", sh.ID, len(run.Events), run.ID)
				}
			})
		}
	}
}

// The service-level twin: nothing is written either. A denial that still left a
// row behind would be a forged run with none of the checks that produce one.
func TestNoShapeWritesARowWhenDenied(t *testing.T) {
	store := testStore(t)
	ctx, scope := context.Background(), shapeScope()
	service := NewService(store, "test", false) // simulation disabled

	before, err := store.Runs(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range shapes {
		if _, err := service.Run(ctx, scope, "", sh.ID, 42, ""); err != ErrDenied {
			t.Fatalf("%s: got err %v, want ErrDenied", sh.ID, err)
		}
	}
	after, err := store.Runs(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("the run table grew from %d to %d rows while every call was denied", len(before), len(after))
	}
}

// The sink failure is the one injection that touches storage rather than just
// building a record, so it gets its own negative: with simulation off there is
// no path to it and the counters do not move.
func TestSinkFailureCannotBeInjectedWithTestModeOff(t *testing.T) {
	store := testStore(t)
	ctx, scope := context.Background(), shapeScope()
	service := NewService(store, "test", false)

	before := struct{ errors, dropped int64 }{store.Log.Errors.Load(), store.Log.Dropped.Load()}
	if _, err := service.Run(ctx, scope, "", "shape_sink_failure", 42, ""); err != ErrDenied {
		t.Fatalf("got err %v, want ErrDenied", err)
	}
	if store.Log.Errors.Load() != before.errors || store.Log.Dropped.Load() != before.dropped {
		t.Fatalf("counters moved from (%d,%d) to (%d,%d) on a denied call: a production path can "+
			"inject a sink failure", before.errors, before.dropped, store.Log.Errors.Load(), store.Log.Dropped.Load())
	}
}

// Regression verdicts cannot be forged from outside either: the shapes that
// produce not_run, failed and undecidable are unreachable when simulation is
// off, so no production path can write a verdict that did not come from a run.
func TestNoVerdictShapeIsReachableWithTestModeOff(t *testing.T) {
	for _, id := range []string{"shape_not_run", "shape_regression_failed", "shape_undecidable"} {
		t.Run(id, func(t *testing.T) {
			if _, err := Simulate(context.Background(), shapeScope(), "", id, 42, "test", false); err != ErrDenied {
				t.Fatalf("got err %v, want ErrDenied: a forged regression verdict is reachable", err)
			}
		})
	}
}

// FR-018 / SC-006: this feature must not widen the sanitize allowlist.
//
// The existing suite already covers the direction that matters most - an
// unlisted value is normalized away (TestLogRegressionSanitizeRules), and the
// four header maps are pinned item by item (TestHeaderAdmissionTiers). Neither
// covers the other direction: that nothing was *added*. Nine new scenarios all
// writing events through Sanitize is exactly the change that would tempt
// someone to add a component or an object type to make one of them fit.

func TestSanitizeCodeTableIsUnchanged(t *testing.T) {
	want := map[string]bool{
		"": true, "AUTHORIZATION_DENIED": true, "FILE_MISSING": true, "FILE_CHANGED": true,
		"DATABASE_UNAVAILABLE": true, "NETWORK_UNAVAILABLE": true, "SEARCH_FAILED": true,
		"MODEL_AUTH": true, "MODEL_QUOTA": true, "OUTPUT_SCHEMA": true, "TIMEOUT": true,
		"CANCELLED": true, "DUPLICATE": true, "LATE_RESULT": true, "INPUT_CONFLICT": true,
		"UI_ERROR": true, "INTERNAL": true, "CLOCK_SKEW": true,
	}
	for code := range codes {
		if !want[code] {
			t.Errorf("error code %q was added to the sanitize table; the allowlist may not grow "+
				"to accommodate a data shape", code)
		}
		delete(want, code)
	}
	for code := range want {
		t.Errorf("error code %q was removed from the sanitize table", code)
	}
}

// The enum allowlists are inline arguments to oneOf, so they are pinned through
// behaviour: every member survives, and a probe of plausible additions - the
// values a new shape might have wanted - is still normalized to "unknown".
func TestSanitizeEnumAllowlistsAreUnchanged(t *testing.T) {
	cases := []struct {
		field   string
		allowed []string
		probe   []string
		get     func(Event) string
		set     func(*Event, string)
	}{
		{
			field: "component",
			// Derived, not typed out again: the module half of this allowlist
			// belongs to scripts/content-boundaries.json, and a second hand-
			// written copy here would be a second place to forget.
			allowed: componentAllowlist,
			probe:   []string{"shape", "simulator", "outbox", "search", "files", "ui"},
			get:     func(e Event) string { return e.Component },
			set:     func(e *Event, v string) { e.Component = v },
		},
		{
			field:   "severity",
			allowed: []string{"debug", "info", "warn", "error"},
			probe:   []string{"trace", "fatal", "critical", "notice"},
			get:     func(e Event) string { return e.Severity },
			set:     func(e *Event, v string) { e.Severity = v },
		},
		{
			field:   "actor_kind",
			allowed: []string{"human", "agent", "system"},
			probe:   []string{"shape", "simulator", "service", "bot"},
			get:     func(e Event) string { return e.ActorKind },
			set:     func(e *Event, v string) { e.ActorKind = v },
		},
		{
			field:   "outcome",
			allowed: []string{"success", "failed", "cancelled", "ignored", "pending"},
			probe:   []string{"skipped", "dropped", "unknown_outcome", "partial"},
			get:     func(e Event) string { return e.Outcome },
			set:     func(e *Event, v string) { e.Outcome = v },
		},
		{
			field:   "action",
			allowed: []string{"simulate", "execute", "query", "export", "client_error", "retry", "cancel", "cleanup", "result"},
			probe:   []string{"shape", "inject", "seed", "fabricate"},
			get:     func(e Event) string { return e.Action },
			set:     func(e *Event, v string) { e.Action = v },
		},
		{
			field:   "object_type",
			allowed: []string{"simulation", "run", "work", "account", "source", "diagnostics"},
			probe:   []string{"shape", "dispatch_outbox", "trace", "span"},
			get:     func(e Event) string { return e.ObjectType },
			set:     func(e *Event, v string) { e.ObjectType = v },
		},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			for _, v := range c.allowed {
				e := Event{}
				c.set(&e, v)
				if got := c.get(Sanitize(e)); got != v {
					t.Errorf("%s=%q no longer survives sanitize (got %q): the allowlist shrank", c.field, v, got)
				}
			}
			for _, v := range c.probe {
				e := Event{}
				c.set(&e, v)
				if got := c.get(Sanitize(e)); got != "unknown" {
					t.Errorf("%s=%q now survives sanitize as %q; the allowlist was widened to let "+
						"a data shape through", c.field, v, got)
				}
			}
		})
	}
}

// The component allowlist has two halves with two owners, and only one of them
// lives in this repository's Go code.
//
// The module names are the content boundary registry's. Before Issue #108 they
// were absent entirely and every ip-profile, workspace-core and topic-planning
// event reached the real sink as "unknown", which is why PR #107's self-check
// could not count refusals by component and had to match on a field
// combination instead.
//
// So this pins the relationship rather than the list: the registry decides
// which modules exist, Sanitize has to admit exactly those, and the union is
// written out once so that widening the allowlist is a visible edit to a test
// and not a quiet addition to a slice.

// contentBoundariesRegistry is the module registry the boundary checker enforces.
type contentBoundariesRegistry struct {
	Modules map[string][]string `json:"modules"`
}

func readModuleRegistry(t *testing.T) []string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve this test's path")
	}
	// server/internal/content/diagnostics -> repository root.
	path := filepath.Clean(filepath.Join(filepath.Dir(self), "..", "..", "..", "..", "scripts", "content-boundaries.json"))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var registry contentBoundariesRegistry
	if err := json.Unmarshal(body, &registry); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	names := make([]string, 0, len(registry.Modules))
	for name := range registry.Modules {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// TestSanitizeComponentAllowlistFollowsTheModuleRegistry fails in both
// directions: a module added to the registry but not admitted here, and a name
// admitted here that the registry does not know about.
func TestSanitizeComponentAllowlistFollowsTheModuleRegistry(t *testing.T) {
	registered := readModuleRegistry(t)
	if len(registered) == 0 {
		// An empty registry would make every assertion below vacuous.
		t.Fatal("the module registry parsed to zero modules; the path or the shape is wrong")
	}

	admitted := slices.Clone(contentModuleComponents)
	slices.Sort(admitted)
	if !slices.Equal(admitted, registered) {
		t.Errorf("contentModuleComponents is %v, the registry has %v; "+
			"scripts/content-boundaries.json is the source and Sanitize has to follow it",
			admitted, registered)
	}

	for _, name := range registered {
		if got := Sanitize(Event{Component: name}).Component; got != name {
			t.Errorf("component=%q does not survive sanitize (got %q); "+
				"events from that module would be recorded as unknown", name, got)
		}
	}
}

// TestSanitizeComponentAllowlistUnionIsPinned is the anti-widening half. The
// test above ties the module names to the registry, which means a module added
// there passes CI on its own - correct, and exactly why the union needs its own
// pin: anything that is NOT a registered module must be added here by hand,
// where it is read.
func TestSanitizeComponentAllowlistUnionIsPinned(t *testing.T) {
	want := []string{
		// Delivery tiers. These name where in the stack something happened.
		"api", "daemon", "database", "executor", "queue", "result", "tool", "web",
		// Content modules, from scripts/content-boundaries.json. "diagnostics"
		// is both a tier and a module and appears once.
		"agent-gateway", "agent-workflow", "diagnostics", "feedback-learning", "idempotency",
		"ip-profile", "knowledge-base", "project-collab", "review-delivery",
		"source-inbox", "topic-planning", "work-editor", "workspace-core",
	}
	slices.Sort(want)

	got := slices.Clone(componentAllowlist)
	slices.Sort(got)
	got = slices.Compact(got)

	if !slices.Equal(got, want) {
		t.Errorf("component allowlist is %v, want %v; widening it lets a data "+
			"shape through the sanitizer and has to be a deliberate edit here", got, want)
	}
}
