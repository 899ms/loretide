package feedbacklearning

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The import's claim-and-replay flow against an in-memory stand-in for the
// few statements a cost import runs, so the flow is exercised by every plain
// `go test`. The stand-in is a transaction with commit and rollback and
// nothing more: it does not wait on locks. Real PostgreSQL - the unique index
// a second importer waits on - is proven by internal/handler's
// TestContentROIConcurrentImportsWithOneKeyWriteOnce and
// TestContentROIImportClaimIsRolledBackWithItsImport.

type memClaim struct{ fingerprint, batchID string }

type memBatch struct {
	id, workspace, kind string
	rowCount, written   int
	rows                []byte
	key, actor          string
	created             time.Time
}

type memCost struct{ id, key string }

type memState struct {
	claims  map[string]memClaim
	batches map[string]memBatch
	costs   []memCost
}

func (s memState) clone() memState {
	next := memState{claims: map[string]memClaim{}, batches: map[string]memBatch{}, costs: append([]memCost{}, s.costs...)}
	for k, v := range s.claims {
		next.claims[k] = v
	}
	for k, v := range s.batches {
		next.batches[k] = v
	}
	return next
}

type memDB struct {
	mu        sync.Mutex
	committed memState
	clock     time.Time
}

func newMemDB() *memDB {
	return &memDB{
		committed: memState{claims: map[string]memClaim{}, batches: map[string]memBatch{}},
		clock:     time.Date(2026, 9, 25, 1, 2, 3, 456789000, time.UTC),
	}
}

func (db *memDB) Begin(context.Context) (pgx.Tx, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.clock = db.clock.Add(time.Second)
	return &memTx{db: db, state: db.committed.clone(), now: db.clock}, nil
}

func (db *memDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("the import reads inside its transaction only")
}

func (db *memDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("the import reads inside its transaction only")
}

type memTx struct {
	pgx.Tx
	db    *memDB
	state memState
	now   time.Time
	done  bool
}

func (tx *memTx) Commit(context.Context) error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if !tx.done {
		tx.db.committed = tx.state
		tx.done = true
	}
	return nil
}

func (tx *memTx) Rollback(context.Context) error {
	tx.done = true
	return nil
}

// memRow scans fixed values, or answers ErrNoRows.
type memRow struct{ values []any }

func (r memRow) Scan(dest ...any) error {
	if r.values == nil {
		return pgx.ErrNoRows
	}
	for i, target := range dest {
		reflect.ValueOf(target).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

func claimKey(args []any) string {
	return args[0].(string) + "\x1f" + args[1].(string) + "\x1f" + args[2].(string)
}

func (tx *memTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "INSERT INTO content_roi_import_claim"):
		if _, taken := tx.state.claims[claimKey(args)]; taken {
			return memRow{} // ON CONFLICT DO NOTHING: no row returned
		}
		tx.state.claims[claimKey(args)] = memClaim{fingerprint: args[3].(string), batchID: args[4].(string)}
		return memRow{values: []any{args[4].(string)}}
	case strings.Contains(sql, "SELECT request_fingerprint, import_batch_id"):
		claim, ok := tx.state.claims[claimKey(args)]
		if !ok {
			return memRow{}
		}
		return memRow{values: []any{claim.fingerprint, claim.batchID}}
	case strings.Contains(sql, "FROM content_roi_import_batch"):
		batch, ok := tx.state.batches[args[1].(string)]
		if !ok || batch.workspace != args[0].(string) {
			return memRow{}
		}
		return memRow{values: []any{batch.id, batch.workspace, batch.kind, batch.rowCount, batch.written,
			batch.rowCount - batch.written, batch.rows, batch.key, batch.actor, batch.created}}
	case strings.Contains(sql, "INSERT INTO content_roi_cost_revision"):
		tx.state.costs = append(tx.state.costs, memCost{id: args[1].(string), key: args[17].(string)})
		return memRow{values: []any{tx.now}}
	}
	panic("unexpected statement: " + sql)
}

func (tx *memTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !strings.Contains(sql, "INSERT INTO content_roi_import_batch") {
		panic("unexpected statement: " + sql)
	}
	tx.state.batches[args[0].(string)] = memBatch{
		id: args[0].(string), workspace: args[1].(string), kind: args[2].(string),
		rowCount: args[3].(int), written: args[4].(int), rows: []byte(args[6].(string)),
		key: args[7].(string), actor: args[8].(string), created: tx.now,
	}
	return pgconn.CommandTag{}, nil
}

type memRows struct {
	pgx.Rows
	ids  []string
	next int
}

func (r *memRows) Next() bool          { r.next++; return r.next <= len(r.ids) }
func (r *memRows) Scan(d ...any) error { *(d[0].(*string)) = r.ids[r.next-1]; return nil }
func (r *memRows) Err() error          { return nil }
func (r *memRows) Close()              {}

