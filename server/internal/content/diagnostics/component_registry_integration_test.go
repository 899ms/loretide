package diagnostics

import (
	"context"
	"testing"
	"time"
)

// Issue #108 from the sink's side.
//
// The unit tests above prove Sanitize admits the module names. This proves the
// consequence that was actually broken: an event from a content module reaches
// the real technical log with its own component, and the panel's and export's
// component filter - which is `payload->>'component'` in SQL, not a Go
// comparison - can select it.
//
// Before the fix every one of these came back as "unknown", so the filter could
// separate nothing and PR #107's self-check had to match on a field combination
// instead of counting by component.
//
// Runs only with LORETIDE_DIAG_TEST_DATABASE_URL set; testStore skips
// otherwise, and a skip is not a pass.
func TestEveryRegisteredModuleKeepsItsComponentThroughTheRealSink(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w-108", Actor: "u-108", Accounts: []string{"a-108"}}
	at := time.Now().UTC()

	// Read from the same slice Sanitize uses, so a module added to the registry
	// is covered here without anyone remembering to add it.
	for _, component := range contentModuleComponents {
		store.Technical(ctx, Event{
			ID: NewID(), Workspace: scope.Workspace, Account: "a-108", Actor: scope.Actor,
			ActorKind: "human", ObjectType: "account", Action: "query", Outcome: "failed",
			Code: "AUTHORIZATION_DENIED", Component: component, Severity: "warn",
			Occurred: at, Received: at,
		})
	}

	page, err := store.Query(ctx, scope, Filter{Limit: 100})
	if err != nil {
		t.Fatalf("query technical log: %v", err)
	}
	if len(page.Events) != len(contentModuleComponents) {
		t.Fatalf("read back %d events, wrote %d", len(page.Events), len(contentModuleComponents))
	}

	seen := map[string]int{}
	for _, e := range page.Events {
		seen[e.Component]++
		if e.Occurred.IsZero() || e.Received.IsZero() {
			t.Errorf("component %q came back with a zero timestamp (occurred=%s received=%s)",
				e.Component, e.Occurred, e.Received)
		}
	}
	if unknown := seen["unknown"]; unknown != 0 {
		t.Errorf("%d events came back as component \"unknown\"; the sanitizer dropped a registered module", unknown)
	}

	for _, component := range contentModuleComponents {
		if seen[component] != 1 {
			t.Errorf("component %q read back %d times, want 1", component, seen[component])
		}
		// The filter is what the panel and the export use. Asserting the stored
		// value without asserting the filter would leave the half that was
		// actually useless untested.
		filtered, err := store.Query(ctx, scope, Filter{Component: component, Limit: 100})
		if err != nil {
			t.Fatalf("query component=%q: %v", component, err)
		}
		if len(filtered.Events) != 1 {
			t.Errorf("filtering by component=%q returned %d events, want 1", component, len(filtered.Events))
			continue
		}
		if got := filtered.Events[0].Component; got != component {
			t.Errorf("filtering by component=%q returned an event from %q", component, got)
		}
	}
}
