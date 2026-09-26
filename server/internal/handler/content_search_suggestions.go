package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/content/idempotency"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Platform search optimization (specs/036 PR 2): suggestions, comparison and
// abandoning under /api/content-search.
//
// This file is the adapter. topic-planning owns suggestions and does not
// import work-editor (FR-101); what a suggestion needs to know about a
// document - its latest version, whether its draft is saved, the body of one
// version - is answered below by searchWorks over work-editor's public Store,
// each read mapped to topic-planning's own errors by searchSuggestionsReadError.
// PR 2 has reads only; PR 3 adds the one write (Apply) to this same adapter.
//
// Mounted outside the generic member middleware, like the theme routes:
// every handler goes through searchScope, so each decision - refusals
// included - reaches workspace-core.Authorize and is recorded. The brand
// comes from X-Workspace-ID; the suggestion id only from chi.URLParam.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md §4.2, §7

// maxSearchSuggestionBody bounds a request: a whole document body of up to
// 200000 characters, each possibly escaped, and the rest.
const maxSearchSuggestionBody = 4 << 20

// searchSuggestionsReadError maps what work-editor or workspace-core says
// about a work, document or version that is not there - missing, another
// brand's, malformed, or gone with a deleted workspace - to topic-planning's
// ErrNotFound, and anything else to ErrStorage. Once a workspace deletion has
// committed its documents are gone with it; answering that as a storage
// failure would turn a 404 into a 503, and answering a database failure as
// "not found" would hide an outage (#274). Same shape as
// content_opdiag_reports.go's opdiagReadError, and not shared with it: each
// adapter maps to its own caller's errors.
func searchSuggestionsReadError(err error) error {
	switch {
	case errors.Is(err, workspacecore.ErrNotFound), errors.Is(err, workeditor.ErrNotFound),
		errors.Is(err, workeditor.ErrInvalid), errors.Is(err, topicplanning.ErrNotFound):
		return topicplanning.ErrNotFound
	case errors.Is(err, workeditor.ErrBaseMoved):
		return topicplanning.ErrBaseMoved
	case errors.Is(err, workeditor.ErrDraftUnsaved):
		return topicplanning.ErrDraftUnsaved
	case errors.Is(err, workeditor.ErrNoChange):
		return topicplanning.ErrNoChange
	default:
		return topicplanning.ErrStorage
	}
}

// searchWorks answers topicplanning.SearchWorks from work-editor's public
// Store.
type searchWorks struct{ store *workeditor.Store }

// Document is the document's latest version and whether its editing copy is
// saved: GetArtifact, then ListVersions (newest first).
func (w searchWorks) Document(ctx context.Context, workspaceID, actor, workID, artifactID string) (topicplanning.SearchDocument, error) {
	if w.store == nil {
		return topicplanning.SearchDocument{}, topicplanning.ErrStorage
	}
	artifact, err := w.store.GetArtifact(ctx, workspaceID, actor, workID, artifactID)
	if err != nil {
		return topicplanning.SearchDocument{}, searchSuggestionsReadError(err)
	}
	versions, err := w.store.ListVersions(ctx, workspaceID, actor, workID, artifactID)
	if err != nil {
		return topicplanning.SearchDocument{}, searchSuggestionsReadError(err)
	}
	document := topicplanning.SearchDocument{
		WorkID: artifact.WorkID, ArtifactID: artifact.ArtifactID,
		DraftSaved: artifact.DraftStatus == workeditor.DraftSaved,
	}
	if len(versions) > 0 {
		document.LatestVersionID = versions[0].VersionID
	}
	return document, nil
}

// VersionBody is one version's body, scoped to the workspace and the
// document: a version of another document or brand is not there.
func (w searchWorks) VersionBody(ctx context.Context, workspaceID, actor, workID, artifactID, versionID string) (string, error) {
	if w.store == nil {
		return "", topicplanning.ErrStorage
	}
	version, err := w.store.GetVersion(ctx, workspaceID, actor, workID, artifactID, versionID)
	if err != nil {
		return "", searchSuggestionsReadError(err)
	}
	return version.Body, nil
}

// Apply turns a stable suggestion key and body into work-editor's public
// ApplyBody contract. The idempotency claim is deliberately owned by the
// version write, before that write checks whether the base has moved.
func (w searchWorks) Apply(ctx context.Context, workspaceID, actor string, apply topicplanning.SearchApply) (string, error) {
	if w.store == nil {
		return "", topicplanning.ErrStorage
	}
	bodyHash := sha256.Sum256([]byte(apply.Body))
	request, err := idempotency.NewRequest("apply-search-suggestion", apply.ArtifactID, apply.Key, struct {
		BaseVersionID string `json:"base_version_id"`
		BodySHA256    string `json:"body_sha256"`
	}{BaseVersionID: apply.BaseVersionID, BodySHA256: hex.EncodeToString(bodyHash[:])})
	if err != nil {
		return "", topicplanning.ErrStorage
	}
	version, err := w.store.ApplyBody(ctx, workspaceID, actor, apply.WorkID, apply.ArtifactID,
		workeditor.BodyApplication{BaseVersionID: apply.BaseVersionID, Body: apply.Body}, request)
	if err != nil {
		return "", searchSuggestionsReadError(err)
	}
	return version.VersionID, nil
}

