package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
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
