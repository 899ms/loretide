package feedbacklearning

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Brand/account operating diagnosis (specs/035, BO-02 / R-057): the
// controlled sets, the parameters and the result shape.
//
// Named "operating diagnosis" (opdiag) throughout, never "diagnostics": that
// is the development diagnostics panel and module (document 13), and the two
// are modelled and shown apart (R-057's last sentence, FR-090 to FR-092).
//
// PR 1 stored report versions and work marks and computed the scope and the
// report-level gaps. PR 2 registers the six dimension calculators, the gaps
// they find and the ROI reference; a version PR 1 generated still recomputes
// to the same bytes, because adding a dimension changes neither the scope
// nor any other dimension (FR-011).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §3, §4, §5

// DiagnosisCalcVersion is the calculator version a report is computed under.
// Changing any existing dimension's algorithm, rounding or result shape adds
// a new version beside this one; adding a dimension does not (FR-011).
const DiagnosisCalcVersion = "opdiag-calc/1"

// MaxDiagnosisTitleRunes bounds a report title (contract §1.1).
const MaxDiagnosisTitleRunes = 200

// MaxPillarRunes bounds one content pillar name (contract §1.2, §4).
const MaxPillarRunes = 100

// MaxMarkNoteRunes bounds a work mark's note (contract §1.2).
const MaxMarkNoteRunes = 2000

// DiagnosisDimension is R-057's second bullet, exactly six (FR-003).
type DiagnosisDimension string

const (
	DimensionConsistency      DiagnosisDimension = "consistency"
	DimensionCoverage         DiagnosisDimension = "coverage"
	DimensionCadence          DiagnosisDimension = "cadence"
	DimensionPerformance      DiagnosisDimension = "performance"
	DimensionAudienceFeedback DiagnosisDimension = "audience_feedback"
	DimensionExecutionFlow    DiagnosisDimension = "execution_flow"
)

var DiagnosisDimensions = []DiagnosisDimension{
	DimensionConsistency, DimensionCoverage, DimensionCadence,
	DimensionPerformance, DimensionAudienceFeedback, DimensionExecutionFlow,
}

// DiagnosisScope is R-057's "账号报告及授权范围内的品牌汇总".
type DiagnosisScope string

const (
	ScopeAccount DiagnosisScope = "account"
	ScopeBrand   DiagnosisScope = "brand"
)

var DiagnosisScopes = []DiagnosisScope{ScopeAccount, ScopeBrand}

// MarkKind is what a person marked a work for (ruling Q2=A).
type MarkKind string

const (
	MarkPillar      MarkKind = "pillar"
	MarkConsistency MarkKind = "consistency"
)

var MarkKinds = []MarkKind{MarkPillar, MarkConsistency}

// MarkVerdict is the mark itself. A pillar mark takes tagged / untagged; a
// consistency mark takes consistent / inconsistent / unsure.
type MarkVerdict string

const (
	VerdictTagged       MarkVerdict = "tagged"
	VerdictUntagged     MarkVerdict = "untagged"
	VerdictConsistent   MarkVerdict = "consistent"
	VerdictInconsistent MarkVerdict = "inconsistent"
	VerdictUnsure       MarkVerdict = "unsure"
)

var MarkVerdicts = []MarkVerdict{
	VerdictTagged, VerdictUntagged, VerdictConsistent, VerdictInconsistent, VerdictUnsure,
}

// verdictsFor is which verdicts each kind takes (the CHECK in the migration
// holds the same pairing).
func verdictsFor(kind MarkKind) []MarkVerdict {
	switch kind {
	case MarkPillar:
		return []MarkVerdict{VerdictTagged, VerdictUntagged}
	case MarkConsistency:
		return []MarkVerdict{VerdictConsistent, VerdictInconsistent, VerdictUnsure}
	default:
		return nil
	}
}

// DataOrigin is D14-V08's "模拟结果和真实联网分别标识". This version has
// people's records and a deterministic calculation, and nothing simulated or
// fetched: exactly one value (FR-073).
type DataOrigin string

