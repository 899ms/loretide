package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
)

// Costs, leads, touches, deals and refunds/adjustments (specs/034 PR 1,
// BO-06 / R-061).
//
// This file is the adapter. feedback-learning stores account, work and
// publication ids as plain strings and imports none of their modules; whether
// each one exists here is answered below, by asking the module that owns it.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §6

// roiAccounts answers "does this content account exist in this workspace"
// through ip-profile's public read.
type roiAccounts struct{ service *ipprofile.Service }

func (a roiAccounts) AccountExists(ctx context.Context, workspaceID, accountID string) error {
	if a.service == nil {
		return feedbacklearning.ErrStorage
	}
	_, err := a.service.Get(ctx, workspaceID, accountID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ipprofile.ErrNotFound):
		return feedbacklearning.ErrNotFound
	default:
		return feedbacklearning.ErrStorage
	}
}

// roiWorks answers "does this work exist in this workspace" through
// work-editor's public read.
type roiWorks struct{ store *workeditor.Store }

func (w roiWorks) WorkExists(ctx context.Context, workspaceID, actor, workID string) error {
	if w.store == nil {
		return feedbacklearning.ErrStorage
	}
	_, err := w.store.GetWork(ctx, workspaceID, actor, workID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workeditor.ErrNotFound), errors.Is(err, workeditor.ErrInvalid):
		return feedbacklearning.ErrNotFound
	default:
		return feedbacklearning.ErrStorage
	}
}

// roiTimezones reads the brand's timezone the way the workspace response
// reports it: the stored loretide.timezone when it is usable, otherwise the
// default. A read that fails answers the default too; the timezone only
// decides which calendar day a dedupe key uses.
type roiTimezones struct{ db dbExecutor }

func (z roiTimezones) Location(ctx context.Context, workspaceID string) *time.Location {
	var settings map[string]any
	if z.db != nil {
		var raw []byte
		if err := z.db.QueryRow(ctx, `SELECT settings FROM workspace WHERE id::text = $1`,
			workspaceID).Scan(&raw); err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &settings)
		}
	}
	filled, _ := timezoneFilled(settings).(map[string]any)
	name, _ := filled[workspaceTimezoneKey].(string)
	if location, err := time.LoadLocation(name); err == nil && name != "" {
		return location
	}
	return nil
}

func (h *Handler) roiStore() *feedbacklearning.ROIStore {
	return &feedbacklearning.ROIStore{
		Store:     h.feedbackStore(),
		Accounts:  roiAccounts{service: h.contentAccountService()},
		Works:     roiWorks{store: h.workEditorStore()},
		Timezones: roiTimezones{db: h.DB},
	}
}

// roiServerWritten names the fields the server writes and a body may not set.
// They are accepted and dropped rather than refused: a client that echoes a
// record back must not fail on them, and nothing it sends there is used.
type roiServerWritten struct {
	RecordedBy json.RawMessage `json:"recorded_by"`
	SourceType json.RawMessage `json:"source_type"`
}

type roiRevisionFields struct {
	BaseRevision *int `json:"base_revision"`
	Voided       bool `json:"voided"`
}

func (f roiRevisionFields) revision() feedbacklearning.Revision {
	return feedbacklearning.Revision{BaseRevision: f.BaseRevision, Voided: f.Voided}
}

// roiCostBody carries the split in CostInput.Allocations (specs/034 PR 2):
// absent or null keeps the previous revision's split, [] ends it.
type roiCostBody struct {
	feedbacklearning.CostInput
	roiRevisionFields
	roiServerWritten
}

type roiLeadBody struct {
	feedbacklearning.LeadInput
	roiRevisionFields
	roiServerWritten
}

type roiMergeBody struct {
	feedbacklearning.MergeInput
	BaseRevision *int `json:"base_revision"`
}

type roiTouchBody struct {
	feedbacklearning.TouchInput
	roiRevisionFields
	roiServerWritten
}

type roiDealBody struct {
	feedbacklearning.DealInput
	roiRevisionFields
	roiServerWritten
}

type roiAdjustmentBody struct {
	feedbacklearning.AdjustmentInput
	roiRevisionFields
	roiServerWritten
}

