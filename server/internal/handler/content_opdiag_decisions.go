package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
)

// Brand/account operating diagnosis (specs/035 PR 3): judgements,
// suggestions, decisions and what adopting produces, profile change
// proposals and todos, under /api/content-operating-diagnosis.
//
// This file is an adapter, like content_opdiag_reports.go. feedback-learning
// decides; it imports neither topic-planning nor ip-profile. The two writes
// adopting a suggestion needs answer its interfaces here, wired into
// h.opdiagStore() the way content_roi_records.go wires roiAccounts and
// roiWorks into h.roiStore():
//
//   - opdiagTopicCards answers TopicCardCreator through topic-planning's
//     public Store: CreateOnce creates a draft card under the suggestion's
//     key (and answers the one already there), Get checks a card to link.
//     Nothing here starts, reviews or publishes a card.
//   - opdiagProfileWriter answers DiagProfileWriter through ip-profile's
//     public Service: the current text items, and - only on an explicit
//     confirmation - SetProfile with every item as it is and the proposed
//     ones confirmed.
//
// In both, a workspace that is gone answers the module's ErrNotFound, the
// same 404 as every other fenced write, and anything else the owning
// module could not do is ErrStorage.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §7

// opdiagTopicCards answers feedbacklearning.TopicCardCreator.
type opdiagTopicCards struct{ store *topicplanning.Store }

func (c opdiagTopicCards) CreateOnce(ctx context.Context, workspaceID, actor, key string, draft feedbacklearning.TopicCardDraft) (string, bool, error) {
	if c.store == nil {
		return "", false, feedbacklearning.ErrStorage
	}
	card := topicplanning.TopicCard{WorkspaceID: workspaceID, IPFit: draft.IPFit}
	if draft.AccountID != "" {
		accountID := draft.AccountID
		card.AccountID = &accountID
	}
	written, created, err := c.store.CreateOnce(ctx, actor, card, key)
	if err != nil {
		return "", false, opdiagTopicError(err)
	}
	return written.TopicCardID, created, nil
}

// Exists answers the link mode: the card is here and is for the account
// ("" = a card with no account). Any mismatch answers like a missing card.
func (c opdiagTopicCards) Exists(ctx context.Context, workspaceID, actor, topicCardID, accountID string) error {
	if c.store == nil {
		return feedbacklearning.ErrStorage
	}
	card, err := c.store.Get(ctx, workspaceID, actor, topicCardID)
	switch {
	case errors.Is(err, topicplanning.ErrInvalid):
		return feedbacklearning.ErrNotFound
	case err != nil:
		return opdiagTopicError(err)
	}
	cardAccount := ""
	if card.AccountID != nil {
		cardAccount = *card.AccountID
	}
	if cardAccount != accountID {
		return feedbacklearning.ErrNotFound
	}
	return nil
}

// opdiagTopicError maps topic-planning's errors: not found - a card, an
// account, or the workspace its fence found gone - is ErrNotFound; a draft
// it refused is ErrInvalid; anything else is ErrStorage.
func opdiagTopicError(err error) error {
	switch {
	case errors.Is(err, topicplanning.ErrNotFound):
		return feedbacklearning.ErrNotFound
	case errors.Is(err, topicplanning.ErrInvalid):
		return feedbacklearning.ErrInvalid
	default:
		return feedbacklearning.ErrStorage
	}
}

// opdiagProfileWriter answers feedbacklearning.DiagProfileWriter.
type opdiagProfileWriter struct{ service *ipprofile.Service }

// profileTextFields answers the eight text items of a profile by their JSON
// names, by pointer, in ip-profile's order.
func profileTextFields(profile *ipprofile.ExpressionProfile) map[string]*ipprofile.TextField {
	return map[string]*ipprofile.TextField{
		"audience":              &profile.Audience,
		"common_questions":      &profile.CommonQuestions,
		"experience":            &profile.Experience,
		"positioning":           &profile.Positioning,
		"content_pillars":       &profile.ContentPillars,
		"expression_style":      &profile.ExpressionStyle,
		"forbidden_expressions": &profile.ForbiddenExpressions,
		"content_goals":         &profile.ContentGoals,
	}
}

// currentRevision reads the account's current revision; an account with
// none answers the zero revision.
func (p opdiagProfileWriter) currentRevision(ctx context.Context, workspaceID, accountID string) (ipprofile.Revision, error) {
	if p.service == nil {
		return ipprofile.Revision{}, feedbacklearning.ErrStorage
	}
	revision, err := p.service.CurrentPersonaRevision(ctx, workspaceID, accountID)
	switch {
	case errors.Is(err, ipprofile.ErrNotFound):
		return ipprofile.Revision{}, nil
	case err != nil:
		return ipprofile.Revision{}, opdiagProfileError(err)
	}
	return revision, nil
}