const DataOriginManualOnly DataOrigin = "manual_only"

var DataOrigins = []DataOrigin{DataOriginManualOnly}

// profileFieldKeys are ip-profile's eleven expression profile items, by their
// JSON names in its own order (ip-profile/profile.go). Copied, not imported:
// this module does not depend on ip-profile. A test reads that file and
// compares, so the two cannot drift.
var profileFieldKeys = []string{
	"audience", "common_questions", "experience", "positioning", "content_pillars",
	"expression_style", "forbidden_expressions", "content_goals",
	"primary_channels", "weekly_hours", "style_samples",
}

// profileTextKeys are the eight of them that are TextField items: the only
// ones a profile proposal may change (FR-067).
var profileTextKeys = profileFieldKeys[:8]

// ProfileFieldConfirmed is ip-profile's "confirmed" field status. Anything
// else, including an item with no revision at all, is not confirmed.
const ProfileFieldConfirmed = "confirmed"

// GapKind is what a gap is missing (contract §3, R-057's 补录待办). The
// report-level configuration gap is PR 1's; the rest are found by the
// dimensions.
type GapKind string

const (
	GapProfileFieldPending      GapKind = "profile_field_pending"
	GapWorkUnchecked            GapKind = "work_unchecked"
	GapWorkUntagged             GapKind = "work_untagged"
	GapCadenceUnset             GapKind = "cadence_unset"
	GapObservationUnset         GapKind = "observation_unset"
	GapPublishedAtMissing       GapKind = "published_at_missing"
	GapMetricMissing            GapKind = "metric_missing"
	GapAccountUnresolved        GapKind = "account_unresolved"
	GapWorkMissing              GapKind = "work_missing"
	GapPublicationStatusUnknown GapKind = "publication_status_unknown"
	GapExcerptUntagged          GapKind = "excerpt_untagged"
)

var GapKinds = []GapKind{
	GapProfileFieldPending, GapWorkUnchecked, GapWorkUntagged, GapCadenceUnset, GapObservationUnset,
	GapPublishedAtMissing, GapMetricMissing, GapAccountUnresolved, GapWorkMissing,
	GapPublicationStatusUnknown, GapExcerptUntagged,
}

// DimensionReason is why a dimension, or one fact of it, cannot be computed
// (FR-012). Ordered as contract §3 lists them.
type DimensionReason string

const (
	DimensionNoData                   DimensionReason = "no_data"
	DimensionMissingConfig            DimensionReason = "missing_config"
	DimensionMissingComparisonWindow  DimensionReason = "missing_comparison_window"
	DimensionMissingObservationWindow DimensionReason = "missing_observation_window"
	DimensionNoDeliveryChannel        DimensionReason = "no_delivery_channel"
)

var DimensionReasons = []DimensionReason{
	DimensionNoData, DimensionMissingConfig, DimensionMissingComparisonWindow,
	DimensionMissingObservationWindow, DimensionNoDeliveryChannel,
}

// Dimension statuses (FR-012): ok with facts, or not computable with a
// reason. Never a zero standing in for the second.
const (
	dimensionOK            = "ok"
	dimensionNotComputable = "not_computable"
)

// Where the page sends a person to fill a gap. A route id, not a URL
// (FR-031).
const (
	fixRouteAccountSettings = "account_settings"
	fixRouteWorkMarks       = "work_marks"
	fixRouteOperatingRules  = "operating_rules"
	fixRoutePublications    = "publications"
	fixRouteFeedback        = "feedback"
	fixRouteWorks           = "works"
)

// DiagnosisRuleID is a rule the result names (contract §5.9). The server
// only ever names a rule; the page translates it (FR-015). None of them, and
// no other string literal in this code, states a cause or a trend (FR-028):
// a guard scans the literals.
type DiagnosisRuleID = string

