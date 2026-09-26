package feedbacklearning

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Judgements and suggestions a person writes on one operating diagnosis
// report version (specs/035 PR 3: FR-050 to FR-054; contract §1.3, §1.4,
// §5.8, §7.3).
//
// R-057 asks for "AI 判断、替代解释、局限" and suggestions. The model runner
// stays disabled, so in this version every one of them is written by a
// person: author_kind is exactly human. A judgement either cites facts of
// that version's result by their reference keys, or says it is qualitative
// and cites none - the page then says there is no data behind it. A
// suggestion names what adopting it would produce: a topic card, a todo, or
// a proposal to change an account's profile. There is no fourth kind, and
// in particular no business memory (FR-068).
//
// Both are revisioned. A change, or a void, is the next revision of the same
// id with the base revision the writer read; an earlier revision is never
// changed. A decision is taken on one revision (opdiag_decisions.go).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §1.3,
// §1.4, §7

// MaxAnnotationBodyRunes bounds a judgement's or a suggestion's text
// (contract §1.3, §1.4).
const MaxAnnotationBodyRunes = 5000

// MaxProfilePatchValueRunes bounds a proposed profile item's text: the same
// bound ip-profile keeps (MaxProfileFieldRunes), copied rather than imported.
const MaxProfilePatchValueRunes = 20000

// MaxProfilePatches is the most items one proposal changes: the eight text
// items, each at most once.
const MaxProfilePatches = 8

// JudgementKind is R-057's "判断、替代解释、局限" (FR-050).
type JudgementKind string

const (
	JudgementKindJudgement   JudgementKind = "judgement"
	JudgementKindAlternative JudgementKind = "alternative_explanation"
	JudgementKindLimitation  JudgementKind = "limitation"
)

var JudgementKinds = []JudgementKind{JudgementKindJudgement, JudgementKindAlternative, JudgementKindLimitation}

// JudgementBasis is whether a judgement cites facts of the result
// (evidence) or none (qualitative, R-057 "允许定性分析") (FR-051).
type JudgementBasis string

const (
	BasisEvidence    JudgementBasis = "evidence"
	BasisQualitative JudgementBasis = "qualitative"
)

var JudgementBases = []JudgementBasis{BasisEvidence, BasisQualitative}

// AuthorKind is who wrote a judgement or suggestion. Exactly one value in
// this version (FR-053): the AI judgement layer is a later card, and adding
// its value needs a migration that widens the CHECK - on purpose.
type AuthorKind string

const AuthorHuman AuthorKind = "human"

var AuthorKinds = []AuthorKind{AuthorHuman}

// SuggestionTarget is what adopting a suggestion produces: R-057's "账号配置
// 修改、选题或待办", exactly three (FR-052). No business memory.
type SuggestionTarget string

const (
	TargetTopicCard       SuggestionTarget = "topic_card"
	TargetTodo            SuggestionTarget = "todo"
	TargetProfileProposal SuggestionTarget = "profile_proposal"
)

var SuggestionTargets = []SuggestionTarget{TargetTopicCard, TargetTodo, TargetProfileProposal}

// ---------------------------------------------------------------- judgements

// JudgementInput is the body of a new judgement, and of a revision together
// with its Revision (base_revision, voided).
type JudgementInput struct {
	Kind             JudgementKind  `json:"kind"`
	Basis            JudgementBasis `json:"basis"`
	EvidenceRefs     []string       `json:"evidence_refs"`
	AboutJudgementID string         `json:"about_judgement_id"`
	Body             string         `json:"body"`
}

// OpDiagJudgement is one stored revision.
type OpDiagJudgement struct {
	JudgementID      string         `json:"judgement_id"`
	Revision         int            `json:"revision"`
	ReportID         string         `json:"report_id"`
	VersionNo        int            `json:"version_no"`
	Kind             JudgementKind  `json:"kind"`
	Basis            JudgementBasis `json:"basis"`
	EvidenceRefs     []string       `json:"evidence_refs"`
	AboutJudgementID string         `json:"about_judgement_id"`
	Body             string         `json:"body"`
	AuthorKind       AuthorKind     `json:"author_kind"`
	Voided           bool           `json:"voided"`
	RecordedBy       string         `json:"recorded_by"`
	CreatedAt        time.Time      `json:"created_at"`
}

