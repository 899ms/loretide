package feedbacklearning

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Deciding on a suggestion, and what adopting one produces (specs/035 PR 3:
// FR-060 to FR-069, FR-032; SC-006 to SC-008; D14-V05 "拒绝建议不改配置或记
// 忆，采纳有动作记录"; rulings Q3, Q3 supplement, Q4, Q8; contract §1.5 to
// §1.8, §7.2, §7.4).
//
// Four rules the shape of this file exists to keep:
//
//   - Every decision is recorded, adopt and reject alike, once per
//     suggestion revision. A reject is that row and its audit entry and
//     nothing else: recordRejection calls no adapter at all, and a guard
//     scans its body for the write interfaces' methods.
//   - Adopting produces one of three things through public interfaces, and
//     never a business memory (FR-068): a todo (this module's own table,
//     ruling Q4), a profile change proposal that still needs an explicit
//     confirmation before ip-profile is written (Q8), or a draft topic card.
//     Nothing here starts, reviews or publishes anything.
//   - A topic card is another module's write, so it cannot share the
//     decision's transaction (Q3=A). The decision is committed first, then
//     the card is created through TopicCardCreator.CreateOnce under the key
//     opdiag-suggestion:<suggestion_id>, then the outcome is recorded. A
//     failure between the second and third step leaves a card and no
//     outcome; a retry asks CreateOnce with the same key, gets the same
//     card back, and records it. One suggestion never has two cards.
//   - This module imports neither topic-planning nor ip-profile.
//     TopicCardCreator and DiagProfileWriter below use this module's types
//     and plain strings, and handler adapters answer them (as 034's
//     Accounts and Works); a guard scans every Go file here for the two
//     imports.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §1.5 to
// §1.8, §7

// MaxDecisionNoteRunes bounds a decision's note (contract §1.5).
const MaxDecisionNoteRunes = 2000

// MaxTodoTitleRunes and MaxTodoNoteRunes bound a todo (contract §1.8).
const (
	MaxTodoTitleRunes = 200
	MaxTodoNoteRunes  = 2000
)

// DecisionKind is D14-V05's adopt or reject.
type DecisionKind string

const (
	DecisionAdopt  DecisionKind = "adopt"
	DecisionReject DecisionKind = "reject"
)

var DecisionKinds = []DecisionKind{DecisionAdopt, DecisionReject}

// AdoptMode is how an adopted topic card suggestion is carried out (Q3=A):
// create a draft card, or link one that exists.
type AdoptMode string

const (
	ModeCreate AdoptMode = "create"
	ModeLink   AdoptMode = "link"
)

var AdoptModes = []AdoptMode{ModeCreate, ModeLink}

// EffectOutcome is what adopting produced: done, or failed with a code.
type EffectOutcome string

const (
	OutcomeDone   EffectOutcome = "done"
	OutcomeFailed EffectOutcome = "failed"
)

var EffectOutcomes = []EffectOutcome{OutcomeDone, OutcomeFailed}

// EffectFailure is why creating the topic card failed: it refused the
// draft, something it needed was not there, or storage failed.
type EffectFailure string

const (
	FailureTargetRefused  EffectFailure = "target_refused"
	FailureTargetNotFound EffectFailure = "target_not_found"
	FailureStorage        EffectFailure = "storage"
)

var EffectFailures = []EffectFailure{FailureTargetRefused, FailureTargetNotFound, FailureStorage}

// ProposalState is a profile change proposal's state (D3 "仍需显式确认").
type ProposalState string

const (
	ProposalProposed  ProposalState = "proposed"
	ProposalConfirmed ProposalState = "confirmed"
	ProposalDismissed ProposalState = "dismissed"
)

var ProposalStates = []ProposalState{ProposalProposed, ProposalConfirmed, ProposalDismissed}

// TodoState is a todo's state.
type TodoState string

const (
	TodoOpen    TodoState = "open"
	TodoDone    TodoState = "done"
	TodoDropped TodoState = "dropped"
)

var TodoStates = []TodoState{TodoOpen, TodoDone, TodoDropped}

// TodoOrigin is where a todo came from: an adopted suggestion, or a data gap
// a person added (FR-032, FR-064).
type TodoOrigin string

const (
	TodoFromSuggestion TodoOrigin = "suggestion"
	TodoFromDataGap    TodoOrigin = "data_gap"
)

var TodoOrigins = []TodoOrigin{TodoFromSuggestion, TodoFromDataGap}

// What a decision's outcome reads as (derived on read, never stored). An
// adopted decision with no outcome row is "已采纳，结果未记录": the card may
// exist, and a retry finds it.
const (
	EffectReadsNone       = "none"
	EffectReadsDone       = "done"
	EffectReadsFailed     = "failed"
	EffectReadsUnrecorded = "unrecorded"
)

// ---------------------------------------------------------------- write interfaces

// TopicCardDraft is what an adopted suggestion asks for. Plain strings only:
// this module does not import topic-planning.
type TopicCardDraft struct {
	AccountID string // "" = no account
	IPFit     string // the suggestion body
}

// TopicCardCreator creates and checks topic cards for adopted suggestions.
// A handler adapter answers it through topic-planning's public Store.
type TopicCardCreator interface {
	// CreateOnce returns the card created under key, creating it the first time.
	CreateOnce(ctx context.Context, workspaceID, actor, key string, draft TopicCardDraft) (topicCardID string, created bool, err error)
	// Exists answers the link mode: the card is here and its account matches.
	Exists(ctx context.Context, workspaceID, actor, topicCardID, accountID string) error
}

// DiagProfileText is an account's current profile revision as a proposal
// shows it: its id, and each of the eight text items' value and status, by
// the item's JSON name. RevisionID "" is an account with no revision yet.
type DiagProfileText struct {
	RevisionID string
	Values     map[string]string
	Status     map[string]string
}

// DiagProfileWriter reads an account's profile text and writes a confirmed
// proposal into it. A handler adapter answers it through ip-profile's public
// Service; this module does not import ip-profile.
type DiagProfileWriter interface {
	CurrentProfileText(ctx context.Context, workspaceID, accountID string) (DiagProfileText, error)
	// ApplyProfilePatches writes a new profile revision: every item as it is
	// now, each patched item's text replaced and confirmed. It answers the
	// new revision's id, and RevisionConflict naming base_revision_id when
	// the current revision is not baseRevisionID.
	ApplyProfilePatches(ctx context.Context, workspaceID, actor, accountID, baseRevisionID string, patches []ProfilePatch) (string, error)
}

// DecisionConflict is a 409 that is not a stale revision: the suggestion
// revision is already decided, the decision already has a done outcome, the
// proposal is no longer proposed, the gap already has a todo (contract §7.1
// step 9). Field names which.
type DecisionConflict struct {
	Field  string
	Reason string
}

func (e DecisionConflict) Error() string { return e.Reason + ": " + e.Field }
func (e DecisionConflict) Unwrap() error { return ErrConflict }

// ---------------------------------------------------------------- records

// DecisionInput is the body of POST /suggestions/{id}/decisions.
type DecisionInput struct {
	SuggestionRevision int          `json:"suggestion_revision"`
	Decision           DecisionKind `json:"decision"`
	Mode               AdoptMode    `json:"mode"`
	LinkTargetID       string       `json:"link_target_id"`
	Note               string       `json:"note"`
}

// RetryInput is the body of POST /decisions/{id}/retry. An empty mode keeps
// the decision's.
type RetryInput struct {
	Mode         AdoptMode `json:"mode"`
	LinkTargetID string    `json:"link_target_id"`
}

// SuggestionDecision is one stored decision.
type SuggestionDecision struct {
	DecisionID         string       `json:"decision_id"`
	SuggestionID       string       `json:"suggestion_id"`
	SuggestionRevision int          `json:"suggestion_revision"`
	Decision           DecisionKind `json:"decision"`
	Mode               AdoptMode    `json:"mode"`
	LinkTargetID       string       `json:"link_target_id"`
	Note               string       `json:"note"`
	DecidedBy          string       `json:"decided_by"`
	CreatedAt          time.Time    `json:"created_at"`
}

// Effect is one stored outcome of an adopted decision.
type Effect struct {
	EffectID    string           `json:"effect_id"`
	DecisionID  string           `json:"decision_id"`
	Outcome     EffectOutcome    `json:"outcome"`
	TargetKind  SuggestionTarget `json:"target_kind"`
	TargetID    string           `json:"target_id"`
	FailureCode EffectFailure    `json:"failure_code"`
	CreatedAt   time.Time        `json:"created_at"`
}

// DecisionView is a decision with its suggestion's target kind, its
// outcomes oldest first, and what they read as.
type DecisionView struct {
	SuggestionDecision
	TargetKind  SuggestionTarget `json:"target_kind"`
	Effects     []Effect         `json:"effects"`
	EffectState string           `json:"effect_state"`
}

// effectState derives what a decision's outcomes read as.
func effectState(decision DecisionKind, effects []Effect) string {
	if decision != DecisionAdopt {
		return EffectReadsNone
	}
	if len(effects) == 0 {
		return EffectReadsUnrecorded
	}
	if slices.ContainsFunc(effects, func(effect Effect) bool { return effect.Outcome == OutcomeDone }) {
		return EffectReadsDone
	}
	return EffectReadsFailed
}

// ProfileProposal is one stored proposal revision.
type ProfileProposal struct {
	ProposalID        string         `json:"proposal_id"`
	Revision          int            `json:"revision"`
	AccountID         string         `json:"account_id"`
	DecisionID        string         `json:"decision_id"`
	BaseRevisionID    string         `json:"base_revision_id"`
	Patches           []ProfilePatch `json:"patches"`
	State             ProposalState  `json:"state"`
	AppliedRevisionID string         `json:"applied_revision_id"`
	Voided            bool           `json:"voided"`
	RecordedBy        string         `json:"recorded_by"`
	CreatedAt         time.Time      `json:"created_at"`
}

// ProposalItem is one patched item: its text now, and what is proposed.
type ProposalItem struct {
	Field         string `json:"field"`
	CurrentValue  string `json:"current_value"`
	CurrentStatus string `json:"current_status"`
	ProposedValue string `json:"proposed_value"`
}

// ProposalView is a proposal with "current -> proposed" per item, and
// whether the account's profile is still the revision it was based on.
type ProposalView struct {
	ProfileProposal
	CurrentRevisionID string         `json:"current_revision_id"`
	BaseIsCurrent     bool           `json:"base_is_current"`
	Items             []ProposalItem `json:"items"`
}

// ProposalConfirmInput is the body of POST /profile-proposals/{id}/confirm:
// the base revision the person compared against.
type ProposalConfirmInput struct {
	BaseRevisionID string `json:"base_revision_id"`
}

// ProposalFilter narrows GET /profile-proposals.
type ProposalFilter struct {
	AccountID string
	State     ProposalState
}

// Todo is one stored todo revision.
type Todo struct {
	TodoID           string     `json:"todo_id"`
	Revision         int        `json:"revision"`
	Title            string     `json:"title"`
	Note             string     `json:"note"`
	AccountID        string     `json:"account_id"`
	OriginKind       TodoOrigin `json:"origin_kind"`
	OriginDecisionID string     `json:"origin_decision_id"`
	OriginReportID   string     `json:"origin_report_id"`
	OriginVersionNo  int        `json:"origin_version_no"`
	OriginGapKey     string     `json:"origin_gap_key"`
	State            TodoState  `json:"state"`
	Voided           bool       `json:"voided"`
	RecordedBy       string     `json:"recorded_by"`
	CreatedAt        time.Time  `json:"created_at"`
}

// GapTodoInput is the body of POST /todos: add one gap of a report version
// as a todo. The account is the gap's; an empty title is the gap key.
type GapTodoInput struct {
	OriginReportID  string `json:"origin_report_id"`
	OriginVersionNo int    `json:"origin_version_no"`
	OriginGapKey    string `json:"origin_gap_key"`
	Title           string `json:"title"`
	Note            string `json:"note"`
}

// TodoRevisionInput is the body of POST /todos/{id}/revisions. An absent
// member keeps the current value.
type TodoRevisionInput struct {
	State *TodoState `json:"state"`
	Title *string    `json:"title"`
	Note  *string    `json:"note"`
}

// TodoFilter narrows GET /todos.
type TodoFilter struct {
	AccountID string
	State     TodoState
}

// DiagnosisAnnotations is everything people wrote on one report version:
// the current revision of each judgement and suggestion, and every decision
// on those suggestions with its outcomes.
type DiagnosisAnnotations struct {
	ReportID    string             `json:"report_id"`
	VersionNo   int                `json:"version_no"`
	Judgements  []OpDiagJudgement  `json:"judgements"`
	Suggestions []OpDiagSuggestion `json:"suggestions"`
	Decisions   []DecisionView     `json:"decisions"`
}

const decisionColumns = `decision_id, suggestion_id, suggestion_revision, decision, mode, link_target_id, note,
	decided_by, created_at`

func scanDecision(row scanner) (SuggestionDecision, error) {
	var d SuggestionDecision
	var decision, mode string
	err := row.Scan(&d.DecisionID, &d.SuggestionID, &d.SuggestionRevision, &decision, &mode, &d.LinkTargetID,
		&d.Note, &d.DecidedBy, &d.CreatedAt)
	d.Decision, d.Mode, d.CreatedAt = DecisionKind(decision), AdoptMode(mode), d.CreatedAt.UTC()
	return d, err
}

const effectColumns = `effect_id, decision_id, outcome, target_kind, target_id, failure_code, created_at`

func scanEffect(row scanner) (Effect, error) {
	var e Effect
	var outcome, kind, failure string
	err := row.Scan(&e.EffectID, &e.DecisionID, &outcome, &kind, &e.TargetID, &failure, &e.CreatedAt)
	e.Outcome, e.TargetKind, e.FailureCode = EffectOutcome(outcome), SuggestionTarget(kind), EffectFailure(failure)
	e.CreatedAt = e.CreatedAt.UTC()
	return e, err
}

const proposalColumns = `proposal_id, revision, account_id, decision_id, base_revision_id, patches::text, state,
	applied_revision_id, voided, recorded_by, created_at`

func scanProposal(row scanner) (ProfileProposal, error) {
	var p ProfileProposal
	var patches, state string
	err := row.Scan(&p.ProposalID, &p.Revision, &p.AccountID, &p.DecisionID, &p.BaseRevisionID, &patches, &state,
		&p.AppliedRevisionID, &p.Voided, &p.RecordedBy, &p.CreatedAt)
	if err != nil {
		return p, err
	}
	if jsonErr := json.Unmarshal([]byte(patches), &p.Patches); jsonErr != nil {
		return p, ErrStorage
	}
	p.Patches, p.State, p.CreatedAt = nonNil(p.Patches), ProposalState(state), p.CreatedAt.UTC()
	return p, nil
}

const todoColumns = `todo_id, revision, title, note, account_id, origin_kind, origin_decision_id, origin_report_id,
	origin_version_no, origin_gap_key, state, voided, recorded_by, created_at`

func scanTodo(row scanner) (Todo, error) {
	var t Todo
	var origin, state string
	err := row.Scan(&t.TodoID, &t.Revision, &t.Title, &t.Note, &t.AccountID, &origin, &t.OriginDecisionID,
		&t.OriginReportID, &t.OriginVersionNo, &t.OriginGapKey, &state, &t.Voided, &t.RecordedBy, &t.CreatedAt)
	t.OriginKind, t.State, t.CreatedAt = TodoOrigin(origin), TodoState(state), t.CreatedAt.UTC()
	return t, err
}

// currentByID keeps the highest revision of each id; rows come ordered by
// id and revision.
func currentByID[T any](rows []T, id func(T) string) map[string]T {
	latest := map[string]T{}
	for _, row := range rows {
		latest[id(row)] = row
	}
	return latest
}

// ---------------------------------------------------------------- the topic card step

// topicCardKey is the idempotency key of an adopted topic card suggestion
// (Q3 supplement, contract §7.4): one per suggestion, whatever its revision,
// so a retry and a later revision's adoption find the same card.
func topicCardKey(suggestionID string) string {
	return "opdiag-suggestion:" + suggestionID
}

// effectFailureFor names why creating the card failed.
func effectFailureFor(err error) EffectFailure {
	switch {
	case errors.Is(err, ErrNotFound):
		return FailureTargetNotFound
	case errors.Is(err, ErrInvalid):
		return FailureTargetRefused
	default:
		return FailureStorage
	}
}

// createTopicCard is step ② of adopting into a topic card: CreateOnce with
// the suggestion's key and a draft of the revision that was decided. It
// answers the card, or the failure to record. It is the only place this
// module asks for a card to be created.
func (s *DiagnosisStore) createTopicCard(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion) (string, EffectFailure) {
	if s.TopicCards == nil {
		return "", FailureStorage
	}
	target := suggestion.target()
	cardID, _, err := s.TopicCards.CreateOnce(ctx, workspaceID, actor, topicCardKey(suggestion.SuggestionID),
		TopicCardDraft{AccountID: target.AccountID, IPFit: suggestion.Body})
	if err != nil {
		return "", effectFailureFor(err)
	}
	if cardID == "" {
		return "", FailureStorage
	}
	return cardID, ""
}

// ---------------------------------------------------------------- deciding

// DecideSuggestion records a person's decision on the current revision of a
// suggestion and, for an adoption, what it produced.
func (s *DiagnosisStore) DecideSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, in DecisionInput) (DecisionView, error) {
	if !s.ready() {
		return DecisionView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DecisionView{}, ErrInvalid
	}
	// Step 2: the suggestion is here.
	current, err := suggestionRevision(ctx, s.DB, workspaceID, suggestionID, 0)
	if err != nil {
		return DecisionView{}, err
	}
	// Steps 4 to 6.
	if err = validateDecision(in, current.TargetKind); err != nil {
		return DecisionView{}, err
	}
	// Step 8: the decision is on the revision the person read, which is
	// the current one, and not a voided one.
	if in.SuggestionRevision != current.Revision {
		return DecisionView{}, RevisionConflict{Field: "suggestion_revision"}
	}
	if current.Voided {
		return DecisionView{}, FieldError{Field: "suggestion_revision", Reason: "a voided suggestion revision takes no decision"}
	}
	if in.Decision == DecisionReject {
		return s.recordRejection(ctx, workspaceID, actor, current, in)
	}
	switch current.TargetKind {
	case TargetTodo:
		return s.adoptIntoTodo(ctx, workspaceID, actor, current, in)
	case TargetProfileProposal:
		return s.adoptIntoProposal(ctx, workspaceID, actor, current, in)
	case TargetTopicCard:
		if in.Mode == ModeLink {
			return s.adoptLinkingTopicCard(ctx, workspaceID, actor, current, in)
		}
		return s.adoptCreatingTopicCard(ctx, workspaceID, actor, current, in)
	default:
		return DecisionView{}, ErrStorage
	}
}

