package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

type topicSourceReader struct {
	db dbExecutor
}

func (r topicSourceReader) Exists(ctx context.Context, workspaceID, sourceID string) (bool, error) {
	if r.db == nil {
		return false, topicplanning.ErrStorage
	}
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM content_source WHERE workspace_id=$1 AND source_id=$2
	)`, workspaceID, sourceID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

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
		Sources:     topicSourceReader{db: h.DB},
		// The same guard the diagnostics store holds, handed over directly so
		// the topic write transaction takes the workspace delete fence itself
		// instead of inheriting it from whether it happened to audit first.
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

func decodeTopicBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

// decodeTopicBodyPatch is stricter than the older topic-body readers: the
// PATCH contract rejects a second JSON value as malformed input. Keep that
// rule local to this new endpoint so existing topic API contracts do not gain
// a new rejection behavior as a side effect.
func decodeTopicBodyPatch(w http.ResponseWriter, r *http.Request, target *topicplanning.TopicBodyPatch) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
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
	// The filter is read from the query string, not from a body: this is a GET
	// and a bookmarked or shared URL should come back to the same list.
	cards, err := h.topicPlanningStore().List(r.Context(), workspace, actor,
		r.URL.Query().Get("account_id"))
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

// PatchContentTopicBody changes only the five authored-text fields that have
// no independent resource or decision semantics. Account, source, action, and
// brief endpoints remain the sole owners of their respective mutations.
func (h *Handler) PatchContentTopicBody(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var body topicplanning.TopicBodyPatch
	if !decodeTopicBodyPatch(w, r, &body) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	if err := body.Validate(); err != nil {
		h.topicError(w, err)
		return
	}
	card, err := h.topicPlanningStore().PatchBody(r.Context(), workspace, actor,
		topicCardIDFromURL(r), body)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// SetContentTopicAccount attaches the card to one of the brand's accounts, or
// detaches it. Its own endpoint rather than a field on the actions body, so a
// decision cannot re-target a card as a side effect.
func (h *Handler) SetContentTopicAccount(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var body struct {
		AccountID *string `json:"account_id"`
	}
	if !decodeTopicBody(w, r, &body) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	// null and "" both mean "no account". A client that clears a select sends
	// one or the other depending on how it models an empty choice, and both
	// readings of "nothing chosen" are the same state here.
	accountID := body.AccountID
	if accountID != nil && *accountID == "" {
		accountID = nil
	}
	card, err := h.topicPlanningStore().SetAccount(r.Context(), workspace, actor,
		topicCardIDFromURL(r), accountID)
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

type setTopicSourcesRequest struct {
	FitSourceIDs      *[]string `json:"fit_source_ids"`
	EvidenceSourceIDs *[]string `json:"evidence_source_ids"`
}

// SetContentTopicSources updates or clears the material references justifying
// a topic card or serving as evidence. Its own endpoint rather than a field
// on the action body, so changing sources does not advance or mutate card state.
func (h *Handler) SetContentTopicSources(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var body setTopicSourcesRequest
	if !decodeTopicBody(w, r, &body) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	topicID := topicCardIDFromURL(r)
	card, err := h.topicPlanningStore().SetSources(r.Context(), workspace, actor,
		topicID, body.FitSourceIDs, body.EvidenceSourceIDs)
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
	var fieldErr topicplanning.FieldError
	switch {
	case errors.As(err, &fieldErr):
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": event.Retryable,
			"next_action": event.Next,
			"field": fieldErr.Field, "reason": fieldErr.Reason,
		})
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