const (
	ruleUnknownIsNotZero            = "common.unknown_is_not_zero"
	ruleNoCrossPlatformRanking      = "common.no_cross_platform_ranking"
	ruleNoScore                     = "common.no_score"
	ruleWindowInBrandTimezone       = "common.window_in_brand_timezone"
	ruleMarksOnOlderProfile         = "consistency.marks_on_older_profile"
	ruleMultiPillarNotAdditive      = "coverage.multi_pillar_not_additive"
	ruleTargetIsBrandChannelLevel   = "cadence.target_is_brand_channel_level"
	ruleIncompleteWeekNotCompared   = "cadence.incomplete_week_not_compared"
	ruleDifferenceIsNotCause        = "performance.difference_is_not_cause"
	ruleSamplesAtDifferentAges      = "performance.samples_taken_at_different_ages"
	ruleStatWindowMixed             = "performance.stat_window_mixed"
	ruleTagsNotAdditive             = "audience_feedback.tags_not_additive"
	ruleNoThreshold                 = "execution_flow.no_threshold"
	ruleROIShownAsIs                = "roi_reference.shown_as_is"
	ruleHistoricalImportUnknownAcct = "scope.historical_import_account_unknown"
)

var DiagnosisRuleIDs = []DiagnosisRuleID{
	ruleUnknownIsNotZero, ruleNoCrossPlatformRanking, ruleNoScore, ruleWindowInBrandTimezone,
	ruleMarksOnOlderProfile, ruleMultiPillarNotAdditive, ruleTargetIsBrandChannelLevel,
	ruleIncompleteWeekNotCompared, ruleDifferenceIsNotCause, ruleSamplesAtDifferentAges,
	ruleStatWindowMixed, ruleTagsNotAdditive, ruleNoThreshold, ruleROIShownAsIs,
	ruleHistoricalImportUnknownAcct,
}

// MaxPillars bounds a coverage dimension's pillar list (contract §4).
const MaxPillars = 50

// Section names (contract §5.2).
const (
	sectionAccount        = "account"
	sectionUnknownAccount = "unknown_account"
	sectionBrand          = "brand"
)

// ---------------------------------------------------------------- params

// DiagnosisParams is contract §4. Timezone and GeneratedAt are the server's
// to write; a body that sends them has them overwritten.
type DiagnosisParams struct {
	Scope            DiagnosisScopeParams      `json:"scope"`
	Window           DiagnosisWindow           `json:"window"`
	ComparisonWindow *DiagnosisDateRange       `json:"comparison_window"`
	Dimensions       []DiagnosisDimensionParam `json:"dimensions"`
	ROIReportRef     *DiagnosisROIRef          `json:"roi_report_ref"`
	GeneratedAt      string                    `json:"generated_at"`
}

// DiagnosisScopeParams is which accounts. brand takes the accounts a person
// ticked, one or more; there is no "empty means all" (FR-005).
type DiagnosisScopeParams struct {
	Kind       DiagnosisScope `json:"kind"`
	AccountIDs []string       `json:"account_ids"`
}

// DiagnosisWindow is the report window, [start 00:00, end+1 00:00) in the
// brand's timezone (FR-002).
type DiagnosisWindow struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

// DiagnosisDateRange is the comparison window: the report window's timezone
// applies.
type DiagnosisDateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// DiagnosisDimensionParam is one selected dimension and its own parameters
// (contract §4). Every member any dimension takes is here, so the strict
// decode does not change when PR 2 fills them in.
type DiagnosisDimensionParam struct {
	Key       DiagnosisDimension `json:"key"`
	Items     []string           `json:"items,omitempty"`
	Pillars   []string           `json:"pillars,omitempty"`
	Metrics   []Metric           `json:"metrics,omitempty"`
	Platforms []Platform         `json:"platforms,omitempty"`
	Sources   []ExcerptSource    `json:"sources,omitempty"`
}

// DiagnosisROIRef names one 034 ROI report version to show as it is (Q6).
type DiagnosisROIRef struct {
	ReportID  string `json:"report_id"`
	VersionNo int    `json:"version_no"`
}