func (p opdiagProfileWriter) CurrentProfileText(ctx context.Context, workspaceID, accountID string) (feedbacklearning.DiagProfileText, error) {
	revision, err := p.currentRevision(ctx, workspaceID, accountID)
	if err != nil {
		return feedbacklearning.DiagProfileText{}, err
	}
	text := feedbacklearning.DiagProfileText{
		RevisionID: revision.RevisionID, Values: map[string]string{}, Status: map[string]string{},
	}
	for key, field := range profileTextFields(&revision.Profile) {
		text.Values[key] = field.Value
		text.Status[key] = string(field.Status)
		if text.Status[key] == "" {
			text.Status[key] = string(ipprofile.FieldPending)
		}
	}
	return text, nil
}

// ApplyProfilePatches writes one new revision: the current profile with
// each patched item's text replaced and confirmed, the persona prompt and
// every other item carried forward by SetProfile itself. The current
// revision must still be the base.
func (p opdiagProfileWriter) ApplyProfilePatches(ctx context.Context, workspaceID, actor, accountID, baseRevisionID string,
	patches []feedbacklearning.ProfilePatch) (string, error) {
	revision, err := p.currentRevision(ctx, workspaceID, accountID)
	if err != nil {
		return "", err
	}
	if revision.RevisionID != baseRevisionID {
		return "", feedbacklearning.RevisionConflict{Field: "base_revision_id"}
	}
	profile := revision.Profile
	fields := profileTextFields(&profile)
	for _, patch := range patches {
		field, ok := fields[patch.Field]
		if !ok {
			return "", feedbacklearning.FieldError{Field: "patches.field", Reason: "not a text item"}
		}
		field.Value, field.Status = patch.Value, ipprofile.FieldConfirmed
	}
	written, err := p.service.SetProfile(ctx, workspaceID, actor, accountID, profile)
	if err != nil {
		return "", opdiagProfileError(err)
	}
	return written.RevisionID, nil
}

// opdiagProfileError maps ip-profile's errors: the workspace gone, or the
// account, is ErrNotFound; a profile it refused is ErrInvalid; a revision
// number it could not claim is the same 409 as a moved base; anything else
// is ErrStorage.
func opdiagProfileError(err error) error {
	switch {
	case errors.Is(err, ipprofile.ErrWorkspaceGone), errors.Is(err, ipprofile.ErrNotFound):
		return feedbacklearning.ErrNotFound
	case errors.Is(err, ipprofile.ErrProfile):
		return feedbacklearning.ErrInvalid
	case errors.Is(err, ipprofile.ErrRevisionConflict):
		return feedbacklearning.RevisionConflict{Field: "base_revision_id"}
	default:
		return feedbacklearning.ErrStorage
	}
}

// opdiagError answers the module's errors; a DecisionConflict is a 409
// naming its field, everything else is roiError's.
func (h *Handler) opdiagError(w http.ResponseWriter, err error) {
	if conflict, ok := errors.AsType[feedbacklearning.DecisionConflict](err); ok {
		h.feedbackDiagnosticError(w, http.StatusConflict, map[string]any{
			"field": conflict.Field, "reason": conflict.Reason,
		})
		return
	}
	h.roiError(w, err)
}

// opdiagRevisionFields are a revision body's base_revision and voided.
type opdiagRevisionFields struct {
	BaseRevision *int `json:"base_revision"`
	Voided       bool `json:"voided"`
}

func (f opdiagRevisionFields) revision() feedbacklearning.Revision {
	return feedbacklearning.Revision{BaseRevision: f.BaseRevision, Voided: f.Voided}
}

type opdiagJudgementRevisionBody struct {
	feedbacklearning.JudgementInput
	opdiagRevisionFields
}

type opdiagSuggestionRevisionBody struct {
	feedbacklearning.SuggestionInput
	opdiagRevisionFields
}

type opdiagTodoRevisionBody struct {
	feedbacklearning.TodoRevisionInput
	opdiagRevisionFields
}

// opdiagVersionPath reads {reportId} and {versionNo}. A version number that
// is not a positive integer answers exactly like a version that is not
// there.
func opdiagVersionPath(r *http.Request) (string, int, bool) {
	versionNo, ok := feedbacklearning.ParseVersionNo(chi.URLParam(r, "versionNo"))
	return chi.URLParam(r, "reportId"), versionNo, ok
}

// ---------------------------------------------------------------- annotations

// GetContentOpDiagAnnotations answers the judgements, suggestions and
// decisions on one report version.
func (h *Handler) GetContentOpDiagAnnotations(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reportID, versionNo, valid := opdiagVersionPath(r)
	if !valid {
		h.opdiagError(w, feedbacklearning.ErrNotFound)
		return
	}
	annotations, err := h.opdiagStore().Annotations(r.Context(), workspace, actor, reportID, versionNo)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, annotations)
}

