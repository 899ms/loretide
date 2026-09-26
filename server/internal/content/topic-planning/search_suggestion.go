package topicplanning

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Search optimization suggestion storage (specs/036 PR 2): create, revise,
// read, list, compare and abandon. Adoption is PR 3.
//
// Every write follows contract §7.1 and FR-103, in this order:
//
//  1. shape checks, outside any transaction (pure);
//  2. begin, whose first statement is the workspace delete fence - a
//     workspace whose deletion has committed answers ErrNotFound here and
//     nothing below runs;
//  3. for a revision or a decision: lock the permanent first revision, then
//     read the current revision (ErrNotFound if none);
//  4. references: the theme (this module), the materials, and the base
//     version and its document (through SearchWorks, reads only);
//  5. revision and state (409);
//  6. the business row, then the audit row, in the same transaction.
//
// The three tables are insert-only: nothing in this package updates or
// deletes them (search_guards_test.go), and PR 2 writes no effect row.
//
// The row lock in step 3 keeps a revision and a decision from passing each
// other: a revision takes the first revision row FOR UPDATE, a decision
// takes it FOR SHARE, and each then reads again in a new statement, so the
// later of the two sees what the earlier one wrote. Two decisions share the
// lock and are told apart by the unique index on (workspace_id,
// suggestion_id): the loser gets the same 409 the read check gives.

// SuggestionStore is the topic planning store with the one port suggestions
// need beyond it. It wraps Store rather than adding a field to it: the
// fence, the audit and the other ports are Store's, reached through the
// embedding.
type SuggestionStore struct {
	*Store
	Works      SearchWorks
	afterApply func() error // test-only seam for the post-version effect write.
}

const searchSuggestionColumns = `workspace_id, suggestion_id, revision, work_id, artifact_id, base_version_id,
	theme_id, theme_revision, target_question, aspects, rationale, evidence_source_ids, proposed_body,
	author_kind, recorded_by, created_at`

const searchDecisionColumns = `decision_id, suggestion_id, suggestion_revision, decision, note, decided_by, created_at`

const searchEffectColumns = `effect_id, decision_id, outcome, version_id, failure_code, created_at`

// decisionInsertHook is a test seam carried on the context, never set in
// production: it runs after every check has passed and just before the
// decision INSERT, so a test can hold two writers there and prove that the
// unique index, not the read check, decides between them.
type decisionInsertHook struct{}

func runDecisionInsertHook(ctx context.Context) {
	if hook, ok := ctx.Value(decisionInsertHook{}).(func()); ok {
		hook()
	}
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanSuggestion(row themeRow) (SearchSuggestion, error) {
	var suggestion SearchSuggestion
	var workspaceID string
	var aspects []string
	var evidence []byte
	err := row.Scan(&workspaceID, &suggestion.SuggestionID, &suggestion.Revision, &suggestion.WorkID,
		&suggestion.ArtifactID, &suggestion.BaseVersionID, &suggestion.ThemeID, &suggestion.ThemeRevision,
		&suggestion.TargetQuestion, &aspects, &suggestion.Rationale, &evidence, &suggestion.ProposedBody,
		&suggestion.AuthorKind, &suggestion.RecordedBy, &suggestion.CreatedAt)
	if err != nil {
		return SearchSuggestion{}, err
	}
	suggestion.Aspects = make([]SuggestionAspect, 0, len(aspects))
	for _, aspect := range aspects {
		suggestion.Aspects = append(suggestion.Aspects, SuggestionAspect(aspect))
	}
	if suggestion.EvidenceSourceIDs, err = decodeStrings(evidence); err != nil {
		return SearchSuggestion{}, ErrStorage
	}
	return suggestion, nil
}

func scanDecision(row themeRow) (SuggestionDecisionRecord, error) {
	var decision SuggestionDecisionRecord
	err := row.Scan(&decision.DecisionID, &decision.SuggestionID, &decision.SuggestionRevision,
		&decision.Decision, &decision.Note, &decision.DecidedBy, &decision.CreatedAt)
	return decision, err
}

func scanEffect(row themeRow) (SuggestionEffect, error) {
	var effect SuggestionEffect
	err := row.Scan(&effect.EffectID, &effect.DecisionID, &effect.Outcome, &effect.VersionID,
		&effect.FailureCode, &effect.CreatedAt)
	return effect, err
}

// themeHead is a theme's current revision, as a suggestion needs it.
type themeHead struct {
	revision int64
	voided   bool
}

func readThemeHead(ctx context.Context, q rowQuerier, workspaceID, themeID string) (themeHead, error) {
	var head themeHead
	err := q.QueryRow(ctx, `SELECT revision, voided FROM content_search_theme_revision
		WHERE workspace_id = $1 AND theme_id = $2 ORDER BY revision DESC LIMIT 1`,
		workspaceID, themeID).Scan(&head.revision, &head.voided)
	return head, err
}

// worksReadError keeps an adapter's two answers apart: "not there" stays
// ErrNotFound, anything else is ErrStorage (FR-104).
func worksReadError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return ErrStorage
}