const judgementColumns = `judgement_id, revision, report_id, version_no, kind, basis, evidence_refs,
	about_judgement_id, body, author_kind, voided, recorded_by, created_at`

func scanJudgement(row scanner) (OpDiagJudgement, error) {
	var j OpDiagJudgement
	var kind, basis, author string
	err := row.Scan(&j.JudgementID, &j.Revision, &j.ReportID, &j.VersionNo, &kind, &basis, &j.EvidenceRefs,
		&j.AboutJudgementID, &j.Body, &author, &j.Voided, &j.RecordedBy, &j.CreatedAt)
	j.Kind, j.Basis, j.AuthorKind = JudgementKind(kind), JudgementBasis(basis), AuthorKind(author)
	j.EvidenceRefs = nonNil(j.EvidenceRefs)
	j.CreatedAt = j.CreatedAt.UTC()
	return j, err
}

// ---------------------------------------------------------------- suggestions

// SuggestionInput is the body of a new suggestion, and of a revision
// together with its Revision. Target is the target kind's own parameters
// (contract §7.3), decoded strictly by that kind.
type SuggestionInput struct {
	Body         string           `json:"body"`
	TargetKind   SuggestionTarget `json:"target_kind"`
	Target       json.RawMessage  `json:"target"`
	JudgementIDs []string         `json:"judgement_ids"`
	EvidenceRefs []string         `json:"evidence_refs"`
}

// ProfilePatch is one proposed profile item: one of the eight text items
// and its proposed text.
type ProfilePatch struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// SuggestionTargetParams is a target as stored, whatever its kind: the
// account (all three kinds), the todo title (todo), the patches (profile
// proposal). Only the kind's own members are ever set.
type SuggestionTargetParams struct {
	AccountID string         `json:"account_id"`
	Title     string         `json:"title,omitempty"`
	Patches   []ProfilePatch `json:"patches,omitempty"`
}

// OpDiagSuggestion is one stored revision.
type OpDiagSuggestion struct {
	SuggestionID string           `json:"suggestion_id"`
	Revision     int              `json:"revision"`
	ReportID     string           `json:"report_id"`
	VersionNo    int              `json:"version_no"`
	Body         string           `json:"body"`
	TargetKind   SuggestionTarget `json:"target_kind"`
	Target       json.RawMessage  `json:"target"`
	JudgementIDs []string         `json:"judgement_ids"`
	EvidenceRefs []string         `json:"evidence_refs"`
	AuthorKind   AuthorKind       `json:"author_kind"`
	Voided       bool             `json:"voided"`
	RecordedBy   string           `json:"recorded_by"`
	CreatedAt    time.Time        `json:"created_at"`
}

// target decodes the stored target. The stored JSON was written by
// decodeSuggestionTarget, so it always decodes.
func (s OpDiagSuggestion) target() SuggestionTargetParams {
	var params SuggestionTargetParams
	_ = json.Unmarshal(s.Target, &params)
	return params
}

const suggestionColumns = `suggestion_id, revision, report_id, version_no, body, target_kind, target::text,
	judgement_ids, evidence_refs, author_kind, voided, recorded_by, created_at`

func scanSuggestion(row scanner) (OpDiagSuggestion, error) {
	var s OpDiagSuggestion
	var kind, target, author string
	err := row.Scan(&s.SuggestionID, &s.Revision, &s.ReportID, &s.VersionNo, &s.Body, &kind, &target,
		&s.JudgementIDs, &s.EvidenceRefs, &author, &s.Voided, &s.RecordedBy, &s.CreatedAt)
	s.TargetKind, s.AuthorKind, s.Target = SuggestionTarget(kind), AuthorKind(author), json.RawMessage(target)
	s.JudgementIDs, s.EvidenceRefs = nonNil(s.JudgementIDs), nonNil(s.EvidenceRefs)
	s.CreatedAt = s.CreatedAt.UTC()
	return s, err
}

// ---------------------------------------------------------------- validation

// versionFacts is what a report version offers a judgement or a suggestion
// to point at: its reference keys (contract §5.8) and its gaps.
type versionFacts struct {
	refs []string
	gaps []DiagnosisGap
}

func versionFactsOf(version DiagnosisReportVersion) (versionFacts, error) {
	var result struct {
		Refs []string       `json:"refs"`
		Gaps []DiagnosisGap `json:"gaps"`
	}
	if err := json.Unmarshal(version.Result, &result); err != nil {
		return versionFacts{}, ErrStorage
	}
	return versionFacts{refs: result.Refs, gaps: result.Gaps}, nil
}