// preparedDiagnosis is params checked and turned into what the calculator
// compares against.
type preparedDiagnosis struct {
	params     DiagnosisParams
	location   *time.Location
	from, to   time.Time // [from, to) of the report window
	accountIDs []string  // sorted, unique
	// hasComparison says whether a comparison window was given;
	// [cmpFrom, cmpTo) is it.
	hasComparison  bool
	cmpFrom, cmpTo time.Time
	// dimensions are the selected dimensions' own params, by key; pillars
	// are the coverage pillars normalized, in the order given.
	dimensions map[DiagnosisDimension]DiagnosisDimensionParam
	pillars    []string
	// generatedAt is params.generated_at; the zero time when it is absent.
	generatedAt time.Time
}

// selected answers whether a dimension is in the params.
func (p preparedDiagnosis) selected(key DiagnosisDimension) bool {
	_, ok := p.dimensions[key]
	return ok
}

// inComparison answers whether a time falls in the comparison window. With
// no comparison window, nothing does.
func (p preparedDiagnosis) inComparison(at *time.Time) bool {
	if at == nil || !p.hasComparison {
		return false
	}
	local := at.In(p.location)
	return !local.Before(p.cmpFrom) && local.Before(p.cmpTo)
}

// inWindow answers whether a time falls in [start 00:00, end+1 00:00) in the
// brand's timezone. An absent time is in no window.
func (p preparedDiagnosis) inWindow(at *time.Time) bool {
	if at == nil {
		return false
	}
	local := at.In(p.location)
	return !local.Before(p.from) && local.Before(p.to)
}

func parseDiagnosisDate(field, text string, location *time.Location) (time.Time, error) {
	day, err := time.ParseInLocation(time.DateOnly, text, location)
	if err != nil {
		return time.Time{}, FieldError{Field: field, Reason: "not a date (YYYY-MM-DD)"}
	}
	return day, nil
}

func diagnosisRange(field, start, end string, location *time.Location) (time.Time, time.Time, error) {
	from, err := parseDiagnosisDate(field+".start", start, location)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	last, err := parseDiagnosisDate(field+".end", end, location)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if last.Before(from) {
		return time.Time{}, time.Time{}, FieldError{Field: field, Reason: "ends before it starts"}
	}
	// The window ends where the day after its last day begins: a calendar
	// step, never a fixed number of hours.
	return from, time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, location), nil
}

// prepareDiagnosisParams checks params in the contract's order - controlled
// sets, then required fields and combinations - and names the first field
// that is wrong. It does not know which dimensions have a calculator; the
// calculator refuses those.
func prepareDiagnosisParams(params DiagnosisParams) (preparedDiagnosis, error) {
	p := preparedDiagnosis{params: params}
	if !oneOf(string(params.Scope.Kind), DiagnosisScopes) {
		return p, FieldError{Field: "scope.kind", Reason: "not one of account, brand"}
	}
	for _, dimension := range params.Dimensions {
		if !oneOf(string(dimension.Key), DiagnosisDimensions) {
			return p, FieldError{Field: "dimensions", Reason: "unknown dimension"}
		}
	}
	seen := map[string]bool{}
	for _, id := range params.Scope.AccountIDs {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return p, FieldError{Field: "scope.account_ids", Reason: "invalid account id"}
		}
		if seen[id] {
			return p, FieldError{Field: "scope.account_ids", Reason: "listed twice"}
		}
		seen[id] = true
		p.accountIDs = append(p.accountIDs, id)
	}
	slices.Sort(p.accountIDs)
	switch {
	case params.Scope.Kind == ScopeAccount && len(p.accountIDs) != 1:
		return p, FieldError{Field: "scope.account_ids", Reason: "an account report takes exactly one account"}
	case params.Scope.Kind == ScopeBrand && len(p.accountIDs) == 0:
		return p, FieldError{Field: "scope.account_ids", Reason: "a brand summary takes the accounts to include"}
	}
	location, err := time.LoadLocation(params.Window.Timezone)
	if err != nil || params.Window.Timezone == "" {
		return p, FieldError{Field: "window.timezone", Reason: "not a timezone"}
	}
	p.location = location
	if p.from, p.to, err = diagnosisRange("window", params.Window.Start, params.Window.End, location); err != nil {
		return p, err
	}
	if params.ComparisonWindow != nil {
		if p.cmpFrom, p.cmpTo, err = diagnosisRange("comparison_window", params.ComparisonWindow.Start,
			params.ComparisonWindow.End, location); err != nil {
			return p, err
		}
		p.hasComparison = true
	}
	p.dimensions = map[DiagnosisDimension]DiagnosisDimensionParam{}
	for _, dimension := range params.Dimensions {
		if p.selected(dimension.Key) {
			return p, FieldError{Field: "dimensions", Reason: "a dimension is listed twice"}
		}
		p.dimensions[dimension.Key] = dimension
		if err = checkDimensionParam(dimension); err != nil {
			return p, err
		}
	}
	for _, pillar := range p.dimensions[DimensionCoverage].Pillars {
		p.pillars = append(p.pillars, normalizePillar(pillar))
	}
	if params.GeneratedAt != "" {
		if p.generatedAt, err = time.Parse(time.RFC3339, params.GeneratedAt); err != nil {
			return p, FieldError{Field: "generated_at", Reason: "not a time"}
		}
	}
	// roi_report_ref (Q6) is checked for existence with the accounts, before
	// any of this; an empty one answers like a missing report version.
	return p, nil
}

