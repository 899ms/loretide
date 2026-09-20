package reviewdelivery

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"go.opentelemetry.io/otel/trace"
)

// Storage for review requests, delivery tasks, publication records and the
// append-only transition log.
//
// Three rules the shape of these files exists to keep:
//
//   - The transition log and the publication records are append-only. These
//     files contain no UPDATE and no DELETE against either table, and a test
//     scans them to keep that true.
//   - The frozen delivery snapshot is written once. No statement here names the
//     snapshot column outside its INSERT.
//   - Nothing reaches outside this process. No HTTP client, no platform
//     credential, no publish interface, no model call - four guards.
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type DiagnosticStore interface {
	AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error
	Technical(context.Context, diagnostics.Event)
}

// Artifacts answers the two questions this module would otherwise import
// work-editor for: does this document's version exist here, and is the document
// a channel draft. It is an interface so the adapter answers it - the module's
// declared dependency on work-editor is a storage-level one and this card does
// not need its Go package.
type Artifacts interface {
	// ResolveVersion returns the work id and the document kind for a version
	// that belongs to this workspace, or ErrNotFound.
	ResolveVersion(ctx context.Context, workspaceID, artifactID, versionID string) (workID, kind string, err error)
}

type Store struct {
	DB          Database
	Diagnostics DiagnosticStore
	Artifacts   Artifacts
	// Guard is workspace-core's delete/write fence. Every write transaction
	// takes it as its first statement, so a workspace deletion that has already
	// committed cannot be followed by an orphan review request. The audit write
	// takes the same lock, but only on paths that audit; leaning on that would
	// make the fence a side effect of logging and lose it the day a write path
	// stops auditing (Issue #104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
	NewID func() string
	// Now is the clock the two derived displays compare against. Injected so a
	// test can place a scheduled time in the past without sleeping. It is NOT a
	// scheduler: nothing in this package wakes up on it.
	Now func() time.Time
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
	// reportFailure has not set yet. work-editor hands its events over the same
	// way.
	return child, diagnostics.Event{
		ID:         s.newID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "review",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "review-delivery",
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

// recordTransition appends one row to the transition log, inside the caller's
// transaction. Every status change goes through it, including the first.
//
// It is the only writer of that table, and it only ever INSERTs: the guard test
// that scans this package for UPDATE or DELETE against content_review_transition
// has exactly one place to be satisfied.
func (s *Store) recordTransition(ctx context.Context, tx pgx.Tx, workspaceID, actor string,
	kind SubjectKind, subjectID, from, to, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO content_review_transition
		(transition_id, workspace_id, subject_kind, subject_id, from_status,
		 to_status, reason, actor_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.newID(), workspaceID, string(kind), subjectID, from, to, reason, actor)
	return err
}

// ListTransitions returns everything that happened to one subject, oldest
// first: the order a person reads a history in.
func (s *Store) ListTransitions(ctx context.Context, workspaceID, actor string, kind SubjectKind, subjectID string) ([]Transition, error) {
	if s == nil || s.DB == nil {
		if s != nil {
			s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", ErrStorage)
		}
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" || subjectID == "" {
		s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", ErrInvalid)
		return nil, ErrInvalid
	}
	if err := ValidateSubjectKind(string(kind)); err != nil {
		s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", err)
		return nil, err
	}
	rows, err := s.DB.Query(ctx, transitionSelect+
		` WHERE workspace_id=$1 AND subject_kind=$2 AND subject_id=$3
		  ORDER BY created_at, transition_id`, workspaceID, string(kind), subjectID)
	if err != nil {
		s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", err)
		return nil, ErrStorage
	}
	defer rows.Close()
	transitions := []Transition{}
	for rows.Next() {
		transition, scanErr := scanTransition(rows)
		if scanErr != nil {
			s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", scanErr)
			return nil, ErrStorage
		}
		transitions = append(transitions, transition)
	}
	if rows.Err() != nil {
		s.reportFailure(ctx, workspaceID, actor, subjectID, "list-transitions", rows.Err())
		return nil, ErrStorage
	}
	return transitions, nil
}

const reviewSelect = `SELECT review_request_id, workspace_id, work_id, artifact_id,
	version_id, account_id, channel, snapshot, status, requested_by, requested_at,
	decided_by, decided_at, decision_note, created_at, updated_at
	FROM content_review_request`

const transitionSelect = `SELECT transition_id, workspace_id, subject_kind,
	subject_id, from_status, to_status, reason, actor_id, created_at
	FROM content_review_transition`

const deliverySelect = `SELECT delivery_task_id, workspace_id, work_id, artifact_id,
	review_request_id, channel, status, scheduled_at, handoff_method,
	created_at, updated_at FROM content_delivery_task`

const publicationSelect = `SELECT publication_record_id, workspace_id, work_id,
	artifact_id, delivery_task_id, channel, status, actor_id, declared_by,
	page_url_or_content_id, receipt_note, verification_note, published_at,
	platform_account, platform_edited, edit_note, version_match, version_id,
	historical_import, created_at
	FROM content_publication_record`

func scanReview(row scanner) (ReviewRequest, error) {
	var request ReviewRequest
	var payload []byte
	err := row.Scan(&request.ReviewRequestID, &request.WorkspaceID, &request.WorkID,
		&request.ArtifactID, &request.VersionID, &request.AccountID, &request.Channel,
		&payload, &request.Status, &request.RequestedBy, &request.RequestedAt,
		&request.DecidedBy, &request.DecidedAt, &request.DecisionNote,
		&request.CreatedAt, &request.UpdatedAt)
	if err != nil {
		return request, err
	}
	if err = decodeSnapshot(payload, &request.Snapshot); err != nil {
		return request, err
	}
	return request, nil
}

func scanTransition(row scanner) (Transition, error) {
	var transition Transition
	err := row.Scan(&transition.TransitionID, &transition.WorkspaceID,
		&transition.SubjectKind, &transition.SubjectID, &transition.FromStatus,
		&transition.ToStatus, &transition.Reason, &transition.ActorID, &transition.CreatedAt)
	return transition, err
}

func scanDelivery(row scanner) (DeliveryTask, error) {
	var task DeliveryTask
	err := row.Scan(&task.DeliveryTaskID, &task.WorkspaceID, &task.WorkID,
		&task.ArtifactID, &task.ReviewRequestID, &task.Channel, &task.Status,
		&task.ScheduledAt, &task.HandoffMethod, &task.CreatedAt, &task.UpdatedAt)
	return task, err
}

func scanPublication(row scanner) (PublicationRecord, error) {
	var record PublicationRecord
	err := row.Scan(&record.PublicationRecordID, &record.WorkspaceID, &record.WorkID,
		&record.ArtifactID, &record.DeliveryTaskID, &record.Channel, &record.Status,
		&record.ActorID, &record.DeclaredBy, &record.PageURLOrContentID,
		&record.ReceiptNote, &record.VerificationNote, &record.PublishedAt,
		&record.PlatformAccount, &record.PlatformEdited, &record.EditNote,
		&record.VersionMatch, &record.VersionID, &record.HistoricalImport,
		&record.CreatedAt)
	return record, err
}
