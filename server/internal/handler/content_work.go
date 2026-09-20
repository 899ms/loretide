package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Works and their documents (specs/024, SOP 7).
//
// The module stores topic_card_id as a plain string and does not import
// topic-planning; confirming the card exists in this workspace is the adapter's
// job, which is here.

func (h *Handler) workScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
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

type workDatabase struct {
	dbExecutor
	txStarter
}

func (d workDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, workeditor.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d workDatabase) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dbExecutor == nil {
		return nil, workeditor.ErrStorage
	}
	return d.dbExecutor.Query(ctx, sql, args...)
}

func (d workDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: workeditor.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

func (h *Handler) workEditorStore() *workeditor.Store {
	var diagnosticStore workeditor.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &workeditor.Store{
		DB:          workDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics: diagnosticStore,
		// The same fence the diagnostics store holds, handed over directly so
		// the write transaction takes the workspace delete lock itself instead
		// of inheriting it from whether it happened to audit first (#104).
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

func workIDFromURL(r *http.Request) string     { return chi.URLParam(r, "id") }
func artifactIDFromURL(r *http.Request) string { return chi.URLParam(r, "artifactId") }
func versionIDFromURL(r *http.Request) string  { return chi.URLParam(r, "versionId") }

func decodeWorkBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

func (h *Handler) workError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workeditor.ErrInvalid):
		h.workDiagnosticError(w, http.StatusBadRequest, false)
	case errors.Is(err, workeditor.ErrConflict):
		// Two writers raced for one revision number. Retryable, and not the
		// caller's mistake, so it says so rather than collapsing into 400.
		h.workDiagnosticError(w, http.StatusConflict, true)
	case errors.Is(err, workeditor.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "work editor unavailable")
	}
}

func (h *Handler) workDiagnosticError(w http.ResponseWriter, status int, retryable bool) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "work-editor",
		Trace: w.Header().Get("X-Diagnostic-Trace"),
	})
	writeJSON(w, status, map[string]any{
		"error": event.Message, "code": event.Code, "trace_id": event.Trace,
		"component": event.Component, "retryable": retryable,
		"next_action": event.Next,
	})
}

type createWorkRequest struct {
	// TopicCardID may be "" for a historical import (SOP 3.3): a piece that
	// was already published never came from a topic card here. Any other work
	// still needs one, and a non-empty value is still confirmed to exist.
	TopicCardID string `json:"topic_card_id"`
	SnapshotID  string `json:"snapshot_id"`
	Title       string `json:"title"`
	// HistoricalImport is read on creation and never again - there is no
	// update path that names the column, and a guard test in work-editor
	// asserts none appears.
	HistoricalImport bool `json:"historical_import"`
}

// CreateContentWork opens a work on a topic card.
func (h *Handler) CreateContentWork(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	var body createWorkRequest
	if !decodeWorkBody(w, r, &body) {
		h.workError(w, workeditor.ErrInvalid)
		return
	}
	// The card is confirmed here rather than inside the module: work-editor is
	// registered against workspace-core and diagnostics only, and importing
	// topic-planning to ask one existence question would widen that for good.
	//
	// "" skips the check instead of failing it. A historical import has no
	// card, and topicCardExists answers false for "" - which is right for its
	// own question ("is this a card here?") and wrong as a refusal.
	//
	// Note what this does NOT do: it does not require historical_import to be
	// true for a card-less work. Whether the body says so is the caller's
	// statement about provenance, and coupling the two would make an operator
	// unable to record one without the other.
	if body.TopicCardID != "" && !h.topicCardExists(r, workspace, body.TopicCardID) {
		h.workError(w, workeditor.ErrNotFound)
		return
	}
	created, err := h.workEditorStore().CreateWork(r.Context(), workspace, actor, workeditor.Work{
		TopicCardID: body.TopicCardID, SnapshotID: body.SnapshotID, Title: body.Title,
		HistoricalImport: body.HistoricalImport,
	})
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// topicCardExists answers the one question work-editor would otherwise have to
// import a module for. A missing card is refused exactly like a foreign one.
func (h *Handler) topicCardExists(r *http.Request, workspaceID, topicCardID string) bool {
	if h.DB == nil || workspaceID == "" || topicCardID == "" {
		return false
	}
	var exists bool
	if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM content_topic_card
		WHERE workspace_id=$1 AND topic_card_id=$2)`, workspaceID, topicCardID).Scan(&exists); err != nil {
		return false
	}
	return exists
}

func (h *Handler) ListContentWorks(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	works, err := h.workEditorStore().ListWorks(r.Context(), workspace, actor,
		r.URL.Query().Get("topic_card_id"))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"works": works})
}

func (h *Handler) GetContentWork(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	work, err := h.workEditorStore().GetWork(r.Context(), workspace, actor, workIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, work)
}

// UpdateContentWork changes the title. There is no status to change here: SOP
// 7.1's states belong to a document's editing copy, and the rest are the
// business of review-delivery and the handover card.
func (h *Handler) UpdateContentWork(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if !decodeWorkBody(w, r, &body) {
		h.workError(w, workeditor.ErrInvalid)
		return
	}
	work, err := h.workEditorStore().RenameWork(r.Context(), workspace, actor, workIDFromURL(r), body.Title)
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, work)
}

type createArtifactRequest struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Position int64  `json:"position"`
}

func (h *Handler) CreateContentArtifact(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	var body createArtifactRequest
	if !decodeWorkBody(w, r, &body) {
		h.workError(w, workeditor.ErrInvalid)
		return
	}
	created, err := h.workEditorStore().CreateArtifact(r.Context(), workspace, actor, workIDFromURL(r),
		workeditor.Artifact{
			Kind: workeditor.Kind(body.Kind), Title: body.Title, Position: body.Position,
		})
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) ListContentArtifacts(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	artifacts, err := h.workEditorStore().ListArtifacts(r.Context(), workspace, actor, workIDFromURL(r))
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
}

// PatchContentArtifact is the autosave endpoint. It never produces a version.
//
// Every field is optional: a keystroke changes the body and nothing else, and a
// rename must not blank the draft.
func (h *Handler) PatchContentArtifact(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.workScope(w, r)
	if !ok {
		return
	}
	var patch workeditor.ArtifactPatch
	if !decodeWorkBody(w, r, &patch) {
		h.workError(w, workeditor.ErrInvalid)
		return
	}
	artifact, err := h.workEditorStore().PatchArtifact(r.Context(), workspace, actor,
		workIDFromURL(r), artifactIDFromURL(r), patch)
	if err != nil {
		h.workError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}