// validateDecision is decision steps 4 to 6: the controlled sets, then the
// mode and link target against the decision and the target kind, then the
// note.
func validateDecision(in DecisionInput, targetKind SuggestionTarget) error {
	if !oneOf(string(in.Decision), DecisionKinds) {
		return FieldError{Field: "decision", Reason: "not one of adopt, reject"}
	}
	if in.Mode != "" && !oneOf(string(in.Mode), AdoptModes) {
		return FieldError{Field: "mode", Reason: "not one of create, link"}
	}
	if in.SuggestionRevision < 1 {
		return FieldError{Field: "suggestion_revision", Reason: "required"}
	}
	if err := checkMode(in.Decision == DecisionAdopt && targetKind == TargetTopicCard, in.Mode, in.LinkTargetID); err != nil {
		return err
	}
	return checkRuneLimit("note", in.Note, MaxDecisionNoteRunes)
}

// checkMode holds mode and link_target_id to where they mean something:
// only adopting a topic card takes a mode, create takes no link target,
// link takes exactly one.
func checkMode(takesMode bool, mode AdoptMode, linkTargetID string) error {
	switch {
	case !takesMode && mode != "":
		return FieldError{Field: "mode", Reason: "only adopting a topic card suggestion takes a mode"}
	case !takesMode && linkTargetID != "":
		return FieldError{Field: "link_target_id", Reason: "only linking a topic card takes a link target"}
	case takesMode && mode == "":
		return FieldError{Field: "mode", Reason: "required: create or link"}
	case mode == ModeCreate && linkTargetID != "":
		return FieldError{Field: "link_target_id", Reason: "creating a card takes no link target"}
	case mode == ModeLink && strings.TrimSpace(linkTargetID) == "":
		return FieldError{Field: "link_target_id", Reason: "required to link a topic card"}
	}
	return nil
}