// suggestionReferences is what the reference checks read, kept for the
// answer so nothing has to be read twice.
type suggestionReferences struct {
	theme    themeHead
	baseBody string
	document SearchDocument
}

// checkSuggestionReferences refuses a theme, material or base version that is
// not this brand's, naming the field; a foreign id and a missing one get the
// same refusal (FR-013, FR-030, FR-031, FR-033). A body equal to the base
// version is refused naming proposed_body (FR-034). It runs inside the fenced
// write transaction and reads the base version through SearchWorks.
func (s *SuggestionStore) checkSuggestionReferences(ctx context.Context, tx pgx.Tx, workspaceID, actor string,
	target SuggestionTarget, content SuggestionContent, creating bool) (suggestionReferences, error) {
	refs := suggestionReferences{}
	head, err := readThemeHead(ctx, tx, workspaceID, target.ThemeID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return refs, FieldError{Field: "theme_id", Reason: "not found"}
	case err != nil:
		return refs, ErrStorage
	case creating && head.voided:
		return refs, FieldError{Field: "theme_id", Reason: "the theme is archived"}
	}
	refs.theme = head
	if len(content.EvidenceSourceIDs) > 0 {
		if s.Sources == nil {
			return refs, ErrStorage
		}
		for _, id := range content.EvidenceSourceIDs {
			exists, existsErr := s.Sources.Exists(ctx, workspaceID, id)
			switch {
			case errors.Is(existsErr, ErrNotFound) || existsErr == nil && !exists:
				return refs, FieldError{Field: "evidence_source_ids", Reason: "not found"}
			case existsErr != nil:
				return refs, ErrStorage
			}
		}
	}
	if s.Works == nil {
		return refs, ErrStorage
	}
	notHere := FieldError{Field: "base_version_id", Reason: "not found"}
	refs.baseBody, err = s.Works.VersionBody(ctx, workspaceID, actor, target.WorkID, target.ArtifactID, target.BaseVersionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return refs, notHere
		}
		return refs, ErrStorage
	}
	if content.ProposedBody == refs.baseBody {
		return refs, FieldError{Field: "proposed_body", Reason: "the same as the base version"}
	}
	refs.document, err = s.Works.Document(ctx, workspaceID, actor, target.WorkID, target.ArtifactID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return refs, notHere
		}
		return refs, ErrStorage
	}
	return refs, nil
}

// insertSuggestion writes one revision row. A unique violation is a second
// writer of the same revision number that got past the read check.
func insertSuggestion(ctx context.Context, tx pgx.Tx, workspaceID string, suggestion SearchSuggestion) (SearchSuggestion, error) {
	evidence, err := encodeStrings(suggestion.EvidenceSourceIDs)
	if err != nil {
		return SearchSuggestion{}, ErrInvalid
	}
	aspects := make([]string, 0, len(suggestion.Aspects))
	for _, aspect := range suggestion.Aspects {
		aspects = append(aspects, string(aspect))
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO content_search_suggestion_revision (
			workspace_id, suggestion_id, revision, work_id, artifact_id, base_version_id, theme_id,
			theme_revision, target_question, aspects, rationale, evidence_source_ids, proposed_body,
			author_kind, recorded_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+searchSuggestionColumns,
		workspaceID, suggestion.SuggestionID, suggestion.Revision, suggestion.WorkID, suggestion.ArtifactID,
		suggestion.BaseVersionID, suggestion.ThemeID, suggestion.ThemeRevision, suggestion.TargetQuestion,
		aspects, suggestion.Rationale, evidence, suggestion.ProposedBody, string(suggestion.AuthorKind),
		suggestion.RecordedBy)
	stored, err := scanSuggestion(row)
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return SearchSuggestion{}, SearchConflict{Field: "base_revision"}
		}
		return SearchSuggestion{}, ErrStorage
	}
	return stored, nil
}

