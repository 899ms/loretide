package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
)

// ROI review report versions (specs/034 PR 4, contract §6): generate a
// report, generate its next version, list reports and versions, read one
// version.
//
// This file is the adapter. Generating a version is the only write, and it
// writes one new row: nothing here changes or removes a version. The report
// and version in the path are read with chi.URLParam only; a {versionNo}
// that is not a positive integer is answered exactly like a version that
// does not exist.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.10, §6, §7

// roiReportBody is ReportRequest: an optional title and the report
// parameters (contract §5.1). The timezone, generation time and who entered
// each rate are the server's to write, as on /preview.
type roiReportBody struct {
	feedbacklearning.ReportRequest
}

// CreateContentROIReport generates version 1 of a new report.
func (h *Handler) CreateContentROIReport(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiReportBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	version, err := h.roiStore().CreateReport(r.Context(), workspace, actor, body.ReportRequest, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

// ListContentROIReports answers the latest version of each report.
func (h *Handler) ListContentROIReports(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reports, err := h.roiStore().ListReports(r.Context(), workspace, actor)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

// ListContentROIReportVersions answers every version of one report.
func (h *Handler) ListContentROIReportVersions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reportID := chi.URLParam(r, "reportId")
	versions, err := h.roiStore().ListReportVersions(r.Context(), workspace, actor, reportID)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report_id": reportID, "versions": versions})
}

// GetContentROIReportVersion answers one version as stored, with
// inputs_changed derived on read.
func (h *Handler) GetContentROIReportVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	versionNo, valid := feedbacklearning.ParseVersionNo(chi.URLParam(r, "versionNo"))
	if !valid {
		h.roiError(w, feedbacklearning.ErrNotFound)
		return
	}
	version, err := h.roiStore().GetReportVersion(r.Context(), workspace, actor, chi.URLParam(r, "reportId"), versionNo)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

// GenerateContentROIReportVersion generates the next version of a report
// from the records as they are now; absent title and params reuse the
// previous version's.
func (h *Handler) GenerateContentROIReportVersion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiReportBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	version, err := h.roiStore().GenerateReportVersion(r.Context(), workspace, actor,
		chi.URLParam(r, "reportId"), body.ReportRequest, time.Now())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}
