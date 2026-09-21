package workeditor

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"go.opentelemetry.io/otel/trace"
)

// Storage for works, documents and their version history.
//
// Two rules the shape of this file exists to keep:
//
//   - The editing copy is MUTABLE and the version history is not. They are
//     different tables, and this file contains no UPDATE and no DELETE against
//     content_artifact_version. A test scans it to keep that true.
//   - A version number is taken in two statements, never one. See appendVersion.
//
// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md

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
	// committed cannot be followed by an orphan work. The audit write takes the
	// same lock, but only on paths that audit; leaning on that would make the
	// fence a side effect of logging and lose it the day a write path stops
	// auditing (Issue #104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
	NewID func() string
}

type scanner interface{ Scan(...any) error }

func (s *Store) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return diagnostics.NewID()
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
	// reportFailure has not set yet. topic-planning hands its events over the
	// same way.
	return child, diagnostics.Event{
		ID:         s.newID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "work",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "work-editor",
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
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
		event.Code = "INPUT_CONFLICT"
		event.Severity = "warn"
	}
	s.Diagnostics.Technical(child, event)
}

const workSelect = `SELECT work_id, workspace_id, topic_card_id, snapshot_id,
	title, historical_import, created_at, updated_at FROM content_work`

const artifactSelect = `SELECT artifact_id, work_id, workspace_id, kind, title,
	position, draft_body, draft_status, draft_saved_at, created_at, updated_at
	FROM content_artifact`

const versionSelect = `SELECT version_id, artifact_id, work_id, workspace_id,
	revision, source, action, body, restored_from, adopted_from, actor_id,
	created_at FROM content_artifact_version`

func scanWork(row scanner) (Work, error) {
	var work Work
	err := row.Scan(&work.WorkID, &work.WorkspaceID, &work.TopicCardID,
		&work.SnapshotID, &work.Title, &work.HistoricalImport,
		&work.CreatedAt, &work.UpdatedAt)
	return work, err
}

func scanArtifact(row scanner) (Artifact, error) {
	var artifact Artifact
	err := row.Scan(&artifact.ArtifactID, &artifact.WorkID, &artifact.WorkspaceID,
		&artifact.Kind, &artifact.Title, &artifact.Position, &artifact.DraftBody,
		&artifact.DraftStatus, &artifact.DraftSavedAt, &artifact.CreatedAt, &artifact.UpdatedAt)
	return artifact, err
}

func scanVersion(row scanner) (ArtifactVersion, error) {
	var version ArtifactVersion
	err := row.Scan(&version.VersionID, &version.ArtifactID, &version.WorkID,
		&version.WorkspaceID, &version.Revision, &version.Source, &version.Action,
		&version.Body, &version.RestoredFrom, &version.AdoptedFrom,
		&version.ActorID, &version.CreatedAt)
	return version, err
}