// lockCurrentSuggestion locks a stable row for the lifetime of a suggestion.
// Locking the latest row lets a writer already waiting on an older revision
// race a new writer locking the new head. The first row is immutable and
// shared by all writers. Read the head in a fresh statement after acquiring
// that lock, so commits made while waiting are visible.
func lockCurrentSuggestion(ctx context.Context, tx pgx.Tx, workspaceID, suggestionID, lock string) (SearchSuggestion, error) {
	var revision int64
	err := tx.QueryRow(ctx, `SELECT revision FROM content_search_suggestion_revision
		WHERE workspace_id = $1 AND suggestion_id = $2 ORDER BY revision ASC LIMIT 1 `+lock,
		workspaceID, suggestionID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return SearchSuggestion{}, ErrNotFound
	}
	if err != nil {
		return SearchSuggestion{}, ErrStorage
	}
	current, err := scanSuggestion(tx.QueryRow(ctx, `SELECT `+searchSuggestionColumns+`
		FROM content_search_suggestion_revision WHERE workspace_id = $1 AND suggestion_id = $2
		ORDER BY revision DESC LIMIT 1`, workspaceID, suggestionID))
	if err != nil {
		return SearchSuggestion{}, ErrStorage
	}
	return current, nil
}

func hasDecision(ctx context.Context, tx pgx.Tx, workspaceID, suggestionID string) (bool, error) {
	var decided bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM content_search_suggestion_decision
		WHERE workspace_id = $1 AND suggestion_id = $2)`, workspaceID, suggestionID).Scan(&decided); err != nil {
		return false, ErrStorage
	}
	return decided, nil
}

// viewSuggestion derives what a read answers besides the stored revision
// (contract §5). A theme that is no longer there reads as changed.
func viewSuggestion(suggestion SearchSuggestion, decision *SuggestionDecisionRecord, effects []SuggestionEffect,
	theme themeHead, themeFound bool, document SearchDocument) SuggestionView {
	state, failure := DeriveSuggestionState(decision, effects)
	if effects == nil {
		effects = []SuggestionEffect{}
	}
	return SuggestionView{
		SearchSuggestion: suggestion,
		State:            state,
		FailureCode:      failure,
		BaseIsCurrent:    document.LatestVersionID != "" && document.LatestVersionID == suggestion.BaseVersionID,
		ThemeChanged:     !themeFound || theme.voided || theme.revision > suggestion.ThemeRevision,
		Decision:         decision,
		Effects:          effects,
	}
}

func (s *SuggestionStore) writable() bool { return s != nil && s.Store != nil }

// CreateSearchSuggestion records revision 1 of a new suggestion.
func (s *SuggestionStore) CreateSearchSuggestion(ctx context.Context, workspaceID, actor string, req SuggestionRequest) (SuggestionView, error) {
	const step = "create-search-suggestion"
	if !s.writable() {
		return SuggestionView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		s.reportThemeFailure(ctx, workspaceID, actor, "", step, ErrInvalid)
		return SuggestionView{}, ErrInvalid
	}
	target, err := NormalizeSuggestionTarget(req.Target)
	if err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, "", step, err)
		return SuggestionView{}, err
	}
	content, err := NormalizeSuggestionContent(req.Content)
	if err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, "", step, err)
		return SuggestionView{}, err
	}
	suggestionID := s.newID()
	fail := func(err error) (SuggestionView, error) {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, err)
		return SuggestionView{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	refs, err := s.checkSuggestionReferences(ctx, tx, workspaceID, actor, target, content, true)
	if err != nil {
		return fail(err)
	}
	stored, err := insertSuggestion(ctx, tx, workspaceID, SearchSuggestion{
		SuggestionID: suggestionID, Revision: 1, SuggestionTarget: target, ThemeRevision: refs.theme.revision,
		SuggestionContent: content, AuthorKind: AuthorHuman, RecordedBy: actor,
	})
	if err != nil {
		return fail(err)
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, suggestionID, step); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	view := viewSuggestion(stored, nil, nil, refs.theme, true, refs.document)
	diff := DiffLines(refs.baseBody, stored.ProposedBody)
	view.Diff = &diff
	return view, nil
}

// ReviseSearchSuggestion appends the next revision of an open suggestion.
// The whole content is submitted; the target cannot change (FR-030), and a
// suggestion with a decision takes no more revisions (FR-039).
func (s *SuggestionStore) ReviseSearchSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, req SuggestionRevisionRequest) (SuggestionView, error) {
	const step = "revise-search-suggestion"
	if !s.writable() {
		return SuggestionView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || suggestionID == "" {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, ErrNotFound)
		return SuggestionView{}, ErrNotFound
	}
	if req.BaseRevision < 1 {
		err := FieldError{Field: "base_revision", Reason: "required"}
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, err)
		return SuggestionView{}, err
	}
	content, err := NormalizeSuggestionContent(req.Content)
	if err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, err)
		return SuggestionView{}, err
	}
	fail := func(err error) (SuggestionView, error) {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, err)
		return SuggestionView{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	current, err := lockCurrentSuggestion(ctx, tx, workspaceID, suggestionID, "FOR UPDATE")
	if err != nil {
		return fail(err)
	}
	for _, field := range []struct{ name, sent, stored string }{
		{"work_id", req.Target.WorkID, current.WorkID},
		{"artifact_id", req.Target.ArtifactID, current.ArtifactID},
		{"base_version_id", req.Target.BaseVersionID, current.BaseVersionID},
		{"theme_id", req.Target.ThemeID, current.ThemeID},
	} {
		if sent := strings.TrimSpace(field.sent); sent != "" && sent != field.stored {
			return fail(FieldError{Field: field.name, Reason: "fixed for the life of a suggestion; write a new one"})
		}
	}
	refs, err := s.checkSuggestionReferences(ctx, tx, workspaceID, actor, current.SuggestionTarget, content, false)
	if err != nil {
		return fail(err)
	}
	if req.BaseRevision != current.Revision {
		return fail(SearchConflict{Field: "base_revision"})
	}
	decided, err := hasDecision(ctx, tx, workspaceID, suggestionID)
	if err != nil {
		return fail(err)
	}
	if decided {
		return fail(SearchConflict{Field: "suggestion_id"})
	}
	stored, err := insertSuggestion(ctx, tx, workspaceID, SearchSuggestion{
		SuggestionID: suggestionID, Revision: current.Revision + 1, SuggestionTarget: current.SuggestionTarget,
		ThemeRevision: refs.theme.revision, SuggestionContent: content, AuthorKind: AuthorHuman, RecordedBy: actor,
	})
	if err != nil {
		return fail(err)
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, suggestionID, step); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	view := viewSuggestion(stored, nil, nil, refs.theme, true, refs.document)
	diff := DiffLines(refs.baseBody, stored.ProposedBody)
	view.Diff = &diff
	return view, nil
}

// DecideSearchSuggestion records the one decision on a suggestion (FR-050).
func (s *SuggestionStore) DecideSearchSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, req DecisionRequest) (SuggestionView, error) {
	if !s.writable() {
		return SuggestionView{}, ErrStorage
	}
	if err := ValidateDecisionRequest(req); err != nil {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, "decide-search-suggestion", err)
		return SuggestionView{}, err
	}
	if req.Decision == DecisionAdopt {
		return s.adoptSearchSuggestion(ctx, workspaceID, actor, suggestionID, req)
	}
	return s.abandonSearchSuggestion(ctx, workspaceID, actor, suggestionID, req)
}

// adoptSearchSuggestion performs all preflight checks before it writes the
// irreversible decision. The version write is deliberately a separate,
// idempotent work-editor transaction; retries resume that operation rather
// than rechecking state which may have moved because the first attempt won.
func (s *SuggestionStore) adoptSearchSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, req DecisionRequest) (SuggestionView, error) {
	const step = "adopt-search-suggestion"
	if workspaceID == "" || actor == "" || suggestionID == "" || s.Works == nil {
		return SuggestionView{}, ErrNotFound
	}
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)
	current, err := lockCurrentSuggestion(ctx, tx, workspaceID, suggestionID, "FOR SHARE")
	if err != nil {
		return SuggestionView{}, err
	}
	if req.Revision != current.Revision {
		return SuggestionView{}, SearchConflict{Field: "revision"}
	}
	decided, err := hasDecision(ctx, tx, workspaceID, suggestionID)
	if err != nil {
		return SuggestionView{}, err
	}
	if decided {
		return SuggestionView{}, SearchConflict{Field: "suggestion_id"}
	}
	document, err := s.Works.Document(ctx, workspaceID, actor, current.WorkID, current.ArtifactID)
	if err != nil {
		return SuggestionView{}, worksReadError(err)
	}
	if document.LatestVersionID != current.BaseVersionID {
		return SuggestionView{}, SearchConflict{Field: "base_version_id"}
	}
	if !document.DraftSaved {
		return SuggestionView{}, SearchConflict{Field: "draft_status"}
	}
	decision, err := scanDecision(tx.QueryRow(ctx, `INSERT INTO content_search_suggestion_decision (
		workspace_id, decision_id, suggestion_id, suggestion_revision, decision, note, decided_by
	) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+searchDecisionColumns,
		workspaceID, s.newID(), suggestionID, current.Revision, string(DecisionAdopt), req.Note, actor))
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return SuggestionView{}, SearchConflict{Field: "suggestion_id"}
		}
		return SuggestionView{}, ErrStorage
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, suggestionID, step); err != nil {
		return SuggestionView{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return SuggestionView{}, ErrStorage
	}
	return s.completeSearchAdoption(ctx, workspaceID, actor, current, decision)
}

