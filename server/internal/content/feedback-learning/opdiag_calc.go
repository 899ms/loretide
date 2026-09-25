package feedbacklearning

import (
	"cmp"
	"encoding/json"
	"slices"
)

// The operating diagnosis calculator (specs/035 contract §5, FR-010 to
// FR-015).
//
// CalculateDiagnosis is a pure function of the params and the stored input
// copies: it reads no table and no clock (the generation time is in the
// params), calls no model, sends nothing out and uses no floating point.
// The same params and inputs give the same bytes under the same calc version,
// whatever order the inputs arrive in.
//
// It computes the report's scope, its sections, the selected dimensions of
// each section (opdiag_dimensions.go), the report-level configuration gaps
// with every gap the dimensions found, the rules, the reference keys and the
// ROI summary a person chose to show beside it. The dimensions only add to
// what PR 1 computed and change none of it, so every version generated under
// PR 1 still recomputes to the same bytes (FR-011).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §5

// diagnosisCalculators are the calc versions this build can recompute. A
// change to an existing dimension adds "opdiag-calc/2" beside the first; it
// does not replace it.
var diagnosisCalculators = map[string]func(DiagnosisParams, []DiagnosisInput) (DiagnosisResult, error){
	DiagnosisCalcVersion: CalculateDiagnosis,
}

// diagnosisInputSet is the stored copies decoded by kind and id.
type diagnosisInputSet struct {
	accounts     map[string]diagAccountFields
	profiles     map[string]diagProfileFields
	rules        *DiagOperatingRules
	publications map[string]diagPublicationFields
	works        map[string]diagWorkFields
	topicCards   map[string]diagTopicCardFields
	metrics      map[string]diagMetricFields
	excerpts     map[string]diagExcerptFields
	reviews      map[string]diagReviewFields
	tasks        map[string]diagDeliveryTaskFields
	marks        map[string]diagWorkMarkFields
	// roi is the copy of the ROI report summary the params named (Q6), and
	// roiID its "<report_id>/<version_no>"; nil when none was named.
	roi   *ReportSummary
	roiID string
}

func decodeInto[T any](target map[string]T, input DiagnosisInput) error {
	if _, dup := target[input.ID]; dup {
		return FieldError{Field: "inputs", Reason: "an input is listed twice"}
	}
	var fields T
	if err := json.Unmarshal(input.Fields, &fields); err != nil {
		return FieldError{Field: "inputs", Reason: "unreadable input"}
	}
	target[input.ID] = fields
	return nil
}

// decodeDiagnosisInputs checks every copy against its fingerprint and
// decodes it. A copy that does not match its fingerprint is not an input.
func decodeDiagnosisInputs(inputs []DiagnosisInput) (*diagnosisInputSet, error) {
	set := &diagnosisInputSet{
		accounts: map[string]diagAccountFields{}, profiles: map[string]diagProfileFields{},
		publications: map[string]diagPublicationFields{}, works: map[string]diagWorkFields{},
		topicCards: map[string]diagTopicCardFields{}, metrics: map[string]diagMetricFields{},
		excerpts: map[string]diagExcerptFields{}, reviews: map[string]diagReviewFields{},
		tasks: map[string]diagDeliveryTaskFields{}, marks: map[string]diagWorkMarkFields{},
	}
	for _, input := range inputs {
		canonical, err := CanonicalJSON(input.Fields)
		if err != nil || diagnosisFingerprint(canonical) != input.Fingerprint {
			return nil, FieldError{Field: "inputs", Reason: "an input does not match its fingerprint"}
		}
		switch input.Kind {
		case inputAccount:
			err = decodeInto(set.accounts, input)
		case inputProfile:
			err = decodeInto(set.profiles, input)
		case inputOperatingRules:
			if set.rules != nil {
				return nil, FieldError{Field: "inputs", Reason: "an input is listed twice"}
			}
			var rules DiagOperatingRules
			if json.Unmarshal(input.Fields, &rules) != nil {
				return nil, FieldError{Field: "inputs", Reason: "unreadable input"}
			}
			set.rules = &rules
		case inputPublication:
			err = decodeInto(set.publications, input)
		case inputWork:
			err = decodeInto(set.works, input)
		case inputTopicCard:
			err = decodeInto(set.topicCards, input)
		case inputMetric:
			err = decodeInto(set.metrics, input)
		case inputExcerpt:
			err = decodeInto(set.excerpts, input)
		case inputReview:
			err = decodeInto(set.reviews, input)
		case inputDeliveryTask:
			err = decodeInto(set.tasks, input)
		case inputWorkMark:
			err = decodeInto(set.marks, input)
		case inputROISummary:
			if set.roi != nil {
				return nil, FieldError{Field: "inputs", Reason: "an input is listed twice"}
			}
			var summary ReportSummary
			if json.Unmarshal(input.Fields, &summary) != nil {
				return nil, FieldError{Field: "inputs", Reason: "unreadable input"}
			}
			set.roi, set.roiID = &summary, input.ID
		default:
			return nil, FieldError{Field: "inputs", Reason: "unknown input kind"}
		}
		if err != nil {
			return nil, err
		}
	}
	return set, nil
}