// CreateWork opens a container for one topic card's output.
//
// topicCardID is a plain string and this package does not import
// topic-planning: the only question it would ask is "does this card exist
// here", and the adapter answers that. Importing the package to ask one
// question widens the module dependency graph for good.
func (s *Store) CreateWork(ctx context.Context, workspaceID, actor string, work Work) (Work, error) {
	if s == nil {
		return Work{}, ErrStorage
	}
	// topic_card_id is NOT required. A work imported from something already
	// published (SOP 3.3) has no topic card: it had a body first, and
	// everything else after. "" is a real state here for the same reason it is
	// for snapshot_id one line below - see content_work's own column comment.
	//
	// The cost is that "the works of this card" can no longer be expressed as
	// "not filtered by card", and every by-card read path has to say so. See
	// ListWorks.
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "create-work", ErrInvalid)
		return Work{}, ErrInvalid
	}
	if err := ValidateTitle(work.Title); err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "create-work", err)
		return Work{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "create-work", err)
		return Work{}, err
	}
	defer tx.Rollback(ctx)
	created := Work{
		WorkID: s.newID(), WorkspaceID: workspaceID,
		TopicCardID: work.TopicCardID, SnapshotID: work.SnapshotID, Title: work.Title,
		// Written once, here. No update path names this column, and a guard
		// test asserts none ever does.
		HistoricalImport: work.HistoricalImport,
	}
	ctx, err = s.audit(ctx, tx, workspaceID, actor, created.WorkID, "create-work")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.WorkID, "create-work", err)
		return Work{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO content_work
		(work_id, workspace_id, topic_card_id, snapshot_id, title, historical_import)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at, updated_at`,
		created.WorkID, workspaceID, created.TopicCardID, created.SnapshotID,
		created.Title, created.HistoricalImport).
		Scan(&created.CreatedAt, &created.UpdatedAt); err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, created.WorkID, "create-work", err)
		return Work{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, created.WorkID, "create-work", err)
		return Work{}, ErrStorage
	}
	return created, nil
}

// ListWorks returns the works of one topic card, or of the whole workspace when
// topicCardID is empty.
func (s *Store) ListWorks(ctx context.Context, workspaceID, actor, topicCardID string) ([]Work, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list-works", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportFailure(ctx, workspaceID, actor, "", "list-works", ErrInvalid)
		return nil, ErrInvalid
	}
	// topic_card_id='' is now a real state (an imported work has no card), so
	// "$2='' OR topic_card_id=$2" has two readings and only one is wanted:
	// with a card named, a work with no card must NOT match. It does not,
	// because '' is never equal to a non-empty id - but that is the whole of
	// specs/031 FR-034 resting on an expression nobody wrote for this, so
	// TestAnImportedWorkIsNotListedUnderAnyTopicCard proves it rather than
	// assuming it.
	//
	// With $2='' the list is the workspace's works, imported ones included.
	// That is deliberate (FR-036): they are real works here. What is excluded
	// is the BY-CARD reading, not the work.
	rows, err := s.DB.Query(ctx, workSelect+
		` WHERE workspace_id=$1 AND ($2='' OR topic_card_id=$2)
		  ORDER BY created_at DESC, work_id`, workspaceID, topicCardID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list-works", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	works := []Work{}
	for rows.Next() {
		work, scanErr := scanWork(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, "", "list-works", scanErr)
			return nil, ErrStorage
		}
		works = append(works, work)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, "", "list-works", rows.Err())
		return nil, ErrStorage
	}
	return works, nil
}

func (s *Store) GetWork(ctx context.Context, workspaceID, actor, workID string) (Work, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, workID, "get-work", ErrStorage)
		}
		return Work{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" {
		s.reportFailure(ctx, workspaceID, actor, workID, "get-work", ErrInvalid)
		return Work{}, ErrInvalid
	}
	work, err := scanWork(s.DB.QueryRow(ctx, workSelect+
		` WHERE workspace_id=$1 AND work_id=$2`, workspaceID, workID))
	if errors.Is(err, pgx.ErrNoRows) {
		s.reportFailure(ctx, workspaceID, actor, workID, "get-work", ErrNotFound)
		return Work{}, ErrNotFound
	}
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "get-work", err)
		return Work{}, ErrStorage
	}
	return work, nil
}

// RenameWork changes the title. There is no status to change: SOP 7.1's states
// belong to the editing copy, and the rest are other cards' business.
func (s *Store) RenameWork(ctx context.Context, workspaceID, actor, workID, title string) (Work, error) {
	if s == nil {
		return Work{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || workID == "" {
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", ErrInvalid)
		return Work{}, ErrInvalid
	}
	if err := ValidateTitle(title); err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", err)
		return Work{}, err
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", err)
		return Work{}, err
	}
	defer tx.Rollback(ctx)
	ctx, err = s.audit(ctx, tx, workspaceID, actor, workID, "rename-work")
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", err)
		return Work{}, err
	}
	work, err := scanWork(tx.QueryRow(ctx, `UPDATE content_work SET title=$3, updated_at=now()
		WHERE workspace_id=$1 AND work_id=$2
		RETURNING work_id, workspace_id, topic_card_id, snapshot_id, title,
			historical_import, created_at, updated_at`,
		workspaceID, workID, title))
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", ErrNotFound)
		return Work{}, ErrNotFound
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", err)
		return Work{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		s.reportFailure(ctx, workspaceID, actor, workID, "rename-work", err)
		return Work{}, ErrStorage
	}
	return work, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
