package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// ProfileReadResponse is the current stored profile plus the decisions derived
// from it. Readiness and neutral expression are computed on every read; neither
// is stored as a second source of truth.
type ProfileReadResponse struct {
	RevisionID            string                      `json:"revision_id,omitempty"`
	Revision              int64                       `json:"revision,omitempty"`
	Profile               ipprofile.ExpressionProfile `json:"profile"`
	Readiness             ipprofile.Readiness         `json:"readiness"`
	UsesNeutralExpression bool                        `json:"uses_neutral_expression"`
}

// SetAccountExpressionProfile confirms the complete profile by appending one
// account revision. It never updates an existing snapshot.
func (h *Handler) SetAccountExpressionProfile(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	accountID := accountIDFromURL(r)
	service := h.contentAccountService()
	if _, err := service.Get(r.Context(), workspace, accountID); err != nil {
		logAccountFailure(r, "set expression profile: load account", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}

	profile, err := decodeExpressionProfile(w, r)
	if err != nil {
		h.accountInputError(w, err)
		return
	}
	revision, err := service.SetProfile(r.Context(), workspace, actor, accountID, profile)
	if err != nil {
		logAccountFailure(r, "set expression profile", err)
		h.profileWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, revision)
}

// GetAccountExpressionProfile reads the latest profile and derives the minimum
// start condition and neutral-expression marker from that same snapshot. A
// real account with no revisions reads as an empty, all-pending profile.
func (h *Handler) GetAccountExpressionProfile(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	accountID := accountIDFromURL(r)
	service := h.contentAccountService()
	if _, err := service.Get(r.Context(), workspace, accountID); err != nil {
		logAccountFailure(r, "get expression profile: load account", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}

	revision, err := service.CurrentPersonaRevision(r.Context(), workspace, accountID)
	if errors.Is(err, ipprofile.ErrNotFound) {
		revision = ipprofile.Revision{Profile: ipprofile.NormalizeProfile(ipprofile.ExpressionProfile{})}
	} else if err != nil {
		logAccountFailure(r, "get expression profile", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}

	writeJSON(w, http.StatusOK, ProfileReadResponse{
		RevisionID:            revision.RevisionID,
		Revision:              revision.Revision,
		Profile:               revision.Profile,
		Readiness:             ipprofile.ProfileReadiness(revision.Profile),
		UsesNeutralExpression: ipprofile.UsesNeutralExpression(revision.Profile),
	})
}

func decodeExpressionProfile(w http.ResponseWriter, r *http.Request) (ipprofile.ExpressionProfile, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.DisallowUnknownFields()
	var profile *ipprofile.ExpressionProfile
	if err := decoder.Decode(&profile); err != nil {
		return ipprofile.ExpressionProfile{}, ipprofile.ErrProfile
	}
	if profile == nil {
		return ipprofile.ExpressionProfile{}, ipprofile.ErrProfile
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ipprofile.ExpressionProfile{}, ipprofile.ErrProfile
	}
	return *profile, nil
}

func (h *Handler) profileWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ipprofile.ErrProfile):
		h.accountInputError(w, err)
	case errors.Is(err, ipprofile.ErrRevisionConflict):
		event := diagnostics.Sanitize(diagnostics.Event{
			Code: "INPUT_CONFLICT", Component: "ip-profile",
			Trace: w.Header().Get("X-Diagnostic-Trace"),
		})
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": err.Error(), "code": event.Code, "trace_id": event.Trace,
			"component": event.Component, "retryable": true, "next_action": event.Next,
		})
	case errors.Is(err, ipprofile.ErrWorkspaceGone):
		// The workspace was deleted while this write was in flight. Answered as
		// 404 and byte-identical to "no such account": a caller must not learn
		// from a refusal that a workspace used to exist. Spelled out rather
		// than left to the default so it stays a decision.
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	}
}
