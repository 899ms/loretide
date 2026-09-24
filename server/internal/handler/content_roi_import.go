package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	publicapiv1 "github.com/multica-ai/multica/server/pkg/publicapi/v1"
)

// Importing pasted costs, leads and deals (specs/034 PR 3, FR-026 to FR-029).
//
// This file is the adapter. It reads the Idempotency-Key header and decodes
// the rows; the claim on the key, the duplicate check, the writes and the
// batch record are all feedback-learning's, in one transaction there. The
// header name is the public API's constant - an adapter may name it, the
// module may not import the idempotency module that also uses it.
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.8, §1.9, §6

// roiImportBody is POST /imports. rows are decoded one by one by record kind,
// so a refusal can say which row.
type roiImportBody struct {
	RecordKind string            `json:"record_kind"`
	DryRun     bool              `json:"dry_run"`
	Rows       []json.RawMessage `json:"rows"`
}

// A row may echo back the fields the server writes; they are dropped, as on
// the single-record endpoints.
type roiImportCostRow struct {
	feedbacklearning.CostInput
	roiServerWritten
}

type roiImportLeadRow struct {
	feedbacklearning.LeadInput
	roiServerWritten
}

type roiImportDealRow struct {
	feedbacklearning.DealInput
	roiServerWritten
}

// decodeROIImportRow decodes one row strictly. A wrong JSON type names the
// JSON field and the row; anything else unreadable names the row.
func decodeROIImportRow(raw json.RawMessage, row int, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			if field := jsonFieldPath(typeErr.Field); field != "" {
				return feedbacklearning.FieldError{Field: field, Reason: "wrong JSON type", Row: row}
			}
		}
		return feedbacklearning.FieldError{Field: "rows", Reason: "not a readable row", Row: row}
	}
	return nil
}

// roiImportInput turns the body into the module's input. An unknown record
// kind is left for the module to refuse, naming record_kind.
func roiImportInput(body roiImportBody) (feedbacklearning.ImportInput, error) {
	in := feedbacklearning.ImportInput{RecordKind: body.RecordKind}
	for index, raw := range body.Rows {
		row := index + 1
		switch feedbacklearning.ImportRecordKind(body.RecordKind) {
		case feedbacklearning.ImportCosts:
			var decoded roiImportCostRow
			if err := decodeROIImportRow(raw, row, &decoded); err != nil {
				return in, err
			}
			in.Costs = append(in.Costs, decoded.CostInput)
		case feedbacklearning.ImportLeads:
			var decoded roiImportLeadRow
			if err := decodeROIImportRow(raw, row, &decoded); err != nil {
				return in, err
			}
			in.Leads = append(in.Leads, decoded.LeadInput)
		case feedbacklearning.ImportDeals:
			var decoded roiImportDealRow
			if err := decodeROIImportRow(raw, row, &decoded); err != nil {
				return in, err
			}
			in.Deals = append(in.Deals, decoded.DealInput)
		default:
			return in, nil
		}
	}
	return in, nil
}

// ImportContentROI is POST /api/content-roi/imports. A real import answers
// 201 with the batch - the first time and on every replay of the same key and
// input, byte for byte; a dry run answers 200 and writes nothing.
func (h *Handler) ImportContentROI(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body roiImportBody
	if err := decodeROIBody(w, r, &body); err != nil {
		h.roiError(w, err)
		return
	}
	in, err := roiImportInput(body)
	if err != nil {
		h.roiError(w, err)
		return
	}
	key := strings.TrimSpace(r.Header.Get(publicapiv1.HeaderIdempotencyKey))
	result, err := h.roiStore().Import(r.Context(), workspace, actor, in, key, body.DryRun)
	if err != nil {
		h.roiError(w, err)
		return
	}
	status := http.StatusCreated
	if body.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

// ListContentROIImports is GET /api/content-roi/imports, newest first.
func (h *Handler) ListContentROIImports(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	imports, err := h.roiStore().ListImports(r.Context(), workspace, actor)
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imports": imports})
}

// GetContentROIImport is GET /api/content-roi/imports/{batchId}: one batch
// with its per-row outcomes. Another workspace's batch answers as missing.
func (h *Handler) GetContentROIImport(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	batch, err := h.roiStore().GetImport(r.Context(), workspace, actor, chi.URLParam(r, "batchId"))
	if err != nil {
		h.roiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}
