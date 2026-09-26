package topicplanning

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"golang.org/x/text/unicode/norm"
)

// Platform search optimization (specs/036 PR 1): search themes.
//
// A search theme is what a person gathered about how people search for one
// subject on one platform: the questions they ask, the keywords and keyword
// groups, the intent a person judged, where it all came from, and which
// materials, topic cards and briefs it relates to (R-060 item 1). Nothing
// here is fetched, estimated or scored: search volume and competition have no
// data source in this version, so they are not stored and every read says
// "unknown" (FR-014, D2).
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

// SearchIntent is a person's judgement of what someone searching wants (Q7).
// The system never infers it from the questions or keywords (FR-019).
type SearchIntent string

const (
	IntentLearn        SearchIntent = "learn"
	IntentSolve        SearchIntent = "solve"
	IntentCompare      SearchIntent = "compare"
	IntentBuy          SearchIntent = "buy"
	IntentFind         SearchIntent = "find"
	IntentUnclassified SearchIntent = "unclassified"
)

// SearchIntents is the controlled set, exactly six (contract §3).
var SearchIntents = []SearchIntent{IntentLearn, IntentSolve, IntentCompare, IntentBuy, IntentFind, IntentUnclassified}

// ThemeOrigin is where a theme's questions and keywords came from: R-060
// "人工输入关键词、已有客户提问及授权素材". There is no research or online
// origin in this version (FR-002).
type ThemeOrigin string

const (
	OriginManualKeyword      ThemeOrigin = "manual_keyword"
	OriginCustomerQuestion   ThemeOrigin = "customer_question"
	OriginAuthorizedMaterial ThemeOrigin = "authorized_material"
)

// ThemeOrigins is the controlled set, exactly three.
var ThemeOrigins = []ThemeOrigin{OriginManualKeyword, OriginCustomerQuestion, OriginAuthorizedMaterial}

// SearchDataOrigin says who put the data here. This version has people only.
type SearchDataOrigin string

const DataOriginManualOnly SearchDataOrigin = "manual_only"

// DataOrigins is the controlled set, exactly one (FR-003).
var DataOrigins = []SearchDataOrigin{DataOriginManualOnly}

// UnknownReason is why a value is unknown. Unknown is always said with a
// reason, never as 0, "" or a missing field (FR-080).
type UnknownReason string

const UnknownNoDataSource UnknownReason = "no_data_source"

// UnknownReasons is the controlled set of reasons a theme read gives,
// exactly one in PR 1.
var UnknownReasons = []UnknownReason{UnknownNoDataSource}

// Limits from contract §1.1.
const (
	MaxThemeNameRunes      = 100
	MaxThemeTextRunes      = 2000
	MaxThemeQuestions      = 50
	MaxThemeQuestionRunes  = 500
	MaxThemeKeywords       = 100
	MaxThemeKeywordRunes   = 100
	MaxThemeReferenceIDs   = 50
	maxSearchThemeBodySize = 1 << 20
)

// SearchConflict is a write that arrived against state that has moved on:
// a base_revision that is no longer the current one, or the second writer of
// the same revision number. Nothing was written. Field is the JSON name the
// 409 points at.
type SearchConflict struct {
	Field string
}

func (e SearchConflict) Error() string { return "stale " + e.Field }

// ThemeContent is everything a person authors on a theme; each revision
// stores a complete copy. It is also the request shape, so a field that is
// not here - search_volume, competition, rank, scope, budget - is refused by
// name when a request carries it (FR-002, FR-014).
type ThemeContent struct {
	Name             string       `json:"name"`
	Platform         string       `json:"platform"`
	AccountID        string       `json:"account_id"`
	BusinessGoal     string       `json:"business_goal"`
	Questions        []string     `json:"questions"`
	Keywords         []string     `json:"keywords"`
	Intent           SearchIntent `json:"intent"`
	Origin           ThemeOrigin  `json:"origin"`
	OriginNote       string       `json:"origin_note"`
	SourceIDs        []string     `json:"source_ids"`
	TopicCardIDs     []string     `json:"topic_card_ids"`
	BriefRevisionIDs []string     `json:"brief_revision_ids"`
	Note             string       `json:"note"`
}

