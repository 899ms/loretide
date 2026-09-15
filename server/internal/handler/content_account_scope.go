package handler

import (
	"encoding/json"
	"net/http"

	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// The account's material scope preference (LT-014).
//
// Its own endpoint rather than a field on PATCH, because the update query
// replaces the settings blob wholesale: a partial write would delete the
// account's other settings. Merging happens in the service, on this side of the
// network, so no client can forget it.

// SetAccountScope records which sources this account starts a run from.
func (h *Handler) SetAccountScope(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Scope string `json:"scope"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body) != nil {
		h.accountInputError(w, ipprofile.ErrScope)
		return
	}
	account, err := h.contentAccountService().SetScope(
		r.Context(), workspace, actor, accountIDFromURL(r), body.Scope)
	if err != nil {
		h.scopeWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

// scopeWriteError keeps "that is not a scope" (400) apart from "you cannot have
// this account" (404, indistinguishable from not existing).
func (h *Handler) scopeWriteError(w http.ResponseWriter, err error) {
	if err == ipprofile.ErrScope {
		h.accountInputError(w, err)
		return
	}
	writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
		workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
}