// publicationAccount is the account a publication record belongs to: its
// work's topic card's account (FR-026). "" when any link is missing - never
// a guess from free text.
func (set *diagnosisInputSet) publicationAccount(id string) string {
	publication, ok := set.publications[id]
	if !ok {
		return ""
	}
	work, ok := set.works[publication.WorkID]
	if !ok || work.TopicCardID == "" {
		return ""
	}
	return set.topicCards[work.TopicCardID].AccountID
}

// taskAccount is the account of a delivery task, through its review.
func (set *diagnosisInputSet) taskAccount(id string) string {
	return set.reviews[set.tasks[id].ReviewRequestID].AccountID
}

// hasUnknownAccount answers whether any record in the inputs belongs to no
// account; a brand summary then has an unknown_account section.
func (set *diagnosisInputSet) hasUnknownAccount() bool {
	for id := range set.publications {
		if set.publicationAccount(id) == "" {
			return true
		}
	}
	for _, review := range set.reviews {
		if review.AccountID == "" {
			return true
		}
	}
	for id := range set.tasks {
		if set.taskAccount(id) == "" {
			return true
		}
	}
	return false
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// CalculateDiagnosis computes a diagnosis result (contract §5.2). The error
// is a FieldError naming the parameter that cannot be used, or naming
// "inputs" when the stored copies are not usable.
func CalculateDiagnosis(params DiagnosisParams, inputs []DiagnosisInput) (DiagnosisResult, error) {
	p, err := prepareDiagnosisParams(params)
	if err != nil {
		return DiagnosisResult{}, err
	}
	if err = checkDiagnosisDimensions(params); err != nil {
		return DiagnosisResult{}, err
	}
	set, err := decodeDiagnosisInputs(inputs)
	if err != nil {
		return DiagnosisResult{}, err
	}

	result := DiagnosisResult{
		CalcVersion: DiagnosisCalcVersion,
		DataOrigin:  DataOriginManualOnly,
		Sections:    []DiagnosisSection{},
		Gaps:        []DiagnosisGap{},
		Rules:       []string{},
		Refs:        []string{"scope"},
	}

	// Scope: the accounts in a fixed order - by name, then id - and never by
	// any number.
	scope := DiagnosisScopeResult{
		Kind:             params.Scope.Kind,
		Accounts:         []DiagnosisScopeAccount{},
		Window:           params.Window,
		ComparisonWindow: params.ComparisonWindow,
		InputCounts: DiagnosisInputCounts{
			Publications: len(set.publications), Works: len(set.works), Metrics: len(set.metrics),
			Excerpts: len(set.excerpts), WorkMarks: len(set.marks), Reviews: len(set.reviews),
			DeliveryTasks: len(set.tasks),
		},
	}
	for _, id := range p.accountIDs {
		account, ok := set.accounts[id]
		if !ok {
			return DiagnosisResult{}, FieldError{Field: "inputs", Reason: "an account in scope has no input"}
		}
		scope.Accounts = append(scope.Accounts, DiagnosisScopeAccount{
			AccountID: id, Platform: account.Platform, DisplayName: account.DisplayName,
			ProfileRevisionID: set.profiles[id].RevisionID,
		})
	}
	slices.SortFunc(scope.Accounts, func(left, right DiagnosisScopeAccount) int {
		return cmp.Or(cmp.Compare(left.DisplayName, right.DisplayName), cmp.Compare(left.AccountID, right.AccountID))
	})
	for _, id := range sortedKeys(set.publications) {
		if work, ok := set.works[set.publications[id].WorkID]; ok && work.HistoricalImport {
			scope.HistoricalImportPublications++
		}
	}
	result.Scope = scope

	// Sections: one per account in scope; for a brand summary, the records
	// no account could be found for, and the brand level.
	for _, account := range scope.Accounts {
		result.Sections = append(result.Sections, DiagnosisSection{
			Section: sectionAccount, AccountID: account.AccountID, Dimensions: map[DiagnosisDimension]DimensionResult{},
		})
		if params.Scope.Kind == ScopeAccount {
			break
		}
	}
	if params.Scope.Kind == ScopeBrand {
		if set.hasUnknownAccount() {
			result.Sections = append(result.Sections, DiagnosisSection{
				Section: sectionUnknownAccount, Dimensions: map[DiagnosisDimension]DimensionResult{},
			})
		}
		result.Sections = append(result.Sections, DiagnosisSection{
			Section: sectionBrand, Dimensions: map[DiagnosisDimension]DimensionResult{},
		})
	}
	// The selected dimensions of each section; the brand section carries
	// only its cadence and execution flow.
	dimensionGaps := []DiagnosisGap{}
	dimensionRefs := make([]map[DiagnosisDimension][]string, len(result.Sections))
	for i := range result.Sections {
		dimensionRefs[i] = map[DiagnosisDimension][]string{}
		for _, dimension := range params.Dimensions {
			if result.Sections[i].Section == sectionBrand && !slices.Contains(brandSectionDimensions, dimension.Key) {
				continue
			}
			out := dimensionCalculators[dimension.Key](set, p, result.Sections[i])
			if result.Sections[i].Section == sectionUnknownAccount && scope.HistoricalImportPublications > 0 &&
				!slices.Contains(out.result.Limits, ruleHistoricalImportUnknownAcct) {
				out.result.Limits = append(out.result.Limits, ruleHistoricalImportUnknownAcct)
			}
			result.Sections[i].Dimensions[dimension.Key] = out.result
			dimensionGaps = append(dimensionGaps, out.gaps...)
			dimensionRefs[i][dimension.Key] = out.refs
		}
	}

	// Report-level configuration gaps: every expression profile item of an
	// account in scope that nobody has confirmed.
	for _, account := range scope.Accounts {
		status := set.profiles[account.AccountID].FieldStatus
		for _, field := range profileFieldKeys {
			if status[field] == ProfileFieldConfirmed {
				continue
			}
			ref := DiagnosisRecordRef{Kind: "profile_field", ID: account.AccountID + "/" + field}
			result.Gaps = append(result.Gaps, DiagnosisGap{
				GapKey:    "scope/" + string(GapProfileFieldPending) + "/" + ref.Kind + "/" + ref.ID,
				Kind:      GapProfileFieldPending,
				Ref:       ref,
				AccountID: account.AccountID,
				FixRoute:  fixRouteAccountSettings,
			})
		}
	}
	// Then every gap a dimension found, each key once (FR-030).
	result.Gaps = mergeGaps(result.Gaps, dimensionGaps)

	// Rules that always apply, then the named limitation of this ruling:
	// historical imports have no topic card, so no account (FR-026).
	result.Rules = append(result.Rules, ruleUnknownIsNotZero, ruleNoCrossPlatformRanking, ruleNoScore,
		ruleWindowInBrandTimezone)
	if scope.HistoricalImportPublications > 0 {
		result.Rules = append(result.Rules, ruleHistoricalImportUnknownAcct)
	}

	// The ROI summary a person chose to show (Q6): the stored copy, as it
	// is. Nothing above read it, and nothing adds, compares or divides it.
	if ref := params.ROIReportRef; ref != nil {
		if set.roi == nil || set.roiID != roiInputID(*ref) {
			return DiagnosisResult{}, FieldError{Field: "inputs", Reason: "the ROI summary named in the params has no input"}
		}
		result.ROIReference = set.roi
		result.Rules = append(result.Rules, ruleROIShownAsIs)
	} else if set.roi != nil {
		return DiagnosisResult{}, FieldError{Field: "inputs", Reason: "an ROI summary no param names"}
	}

	// Reference keys (contract §5.8): the scope, each section's dimensions,
	// each gap.
	for i, section := range result.Sections {
		name := sectionRefName(section)
		for _, dimension := range DiagnosisDimensions {
			if _, ok := section.Dimensions[dimension]; ok {
				result.Refs = append(result.Refs, name+"/"+string(dimension))
				result.Refs = append(result.Refs, dimensionRefs[i][dimension]...)
			}
		}
	}
	for _, gap := range result.Gaps {
		result.Refs = append(result.Refs, "gap/"+gap.GapKey)
	}
	if result.ROIReference != nil {
		result.Refs = append(result.Refs, "roi_reference")
	}
	return result, nil
}

// RecomputeDiagnosis recomputes a stored version from its stored params and
// inputs with its stored calc version - never the current one (FR-045). It
// is the check a test runs, not something a page calls: a stored version is
// shown as stored.
func RecomputeDiagnosis(calcVersion string, params, inputs []byte) (DiagnosisResult, error) {
	calculate, ok := diagnosisCalculators[calcVersion]
	if !ok {
		return DiagnosisResult{}, ErrCalcVersionUnavailable
	}
	var decodedParams DiagnosisParams
	if err := json.Unmarshal(params, &decodedParams); err != nil {
		return DiagnosisResult{}, ErrStorage
	}
	var decodedInputs []DiagnosisInput
	if err := json.Unmarshal(inputs, &decodedInputs); err != nil {
		return DiagnosisResult{}, ErrStorage
	}
	return calculate(decodedParams, decodedInputs)
}
