package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Persona prompt revisions. Setting a prompt APPENDS a revision; there is no
// endpoint that edits or deletes one, because a run that pinned a revision has
// to keep seeing what it pinned.

// SetAccountPersonaPrompt records a new revision for the account.
func (h *Handler) SetAccountPersonaPrompt(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	accountID := accountIDFromURL(r)
	if _, err := h.contentAccountService().Get(r.Context(), workspace, accountID); err != nil {
		// Unknown account and another brand's account are the same answer.
		logAccountFailure(r, "set persona prompt: load account", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}
	var body struct {
		PersonaPrompt string `json:"persona_prompt"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&body) != nil {
		h.accountInputError(w, ipprofile.ErrPersonaPromptTooLong)
		return
	}
	revision, err := h.contentAccountService().SetPersonaPrompt(
		r.Context(), workspace, actor, accountID, body.PersonaPrompt)
	if err != nil {
		logAccountFailure(r, "set persona prompt", err)
		h.personaWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, revision)
}

// GetAccountPersonaPrompt returns the current revision: the highest-numbered
// one. There is no pointer column that could disagree with this.
func (h *Handler) GetAccountPersonaPrompt(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	revision, err := h.contentAccountService().CurrentPersonaRevision(
		r.Context(), workspace, accountIDFromURL(r))
	if err != nil {
		logAccountFailure(r, "get current persona revision", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

// GetAccountPersonaRevision returns one revision by id - what a run pins.
func (h *Handler) GetAccountPersonaRevision(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	revision, err := h.contentAccountService().PersonaRevision(
		r.Context(), workspace, accountIDFromURL(r), chi.URLParam(r, "revisionId"))
	if err != nil {
		logAccountFailure(r, "get persona revision", err)
		h.writeAccountRefusal(w, r, workspacecore.ReasonNotMember)
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

func (h *Handler) ListAccountPersonaRevisions(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.accountScope(w, r)
	if !ok {
		return
	}
	revisions, err := h.contentAccountService().ListPersonaRevisions(
		r.Context(), workspace, accountIDFromURL(r))
	if err != nil {
		logAccountFailure(r, "list persona revisions", err)
		writeError(w, http.StatusServiceUnavailable, "revisions unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revisions": revisions})
}

// personaWriteError separates a prompt that is too long (400) from persistent
// contention (409, the write did not happen and may be retried) from anything
// that means "you cannot have this" (404, same as not existing).
func (h *Handler) personaWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ipprofile.ErrPersonaPromptTooLong):
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
	default:
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	}
}

// contentRevisionStore adapts the generated queries to the module's interface.
// Append-only: it offers an insert and reads, and nothing that mutates a row.
type contentRevisionStore struct{ q *db.Queries }

func revisionFromRow(revisionID, accountID, workspaceID string, revision int64, prompt string, rawProfile []byte, created string) ipprofile.Revision {
	// A revision written before migration 482 has an empty profile document.
	// It reads back as every field pending, which is exactly right: nobody
	// filled those fields in, so nothing about them is confirmed.
	profile := ipprofile.ExpressionProfile{}
	if len(rawProfile) > 0 {
		_ = json.Unmarshal(rawProfile, &profile)
	}
	profile = ipprofile.NormalizeProfile(profile)
	return ipprofile.Revision{
		RevisionID: revisionID, AccountID: accountID, WorkspaceID: workspaceID,
		Revision: revision, PersonaPrompt: prompt, Profile: profile, CreatedAt: created,
	}
}

func (s contentRevisionStore) NextRevision(ctx context.Context, workspaceID, accountID string) (int64, error) {
	next, err := s.q.NextContentAccountRevision(ctx, db.NextContentAccountRevisionParams{
		AccountID: accountID, WorkspaceID: workspaceID,
	})
	return int64(next), err
}

func (s contentRevisionStore) InsertRevision(ctx context.Context, revision ipprofile.Revision) (ipprofile.Revision, error) {
	profile, err := json.Marshal(revision.Profile)
	if err != nil {
		return ipprofile.Revision{}, err
	}
	row, err := s.q.InsertContentAccountRevision(ctx, db.InsertContentAccountRevisionParams{
		RevisionID: revision.RevisionID, AccountID: revision.AccountID,
		WorkspaceID: revision.WorkspaceID, Revision: revision.Revision,
		PersonaPrompt: revision.PersonaPrompt, Profile: profile,
	})
	if err != nil {
		// Somebody else claimed this revision number. Retryable, and the module
		// is what decides how many times.
		if isUniqueViolation(err) {
			return ipprofile.Revision{}, ipprofile.ErrRevisionTaken
		}
		return ipprofile.Revision{}, err
	}
	return revisionFromRow(row.RevisionID, row.AccountID, row.WorkspaceID, row.Revision,
		row.PersonaPrompt, row.Profile, timestampToString(row.CreatedAt)), nil
}

func (s contentRevisionStore) GetRevision(ctx context.Context, workspaceID, revisionID string) (ipprofile.Revision, error) {
	row, err := s.q.GetContentAccountRevision(ctx, db.GetContentAccountRevisionParams{
		RevisionID: revisionID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return ipprofile.Revision{}, accountStorageError(err)
	}
	return revisionFromRow(row.RevisionID, row.AccountID, row.WorkspaceID, row.Revision,
		row.PersonaPrompt, row.Profile, timestampToString(row.CreatedAt)), nil
}

func (s contentRevisionStore) CurrentRevision(ctx context.Context, workspaceID, accountID string) (ipprofile.Revision, error) {
	row, err := s.q.GetCurrentContentAccountRevision(ctx, db.GetCurrentContentAccountRevisionParams{
		AccountID: accountID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return ipprofile.Revision{}, accountStorageError(err)
	}
	return revisionFromRow(row.RevisionID, row.AccountID, row.WorkspaceID, row.Revision,
		row.PersonaPrompt, row.Profile, timestampToString(row.CreatedAt)), nil
}

func (s contentRevisionStore) ListRevisions(ctx context.Context, workspaceID, accountID string) ([]ipprofile.Revision, error) {
	rows, err := s.q.ListContentAccountRevisions(ctx, db.ListContentAccountRevisionsParams{
		AccountID: accountID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	revisions := make([]ipprofile.Revision, 0, len(rows))
	for _, row := range rows {
		revisions = append(revisions, revisionFromRow(row.RevisionID, row.AccountID,
			row.WorkspaceID, row.Revision, row.PersonaPrompt, row.Profile,
			timestampToString(row.CreatedAt)))
	}
	return revisions, nil
}
