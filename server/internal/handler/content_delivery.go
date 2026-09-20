package handler

import (
	"net/http"
	"time"

	reviewdelivery "github.com/multica-ai/multica/server/internal/content/review-delivery"
)

// Delivery tasks and publication records (specs/025, SOP 9.1-9.3).
//
// Nothing here publishes anything. There is no endpoint that reaches a
// platform, no credential to reach one with, and no scheduler behind the
// planned time - SOP 9.2: "系统不保存平台发布密钥，也不提供发布执行接口".

type createDeliveryRequest struct {
	ArtifactID      string `json:"artifact_id"`
	Channel         string `json:"channel"`
	ReviewRequestID string `json:"review_request_id"`
}

func (h *Handler) CreateContentDelivery(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	var body createDeliveryRequest
	if !decodeReviewBody(w, r, &body) {
		h.reviewError(w, reviewdelivery.ErrInvalid)
		return
	}
	created, err := h.reviewDeliveryStore().CreateTask(r.Context(), workspace, actor,
		reviewdelivery.CreateTaskRequest{
			ArtifactID: body.ArtifactID, Channel: reviewdelivery.Channel(body.Channel),
			ReviewRequestID: body.ReviewRequestID,
		})
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// ListContentDeliveries returns the workspace's tasks, each carrying the two
// derived flags: whether its planned time has passed, and whether it was handed
// over with nothing written down yet.
//
// Both are computed here from rows that already exist. Neither is stored, and
// neither asks a platform anything.
func (h *Handler) ListContentDeliveries(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	tasks, err := h.reviewDeliveryStore().ListTasks(r.Context(), workspace, actor,
		r.URL.Query().Get("artifact_id"), r.URL.Query().Get("status"))
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": tasks})
}

type advanceDeliveryRequest struct {
	Status        string  `json:"status"`
	ScheduledAt   *string `json:"scheduled_at"`
	HandoffMethod string  `json:"handoff_method"`
	Reason        string  `json:"reason"`
	// KeepApprovedSnapshot is SOP 9.3's deliberate "deliver the old approved
	// snapshot anyway". It is a separate flag rather than a status value
	// because it is a choice about which snapshot goes out, not a new state.
	KeepApprovedSnapshot bool `json:"keep_approved_snapshot"`
}

// AdvanceContentDelivery moves a task to another status.
//
// Reaching handed_off writes no publication record. SOP 9.1 is explicit that
// exporting, copying and handing over to someone else are not publishing, and a
// test asserts the count stays at zero for all three methods.
func (h *Handler) AdvanceContentDelivery(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	var body advanceDeliveryRequest
	if !decodeReviewBody(w, r, &body) {
		h.reviewError(w, reviewdelivery.ErrInvalid)
		return
	}
	store := h.reviewDeliveryStore()
	if body.KeepApprovedSnapshot {
		task, err := store.KeepDeliveringApprovedSnapshot(r.Context(), workspace, actor,
			deliveryIDFromURL(r), body.Reason)
		if err != nil {
			h.reviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
		return
	}
	scheduledAt, err := parseScheduledAt(body.ScheduledAt)
	if err != nil {
		h.reviewError(w, err)
		return
	}
	task, err := store.Advance(r.Context(), workspace, actor, deliveryIDFromURL(r),
		reviewdelivery.DeliveryAdvance{
			To: reviewdelivery.DeliveryStatus(body.Status), ScheduledAt: scheduledAt,
			HandoffMethod: reviewdelivery.HandoffMethod(body.HandoffMethod),
			Reason:        body.Reason,
		})
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// parseScheduledAt reads the planned time. A malformed one is named rather than
// dropped: someone who typed a time deserves to be told the format was wrong,
// not to find the field empty afterwards.
func parseScheduledAt(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, reviewdelivery.FieldError{Field: "scheduled_at", Reason: "not an RFC 3339 timestamp"}
	}
	utc := parsed.UTC()
	return &utc, nil
}

type recordPublicationRequest struct {
	ArtifactID         string  `json:"artifact_id"`
	DeliveryTaskID     string  `json:"delivery_task_id"`
	Channel            string  `json:"channel"`
	Status             string  `json:"status"`
	DeclaredBy         string  `json:"declared_by"`
	PageURLOrContentID string  `json:"page_url_or_content_id"`
	ReceiptNote        string  `json:"receipt_note"`
	VerificationNote   string  `json:"verification_note"`
	PublishedAt        *string `json:"published_at"`
	PlatformAccount    string  `json:"platform_account"`
	PlatformEdited     bool    `json:"platform_edited"`
	EditNote           string  `json:"edit_note"`
	VersionMatch       string  `json:"version_match"`
}

// RecordContentPublication writes down one person's statement about what
// happened on a platform.
//
// There is no actor field in the request. Who typed it comes from the session,
// because it is the one field on this row that must not be forgeable; who SAID
// it is declared_by, free text, because SOP 9.2 enumerates neither.
func (h *Handler) RecordContentPublication(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	var body recordPublicationRequest
	if !decodeReviewBody(w, r, &body) {
		h.reviewError(w, reviewdelivery.ErrInvalid)
		return
	}
	record, err := h.reviewDeliveryStore().Record(r.Context(), workspace, actor,
		reviewdelivery.RecordRequest{
			ArtifactID: body.ArtifactID, DeliveryTaskID: body.DeliveryTaskID,
			Channel:    reviewdelivery.Channel(body.Channel),
			Status:     reviewdelivery.PublicationStatus(body.Status),
			DeclaredBy: body.DeclaredBy, PageURLOrContentID: body.PageURLOrContentID,
			ReceiptNote: body.ReceiptNote, VerificationNote: body.VerificationNote,
			PublishedAt: body.PublishedAt, PlatformAccount: body.PlatformAccount,
			PlatformEdited: body.PlatformEdited, EditNote: body.EditNote,
			VersionMatch: reviewdelivery.VersionMatch(body.VersionMatch),
		})
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

// ListContentPublications returns a document's records, newest first. The first
// element is the current status - there is no column that holds it.
func (h *Handler) ListContentPublications(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.reviewScope(w, r)
	if !ok {
		return
	}
	records, err := h.reviewDeliveryStore().ListPublications(r.Context(), workspace, actor,
		r.URL.Query().Get("artifact_id"))
	if err != nil {
		h.reviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"publications": records})
}
