package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	"github.com/multica-ai/multica/server/internal/util"
)

// Starting a brief revision and reading back what a start fixed (EP-04b).
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

func briefRevisionIDFromURL(r *http.Request) string { return chi.URLParam(r, "revisionId") }
func snapshotIDFromURL(r *http.Request) string      { return chi.URLParam(r, "snapshotId") }

// StartContentBrief records one start of one brief revision.
//
// Not idempotent, deliberately: starting the same revision again with a
// different material scope is an ordinary thing to want, so it produces a
// second snapshot. There is no "already started" branch here, and adding one
// would take the behaviour back to a one-start-per-revision model nobody chose.
func (h *Handler) StartContentBrief(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	var request topicplanning.StartRequest
	if !decodeTopicBody(w, r, &request) {
		h.topicError(w, topicplanning.ErrInvalid)
		return
	}
	// The brand switch is read here, not sent by the caller: a caller that
	// could set it would be writing its own history. It is recorded and never
	// acted on - triggering a precheck is EP-06.
	request.AutoPrecheck = h.workspaceAutoPrecheck(r, workspace)

	snapshot, err := h.topicPlanningStore().Start(r.Context(), workspace, actor,
		topicCardIDFromURL(r), briefRevisionIDFromURL(r), request)
	if err != nil {
		h.startError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

// ListContentBriefStarts returns every start of one brief revision.
func (h *Handler) ListContentBriefStarts(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	snapshots, err := h.topicPlanningStore().ListSnapshots(r.Context(), workspace, actor,
		topicCardIDFromURL(r), briefRevisionIDFromURL(r))
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"start_snapshots": snapshots})
}

// GetContentStartSnapshot returns one start by its stable key.
func (h *Handler) GetContentStartSnapshot(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.topicScope(w, r)
	if !ok {
		return
	}
	snapshot, err := h.topicPlanningStore().GetSnapshot(r.Context(), workspace, actor,
		topicCardIDFromURL(r), snapshotIDFromURL(r))
	if err != nil {
		h.topicError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// startError adds one case to the topic mapping: a readiness gap names the
// fields that are still missing.
//
// Deliberately NOT the 404 that every other refusal collapses into. Those hide
// whether somebody else's resource exists; this gap is in the caller's OWN
// account, so naming it reveals nothing and is the only way the page can say
// what to go and fix.
func (h *Handler) startError(w http.ResponseWriter, err error) {
	var readiness topicplanning.ReadinessError
	if errors.As(err, &readiness) {
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "topic-planning",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": event.Message, "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": event.Retryable,
			"next_action": event.Next,
			// The only field this response adds. Field names, not a sentence:
			// the page points at the fields rather than translating prose.
			"missing": readiness.Missing,
		})
		return
	}
	h.topicError(w, err)
}

// workspaceAutoPrecheck reads the brand switch, falling back to the same
// default a workspace response fills in.
//
// A read failure is not a refusal: the switch is recorded, not enforced, and
// failing a start because a settings read hiccuped would stop work for a value
// nothing acts on yet.
func (h *Handler) workspaceAutoPrecheck(r *http.Request, workspaceID string) bool {
	if h.Queries == nil || workspaceID == "" {
		return defaultWorkspaceAutoPrecheck
	}
	id, err := util.ParseUUID(workspaceID)
	if err != nil {
		return defaultWorkspaceAutoPrecheck
	}
	workspace, err := h.Queries.GetWorkspace(r.Context(), id)
	if err != nil || len(workspace.Settings) == 0 {
		return defaultWorkspaceAutoPrecheck
	}
	var settings map[string]any
	if err = json.Unmarshal(workspace.Settings, &settings); err != nil {
		return defaultWorkspaceAutoPrecheck
	}
	if value, isBool := settings[workspaceAutoPrecheckKey].(bool); isBool {
		return value
	}
	return defaultWorkspaceAutoPrecheck
}