// checkRequiredText refuses a blank text and an over-long one.
func checkRequiredText(field, value string, limit int) error {
	if strings.TrimSpace(value) == "" {
		return FieldError{Field: field, Reason: "required"}
	}
	return checkRuneLimit(field, value, limit)
}

// checkRefs is decision step 7 for evidence_refs: every key is one of the
// version's reference keys, and none is given twice.
func checkRefs(field string, refs []string, facts versionFacts) error {
	seen := map[string]bool{}
	for _, ref := range refs {
		if !slices.Contains(facts.refs, ref) {
			return FieldError{Field: field, Reason: "not a reference key of this report version"}
		}
		if seen[ref] {
			return FieldError{Field: field, Reason: "listed twice"}
		}
		seen[ref] = true
	}
	return nil
}

// validateJudgement is decision steps 4 to 7 on a judgement: the controlled
// sets, then basis against evidence_refs and about_judgement_id against the
// kind, then the text, then the references. others are the current
// judgements of the same version, by id.
func validateJudgement(in JudgementInput, facts versionFacts, self string, others map[string]OpDiagJudgement) error {
	if !oneOf(string(in.Kind), JudgementKinds) {
		return FieldError{Field: "kind", Reason: "not one of judgement, alternative_explanation, limitation"}
	}
	if !oneOf(string(in.Basis), JudgementBases) {
		return FieldError{Field: "basis", Reason: "not one of evidence, qualitative"}
	}
	switch {
	case in.Basis == BasisEvidence && len(in.EvidenceRefs) == 0:
		return FieldError{Field: "evidence_refs", Reason: "an evidence judgement cites at least one fact"}
	case in.Basis == BasisQualitative && len(in.EvidenceRefs) > 0:
		return FieldError{Field: "evidence_refs", Reason: "a qualitative judgement cites no fact"}
	case in.AboutJudgementID != "" && in.Kind != JudgementKindAlternative:
		return FieldError{Field: "about_judgement_id", Reason: "only an alternative explanation points at a judgement"}
	}
	if err := checkRequiredText("body", in.Body, MaxAnnotationBodyRunes); err != nil {
		return err
	}
	if err := checkRefs("evidence_refs", in.EvidenceRefs, facts); err != nil {
		return err
	}
	if in.AboutJudgementID != "" {
		about, ok := others[in.AboutJudgementID]
		if !ok || about.Voided || in.AboutJudgementID == self {
			return FieldError{Field: "about_judgement_id", Reason: "not a judgement of this report version"}
		}
	}
	return nil
}

// strictDecode decodes one JSON object refusing unknown members; an error
// names the member, prefixed.
func strictDecode(raw json.RawMessage, prefix string, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		return nil
	}
	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok && typeErr.Field != "" {
		return FieldError{Field: prefix + "." + typeErr.Field, Reason: "wrong JSON type"}
	}
	const unknown = `json: unknown field "`
	if text := err.Error(); strings.HasPrefix(text, unknown) && strings.HasSuffix(text, `"`) {
		name := strings.TrimSuffix(strings.TrimPrefix(text, unknown), `"`)
		if name != "" && len(name) <= 64 && !strings.ContainsFunc(name, func(r rune) bool {
			return !(r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		}) {
			return FieldError{Field: prefix + "." + name, Reason: "unknown field"}
		}
	}
	return FieldError{Field: prefix, Reason: "not an object of this target kind"}
}

