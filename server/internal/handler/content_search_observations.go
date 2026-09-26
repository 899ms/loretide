package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// Search metrics and ranking observations (specs/036 PR 4) under
// /api/content-search.
//
// This file is the adapter. feedback-learning owns both; it stores theme,
// publication record and account ids as plain strings and imports none of
// their modules (FR-078). Whether each one is here is answered below:
//
//   - a theme, by searchThemes over topic-planning's public read;
//   - a publication record, by 027's feedbackPublications;
//   - an account, by 034's roiAccounts.
//
// Mounted outside the generic member middleware, like the themes: every
// handler goes through feedbackScope, so each decision - refusals included -
// reaches workspace-core.Authorize. The observation id comes only from
// chi.URLParam.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md
// §4.3, §4.4, §7, §7.3

// searchObservationsReadError maps what topic-planning's read says about a
// theme or a workspace that is not there - deleted, or never this caller's -
// to feedback-learning's ErrNotFound, and anything else to ErrStorage. Once a
// workspace deletion has committed its themes are gone with it; answering
// that as a storage failure would turn a write the fence should refuse with
// 404 into a 503, and answering a database failure as "not found" would hide
// an outage behind a 404. Same shape as content_opdiag_reports.go's
// opdiagReadError.
func searchObservationsReadError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspacecore.ErrNotFound), errors.Is(err, topicplanning.ErrNotFound),
		errors.Is(err, topicplanning.ErrInvalid), errors.Is(err, feedbacklearning.ErrNotFound):
		return feedbacklearning.ErrNotFound
	default:
		return feedbacklearning.ErrStorage
	}
}

// searchThemes answers "is this search theme here" through topic-planning's
// public read. An archived theme is here.
type searchThemes struct{ store *topicplanning.Store }

func (t searchThemes) ThemeExists(ctx context.Context, workspaceID, actor, themeID string) error {
	if t.store == nil {
		return feedbacklearning.ErrStorage
	}
	_, err := t.store.GetSearchTheme(ctx, workspaceID, actor, themeID)
	return searchObservationsReadError(err)
}

// feedbackSearchStore is the search half of feedback-learning with its three
// adapters: 027's publication records, 034's accounts and the themes above.
func (h *Handler) feedbackSearchStore() *feedbacklearning.SearchStore {
	return &feedbacklearning.SearchStore{
		Store:    h.feedbackStore(),
		Accounts: roiAccounts{service: h.contentAccountService()},
		Themes:   searchThemes{store: h.topicPlanningStore()},
	}
}

// searchObservationError adds 409 for a stale revision to the feedback
// answers: 400 naming the field, 404 identical to "no such workspace" for
// anything foreign or missing, 503 for storage.
func (h *Handler) searchObservationError(w http.ResponseWriter, err error) {
	if conflict, ok := errors.AsType[feedbacklearning.RevisionConflict](err); ok {
		h.feedbackDiagnosticError(w, http.StatusConflict, map[string]any{
			"field": conflict.Field, "reason": "stale revision",
		})
		return
	}
	h.feedbackError(w, err)
}

// ListContentSearchMetrics answers every search metric of one publication
// record, newest sample first. publication_record_id is required.
func (h *Handler) ListContentSearchMetrics(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	recordID := r.URL.Query().Get("publication_record_id")
	metrics, err := h.feedbackSearchStore().ListSearchMetrics(r.Context(), workspace, actor, recordID)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"publication_record_id": recordID, "metrics": metrics})
}

// RecordContentSearchMetric records one search metric. The source type is
// the endpoint's, never the body's.
func (h *Handler) RecordContentSearchMetric(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		h.searchObservationError(w, feedbacklearning.ErrInvalid)
		return
	}
	input, err := feedbacklearning.DecodeSearchMetric(body)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	metric, err := h.feedbackSearchStore().RecordSearchMetric(r.Context(), workspace, actor, input)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, metric)
}

// ListContentSearchRankObservations answers the current revision of each
// matching observation, one by one, newest first. At least one of theme_id,
// publication_record_id and query is required; include_voided=true adds
// voided ones.
func (h *Handler) ListContentSearchRankObservations(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	filter, err := feedbacklearning.ParseRankObservationFilter(query.Get("theme_id"),
		query.Get("publication_record_id"), query.Get("query"), query.Get("include_voided"))
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	observations, err := h.feedbackSearchStore().ListRankObservations(r.Context(), workspace, actor, filter)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"observations": observations})
}

// RecordContentSearchRankObservation records revision 1 of one observation.
func (h *Handler) RecordContentSearchRankObservation(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		h.searchObservationError(w, feedbacklearning.ErrInvalid)
		return
	}
	input, err := feedbacklearning.DecodeRankObservation(body)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	observation, err := h.feedbackSearchStore().RecordRankObservation(r.Context(), workspace, actor, input)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, observation)
}

// ReviseContentSearchRankObservation appends a revision; voided = true voids.
//
// Contract §7.1 puts the path before the body: an observation that is not
// here answers 404 whatever the body holds, so it is read before the body is
// decoded. The store reads it again inside the fenced transaction.
func (h *Handler) ReviseContentSearchRankObservation(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	observationID := chi.URLParam(r, "observationId")
	store := h.feedbackSearchStore()
	if _, err := store.GetRankObservation(r.Context(), workspace, actor, observationID); err != nil {
		h.searchObservationError(w, err)
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		h.searchObservationError(w, feedbacklearning.ErrInvalid)
		return
	}
	revision, err := feedbacklearning.DecodeRankObservationRevision(body)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	observation, err := store.ReviseRankObservation(r.Context(), workspace, actor, observationID, revision)
	if err != nil {
		h.searchObservationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, observation)
}