func (tx *memTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !strings.Contains(sql, "SELECT r.cost_id FROM content_roi_cost_revision") {
		panic("unexpected statement: " + sql)
	}
	rows := &memRows{}
	for _, cost := range tx.state.costs {
		if cost.key == args[1].(string) && cost.id != args[2].(string) {
			rows.ids = append(rows.ids, cost.id)
		}
	}
	return rows, nil
}

type memGuard struct{}

func (memGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error { return nil }

type memDiagnostics struct{}

func (memDiagnostics) AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error {
	return nil
}
func (memDiagnostics) Technical(context.Context, diagnostics.Event) {}

func memImportStore(db *memDB) *ROIStore {
	return &ROIStore{Store: &Store{DB: db, Diagnostics: memDiagnostics{}, Guard: memGuard{}}}
}

func costRows(amounts ...string) ImportInput {
	in := ImportInput{RecordKind: "cost"}
	for _, amount := range amounts {
		currency := "CNY"
		if amount == "RMB" {
			amount, currency = "1.00", "RMB"
		}
		in.Costs = append(in.Costs, importCost("拍摄-"+amount, amount, currency))
	}
	return in
}

// FR-028 / T064 at the module: the same key and input answers the original
// response byte for byte and writes nothing more; the same key with other
// input is IdempotencyConflict.
func TestAReplayIsTheOriginalResponseAndWritesNothing(t *testing.T) {
	db := newMemDB()
	store := memImportStore(db)
	ctx := t.Context()
	in := costRows("100.00", "200.00")

	first, err := store.Import(ctx, "ws", "u1", in, "key-1", false)
	if err != nil || first.WrittenCount != 2 {
		t.Fatalf("first import = %+v, %v", first, err)
	}
	original, _ := json.Marshal(first)
	for range 2 {
		replay, replayErr := store.Import(ctx, "ws", "u1", costRows("100.00", "200.00"), "key-1", false)
		if replayErr != nil {
			t.Fatal(replayErr)
		}
		if again, _ := json.Marshal(replay); string(again) != string(original) {
			t.Fatalf("the replay differs:\n%s\n%s", original, again)
		}
	}
	if len(db.committed.costs) != 2 || len(db.committed.batches) != 1 || len(db.committed.claims) != 1 {
		t.Fatalf("after replays: %d costs, %d batches, %d claims", len(db.committed.costs),
			len(db.committed.batches), len(db.committed.claims))
	}
	if _, err = store.Import(ctx, "ws", "u1", costRows("100.00"), "key-1", false); !errors.As(err, new(IdempotencyConflict)) {
		t.Fatalf("same key, other rows = %v, want IdempotencyConflict", err)
	}
}

// FR-028 / T066 at the module: a claim belongs to its import's transaction.
// An import that fails on a row leaves no claim, and the same key with the
// row fixed goes through.
func TestAFailedImportLeavesItsKeyFree(t *testing.T) {
	db := newMemDB()
	store := memImportStore(db)
	ctx := t.Context()
	_, err := store.Import(ctx, "ws", "u1", costRows("100.00", "RMB"), "key-1", false)
	if fieldErr, ok := errors.AsType[FieldError](err); !ok || fieldErr.Row != 2 || fieldErr.Field != "currency" {
		t.Fatalf("bad import = %v, want row 2 currency", err)
	}
	if len(db.committed.claims) != 0 || len(db.committed.costs) != 0 || len(db.committed.batches) != 0 {
		t.Fatalf("a failed import left %d claims, %d costs, %d batches", len(db.committed.claims),
			len(db.committed.costs), len(db.committed.batches))
	}
	fixed, err := store.Import(ctx, "ws", "u1", costRows("100.00", "300.00"), "key-1", false)
	if err != nil || fixed.WrittenCount != 2 {
		t.Fatalf("the corrected import = %+v, %v", fixed, err)
	}
}

// T065 at the module: a dry run claims nothing and writes nothing.
func TestADryRunClaimsNothing(t *testing.T) {
	db := newMemDB()
	store := memImportStore(db)
	result, err := store.Import(t.Context(), "ws", "u1", costRows("100.00", "100.00"), "key-1", true)
	if err != nil || !result.DryRun || result.ImportBatchID != "" || result.CreatedAt != nil {
		t.Fatalf("dry run = %+v, %v", result, err)
	}
	if result.Rows[1].Outcome != OutcomeDuplicate || len(result.Rows[1].DuplicateOfRows) != 1 || result.Rows[1].DuplicateOfRows[0] != 1 {
		t.Errorf("the second identical row = %+v", result.Rows[1])
	}
	if len(db.committed.claims)+len(db.committed.costs)+len(db.committed.batches) != 0 {
		t.Fatalf("a dry run wrote: %+v", db.committed)
	}
}