// decodeSuggestionTarget decodes a target by its kind (contract §7.3),
// refusing a member another kind takes, and answers it as stored.
func decodeSuggestionTarget(kind SuggestionTarget, raw json.RawMessage) (SuggestionTargetParams, error) {
	switch kind {
	case TargetTopicCard:
		var target struct {
			AccountID string `json:"account_id"`
		}
		err := strictDecode(raw, "target", &target)
		return SuggestionTargetParams{AccountID: target.AccountID}, err
	case TargetTodo:
		var target struct {
			Title     string `json:"title"`
			AccountID string `json:"account_id"`
		}
		if err := strictDecode(raw, "target", &target); err != nil {
			return SuggestionTargetParams{}, err
		}
		if err := checkRequiredText("target.title", target.Title, MaxTodoTitleRunes); err != nil {
			return SuggestionTargetParams{}, err
		}
		return SuggestionTargetParams{AccountID: target.AccountID, Title: target.Title}, nil
	case TargetProfileProposal:
		var target struct {
			AccountID string         `json:"account_id"`
			Patches   []ProfilePatch `json:"patches"`
		}
		if err := strictDecode(raw, "target", &target); err != nil {
			return SuggestionTargetParams{}, err
		}
		if strings.TrimSpace(target.AccountID) == "" {
			return SuggestionTargetParams{}, FieldError{Field: "target.account_id", Reason: "required for a profile proposal"}
		}
		if err := checkProfilePatches(target.Patches); err != nil {
			return SuggestionTargetParams{}, err
		}
		return SuggestionTargetParams{AccountID: target.AccountID, Patches: target.Patches}, nil
	default:
		return SuggestionTargetParams{}, FieldError{Field: "target_kind", Reason: "not one of topic_card, todo, profile_proposal"}
	}
}

// checkProfilePatches holds a proposal to the eight text items (FR-067):
// the persona prompt, the channel list, the weekly hours and the style
// samples are not proposable, and nothing is proposed twice.
func checkProfilePatches(patches []ProfilePatch) error {
	if len(patches) == 0 || len(patches) > MaxProfilePatches {
		return FieldError{Field: "target.patches", Reason: "one to eight items"}
	}
	seen := map[string]bool{}
	for _, patch := range patches {
		if !slices.Contains(profileTextKeys, patch.Field) {
			return FieldError{Field: "target.patches.field", Reason: "not one of the eight text items of an expression profile"}
		}
		if seen[patch.Field] {
			return FieldError{Field: "target.patches.field", Reason: "listed twice"}
		}
		seen[patch.Field] = true
		if utf8.RuneCountInString(patch.Value) > MaxProfilePatchValueRunes {
			return FieldError{Field: "target.patches.value", Reason: "too long"}
		}
	}
	return nil
}

// validateSuggestion is decision steps 4 to 7 on a suggestion, apart from
// the account's existence, which the store asks before it: the target kind
// and its parameters, the text, then the judgements and references, which
// must belong to the same report version.
func validateSuggestion(in SuggestionInput, facts versionFacts, judgements map[string]OpDiagJudgement) (SuggestionTargetParams, error) {
	if !oneOf(string(in.TargetKind), SuggestionTargets) {
		return SuggestionTargetParams{}, FieldError{Field: "target_kind", Reason: "not one of topic_card, todo, profile_proposal"}
	}
	target, err := decodeSuggestionTarget(in.TargetKind, in.Target)
	if err != nil {
		return SuggestionTargetParams{}, err
	}
	if err = checkRequiredText("body", in.Body, MaxAnnotationBodyRunes); err != nil {
		return SuggestionTargetParams{}, err
	}
	seen := map[string]bool{}
	for _, id := range in.JudgementIDs {
		judgement, ok := judgements[id]
		if !ok || judgement.Voided {
			return SuggestionTargetParams{}, FieldError{Field: "judgement_ids", Reason: "not a judgement of this report version"}
		}
		if seen[id] {
			return SuggestionTargetParams{}, FieldError{Field: "judgement_ids", Reason: "listed twice"}
		}
		seen[id] = true
	}
	if err = checkRefs("evidence_refs", in.EvidenceRefs, facts); err != nil {
		return SuggestionTargetParams{}, err
	}
	return target, nil
}

// ---------------------------------------------------------------- reads

// opdiagQuerier is the database or an open transaction.
type opdiagQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// currentJudgements answers the latest revision of every judgement on one
// report version, by id.
func currentJudgements(ctx context.Context, q opdiagQuerier, workspaceID, reportID string, versionNo int) (map[string]OpDiagJudgement, error) {
	rows, err := q.Query(ctx, `SELECT `+judgementColumns+` FROM content_opdiag_judgement_revision
		WHERE workspace_id=$1 AND report_id=$2 AND version_no=$3 ORDER BY judgement_id, revision`,
		workspaceID, reportID, versionNo)
	if err != nil {
		return nil, ErrStorage
	}
	all, err := collect(rows, scanJudgement)
	if err != nil {
		return nil, err
	}
	current := map[string]OpDiagJudgement{}
	for _, judgement := range all {
		current[judgement.JudgementID] = judgement
	}
	return current, nil
}