// roiAttributionBody is a judgement revision. base_revision is 0 for the
// first judgement on a deal.
type roiAttributionBody struct {
	feedbacklearning.AttributionInput
	roiRevisionFields
	roiServerWritten
}

// decodeROIBody decodes strictly. A value of the wrong JSON type is refused
// naming its field - an amount sent as a JSON number is the case that
// matters: it may already have been through a float before it got here.
func decodeROIBody(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			if field := jsonFieldPath(typeErr.Field); field != "" {
				return feedbacklearning.FieldError{Field: field, Reason: "wrong JSON type"}
			}
		}
		return feedbacklearning.ErrInvalid
	}
	return nil
}

// jsonFieldPath turns UnmarshalTypeError.Field into the JSON path a caller
// sent. encoding/json writes an embedded struct into that path by its Go type
// name ("CostInput.amount"), and the bodies here are built from embedded
// module inputs and unexported helper structs ("roiRevisionFields"); a Go
// type name is not part of the contract and must not reach a response. Every
// JSON name in these bodies is snake_case, so a segment holding any
// upper-case letter is a Go name and is dropped.
func jsonFieldPath(field string) string {
	kept := []string{}
	for _, segment := range strings.Split(field, ".") {
		if segment == "" || strings.IndexFunc(segment, unicode.IsUpper) >= 0 {
			continue
		}
		kept = append(kept, segment)
	}
	return strings.Join(kept, ".")
}

// roiError maps the module's errors. A refusal and a missing row answer
// byte for byte alike apart from the trace id.
func (h *Handler) roiError(w http.ResponseWriter, err error) {
	if conflict, ok := errors.AsType[feedbacklearning.RevisionConflict](err); ok {
		h.feedbackDiagnosticError(w, http.StatusConflict, map[string]any{
			"field": conflict.Field, "reason": "stale revision",
		})
		return
	}
	if _, ok := errors.AsType[feedbacklearning.IdempotencyConflict](err); ok {
		h.feedbackDiagnosticError(w, http.StatusConflict, map[string]any{
			"field": feedbacklearning.IdempotencyKeyField, "reason": "used with a different request",
		})
		return
	}
	if duplicate, ok := errors.AsType[feedbacklearning.PossibleDuplicate](err); ok {
		h.feedbackDiagnosticError(w, http.StatusConflict, map[string]any{
			"code": "possible_duplicate", "matches": duplicate.Matches,
		})
		return
	}
	h.feedbackError(w, err)
}

func roiListFilter(r *http.Request) (feedbacklearning.ROIListFilter, error) {
	query := r.URL.Query()
	filter := feedbacklearning.ROIListFilter{
		AccountID:       query.Get("account_id"),
		WorkID:          query.Get("work_id"),
		LeadID:          query.Get("lead_id"),
		IncludeInactive: query.Get("include_inactive") == "true",
	}
	for _, bound := range []struct {
		name   string
		target **time.Time
	}{{"from", &filter.From}, {"to", &filter.To}} {
		text := query.Get(bound.name)
		if text == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return filter, feedbacklearning.FieldError{Field: bound.name, Reason: "not an RFC 3339 timestamp"}
		}
		*bound.target = &parsed
	}
	return filter, nil
}

// ---------------------------------------------------------------- costs

func (h *Handler) ListContentROICosts(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	filter, err := roiListFilter(r)
	if err != nil {
		h.roiError(w, err)
		return
	}
	costs, err := h.roiStore().ListCosts(r.Context(), workspace, actor, filter)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"costs": costs})
}

// CreateContentROICost writes revision 1. The source type is the endpoint's,
// never the body's.
func (h *Handler) CreateContentROICost(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiCostBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	cost, err := h.roiStore().CreateCost(r.Context(), workspace, actor, body.CostInput, feedbacklearning.RecordManual)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cost)
}

func (h *Handler) GetContentROICost(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	cost, err := h.roiStore().GetCost(r.Context(), workspace, actor, chi.URLParam(r, "costId"))
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cost)
}

