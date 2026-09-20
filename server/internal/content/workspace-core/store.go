package workspacecore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"go.opentelemetry.io/otel/trace"
)

// Reading and writing the brand's operating rules.
//
// This is the first storage this package has ever had, and that is worth a
// sentence. The package comment says it does not look objects up, and that
// still holds: what it refuses to do is read the CALLER's tables, because a
// caller that lets it would get "allowed" back for an object belonging to a
// different brand. The workspace row is not a caller's table - it is the row
// this package is about - and SOP 3.2's settings live on it.
//
// Two rules the shape of this file exists to keep:
//
//   - Every write takes the delete fence as the transaction's FIRST statement
//     (#104), then audits in the same transaction. Not because the audit
//     happens to lock: leaning on that would make the fence a side effect of
//     logging and lose it the day a write path stops auditing.
//   - A write NEVER replaces the settings blob. It sets one key with
//     jsonb_set, so a brand's timezone and precheck switch cannot be collateral
//     damage of saving a cadence. LT-009 and LT-015 merged on the client and
//     sent the whole blob back; LT-014 stopped doing that and said why in
//     router.go, and this follows LT-014.
//
// Contract: specs/029-operating-rules/contracts/operating-rules.md

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type DiagnosticStore interface {
	AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error
	Technical(context.Context, diagnostics.Event)
}

type Store struct {
	DB          Database
	Diagnostics DiagnosticStore
	// Guard is the workspace delete/write fence (#104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
}

// ReadRules returns the brand's rules, filling defaults on the way out.
//
// Read-only: no transaction, no fence, and nothing is written back. Reading a
// workspace must never modify it - the same rule timezoneFilled and
// autoPrecheckFilled follow on the handler side.
func (s *Store) ReadRules(ctx context.Context, workspaceID string) (Rules, error) {
	if s == nil || s.DB == nil {
		return Rules{}, ErrStorage
	}
	// The workspace table's id is a uuid, unlike the text workspace_id every
	// content table carries. An id that is not one matches nothing, and saying
	// so here keeps it a 404 instead of letting the cast fail into a 500 - the
	// same answer the write fence gives a malformed id.
	if _, err := uuid.Parse(workspaceID); err != nil {
		return Rules{}, ErrNotFound
	}
	var settings []byte
	err := s.DB.QueryRow(ctx,
		`SELECT settings FROM workspace WHERE id = $1::uuid`, workspaceID).Scan(&settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rules{}, ErrNotFound
	}
	if err != nil {
		return Rules{}, ErrStorage
	}
	var blob map[string]any
	if len(settings) > 0 {
		// A settings column that will not parse is not a reason to fail the
		// page. It degrades to defaults, like every other reader of this blob.
		_ = json.Unmarshal(settings, &blob)
	}
	return RulesFrom(blob), nil
}

// WriteRules validates and stores the rules, replacing only this card's key.
func (s *Store) WriteRules(ctx context.Context, workspaceID, actor string, rules Rules) (Rules, error) {
	if err := ValidateRules(rules); err != nil {
		return Rules{}, err
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		return Rules{}, ErrStorage
	}
	if err := s.writeKey(ctx, workspaceID, actor,
		`UPDATE workspace
		 SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY[$2::text], $3::jsonb, true),
		     updated_at = now()
		 WHERE id = $1::uuid`,
		OperatingRulesKey, encoded, "operating-rules.write"); err != nil {
		return Rules{}, err
	}
	return rules, nil
}

// WriteHomepage stores one account's public page link.
//
// A different row from WriteRules - the account, not the workspace - but the
// same fence, the same audit, and the same one-key write. The account's
// loretide.scope (LT-014) survives a homepage save for exactly that reason.
func (s *Store) WriteHomepage(ctx context.Context, workspaceID, actor, accountID, link string) error {
	if err := ValidateHomepage(link); err != nil {
		return err
	}
	encoded, err := json.Marshal(link)
	if err != nil {
		return ErrStorage
	}
	return s.writeKey(ctx, workspaceID, actor,
		`UPDATE content_account
		 SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), ARRAY[$2::text], $3::jsonb, true),
		     updated_at = now()
		 WHERE account_id = $4::text AND workspace_id = $1::text`,
		HomepageKey, encoded, "operating-rules.homepage", accountID)
}

// writeKey runs one targeted settings update inside a fenced, audited
// transaction.
//
// The statement is passed in rather than built here so the two callers cannot
// accidentally share a WHERE clause: one is scoped by workspace id, the other
// by account id AND workspace id, and merging them would be the kind of
// convenience that leaks a row across brands.
func (s *Store) writeKey(
	ctx context.Context,
	workspaceID, actor, statement, key string,
	value []byte,
	step string,
	extra ...any,
) error {
	if s == nil || s.DB == nil || s.Diagnostics == nil || s.Guard == nil {
		return ErrStorage
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ErrStorage
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// First statement in the transaction, before anything is read or written.
	if err = s.Guard.LockForContentDiagnosticWrite(ctx, tx, workspaceID); err != nil {
		// A workspace that is gone is reported exactly as a foreign one, so
		// the boundary answers 404 either way and the response cannot be used
		// to tell a deleted brand from one the caller never had.
		if errors.Is(err, diagnostics.ErrDenied) {
			return ErrNotFound
		}
		return ErrStorage
	}

	args := append([]any{workspaceID, key, value}, extra...)
	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, key, step, err)
		return ErrStorage
	}
	if tag.RowsAffected() == 0 {
		// Nothing matched: either the row is not there or it belongs to
		// somebody else. Both are 404, and deliberately the same one.
		return ErrNotFound
	}

	child, err := s.audit(ctx, tx, workspaceID, actor, key, step)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, key, step, err)
		return ErrStorage
	}
	if err := tx.Commit(child); err != nil {
		s.reportFailure(ctx, workspaceID, actor, key, step, err)
		return ErrStorage
	}
	return nil
}

func (s *Store) event(ctx context.Context, workspaceID, actor, objectID, step string) (context.Context, diagnostics.Event) {
	child, parent := diagnostics.Child(ctx)
	span := trace.SpanContextFromContext(child)
	// Built unsanitized on purpose: Sanitize is the sink's job, and running it
	// here too would derive Message/Next/Retryable from a Code that
	// reportFailure has not set yet. The other content stores hand their
	// events over the same way.
	return child, diagnostics.Event{
		ID:         diagnostics.NewID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "operating_rules",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "workspace-core",
		Severity:   "info",
		Build:      s.Build,
		Occurred:   time.Now().UTC(),
	}
}

func (s *Store) audit(ctx context.Context, tx pgx.Tx, workspaceID, actor, objectID, step string) (context.Context, error) {
	child, event := s.event(ctx, workspaceID, actor, objectID, step)
	err := s.Diagnostics.AuditTx(child, tx, diagnostics.Scope{Workspace: workspaceID, Actor: actor}, event)
	return child, err
}

func (s *Store) reportFailure(ctx context.Context, workspaceID, actor, objectID, step string, err error) {
	if s == nil || s.Diagnostics == nil {
		return
	}
	child, event := s.event(ctx, workspaceID, actor, objectID, step)
	event.Outcome = "failed"
	event.Severity = "error"
	event.Code = "DATABASE_UNAVAILABLE"
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) {
		event.Code = "INPUT_CONFLICT"
		event.Severity = "warn"
	}
	s.Diagnostics.Technical(child, event)
}