// latestJudgement answers the latest revision of one judgement.
func latestJudgement(ctx context.Context, q opdiagQuerier, workspaceID, judgementID string) (OpDiagJudgement, error) {
	judgement, err := scanJudgement(q.QueryRow(ctx, `SELECT `+judgementColumns+` FROM content_opdiag_judgement_revision
		WHERE workspace_id=$1 AND judgement_id=$2 ORDER BY revision DESC LIMIT 1`, workspaceID, judgementID))
	if errors.Is(err, pgx.ErrNoRows) {
		return OpDiagJudgement{}, ErrNotFound
	}
	if err != nil {
		return OpDiagJudgement{}, ErrStorage
	}
	return judgement, nil
}

// currentSuggestions answers the latest revision of every suggestion on one
// report version, by id.
func currentSuggestions(ctx context.Context, q opdiagQuerier, workspaceID, reportID string, versionNo int) (map[string]OpDiagSuggestion, error) {
	rows, err := q.Query(ctx, `SELECT `+suggestionColumns+` FROM content_opdiag_suggestion_revision
		WHERE workspace_id=$1 AND report_id=$2 AND version_no=$3 ORDER BY suggestion_id, revision`,
		workspaceID, reportID, versionNo)
	if err != nil {
		return nil, ErrStorage
	}
	all, err := collect(rows, scanSuggestion)
	if err != nil {
		return nil, err
	}
	current := map[string]OpDiagSuggestion{}
	for _, suggestion := range all {
		current[suggestion.SuggestionID] = suggestion
	}
	return current, nil
}

// suggestionRevision answers one revision of a suggestion; revision 0 is
// the latest.
func suggestionRevision(ctx context.Context, q opdiagQuerier, workspaceID, suggestionID string, revision int) (OpDiagSuggestion, error) {
	query := `SELECT ` + suggestionColumns + ` FROM content_opdiag_suggestion_revision
		WHERE workspace_id=$1 AND suggestion_id=$2`
	args := []any{workspaceID, suggestionID}
	if revision > 0 {
		query += ` AND revision=$3`
		args = append(args, revision)
	} else {
		query += ` ORDER BY revision DESC LIMIT 1`
	}
	suggestion, err := scanSuggestion(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return OpDiagSuggestion{}, ErrNotFound
	}
	if err != nil {
		return OpDiagSuggestion{}, ErrStorage
	}
	return suggestion, nil
}

// sortedValues answers a map's values ordered by created_at, then id.
func sortedValues[T any](items map[string]T, key func(T) (time.Time, string)) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	slices.SortFunc(out, func(left, right T) int {
		leftAt, leftID := key(left)
		rightAt, rightID := key(right)
		return cmp.Or(leftAt.Compare(rightAt), cmp.Compare(leftID, rightID))
	})
	return out
}

// versionForAnnotation is decision step 2 for a path naming a report
// version: it is here, and it answers what a judgement may point at.
func (s *DiagnosisStore) versionForAnnotation(ctx context.Context, workspaceID, reportID string, versionNo int) (versionFacts, error) {
	if reportID == "" || versionNo < 1 {
		return versionFacts{}, ErrNotFound
	}
	version, err := s.readDiagnosisVersion(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return versionFacts{}, err
	}
	return versionFactsOf(version)
}

// ---------------------------------------------------------------- judgement writes

