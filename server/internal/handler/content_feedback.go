package handler

import (
	"net/http"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
)

// Feedback excerpts, the pending-registration list and the AI review
// placeholder (specs/027, SOP 10.1, 7.1).

// RecordContentFeedback writes one excerpt.
//
// Nothing here redacts anything. SOP 10.1 supports a person redacting a quote;
// it does not redact for them, and an automatic redactor that misses one name
// is worse than none because it teaches people to stop checking.
func (h *Handler) RecordContentFeedback(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.ExcerptInput
	if !decodeFeedbackBody(w, r, &body) {
		h.feedbackError(w, feedbacklearning.ErrInvalid)
		return
	}
	written, err := h.feedbackStore().Excerpt(r.Context(), workspace, actor, body)
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, written)
}

// ListContentFeedback returns excerpts, newest occurrence first.
//
// The quote and the operator's reading arrive as two fields and stay that way:
// a later reader has to be able to tell the evidence from the judgement.
func (h *Handler) ListContentFeedback(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	excerpts, err := h.feedbackStore().ListExcerpts(r.Context(), workspace, actor,
		r.URL.Query().Get("publication_record_id"))
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"excerpts": excerpts,
		// SOP 7.1's AI review row. This phase produces exactly one of its
		// seven values, and says so rather than leaving the caller to guess
		// whether the field is missing or the work is pending.
		"review_state": feedbacklearning.StatePendingData,
	})
}

// ListContentFeedbackPending is SOP 2's fifth workbench item: published, and
// nobody has recorded any numbers yet.
//
// There is no time comparison in it. SOP 3.2's brand-level 反馈观察时点 does
// not exist yet, so "is it due" has no answer; a hard-coded number of days
// would be a rule the SOP never stated, sitting where no operator can see or
// change it.
func (h *Handler) ListContentFeedbackPending(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	pending, err := h.feedbackStore().PendingRegistrations(r.Context(), workspace, actor)
	if err != nil {
		h.feedbackError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pending": pending, "count": len(pending),
	})
}
