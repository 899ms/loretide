package diagnostics

import (
	"context"
	"testing"
)

// Feature 010 (specs/010-diag-evidence-gaps) FR / SC map:
//
//	FR-007  a committed run's audit event carries the run's own operation_id
//	        and trace_id
//	FR-010  Query / Runs / GetRun write nothing                    (SC-006)
//
// These close the DIAG-05 and DIAG-08 rows of the acceptance mapping. Both need
// a real transaction, so both skip without LORETIDE_DIAG_TEST_DATABASE_URL — and
// a skip is recorded as not run, never as a pass.

// contentTables is every table the diagnostics module owns. The read-only
// assertion counts all of them: a write that landed in a table the test forgot
// to look at would otherwise pass unnoticed.
var contentTables = []string{
	"content_diagnostic_run",
	"content_operation_audit",
	"content_technical_log",
	"content_dispatch_outbox",
}

func tableCounts(t *testing.T, s *Store) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range contentTables {
		var n int
		if err := s.Pool().QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// FR-007. CommitRun blanks run.Events before storing the run row, so the
// correlation ids reach the database only through the audit event written in the
// same transaction — copied there from run.Events[0].
//
// The assertion is equality, not "both are non-empty". A copy that produced a
// fresh random id would satisfy non-empty and would be exactly the defect worth
// catching: the panel would show a trace that leads nowhere.
func TestCommittedRunWritesBackItsOperationAndTraceIDs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	run, err := Simulate(ctx, scope, "a", "normal", 7, "test", true)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if len(run.Events) == 0 {
		t.Fatal("the simulated run produced no events to correlate")
	}
	wantTrace, wantOperation := run.Events[0].Trace, run.Events[0].Operation
	if wantTrace == "" || wantOperation == "" {
		t.Fatalf("the run itself carries no correlation ids (trace %q, operation %q)", wantTrace, wantOperation)
	}

	if err = s.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatalf("commit run: %v", err)
	}

	page, err := s.Query(ctx, scope, Filter{Kind: "audit", Run: run.ID, Limit: 10})
	if err != nil {
		t.Fatalf("read the audit trail back: %v", err)
	}
	if len(page.Events) == 0 {
		t.Fatalf("committing run %s wrote no audit event", run.ID)
	}
	for _, e := range page.Events {
		if e.Trace != wantTrace {
			t.Errorf("written-back trace_id = %q, want %q (the run's own)", e.Trace, wantTrace)
		}
		if e.Operation != wantOperation {
			t.Errorf("written-back operation_id = %q, want %q (the run's own)", e.Operation, wantOperation)
		}
		if e.Run != run.ID {
			t.Errorf("written-back run_id = %q, want %q", e.Run, run.ID)
		}
	}
}

// FR-010, SC-006. Reading is reading. The three read methods are held to it by
// counting every row the module owns before and after: a write of any kind, to
// any of its tables, moves one of these numbers.
//
// Counting rows rather than opening a read-only transaction is deliberate — a
// read-only transaction would prove that transaction is read-only, while these
// methods open their own connections and would be unaffected by it.
func TestReadPathsDoNotWrite(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}

	// Give the read paths something to find; a read over an empty database
	// could be "read-only" for the wrong reason.
	run, err := Simulate(ctx, scope, "a", "normal", 11, "test", true)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if err = s.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatalf("commit run: %v", err)
	}
	s.Technical(ctx, run.Events[0])

	before := tableCounts(t, s)

	if _, err = s.Query(ctx, scope, Filter{Limit: 50}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if _, err = s.Query(ctx, scope, Filter{Kind: "audit", Limit: 50}); err != nil {
		t.Fatalf("Query(audit): %v", err)
	}
	runs, err := s.Runs(ctx, scope)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("Runs returned nothing; the read paths were exercised over an empty result")
	}
	if _, err = s.GetRun(ctx, scope, run.ID); err != nil {
		t.Fatalf("GetRun: %v", err)
	}

	after := tableCounts(t, s)
	for _, table := range contentTables {
		if before[table] != after[table] {
			t.Errorf("%s went from %d rows to %d across Query/Runs/GetRun; a read path is writing", table, before[table], after[table])
		}
	}
}