// RecordJudgement writes revision 1 of a new judgement on a report version.
func (s *DiagnosisStore) RecordJudgement(ctx context.Context, workspaceID, actor, reportID string, versionNo int, in JudgementInput) (OpDiagJudgement, error) {
	if !s.ready() {
		return OpDiagJudgement{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return OpDiagJudgement{}, ErrInvalid
	}
	facts, err := s.versionForAnnotation(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return OpDiagJudgement{}, err
	}
	others, err := currentJudgements(ctx, s.DB, workspaceID, reportID, versionNo)
	if err != nil {
		return OpDiagJudgement{}, err
	}
	if err = validateJudgement(in, facts, "", others); err != nil {
		return OpDiagJudgement{}, err
	}
	judgement := OpDiagJudgement{
		JudgementID: s.newID(), Revision: 1, ReportID: reportID, VersionNo: versionNo, Kind: in.Kind,
		Basis: in.Basis, EvidenceRefs: nonNil(in.EvidenceRefs), AboutJudgementID: in.AboutJudgementID,
		Body: in.Body, AuthorKind: AuthorHuman, RecordedBy: actor,
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "record-judgement", func(ctx context.Context, tx pgx.Tx) (string, error) {
		return judgement.JudgementID, insertJudgement(ctx, tx, workspaceID, &judgement)
	})
	if err != nil {
		return OpDiagJudgement{}, err
	}
	return judgement, nil
}

// ReviseJudgement writes the next revision of a judgement: new text, or a
// void. It stays on the report version it was written on.
func (s *DiagnosisStore) ReviseJudgement(ctx context.Context, workspaceID, actor, judgementID string, in JudgementInput, revision Revision) (OpDiagJudgement, error) {
	if !s.ready() {
		return OpDiagJudgement{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return OpDiagJudgement{}, ErrInvalid
	}
	previous, err := latestJudgement(ctx, s.DB, workspaceID, judgementID)
	if err != nil {
		return OpDiagJudgement{}, err
	}
	next := previous
	next.Voided, next.RecordedBy = revision.Voided, actor
	if !revision.Voided {
		facts, factsErr := s.versionForAnnotation(ctx, workspaceID, previous.ReportID, previous.VersionNo)
		if factsErr != nil {
			return OpDiagJudgement{}, factsErr
		}
		others, readErr := currentJudgements(ctx, s.DB, workspaceID, previous.ReportID, previous.VersionNo)
		if readErr != nil {
			return OpDiagJudgement{}, readErr
		}
		if err = validateJudgement(in, facts, judgementID, others); err != nil {
			return OpDiagJudgement{}, err
		}
		next.Kind, next.Basis, next.EvidenceRefs = in.Kind, in.Basis, nonNil(in.EvidenceRefs)
		next.AboutJudgementID, next.Body = in.AboutJudgementID, in.Body
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "revise-judgement", func(ctx context.Context, tx pgx.Tx) (string, error) {
		latest, readErr := latestJudgement(ctx, tx, workspaceID, judgementID)
		if readErr != nil {
			return judgementID, readErr
		}
		if err := checkBase(revision, latest.Revision); err != nil {
			return judgementID, err
		}
		next.Revision = latest.Revision + 1
		return judgementID, insertJudgement(ctx, tx, workspaceID, &next)
	})
	if err != nil {
		return OpDiagJudgement{}, err
	}
	return next, nil
}

func insertJudgement(ctx context.Context, tx pgx.Tx, workspaceID string, j *OpDiagJudgement) error {
	err := tx.QueryRow(ctx, `INSERT INTO content_opdiag_judgement_revision
		(workspace_id, judgement_id, revision, report_id, version_no, kind, basis, evidence_refs,
		 about_judgement_id, body, author_kind, voided, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING created_at`,
		workspaceID, j.JudgementID, j.Revision, j.ReportID, j.VersionNo, string(j.Kind), string(j.Basis),
		nonNil(j.EvidenceRefs), j.AboutJudgementID, j.Body, string(AuthorHuman), j.Voided, j.RecordedBy).Scan(&j.CreatedAt)
	if err != nil {
		return insertError(err)
	}
	j.AuthorKind, j.CreatedAt = AuthorHuman, j.CreatedAt.UTC()
	return nil
}

// ---------------------------------------------------------------- suggestion writes

// checkTargetAccount is decision step 2 for a target: the account it names
// is here. An empty account is allowed for a topic card and a todo.
func (s *DiagnosisStore) checkTargetAccount(ctx context.Context, workspaceID, accountID string) error {
	if accountID == "" {
		return nil
	}
	if s.Accounts == nil {
		return ErrStorage
	}
	return adapterError(s.Accounts.AccountExists(ctx, workspaceID, accountID))
}

// prepareSuggestion checks a suggestion against its report version and
// answers its target as stored.
func (s *DiagnosisStore) prepareSuggestion(ctx context.Context, workspaceID, reportID string, versionNo int, in SuggestionInput) (json.RawMessage, error) {
	facts, err := s.versionForAnnotation(ctx, workspaceID, reportID, versionNo)
	if err != nil {
		return nil, err
	}
	judgements, err := currentJudgements(ctx, s.DB, workspaceID, reportID, versionNo)
	if err != nil {
		return nil, err
	}
	target, err := validateSuggestion(in, facts, judgements)
	if err != nil {
		return nil, err
	}
	if err = s.checkTargetAccount(ctx, workspaceID, target.AccountID); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(target)
	if err != nil {
		return nil, ErrStorage
	}
	return encoded, nil
}

// RecordSuggestion writes revision 1 of a new suggestion on a report
// version.
func (s *DiagnosisStore) RecordSuggestion(ctx context.Context, workspaceID, actor, reportID string, versionNo int, in SuggestionInput) (OpDiagSuggestion, error) {
	if !s.ready() {
		return OpDiagSuggestion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return OpDiagSuggestion{}, ErrInvalid
	}
	target, err := s.prepareSuggestion(ctx, workspaceID, reportID, versionNo, in)
	if err != nil {
		return OpDiagSuggestion{}, err
	}
	suggestion := OpDiagSuggestion{
		SuggestionID: s.newID(), Revision: 1, ReportID: reportID, VersionNo: versionNo, Body: in.Body,
		TargetKind: in.TargetKind, Target: target, JudgementIDs: nonNil(in.JudgementIDs),
		EvidenceRefs: nonNil(in.EvidenceRefs), AuthorKind: AuthorHuman, RecordedBy: actor,
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "record-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		return suggestion.SuggestionID, insertSuggestion(ctx, tx, workspaceID, &suggestion)
	})
	if err != nil {
		return OpDiagSuggestion{}, err
	}
	return suggestion, nil
}

// ReviseSuggestion writes the next revision of a suggestion: new text or
// target, or a void. A decision taken on an earlier revision stays on it.
func (s *DiagnosisStore) ReviseSuggestion(ctx context.Context, workspaceID, actor, suggestionID string, in SuggestionInput, revision Revision) (OpDiagSuggestion, error) {
	if !s.ready() {
		return OpDiagSuggestion{}, ErrStorage
	}
	if workspaceID == "" || actor == "" {
		return OpDiagSuggestion{}, ErrInvalid
	}
	previous, err := suggestionRevision(ctx, s.DB, workspaceID, suggestionID, 0)
	if err != nil {
		return OpDiagSuggestion{}, err
	}
	next := previous
	next.Voided, next.RecordedBy = revision.Voided, actor
	if !revision.Voided {
		target, prepareErr := s.prepareSuggestion(ctx, workspaceID, previous.ReportID, previous.VersionNo, in)
		if prepareErr != nil {
			return OpDiagSuggestion{}, prepareErr
		}
		next.Body, next.TargetKind, next.Target = in.Body, in.TargetKind, target
		next.JudgementIDs, next.EvidenceRefs = nonNil(in.JudgementIDs), nonNil(in.EvidenceRefs)
	}
	err = s.roi().inTx(ctx, workspaceID, actor, "revise-suggestion", func(ctx context.Context, tx pgx.Tx) (string, error) {
		latest, readErr := suggestionRevision(ctx, tx, workspaceID, suggestionID, 0)
		if readErr != nil {
			return suggestionID, readErr
		}
		if err := checkBase(revision, latest.Revision); err != nil {
			return suggestionID, err
		}
		next.Revision = latest.Revision + 1
		return suggestionID, insertSuggestion(ctx, tx, workspaceID, &next)
	})
	if err != nil {
		return OpDiagSuggestion{}, err
	}
	return next, nil
}

func insertSuggestion(ctx context.Context, tx pgx.Tx, workspaceID string, s *OpDiagSuggestion) error {
	err := tx.QueryRow(ctx, `INSERT INTO content_opdiag_suggestion_revision
		(workspace_id, suggestion_id, revision, report_id, version_no, body, target_kind, target,
		 judgement_ids, evidence_refs, author_kind, voided, recorded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13) RETURNING created_at`,
		workspaceID, s.SuggestionID, s.Revision, s.ReportID, s.VersionNo, s.Body, string(s.TargetKind),
		string(s.Target), nonNil(s.JudgementIDs), nonNil(s.EvidenceRefs), string(AuthorHuman), s.Voided,
		s.RecordedBy).Scan(&s.CreatedAt)
	if err != nil {
		return insertError(err)
	}
	s.AuthorKind, s.CreatedAt = AuthorHuman, s.CreatedAt.UTC()
	return nil
}