// checkDimensionParam checks one dimension's own params (contract §4): a
// dimension takes only its own members, every value is in its controlled
// set, nothing is listed twice, and a pillar is neither blank nor too long.
// An empty list is not refused - it is that dimension's missing_config, so a
// person can still read the others.
func checkDimensionParam(dimension DiagnosisDimensionParam) error {
	field := "dimensions." + string(dimension.Key) + "."
	takes := map[DiagnosisDimension][]string{
		DimensionConsistency:      {"items"},
		DimensionCoverage:         {"pillars"},
		DimensionPerformance:      {"metrics", "platforms"},
		DimensionAudienceFeedback: {"sources"},
	}[dimension.Key]
	for _, member := range []struct {
		name  string
		given int
	}{
		{"items", len(dimension.Items)}, {"pillars", len(dimension.Pillars)}, {"metrics", len(dimension.Metrics)},
		{"platforms", len(dimension.Platforms)}, {"sources", len(dimension.Sources)},
	} {
		if member.given > 0 && !slices.Contains(takes, member.name) {
			return FieldError{Field: field + member.name, Reason: "not a parameter of this dimension"}
		}
	}
	if err := checkMembers(field+"items", dimension.Items, profileFieldKeys); err != nil {
		return err
	}
	if err := checkMembers(field+"metrics", dimension.Metrics, Metrics); err != nil {
		return err
	}
	if err := checkMembers(field+"platforms", dimension.Platforms, Platforms); err != nil {
		return err
	}
	if err := checkMembers(field+"sources", dimension.Sources, ExcerptSources); err != nil {
		return err
	}
	if len(dimension.Pillars) > MaxPillars {
		return FieldError{Field: field + "pillars", Reason: "too many pillars"}
	}
	seen := map[string]bool{}
	for _, pillar := range dimension.Pillars {
		name := normalizePillar(pillar)
		if name == "" {
			return FieldError{Field: field + "pillars", Reason: "a pillar is blank"}
		}
		if err := checkRuneLimit(field+"pillars", name, MaxPillarRunes); err != nil {
			return err
		}
		if seen[name] {
			return FieldError{Field: field + "pillars", Reason: "a pillar is listed twice"}
		}
		seen[name] = true
	}
	return nil
}

// checkMembers refuses a value outside its set and a value given twice.
func checkMembers[T ~string, S ~string](field string, values []T, allowed []S) error {
	seen := map[T]bool{}
	for _, value := range values {
		if !oneOf(string(value), allowed) {
			return FieldError{Field: field, Reason: "not in its controlled set"}
		}
		if seen[value] {
			return FieldError{Field: field, Reason: "listed twice"}
		}
		seen[value] = true
	}
	return nil
}