func adoptionFailure(err error) EffectFailure {
	switch {
	case errors.Is(err, ErrBaseMoved):
		return FailureBaseMoved
	case errors.Is(err, ErrDraftUnsaved):
		return FailureDraftUnsaved
	case errors.Is(err, ErrNoChange):
		return FailureNoChange
	case errors.Is(err, ErrNotFound):
		return FailureTargetNotFound
	default:
		return FailureStorage
	}
}

// recordSearchEffect serializes retries on the decision row. Therefore at
// most one done row can be inserted even when two retry requests arrive at
// once; a second caller receives decision_id conflict after the first commit.
func (s *SuggestionStore) recordSearchEffect(ctx context.Context, workspaceID, actor string, decision SuggestionDecisionRecord,
	outcome EffectOutcome, versionID string, failure EffectFailure) error {
	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	stored, err := scanDecision(tx.QueryRow(ctx, `SELECT `+searchDecisionColumns+` FROM content_search_suggestion_decision
		WHERE workspace_id=$1 AND decision_id=$2 FOR UPDATE`, workspaceID, decision.DecisionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || stored.Decision != DecisionAdopt {
		return ErrStorage
	}
	var done bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM content_search_suggestion_effect
		WHERE workspace_id=$1 AND decision_id=$2 AND outcome='done')`, workspaceID, decision.DecisionID).Scan(&done); err != nil {
		return ErrStorage
	}
	if done {
		return SearchConflict{Field: "decision_id"}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO content_search_suggestion_effect
		(workspace_id, effect_id, decision_id, outcome, version_id, failure_code)
		VALUES ($1,$2,$3,$4,$5,$6)`, workspaceID, s.newID(), decision.DecisionID, outcome, versionID, failure); err != nil {
		return ErrStorage
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, decision.SuggestionID, "record-search-suggestion-effect"); err != nil {
		return ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return ErrStorage
	}
	return nil
}

