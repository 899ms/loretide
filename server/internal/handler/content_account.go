package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Brand content accounts. Every handler here decides with content/workspace-core
// first and answers refusals through its canonical mapping, so "not a member",
// "wrong brand" and "no such account" are one response - a caller cannot use a
// refusal to find out which account ids are real.

// accountScope authorizes the request and returns the workspace it may act in.
func (h *Handler) accountScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	workspace := h.resolveWorkspaceID(r)
	actor := r.Header.Get("X-User-ID")
	decision := workspacecore.Authorize(r.Context(), h.diagnosticMembership(),
		h.diagnosticRefusalRecorder(), actor, workspace, "owner", "admin", "member")
	if !decision.Allowed {
		h.writeAccountRefusal(w, r, decision.Reason)
		return "", "", false
	}
	return workspace, actor, true
}

func (h *Handler) writeAccountRefusal(w http.ResponseWriter, r *http.Request, reason workspacecore.Reason) {
	writeJSON(w, workspacecore.RefusalStatus(reason),
		workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	_ = r
}

// accountInputError answers a malformed input: an unsupported platform or a
// blank display name. 400 with a diagnostic error object, never a database
// constraint error leaking through.
func (h *Handler) accountInputError(w http.ResponseWriter, err error) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "ip-profile",
		Trace: w.Header().Get("X-Diagnostic-Trace"),
	})
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": err.Error(), "code": event.Code, "trace_id": event.Trace,
		"component": event.Component, "retryable": event.Retryable,
		"next_action": event.Next,
	})
}

func (h *Handler) contentAccountService() *ipprofile.Service {
	return &ipprofile.Service{
		Store: contentAccountStore{q: h.Queries},
		Audit: contentAccountAuditor{h: h},
		Build: "loretide",
	}
}

func (h *Handler) CreateContentAccount(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Platform    string         `json:"platform"`
		DisplayName string         `json:"display_name"`
		Settings    map[string]any `json:"settings"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body) != nil {
		h.accountInputError(w, ipprofile.ErrPlatform)
		return
	}
	account, err := h.contentAccountService().Create(r.Context(), workspace, actor,
		body.Platform, body.DisplayName, body.Settings)
	if err != nil {
		h.accountWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (h *Handler) GetContentAccount(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	account, err := h.contentAccountService().Get(r.Context(), workspace, accountIDFromURL(r))
	if err != nil {
		// Not found and belongs-to-another-brand are the same answer by design.
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *Handler) ListContentAccounts(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	accounts, err := h.contentAccountService().List(r.Context(), workspace)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "accounts unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (h *Handler) UpdateContentAccount(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Platform    *string        `json:"platform"`
		DisplayName *string        `json:"display_name"`
		Settings    map[string]any `json:"settings"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body) != nil {
		h.accountInputError(w, ipprofile.ErrPlatform)
		return
	}
	account, err := h.contentAccountService().Update(r.Context(), workspace, actor,
		accountIDFromURL(r), ipprofile.Patch{
			Platform: body.Platform, DisplayName: body.DisplayName, Settings: body.Settings,
		})
	if err != nil {
		h.accountWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

// accountWriteError keeps input problems (400) apart from anything that means
// "you cannot have this" (404, indistinguishable from not existing).
func (h *Handler) accountWriteError(w http.ResponseWriter, err error) {
	switch err {
	case ipprofile.ErrPlatform, ipprofile.ErrDisplayName:
		h.accountInputError(w, err)
	default:
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	}
}

func accountIDFromURL(r *http.Request) string { return workspaceIDFromURL(r, "id") }

// contentAccountStore adapts the generated queries to the module's interface.
// The module cannot import the generated package - the content boundary checker
// approves only pgx, uuid, websocket and otel upstream - so the adaptation lives
// here, on this side of the boundary.
type contentAccountStore struct{ q *db.Queries }

func accountFromRow(account_id, workspace_id, platform, display_name string, settings []byte, created, updated string) ipprofile.Account {
	parsed := map[string]any{}
	if len(settings) > 0 {
		_ = json.Unmarshal(settings, &parsed)
	}
	return ipprofile.Account{
		AccountID: account_id, WorkspaceID: workspace_id, Platform: platform,
		DisplayName: display_name, Settings: parsed, CreatedAt: created, UpdatedAt: updated,
	}
}

func (s contentAccountStore) CreateAccount(ctx context.Context, a ipprofile.Account) (ipprofile.Account, error) {
	settings, err := json.Marshal(a.Settings)
	if err != nil {
		return ipprofile.Account{}, err
	}
	row, err := s.q.CreateContentAccount(ctx, db.CreateContentAccountParams{
		AccountID: a.AccountID, WorkspaceID: a.WorkspaceID,
		Platform: a.Platform, DisplayName: a.DisplayName, Settings: settings,
	})
	if err != nil {
		return ipprofile.Account{}, err
	}
	return accountFromRow(row.AccountID, row.WorkspaceID, row.Platform, row.DisplayName,
		row.Settings, timestampToString(row.CreatedAt), timestampToString(row.UpdatedAt)), nil
}

func (s contentAccountStore) GetAccount(ctx context.Context, workspaceID, accountID string) (ipprofile.Account, error) {
	row, err := s.q.GetContentAccount(ctx, db.GetContentAccountParams{
		AccountID: accountID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return ipprofile.Account{}, ipprofile.ErrNotFound
	}
	return accountFromRow(row.AccountID, row.WorkspaceID, row.Platform, row.DisplayName,
		row.Settings, timestampToString(row.CreatedAt), timestampToString(row.UpdatedAt)), nil
}

func (s contentAccountStore) ListAccounts(ctx context.Context, workspaceID string) ([]ipprofile.Account, error) {
	rows, err := s.q.ListContentAccounts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	accounts := make([]ipprofile.Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, accountFromRow(row.AccountID, row.WorkspaceID, row.Platform,
			row.DisplayName, row.Settings, timestampToString(row.CreatedAt), timestampToString(row.UpdatedAt)))
	}
	return accounts, nil
}

func (s contentAccountStore) UpdateAccount(ctx context.Context, workspaceID, accountID string, patch ipprofile.Patch) (ipprofile.Account, error) {
	params := db.UpdateContentAccountParams{AccountID: accountID, WorkspaceID: workspaceID}
	if patch.Platform != nil {
		params.Platform = pgtype.Text{String: *patch.Platform, Valid: true}
	}
	if patch.DisplayName != nil {
		params.DisplayName = pgtype.Text{String: *patch.DisplayName, Valid: true}
	}
	if patch.Settings != nil {
		settings, err := json.Marshal(patch.Settings)
		if err != nil {
			return ipprofile.Account{}, err
		}
		params.Settings = settings
	}
	row, err := s.q.UpdateContentAccount(ctx, params)
	if err != nil {
		return ipprofile.Account{}, ipprofile.ErrNotFound
	}
	return accountFromRow(row.AccountID, row.WorkspaceID, row.Platform, row.DisplayName,
		row.Settings, timestampToString(row.CreatedAt), timestampToString(row.UpdatedAt)), nil
}

// contentAccountAuditor sends account changes to the diagnostics audit log.
type contentAccountAuditor struct{ h *Handler }

func (a contentAccountAuditor) Audit(ctx context.Context, event diagnostics.Event) error {
	if a.h.ContentDiagnostics == nil || a.h.ContentDiagnostics.Store == nil {
		return nil
	}
	return a.h.ContentDiagnostics.Store.Audit(ctx,
		diagnostics.Scope{Workspace: event.Workspace, Actor: event.Actor}, event)
}
