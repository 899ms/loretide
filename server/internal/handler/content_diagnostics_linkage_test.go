//go:build dbtest

package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The chain a reader walks when something failed:
//
//	audit event --(trace_id)--> technical event --(run_id)--> run --(original_run_id)--> original run
//
// Every hop is already tested on its own. What was never asserted is that the
// identifiers actually line up end to end - each link being non-empty is not
// the same as the chain connecting. These tests start from a real reproduction
// rather than from hand-placed rows, so the ids are whatever the product wrote.
//
// FR-016, FR-017, FR-018 (specs/008-diag-linkage-and-invariants).

// simulateRun drives a real simulation through the handler and returns the run.
func simulateRun(t *testing.T, h Handler, ws, scenario string, seed int, original string) diagnostics.Run {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"scenario": scenario, "seed": seed, "original_run_id": original,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-diagnostics/simulate", string(body)),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	rec := callTraced(t, h, h.ContentDiagnosticSimulate, req)
	if rec.Code != 201 {
		t.Fatalf("simulate %s: status %d, body %s", scenario, rec.Code, rec.Body.String())
	}
	var run diagnostics.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return run
}

func diagnosticScopeFor(ws string) diagnostics.Scope {
	return diagnostics.Scope{Workspace: ws, Actor: testUserID}
}

// firstAuditEventFor returns an audit row the workspace recorded for this run.
func firstAuditEventFor(t *testing.T, h Handler, ws, runID string) diagnostics.Event {
	t.Helper()
	page, err := h.ContentDiagnostics.Store.Query(context.Background(), diagnosticScopeFor(ws),
		diagnostics.Filter{Kind: "audit", Run: runID, Limit: 10})
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if len(page.Events) == 0 {
		t.Fatalf("run %s produced no audit event", runID)
	}
	return page.Events[0]
}

// Hop 1 and hop 2: audit -> trace -> technical -> run.
func TestContentDiagnosticLinkageWalksFromAuditToRun(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	run := simulateRun(t, h, ws, "timeout", 7, "")

	audit := firstAuditEventFor(t, h, ws, run.ID)
	if audit.Trace == "" {
		t.Fatal("audit event carries no trace id, so the chain cannot start")
	}

	// Hop 1: the trace id off the audit row finds technical events.
	page, err := h.ContentDiagnostics.Store.Query(context.Background(), diagnosticScopeFor(ws),
		diagnostics.Filter{Trace: audit.Trace, Limit: 50})
	if err != nil {
		t.Fatalf("query technical by trace: %v", err)
	}
	if len(page.Events) == 0 {
		t.Fatalf("trace %s from the audit row matched no technical event", audit.Trace)
	}

	// Hop 2: every matched event carries this trace, and every event that names
	// a run names THIS run. Asserting "at least one matches" would pass even if
	// the filter had silently degraded and returned the whole table.
	//
	// The one event without a run id is the HTTP boundary record (it has a
	// status and no step): it covers the request, which is wider than the run,
	// so demanding a run id there would be wrong. It is matched on shape rather
	// than skipped by position, so a genuinely unlinked step still fails.
	linkedToRun := 0
	for _, e := range page.Events {
		if e.Trace != audit.Trace {
			t.Errorf("technical event %s carries trace %s, want %s", e.ID, e.Trace, audit.Trace)
		}
		if e.Run == run.ID {
			linkedToRun++
			continue
		}
		if e.Run == "" && e.Status != 0 {
			continue // request boundary event
		}
		t.Errorf("technical event %s (component=%s step=%q status=%d) belongs to run %q, want %q",
			e.ID, e.Component, e.Step, e.Status, e.Run, run.ID)
	}
	if linkedToRun == 0 {
		t.Fatalf("no technical event under trace %s names run %s", audit.Trace, run.ID)
	}

	// And the run the technical events name is readable and is the same run.
	stored, err := h.ContentDiagnostics.Store.GetRun(context.Background(), diagnosticScopeFor(ws), run.ID)
	if err != nil {
		t.Fatalf("get run named by the technical event: %v", err)
	}
	if stored.ID != run.ID {
		t.Errorf("run id %s, want %s", stored.ID, run.ID)
	}
}

