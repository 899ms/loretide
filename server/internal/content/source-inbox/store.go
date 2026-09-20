package sourceinbox

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"go.opentelemetry.io/otel/trace"
)

// Storage for sources, their snapshots and the append-only organising log.
//
// Three rules the shape of these files exists to keep:
//
//   - content_source_snapshot and content_source_revision are append-only.
//     These files contain no UPDATE and no DELETE against either, and a test
//     scans them to keep that true.
//   - Five columns of content_source are written once. No UPDATE here names
//     kind, url, captured_at, recorded_by or historical_import in its SET list.
//   - Nothing reaches outside this process. No HTTP client, no fetcher, no
//     model call - a url is a string that gets stored, not visited.

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type DiagnosticStore interface {
	AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error
	Technical(context.Context, diagnostics.Event)
}

type Store struct {
	DB          Database
	Diagnostics DiagnosticStore
	// Guard is workspace-core's delete/write fence. Every write transaction
	// takes it as its first statement, so a workspace deletion that has already
	// committed cannot be followed by an orphan source. The audit write takes
	// the same lock, but only on paths that audit; leaning on that would make
	// the fence a side effect of logging and lose it the day a write path stops
	// auditing (Issue #104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
	NewID func() string
	Now   func() time.Time
}

type scanner interface{ Scan(...any) error }

func (s *Store) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return diagnostics.NewID()
}

func (s *Store) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Store) begin(ctx context.Context, workspaceID string) (pgx.Tx, error) {
	if s == nil || s.DB == nil || s.Diagnostics == nil || s.Guard == nil {
		return nil, ErrStorage
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, ErrStorage
	}
	if err = s.Guard.LockForContentDiagnosticWrite(ctx, tx, workspaceID); err != nil {
		_ = tx.Rollback(ctx)
		// A workspace that is gone is reported exactly as a foreign one, so the
		// boundary answers 404 and the response cannot be used to tell a
		// deleted workspace from one the caller never had.
		if errors.Is(err, diagnostics.ErrDenied) {
			return nil, ErrNotFound
		}
		return nil, ErrStorage
	}
	return tx, nil
}

func (s *Store) event(ctx context.Context, workspaceID, actor, objectID, step string) (context.Context, diagnostics.Event) {
	child, parent := diagnostics.Child(ctx)
	span := trace.SpanContextFromContext(child)
	// Built unsanitized on purpose: Sanitize is the sink's job, and running it
	// here as well would derive Message/Next/Retryable from a Code that
	// reportFailure has not set yet.
	return child, diagnostics.Event{
		ID:         s.newID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "source",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "source-inbox",
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

// appendRevision writes one row to the organising log, inside the caller's
// transaction. Every change to the mutable half goes through it, including
// each item of a bulk operation.
//
// It is the only writer of that table, so the guard test that scans this
// package for UPDATE or DELETE against content_source_revision has exactly one
// place to be satisfied.
func (s *Store) appendRevision(ctx context.Context, tx pgx.Tx, workspaceID, sourceID, actor string, fields []string) error {
	_, err := tx.Exec(ctx, `INSERT INTO content_source_revision
		(revision_id, workspace_id, source_id, changed_fields, actor_id)
		VALUES ($1,$2,$3,$4,$5)`,
		s.newID(), workspaceID, sourceID, fields, actor)
	return err
}

// ListRevisions returns how one item was organised, oldest first: the order a
// person reads a history in.
func (s *Store) ListRevisions(ctx context.Context, workspaceID, actor, sourceID string) ([]Revision, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, sourceID, "list-revisions", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || sourceID == "" {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "list-revisions", ErrInvalid)
		return nil, ErrInvalid
	}
	rows, err := s.DB.Query(ctx, `SELECT revision_id, workspace_id, source_id,
		changed_fields, actor_id, created_at
		FROM content_source_revision
		WHERE workspace_id = $1 AND source_id = $2
		ORDER BY created_at, revision_id`, workspaceID, sourceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "list-revisions", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	revisions := []Revision{}
	for rows.Next() {
		revision, scanErr := scanRevision(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, sourceID, "list-revisions", scanErr)
			return nil, ErrStorage
		}
		revisions = append(revisions, revision)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, sourceID, "list-revisions", rows.Err())
		return nil, ErrStorage
	}
	return revisions, nil
}

func scanRevision(row scanner) (Revision, error) {
	var revision Revision
	err := row.Scan(&revision.RevisionID, &revision.WorkspaceID, &revision.SourceID,
		&revision.ChangedFields, &revision.ActorID, &revision.CreatedAt)
	if revision.ChangedFields == nil {
		revision.ChangedFields = []string{}
	}
	return revision, err
}
