//go:build dbtest

package handler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestRunSearchQuery_StatementTimeoutFires exercises the safety net end
// to end against a live Postgres, proving that a deliberately hung
// pg_sleep query is cut off by SET LOCAL statement_timeout (SQLSTATE
// 57014) before the production search cap could ever be reached. Skips
// gracefully if the database is not reachable — mirrors the pattern in
// handler_test.go so CI without a DB stays green.
func TestRunSearchQuery_StatementTimeoutFires(t *testing.T) {
	if testPool == nil {
		t.Skip("DATABASE_URL not set; skipping live-Postgres search timeout test")
	}
	// Override the search timeout for this test only: 200 ms is short
	// enough that pg_sleep(2) is guaranteed to hit it, and this keeps
	// the test snappy. We restore the constant via t.Cleanup so other
	// tests keep the production value.
	oldTimeout := searchStatementTimeout
	setSearchStatementTimeoutForTest(t, 200*time.Millisecond)
	t.Cleanup(func() { setSearchStatementTimeoutForTest(t, oldTimeout) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := runSearchQuery(ctx, testPool, "SELECT pg_sleep(2)", nil, func(rows pgx.Rows) error {
		for rows.Next() {
			// nothing to scan — but iterate so pgx surfaces the
			// server error once the statement_timeout fires
		}
		return rows.Err()
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected statement_timeout error, got nil")
	}
	if !isSearchStatementTimeout(err) {
		t.Fatalf("expected SQLSTATE 57014 (statement_timeout), got: %v", err)
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("statement_timeout did not cut hung query fast enough: elapsed=%s (want <1.5s)", elapsed)
	}
}

func TestRunSearchQuery_WorkMemIsTransactionLocal(t *testing.T) {
	if testPool == nil {
		t.Skip("DATABASE_URL not set; skipping live-Postgres search work_mem test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := testPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire dedicated connection: %v", err)
	}
	defer conn.Release()

	var before string
	if err := conn.QueryRow(ctx, "SHOW work_mem").Scan(&before); err != nil {
		t.Fatalf("read baseline work_mem: %v", err)
	}

	var during string
	err = runSearchQuery(ctx, conn, "SELECT current_setting('work_mem')", nil, func(rows pgx.Rows) error {
		if !rows.Next() {
			return rows.Err()
		}
		return rows.Scan(&during)
	})
	if err != nil {
		t.Fatalf("run search query: %v", err)
	}
	wantDuring := before
	if configured := searchWorkMemValue(); configured != "" {
		wantDuring = configured
	}
	if during != wantDuring {
		t.Fatalf("work_mem during search = %q, want %q", during, wantDuring)
	}

	var after string
	if err := conn.QueryRow(ctx, "SHOW work_mem").Scan(&after); err != nil {
		t.Fatalf("read work_mem after search: %v", err)
	}
	if after != before {
		t.Fatalf("transaction-local work_mem leaked: before=%q after=%q", before, after)
	}
}
