package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// topicScope delegates every decision to workspace-core. The topic routes are
// deliberately outside the generic member middleware so this canonical
// boundary records refusals and keeps missing workspaces indistinguishable from
// foreign ones.
func (h *Handler) topicScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
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

type topicDatabase struct {
	dbExecutor
	txStarter
}

func (d topicDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, topicplanning.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d topicDatabase) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dbExecutor == nil {
		return nil, topicplanning.ErrStorage
	}
	return d.dbExecutor.Query(ctx, sql, args...)
}

func (d topicDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: topicplanning.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }

func (h *Handler) topicPlanningStore() *topicplanning.Store {
	var diagnosticStore topicplanning.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &topicplanning.Store{
		DB:          topicDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics: diagnosticStore,
		Accounts:    h.contentAccountService(),
		Build:       build,
	}
}

func decodeTopicBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

func topicCardIDFromURL(r *http.Request) string { return chi.URLParam(r, "id") }

func (h *Handler) CreateContentTopic(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var card topicplanning.TopicCard
	if !decodeTopicBody(w, r, &card) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	card.WorkspaceID = workspace
	created, err := h.topicPlanningStore().Create(r.Context(), actor, card)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) ListContentTopics(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	cards, err := h.topicPlanningStore().List(r.Context(), workspace, actor)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topic_cards": cards})
}

func (h *Handler) GetContentTopic(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	card, err := h.topicPlanningStore().Get(r.Context(), workspace, actor, topicCardIDFromURL(r))
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (h *Handler) ActOnContentTopic(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var body topicplanning.ActionRequest
	if !decodeTopicBody(w, r, &body) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	result, err := h.topicPlanningStore().Act(r.Context(), workspace, actor, topicCardIDFromURL(r), body)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AppendContentBrief(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var body topicplanning.BriefRevision
	if !decodeTopicBody(w, r, &body) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	revision, err := h.topicPlanningStore().AppendBrief(r.Context(), workspace, actor, topicCardIDFromURL(r), body)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, revision)
}

func (h *Handler) ListContentBriefs(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	revisions, err := h.topicPlanningStore().ListBriefs(r.Context(), workspace, actor, topicCardIDFromURL(r))
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"brief_revisions": revisions})
}

func (h *Handler) GetContentBrief(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	revision, err := h.topicPlanningStore().GetBrief(r.Context(), workspace, actor,
		topicCardIDFromURL(r), chi.URLParam(r, "revisionId"))
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

func (h *Handler) topicError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, topicplanning.ErrInvalid):
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": event.Retryable,
			"next_action": event.Next,
		})
	case errors.Is(err, topicplanning.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "topic planning unavailable")
	}
}