func (s *SuggestionStore) completeSearchAdoption(ctx context.Context, workspaceID, actor string,
	suggestion SearchSuggestion, decision SuggestionDecisionRecord) (SuggestionView, error) {
	versionID, applyErr := s.Works.Apply(ctx, workspaceID, actor, SearchApply{
		Key: "search-suggestion:" + suggestion.SuggestionID, WorkID: suggestion.WorkID,
		ArtifactID: suggestion.ArtifactID, BaseVersionID: suggestion.BaseVersionID, Body: suggestion.ProposedBody,
	})
	if applyErr != nil {
		if err := s.recordSearchEffect(ctx, workspaceID, actor, decision, EffectFailed, "", adoptionFailure(applyErr)); err != nil {
			return SuggestionView{}, err
		}
		return s.GetSearchSuggestion(ctx, workspaceID, actor, suggestion.SuggestionID)
	}
	if s.afterApply != nil && s.afterApply() != nil {
		// Test-only: model an unavailable effect ledger after the idempotent
		// work-editor transaction committed. The next retry must converge.
		return s.GetSearchSuggestion(ctx, workspaceID, actor, suggestion.SuggestionID)
	}
	if err := s.recordSearchEffect(ctx, workspaceID, actor, decision, EffectDone, versionID, ""); err != nil {
		// The version transaction has already committed. Returning the derived
		// view exposes adopt_unrecorded so a retry can finish recording it.
		return s.GetSearchSuggestion(ctx, workspaceID, actor, suggestion.SuggestionID)
	}
	return s.GetSearchSuggestion(ctx, workspaceID, actor, suggestion.SuggestionID)
}