// Hop 3: run -> original_run_id -> the original fault.
func TestContentDiagnosticLinkageReachesTheOriginalRun(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	original := simulateRun(t, h, ws, "timeout", 3, "")
	repro := simulateRun(t, h, ws, "timeout", 4, original.ID)

	if repro.ID == original.ID {
		t.Fatal("reproduction reused the original run id")
	}
	if repro.Original != original.ID {
		t.Fatalf("reproduction links to %q, want %q", repro.Original, original.ID)
	}

	reached, err := h.ContentDiagnostics.Store.GetRun(context.Background(), diagnosticScopeFor(ws), repro.Original)
	if err != nil {
		t.Fatalf("get original run: %v", err)
	}
	if reached.ID != original.ID {
		t.Errorf("followed original_run_id to %s, want %s", reached.ID, original.ID)
	}
	if reached.Scenario != original.Scenario {
		t.Errorf("reached a run with scenario %q, want %q", reached.Scenario, original.Scenario)
	}

	// The whole chain, from the reproduction's own audit row back to the original.
	audit := firstAuditEventFor(t, h, ws, repro.ID)
	page, err := h.ContentDiagnostics.Store.Query(context.Background(), diagnosticScopeFor(ws),
		diagnostics.Filter{Trace: audit.Trace, Limit: 50})
	if err != nil || len(page.Events) == 0 {
		t.Fatalf("technical events for the reproduction: %v", err)
	}
	runID := ""
	for _, e := range page.Events {
		if e.Run != "" {
			runID = e.Run
			break
		}
	}
	if runID == "" {
		t.Fatal("no technical event under the reproduction's trace names a run")
	}
	end, err := h.ContentDiagnostics.Store.GetRun(context.Background(), diagnosticScopeFor(ws), runID)
	if err != nil {
		t.Fatalf("get run from the reproduction's technical event: %v", err)
	}
	if end.ID != repro.ID {
		t.Errorf("the reproduction's trace led to run %s, want %s", end.ID, repro.ID)
	}
	if end.Original != original.ID {
		t.Errorf("walking audit -> trace -> run -> original ended at %q, want %q",
			end.Original, original.ID)
	}
}

// FR-018. The event query builds its predicates as ($n=” OR payload->>'k'=$n),
// so an empty identifier does not narrow anything - it matches every row. A
// chain test that did not check this could "connect" simply because each hop
// matched a pile of unrelated records.
func TestContentDiagnosticLinkageDoesNotTreatAnEmptyIdentifierAsAWildcard(t *testing.T) {
	h, ws := newDiagnosticWorkspace(t)
	run := simulateRun(t, h, ws, "timeout", 13, "")

	scope := diagnosticScopeFor(ws)
	all, err := h.ContentDiagnostics.Store.Query(context.Background(), scope,
		diagnostics.Filter{Limit: 100})
	if err != nil {
		t.Fatalf("unfiltered query: %v", err)
	}
	if len(all.Events) == 0 {
		t.Fatal("the run produced no technical events to compare against")
	}

	// An empty trace must not be used as a query key: it returns everything.
	empty, err := h.ContentDiagnostics.Store.Query(context.Background(), scope,
		diagnostics.Filter{Trace: "", Limit: 100})
	if err != nil {
		t.Fatalf("empty-trace query: %v", err)
	}
	if len(empty.Events) != len(all.Events) {
		t.Fatalf("expected an empty trace filter to behave as no filter (%d rows), got %d",
			len(all.Events), len(empty.Events))
	}

	// This is the rule a caller must follow: only a real identifier may be used
	// as a hop. A genuine trace narrows the result; the empty one does not.
	audit := firstAuditEventFor(t, h, ws, run.ID)
	narrowed, err := h.ContentDiagnostics.Store.Query(context.Background(), scope,
		diagnostics.Filter{Trace: audit.Trace, Limit: 100})
	if err != nil {
		t.Fatalf("narrowed query: %v", err)
	}
	for _, e := range narrowed.Events {
		if e.Trace != audit.Trace {
			t.Errorf("narrowed query returned trace %s, want %s", e.Trace, audit.Trace)
		}
	}

	// A well-formed id that belongs to nothing must return zero rows, not all.
	absent, err := h.ContentDiagnostics.Store.Query(context.Background(), scope,
		diagnostics.Filter{Trace: diagnostics.NewID(), Limit: 100})
	if err != nil {
		t.Fatalf("absent-trace query: %v", err)
	}
	if len(absent.Events) != 0 {
		t.Errorf("an unrelated trace matched %d events, want 0", len(absent.Events))
	}
}
