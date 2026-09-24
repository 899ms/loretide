package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
)

// Marketing node endpoints (specs/033 PR 1). Contract:
// specs/033-marketing-nodes/contracts/marketing-nodes.md §2.
//
// Mounted under /api/content-marketing-nodes outside the generic member
// middleware, like /api/content-topics: every handler goes through
// topicScope, so each decision - refusals included - reaches
// workspace-core.Authorize and is recorded. The brand comes from
// X-Workspace-ID; the node id comes only from chi.URLParam.

const maxMarketingNodeBody = 1 << 20

func marketingNodeIDFromURL(r *http.Request) string { return chi.URLParam(r, "nodeId") }

// readMarketingNodeBody reads the whole body, bounded. The module decodes it
// strictly (unknown keys and a second JSON value are refused) and keeps
// lead_days' missing / null / number apart, which a decoder here could not.
func readMarketingNodeBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMarketingNodeBody))
	return body, err == nil
}

func (h *Handler) ListContentMarketingNodes(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	status, err := topicplanning.ParseNodeStatusFilter(r.URL.Query().Get("status"))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	nodes, err := h.topicPlanningStore().ListNodes(r.Context(), workspace, actor, status)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"marketing_nodes": nodes})
}

func (h *Handler) CreateContentMarketingNode(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeCreateNode(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	node, err := h.topicPlanningStore().CreateNode(r.Context(), workspace, actor, req)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

// ImportContentMarketingNodes takes rows a page parsed from a pasted list.
// Every row gets an outcome; a bad row does not fail the others.
func (h *Handler) ImportContentMarketingNodes(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	rows, err := topicplanning.DecodeImport(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	result, err := h.topicPlanningStore().ImportNodes(r.Context(), workspace, actor, rows)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetContentMarketingNode(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	node, err := h.topicPlanningStore().GetNode(r.Context(), workspace, actor, marketingNodeIDFromURL(r))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) ListContentMarketingNodeRevisions(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	revisions, err := h.topicPlanningStore().ListNodeRevisions(r.Context(), workspace, actor, marketingNodeIDFromURL(r))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
}

func (h *Handler) ReviseContentMarketingNode(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeReviseNode(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	node, err := h.topicPlanningStore().ReviseNode(r.Context(), workspace, actor, marketingNodeIDFromURL(r), req)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) ConfirmContentMarketingNode(w http.ResponseWriter, r *http.Request) {
	h.transitionContentMarketingNode(w, r, (*topicplanning.Store).ConfirmNode)
}

func (h *Handler) CancelContentMarketingNode(w http.ResponseWriter, r *http.Request) {
	h.transitionContentMarketingNode(w, r, (*topicplanning.Store).CancelNode)
}

type marketingNodeTransition func(*topicplanning.Store, context.Context, string, string, string, topicplanning.TransitionRequest) (topicplanning.MarketingNode, error)

func (h *Handler) transitionContentMarketingNode(w http.ResponseWriter, r *http.Request, transition marketingNodeTransition) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeTransition(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	node, err := transition(h.topicPlanningStore(), r.Context(), workspace, actor, marketingNodeIDFromURL(r), req)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// marketingNodeError adds the one answer topic cards do not have - 409 for a
// base_revision that is no longer current - and otherwise answers exactly as
// the topic endpoints do: 400 naming the field, 404 identical to "no such
// workspace" for anything foreign or missing, 503 for storage.
func (h *Handler) marketingNodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, topicplanning.ErrConflict) {
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": false, "next_action": event.Next,
			"field": "base_revision", "reason": "the node changed since this revision was read",
		})
		return
	}
	h.topicError(w, err)
}

// Candidates, adoption and schedule impact (specs/033 PR 2, contract §2).
// Both path parameters come from chi.URLParam only.

func marketingCandidateIDFromURL(r *http.Request) string { return chi.URLParam(r, "candidateId") }

// marketingSourceStatusReader is the SourceStatusReader adapter: a material's
// status, read by brand and id from content_source. Strings only, so
// topic-planning does not import source-inbox. No row is "not found", the
// same answer for a foreign material as for a missing one.
type marketingSourceStatusReader struct {
	db dbExecutor
}

func (r marketingSourceStatusReader) Status(ctx context.Context, workspaceID, sourceID string) (string, bool, error) {
	if r.db == nil {
		return "", false, topicplanning.ErrStorage
	}
	var status string
	err := r.db.QueryRow(ctx, `SELECT status FROM content_source WHERE workspace_id=$1 AND source_id=$2`,
		workspaceID, sourceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return status, true, nil
}

// marketingNodeStore is the topic planning store with the material status
// port attached, which only candidate reads need.
func (h *Handler) marketingNodeStore() *topicplanning.Store {
	store := h.topicPlanningStore()
	store.SourceStatuses = marketingSourceStatusReader{db: h.DB}
	return store
}

func (h *Handler) ListContentMarketingCandidates(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	candidates, err := h.marketingNodeStore().ListCandidates(r.Context(), workspace, actor, marketingNodeIDFromURL(r))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

func (h *Handler) SyncContentMarketingCandidates(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	candidates, err := h.marketingNodeStore().SyncCandidates(r.Context(), workspace, actor, marketingNodeIDFromURL(r))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

func (h *Handler) EditContentMarketingCandidate(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	patch, err := topicplanning.DecodeCandidatePatch(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	candidate, err := h.marketingNodeStore().EditCandidate(r.Context(), workspace, actor,
		marketingNodeIDFromURL(r), marketingCandidateIDFromURL(r), patch)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, candidate)
}

// AdoptContentMarketingCandidate creates or links a draft topic card. It
// answers 200 with the same body whether the card was made now or by an
// earlier adoption of the same candidate.
func (h *Handler) AdoptContentMarketingCandidate(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeAdopt(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	result, err := h.marketingNodeStore().AdoptCandidate(r.Context(), workspace, actor,
		marketingNodeIDFromURL(r), marketingCandidateIDFromURL(r), req)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListContentMarketingImpact(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	items, err := h.marketingNodeStore().ListImpact(r.Context(), workspace, actor, marketingNodeIDFromURL(r))
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"impact": items})
}

func (h *Handler) DecideContentMarketingImpact(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	body, ok := readMarketingNodeBody(w, r)
	if !ok {
		h.marketingNodeError(w, topicplanning.ErrInvalid)
		return
	}
	req, err := topicplanning.DecodeImpactDecision(body)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	candidate, err := h.marketingNodeStore().DecideImpact(r.Context(), workspace, actor,
		marketingNodeIDFromURL(r), marketingCandidateIDFromURL(r), req)
	if err != nil {
		h.marketingNodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, candidate)
}
