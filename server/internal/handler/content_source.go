package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	sourceinbox "github.com/multica-ai/multica/server/internal/content/source-inbox"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// The material inbox (specs/028, SOP §4).
//
// Nothing here fetches a URL. The module cannot - a guard test scans it for any
// outbound call - and this adapter does not do it on the module's behalf
// either: a url arrives, is checked for shape, and is stored.

func (h *Handler) sourceScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
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

type sourceDatabase struct {
	dbExecutor
	txStarter
}

func (d sourceDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, sourceinbox.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d sourceDatabase) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dbExecutor == nil {
		return nil, sourceinbox.ErrStorage
	}
	return d.dbExecutor.Query(ctx, sql, args...)
}

func (d sourceDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: sourceinbox.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

func (h *Handler) sourceInboxStore() *sourceinbox.Store {
	var diagnosticStore sourceinbox.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &sourceinbox.Store{
		DB:          sourceDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics: diagnosticStore,
		// The same fence the diagnostics store holds, handed over directly so
		// the write transaction takes the workspace delete lock itself instead
		// of inheriting it from whether it happened to audit first (#104).
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

func sourceIDFromURL(r *http.Request) string { return chi.URLParam(r, "sourceId") }

func decodeSourceBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

// sourceError maps the module's errors. A refusal and a missing row answer
// identically, so a caller cannot use the response to learn that something
// exists in another workspace.
func (h *Handler) sourceError(w http.ResponseWriter, err error) {
	var fieldErr sourceinbox.FieldError
	switch {
	case errors.As(err, &fieldErr):
		h.sourceDiagnosticError(w, http.StatusBadRequest, map[string]any{
			"field": fieldErr.Field, "reason": fieldErr.Reason,
		})
	case errors.Is(err, sourceinbox.ErrInvalid):
		h.sourceDiagnosticError(w, http.StatusBadRequest, nil)
	case errors.Is(err, sourceinbox.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "source inbox unavailable")
	}
}

func (h *Handler) sourceDiagnosticError(w http.ResponseWriter, status int, detail map[string]any) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "source-inbox",
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

func (h *Handler) ListContentSources(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	sources, err := h.sourceInboxStore().ListSources(r.Context(), workspace, actor,
		r.URL.Query().Get("status"), r.URL.Query().Get("tag"))
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

type createSourceRequest struct {
	Kind              string   `json:"kind"`
	URL               string   `json:"url"`
	Content           string   `json:"content"`
	Title             string   `json:"title"`
	Tags              []string `json:"tags"`
	Annotation        string   `json:"annotation"`
	PersonalJudgement string   `json:"personal_judgement"`
	HistoricalImport  bool     `json:"historical_import"`
}

// CreateContentSource records one act of collecting.
//
// The response carries `duplicates`: the items that already hold this exact
// content. It is a hint and nothing more - the new item was created, and
// neither it nor the older ones were touched (R-011).
func (h *Handler) CreateContentSource(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	var body createSourceRequest
	if !decodeSourceBody(w, r, &body) {
		h.sourceDiagnosticError(w, http.StatusBadRequest, nil)
		return
	}
	created, err := h.sourceInboxStore().CreateSource(r.Context(), workspace, actor,
		sourceinbox.NewSource{
			Kind:              sourceinbox.Kind(body.Kind),
			URL:               body.URL,
			Content:           body.Content,
			Title:             body.Title,
			Tags:              body.Tags,
			Annotation:        body.Annotation,
			PersonalJudgement: body.PersonalJudgement,
			HistoricalImport:  body.HistoricalImport,
		})
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"source": created.Source, "snapshot": created.Snapshot,
		"duplicates": created.Duplicates,
	})
}

func (h *Handler) GetContentSource(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	source, snapshot, err := h.sourceInboxStore().GetSource(r.Context(), workspace, actor, sourceIDFromURL(r))
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source, "snapshot": snapshot})
}

// organizeSourceRequest is the mutable half only.
//
// The five immutable columns are absent, and DisallowUnknownFields turns an
// attempt to send one into a 400 rather than a silent no-op: a caller that
// tried to change captured_at should be told it cannot, not left believing it
// worked.
type organizeSourceRequest struct {
	Title             *string   `json:"title"`
	Tags              *[]string `json:"tags"`
	Annotation        *string   `json:"annotation"`
	PersonalJudgement *string   `json:"personal_judgement"`
	Status            *string   `json:"status"`
}

func (h *Handler) OrganizeContentSource(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	var body organizeSourceRequest
	if !decodeSourceBody(w, r, &body) {
		h.sourceDiagnosticError(w, http.StatusBadRequest, nil)
		return
	}
	patch := sourceinbox.Organize{
		Title: body.Title, Tags: body.Tags,
		Annotation: body.Annotation, PersonalJudgement: body.PersonalJudgement,
	}
	if body.Status != nil {
		status := sourceinbox.Status(*body.Status)
		patch.Status = &status
	}
	source, err := h.sourceInboxStore().OrganizeSource(r.Context(), workspace, actor, sourceIDFromURL(r), patch)
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source})
}

func (h *Handler) ListContentSourceRevisions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	// The source is loaded first so that a revision list for another brand's
	// id answers 404 rather than an empty list: an empty list would confirm
	// the id parses while saying nothing exists, and those are different
	// answers to give a stranger.
	if _, _, err := h.sourceInboxStore().GetSource(r.Context(), workspace, actor, sourceIDFromURL(r)); err != nil {
		h.sourceError(w, err)
		return
	}
	revisions, err := h.sourceInboxStore().ListRevisions(r.Context(), workspace, actor, sourceIDFromURL(r))
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
}

type bulkSourceRequest struct {
	SourceIDs []string `json:"source_ids"`
	AddTags   []string `json:"add_tags"`
	Status    *string  `json:"status"`
}

func (h *Handler) BulkOrganizeContentSources(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	var body bulkSourceRequest
	if !decodeSourceBody(w, r, &body) {
		h.sourceDiagnosticError(w, http.StatusBadRequest, nil)
		return
	}
	var status *sourceinbox.Status
	if body.Status != nil {
		value := sourceinbox.Status(*body.Status)
		status = &value
	}
	results, err := h.sourceInboxStore().BulkOrganize(r.Context(), workspace, actor,
		body.SourceIDs, body.AddTags, status)
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// ContentSourceDuplicates answers "what already holds this content".
//
// A read. It returns ids and changes nothing: merging two collections is a
// person's decision (§4), and removing one would throw away its own annotation,
// time and collector (R-011).
func (h *Handler) ContentSourceDuplicates(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.sourceScope(w, r)
	if !ok {
		return
	}
	ids, err := h.sourceInboxStore().DuplicatesByHash(r.Context(), workspace, actor,
		r.URL.Query().Get("content_hash"))
	if err != nil {
		h.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source_ids": ids})
}
