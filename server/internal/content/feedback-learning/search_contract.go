package feedbacklearning

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Search metrics and ranking observations (specs/036 PR 4, R-060 item 5).
//
// Both are things a person saw on a platform and typed in. The system never
// went and looked, estimates nothing and ranks nothing:
//
//   - A search metric is one number a platform's back end showed - search
//     impressions or visits from search - with the window it covers. Only
//     those two, and only when the platform showed them (FR-071). A number
//     the platform does not show is not recorded; nil is "not shown", 0 is a
//     confirmed zero.
//   - A ranking observation is one person looking once: when, what they
//     searched, under which conditions, and where the piece was (or how far
//     they looked without finding it). One observation is never a ranking:
//     every one carries the rule id rank.single_observation, and nothing here
//     combines two of them (FR-075).
//
// Search volume and competition are not here at all (Q5): no column, no
// field, and a request that carries either is refused by name.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md
// §1.5, §1.6, §3, §4.3, §4.4

// SearchMetric is what a platform offers about search, exactly two (FR-071).
// There is no search_volume, search_rank, competition or 'other'.
type SearchMetric string

const (
	SearchMetricImpression SearchMetric = "search_impression"
	SearchMetricVisit      SearchMetric = "search_visit"
)

var SearchMetrics = []SearchMetric{SearchMetricImpression, SearchMetricVisit}

// RankResultKind is what one look found (FR-074): the piece at a position,
// or not in the first scanned_depth results. "Not found" always says how far
// the person looked, or it could not be told apart from "did not look".
type RankResultKind string

const (
	RankPosition RankResultKind = "position"
	RankNotFound RankResultKind = "not_found"
)

var RankResultKinds = []RankResultKind{RankPosition, RankNotFound}

// SearchMetricSource is where a search metric came from, written by the
// server from the entry point. This version has one entry point.
type SearchMetricSource string

const SearchSourceManual SearchMetricSource = "manual"

var SearchMetricSources = []SearchMetricSource{SearchSourceManual}

// RuleRankSingleObservation is the rule id every observation carries. The
// page translates it; the server only names it (contract §3).
const RuleRankSingleObservation = "rank.single_observation"

// Limits from contract §1.5 and §1.6, in runes.
const (
	MaxRankQueryRunes        = 200
	MaxRankConditionsRunes   = 1000
	MaxRankEvidenceRunes     = 2000
	maxSearchObservationBody = 1 << 20
)

// ObservedAtSkew is how far past the server's clock observed_at may lie: a
// device clock a few minutes fast is ordinary, an observation from tomorrow
// is a typing mistake (spec Edge Cases).
const ObservedAtSkew = 10 * time.Minute

// SearchMetricInput is a search metric on its way in. source_type and
// recorded_by are absent: the server writes them.
type SearchMetricInput struct {
	PublicationRecordID string       `json:"publication_record_id"`
	Platform            Platform     `json:"platform"`
	AccountID           string       `json:"account_id"`
	Metric              SearchMetric `json:"metric"`
	// Value nil is "the platform does not show me this"; a pointer to 0 is
	// "I looked, it is zero". Carried to the column untouched.
	Value        *int64 `json:"value"`
	Unit         string `json:"unit"`
	StatWindow   string `json:"stat_window"`
	SampledAt    string `json:"sampled_at"`
	EvidenceNote string `json:"evidence_note"`
}

// SearchMetricRecord is one stored search metric, as answered.
type SearchMetricRecord struct {
	SearchMetricID      string             `json:"search_metric_id"`
	PublicationRecordID string             `json:"publication_record_id"`
	Platform            Platform           `json:"platform"`
	AccountID           string             `json:"account_id"`
	Metric              SearchMetric       `json:"metric"`
	Value               *int64             `json:"value"`
	Unit                string             `json:"unit"`
	StatWindow          string             `json:"stat_window"`
	SampledAt           time.Time          `json:"sampled_at"`
	EvidenceNote        string             `json:"evidence_note"`
	RecordedBy          string             `json:"recorded_by"`
	SourceType          SearchMetricSource `json:"source_type"`
	CreatedAt           time.Time          `json:"created_at"`
	DataOrigin          DataOrigin         `json:"data_origin"`
}