// normalizePillar is the one normalization a pillar name gets: NFC and no
// surrounding white space. No case folding and no merging of synonyms.
func normalizePillar(name string) string {
	return strings.TrimSpace(norm.NFC.String(name))
}

func checkRuneLimit(field, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return FieldError{Field: field, Reason: "too long"}
	}
	return nil
}

// ---------------------------------------------------------------- result

// DiagnosisResult is contract §5.2. Arrays are sorted by fixed keys, never
// by a number; there is no score, grade, rating, rank or level anywhere in
// it (FR-013).
type DiagnosisResult struct {
	CalcVersion  string               `json:"calc_version"`
	DataOrigin   DataOrigin           `json:"data_origin"`
	Scope        DiagnosisScopeResult `json:"scope"`
	Sections     []DiagnosisSection   `json:"sections"`
	Gaps         []DiagnosisGap       `json:"gaps"`
	ROIReference *ReportSummary       `json:"roi_reference"`
	Rules        []string             `json:"rules"`
	Refs         []string             `json:"refs"`
}

// DiagnosisScopeResult is what the report looked at: which accounts under
// which profile revision, the two windows, and how many of each input.
type DiagnosisScopeResult struct {
	Kind                         DiagnosisScope          `json:"kind"`
	Accounts                     []DiagnosisScopeAccount `json:"accounts"`
	Window                       DiagnosisWindow         `json:"window"`
	ComparisonWindow             *DiagnosisDateRange     `json:"comparison_window"`
	InputCounts                  DiagnosisInputCounts    `json:"input_counts"`
	HistoricalImportPublications int                     `json:"historical_import_publications"`
}

// DiagnosisScopeAccount is one account in scope. ProfileRevisionID is ""
// when the account has no profile revision yet.
type DiagnosisScopeAccount struct {
	AccountID         string `json:"account_id"`
	Platform          string `json:"platform"`
	DisplayName       string `json:"display_name"`
	ProfileRevisionID string `json:"profile_revision_id"`
}

// DiagnosisInputCounts is how many of each input kind the report read.
type DiagnosisInputCounts struct {
	Publications  int `json:"publications"`
	Works         int `json:"works"`
	Metrics       int `json:"metrics"`
	Excerpts      int `json:"excerpts"`
	WorkMarks     int `json:"work_marks"`
	Reviews       int `json:"reviews"`
	DeliveryTasks int `json:"delivery_tasks"`
}

// DiagnosisSection is one account, the records no account could be found
// for, or the brand level (contract §5.2).
type DiagnosisSection struct {
	Section    string                                 `json:"section"`
	AccountID  string                                 `json:"account_id,omitempty"`
	Dimensions map[DiagnosisDimension]DimensionResult `json:"dimensions"`
}

// DimensionResult is one dimension of one section: ok with facts, or
// not_computable with a reason - never a zero standing in for "unknown"
// (FR-012). Facts are each dimension's own type (opdiag_dimensions.go), and
// every number in them is a count or a rational number written as a string.
type DimensionResult struct {
	Status       string                `json:"status"`
	Reason       string                `json:"reason,omitempty"`
	Facts        json.RawMessage       `json:"facts,omitempty"`
	Completeness DimensionCompleteness `json:"completeness"`
	Limits       []string              `json:"limits"`
	Records      []DiagnosisRecordRef  `json:"records"`
}

// DimensionCompleteness is two counts and the gaps between them (FR-030).
type DimensionCompleteness struct {
	Expected int      `json:"expected"`
	Present  int      `json:"present"`
	GapKeys  []string `json:"gap_keys"`
}

// DiagnosisRecordRef points at one record or item.
type DiagnosisRecordRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// DiagnosisGap is one missing piece: a stable key, what is missing, where,
// for which account, and which page fixes it (FR-031).
type DiagnosisGap struct {
	GapKey    string             `json:"gap_key"`
	Kind      GapKind            `json:"kind"`
	Dimension string             `json:"dimension"`
	Ref       DiagnosisRecordRef `json:"ref"`
	AccountID string             `json:"account_id"`
	FixRoute  string             `json:"fix_route"`
}
