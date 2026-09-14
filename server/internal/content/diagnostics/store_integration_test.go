package diagnostics

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type isolatedWorkspaceWriteGuard struct{}

func (isolatedWorkspaceWriteGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("LORETIDE_DIAG_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("isolated PostgreSQL fixture not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "diag_test_" + NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		for _, table := range []string{"content_dispatch_outbox", "content_technical_log", "content_operation_audit", "content_diagnostic_run"} {
			_, _ = admin.Exec(ctx, "DROP TABLE IF EXISTS "+schema+"."+table)
		}
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema)
		admin.Close()
	})
	// 468-473 build the module's three original tables; 474-476 add the durable
	// dispatch outbox and its two indexes.
	for n := 468; n <= 476; n++ {
		paths, _ := filepath.Glob(fmt.Sprintf("../../../migrations/%d_*.up.sql", n))
		if len(paths) != 1 {
			t.Fatalf("migration %d missing", n)
		}
		b, _ := os.ReadFile(paths[0])
		if _, err = pool.Exec(ctx, string(b)); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(b)); err != nil {
			t.Fatal("migration not repeatable", err)
		}
	}
	return NewStore(pool, isolatedWorkspaceWriteGuard{})
}
func TestPostgresAuditRollbackIsolationAndRetention(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u", Accounts: []string{"a"}}
	run, _ := Simulate(ctx, scope, "a", "normal", 1, "test", true)
	if s.CommitRun(ctx, scope, run, true) == nil {
		t.Fatal("audit failure succeeded")
	}
	rows, _ := s.Runs(ctx, scope)
	if len(rows) != 0 {
		t.Fatal("rollback left business data")
	}
	if err := s.CommitRun(ctx, scope, run, false); err != nil {
		t.Fatal(err)
	}
	if s.CommitRun(ctx, scope, run, false) == nil {
		t.Fatal("duplicate committed")
	}
	for _, e := range run.Events {
		s.Technical(ctx, e)
		s.Technical(ctx, e)
	}
	p, err := s.Query(ctx, scope, Filter{Limit: 2})
	if err != nil || len(p.Events) != 2 || !p.More {
		t.Fatal(p, err)
	}
	if _, err = s.GetRun(ctx, Scope{Workspace: "other", Actor: "u"}, run.ID); err == nil {
		t.Fatal("cross brand")
	}
	if _, err = s.GetRun(ctx, Scope{Workspace: "w", Actor: "u", Accounts: []string{"b"}}, run.ID); err == nil {
		t.Fatal("cross account")
	}
	hidden, _ := s.Query(ctx, Scope{Workspace: "w", Actor: "u", Accounts: []string{"b"}}, Filter{})
	if len(hidden.Events) != 0 {
		t.Fatal("query leakage")
	}
	s.MaxLogs = 2
	if err = s.PruneTechnical(ctx); err != nil {
		t.Fatal(err)
	}
	p, err = s.Query(ctx, scope, Filter{After: 1})
	if err != nil || !p.Gap {
		t.Fatal("expired cursor not visible", p, err)
	}
	a, _ := s.Query(ctx, scope, Filter{Kind: "audit"})
	if len(a.Events) != 1 {
		t.Fatal("technical cleanup touched audit")
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.Technical(cancelCtx, run.Events[0])
	if s.Log.Errors.Load() == 0 {
		t.Fatal("sink failure invisible")
	}
	if _, err = s.pool.Exec(ctx, `CREATE FUNCTION simulate_full_disk() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture disk full' USING ERRCODE='53100'; END $$; CREATE TRIGGER diagnostic_full_disk BEFORE INSERT ON content_technical_log FOR EACH ROW EXECUTE FUNCTION simulate_full_disk()`); err != nil {
		t.Fatal(err)
	}
	before := s.Log.Errors.Load()
	s.Technical(ctx, run.Events[0])
	if s.Log.Errors.Load() != before+1 {
		t.Fatal("disk full not reported")
	}
	a, _ = s.Query(ctx, scope, Filter{Kind: "audit"})
	if len(a.Events) != 1 {
		t.Fatal("disk failure modified audit")
	}
	_, _ = s.pool.Exec(ctx, "DROP TRIGGER diagnostic_full_disk ON content_technical_log; DROP FUNCTION simulate_full_disk()")
}
func TestPostgresFullScenariosExportAndHealth(t *testing.T) {
	store := testStore(t)
	service := NewService(store, "test", true)
	ctx := context.Background()
	scope := Scope{Workspace: "w", Actor: "u"}
	for _, scenario := range Scenarios {
		r, err := service.Run(ctx, scope, "", scenario.ID, 42, "")
		if err != nil || r.Regression != "passed" {
			t.Fatalf("%s: %+v %v", scenario.ID, r, err)
		}
		bundle, err := service.Export(ctx, scope, r.ID, true)
		if err != nil || !bundle.Redacted {
			t.Fatal(err)
		}
	}
	service.Heartbeat("daemon", "test", time.Now().Add(-time.Minute))
	service.Heartbeat("files", "test", time.Now().Add(time.Minute))
	o, err := service.Overview(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range o.Components {
		if c.Name == "daemon" && c.Status != "unavailable" {
			t.Fatal("stale heartbeat healthy")
		}
		if c.Name == "files" && c.Status != "unknown" {
			t.Fatal("clock skew ignored")
		}
		if c.Name == "executor" && c.Status != "unverified" {
			t.Fatal("simulation claimed real health")
		}
	}
	service.Enabled = false
	if _, err = service.Run(ctx, scope, "", "normal", 1, ""); err == nil {
		t.Fatal("non-test injection")
	}
}