// searchSuggestionsStore is the topic planning store with the theme adapters
// and the document reads.
func (h *Handler) searchSuggestionsStore() *topicplanning.SuggestionStore {
	return &topicplanning.SuggestionStore{
		Store: h.searchThemesStore(),
		Works: searchWorks{store: h.workEditorStore()},
	}
}

func searchSuggestionIDFromURL(r *http.Request) string { return chi.URLParam(r, "suggestionId") }

// readSearchSuggestionBody reads the whole body, bounded. The module decodes
// it strictly, naming an unknown or wrongly typed member.
func readSearchSuggestionBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSearchSuggestionBody))
	return body, err == nil
}

// searchSuggestionConflictReasons says, per field a 409 names, what moved.
var searchSuggestionConflictReasons = map[string]string{
	"base_revision":   "the suggestion changed since this revision was read",
	"revision":        "the suggestion changed since this revision was read",
	"suggestion_id":   "this suggestion already has a decision",
	"decision_id":     "this adoption is already complete",
	"base_version_id": "the document has a newer version than the suggestion base",
	"draft_status":    "the document has unsaved edits",
}

// searchSuggestionError adds 409 to the topic answers: 400 naming the field,
// 404 identical to "no such workspace" for anything foreign or missing, 503
// for storage.
func (h *Handler) searchSuggestionError(w http.ResponseWriter, err error) {
	if conflict, ok := errors.AsType[topicplanning.SearchConflict](err); ok {
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": false, "next_action": event.Next,
			"field": conflict.Field, "reason": searchSuggestionConflictReasons[conflict.Field],
		})
		return
	}
	h.topicError(w, err)
}

// ListContentSearchSuggestions answers the current revision of each
// suggestion, filtered by work_id, artifact_id, theme_id and state.
func (h *Handler) ListContentSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	filter, err := topicplanning.ParseSuggestionFilter(query.Get("work_id"), query.Get("artifact_id"),
		query.Get("theme_id"), query.Get("state"))
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	suggestions, err := h.searchSuggestionsStore().ListSearchSuggestions(r.Context(), workspace, actor, filter)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

// CreateContentSearchSuggestion records revision 1 of a new suggestion.
func (h *Handler) CreateContentSearchSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	body, ok := readSearchSuggestionBody(w, r)
	if !ok {
		h.searchSuggestionError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeSuggestionCreate(body)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	suggestion, err := h.searchSuggestionsStore().CreateSearchSuggestion(r.Context(), workspace, actor, req)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, suggestion)
}

// CompareContentSearchSuggestions answers ids=a,b[,c,d] side by side. Read
// only.
func (h *Handler) CompareContentSearchSuggestions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	ids, err := topicplanning.ParseCompareIDs(r.URL.Query().Get("ids"))
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	comparison, err := h.searchSuggestionsStore().CompareSearchSuggestions(r.Context(), workspace, actor, ids)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comparison)
}

// GetContentSearchSuggestion answers the current revision with its
// difference, decision, effects and state.
func (h *Handler) GetContentSearchSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	suggestion, err := h.searchSuggestionsStore().GetSearchSuggestion(r.Context(), workspace, actor, searchSuggestionIDFromURL(r))
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, suggestion)
}

// ReviseContentSearchSuggestion appends a revision.
//
// Contract §7.1 puts the path before the body: a suggestion that is not here
// answers 404 whatever the body holds. The store reads it again inside the
// fenced transaction.
func (h *Handler) ReviseContentSearchSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	suggestionID := searchSuggestionIDFromURL(r)
	store := h.searchSuggestionsStore()
	if err := store.SuggestionExists(r.Context(), workspace, actor, suggestionID); err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	body, ok := readSearchSuggestionBody(w, r)
	if !ok {
		h.searchSuggestionError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeSuggestionRevision(body)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	suggestion, err := store.ReviseSearchSuggestion(r.Context(), workspace, actor, suggestionID, req)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, suggestion)
}

// DecideContentSearchSuggestion records an adopt or abandon decision.
func (h *Handler) DecideContentSearchSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	suggestionID := searchSuggestionIDFromURL(r)
	store := h.searchSuggestionsStore()
	if err := store.SuggestionExists(r.Context(), workspace, actor, suggestionID); err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	body, ok := readSearchSuggestionBody(w, r)
	if !ok {
		h.searchSuggestionError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeSuggestionDecision(body)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	suggestion, err := store.DecideSearchSuggestion(r.Context(), workspace, actor, suggestionID, req)
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, suggestion)
}

// RetryContentSearchSuggestionDecision retries a decided adoption from its
// stored proposal; no new body or revision is accepted on this route.
func (h *Handler) RetryContentSearchSuggestionDecision(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.searchScope(w, r)
	if !ok {
		return
	}
	view, err := h.searchSuggestionsStore().RetrySearchSuggestionDecision(r.Context(), workspace, actor, chi.URLParam(r, "decisionId"))
	if err != nil {
		h.searchSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}