// SearchTheme is one stored revision.
type SearchTheme struct {
	ThemeID  string `json:"theme_id"`
	Revision int64  `json:"revision"`
	Voided   bool   `json:"voided"`
	ThemeContent
	RecordedBy string    `json:"recorded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// SearchUnknown is a value this version cannot know.
type SearchUnknown struct {
	Status string        `json:"status"`
	Reason UnknownReason `json:"reason"`
}

// SearchThemeView is a theme as it is answered: the stored revision plus
// what is derived on read. search_volume and competition are always unknown
// for want of a data source; there is no rank here at all - that comes from
// feedback-learning's observations, joined on the page (contract §4.1).
type SearchThemeView struct {
	SearchTheme
	SearchVolume SearchUnknown    `json:"search_volume"`
	Competition  SearchUnknown    `json:"competition"`
	DataOrigin   SearchDataOrigin `json:"data_origin"`
}

// ViewTheme derives the read-only fields.
func ViewTheme(theme SearchTheme) SearchThemeView {
	unknown := SearchUnknown{Status: "unknown", Reason: UnknownNoDataSource}
	return SearchThemeView{SearchTheme: theme, SearchVolume: unknown, Competition: unknown, DataOrigin: DataOriginManualOnly}
}

// ThemeRevisionRequest is a whole new revision of an existing theme.
type ThemeRevisionRequest struct {
	BaseRevision int64
	Voided       bool
	Content      ThemeContent
}

// ThemeFilter is the list's query (FR-015). An empty field does not filter.
type ThemeFilter struct {
	Platform        string
	AccountID       string
	TopicCardID     string
	IncludeArchived bool
}

// searchServerWritten are fields the server writes. A body may carry them -
// a client echoing a theme back must not fail on them - and they are
// dropped, never used (contract §4). search_volume and competition are not
// among them: they are refused, so nobody can believe a number was taken.
type searchServerWritten struct {
	RecordedBy json.RawMessage `json:"recorded_by"`
	DataOrigin json.RawMessage `json:"data_origin"`
}

type themeCreateWire struct {
	ThemeContent
	searchServerWritten
}

type themeRevisionWire struct {
	ThemeContent
	searchServerWritten
	BaseRevision *int64 `json:"base_revision"`
	Voided       bool   `json:"voided"`
}

// DecodeThemeCreate reads a create body strictly.
func DecodeThemeCreate(data []byte) (ThemeContent, error) {
	var wire themeCreateWire
	if err := decodeSearchStrict(data, &wire); err != nil {
		return ThemeContent{}, err
	}
	return wire.ThemeContent, nil
}

// DecodeThemeRevision reads a revision body strictly. base_revision is
// required.
func DecodeThemeRevision(data []byte) (ThemeRevisionRequest, error) {
	var wire themeRevisionWire
	if err := decodeSearchStrict(data, &wire); err != nil {
		return ThemeRevisionRequest{}, err
	}
	if wire.BaseRevision == nil || *wire.BaseRevision < 1 {
		return ThemeRevisionRequest{}, FieldError{Field: "base_revision", Reason: "required"}
	}
	return ThemeRevisionRequest{BaseRevision: *wire.BaseRevision, Voided: wire.Voided, Content: wire.ThemeContent}, nil
}

// decodeSearchStrict decodes exactly one JSON value with no unknown members.
// An unknown member is refused by its own name, a wrongly typed value by its
// JSON path; nothing else about the request, and never a Go type name,
// reaches the answer. The same rule as handler/content_roi_records.go
// jsonFieldPath and handler/content_opdiag_reports.go unknownJSONField,
// restated here because a module cannot import the handler.
func decodeSearchStrict(data []byte, target any) error {
	return decodeSearchStrictLimit(data, target, maxSearchThemeBodySize)
}

// decodeSearchStrictLimit is decodeSearchStrict with the size bound given:
// a suggestion carries a whole document body (PR 2).
func decodeSearchStrictLimit(data []byte, target any, limit int) error {
	if len(data) > limit {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			if field := searchJSONFieldPath(typeErr.Field); field != "" {
				return FieldError{Field: field, Reason: "wrong JSON type"}
			}
		}
		if field := searchUnknownJSONField(err); field != "" {
			return FieldError{Field: field, Reason: "unknown field"}
		}
		return ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// searchJSONFieldPath drops the Go names encoding/json writes into a path
// for embedded structs ("ThemeContent.name"). Every JSON name here is
// snake_case, so a segment with an upper-case letter is a Go name.
func searchJSONFieldPath(field string) string {
	kept := []string{}
	for _, segment := range strings.Split(field, ".") {
		if segment == "" || strings.IndexFunc(segment, unicode.IsUpper) >= 0 {
			continue
		}
		kept = append(kept, segment)
	}
	return strings.Join(kept, ".")
}

// searchUnknownJSONField reads the member name out of `json: unknown field
// "rank"`. Anything that does not look like a JSON key is not echoed.
func searchUnknownJSONField(err error) string {
	const prefix = `json: unknown field "`
	text := err.Error()
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, `"`) {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimPrefix(text, prefix), `"`)
	if name == "" || len(name) > 64 || strings.ContainsFunc(name, func(r rune) bool {
		return !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		return ""
	}
	return name
}

func validPlatform(value string) bool {
	for _, platform := range ipprofile.Platforms {
		if string(platform) == value {
			return true
		}
	}
	return false
}

func validIntent(value SearchIntent) bool {
	for _, intent := range SearchIntents {
		if intent == value {
			return true
		}
	}
	return false
}

func validOrigin(value ThemeOrigin) bool {
	for _, origin := range ThemeOrigins {
		if origin == value {
			return true
		}
	}
	return false
}

// NormalizeSearchTerms is FR-017: NFC, trim, drop empty entries, and drop a
// later entry equal to an earlier one after that. Nothing else - no case
// folding, no synonyms, no splitting: "Cashmere" and "cashmere" stay two.
func NormalizeSearchTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		term := strings.TrimSpace(norm.NFC.String(value))
		if term == "" {
			continue
		}
		if _, dup := seen[term]; dup {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}

// normalizeThemeIDs trims, drops empty and repeated ids, and caps the list.
func normalizeThemeIDs(field string, values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > MaxThemeReferenceIDs {
		return nil, FieldError{Field: field, Reason: "at most 50 ids"}
	}
	return out, nil
}

func termsTooLong(values []string, limit int) bool {
	for _, value := range values {
		if utf8.RuneCountInString(value) > limit {
			return true
		}
	}
	return false
}

// NormalizeThemeContent checks a theme's shape and returns it in stored
// form. It is pure: whether the account, materials, topic cards and briefs
// are this brand's is checked inside the write transaction (FR-013).
func NormalizeThemeContent(c ThemeContent) (ThemeContent, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || tooLong(c.Name, MaxThemeNameRunes) {
		return ThemeContent{}, FieldError{Field: "name", Reason: "1-100 characters"}
	}
	if !validPlatform(c.Platform) {
		return ThemeContent{}, FieldError{Field: "platform", Reason: "unknown platform"}
	}
	c.AccountID = strings.TrimSpace(c.AccountID)
	if tooLong(c.BusinessGoal, MaxThemeTextRunes) {
		return ThemeContent{}, FieldError{Field: "business_goal", Reason: "too long"}
	}
	c.Questions = NormalizeSearchTerms(c.Questions)
	if len(c.Questions) > MaxThemeQuestions || termsTooLong(c.Questions, MaxThemeQuestionRunes) {
		return ThemeContent{}, FieldError{Field: "questions", Reason: "at most 50 questions of 500 characters"}
	}
	c.Keywords = NormalizeSearchTerms(c.Keywords)
	if len(c.Keywords) > MaxThemeKeywords || termsTooLong(c.Keywords, MaxThemeKeywordRunes) {
		return ThemeContent{}, FieldError{Field: "keywords", Reason: "at most 100 keywords of 100 characters"}
	}
	if len(c.Questions) == 0 && len(c.Keywords) == 0 {
		return ThemeContent{}, FieldError{Field: "questions", Reason: "a question or a keyword is required"}
	}
	if !validIntent(c.Intent) {
		return ThemeContent{}, FieldError{Field: "intent", Reason: "unknown intent"}
	}
	if !validOrigin(c.Origin) {
		return ThemeContent{}, FieldError{Field: "origin", Reason: "unknown origin"}
	}
	if tooLong(c.OriginNote, MaxThemeTextRunes) {
		return ThemeContent{}, FieldError{Field: "origin_note", Reason: "too long"}
	}
	if c.Origin == OriginCustomerQuestion && strings.TrimSpace(c.OriginNote) == "" {
		return ThemeContent{}, FieldError{Field: "origin_note", Reason: "say where the customer questions came from"}
	}
	var err error
	if c.SourceIDs, err = normalizeThemeIDs("source_ids", c.SourceIDs); err != nil {
		return ThemeContent{}, err
	}
	if c.Origin == OriginAuthorizedMaterial && len(c.SourceIDs) == 0 {
		return ThemeContent{}, FieldError{Field: "source_ids", Reason: "an authorized material is required"}
	}
	if c.TopicCardIDs, err = normalizeThemeIDs("topic_card_ids", c.TopicCardIDs); err != nil {
		return ThemeContent{}, err
	}
	if c.BriefRevisionIDs, err = normalizeThemeIDs("brief_revision_ids", c.BriefRevisionIDs); err != nil {
		return ThemeContent{}, err
	}
	if tooLong(c.Note, MaxThemeTextRunes) {
		return ThemeContent{}, FieldError{Field: "note", Reason: "too long"}
	}
	return c, nil
}

// ParseThemeFilter reads the list query. A platform outside the set is
// refused by name rather than answered with an empty list.
func ParseThemeFilter(platform, accountID, topicCardID, includeArchived string) (ThemeFilter, error) {
	if platform != "" && !validPlatform(platform) {
		return ThemeFilter{}, FieldError{Field: "platform", Reason: "unknown platform"}
	}
	return ThemeFilter{
		Platform: platform, AccountID: accountID, TopicCardID: topicCardID,
		IncludeArchived: includeArchived == "true",
	}, nil
}

// ---------------------------------------------------------------------------
// Search optimization suggestions (specs/036 PR 2): suggestions, comparison
// and abandoning.
//
// A suggestion is a whole proposed body for one version of one document,
// aimed at one question of one search theme, with the reason for it (R-060
// item 3). It is written by a person (D1): nothing here generates, scores or
// ranks one. Its difference from the base version is computed on read, and so
// is its state. A person may abandon it - which changes nothing anywhere but
// the decision table (FR-051) - and, from PR 3, adopt it.

// SuggestionAspect labels what a suggestion changes. All four are text in
// the document body (Q4): a suggestion never touches a work or document
// title or any unversioned field (FR-032).
type SuggestionAspect string

const (
	AspectTitle       SuggestionAspect = "title"
	AspectBody        SuggestionAspect = "body"
	AspectTopics      SuggestionAspect = "topics"
	AspectDescription SuggestionAspect = "description"
)

// SuggestionAspects is the controlled set, exactly four (R-060 item 3).
var SuggestionAspects = []SuggestionAspect{AspectTitle, AspectBody, AspectTopics, AspectDescription}

// AuthorKind is who wrote a suggestion. This version has people only (D1);
// an AI author is a later card and a migration.
type AuthorKind string

const AuthorHuman AuthorKind = "human"

// AuthorKinds is the controlled set, exactly one.
var AuthorKinds = []AuthorKind{AuthorHuman}

// SuggestionDecision is what a person decided about a suggestion (FR-050).
type SuggestionDecision string

const (
	DecisionAdopt   SuggestionDecision = "adopt"
	DecisionAbandon SuggestionDecision = "abandon"
)

// SuggestionDecisions is the controlled set, exactly two. PR 2 accepts
// abandon only; adopt is refused by name until PR 3 opens it.
var SuggestionDecisions = []SuggestionDecision{DecisionAdopt, DecisionAbandon}

// EffectOutcome is what one attempt at adopting did.
type EffectOutcome string

const (
	EffectDone   EffectOutcome = "done"
	EffectFailed EffectOutcome = "failed"
)

// EffectOutcomes is the controlled set, exactly two.
var EffectOutcomes = []EffectOutcome{EffectDone, EffectFailed}

// EffectFailure is why an adoption attempt failed (FR-053).
type EffectFailure string

const (
	FailureBaseMoved      EffectFailure = "base_moved"
	FailureDraftUnsaved   EffectFailure = "draft_unsaved"
	FailureNoChange       EffectFailure = "no_change"
	FailureTargetNotFound EffectFailure = "target_not_found"
	FailureStorage        EffectFailure = "storage"
)

// EffectFailures is the controlled set, exactly five.
var EffectFailures = []EffectFailure{FailureBaseMoved, FailureDraftUnsaved, FailureNoChange, FailureTargetNotFound, FailureStorage}

// SuggestionState is derived on every read and never stored (FR-038,
// contract §5).
type SuggestionState string

const (
	StateOpen            SuggestionState = "open"
	StateAdopted         SuggestionState = "adopted"
	StateAdoptFailed     SuggestionState = "adopt_failed"
	StateAdoptUnrecorded SuggestionState = "adopt_unrecorded"
	StateAbandoned       SuggestionState = "abandoned"
)

// SuggestionStates is the controlled set, exactly five.
var SuggestionStates = []SuggestionState{StateOpen, StateAdopted, StateAdoptFailed, StateAdoptUnrecorded, StateAbandoned}

// Limits from contract §1.2 and §1.3.
const (
	MaxTargetQuestionRunes = 500
	MaxRationaleRunes      = 5000
	// MaxProposedBodyRunes is work-editor's MaxBodyRunes: a proposed body the
	// document could not hold is not a proposal. Restated, not imported,
	// because topic-planning does not import work-editor (FR-101).
	MaxProposedBodyRunes = 200000
	MaxDecisionNoteRunes = 2000
	MinComparedIDs       = 2
	MaxComparedIDs       = 4
	// maxSearchSuggestionBodySize bounds a suggestion request: a whole body
	// of 200000 characters, each possibly escaped as \uXXXX, and the rest.
	maxSearchSuggestionBodySize = 4 << 20
)

// SuggestionTarget is where a suggestion points: one version of one document
// of one work, and one search theme. The same on every revision of a
// suggestion (FR-030, FR-031): a different base is a new suggestion.
type SuggestionTarget struct {
	WorkID        string `json:"work_id"`
	ArtifactID    string `json:"artifact_id"`
	BaseVersionID string `json:"base_version_id"`
	ThemeID       string `json:"theme_id"`
}

// SuggestionContent is what a person writes on a suggestion; each revision
// stores a whole copy.
type SuggestionContent struct {
	TargetQuestion    string             `json:"target_question"`
	Aspects           []SuggestionAspect `json:"aspects"`
	Rationale         string             `json:"rationale"`
	EvidenceSourceIDs []string           `json:"evidence_source_ids"`
	ProposedBody      string             `json:"proposed_body"`
}

// SearchSuggestion is one stored revision. ThemeRevision and AuthorKind are
// written by the server.
type SearchSuggestion struct {
	SuggestionID string `json:"suggestion_id"`
	Revision     int64  `json:"revision"`
	SuggestionTarget
	ThemeRevision int64 `json:"theme_revision"`
	SuggestionContent
	AuthorKind AuthorKind `json:"author_kind"`
	RecordedBy string     `json:"recorded_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// SuggestionDecisionRecord is the one decision on a suggestion.
type SuggestionDecisionRecord struct {
	DecisionID         string             `json:"decision_id"`
	SuggestionID       string             `json:"suggestion_id"`
	SuggestionRevision int64              `json:"suggestion_revision"`
	Decision           SuggestionDecision `json:"decision"`
	Note               string             `json:"note"`
	DecidedBy          string             `json:"decided_by"`
	CreatedAt          time.Time          `json:"created_at"`
}

// SuggestionEffect is one attempt at carrying out an adoption. Rows are
// written from PR 3; PR 2 reads them to derive a suggestion's state.
type SuggestionEffect struct {
	EffectID    string        `json:"effect_id"`
	DecisionID  string        `json:"decision_id"`
	Outcome     EffectOutcome `json:"outcome"`
	VersionID   string        `json:"version_id"`
	FailureCode EffectFailure `json:"failure_code"`
	CreatedAt   time.Time     `json:"created_at"`
}

// SuggestionView is a suggestion as it is answered: the current revision
// plus what is derived on read. Diff is given for a single suggestion and in
// a comparison, not in a list. There is no score, rank, keyword count or
// recommendation here, and nothing that could hold one (FR-036).
type SuggestionView struct {
	SearchSuggestion
	State SuggestionState `json:"state"`
	// FailureCode is the latest failed attempt's code, for adopt_failed only.
	FailureCode   EffectFailure             `json:"failure_code,omitempty"`
	BaseIsCurrent bool                      `json:"base_is_current"`
	ThemeChanged  bool                      `json:"theme_changed"`
	Diff          *Diff                     `json:"diff,omitempty"`
	Decision      *SuggestionDecisionRecord `json:"decision"`
	Effects       []SuggestionEffect        `json:"effects"`
}

// SuggestionComparison puts two to four suggestions side by side, ordered by
// created_at, then suggestion_id - never by any judgement (FR-040).
type SuggestionComparison struct {
	Suggestions []SuggestionView `json:"suggestions"`
	// SameBase is true when every suggestion compared has the same base
	// version.
	SameBase bool `json:"same_base"`
}

// DeriveSuggestionState is contract §5: no decision is open; abandon is
// abandoned; adopt with a done effect is adopted, with no effect yet is
// adopt_unrecorded, and with the latest effect failed is adopt_failed (and
// that effect's code). effects are in the order they were written.
func DeriveSuggestionState(decision *SuggestionDecisionRecord, effects []SuggestionEffect) (SuggestionState, EffectFailure) {
	switch {
	case decision == nil:
		return StateOpen, ""
	case decision.Decision == DecisionAbandon:
		return StateAbandoned, ""
	}
	for _, effect := range effects {
		if effect.Outcome == EffectDone {
			return StateAdopted, ""
		}
	}
	if len(effects) == 0 {
		return StateAdoptUnrecorded, ""
	}
	return StateAdoptFailed, effects[len(effects)-1].FailureCode
}

// SuggestionRequest is a create body.
type SuggestionRequest struct {
	Target  SuggestionTarget
	Content SuggestionContent
}

// SuggestionRevisionRequest is a revision body. Target fields may be
// repeated; one that differs from the suggestion's own is refused by name.
type SuggestionRevisionRequest struct {
	BaseRevision int64
	Target       SuggestionTarget
	Content      SuggestionContent
}

// DecisionRequest is a decision body. Revision is the suggestion revision
// the person saw.
type DecisionRequest struct {
	Decision SuggestionDecision
	Revision int64
	Note     string
}

// suggestionServerWritten are fields the server writes. A client echoing a
// suggestion back may carry them; they are dropped, never used (contract §4).
type suggestionServerWritten struct {
	RecordedBy    json.RawMessage `json:"recorded_by"`
	ThemeRevision json.RawMessage `json:"theme_revision"`
	AuthorKind    json.RawMessage `json:"author_kind"`
}

type suggestionCreateWire struct {
	SuggestionTarget
	SuggestionContent
	suggestionServerWritten
}

type suggestionRevisionWire struct {
	SuggestionTarget
	SuggestionContent
	suggestionServerWritten
	BaseRevision *int64 `json:"base_revision"`
}

type decisionWire struct {
	Decision SuggestionDecision `json:"decision"`
	Revision *int64             `json:"revision"`
	Note     string             `json:"note"`
}

// DecodeSuggestionCreate reads a create body strictly: an unknown member -
// a score, a rank, a keyword count - is refused by its own name.
func DecodeSuggestionCreate(data []byte) (SuggestionRequest, error) {
	var wire suggestionCreateWire
	if err := decodeSearchStrictLimit(data, &wire, maxSearchSuggestionBodySize); err != nil {
		return SuggestionRequest{}, err
	}
	return SuggestionRequest{Target: wire.SuggestionTarget, Content: wire.SuggestionContent}, nil
}

// DecodeSuggestionRevision reads a revision body strictly. base_revision is
// required.
func DecodeSuggestionRevision(data []byte) (SuggestionRevisionRequest, error) {
	var wire suggestionRevisionWire
	if err := decodeSearchStrictLimit(data, &wire, maxSearchSuggestionBodySize); err != nil {
		return SuggestionRevisionRequest{}, err
	}
	if wire.BaseRevision == nil || *wire.BaseRevision < 1 {
		return SuggestionRevisionRequest{}, FieldError{Field: "base_revision", Reason: "required"}
	}
	return SuggestionRevisionRequest{
		BaseRevision: *wire.BaseRevision, Target: wire.SuggestionTarget, Content: wire.SuggestionContent,
	}, nil
}

// DecodeSuggestionDecision reads a decision body strictly and checks it.
func DecodeSuggestionDecision(data []byte) (DecisionRequest, error) {
	var wire decisionWire
	if err := decodeSearchStrictLimit(data, &wire, maxSearchThemeBodySize); err != nil {
		return DecisionRequest{}, err
	}
	req := DecisionRequest{Decision: wire.Decision, Note: wire.Note}
	if wire.Revision != nil {
		req.Revision = *wire.Revision
	}
	return req, ValidateDecisionRequest(req)
}

// ValidateDecisionRequest checks a decision's shape. adopt is a real value of
// the set, and is refused by name in this version: the adoption path - the
// document write and its effects - opens in PR 3, and a decision that says
// "adopt" with nothing behind it would read as adopt_unrecorded forever.
func ValidateDecisionRequest(req DecisionRequest) error {
	switch req.Decision {
	case DecisionAbandon:
	case DecisionAdopt:
		return FieldError{Field: "decision", Reason: "not available in this version"}
	default:
		return FieldError{Field: "decision", Reason: "unknown decision"}
	}
	if req.Revision < 1 {
		return FieldError{Field: "revision", Reason: "required"}
	}
	if tooLong(req.Note, MaxDecisionNoteRunes) {
		return FieldError{Field: "note", Reason: "too long"}
	}
	return nil
}

// NormalizeSuggestionTarget trims the four ids; each is required.
func NormalizeSuggestionTarget(t SuggestionTarget) (SuggestionTarget, error) {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"work_id", &t.WorkID}, {"artifact_id", &t.ArtifactID},
		{"base_version_id", &t.BaseVersionID}, {"theme_id", &t.ThemeID},
	} {
		*field.value = strings.TrimSpace(*field.value)
		if *field.value == "" {
			return SuggestionTarget{}, FieldError{Field: field.name, Reason: "required"}
		}
	}
	return t, nil
}

func validAspect(value SuggestionAspect) bool {
	return slices.Contains(SuggestionAspects, value)
}

// NormalizeSuggestionContent checks a suggestion's shape and returns it in
// stored form. It is pure: whether the theme, the materials and the base
// version are this brand's, and whether the body differs from the base, is
// checked inside the write transaction. The proposed body is kept byte for
// byte: the difference describes what is really there.
func NormalizeSuggestionContent(c SuggestionContent) (SuggestionContent, error) {
	c.TargetQuestion = strings.TrimSpace(c.TargetQuestion)
	if c.TargetQuestion == "" || tooLong(c.TargetQuestion, MaxTargetQuestionRunes) {
		return SuggestionContent{}, FieldError{Field: "target_question", Reason: "1-500 characters"}
	}
	if len(c.Aspects) == 0 {
		return SuggestionContent{}, FieldError{Field: "aspects", Reason: "at least one aspect"}
	}
	aspects := make([]SuggestionAspect, 0, len(c.Aspects))
	for _, aspect := range c.Aspects {
		if !validAspect(aspect) {
			return SuggestionContent{}, FieldError{Field: "aspects", Reason: "unknown aspect"}
		}
		if slices.Contains(aspects, aspect) {
			return SuggestionContent{}, FieldError{Field: "aspects", Reason: "repeated aspect"}
		}
		aspects = append(aspects, aspect)
	}
	slices.Sort(aspects)
	c.Aspects = aspects
	if strings.TrimSpace(c.Rationale) == "" || tooLong(c.Rationale, MaxRationaleRunes) {
		return SuggestionContent{}, FieldError{Field: "rationale", Reason: "1-5000 characters"}
	}
	var err error
	if c.EvidenceSourceIDs, err = normalizeThemeIDs("evidence_source_ids", c.EvidenceSourceIDs); err != nil {
		return SuggestionContent{}, err
	}
	if tooLong(c.ProposedBody, MaxProposedBodyRunes) {
		return SuggestionContent{}, FieldError{Field: "proposed_body", Reason: "too long"}
	}
	return c, nil
}

// SuggestionFilter is the list's query (FR-041). An empty field does not
// filter.
type SuggestionFilter struct {
	WorkID     string
	ArtifactID string
	ThemeID    string
	State      SuggestionState
}

// ParseSuggestionFilter reads the list query. A state outside the set is
// refused by name rather than answered with an empty list.
func ParseSuggestionFilter(workID, artifactID, themeID, state string) (SuggestionFilter, error) {
	if state != "" && !slices.Contains(SuggestionStates, SuggestionState(state)) {
		return SuggestionFilter{}, FieldError{Field: "state", Reason: "unknown state"}
	}
	return SuggestionFilter{
		WorkID: strings.TrimSpace(workID), ArtifactID: strings.TrimSpace(artifactID),
		ThemeID: strings.TrimSpace(themeID), State: SuggestionState(state),
	}, nil
}

// ParseCompareIDs reads ids=a,b[,c,d]: two to four distinct ids (FR-040).
func ParseCompareIDs(raw string) ([]string, error) {
	refuse := FieldError{Field: "ids", Reason: "two to four distinct suggestion ids"}
	ids := []string{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || slices.Contains(ids, id) {
			return nil, refuse
		}
		ids = append(ids, id)
	}
	if len(ids) < MinComparedIDs || len(ids) > MaxComparedIDs {
		return nil, refuse
	}
	return ids, nil
}
