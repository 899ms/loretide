package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/content/idempotency"
	reviewdelivery "github.com/multica-ai/multica/server/internal/content/review-delivery"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Review requests (specs/025, SOP 8).
//
// The module stores work_id / artifact_id / account_id as plain strings and
// imports neither work-editor nor ip-profile; confirming that a version belongs
// here and that the document is a channel draft is the adapter's job, which is
// here.

func (h *Handler) reviewScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
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

type reviewDatabase struct {
	dbExecutor
	txStarter
}

func (d reviewDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, reviewdelivery.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d reviewDatabase) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dbExecutor == nil {
		return nil, reviewdelivery.ErrStorage
	}
	return d.dbExecutor.Query(ctx, sql, args...)
}

func (d reviewDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: reviewdelivery.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

// reviewArtifacts answers the two questions review-delivery would otherwise
// have to import work-editor for: does this version belong to this document in
// this workspace, and what kind of document is it.
//
// An empty versionID asks about the document alone - a delivery task points at
// a document, and which version goes out comes from the approved request.
type reviewArtifacts struct{ db dbExecutor }

func (a reviewArtifacts) ResolveVersion(ctx context.Context, workspaceID, artifactID, versionID string) (string, string, error) {
	if a.db == nil || workspaceID == "" || artifactID == "" {
		return "", "", reviewdelivery.ErrNotFound
	}
	var workID, kind string
	var err error
	if versionID == "" {
		err = a.db.QueryRow(ctx, `SELECT work_id, kind FROM content_artifact
			WHERE workspace_id=$1 AND artifact_id=$2`, workspaceID, artifactID).Scan(&workID, &kind)
	} else {
		// Joined rather than asked in two queries: a version id from another
		// document - or another brand - must be refused exactly like a missing
		// one, and two queries give two chances to answer differently.
		err = a.db.QueryRow(ctx, `SELECT a.work_id, a.kind FROM content_artifact a
			JOIN content_artifact_version v
			  ON v.workspace_id = a.workspace_id AND v.artifact_id = a.artifact_id
			WHERE a.workspace_id=$1 AND a.artifact_id=$2 AND v.version_id=$3`,
			workspaceID, artifactID, versionID).Scan(&workID, &kind)
	}
	if err != nil {
		return "", "", reviewdelivery.ErrNotFound
	}
	return workID, kind, nil
}

func (h *Handler) reviewDeliveryStore() *reviewdelivery.Store {
	var diagnosticStore reviewdelivery.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &reviewdelivery.Store{
		DB:          reviewDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics: diagnosticStore,
		Artifacts:   reviewArtifacts{db: h.DB},
		// The same fence the diagnostics store holds, handed over directly so
		// the write transaction takes the workspace delete lock itself instead
		// of inheriting it from whether it happened to audit first (#104).
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

func reviewIDFromURL(r *http.Request) string   { return chi.URLParam(r, "reviewId") }
func deliveryIDFromURL(r *http.Request) string { return chi.URLParam(r, "deliveryId") }

func decodeReviewBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1*1024*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

// reviewError maps the module's errors. A refusal and a missing row answer
// identically, so a caller cannot use the response to learn that something
// exists in another workspace.
func (h *Handler) reviewError(w http.ResponseWriter, err error) {
	var fieldErr reviewdelivery.FieldError
	var transitionErr reviewdelivery.TransitionError
	switch {
	case errors.Is(err, idempotency.ErrInvalid):
		writeError(w, http.StatusBadRequest, "Idempotency-Key is required and must be at most 255 bytes")
	case errors.Is(err, idempotency.ErrConflict):
		writeError(w, http.StatusConflict, "Idempotency-Key conflicts with a different import request")
	case errors.As(err, &fieldErr):
		h.reviewDiagnosticError(w, http.StatusBadRequest, map[string]any{
			"field": fieldErr.Field, "reason": fieldErr.Reason,
		})
	case errors.As(err, &transitionErr):
		h.reviewDiagnosticError(w, http.StatusBadRequest, map[string]any{
			"from": transitionErr.From, "to": transitionErr.To,
		})
	case errors.Is(err, reviewdelivery.ErrInvalid):
		h.reviewDiagnosticError(w, http.StatusBadRequest, nil)
	case errors.Is(err, reviewdelivery.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "review delivery unavailable")
	}
}

// reviewDiagnosticError names the field or the refused move.
//
// This is deliberately a different shape from a refusal: the input is the
// caller's own, so saying what is wrong with it reveals nothing about anyone
// else, and "参数错误" would leave the person filling the form with nothing to
// act on.
func (h *Handler) reviewDiagnosticError(w http.ResponseWriter, status int, detail map[string]any) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "review-delivery",
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

type submitReviewRequest struct {
	ArtifactID      string `json:"artifact_id"`
	VersionID       string `json:"version_id"`
	AccountID       string `json:"account_id"`
	Channel         string `json:"channel"`
	StartSnapshotID string `json:"start_snapshot_id"`
}

// SubmitContentReview freezes one channel draft version for review.
func (h *Handler) SubmitContentReview(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	var body submitReviewRequest
	if !decodeReviewBody(w, r, &body) {
		h.reviewError(w, reviewdelivery.ErrInvalid)
		return
	}
	created, err := h.reviewDeliveryStore().Submit(r.Context(), workspace, actor,
		reviewdelivery.SubmitRequest{
			ArtifactID: body.ArtifactID, VersionID: body.VersionID,
			AccountID: body.AccountID, Channel: reviewdelivery.Channel(body.Channel),
			StartSnapshotID: body.StartSnapshotID,
		})
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) ListContentReviews(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	requests, err := h.reviewDeliveryStore().ListReviews(r.Context(), workspace, actor,
		r.URL.Query().Get("artifact_id"), r.URL.Query().Get("status"))
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": requests})
}

// GetContentReview returns one request with its frozen snapshot and the whole
// of its transition history.
func (h *Handler) GetContentReview(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	store := h.reviewDeliveryStore()
	request, err := store.GetReview(r.Context(), workspace, actor, reviewIDFromURL(r))
	if err != nil {
		h.reviewError(w, err)
		return
	}
	transitions, err := store.ListTransitions(r.Context(), workspace, actor,
		reviewdelivery.SubjectReviewRequest, request.ReviewRequestID)
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"review": request, "transitions": transitions,
	})
}

// DecideContentReview records a disposition.
//
// There is no path here that a model could take: SOP 8 says the AI self-check
// is reference material and cannot perform the human approval, and the actor is
// the session's human header. Constitution IX keeps executors disabled besides.
func (h *Handler) DecideContentReview(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !decodeReviewBody(w, r, &body) {
		h.reviewError(w, reviewdelivery.ErrInvalid)
		return
	}
	request, err := h.reviewDeliveryStore().Decide(r.Context(), workspace, actor,
		reviewIDFromURL(r), reviewdelivery.ReviewStatus(body.Status), body.Note)
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, request)
}
