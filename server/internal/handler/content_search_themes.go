package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Platform search optimization (specs/036 PR 1): search themes under
// /api/content-search.
//
// This file is the adapter. topic-planning owns the themes; the two things a
// theme write asks of other modules - does this content account exist and on
// which platform (ip-profile), does this material exist (content_source,
// read the way content_topic.go's topicSourceReader reads it) - are answered
// below, each mapped to topic-planning's own errors by searchThemesReadError.
//
// Mounted outside the generic member middleware, like /api/content-topics:
// every handler goes through searchScope, so each decision - refusals
// included - reaches workspace-core.Authorize and is recorded. The brand
// comes from X-Workspace-ID; the theme id comes only from chi.URLParam.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md §4.1, §7

const maxSearchThemeBody = 1 << 20

// searchThemesReadError maps what an owning module's read says about a
// workspace or a record that is not there - deleted, or never this caller's -
// to topic-planning's ErrNotFound, and anything else to ErrStorage. Once a
// workspace deletion has committed, its accounts and materials are gone
// with it; answering that as a storage failure would turn a write the fence
// should refuse with 404 into a 503, and answering a database failure as
// "not found" would hide an outage behind a 404 (Issue #274 is that second
// bug elsewhere). Same shape as content_opdiag_reports.go's opdiagReadError.
func searchThemesReadError(err error) error {
	switch {
	case errors.Is(err, workspacecore.ErrNotFound), errors.Is(err, ipprofile.ErrNotFound),
		errors.Is(err, topicplanning.ErrNotFound):
		return topicplanning.ErrNotFound
	default:
		return topicplanning.ErrStorage
	}
}

// searchThemeAccounts answers from ip-profile's public reads.
type searchThemeAccounts struct{ service *ipprofile.Service }

func (a searchThemeAccounts) Get(ctx context.Context, workspaceID, accountID string) (ipprofile.Account, error) {
	if a.service == nil {
		return ipprofile.Account{}, topicplanning.ErrStorage
	}
	account, err := a.service.Get(ctx, workspaceID, accountID)
	if err != nil {
		return ipprofile.Account{}, searchThemesReadError(err)
	}
	return account, nil
}

func (a searchThemeAccounts) CurrentPersonaRevision(ctx context.Context, workspaceID, accountID string) (ipprofile.Revision, error) {
	if a.service == nil {
		return ipprofile.Revision{}, topicplanning.ErrStorage
	}
	revision, err := a.service.CurrentPersonaRevision(ctx, workspaceID, accountID)
	if err != nil {
		return ipprofile.Revision{}, searchThemesReadError(err)
	}
	return revision, nil
}

// searchThemeSources answers whether a material is this brand's. A material
// that is not here - including every material of a deleted workspace - is
// "no"; only a failed read is an error, and it is ErrStorage.
type searchThemeSources struct{ db dbExecutor }

func (s searchThemeSources) Exists(ctx context.Context, workspaceID, sourceID string) (bool, error) {
	if s.db == nil {
		return false, topicplanning.ErrStorage
	}
	exists, err := topicSourceReader{db: s.db}.Exists(ctx, workspaceID, sourceID)
	if err != nil {
		return false, searchThemesReadError(err)
	}
	return exists, nil
}

// searchThemesStore is the topic planning store with the theme adapters.
func (h *Handler) searchThemesStore() *topicplanning.Store {
	store := h.topicPlanningStore()
	store.Accounts = searchThemeAccounts{service: h.contentAccountService()}
	store.Sources = searchThemeSources{db: h.DB}
	return store
}

// searchScope is topicScope: the same canonical boundary, the same refusal.
func (h *Handler) searchScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	return h.topicScope(w, r)
}

func searchThemeIDFromURL(r *http.Request) string { return chi.URLParam(r, "themeId") }

// readSearchBody reads the whole body, bounded. The module decodes it
// strictly, naming an unknown or wrongly typed member.
func readSearchBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSearchThemeBody))
	return body, err == nil
}

// searchError adds 409 for a stale revision to the topic answers: 400
// naming the field, 404 identical to "no such workspace" for anything
// foreign or missing, 503 for storage.
func (h *Handler) searchError(w http.ResponseWriter, err error) {
	if conflict, ok := errors.AsType[topicplanning.SearchConflict](err); ok {
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": false, "next_action": event.Next,
			"field": conflict.Field, "reason": "the theme changed since this revision was read",
		})
		return
	}
	h.topicError(w, err)
}

// ListContentSearchThemes answers the current revision of each theme,
// filtered by platform, account_id, topic_card_id and include_archived.
func (h *Handler) ListContentSearchThemes(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	filter, err := topicplanning.ParseThemeFilter(query.Get("platform"), query.Get("account_id"),
		query.Get("topic_card_id"), query.Get("include_archived"))
	if err != nil {
		h.searchError(w, err)
		return
	}
	themes, err := h.searchThemesStore().ListSearchThemes(r.Context(), workspace, actor, filter)
	if err != nil {
		h.searchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"themes": themes})
}

// CreateContentSearchTheme records revision 1 of a new theme.
func (h *Handler) CreateContentSearchTheme(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		h.searchError(w, topicplanning.ErrInvalid)
		return
	}
	content, err := topicplanning.DecodeThemeCreate(body)
	if err != nil {
		h.searchError(w, err)
		return
	}
	theme, err := h.searchThemesStore().CreateSearchTheme(r.Context(), workspace, actor, content)
	if err != nil {
		h.searchError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, theme)
}

// GetContentSearchTheme answers the theme's current revision.
func (h *Handler) GetContentSearchTheme(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	theme, err := h.searchThemesStore().GetSearchTheme(r.Context(), workspace, actor, searchThemeIDFromURL(r))
	if err != nil {
		h.searchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, theme)
}

// ListContentSearchThemeRevisions answers every revision, newest first.
func (h *Handler) ListContentSearchThemeRevisions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	themeID := searchThemeIDFromURL(r)
	revisions, err := h.searchThemesStore().ListSearchThemeRevisions(r.Context(), workspace, actor, themeID)
	if err != nil {
		h.searchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"theme_id": themeID, "revisions": revisions})
}

// ReviseContentSearchTheme appends a revision; voided = true archives.
//
// Contract §7.1 puts the path before the body: a theme that is not here
// answers 404 whatever the body holds, so it is read before the body is
// decoded. The store reads it again inside the fenced transaction.
func (h *Handler) ReviseContentSearchTheme(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	themeID := searchThemeIDFromURL(r)
	store := h.searchThemesStore()
	if _, err := store.GetSearchTheme(r.Context(), workspace, actor, themeID); err != nil {
		h.searchError(w, err)
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		h.searchError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeThemeRevision(body)
	if err != nil {
		h.searchError(w, err)
		return
	}
	theme, err := store.ReviseSearchTheme(r.Context(), workspace, actor, themeID, req)
	if err != nil {
		h.searchError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, theme)
}
