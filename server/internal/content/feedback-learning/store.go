package feedbacklearning

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"go.opentelemetry.io/otel/trace"
)

// Storage for manual metrics and feedback excerpts.
//
// Three rules the shape of these files exists to keep:
//
//   - Both tables are append-only. No UPDATE, no DELETE against either; a
//     correction is a new row (R-044 "更新保留历史"). A test scans for both,
//     and for the INSERT, so an empty module cannot satisfy it.
//   - A nil metric value reaches the column as NULL and comes back as nil.
//     Nothing here turns "unknown" into 0.
//   - Nothing reaches outside this process, aggregates anything, or merges
//     read with play.
//
// Contract: specs/027-feedback-manual/contracts/feedback-manual.md

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type DiagnosticStore interface {
	AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error
	Technical(context.Context, diagnostics.Event)
}

// Publications answers the questions this module would otherwise import
// review-delivery for: does this publication record exist here, and which
// version does it correspond to.
//
// It is an interface so the adapter answers it. The version takes two hops -
// publication record to delivery task to review request - and "" is a real
// answer: a record somebody entered for history has no delivery task, so there
// is no version to find. That never refuses the recording.
type Publications interface {
	// Resolve returns the work id, artifact id and version id of a publication
	// record in this workspace, or ErrNotFound. versionID is "" when it cannot
	// be resolved.
	Resolve(ctx context.Context, workspaceID, publicationRecordID string) (workID, artifactID, versionID string, err error)
}

// Observation answers, for one publication record, whether the brand's
// 反馈观察时点 has elapsed (specs/029, SOP 3.2).
//
// An interface answered by the adapter rather than a direct call into
// workspace-core's store, for the same reason Publications is one: this module
// reads its own tables and nobody else's. The Due type itself does come from
// workspace-core - it is a declared dependency, and restating a three-valued
// enum here would be a second definition waiting to drift.
//
// An error is NOT a reason to drop a record. The implementation answers
// DueUnknown when it cannot tell, and DueUnknown keeps the record on the list.
type Observation interface {
	DueFor(ctx context.Context, workspaceID, channel string, publishedAt *time.Time) workspacecore.Due
}

type Store struct {
	DB           Database
	Diagnostics  DiagnosticStore
	Publications Publications
	// Observation is specs/029's window. A nil one means every record answers
	// DueUnknown, which keeps the list exactly as it was before 029 - the
	// behaviour a test fixture that has not wired it should get.
	Observation Observation
	// Guard is workspace-core's delete/write fence. Every write transaction
	// takes it as its first statement, so a workspace deletion that has
	// already committed cannot be followed by an orphan metric. The audit
	// write takes the same lock, but only on paths that audit; leaning on that
	// would make the fence a side effect of logging and lose it the day a
	// write path stops auditing (Issue #104).
	Guard diagnostics.WorkspaceWriteGuard
	Build string
	NewID func() string
}

type scanner interface{ Scan(...any) error }

// dueFor asks the brand's window, defaulting to DueUnknown.
//
// Unknown is the safe default in both directions: it is what a brand with no
// window genuinely is, and it is the answer that KEEPS a record on the pending
// list rather than hiding it.
func (s *Store) dueFor(ctx context.Context, workspaceID, channel string, publishedAt *time.Time) workspacecore.Due {
	if s == nil || s.Observation == nil {
		return workspacecore.DueUnknown
	}
	return s.Observation.DueFor(ctx, workspaceID, channel, publishedAt)
}

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
		// A workspace that is gone is reported exactly as a foreign one, so
		// the boundary answers 404 and the response cannot be used to tell a
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
	// reportFailure has not set yet. work-editor and review-delivery hand
	// their events over the same way.
	return child, diagnostics.Event{
		ID:         s.newID(),
		Workspace:  workspaceID,
		Actor:      actor,
		ActorKind:  "human",
		ObjectType: "feedback",
		ObjectID:   objectID,
		Action:     "execute",
		Outcome:    "success",
		Trace:      span.TraceID().String(),
		Span:       span.SpanID().String(),
		Parent:     parent,
		Step:       step,
		Component:  "feedback-learning",
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

const metricSelect = `SELECT manual_metric_id, workspace_id, publication_record_id,
	platform, account_id, metric, value, unit, stat_window, sampled_at,
	recorded_by, evidence_note, source_type, created_at
	FROM content_manual_metric`

const excerptSelect = `SELECT feedback_excerpt_id, workspace_id, publication_record_id,
	source_type, redacted_excerpt, interpretation, tags, occurred_at,
	recorded_by, created_at FROM content_feedback_excerpt`

// scanMetric reads a row. Value is scanned into *int64, so a NULL column stays
// nil all the way out - there is no branch here that could turn it into 0.
func scanMetric(row scanner) (ManualMetric, error) {
	var metric ManualMetric
	err := row.Scan(&metric.ManualMetricID, &metric.WorkspaceID,
		&metric.PublicationRecordID, &metric.Platform, &metric.AccountID,
		&metric.Metric, &metric.Value, &metric.Unit, &metric.StatWindow,
		&metric.SampledAt, &metric.RecordedBy, &metric.EvidenceNote,
		&metric.SourceType, &metric.CreatedAt)
	return metric, err
}

func scanExcerpt(row scanner) (FeedbackExcerpt, error) {
	var excerpt FeedbackExcerpt
	err := row.Scan(&excerpt.FeedbackExcerptID, &excerpt.WorkspaceID,
		&excerpt.PublicationRecordID, &excerpt.SourceType, &excerpt.RedactedExcerpt,
		&excerpt.Interpretation, &excerpt.Tags, &excerpt.OccurredAt,
		&excerpt.RecordedBy, &excerpt.CreatedAt)
	if excerpt.Tags == nil {
		// Empty, not nil: this is marshalled into a response, and a nil slice
		// reads back as JSON null while an empty one reads back as []. A
		// consumer should not have to know which of the two it received.
		excerpt.Tags = []string{}
	}
	return excerpt, err
}