// RetrySearchSuggestionDecision retries only an adopted decision that has no
// done effect. It intentionally skips the original preflight: ApplyBody's
// idempotency claim returns the version the first attempt already wrote.
func (s *SuggestionStore) RetrySearchSuggestionDecision(ctx context.Context, workspaceID, actor, decisionID string) (SuggestionView, error) {
	if !s.writable() || workspaceID == "" || actor == "" || decisionID == "" {
		return SuggestionView{}, ErrNotFound
	}
	decision, err := scanDecision(s.DB.QueryRow(ctx, `SELECT `+searchDecisionColumns+` FROM content_search_suggestion_decision
		WHERE workspace_id=$1 AND decision_id=$2`, workspaceID, decisionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionView{}, ErrNotFound
	}
	if err != nil {
		return SuggestionView{}, ErrStorage
	}
	if decision.Decision != DecisionAdopt {
		return SuggestionView{}, SearchConflict{Field: "decision_id"}
	}
	suggestions, err := s.currentSuggestions(ctx, workspaceID, []string{decision.SuggestionID}, SuggestionFilter{})
	if err != nil {
		return SuggestionView{}, ErrStorage
	}
	if len(suggestions) != 1 {
		return SuggestionView{}, ErrNotFound
	}
	view, err := s.GetSearchSuggestion(ctx, workspaceID, actor, decision.SuggestionID)
	if err != nil {
		return SuggestionView{}, err
	}
	if view.State == StateAdopted {
		return SuggestionView{}, SearchConflict{Field: "decision_id"}
	}
	return s.completeSearchAdoption(ctx, workspaceID, actor, suggestions[0], decision)
}

// abandonSearchSuggestion writes the abandon decision and its audit row, and
// nothing else (FR-051): no work-editor write, no effect row, no change to the
// theme, the topic card, the brief or the document. Abandoning does not ask
// whether the base version is still the latest. The document is read - a
// read - only to answer with the suggestion as it now stands.
func (s *SuggestionStore) abandonSearchSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, req DecisionRequest) (SuggestionView, error) {
	const step = "abandon-search-suggestion"
	if workspaceID == "" || actor == "" || suggestionID == "" {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, ErrNotFound)
		return SuggestionView{}, ErrNotFound
	}
	decisionID := s.newID()
	fail := func(err error) (SuggestionView, error) {
		s.reportThemeFailure(ctx, workspaceID, actor, suggestionID, step, err)
		return SuggestionView{}, err
	}

	tx, err := s.begin(ctx, workspaceID)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	current, err := lockCurrentSuggestion(ctx, tx, workspaceID, suggestionID, "FOR SHARE")
	if err != nil {
		return fail(err)
	}
	if s.Works == nil {
		return fail(ErrStorage)
	}
	document, err := s.Works.Document(ctx, workspaceID, actor, current.WorkID, current.ArtifactID)
	if err != nil {
		return fail(worksReadError(err))
	}
	theme, err := readThemeHead(ctx, tx, workspaceID, current.ThemeID)
	themeFound := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fail(ErrStorage)
	}
	if req.Revision != current.Revision {
		return fail(SearchConflict{Field: "revision"})
	}
	decided, err := hasDecision(ctx, tx, workspaceID, suggestionID)
	if err != nil {
		return fail(err)
	}
	if decided {
		return fail(SearchConflict{Field: "suggestion_id"})
	}
	runDecisionInsertHook(ctx)
	decision, err := scanDecision(tx.QueryRow(ctx, `
		INSERT INTO content_search_suggestion_decision (
			workspace_id, decision_id, suggestion_id, suggestion_revision, decision, note, decided_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+searchDecisionColumns,
		workspaceID, decisionID, suggestionID, current.Revision, string(DecisionAbandon), req.Note, actor))
	if err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return fail(SearchConflict{Field: "suggestion_id"})
		}
		return fail(ErrStorage)
	}
	if _, err = s.audit(ctx, tx, workspaceID, actor, suggestionID, step); err != nil {
		return fail(ErrStorage)
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(ErrStorage)
	}
	return viewSuggestion(current, &decision, nil, theme, themeFound, document), nil
}

func (s *SuggestionStore) readable() bool { return s != nil && s.Store != nil && s.DB != nil }

// SuggestionExists answers whether a suggestion is this brand's, reading
// this module only. It lets a handler answer a path that is not here before
// it reads the body (contract §7.1).
func (s *SuggestionStore) SuggestionExists(ctx context.Context, workspaceID, actor, suggestionID string) error {
	if !s.readable() {
		return ErrStorage
	}
	if workspaceID == "" || actor == "" || suggestionID == "" {
		return ErrNotFound
	}
	var found bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM content_search_suggestion_revision
		WHERE workspace_id = $1 AND suggestion_id = $2)`, workspaceID, suggestionID).Scan(&found); err != nil {
		return ErrStorage
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (s *SuggestionStore) querySuggestions(ctx context.Context, sql string, args ...any) ([]SearchSuggestion, error) {
	rows, err := s.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, ErrStorage
	}
	defer rows.Close()
	suggestions := []SearchSuggestion{}
	for rows.Next() {
		suggestion, scanErr := scanSuggestion(rows)
		if scanErr != nil {
			return nil, ErrStorage
		}
		suggestions = append(suggestions, suggestion)
	}
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	return suggestions, nil
}

// currentSuggestions answers the current revision of each suggestion that
// matches; an empty filter value does not filter.
func (s *SuggestionStore) currentSuggestions(ctx context.Context, workspaceID string, ids []string, filter SuggestionFilter) ([]SearchSuggestion, error) {
	return s.querySuggestions(ctx, `SELECT DISTINCT ON (suggestion_id) `+searchSuggestionColumns+`
		FROM content_search_suggestion_revision
		WHERE workspace_id = $1
		  AND (cardinality($2::text[]) = 0 OR suggestion_id = ANY($2::text[]))
		  AND ($3 = '' OR work_id = $3) AND ($4 = '' OR artifact_id = $4) AND ($5 = '' OR theme_id = $5)
		ORDER BY suggestion_id, revision DESC`,
		workspaceID, normalizeStrings(ids), filter.WorkID, filter.ArtifactID, filter.ThemeID)
}

// views derives what each suggestion reads as. The document of each is read
// once through SearchWorks; with withDiff the base body too.
func (s *SuggestionStore) views(ctx context.Context, workspaceID, actor string, suggestions []SearchSuggestion, withDiff bool) ([]SuggestionView, error) {
	if len(suggestions) == 0 {
		return []SuggestionView{}, nil
	}
	if s.Works == nil {
		return nil, ErrStorage
	}
	suggestionIDs, themeIDs := []string{}, []string{}
	for _, suggestion := range suggestions {
		suggestionIDs = append(suggestionIDs, suggestion.SuggestionID)
		if !slices.Contains(themeIDs, suggestion.ThemeID) {
			themeIDs = append(themeIDs, suggestion.ThemeID)
		}
	}
	decisions := map[string]*SuggestionDecisionRecord{}
	decisionIDs := []string{}
	rows, err := s.DB.Query(ctx, `SELECT `+searchDecisionColumns+` FROM content_search_suggestion_decision
		WHERE workspace_id = $1 AND suggestion_id = ANY($2::text[]) ORDER BY created_at, decision_id`,
		workspaceID, suggestionIDs)
	if err != nil {
		return nil, ErrStorage
	}
	for rows.Next() {
		decision, scanErr := scanDecision(rows)
		if scanErr != nil {
			rows.Close()
			return nil, ErrStorage
		}
		if _, seen := decisions[decision.SuggestionID]; !seen {
			decisions[decision.SuggestionID] = &decision
			decisionIDs = append(decisionIDs, decision.DecisionID)
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, ErrStorage
	}
	effects := map[string][]SuggestionEffect{}
	if len(decisionIDs) > 0 {
		rows, err = s.DB.Query(ctx, `SELECT `+searchEffectColumns+` FROM content_search_suggestion_effect
			WHERE workspace_id = $1 AND decision_id = ANY($2::text[]) ORDER BY created_at, effect_id`,
			workspaceID, decisionIDs)
		if err != nil {
			return nil, ErrStorage
		}
		for rows.Next() {
			effect, scanErr := scanEffect(rows)
			if scanErr != nil {
				rows.Close()
				return nil, ErrStorage
			}
			effects[effect.DecisionID] = append(effects[effect.DecisionID], effect)
		}
		rows.Close()
		if rows.Err() != nil {
			return nil, ErrStorage
		}
	}
	themes := map[string]themeHead{}
	rows, err = s.DB.Query(ctx, `SELECT DISTINCT ON (theme_id) theme_id, revision, voided
		FROM content_search_theme_revision WHERE workspace_id = $1 AND theme_id = ANY($2::text[])
		ORDER BY theme_id, revision DESC`, workspaceID, themeIDs)
	if err != nil {
		return nil, ErrStorage
	}
	for rows.Next() {
		var themeID string
		var head themeHead
		if scanErr := rows.Scan(&themeID, &head.revision, &head.voided); scanErr != nil {
			rows.Close()
			return nil, ErrStorage
		}
		themes[themeID] = head
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, ErrStorage
	}

	documents := map[[2]string]SearchDocument{}
	bodies := map[[3]string]string{}
	views := make([]SuggestionView, 0, len(suggestions))
	for _, suggestion := range suggestions {
		docKey := [2]string{suggestion.WorkID, suggestion.ArtifactID}
		document, ok := documents[docKey]
		if !ok {
			if document, err = s.Works.Document(ctx, workspaceID, actor, suggestion.WorkID, suggestion.ArtifactID); err != nil {
				return nil, worksReadError(err)
			}
			documents[docKey] = document
		}
		decision := decisions[suggestion.SuggestionID]
		var decisionEffects []SuggestionEffect
		if decision != nil {
			decisionEffects = effects[decision.DecisionID]
		}
		theme, themeFound := themes[suggestion.ThemeID]
		view := viewSuggestion(suggestion, decision, decisionEffects, theme, themeFound, document)
		if withDiff {
			bodyKey := [3]string{suggestion.WorkID, suggestion.ArtifactID, suggestion.BaseVersionID}
			base, seen := bodies[bodyKey]
			if !seen {
				if base, err = s.Works.VersionBody(ctx, workspaceID, actor, suggestion.WorkID, suggestion.ArtifactID, suggestion.BaseVersionID); err != nil {
					return nil, worksReadError(err)
				}
				bodies[bodyKey] = base
			}
			diff := DiffLines(base, suggestion.ProposedBody)
			view.Diff = &diff
		}
		views = append(views, view)
	}
	return views, nil
}

func sortSuggestionViews(views []SuggestionView) {
	slices.SortFunc(views, func(a, b SuggestionView) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.SuggestionID, b.SuggestionID))
	})
}