// ReviseContentROICost writes the next revision; voiding is a revision too.
func (h *Handler) ReviseContentROICost(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiCostBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	cost, err := h.roiStore().ReviseCost(r.Context(), workspace, actor, chi.URLParam(r, "costId"),
		body.CostInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cost)
}

// ---------------------------------------------------------------- leads

func (h *Handler) ListContentROILeads(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	filter, err := roiListFilter(r)
	if err != nil {
		h.roiError(w, err)
		return
	}
	leads, err := h.roiStore().ListLeads(r.Context(), workspace, actor, filter)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"leads": leads})
}

func (h *Handler) CreateContentROILead(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiLeadBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	lead, err := h.roiStore().CreateLead(r.Context(), workspace, actor, body.LeadInput, feedbacklearning.RecordManual)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lead)
}

func (h *Handler) GetContentROILead(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	lead, err := h.roiStore().GetLead(r.Context(), workspace, actor, chi.URLParam(r, "leadId"))
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lead)
}

func (h *Handler) ReviseContentROILead(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiLeadBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	lead, err := h.roiStore().ReviseLead(r.Context(), workspace, actor, chi.URLParam(r, "leadId"),
		body.LeadInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lead)
}

// MergeContentROILead merges the lead in the path into target_lead_id, or
// unmerges it when the target is empty. It writes a revision of the path's
// lead and nothing else.
func (h *Handler) MergeContentROILead(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiMergeBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	lead, err := h.roiStore().MergeLead(r.Context(), workspace, actor, chi.URLParam(r, "leadId"),
		body.MergeInput, feedbacklearning.Revision{BaseRevision: body.BaseRevision})
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lead)
}

func (h *Handler) AddContentROITouch(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiTouchBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	touch, err := h.roiStore().AddTouch(r.Context(), workspace, actor, chi.URLParam(r, "leadId"), body.TouchInput)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, touch)
}

func (h *Handler) ReviseContentROITouch(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiTouchBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	touch, err := h.roiStore().ReviseTouch(r.Context(), workspace, actor, chi.URLParam(r, "leadId"),
		chi.URLParam(r, "touchId"), body.TouchInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, touch)
}

// ---------------------------------------------------------------- deals

func (h *Handler) ListContentROIDeals(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	filter, err := roiListFilter(r)
	if err != nil {
		h.roiError(w, err)
		return
	}
	deals, err := h.roiStore().ListDeals(r.Context(), workspace, actor, filter)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deals": deals})
}

func (h *Handler) CreateContentROIDeal(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiDealBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	deal, err := h.roiStore().CreateDeal(r.Context(), workspace, actor, body.DealInput, feedbacklearning.RecordManual)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deal)
}

func (h *Handler) GetContentROIDeal(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	deal, err := h.roiStore().GetDeal(r.Context(), workspace, actor, chi.URLParam(r, "dealId"))
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deal)
}

func (h *Handler) ReviseContentROIDeal(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiDealBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	deal, err := h.roiStore().ReviseDeal(r.Context(), workspace, actor, chi.URLParam(r, "dealId"),
		body.DealInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deal)
}

func (h *Handler) AddContentROIAdjustment(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiAdjustmentBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	adjustment, err := h.roiStore().AddAdjustment(r.Context(), workspace, actor, chi.URLParam(r, "dealId"),
		body.AdjustmentInput)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adjustment)
}

func (h *Handler) ReviseContentROIAdjustment(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiAdjustmentBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	adjustment, err := h.roiStore().ReviseAdjustment(r.Context(), workspace, actor, chi.URLParam(r, "dealId"),
		chi.URLParam(r, "adjustmentId"), body.AdjustmentInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adjustment)
}

// RecordContentROIAttribution writes the next revision of the operator's
// judgement on the deal in the path (specs/034 PR 2, FR-020). It writes that
// one row and nothing else; the touches it names are read, not revised.
func (h *Handler) RecordContentROIAttribution(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiAttributionBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	attribution, err := h.roiStore().RecordAttribution(r.Context(), workspace, actor, chi.URLParam(r, "dealId"),
		body.AttributionInput, body.revision())
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, attribution)
}
