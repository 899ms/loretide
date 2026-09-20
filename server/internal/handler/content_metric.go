package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Manual metrics (specs/027, SOP 10.1).
//
// The module stores publication_record_id as a plain string and does not
// import review-delivery; resolving the record and walking two hops to its
// version is the adapter's job, which is here.

func (h *Handler) feedbackScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	workspace := h.resolveWorkspaceID(r)
	actor := r.Header.Get("X-User-ID")
	decision := workspacecore.Authorize(r.Context(), h.diagnosticMembership(),
		h.diagnosticRefusalRecorder(), actor, workspace, "owner", "admin", "member")
	if !decision.Allowed {
		writeJSON(w, workspacecore.RefusalStatus(decision.Reason),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
		return "", "", false
	}
	return workspace, actor, true
}

type feedbackDatabase struct {
	dbExecutor
	txStarter
}

func (d feedbackDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, feedbacklearning.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d feedbackDatabase) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dbExecutor == nil {
		return nil, feedbacklearning.ErrStorage
	}
	return d.dbExecutor.Query(ctx, sql, args...)
}

func (d feedbackDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: feedbacklearning.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

// feedbackPublications answers the two questions feedback-learning would
// otherwise have to import review-delivery for.
//
// The version takes two hops - publication record to delivery task to review
// request - because 025's publication record deliberately carries no
// version_id. A record entered for history has no delivery task, so there is
// nothing to walk, and "" is the honest answer rather than an error: the
// numbers are real, we just do not know which version they belong to.
type feedbackPublications struct{ db dbExecutor }

func (p feedbackPublications) Resolve(ctx context.Context, workspaceID, publicationRecordID string) (string, string, string, error) {
	if p.db == nil || workspaceID == "" || publicationRecordID == "" {
		return "", "", "", feedbacklearning.ErrNotFound
	}
	var workID, artifactID, deliveryTaskID string
	if err := p.db.QueryRow(ctx, `SELECT work_id, artifact_id, delivery_task_id
		FROM content_publication_record
		WHERE workspace_id=$1 AND publication_record_id=$2`,
		workspaceID, publicationRecordID).Scan(&workID, &artifactID, &deliveryTaskID); err != nil {
		return "", "", "", feedbacklearning.ErrNotFound
	}
	if deliveryTaskID == "" {
		return workID, artifactID, "", nil
	}
	var versionID string
	if err := p.db.QueryRow(ctx, `SELECT r.version_id
		FROM content_delivery_task t
		JOIN content_review_request r
		  ON r.workspace_id = t.workspace_id AND r.review_request_id = t.review_request_id
		WHERE t.workspace_id=$1 AND t.delivery_task_id=$2`,
		workspaceID, deliveryTaskID).Scan(&versionID); err != nil {
		// A task with no approved request behind it yet. Not an error for the
		// recording; the version is simply not known.
		return workID, artifactID, "", nil
	}
	return workID, artifactID, versionID, nil
}

func (h *Handler) feedbackStore() *feedbacklearning.Store {
	var diagnosticStore feedbacklearning.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &feedbacklearning.Store{
		DB:           feedbackDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics:  diagnosticStore,
		Publications: feedbackPublications{db: h.DB},
		// The same fence the diagnostics store holds, handed over directly so
		// the write transaction takes the workspace delete lock itself instead
		// of inheriting it from whether it happened to audit first (#104).
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

func decodeFeedbackBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

// feedbackError maps the module's errors. A refusal and a missing row answer
// identically, so a caller cannot use the response to learn that something
// exists in another workspace.
func (h *Handler) feedbackError(w http.ResponseWriter, err error) {
	var fieldErr feedbacklearning.FieldError
	switch {
	case errors.As(err, &fieldErr):
		detail := map[string]any{"field": fieldErr.Field, "reason": fieldErr.Reason}
		if fieldErr.Row > 0 {
			// Which line of a pasted batch. "Some row is wrong" in a paste of
			// forty rows is not an answer anybody can act on.
			detail["row"] = fieldErr.Row
		}
		h.feedbackDiagnosticError(w, http.StatusBadRequest, detail)
	case errors.Is(err, feedbacklearning.ErrInvalid):
		h.feedbackDiagnosticError(w, http.StatusBadRequest, nil)
	case errors.Is(err, feedbacklearning.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "feedback storage unavailable")
	}
}

func (h *Handler) feedbackDiagnosticError(w http.ResponseWriter, status int, detail map[string]any) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "feedback-learning",
		Trace: w.Header().Get("X-Diagnostic-Trace"),
	})
	body := map[string]any{
		"error": event.Message, "code": event.Code, "trace_id": event.Trace,
		"component": event.Component, "retryable": false,
		"next_action": event.Next,
	}
	for key, value := range detail {
		body[key] = value
	}
	writeJSON(w, status, body)
}

// RecordContentMetric writes one observation from the form.
//
// The source type is NOT in the request body. This endpoint means "a person
// typed it", the import endpoint means "it came out of a paste", and the
// server writes that - a caller that could declare where its data came from is
// not reporting a source.
func (h *Handler) RecordContentMetric(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.MetricInput
	if !decodeFeedbackBody(w, r, &body) {
		h.feedbackError(w, feedbacklearning.ErrInvalid)
		return
	}
	written, err := h.feedbackStore().Record(r.Context(), workspace, actor, body,
		feedbacklearning.SourceManual)
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, written)
}

// ImportContentMetrics writes a pasted batch, or none of it.
//
// All or nothing: a partial write leaves somebody believing all forty rows
// landed, and "how many got in" is the only question this feature has to
// answer.
func (h *Handler) ImportContentMetrics(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Metrics []feedbacklearning.MetricInput `json:"metrics"`
	}
	if !decodeFeedbackBody(w, r, &body) {
		h.feedbackError(w, feedbacklearning.ErrInvalid)
		return
	}
	written, err := h.feedbackStore().RecordBatch(r.Context(), workspace, actor, body.Metrics,
		feedbacklearning.SourceCSVImport)
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"metrics": written, "recorded": len(written),
	})
}

// ListContentMetrics returns observations, newest sample first.
//
// Rows, never totals. Adding them up is 10.2's business, and a convenience
// total here is the first step towards ranking two platforms' incomparable
// numbers against each other.
func (h *Handler) ListContentMetrics(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	metrics, err := h.feedbackStore().ListMetrics(r.Context(), workspace, actor,
		r.URL.Query().Get("publication_record_id"), r.URL.Query().Get("metric"))
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics})
}
