package feedbacklearning

import (
	"cmp"
	"encoding/json"
	"slices"
)

// The completeness of a diagnosis and its 补录待办 (specs/035 FR-030 to
// FR-033, contract §5.2).
//
// A gap is one missing piece of input: a stable key, what is missing, which
// record or item it is about, the account (when there is one) and which page
// fills it. The key is the dimension that found it, the kind and the record,
// so the same gap found twice is the same key. A gap that is not any one
// dimension's - an expression profile item nobody confirmed, a record no
// account could be found for - is keyed under "scope/", which is PR 1's
// spelling for the report-level configuration gaps and is kept as it is, so
// two dimensions that meet it list it once in the report.
//
// The 补录待办 is this list as a stored version holds it. It is never stored
// a second time (FR-032); PR 3 lets a person turn one gap into a to-do.
//
// Completeness is two counts and the gaps between them (FR-033). It is not
// folded into a percentage or anything that reads like a score.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §5.2

// gapFixRoutes says which page fills each kind of gap. A page route id, not
// a URL (FR-031).
var gapFixRoutes = map[GapKind]string{
	GapProfileFieldPending:      fixRouteAccountSettings,
	GapWorkUnchecked:            fixRouteWorkMarks,
	GapWorkUntagged:             fixRouteWorkMarks,
	GapCadenceUnset:             fixRouteOperatingRules,
	GapObservationUnset:         fixRouteOperatingRules,
	GapPublishedAtMissing:       fixRoutePublications,
	GapPublicationStatusUnknown: fixRoutePublications,
	GapMetricMissing:            fixRouteFeedback,
	GapExcerptUntagged:          fixRouteFeedback,
	GapAccountUnresolved:        fixRouteWorks,
	GapWorkMissing:              fixRouteWorks,
}

// Record kinds a gap or a dimension points at.
const (
	refPublication  = "publication"
	refWork         = "work"
	refExcerpt      = "excerpt"
	refMetric       = "metric"
	refReview       = "review"
	refDeliveryTask = "delivery_task"
	refChannel      = "channel"
	refProfileField = "profile_field"
)

// newGap builds a gap with its deterministic key. dimension "" is a
// report-level gap, keyed "scope/...".
func newGap(dimension DiagnosisDimension, kind GapKind, ref DiagnosisRecordRef, accountID string) DiagnosisGap {
	prefix := string(dimension)
	if prefix == "" {
		prefix = "scope"
	}
	return DiagnosisGap{
		GapKey:    prefix + "/" + string(kind) + "/" + ref.Kind + "/" + ref.ID,
		Kind:      kind,
		Dimension: string(dimension),
		Ref:       ref,
		AccountID: accountID,
		FixRoute:  gapFixRoutes[kind],
	}
}

// profileFieldGap is PR 1's report-level gap for an item nobody confirmed,
// built the same way, so the consistency dimension names the same key.
func profileFieldGap(accountID, field string) DiagnosisGap {
	return newGap("", GapProfileFieldPending, DiagnosisRecordRef{Kind: refProfileField, ID: accountID + "/" + field}, accountID)
}

// unresolvedGap is the report-level gap of a publication record no account
// could be found for: its work is not there, or its work has no topic card
// with an account - every historical import among them (FR-026).
func (set *diagnosisInputSet) unresolvedGap(publicationID string) DiagnosisGap {
	ref := DiagnosisRecordRef{Kind: refPublication, ID: publicationID}
	if _, ok := set.works[set.publications[publicationID].WorkID]; !ok {
		return newGap("", GapWorkMissing, ref, "")
	}
	return newGap("", GapAccountUnresolved, ref, "")
}

// mergeGaps adds gaps to a list, each key once - the first one found stays -
// and sorts the list by key.
func mergeGaps(list []DiagnosisGap, more []DiagnosisGap) []DiagnosisGap {
	seen := map[string]bool{}
	for _, gap := range list {
		seen[gap.GapKey] = true
	}
	for _, gap := range more {
		if !seen[gap.GapKey] {
			seen[gap.GapKey] = true
			list = append(list, gap)
		}
	}
	slices.SortFunc(list, func(left, right DiagnosisGap) int { return cmp.Compare(left.GapKey, right.GapKey) })
	return list
}

// ---------------------------------------------------------------- building one dimension

// dimensionOutput is one dimension of one section with the gaps it found and
// the reference keys of its facts (contract §5.8).
type dimensionOutput struct {
	result DimensionResult
	gaps   []DiagnosisGap
	refs   []string
}

// dimensionBuilder collects what a dimension counted, its gaps, limits,
// records and references, and writes the result in a fixed order.
type dimensionBuilder struct {
	expected, present int
	gaps              []DiagnosisGap
	limits            []string
	records           []DiagnosisRecordRef
	refs              []string
}

func (b *dimensionBuilder) gap(gap DiagnosisGap) { b.gaps = append(b.gaps, gap) }

func (b *dimensionBuilder) limit(rule DiagnosisRuleID) {
	if !slices.Contains(b.limits, rule) {
		b.limits = append(b.limits, rule)
	}
}

func (b *dimensionBuilder) record(kind, id string) {
	b.records = append(b.records, DiagnosisRecordRef{Kind: kind, ID: id})
}

func (b *dimensionBuilder) ref(key string) { b.refs = append(b.refs, key) }

func (b *dimensionBuilder) finish(result DimensionResult) dimensionOutput {
	gapKeys := []string{}
	for _, gap := range b.gaps {
		gapKeys = append(gapKeys, gap.GapKey)
	}
	slices.Sort(gapKeys)
	records := slices.Clone(b.records)
	slices.SortFunc(records, func(left, right DiagnosisRecordRef) int {
		return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
	})
	result.Completeness = DimensionCompleteness{
		Expected: b.expected, Present: b.present, GapKeys: slices.Compact(gapKeys),
	}
	result.Limits = append([]string{}, b.limits...)
	result.Records = append([]DiagnosisRecordRef{}, slices.Compact(records)...)
	return dimensionOutput{result: result, gaps: b.gaps, refs: append([]string{}, b.refs...)}
}

// ok writes an ok result with its facts.
func (b *dimensionBuilder) ok(facts any) dimensionOutput {
	encoded, err := json.Marshal(facts)
	if err != nil {
		// The facts are plain structs of strings, integers and booleans;
		// this cannot fail, and if it did the dimension is not computable
		// rather than a zero.
		return b.notComputable(DimensionNoData)
	}
	return b.finish(DimensionResult{Status: dimensionOK, Facts: encoded})
}

// notComputable writes a not_computable result with its reason (FR-012).
// The references of its facts are dropped: there are no facts to cite.
func (b *dimensionBuilder) notComputable(reason DimensionReason) dimensionOutput {
	b.refs = nil
	return b.finish(DimensionResult{Status: dimensionNotComputable, Reason: string(reason)})
}
