package handler

import (
	"net/http"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
)

// Brand/account operating diagnosis (specs/035 PR 2): POST
// /api/content-operating-diagnosis/preview computes a diagnosis from the
// given params and the inputs as they are now, and stores nothing.
//
// It is an adapter like content_opdiag_reports.go and uses the same store
// wiring (h.opdiagStore): every other module is read through that file's
// adapters, and the ROI summary a params may name is feedback-learning's own
// read, so this file imports no other content module. Authorize comes first
// (feedbackScope); a refusal builds nothing and reads nothing. There is no
// write, so there is no fence - a workspace deleted meanwhile answers like a
// missing one because its operating rules read finds nothing
// (opdiagReadError).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §7

// opdiagPreviewBody is the params only: nothing is stored, so there is no
// title. The timezone and generation time are the server's to write.
type opdiagPreviewBody struct {
	Params *feedbacklearning.DiagnosisParams `json:"params"`
}

// PreviewContentOpDiagReport answers contract §5.2's result for the params,
// without saving it.
func (h *Handler) PreviewContentOpDiagReport(w http.ResponseWriter, r *http.Request) {
	h.previewOpdiag(w, r, h.opdiagStore)
}

func (h *Handler) previewOpdiag(w http.ResponseWriter, r *http.Request, store func() *feedbacklearning.DiagnosisStore) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagPreviewBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	result, err := store().PreviewDiagnosis(r.Context(), workspace, actor, body.Params, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