// RankObservationInput is an observation on its way in.
type RankObservationInput struct {
	Platform            Platform       `json:"platform"`
	AccountID           string         `json:"account_id"`
	Query               string         `json:"query"`
	ThemeID             string         `json:"theme_id"`
	PublicationRecordID string         `json:"publication_record_id"`
	ObservedAt          string         `json:"observed_at"`
	Conditions          string         `json:"conditions"`
	ResultKind          RankResultKind `json:"result_kind"`
	Position            *int           `json:"position"`
	ScannedDepth        *int           `json:"scanned_depth"`
	EvidenceNote        string         `json:"evidence_note"`
}

// RankObservation is one stored revision of one observation, as answered.
// Every field is about this one look; there is no field that combines it
// with another (FR-075) - no average, best, median or current position.
type RankObservation struct {
	ObservationID       string         `json:"observation_id"`
	Revision            int            `json:"revision"`
	Voided              bool           `json:"voided"`
	Platform            Platform       `json:"platform"`
	AccountID           string         `json:"account_id"`
	Query               string         `json:"query"`
	ThemeID             string         `json:"theme_id"`
	PublicationRecordID string         `json:"publication_record_id"`
	ObservedAt          time.Time      `json:"observed_at"`
	Conditions          string         `json:"conditions"`
	ResultKind          RankResultKind `json:"result_kind"`
	Position            *int           `json:"position"`
	ScannedDepth        *int           `json:"scanned_depth"`
	EvidenceNote        string         `json:"evidence_note"`
	RecordedBy          string         `json:"recorded_by"`
	CreatedAt           time.Time      `json:"created_at"`
	DataOrigin          DataOrigin     `json:"data_origin"`
	Rule                string         `json:"rule"`
}

// RankObservationRevision is a whole new revision of an observation.
type RankObservationRevision struct {
	BaseRevision int
	Voided       bool
	Input        RankObservationInput
}

// RankObservationFilter is the list's query. At least one of ThemeID,
// PublicationRecordID and Query is required; the three combine with AND.
type RankObservationFilter struct {
	ThemeID             string
	PublicationRecordID string
	Query               string
	IncludeVoided       bool
}

// searchObservationServerWritten are fields the server writes. A body may
// carry them - a client echoing a record back must not fail on them - and
// they are dropped, never used (contract §4). search_volume, competition and
// rank are not among them: they are refused by name.
type searchObservationServerWritten struct {
	RecordedBy     json.RawMessage `json:"recorded_by"`
	SourceType     json.RawMessage `json:"source_type"`
	DataOrigin     json.RawMessage `json:"data_origin"`
	Rule           json.RawMessage `json:"rule"`
	CreatedAt      json.RawMessage `json:"created_at"`
	SearchMetricID json.RawMessage `json:"search_metric_id"`
	ObservationID  json.RawMessage `json:"observation_id"`
	Revision       json.RawMessage `json:"revision"`
}

type searchMetricWire struct {
	SearchMetricInput
	searchObservationServerWritten
}

type rankObservationWire struct {
	RankObservationInput
	searchObservationServerWritten
}

type rankRevisionWire struct {
	RankObservationInput
	searchObservationServerWritten
	BaseRevision *int `json:"base_revision"`
	Voided       bool `json:"voided"`
}

// DecodeSearchMetric reads a search metric body strictly.
func DecodeSearchMetric(data []byte) (SearchMetricInput, error) {
	var wire searchMetricWire
	if err := decodeSearchStrict(data, &wire); err != nil {
		return SearchMetricInput{}, err
	}
	return wire.SearchMetricInput, nil
}