// RecordContentOpDiagJudgement writes a new judgement on a report version.
func (h *Handler) RecordContentOpDiagJudgement(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reportID, versionNo, valid := opdiagVersionPath(r)
	if !valid {
		h.opdiagError(w, feedbacklearning.ErrNotFound)
		return
	}
	var body feedbacklearning.JudgementInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	judgement, err := h.opdiagStore().RecordJudgement(r.Context(), workspace, actor, reportID, versionNo, body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, judgement)
}

// ReviseContentOpDiagJudgement writes a judgement's next revision.
func (h *Handler) ReviseContentOpDiagJudgement(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagJudgementRevisionBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	judgement, err := h.opdiagStore().ReviseJudgement(r.Context(), workspace, actor, chi.URLParam(r, "judgementId"),
		body.JudgementInput, body.revision())
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, judgement)
}

// RecordContentOpDiagSuggestion writes a new suggestion on a report version.
func (h *Handler) RecordContentOpDiagSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	reportID, versionNo, valid := opdiagVersionPath(r)
	if !valid {
		h.opdiagError(w, feedbacklearning.ErrNotFound)
		return
	}
	var body feedbacklearning.SuggestionInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	suggestion, err := h.opdiagStore().RecordSuggestion(r.Context(), workspace, actor, reportID, versionNo, body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, suggestion)
}

// ReviseContentOpDiagSuggestion writes a suggestion's next revision.
func (h *Handler) ReviseContentOpDiagSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagSuggestionRevisionBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	suggestion, err := h.opdiagStore().ReviseSuggestion(r.Context(), workspace, actor, chi.URLParam(r, "suggestionId"),
		body.SuggestionInput, body.revision())
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, suggestion)
}

// ---------------------------------------------------------------- decisions

// DecideContentOpDiagSuggestion adopts or rejects one suggestion revision.
func (h *Handler) DecideContentOpDiagSuggestion(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.DecisionInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	view, err := h.opdiagStore().DecideSuggestion(r.Context(), workspace, actor, chi.URLParam(r, "suggestionId"), body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// RetryContentOpDiagDecision carries out an adopted topic card decision
// again.
func (h *Handler) RetryContentOpDiagDecision(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.RetryInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	view, err := h.opdiagStore().RetryDecision(r.Context(), workspace, actor, chi.URLParam(r, "decisionId"), body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// ---------------------------------------------------------------- proposals

// ListContentOpDiagProfileProposals answers the proposals, by account_id or
// state, each with "current value -> proposed value".
func (h *Handler) ListContentOpDiagProfileProposals(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	proposals, err := h.opdiagStore().ListProposals(r.Context(), workspace, actor, feedbacklearning.ProposalFilter{
		AccountID: query.Get("account_id"), State: feedbacklearning.ProposalState(query.Get("state")),
	})
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": proposals})
}

// ConfirmContentOpDiagProfileProposal is the explicit confirmation that
// writes a proposal into the account's profile.
func (h *Handler) ConfirmContentOpDiagProfileProposal(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.ProposalConfirmInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	proposal, err := h.opdiagStore().ConfirmProposal(r.Context(), workspace, actor, chi.URLParam(r, "proposalId"), body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, proposal)
}

// DismissContentOpDiagProfileProposal gives a proposal up.
func (h *Handler) DismissContentOpDiagProfileProposal(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body struct{}
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	proposal, err := h.opdiagStore().DismissProposal(r.Context(), workspace, actor, chi.URLParam(r, "proposalId"))
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, proposal)
}

// ---------------------------------------------------------------- todos

// ListContentOpDiagTodos answers the current todos, by account_id or state.
func (h *Handler) ListContentOpDiagTodos(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	todos, err := h.opdiagStore().ListTodos(r.Context(), workspace, actor, feedbacklearning.TodoFilter{
		AccountID: query.Get("account_id"), State: feedbacklearning.TodoState(query.Get("state")),
	})
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todos": todos})
}

// AddContentOpDiagTodo adds one gap of a report version as a todo.
func (h *Handler) AddContentOpDiagTodo(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body feedbacklearning.GapTodoInput
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	todo, err := h.opdiagStore().AddGapTodo(r.Context(), workspace, actor, body)
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, todo)
}

// ReviseContentOpDiagTodo writes a todo's next revision.
func (h *Handler) ReviseContentOpDiagTodo(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.feedbackScope(w, r)
	if !ok {
		return
	}
	var body opdiagTodoRevisionBody
	if err := decodeOpdiagBody(w, r, &body); err != nil {
		h.opdiagError(w, err)
		return
	}
	todo, err := h.opdiagStore().ReviseTodo(r.Context(), workspace, actor, chi.URLParam(r, "todoId"),
		body.TodoRevisionInput, body.revision())
	if err != nil {
		h.opdiagError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, todo)
}
