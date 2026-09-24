package handler

import (
	"net/http"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
)

// POST /api/content-roi/preview (specs/034 PR 2, contract §6): compute a
// report once from the given parameters and return it. Nothing is saved - no
// record, no report, no audit event; saving a report version is PR 4.
//
// This file is the adapter. The window, rates, method, stages and scope come
// from the body; the brand's timezone, the generation time and who entered
// each rate are the server's to write, and a body that sends them is ignored
// on those fields, as with recorded_by on a record.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §5, §6

// roiPreviewBody is contract §5.1's params.
type roiPreviewBody struct {
	feedbacklearning.ReportParams
}

// PreviewContentROI answers contract §5.2's result for the brand in the
// header.
func (h *Handler) PreviewContentROI(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiPreviewBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	result, err := h.roiStore().PreviewReport(r.Context(), workspace, actor, body.ReportParams, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