// GetSearchSuggestion answers a suggestion's current revision with its
// difference from the base version, its decision and effects, and its
// derived state.
func (s *SuggestionStore) GetSearchSuggestion(ctx context.Context, workspaceID, actor, suggestionID string) (SuggestionView, error) {
	if !s.readable() {
		return SuggestionView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" || suggestionID == "" {
		return SuggestionView{}, ErrNotFound
	}
	suggestions, err := s.currentSuggestions(ctx, workspaceID, []string{suggestionID}, SuggestionFilter{})
	if err != nil {
		return SuggestionView{}, err
	}
	if len(suggestions) == 0 {
		return SuggestionView{}, ErrNotFound
	}
	views, err := s.views(ctx, workspaceID, actor, suggestions, true)
	if err != nil {
		return SuggestionView{}, err
	}
	return views[0], nil
}

// ListSearchSuggestions answers the current revision of each suggestion,
// filtered by work, document, theme and derived state (FR-041), ordered by
// created_at, then suggestion_id. No diff: a list is for choosing.
func (s *SuggestionStore) ListSearchSuggestions(ctx context.Context, workspaceID, actor string, filter SuggestionFilter) ([]SuggestionView, error) {
	if !s.readable() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrNotFound
	}
	suggestions, err := s.currentSuggestions(ctx, workspaceID, nil, filter)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, workspaceID, actor, suggestions, false)
	if err != nil {
		return nil, err
	}
	kept := []SuggestionView{}
	for _, view := range views {
		if filter.State == "" || view.State == filter.State {
			kept = append(kept, view)
		}
	}
	sortSuggestionViews(kept)
	return kept, nil
}

