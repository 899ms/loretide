package handler

import (
	"encoding/json"
	"net/http"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// An account's public page link (specs/029, SOP 3.2: "渠道设置保存账号名称或
// 主页链接以便标识").
//
// The name half is content_account.display_name and has been there since
// LT-011. This is the other half, and it lives at settings["loretide.homepage"]
// rather than in a new column - the ruling (Q2=A) chose the key so this card
// ships without a migration.
//
// Its own endpoint rather than a field on PATCH, for the reason LT-014 gives
// in router.go: the account update query assigns the settings blob wholesale,
// so a partial write would delete the account's material scope. Merging
// happens server-side, where no client can forget it.
//
// The link is STORED and never followed. Nothing in this handler or in
// workspace-core fetches it, and a guard test scans for anything that could.

// SetContentAccountHomepage records where an account can be found.
func (h *Handler) SetContentAccountHomepage(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.operatingRulesScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Homepage string `json:"homepage"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil {
		h.operatingRulesError(w, workspacecore.ErrInvalid)
		return
	}
	// accountIDFromURL reads chi's route parameter and nothing else. The
	// account it names is checked against THIS workspace in the UPDATE's WHERE
	// clause, so an id belonging to another brand matches no row and comes
	// back as the same 404 a missing account produces (LT-011/012/013).
	err := h.operatingRulesStore().WriteHomepage(
		r.Context(), workspace, actor, accountIDFromURL(r), body.Homepage)
	if err != nil {
		h.operatingRulesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"homepage": body.Homepage})
}
