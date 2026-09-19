package diagnostics

import (
	"errors"
	"testing"
	"time"
)

func auditTxEvent(id string) Event {
	return Event{
		ID:         id,
		Workspace:  "w",
		Actor:      "u",
		ActorKind:  "human",
		ObjectType: "work",
		ObjectID:   "topic-1",
		Action:     "execute",
		Outcome:    "success",
		Component:  "topic-planning",
		Severity:   "info",
		Occurred:   time.Now().UTC(),
	}
}

func TestAuditTxSharesTheCallersCommitAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := t.Context()
	scope := Scope{Workspace: "w", Actor: "u"}

	committedID := NewID()
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AuditTx(ctx, tx, scope, auditTxEvent(committedID)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("audit in caller transaction: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("caller commit: %v", err)
	}

	rolledBackID := NewID()
	tx, err = store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AuditTx(ctx, tx, scope, auditTxEvent(rolledBackID)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("audit before rollback: %v", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatalf("caller rollback: %v", err)
	}

	for id, want := range map[string]int{committedID: 1, rolledBackID: 0} {
		var got int
		if err = store.pool.QueryRow(ctx,
			"SELECT count(*) FROM content_operation_audit WHERE event_id=$1", id).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", id, err)
		}
		if got != want {
			t.Errorf("audit %s count = %d, want %d", id, got, want)
		}
	}
}

func TestAuditTxRefusesUnauthorizedScopeWithoutWriting(t *testing.T) {
	store := testStore(t)
	ctx := t.Context()
	id := NewID()
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	err = store.AuditTx(ctx, tx, Scope{Workspace: "other", Actor: "u"}, auditTxEvent(id))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("unauthorized audit = %v, want ErrDenied", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit after refusal: %v", err)
	}

	var count int
	if err = store.pool.QueryRow(ctx,
		"SELECT count(*) FROM content_operation_audit WHERE event_id=$1", id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unauthorized audit wrote %d rows", count)
	}
}

func TestAuditTxFailsClosedWithMissingDependencies(t *testing.T) {
	var store *Store
	if err := store.AuditTx(t.Context(), nil, Scope{}, Event{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil store/transaction = %v, want ErrUnavailable", err)
	}
}