// CompareSearchSuggestions answers two to four suggestions side by side,
// each with its difference from its own base version (FR-040). It reads and
// writes nothing else: no ranking, no score, no recommendation. A suggestion
// that is not this brand's answers like one that does not exist.
func (s *SuggestionStore) CompareSearchSuggestions(ctx context.Context, workspaceID, actor string, ids []string) (SuggestionComparison, error) {
	if !s.readable() {
		return SuggestionComparison{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return SuggestionComparison{}, ErrNotFound
	}
	if len(ids) < MinComparedIDs || len(ids) > MaxComparedIDs {
		return SuggestionComparison{}, FieldError{Field: "ids", Reason: "two to four distinct suggestion ids"}
	}
	suggestions, err := s.currentSuggestions(ctx, workspaceID, ids, SuggestionFilter{})
	if err != nil {
		return SuggestionComparison{}, err
	}
	if len(suggestions) != len(ids) {
		return SuggestionComparison{}, ErrNotFound
	}
	views, err := s.views(ctx, workspaceID, actor, suggestions, true)
	if err != nil {
		return SuggestionComparison{}, err
	}
	sortSuggestionViews(views)
	sameBase := true
	for _, view := range views[1:] {
		if view.BaseVersionID != views[0].BaseVersionID {
			sameBase = false
		}
	}
	return SuggestionComparison{Suggestions: views, SameBase: sameBase}, nil
}
