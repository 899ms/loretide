package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// WorkspaceWriteGuard is the workspace-core half of the delete/write protocol.
// Implementations may only lock through the supplied transaction; the Store
// keeps that transaction open through the diagnostic write and commit.
type WorkspaceWriteGuard interface {
	LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error
}

type Store struct {
	pool      *pgxpool.Pool
	guard     WorkspaceWriteGuard
	Log       *LogBuffer
	MaxLogs   int
	Retention time.Duration
}

func NewStore(pool *pgxpool.Pool, guard WorkspaceWriteGuard) *Store {
	return &Store{pool: pool, guard: guard, Log: NewLogBuffer(1000), MaxLogs: 10000, Retention: 7 * 24 * time.Hour}
}

type Page struct {
	Events []Event `json:"events"`
	Cursor int64   `json:"cursor"`
	Gap    bool    `json:"gap"`
	More   bool    `json:"has_more"`
}
type Filter struct {
	After     int64
	Limit     int
	Kind      string
	Trace     string
	Component string
	Severity  string
	Code      string
	Run       string
	From      time.Time
	Until     time.Time
}

func (s *Store) withWorkspaceWrite(ctx context.Context, workspaceID string, write func(pgx.Tx) error) error {
	if s.pool == nil || s.guard == nil {
		return ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if err = s.guard.LockForContentDiagnosticWrite(ctx, tx, workspaceID); err != nil {
		return err
	}
	if err = write(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) CommitRun(ctx context.Context, scope Scope, run Run, failAudit bool) error {
	if !scope.Allows(run.Workspace, run.Account) || run.Actor != scope.Actor {
		return ErrDenied
	}
	saved := run
	saved.Events = nil
	body, _ := json.Marshal(saved)
	return s.withWorkspaceWrite(ctx, run.Workspace, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO content_diagnostic_run(run_id,workspace_id,account_id,payload) VALUES($1,$2,$3,$4)", run.ID, run.Workspace, run.Account, body); err != nil {
			return ErrConflict
		}
		e := Sanitize(Event{ID: NewID(), Workspace: run.Workspace, Account: run.Account, Actor: run.Actor, ActorKind: "human", ObjectType: "simulation", ObjectID: run.ID, Version: "1", Action: "simulate", Outcome: "success", Run: run.ID, Code: run.Actual, Occurred: run.Created, Received: time.Now().UTC(), Component: "diagnostics", Severity: "info", Test: true, Build: run.Build})
		if len(run.Events) > 0 {
			e.Trace = run.Events[0].Trace
			e.Operation = run.Events[0].Operation
		}
		if run.Status != "completed" {
			e.Outcome = "failed"
		}
		if failAudit {
			if _, err := tx.Exec(ctx, "SELECT 1 / 0"); err != nil {
				return ErrUnavailable
			}
		}
		return s.appendAudit(ctx, tx, e)
	})
}
func (s *Store) appendAudit(ctx context.Context, tx pgx.Tx, e Event) error {
	payload, _ := json.Marshal(Sanitize(e))
	_, err := tx.Exec(ctx, "INSERT INTO content_operation_audit(event_id,workspace_id,account_id,payload) VALUES($1,$2,$3,$4)", e.ID, e.Workspace, e.Account, payload)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Store) Audit(ctx context.Context, scope Scope, e Event) error {
	if !scope.Allows(e.Workspace, e.Account) || scope.Actor != e.Actor {
		return ErrDenied
	}
	return s.withWorkspaceWrite(ctx, e.Workspace, func(tx pgx.Tx) error { return s.appendAudit(ctx, tx, e) })
}
func (s *Store) Technical(ctx context.Context, e Event) {
	e = Sanitize(e)
	payload, _ := json.Marshal(e)
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err := s.withWorkspaceWrite(ctx, e.Workspace, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO content_technical_log(event_id,workspace_id,account_id,payload) VALUES($1,$2,$3,$4) ON CONFLICT(event_id) DO NOTHING", e.ID, e.Workspace, e.Account, payload)
		if err != nil {
			return ErrUnavailable
		}
		return nil
	})
	if err != nil {
		s.Log.Errors.Add(1)
		s.Log.Dropped.Add(1)
		return
	}
	s.Log.Append(e)
	_ = s.PruneTechnical(ctx)
}
func (s *Store) Query(ctx context.Context, scope Scope, f Filter) (Page, error) {
	p := Page{Events: []Event{}, Cursor: f.After}
	if !scope.Allows(scope.Workspace, "") {
		return p, ErrDenied
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.After < 0 {
		return p, ErrConflict
	}
	table := "content_technical_log"
	if f.Kind == "audit" {
		table = "content_operation_audit"
	} else if f.Kind != "" && f.Kind != "technical" {
		return p, ErrConflict
	}
	accounts := scope.Accounts
	if accounts == nil {
		accounts = []string{}
	}
	var oldest int64
	err := s.pool.QueryRow(ctx, "SELECT COALESCE(min(sequence),0) FROM "+table+" WHERE workspace_id=$1 AND (account_id='' OR account_id=ANY($2))", scope.Workspace, accounts).Scan(&oldest)
	if err != nil {
		return p, ErrUnavailable
	}
	p.Gap = f.After > 0 && oldest > f.After+1
	rows, err := s.pool.Query(ctx, "SELECT sequence,payload FROM "+table+" WHERE workspace_id=$1 AND (account_id='' OR account_id=ANY($2)) AND sequence>$3 AND ($4='' OR payload->>'trace_id'=$4) AND ($5='' OR payload->>'component'=$5) AND ($6='' OR payload->>'severity'=$6) AND ($7='' OR payload->>'error_code'=$7) AND ($8='' OR payload->>'run_id'=$8) AND ($9::timestamptz IS NULL OR received_at >= $9) AND ($10::timestamptz IS NULL OR received_at <= $10) ORDER BY sequence LIMIT $11", scope.Workspace, accounts, f.After, f.Trace, f.Component, f.Severity, f.Code, f.Run, nullableTime(f.From), nullableTime(f.Until), f.Limit+1)
	if err != nil {
		return p, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var data []byte
		if err = rows.Scan(&seq, &data); err != nil {
			return p, ErrUnavailable
		}
		var e Event
		if json.Unmarshal(data, &e) != nil {
			return p, ErrUnavailable
		}
		e.Sequence = seq
		p.Events = append(p.Events, e)
	}
	if rows.Err() != nil {
		return p, ErrUnavailable
	}
	if len(p.Events) > f.Limit {
		p.More = true
		p.Events = p.Events[:f.Limit]
	}
	if len(p.Events) > 0 {
		p.Cursor = p.Events[len(p.Events)-1].Sequence
	}
	return p, nil
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
func (s *Store) Runs(ctx context.Context, scope Scope) ([]Run, error) {
	if !scope.Allows(scope.Workspace, "") {
		return nil, ErrDenied
	}
	accounts := scope.Accounts
	if accounts == nil {
		accounts = []string{}
	}
	rows, err := s.pool.Query(ctx, "SELECT payload FROM content_diagnostic_run WHERE workspace_id=$1 AND (account_id='' OR account_id=ANY($2)) ORDER BY created_at DESC LIMIT 50", scope.Workspace, accounts)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var b []byte
		if rows.Scan(&b) != nil {
			return nil, ErrUnavailable
		}
		var r Run
		if json.Unmarshal(b, &r) != nil {
			return nil, ErrUnavailable
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
func (s *Store) GetRun(ctx context.Context, scope Scope, id string) (Run, error) {
	var b []byte
	if !scope.Allows(scope.Workspace, "") {
		return Run{}, ErrDenied
	}
	err := s.pool.QueryRow(ctx, "SELECT payload FROM content_diagnostic_run WHERE run_id=$1 AND workspace_id=$2", id, scope.Workspace).Scan(&b)
	if err != nil {
		return Run{}, ErrDenied
	}
	var r Run
	if json.Unmarshal(b, &r) != nil {
		return r, ErrUnavailable
	}
	if !scope.Allows(r.Workspace, r.Account) {
		return Run{}, ErrDenied
	}
	p, err := s.Query(ctx, scope, Filter{Run: id, Limit: 100})
	if err != nil {
		return r, err
	}
	r.Events = p.Events
	if len(r.Events) == 0 {
		r.Gaps = append(r.Gaps, "TECHNICAL_LOG_EXPIRED_OR_UNAVAILABLE")
	}
	return r, nil
}
func (s *Store) PruneTechnical(ctx context.Context) error {
	if s.MaxLogs < 1 || s.Retention <= 0 {
		return ErrConflict
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM content_technical_log WHERE received_at < $1 OR sequence < COALESCE((SELECT sequence FROM content_technical_log ORDER BY sequence DESC OFFSET $2 LIMIT 1),0)", time.Now().Add(-s.Retention), s.MaxLogs-1)
	if err != nil {
		s.Log.Errors.Add(1)
	}
	return err
}
func (s *Store) Check(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("database not configured")
	}
	return s.pool.Ping(ctx)
}