// DecodeRankObservation reads a new observation's body strictly. A revision
// field in it is refused by name: a new observation has no base.
func DecodeRankObservation(data []byte) (RankObservationInput, error) {
	var wire rankObservationWire
	if err := decodeSearchStrict(data, &wire); err != nil {
		return RankObservationInput{}, err
	}
	return wire.RankObservationInput, nil
}

// DecodeRankObservationRevision reads a revision body strictly.
// base_revision is required.
func DecodeRankObservationRevision(data []byte) (RankObservationRevision, error) {
	var wire rankRevisionWire
	if err := decodeSearchStrict(data, &wire); err != nil {
		return RankObservationRevision{}, err
	}
	if wire.BaseRevision == nil || *wire.BaseRevision < 1 {
		return RankObservationRevision{}, FieldError{Field: "base_revision", Reason: "invalid or missing"}
	}
	return RankObservationRevision{
		BaseRevision: *wire.BaseRevision, Voided: wire.Voided, Input: wire.RankObservationInput,
	}, nil
}

// decodeSearchStrict decodes exactly one JSON value with no unknown members.
// An unknown member is refused by its own name, a wrongly typed value by its
// JSON path; never a Go type name. The same rule as topic-planning's
// decodeSearchStrict, restated because modules do not import each other.
func decodeSearchStrict(data []byte, target any) error {
	if len(data) > maxSearchObservationBody {
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
// for embedded structs ("SearchMetricInput.value"). Every JSON name here is
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
// "search_volume"`. Anything that does not look like a JSON key is not
// echoed.
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

// ValidateSearchMetric checks a search metric in the contract's order -
// controlled sets, required fields, values, lengths - and answers the
// sampling time in UTC. It reads no database.
func ValidateSearchMetric(input SearchMetricInput) (time.Time, error) {
	if input.PublicationRecordID == "" {
		return time.Time{}, invalidField("publication_record_id")
	}
	if !oneOf(string(input.Platform), Platforms) {
		return time.Time{}, invalidField("platform")
	}
	if !oneOf(string(input.Metric), SearchMetrics) {
		return time.Time{}, invalidField("metric")
	}
	if input.Value != nil && *input.Value < 0 {
		return time.Time{}, FieldError{Field: "value", Reason: "negative"}
	}
	if strings.TrimSpace(input.StatWindow) == "" {
		return time.Time{}, FieldError{Field: "stat_window", Reason: "required"}
	}
	sampledAt, err := time.Parse(time.RFC3339, input.SampledAt)
	if err != nil {
		return time.Time{}, FieldError{Field: "sampled_at", Reason: "not an RFC 3339 timestamp"}
	}
	for _, field := range []struct{ name, value string }{
		{"account_id", input.AccountID}, {"unit", input.Unit}, {"stat_window", input.StatWindow},
	} {
		if err := ValidateShort(field.name, field.value); err != nil {
			return time.Time{}, err
		}
	}
	if err := ValidateNote("evidence_note", input.EvidenceNote); err != nil {
		return time.Time{}, err
	}
	return sampledAt.UTC(), nil
}

// NormalizeRankQuery is FR-073: NFC, then trim. Nothing else - no case
// folding, no splitting.
func NormalizeRankQuery(query string) string {
	return strings.TrimSpace(norm.NFC.String(query))
}

// ValidateRankObservation checks an observation against the server's clock
// now and answers it normalized: the query in NFC and trimmed, observed_at
// in UTC. The first failure is named by its field.
func ValidateRankObservation(input RankObservationInput, now time.Time) (RankObservation, error) {
	if !oneOf(string(input.Platform), Platforms) {
		return RankObservation{}, invalidField("platform")
	}
	if !oneOf(string(input.ResultKind), RankResultKinds) {
		return RankObservation{}, invalidField("result_kind")
	}
	query := NormalizeRankQuery(input.Query)
	if query == "" {
		return RankObservation{}, FieldError{Field: "query", Reason: "required"}
	}
	if utf8.RuneCountInString(query) > MaxRankQueryRunes {
		return RankObservation{}, FieldError{Field: "query", Reason: "too long"}
	}
	observedAt, err := time.Parse(time.RFC3339, input.ObservedAt)
	if err != nil {
		return RankObservation{}, FieldError{Field: "observed_at", Reason: "not an RFC 3339 timestamp"}
	}
	if observedAt.After(now.Add(ObservedAtSkew)) {
		return RankObservation{}, FieldError{Field: "observed_at", Reason: "in the future"}
	}
	if strings.TrimSpace(input.Conditions) == "" {
		return RankObservation{}, FieldError{Field: "conditions", Reason: "required"}
	}
	if utf8.RuneCountInString(input.Conditions) > MaxRankConditionsRunes {
		return RankObservation{}, FieldError{Field: "conditions", Reason: "too long"}
	}
	if err := checkRankResult(input); err != nil {
		return RankObservation{}, err
	}
	if strings.TrimSpace(input.EvidenceNote) == "" {
		return RankObservation{}, FieldError{Field: "evidence_note", Reason: "required"}
	}
	if utf8.RuneCountInString(input.EvidenceNote) > MaxRankEvidenceRunes {
		return RankObservation{}, FieldError{Field: "evidence_note", Reason: "too long"}
	}
	for _, field := range []struct{ name, value string }{
		{"account_id", input.AccountID}, {"theme_id", input.ThemeID},
		{"publication_record_id", input.PublicationRecordID},
	} {
		if err := ValidateShort(field.name, field.value); err != nil {
			return RankObservation{}, err
		}
	}
	return RankObservation{
		Platform: input.Platform, AccountID: input.AccountID, Query: query,
		ThemeID: input.ThemeID, PublicationRecordID: input.PublicationRecordID,
		ObservedAt: observedAt.UTC(), Conditions: input.Conditions, ResultKind: input.ResultKind,
		Position: input.Position, ScannedDepth: input.ScannedDepth, EvidenceNote: input.EvidenceNote,
	}, nil
}

// checkRankResult is FR-074: exactly one of the two integers, the one the
// kind asks for, at least 1. The one the kind needs is named when it is
// missing or below 1; the other is named when it was sent as well.
func checkRankResult(input RankObservationInput) error {
	needed, neededName, other, otherName := input.Position, "position", input.ScannedDepth, "scanned_depth"
	if input.ResultKind == RankNotFound {
		needed, neededName, other, otherName = input.ScannedDepth, "scanned_depth", input.Position, "position"
	}
	if needed == nil || *needed < 1 {
		return FieldError{Field: neededName, Reason: "required and at least 1 for this result_kind"}
	}
	if other != nil {
		return FieldError{Field: otherName, Reason: "not allowed for this result_kind"}
	}
	return nil
}

// ParseRankObservationFilter reads the list's query. At least one of
// theme_id, publication_record_id and query is required; with none the
// refusal names theme_id, the first of the three.
func ParseRankObservationFilter(themeID, publicationRecordID, query, includeVoided string) (RankObservationFilter, error) {
	filter := RankObservationFilter{
		ThemeID: themeID, PublicationRecordID: publicationRecordID, Query: NormalizeRankQuery(query),
	}
	switch includeVoided {
	case "", "false":
	case "true":
		filter.IncludeVoided = true
	default:
		return RankObservationFilter{}, invalidField("include_voided")
	}
	if filter.ThemeID == "" && filter.PublicationRecordID == "" && filter.Query == "" {
		return RankObservationFilter{}, FieldError{Field: "theme_id", Reason: "one of theme_id, publication_record_id or query is required"}
	}
	return filter, nil
}

// ViewRankObservation adds what every observation says about itself: it was
// typed in by a person, and it is one look, not a ranking.
func ViewRankObservation(observation RankObservation) RankObservation {
	observation.DataOrigin = DataOriginManualOnly
	observation.Rule = RuleRankSingleObservation
	return observation
}