// recordRejection is the whole of a reject (FR-061, SC-006): the fence,
// the decision row, the audit entry. It calls no adapter - no card, no
// profile, no todo, no proposal, no outcome - and a guard holds its body to
// that.
func (s *DiagnosisStore) recordRejection(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, in DecisionInput) (DecisionView, error) {
	decision := s.newDecision(actor, suggestion, in)
	err := s.roi().inTx(ctx, workspaceID, actor, "reject-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		return decision.DecisionID, s.insertDecision(ctx, tx, workspaceID, &decision)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return viewOf(decision, suggestion.TargetKind, nil), nil
}

func (s *DiagnosisStore) newDecision(actor string, suggestion OpDiagSuggestion, in DecisionInput) SuggestionDecision {
	return SuggestionDecision{
		DecisionID: s.newID(), SuggestionID: suggestion.SuggestionID, SuggestionRevision: suggestion.Revision,
		Decision: in.Decision, Mode: in.Mode, LinkTargetID: in.LinkTargetID, Note: in.Note, DecidedBy: actor,
	}
}

func viewOf(decision SuggestionDecision, targetKind SuggestionTarget, effects []Effect) DecisionView {
	effects = nonNil(effects)
	return DecisionView{
		SuggestionDecision: decision, TargetKind: targetKind, Effects: effects,
		EffectState: effectState(decision.Decision, effects),
	}
}

// insertDecision is decision step 9 and the insert, inside the caller's
// fenced transaction: the suggestion revision is still the latest and has
// no decision yet. The unique index on (suggestion_id, suggestion_revision)
// turns a second decision that got past the read into the same 409.
func (s *DiagnosisStore) insertDecision(ctx context.Context, tx pgx.Tx, workspaceID string, d *SuggestionDecision) error {
	latest, err := suggestionRevision(ctx, tx, workspaceID, d.SuggestionID, 0)
	if err != nil {
		return err
	}
	if latest.Revision != d.SuggestionRevision {
		return RevisionConflict{Field: "suggestion_revision"}
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT decision_id FROM content_opdiag_decision
		WHERE workspace_id=$1 AND suggestion_id=$2 AND suggestion_revision=$3`,
		workspaceID, d.SuggestionID, d.SuggestionRevision).Scan(&existing)
	switch {
	case err == nil:
		return DecisionConflict{Field: "suggestion_id", Reason: "this suggestion revision is already decided"}
	case !errors.Is(err, pgx.ErrNoRows):
		return ErrStorage
	}
	if s.BeforeDecisionInsert != nil {
		s.BeforeDecisionInsert(ctx, d.SuggestionID)
	}
	err = tx.QueryRow(ctx, `INSERT INTO content_opdiag_decision
		(workspace_id, decision_id, suggestion_id, suggestion_revision, decision, mode, link_target_id, note, decided_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING created_at`,
		workspaceID, d.DecisionID, d.SuggestionID, d.SuggestionRevision, string(d.Decision), string(d.Mode),
		d.LinkTargetID, d.Note, d.DecidedBy).Scan(&d.CreatedAt)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return DecisionConflict{Field: "suggestion_id", Reason: "this suggestion revision is already decided"}
	}
	if err != nil {
		return ErrStorage
	}
	d.CreatedAt = d.CreatedAt.UTC()
	return nil
}

// insertEffect writes one outcome row inside the caller's transaction.
func (s *DiagnosisStore) insertEffect(ctx context.Context, tx pgx.Tx, workspaceID string, e *Effect) error {
	e.EffectID = s.newID()
	if err := tx.QueryRow(ctx, `INSERT INTO content_opdiag_effect
		(workspace_id, effect_id, decision_id, outcome, target_kind, target_id, failure_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING created_at`,
		workspaceID, e.EffectID, e.DecisionID, string(e.Outcome), string(e.TargetKind), e.TargetID,
		string(e.FailureCode)).Scan(&e.CreatedAt); err != nil {
		return ErrStorage
	}
	e.CreatedAt = e.CreatedAt.UTC()
	return nil
}

// adoptIntoTodo is one transaction (FR-064): the decision, revision 1 of an
// open todo, and a done outcome pointing at it.
func (s *DiagnosisStore) adoptIntoTodo(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, in DecisionInput) (DecisionView, error) {
	target := suggestion.target()
	if err := s.checkTargetAccount(ctx, workspaceID, target.AccountID); err != nil {
		return DecisionView{}, err
	}
	decision := s.newDecision(actor, suggestion, in)
	todo := Todo{
		TodoID: s.newID(), Revision: 1, Title: target.Title, AccountID: target.AccountID,
		OriginKind: TodoFromSuggestion, OriginDecisionID: decision.DecisionID, State: TodoOpen, RecordedBy: actor,
	}
	effect := Effect{DecisionID: decision.DecisionID, Outcome: OutcomeDone, TargetKind: TargetTodo, TargetID: todo.TodoID}
	err := s.roi().inTx(ctx, workspaceID, actor, "adopt-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := s.insertDecision(ctx, tx, workspaceID, &decision); err != nil {
			return decision.DecisionID, err
		}
		if err := insertTodo(ctx, tx, workspaceID, &todo); err != nil {
			return decision.DecisionID, err
		}
		return decision.DecisionID, s.insertEffect(ctx, tx, workspaceID, &effect)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return viewOf(decision, suggestion.TargetKind, []Effect{effect}), nil
}

// adoptIntoProposal is one transaction (FR-065): the decision, revision 1 of
// a proposal based on the account's current profile revision, and a done
// outcome. ip-profile is read, and not written: only a confirmation writes
// it (ConfirmProposal).
func (s *DiagnosisStore) adoptIntoProposal(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, in DecisionInput) (DecisionView, error) {
	target := suggestion.target()
	if err := s.checkTargetAccount(ctx, workspaceID, target.AccountID); err != nil {
		return DecisionView{}, err
	}
	profile, err := s.Accounts.CurrentProfile(ctx, workspaceID, target.AccountID)
	if err != nil {
		return DecisionView{}, adapterError(err)
	}
	decision := s.newDecision(actor, suggestion, in)
	proposal := ProfileProposal{
		ProposalID: s.newID(), Revision: 1, AccountID: target.AccountID, DecisionID: decision.DecisionID,
		BaseRevisionID: profile.RevisionID, Patches: target.Patches, State: ProposalProposed, RecordedBy: actor,
	}
	effect := Effect{DecisionID: decision.DecisionID, Outcome: OutcomeDone, TargetKind: TargetProfileProposal,
		TargetID: proposal.ProposalID}
	err = s.roi().inTx(ctx, workspaceID, actor, "adopt-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := s.insertDecision(ctx, tx, workspaceID, &decision); err != nil {
			return decision.DecisionID, err
		}
		if err := insertProposal(ctx, tx, workspaceID, &proposal); err != nil {
			return decision.DecisionID, err
		}
		return decision.DecisionID, s.insertEffect(ctx, tx, workspaceID, &effect)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return viewOf(decision, suggestion.TargetKind, []Effect{effect}), nil
}

// adoptLinkingTopicCard links an existing card (Q3=A): the card is checked,
// read only, to be here and for the suggestion's account; then the decision
// and a done outcome are one transaction.
func (s *DiagnosisStore) adoptLinkingTopicCard(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, in DecisionInput) (DecisionView, error) {
	if err := s.checkLinkTarget(ctx, workspaceID, actor, suggestion, in.LinkTargetID); err != nil {
		return DecisionView{}, err
	}
	decision := s.newDecision(actor, suggestion, in)
	effect := Effect{DecisionID: decision.DecisionID, Outcome: OutcomeDone, TargetKind: TargetTopicCard,
		TargetID: in.LinkTargetID}
	err := s.roi().inTx(ctx, workspaceID, actor, "adopt-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := s.insertDecision(ctx, tx, workspaceID, &decision); err != nil {
			return decision.DecisionID, err
		}
		return decision.DecisionID, s.insertEffect(ctx, tx, workspaceID, &effect)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return viewOf(decision, suggestion.TargetKind, []Effect{effect}), nil
}

// checkLinkTarget is decision step 2 for a link: the card is here and is
// for the suggestion's account. Anything else answers like a missing card.
func (s *DiagnosisStore) checkLinkTarget(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, cardID string) error {
	if s.TopicCards == nil {
		return ErrStorage
	}
	return adapterError(s.TopicCards.Exists(ctx, workspaceID, actor, cardID, suggestion.target().AccountID))
}

// adoptCreatingTopicCard is Q3=A's three steps: ① the decision, committed;
// ② the card, through CreateOnce under the suggestion's key; ③ the outcome,
// done with the card or failed with a code. When ③ itself cannot be
// written the decision stands with no outcome - "adopted, outcome not
// recorded" - and a retry converges on the same card.
func (s *DiagnosisStore) adoptCreatingTopicCard(ctx context.Context, workspaceID, actor string, suggestion OpDiagSuggestion, in DecisionInput) (DecisionView, error) {
	decision := s.newDecision(actor, suggestion, in)
	err := s.roi().inTx(ctx, workspaceID, actor, "adopt-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		return decision.DecisionID, s.insertDecision(ctx, tx, workspaceID, &decision)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return s.carryOutTopicCard(ctx, workspaceID, actor, decision, suggestion, ModeCreate, "")
}

// carryOutTopicCard is steps ② and ③ of a topic card adoption, for the
// adoption itself and for a retry. link checks the card; create creates it.
func (s *DiagnosisStore) carryOutTopicCard(ctx context.Context, workspaceID, actor string, decision SuggestionDecision,
	suggestion OpDiagSuggestion, mode AdoptMode, linkTargetID string) (DecisionView, error) {
	effect := Effect{DecisionID: decision.DecisionID, TargetKind: TargetTopicCard}
	if mode == ModeLink {
		if err := s.checkLinkTarget(ctx, workspaceID, actor, suggestion, linkTargetID); err != nil {
			return DecisionView{}, err
		}
		effect.Outcome, effect.TargetID = OutcomeDone, linkTargetID
	} else {
		cardID, failure := s.createTopicCard(ctx, workspaceID, actor, suggestion)
		if failure != "" {
			effect.Outcome, effect.FailureCode = OutcomeFailed, failure
		} else {
			effect.Outcome, effect.TargetID = OutcomeDone, cardID
		}
	}
	if s.BeforeEffectRecord != nil {
		if err := s.BeforeEffectRecord(ctx, decision.DecisionID); err != nil {
			return DecisionView{}, ErrStorage
		}
	}
	err := s.roi().inTx(ctx, workspaceID, actor, "record-effect", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := lockDecision(ctx, tx, workspaceID, decision.DecisionID); err != nil {
			return decision.DecisionID, err
		}
		effects, err := decisionEffects(ctx, tx, workspaceID, []string{decision.DecisionID})
		if err != nil {
			return decision.DecisionID, err
		}
		if effectState(DecisionAdopt, effects[decision.DecisionID]) == EffectReadsDone {
			return decision.DecisionID, DecisionConflict{Field: "decision_id", Reason: "this decision already has a done outcome"}
		}
		return decision.DecisionID, s.insertEffect(ctx, tx, workspaceID, &effect)
	})
	if err != nil {
		return DecisionView{}, err
	}
	return s.readDecisionView(ctx, workspaceID, decision.DecisionID)
}

// lockDecision serializes the outcome writes of one decision, so two
// retries at once record at most one done outcome.
func lockDecision(ctx context.Context, tx pgx.Tx, workspaceID, decisionID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"content-opdiag-decision:"+workspaceID+":"+decisionID); err != nil {
		return ErrStorage
	}
	return nil
}

// RetryDecision carries out an adopted topic card decision again (FR-063):
// allowed while it has no done outcome - failed, or none recorded at all.
// The mode may change to link; create asks CreateOnce with the same key,
// so a card an earlier attempt created is found, not duplicated.
func (s *DiagnosisStore) RetryDecision(ctx context.Context, workspaceID, actor, decisionID string, in RetryInput) (DecisionView, error) {
	if !s.ready() {
		return DecisionView{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DecisionView{}, ErrInvalid
	}
	view, err := s.readDecisionView(ctx, workspaceID, decisionID)
	if err != nil {
		return DecisionView{}, err
	}
	// A reject, and an adoption carried out in one transaction, have nothing
	// to retry.
	if view.Decision != DecisionAdopt || view.TargetKind != TargetTopicCard {
		return DecisionView{}, DecisionConflict{Field: "decision_id", Reason: "only an adopted topic card decision is retried"}
	}
	if in.Mode != "" && !oneOf(string(in.Mode), AdoptModes) {
		return DecisionView{}, FieldError{Field: "mode", Reason: "not one of create, link"}
	}
	mode := cmp.Or(in.Mode, view.Mode)
	if in.Mode == "" && mode == ModeLink && in.LinkTargetID == "" {
		in.LinkTargetID = view.LinkTargetID
	}
	if err = checkMode(true, mode, in.LinkTargetID); err != nil {
		return DecisionView{}, err
	}
	if view.EffectState == EffectReadsDone {
		return DecisionView{}, DecisionConflict{Field: "decision_id", Reason: "this decision already has a done outcome"}
	}
	suggestion, err := suggestionRevision(ctx, s.DB, workspaceID, view.SuggestionID, view.SuggestionRevision)
	if err != nil {
		return DecisionView{}, ErrStorage
	}
	return s.carryOutTopicCard(ctx, workspaceID, actor, view.SuggestionDecision, suggestion, mode, in.LinkTargetID)
}

// ---------------------------------------------------------------- reading decisions

// decisionEffects answers the outcomes of the given decisions, oldest first.
func decisionEffects(ctx context.Context, q opdiagQuerier, workspaceID string, decisionIDs []string) (map[string][]Effect, error) {
	byDecision := map[string][]Effect{}
	if len(decisionIDs) == 0 {
		return byDecision, nil
	}
	rows, err := q.Query(ctx, `SELECT `+effectColumns+` FROM content_opdiag_effect
		WHERE workspace_id=$1 AND decision_id = ANY($2) ORDER BY created_at, effect_id`, workspaceID, decisionIDs)
	if err != nil {
		return nil, ErrStorage
	}
	effects, err := collect(rows, scanEffect)
	if err != nil {
		return nil, err
	}
	for _, effect := range effects {
		byDecision[effect.DecisionID] = append(byDecision[effect.DecisionID], effect)
	}
	return byDecision, nil
}

// readDecisionView answers one decision with its outcomes.
func (s *DiagnosisStore) readDecisionView(ctx context.Context, workspaceID, decisionID string) (DecisionView, error) {
	decision, err := scanDecision(s.DB.QueryRow(ctx, `SELECT `+decisionColumns+` FROM content_opdiag_decision
		WHERE workspace_id=$1 AND decision_id=$2`, workspaceID, decisionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DecisionView{}, ErrNotFound
	}
	if err != nil {
		return DecisionView{}, ErrStorage
	}
	suggestion, err := suggestionRevision(ctx, s.DB, workspaceID, decision.SuggestionID, decision.SuggestionRevision)
	if err != nil {
		return DecisionView{}, ErrStorage
	}
	effects, err := decisionEffects(ctx, s.DB, workspaceID, []string{decisionID})
	if err != nil {
		return DecisionView{}, err
	}
	return viewOf(decision, suggestion.TargetKind, effects[decisionID]), nil
}

// Annotations answers everything written on one report version (GET
// .../annotations).
func (s *DiagnosisStore) Annotations(ctx context.Context, workspaceID, actor, reportID string, versionNo int) (DiagnosisAnnotations, error) {
	if !s.ready() {
		return DiagnosisAnnotations{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return DiagnosisAnnotations{}, ErrInvalid
	}
	if _, err := s.versionForAnnotation(ctx, workspaceID, reportID, versionNo); err != nil {
		return DiagnosisAnnotations{}, err
	}
	judgements, err := currentJudgements(ctx, s.DB, workspaceID, reportID, versionNo)
	if err != nil {
		return DiagnosisAnnotations{}, err
	}
	suggestions, err := currentSuggestions(ctx, s.DB, workspaceID, reportID, versionNo)
	if err != nil {
		return DiagnosisAnnotations{}, err
	}
	out := DiagnosisAnnotations{
		ReportID: reportID, VersionNo: versionNo,
		Judgements:  sortedValues(judgements, func(j OpDiagJudgement) (time.Time, string) { return j.CreatedAt, j.JudgementID }),
		Suggestions: sortedValues(suggestions, func(s OpDiagSuggestion) (time.Time, string) { return s.CreatedAt, s.SuggestionID }),
		Decisions:   []DecisionView{},
	}
	ids := make([]string, 0, len(suggestions))
	for id := range suggestions {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT `+decisionColumns+` FROM content_opdiag_decision
		WHERE workspace_id=$1 AND suggestion_id = ANY($2) ORDER BY created_at, decision_id`, workspaceID, ids)
	if err != nil {
		return DiagnosisAnnotations{}, ErrStorage
	}
	decisions, err := collect(rows, scanDecision)
	if err != nil {
		return DiagnosisAnnotations{}, err
	}
	decisionIDs := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		decisionIDs = append(decisionIDs, decision.DecisionID)
	}
	effects, err := decisionEffects(ctx, s.DB, workspaceID, decisionIDs)
	if err != nil {
		return DiagnosisAnnotations{}, err
	}
	for _, decision := range decisions {
		out.Decisions = append(out.Decisions,
			viewOf(decision, suggestions[decision.SuggestionID].TargetKind, effects[decision.DecisionID]))
	}
	return out, nil
}

// ---------------------------------------------------------------- proposals

func insertProposal(ctx context.Context, tx pgx.Tx, workspaceID string, p *ProfileProposal) error {
	patches, err := json.Marshal(nonNil(p.Patches))
	if err != nil {
		return ErrStorage
	}
	err = tx.QueryRow(ctx, `INSERT INTO content_opdiag_profile_proposal_revision
		(workspace_id, proposal_id, revision, account_id, decision_id, base_revision_id, patches, state,
		 applied_revision_id, voided, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11) RETURNING created_at`,
		workspaceID, p.ProposalID, p.Revision, p.AccountID, p.DecisionID, p.BaseRevisionID, string(patches),
		string(p.State), p.AppliedRevisionID, p.Voided, p.RecordedBy).Scan(&p.CreatedAt)
	if err != nil {
		return insertError(err)
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return nil
}

func latestProposal(ctx context.Context, q opdiagQuerier, workspaceID, proposalID string) (ProfileProposal, error) {
	proposal, err := scanProposal(q.QueryRow(ctx, `SELECT `+proposalColumns+` FROM content_opdiag_profile_proposal_revision
		WHERE workspace_id=$1 AND proposal_id=$2 ORDER BY revision DESC LIMIT 1`, workspaceID, proposalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ProfileProposal{}, ErrNotFound
	}
	if err != nil {
		return ProfileProposal{}, ErrStorage
	}
	return proposal, nil
}

// lockProposal serializes the state changes of one proposal.
func lockProposal(ctx context.Context, tx pgx.Tx, workspaceID, proposalID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"content-opdiag-proposal:"+workspaceID+":"+proposalID); err != nil {
		return ErrStorage
	}
	return nil
}

// ConfirmProposal is the explicit confirmation a proposal waits for (FR-066,
// ruling Q8=A). While the account's current profile revision is still the
// one the proposal was based on - and the one the person compared against -
// it writes a new ip-profile revision through DiagProfileWriter, every other
// item as it is and each patched item confirmed, then records the proposal
// confirmed with the new revision's id. Otherwise 409 base_revision_id and
// ip-profile is not written.
//
// Known limit (plan.md, Q8): between reading the current revision and
// ip-profile's write, somebody else's change could land; ip-profile's
// SetProfile takes no base revision. A follow-up card gives it one.
//
// The proposal's own state is held by a lock taken first, in a transaction
// of its own that holds no workspace fence, so a second confirmation waits
// and then finds it confirmed; the fence is only taken by the write that
// records the outcome, after ip-profile's own fenced write has finished.
func (s *DiagnosisStore) ConfirmProposal(ctx context.Context, workspaceID, actor, proposalID string, in ProposalConfirmInput) (ProfileProposal, error) {
	if !s.ready() {
		return ProfileProposal{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return ProfileProposal{}, ErrInvalid
	}
	proposal, err := latestProposal(ctx, s.DB, workspaceID, proposalID)
	if err != nil {
		return ProfileProposal{}, err
	}
	if err = s.checkTargetAccount(ctx, workspaceID, proposal.AccountID); err != nil {
		return ProfileProposal{}, err
	}
	if s.Profiles == nil {
		return ProfileProposal{}, ErrStorage
	}
	hold, err := s.DB.Begin(ctx)
	if err != nil {
		return ProfileProposal{}, ErrStorage
	}
	defer func() { _ = hold.Rollback(ctx) }()
	if err = lockProposal(ctx, hold, workspaceID, proposalID); err != nil {
		return ProfileProposal{}, err
	}
	if proposal, err = latestProposal(ctx, hold, workspaceID, proposalID); err != nil {
		return ProfileProposal{}, err
	}
	if proposal.State != ProposalProposed || proposal.Voided {
		return ProfileProposal{}, DecisionConflict{Field: "proposal_id", Reason: "this proposal is no longer proposed"}
	}
	// Step 8: the base the person compared against is the proposal's, and
	// the account's profile has not moved on since.
	if in.BaseRevisionID != proposal.BaseRevisionID {
		return ProfileProposal{}, RevisionConflict{Field: "base_revision_id"}
	}
	current, err := s.Profiles.CurrentProfileText(ctx, workspaceID, proposal.AccountID)
	if err != nil {
		return ProfileProposal{}, adapterError(err)
	}
	if current.RevisionID != proposal.BaseRevisionID {
		return ProfileProposal{}, RevisionConflict{Field: "base_revision_id"}
	}
	applied, err := s.Profiles.ApplyProfilePatches(ctx, workspaceID, actor, proposal.AccountID, proposal.BaseRevisionID, proposal.Patches)
	if err != nil {
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
			return ProfileProposal{}, err
		}
		return ProfileProposal{}, adapterError(err)
	}
	next := proposal
	next.Revision, next.State, next.AppliedRevisionID, next.RecordedBy = proposal.Revision+1, ProposalConfirmed, applied, actor
	err = s.roi().inTx(ctx, workspaceID, actor, "confirm-proposal", func(ctx context.Context, tx pgx.Tx) (string, error) {
		return proposalID, insertProposal(ctx, tx, workspaceID, &next)
	})
	if err != nil {
		return ProfileProposal{}, err
	}
	if err = hold.Commit(ctx); err != nil {
		return ProfileProposal{}, ErrStorage
	}
	return next, nil
}

// DismissProposal gives a proposal up (FR-066). ip-profile is not touched.
func (s *DiagnosisStore) DismissProposal(ctx context.Context, workspaceID, actor, proposalID string) (ProfileProposal, error) {
	if !s.ready() {
		return ProfileProposal{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return ProfileProposal{}, ErrInvalid
	}
	if _, err := latestProposal(ctx, s.DB, workspaceID, proposalID); err != nil {
		return ProfileProposal{}, err
	}
	var next ProfileProposal
	err := s.roi().inTx(ctx, workspaceID, actor, "dismiss-proposal", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if err := lockProposal(ctx, tx, workspaceID, proposalID); err != nil {
			return proposalID, err
		}
		current, err := latestProposal(ctx, tx, workspaceID, proposalID)
		if err != nil {
			return proposalID, err
		}
		if current.State != ProposalProposed || current.Voided {
			return proposalID, DecisionConflict{Field: "proposal_id", Reason: "this proposal is no longer proposed"}
		}
		next = current
		next.Revision, next.State, next.RecordedBy = current.Revision+1, ProposalDismissed, actor
		return proposalID, insertProposal(ctx, tx, workspaceID, &next)
	})
	if err != nil {
		return ProfileProposal{}, err
	}
	return next, nil
}

// ListProposals answers the current revision of each proposal with "current
// value -> proposed value" per item, read now through DiagProfileWriter.
func (s *DiagnosisStore) ListProposals(ctx context.Context, workspaceID, actor string, filter ProposalFilter) ([]ProposalView, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	if filter.State != "" && !oneOf(string(filter.State), ProposalStates) {
		return nil, FieldError{Field: "state", Reason: "not one of proposed, confirmed, dismissed"}
	}
	rows, err := s.DB.Query(ctx, `SELECT `+proposalColumns+` FROM content_opdiag_profile_proposal_revision
		WHERE workspace_id=$1 AND ($2='' OR account_id=$2) ORDER BY proposal_id, revision`, workspaceID, filter.AccountID)
	if err != nil {
		return nil, ErrStorage
	}
	all, err := collect(rows, scanProposal)
	if err != nil {
		return nil, err
	}
	current := currentByID(all, func(p ProfileProposal) string { return p.ProposalID })
	profiles := map[string]DiagProfileText{}
	views := []ProposalView{}
	for _, proposal := range sortedValues(current, func(p ProfileProposal) (time.Time, string) { return p.CreatedAt, p.ProposalID }) {
		if proposal.Voided || (filter.State != "" && proposal.State != filter.State) {
			continue
		}
		profile, ok := profiles[proposal.AccountID]
		if !ok {
			if s.Profiles == nil {
				return nil, ErrStorage
			}
			profile, err = s.Profiles.CurrentProfileText(ctx, workspaceID, proposal.AccountID)
			switch {
			case errors.Is(err, ErrNotFound):
				profile = DiagProfileText{}
			case err != nil:
				return nil, ErrStorage
			}
			profiles[proposal.AccountID] = profile
		}
		view := ProposalView{
			ProfileProposal: proposal, CurrentRevisionID: profile.RevisionID,
			BaseIsCurrent: profile.RevisionID == proposal.BaseRevisionID, Items: []ProposalItem{},
		}
		for _, patch := range proposal.Patches {
			view.Items = append(view.Items, ProposalItem{
				Field: patch.Field, CurrentValue: profile.Values[patch.Field],
				CurrentStatus: profile.Status[patch.Field], ProposedValue: patch.Value,
			})
		}
		views = append(views, view)
	}
	return views, nil
}

// ---------------------------------------------------------------- todos

func insertTodo(ctx context.Context, tx pgx.Tx, workspaceID string, t *Todo) error {
	err := tx.QueryRow(ctx, `INSERT INTO content_opdiag_todo_revision
		(workspace_id, todo_id, revision, title, note, account_id, origin_kind, origin_decision_id,
		 origin_report_id, origin_version_no, origin_gap_key, state, voided, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING created_at`,
		workspaceID, t.TodoID, t.Revision, t.Title, t.Note, t.AccountID, string(t.OriginKind), t.OriginDecisionID,
		t.OriginReportID, t.OriginVersionNo, t.OriginGapKey, string(t.State), t.Voided, t.RecordedBy).Scan(&t.CreatedAt)
	if err != nil {
		return insertError(err)
	}
	t.CreatedAt = t.CreatedAt.UTC()
	return nil
}

func latestTodo(ctx context.Context, q opdiagQuerier, workspaceID, todoID string) (Todo, error) {
	todo, err := scanTodo(q.QueryRow(ctx, `SELECT `+todoColumns+` FROM content_opdiag_todo_revision
		WHERE workspace_id=$1 AND todo_id=$2 ORDER BY revision DESC LIMIT 1`, workspaceID, todoID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	if err != nil {
		return Todo{}, ErrStorage
	}
	return todo, nil
}

// AddGapTodo adds one gap of a report version as a todo (FR-032). The gap
// must be in that version's gap list; a gap that already has a todo which
// is not voided answers 409 origin_gap_key, so pressing "加入待办" twice
// makes one.
func (s *DiagnosisStore) AddGapTodo(ctx context.Context, workspaceID, actor string, in GapTodoInput) (Todo, error) {
	if !s.ready() {
		return Todo{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return Todo{}, ErrInvalid
	}
	facts, err := s.versionForAnnotation(ctx, workspaceID, in.OriginReportID, in.OriginVersionNo)
	if err != nil {
		return Todo{}, err
	}
	if strings.TrimSpace(in.OriginGapKey) == "" {
		return Todo{}, FieldError{Field: "origin_gap_key", Reason: "required"}
	}
	title := cmp.Or(in.Title, in.OriginGapKey)
	if err = checkRequiredText("title", title, MaxTodoTitleRunes); err != nil {
		return Todo{}, err
	}
	if err = checkRuneLimit("note", in.Note, MaxTodoNoteRunes); err != nil {
		return Todo{}, err
	}
	gapIndex := slices.IndexFunc(facts.gaps, func(gap DiagnosisGap) bool { return gap.GapKey == in.OriginGapKey })
	if gapIndex < 0 {
		return Todo{}, FieldError{Field: "origin_gap_key", Reason: "not a gap of this report version"}
	}
	todo := Todo{
		TodoID: s.newID(), Revision: 1, Title: title, Note: in.Note, AccountID: facts.gaps[gapIndex].AccountID,
		OriginKind: TodoFromDataGap, OriginReportID: in.OriginReportID, OriginVersionNo: in.OriginVersionNo,
		OriginGapKey: in.OriginGapKey, State: TodoOpen, RecordedBy: actor,
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "add-todo", func(ctx context.Context, tx pgx.Tx) (string, error) {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"content-opdiag-gap:"+workspaceID+":"+in.OriginReportID+":"+itoa(in.OriginVersionNo)+":"+in.OriginGapKey); err != nil {
			return todo.TodoID, ErrStorage
		}
		rows, err := tx.Query(ctx, `SELECT `+todoColumns+` FROM content_opdiag_todo_revision
			WHERE workspace_id=$1 AND origin_report_id=$2 AND origin_version_no=$3 AND origin_gap_key=$4
			ORDER BY todo_id, revision`, workspaceID, in.OriginReportID, in.OriginVersionNo, in.OriginGapKey)
		if err != nil {
			return todo.TodoID, ErrStorage
		}
		existing, err := collect(rows, scanTodo)
		if err != nil {
			return todo.TodoID, err
		}
		for _, current := range currentByID(existing, func(t Todo) string { return t.TodoID }) {
			if !current.Voided {
				return todo.TodoID, DecisionConflict{Field: "origin_gap_key", Reason: "this gap already has a todo"}
			}
		}
		return todo.TodoID, insertTodo(ctx, tx, workspaceID, &todo)
	})
	if err != nil {
		return Todo{}, err
	}
	return todo, nil
}

// ReviseTodo writes the next revision of a todo: a state, title or note
// change, or a void. Where it came from never changes.
func (s *DiagnosisStore) ReviseTodo(ctx context.Context, workspaceID, actor, todoID string, in TodoRevisionInput, revision Revision) (Todo, error) {
	if !s.ready() {
		return Todo{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return Todo{}, ErrInvalid
	}
	previous, err := latestTodo(ctx, s.DB, workspaceID, todoID)
	if err != nil {
		return Todo{}, err
	}
	next := previous
	next.Voided, next.RecordedBy = revision.Voided, actor
	if in.State != nil {
		if !oneOf(string(*in.State), TodoStates) {
			return Todo{}, FieldError{Field: "state", Reason: "not one of open, done, dropped"}
		}
		next.State = *in.State
	}
	if in.Title != nil {
		if err = checkRequiredText("title", *in.Title, MaxTodoTitleRunes); err != nil {
			return Todo{}, err
		}
		next.Title = *in.Title
	}
	if in.Note != nil {
		if err = checkRuneLimit("note", *in.Note, MaxTodoNoteRunes); err != nil {
			return Todo{}, err
		}
		next.Note = *in.Note
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "revise-todo", func(ctx context.Context, tx pgx.Tx) (string, error) {
		latest, readErr := latestTodo(ctx, tx, workspaceID, todoID)
		if readErr != nil {
			return todoID, readErr
		}
		if err := checkBase(revision, latest.Revision); err != nil {
			return todoID, err
		}
		next.Revision = latest.Revision + 1
		return todoID, insertTodo(ctx, tx, workspaceID, &next)
	})
	if err != nil {
		return Todo{}, err
	}
	return next, nil
}

// ListTodos answers the current revision of each todo that is not voided,
// newest first.
func (s *DiagnosisStore) ListTodos(ctx context.Context, workspaceID, actor string, filter TodoFilter) ([]Todo, error) {
	if !s.ready() {
		return nil, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return nil, ErrInvalid
	}
	if filter.State != "" && !oneOf(string(filter.State), TodoStates) {
		return nil, FieldError{Field: "state", Reason: "not one of open, done, dropped"}
	}
	rows, err := s.DB.Query(ctx, `SELECT `+todoColumns+` FROM content_opdiag_todo_revision
		WHERE workspace_id=$1 ORDER BY todo_id, revision`, workspaceID)
	if err != nil {
		return nil, ErrStorage
	}
	all, err := collect(rows, scanTodo)
	if err != nil {
		return nil, err
	}
	todos := []Todo{}
	for _, todo := range currentByID(all, func(t Todo) string { return t.TodoID }) {
		if todo.Voided || (filter.State != "" && todo.State != filter.State) ||
			(filter.AccountID != "" && todo.AccountID != filter.AccountID) {
			continue
		}
		todos = append(todos, todo)
	}
	slices.SortFunc(todos, func(left, right Todo) int {
		// The first revision's time is not kept on the current one, so the
		// order is by the current revision's time: most recently touched
		// first.
		return cmp.Or(right.CreatedAt.Compare(left.CreatedAt), cmp.Compare(left.TodoID, right.TodoID))
	})
	return todos, nil
}
