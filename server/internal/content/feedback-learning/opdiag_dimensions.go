package feedbacklearning

import (
	"cmp"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// The six dimensions of an operating diagnosis (specs/035 PR 2: FR-020 to
// FR-029, contract §5.3 to §5.7).
//
// Each calculator is a pure function of the stored input copies, the checked
// params and one section. What they share:
//
//   - Unknown stays unknown (D2). A metric sample with no value is counted as
//     unknown and never added in as 0; a channel with no cadence target is
//     "not set" and never compared as 0; a dimension with nothing to count is
//     not_computable with a reason, never an ok with zeros in it.
//   - Nothing is compared across platforms. Performance is grouped by
//     (platform, metric) and each group stands alone: there is no total over
//     groups, no comparison between them and no ordering by any number
//     (FR-023). Every list is ordered by a fixed key.
//   - Nothing is concluded. The server names rule ids and states counts,
//     sums, means and one difference, each with how many records it came
//     from; it writes no sentence and no cause (FR-015, FR-028).
//   - Every number is a count or a big.Int / big.Rat written as a string;
//     there is no floating point anywhere (FR-014).
//   - The dimensions are the ones a person selects, with the parameters that
//     person gives. There is no preset role or industry and no default list
//     of pillars, items or metrics (FR-004).
//
// Account attribution follows FR-026: a publication record belongs to the
// account of its work's topic card; a metric and a review to their own
// account_id; a delivery task to its review's account. Anything without one
// is the unknown_account section, and a historical import - which has no
// topic card - always is (the named limitation of contract §5.9).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §5

// dimensionCalculator computes one dimension for one section.
type dimensionCalculator func(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput

// dimensionCalculators is every dimension this build computes: all six.
var dimensionCalculators = map[DiagnosisDimension]dimensionCalculator{
	DimensionConsistency:      calculateConsistency,
	DimensionCoverage:         calculateCoverage,
	DimensionCadence:          calculateCadence,
	DimensionPerformance:      calculatePerformance,
	DimensionAudienceFeedback: calculateAudienceFeedback,
	DimensionExecutionFlow:    calculateExecutionFlow,
}

// brandSectionDimensions are the only dimensions the brand section carries:
// brand-level counts of cadence and execution. The others would read as a
// brand total, so they are not repeated there (contract §5.2).
var brandSectionDimensions = []DiagnosisDimension{DimensionCadence, DimensionExecutionFlow}

// ---------------------------------------------------------------- shared helpers

// diagParseTime reads a stored time. "" is absent; so is a time that does
// not parse, which a stored copy never holds.
func diagParseTime(text string) *time.Time {
	if text == "" {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	return &at
}

func diagParseTimePtr(text *string) *time.Time {
	if text == nil {
		return nil
	}
	return diagParseTime(*text)
}

// sectionRefName is a section as a reference key names it: account:<id>,
// unknown_account or brand.
func sectionRefName(section DiagnosisSection) string {
	if section.Section == sectionAccount {
		return sectionAccount + ":" + section.AccountID
	}
	return section.Section
}

// inSection answers whether a record with this account belongs to the
// section. The brand section holds everything in scope.
func inSection(section DiagnosisSection, accountID string) bool {
	switch section.Section {
	case sectionAccount:
		return accountID != "" && accountID == section.AccountID
	case sectionUnknownAccount:
		return accountID == ""
	default:
		return true
	}
}

func isPublished(status string) bool {
	return slices.Contains(PublishedStatuses, status)
}

// windowPublications are the section's published records whose published_at
// falls in the report window, by id.
func (set *diagnosisInputSet) windowPublications(p preparedDiagnosis, section DiagnosisSection) []string {
	ids := []string{}
	for _, id := range sortedKeys(set.publications) {
		publication := set.publications[id]
		if isPublished(publication.Status) && p.inWindow(diagParseTimePtr(publication.PublishedAt)) &&
			inSection(section, set.publicationAccount(id)) {
			ids = append(ids, id)
		}
	}
	return ids
}

// windowWorks are the works of those records that are here, by id.
func (set *diagnosisInputSet) windowWorks(p preparedDiagnosis, section DiagnosisSection) []string {
	ids := []string{}
	for _, id := range set.windowPublications(p, section) {
		workID := set.publications[id].WorkID
		if _, ok := set.works[workID]; ok && !slices.Contains(ids, workID) {
			ids = append(ids, workID)
		}
	}
	slices.Sort(ids)
	return ids
}

// unresolved adds the report-level gap of each publication record in the
// unknown_account section, so the 补录待办 says which records have no account
// and why.
func (b *dimensionBuilder) unresolved(set *diagnosisInputSet, section DiagnosisSection, publicationIDs []string) {
	if section.Section != sectionUnknownAccount {
		return
	}
	for _, id := range publicationIDs {
		b.gap(set.unresolvedGap(id))
	}
}

type markKey struct{ work, item string }

// currentMarks is the current mark of each (work, item) of one kind: the
// latest by (created_at, mark_id). Earlier marks stay in the record and are
// not counted.
func (set *diagnosisInputSet) currentMarks(kind MarkKind) map[markKey]diagWorkMarkFields {
	type dated struct {
		id   string
		at   time.Time
		mark diagWorkMarkFields
	}
	latest := map[markKey]dated{}
	for _, id := range sortedKeys(set.marks) {
		mark := set.marks[id]
		if mark.Kind != string(kind) {
			continue
		}
		var at time.Time
		if parsed := diagParseTime(mark.CreatedAt); parsed != nil {
			at = *parsed
		}
		key := markKey{mark.WorkID, mark.Item}
		current, ok := latest[key]
		if !ok || cmp.Or(at.Compare(current.at), cmp.Compare(id, current.id)) > 0 {
			latest[key] = dated{id: id, at: at, mark: mark}
		}
	}
	out := map[markKey]diagWorkMarkFields{}
	for key, entry := range latest {
		out[key] = entry.mark
	}
	return out
}

// DiagnosisNumber is a rational fact: ok with its reduced value and a
// display rounded half away from zero to two places, or not computable with
// a reason. Never a zero standing in for the second.
type DiagnosisNumber struct {
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Value   string `json:"value,omitempty"`
	Display string `json:"display,omitempty"`
}

func ratNumber(value *big.Rat) DiagnosisNumber {
	return DiagnosisNumber{Status: dimensionOK, Value: value.RatString(), Display: FormatRat(value, 2)}
}

func unknownNumber(reason DimensionReason) DiagnosisNumber {
	return DiagnosisNumber{Status: dimensionNotComputable, Reason: string(reason)}
}

// indexOr orders a value by its place in a controlled set, anything outside
// the set after it.
func indexOr[T ~string](values []T, value string) int {
	if index := slices.Index(values, T(value)); index >= 0 {
		return index
	}
	return len(values)
}

// ---------------------------------------------------------------- consistency (§5.3, FR-020)

// ConsistencyFacts are, for one account: how many works in the window were
// looked at, and per selected profile item its status and how the works were
// marked against it.
type ConsistencyFacts struct {
	Works                int                    `json:"works"`
	Items                []ConsistencyItemFacts `json:"items"`
	MarksOnOlderRevision int                    `json:"marks_on_older_revision"`
}

// ConsistencyItemFacts is one profile item: confirmed or pending, and the
// four counts of the works' current marks. Unchecked is a work nobody marked
// for this item, or whose current mark is for another account.
type ConsistencyItemFacts struct {
	Item          string `json:"item"`
	ProfileStatus string `json:"profile_status"`
	Consistent    int    `json:"consistent"`
	Inconsistent  int    `json:"inconsistent"`
	Unsure        int    `json:"unsure"`
	Unchecked     int    `json:"unchecked"`
}

func calculateConsistency(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	name := sectionRefName(section)
	if section.Section != sectionAccount {
		// No account, so no profile to hold the works against.
		b.unresolved(set, section, set.windowPublications(p, section))
		return b.notComputable(DimensionNoData)
	}
	items := p.dimensions[DimensionConsistency].Items
	if len(items) == 0 {
		return b.notComputable(DimensionMissingConfig)
	}
	accountID := section.AccountID
	profile := set.profiles[accountID]
	facts := ConsistencyFacts{Items: []ConsistencyItemFacts{}}
	for _, item := range items {
		status := "pending"
		if profile.FieldStatus[item] == ProfileFieldConfirmed {
			status = ProfileFieldConfirmed
		} else {
			b.gap(profileFieldGap(accountID, item))
		}
		facts.Items = append(facts.Items, ConsistencyItemFacts{Item: item, ProfileStatus: status})
		b.ref(name + "/consistency/" + item)
	}
	works := set.windowWorks(p, section)
	if len(works) == 0 {
		return b.notComputable(DimensionNoData)
	}
	facts.Works = len(works)
	current := set.currentMarks(MarkConsistency)
	for _, workID := range works {
		b.record(refWork, workID)
		unchecked := false
		for i, item := range items {
			mark, ok := current[markKey{workID, item}]
			if !ok || mark.AccountID != accountID {
				facts.Items[i].Unchecked++
				unchecked = true
				continue
			}
			switch MarkVerdict(mark.Verdict) {
			case VerdictConsistent:
				facts.Items[i].Consistent++
			case VerdictInconsistent:
				facts.Items[i].Inconsistent++
			default:
				facts.Items[i].Unsure++
			}
			b.present++
			if mark.ProfileRevisionID != profile.RevisionID {
				facts.MarksOnOlderRevision++
			}
		}
		if unchecked {
			b.gap(newGap(DimensionConsistency, GapWorkUnchecked, DiagnosisRecordRef{Kind: refWork, ID: workID}, accountID))
		}
	}
	b.expected = len(works) * len(items)
	if facts.MarksOnOlderRevision > 0 {
		b.limit(ruleMarksOnOlderProfile)
	}
	return b.ok(facts)
}

// ---------------------------------------------------------------- coverage (§5.3, FR-021)

// CoverageFacts are the works in the window by the pillars a person listed.
// A work may be in several pillars, so the pillar counts do not add up to
// the works; Untagged is a work in none of the listed pillars.
type CoverageFacts struct {
	Works    int                  `json:"works"`
	Pillars  []CoveragePillarFact `json:"pillars"`
	Untagged int                  `json:"untagged"`
}

// CoveragePillarFact is one listed pillar and how many works are currently
// marked as in it.
type CoveragePillarFact struct {
	Pillar string `json:"pillar"`
	Works  int    `json:"works"`
}

func calculateCoverage(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	b.limit(ruleMultiPillarNotAdditive)
	name := sectionRefName(section)
	b.unresolved(set, section, set.windowPublications(p, section))
	if len(p.pillars) == 0 {
		return b.notComputable(DimensionMissingConfig)
	}
	works := set.windowWorks(p, section)
	if len(works) == 0 {
		return b.notComputable(DimensionNoData)
	}
	facts := CoverageFacts{Works: len(works), Pillars: []CoveragePillarFact{}}
	for _, pillar := range p.pillars {
		facts.Pillars = append(facts.Pillars, CoveragePillarFact{Pillar: pillar})
		b.ref(name + "/coverage/" + pillar)
	}
	current := set.currentMarks(MarkPillar)
	for _, workID := range works {
		b.record(refWork, workID)
		tagged := false
		for i, pillar := range p.pillars {
			if mark, ok := current[markKey{workID, pillar}]; ok && mark.Verdict == string(VerdictTagged) {
				facts.Pillars[i].Works++
				tagged = true
			}
		}
		if tagged {
			b.present++
			continue
		}
		facts.Untagged++
		b.gap(newGap(DimensionCoverage, GapWorkUntagged, DiagnosisRecordRef{Kind: refWork, ID: workID}, section.AccountID))
	}
	b.expected = len(works)
	return b.ok(facts)
}

// ---------------------------------------------------------------- cadence (§5.6, FR-022)

// CadenceFacts are published records per delivery channel and ISO week of
// the brand's timezone, against the brand's weekly target for that channel.
type CadenceFacts struct {
	Channels           []CadenceChannelFacts `json:"channels"`
	PublishedAtMissing int                   `json:"published_at_missing"`
}

// CadenceChannelFacts is one channel: its target and its weeks.
type CadenceChannelFacts struct {
	Channel string             `json:"channel"`
	Target  CadenceTarget      `json:"target"`
	Weeks   []CadenceWeekFacts `json:"weeks"`
}

// CadenceTarget is the brand's operating rule for the channel: not set, or
// set to a number of pieces a week - 0 included, which is a target.
type CadenceTarget struct {
	Set     bool   `json:"set"`
	PerWeek *int64 `json:"per_week,omitempty"`
}

// CadenceWeekFacts is one ISO week. Met is there only when the week was
// compared: a complete week against a target that is set.
type CadenceWeekFacts struct {
	ISOWeek   string `json:"iso_week"`
	Start     string `json:"start"`
	Complete  bool   `json:"complete"`
	Published int    `json:"published"`
	Compared  bool   `json:"compared"`
	Met       *bool  `json:"met,omitempty"`
}

// diagWeek is one ISO week in the brand's timezone, Monday 00:00 to the next
// Monday 00:00.
type diagWeek struct {
	label      string
	start, end time.Time
	complete   bool
}

// isoWeeks are the ISO weeks the report window touches, in order. A week the
// window does not wholly cover is incomplete. Calendar steps, never a fixed
// number of hours, so a daylight-saving change does not move a week.
func (p preparedDiagnosis) isoWeeks() []diagWeek {
	sinceMonday := (int(p.from.Weekday()) + 6) % 7
	start := time.Date(p.from.Year(), p.from.Month(), p.from.Day()-sinceMonday, 0, 0, 0, 0, p.location)
	weeks := []diagWeek{}
	for start.Before(p.to) {
		end := time.Date(start.Year(), start.Month(), start.Day()+7, 0, 0, 0, 0, p.location)
		year, week := start.ISOWeek()
		weeks = append(weeks, diagWeek{
			label: fmt.Sprintf("%04d-W%02d", year, week), start: start, end: end,
			complete: !start.Before(p.from) && !end.After(p.to),
		})
		start = end
	}
	return weeks
}

// cadenceChannels are the channels a section is held against: an account's
// own platform when it is one of the four delivery channels, and all four
// for the brand and for the records without an account.
func (set *diagnosisInputSet) cadenceChannels(section DiagnosisSection) ([]string, bool) {
	if section.Section != sectionAccount {
		channels := []string{}
		for _, platform := range Platforms {
			channels = append(channels, string(platform))
		}
		return channels, true
	}
	platform := set.accounts[section.AccountID].Platform
	if !oneOf(platform, Platforms) {
		return nil, false
	}
	return []string{platform}, true
}

// operatingRules is the stored rules copy as workspace-core reads rules.
func (set *diagnosisInputSet) operatingRules() workspacecore.Rules {
	rules := workspacecore.Rules{Cadence: map[string]int64{}}
	if set.rules != nil {
		if set.rules.Cadence != nil {
			rules.Cadence = set.rules.Cadence
		}
		rules.Observation = set.rules.Observation
	}
	return rules
}

func calculateCadence(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	b.limit(ruleTargetIsBrandChannelLevel)
	name := sectionRefName(section)
	channels, ok := set.cadenceChannels(section)
	if !ok {
		return b.notComputable(DimensionNoDeliveryChannel)
	}
	rules := set.operatingRules()
	weeks := p.isoWeeks()
	facts := CadenceFacts{Channels: []CadenceChannelFacts{}}
	dated, counted := 0, []string{}
	for _, channel := range channels {
		target := CadenceTarget{}
		if perWeek, isSet := workspacecore.ReadCadence(rules, channel); isSet {
			target = CadenceTarget{Set: true, PerWeek: &perWeek}
		} else {
			b.gap(newGap(DimensionCadence, GapCadenceUnset, DiagnosisRecordRef{Kind: refChannel, ID: channel}, ""))
		}
		counts := make([]int, len(weeks))
		for _, id := range sortedKeys(set.publications) {
			publication := set.publications[id]
			if publication.Channel != channel || !isPublished(publication.Status) ||
				!inSection(section, set.publicationAccount(id)) {
				continue
			}
			at := diagParseTimePtr(publication.PublishedAt)
			if at == nil {
				// Published, but nobody knows when: in no week, and a gap.
				facts.PublishedAtMissing++
				counted = append(counted, id)
				b.gap(newGap(DimensionCadence, GapPublishedAtMissing, DiagnosisRecordRef{Kind: refPublication, ID: id}, section.AccountID))
				continue
			}
			if !p.inWindow(at) {
				continue
			}
			local := at.In(p.location)
			for i, week := range weeks {
				if !local.Before(week.start) && local.Before(week.end) {
					counts[i]++
				}
			}
			dated++
			counted = append(counted, id)
			b.record(refPublication, id)
		}
		channelFacts := CadenceChannelFacts{Channel: channel, Target: target, Weeks: []CadenceWeekFacts{}}
		for i, week := range weeks {
			weekFacts := CadenceWeekFacts{
				ISOWeek: week.label, Start: week.start.Format(time.DateOnly), Complete: week.complete,
				Published: counts[i], Compared: week.complete && target.Set,
			}
			if weekFacts.Compared {
				met := int64(counts[i]) >= *target.PerWeek
				weekFacts.Met = &met
			}
			if !week.complete {
				b.limit(ruleIncompleteWeekNotCompared)
			}
			channelFacts.Weeks = append(channelFacts.Weeks, weekFacts)
			b.ref(name + "/cadence/" + channel + "/" + week.label)
		}
		facts.Channels = append(facts.Channels, channelFacts)
	}
	slices.Sort(counted)
	b.unresolved(set, section, counted)
	b.expected, b.present = dated+facts.PublishedAtMissing, dated
	if dated == 0 {
		// With no dated record in the window, "0 this week" and "published
		// but not recorded" cannot be told apart.
		return b.notComputable(DimensionNoData)
	}
	return b.ok(facts)
}

// ---------------------------------------------------------------- performance (§5.4, FR-023, ruling Q7)

// PerformanceFacts are the groups (platform, metric). Each group stands
// alone: there is no total across groups and no order by any number.
type PerformanceFacts struct {
	Groups []PerformanceGroup `json:"groups"`
}

// PerformanceGroup is one platform's one metric in the two windows, and the
// difference between their means when both can be computed.
type PerformanceGroup struct {
	Platform        string                `json:"platform"`
	Metric          string                `json:"metric"`
	Current         PerformanceWindowFact `json:"current"`
	Baseline        PerformanceWindowFact `json:"baseline"`
	Change          DiagnosisNumber       `json:"change"`
	StatWindows     []string              `json:"stat_windows"`
	StatWindowMixed bool                  `json:"stat_window_mixed"`
}

// PerformanceWindowFact is one window of one group, over the last sample of
// each publication record taken no later than the report's generation.
// Publications counts every record with a sample, the unknown ones too; Sum
// is null when no sample has a value - an unknown, not a 0.
type PerformanceWindowFact struct {
	Publications int             `json:"publications"`
	WithValue    int             `json:"with_value"`
	Unknown      int             `json:"unknown"`
	Sum          *string         `json:"sum"`
	Mean         DiagnosisNumber `json:"mean"`
}

type perfGroupKey struct{ platform, metric string }

// perfSlot is one publication record in one window of one group.
type perfSlot struct {
	group   perfGroupKey
	current bool
	pubID   string
}

// perfSample is a candidate for the last sample of a slot.
type perfSample struct {
	id                   string
	sampledAt, createdAt time.Time
	fields               diagMetricFields
}

// later answers whether a sample was taken after another: by sampled_at,
// then created_at, then id (ruling Q7).
func (s perfSample) later(other perfSample) bool {
	return cmp.Or(s.sampledAt.Compare(other.sampledAt), s.createdAt.Compare(other.createdAt),
		cmp.Compare(s.id, other.id)) > 0
}

// perfWindow adds up one window of one group.
type perfWindow struct {
	publications, withValue, unknown int
	sum                              *big.Int
}

func (w perfWindow) fact() (PerformanceWindowFact, *big.Rat) {
	fact := PerformanceWindowFact{Publications: w.publications, WithValue: w.withValue, Unknown: w.unknown,
		Mean: unknownNumber(DimensionNoData)}
	if w.withValue == 0 {
		return fact, nil
	}
	sum := w.sum.String()
	fact.Sum = &sum
	mean := new(big.Rat).SetFrac(w.sum, big.NewInt(int64(w.withValue)))
	fact.Mean = ratNumber(mean)
	return fact, mean
}

func calculatePerformance(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	b.limit(ruleDifferenceIsNotCause)
	b.limit(ruleSamplesAtDifferentAges)
	name := sectionRefName(section)
	params := p.dimensions[DimensionPerformance]
	windowIDs := set.windowPublications(p, section)
	b.unresolved(set, section, windowIDs)
	if len(params.Metrics) == 0 {
		return b.notComputable(DimensionMissingConfig)
	}
	if !p.hasComparison {
		return b.notComputable(DimensionMissingComparisonWindow)
	}

	// The last sample per slot. A sample taken after the report was
	// generated is not one it could have read.
	last := map[perfSlot]perfSample{}
	sampled := map[string]bool{}
	for _, id := range sortedKeys(set.metrics) {
		metric := set.metrics[id]
		if !slices.Contains(params.Metrics, Metric(metric.Metric)) ||
			(len(params.Platforms) > 0 && !slices.Contains(params.Platforms, Platform(metric.Platform))) {
			continue
		}
		sampledAt := diagParseTime(metric.SampledAt)
		if sampledAt == nil || (!p.generatedAt.IsZero() && sampledAt.After(p.generatedAt)) {
			continue
		}
		sampled[metric.PublicationRecordID] = true
		publication, ok := set.publications[metric.PublicationRecordID]
		if !ok || !isPublished(publication.Status) || !inSection(section, metric.AccountID) {
			continue
		}
		at := diagParseTimePtr(publication.PublishedAt)
		current := p.inWindow(at)
		if !current && !p.inComparison(at) {
			continue
		}
		sample := perfSample{id: id, sampledAt: *sampledAt, fields: metric}
		if createdAt := diagParseTime(metric.CreatedAt); createdAt != nil {
			sample.createdAt = *createdAt
		}
		slot := perfSlot{group: perfGroupKey{metric.Platform, metric.Metric}, current: current, pubID: metric.PublicationRecordID}
		if previous, seen := last[slot]; !seen || sample.later(previous) {
			last[slot] = sample
		}
	}

	// A record in the window with no sample of the selected metrics at all:
	// one gap per record, not one per metric.
	for _, id := range windowIDs {
		b.expected++
		if sampled[id] {
			b.present++
			continue
		}
		b.gap(newGap(DimensionPerformance, GapMetricMissing, DiagnosisRecordRef{Kind: refPublication, ID: id}, section.AccountID))
	}
	if len(last) == 0 {
		return b.notComputable(DimensionNoData)
	}

	type groupSums struct {
		current, baseline perfWindow
		statWindows       []string
	}
	groups := map[perfGroupKey]*groupSums{}
	for slot, sample := range last {
		sums := groups[slot.group]
		if sums == nil {
			sums = &groupSums{current: perfWindow{sum: new(big.Int)}, baseline: perfWindow{sum: new(big.Int)}}
			groups[slot.group] = sums
		}
		window := &sums.baseline
		if slot.current {
			window = &sums.current
		}
		window.publications++
		if value := sample.fields.Value; value != nil {
			window.withValue++
			window.sum.Add(window.sum, big.NewInt(*value))
		} else {
			window.unknown++
		}
		spelling := strings.TrimSpace(norm.NFC.String(sample.fields.StatWindow))
		if !slices.Contains(sums.statWindows, spelling) {
			sums.statWindows = append(sums.statWindows, spelling)
		}
		b.record(refMetric, sample.id)
		b.record(refPublication, slot.pubID)
	}

	keys := make([]perfGroupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(left, right perfGroupKey) int {
		return cmp.Or(cmp.Compare(indexOr(Platforms, left.platform), indexOr(Platforms, right.platform)),
			cmp.Compare(left.platform, right.platform),
			cmp.Compare(indexOr(Metrics, left.metric), indexOr(Metrics, right.metric)),
			cmp.Compare(left.metric, right.metric))
	})
	facts := PerformanceFacts{Groups: []PerformanceGroup{}}
	for _, key := range keys {
		sums := groups[key]
		current, currentMean := sums.current.fact()
		baseline, baselineMean := sums.baseline.fact()
		change := unknownNumber(DimensionNoData)
		if currentMean != nil && baselineMean != nil {
			change = ratNumber(new(big.Rat).Sub(currentMean, baselineMean))
		}
		slices.Sort(sums.statWindows)
		group := PerformanceGroup{
			Platform: key.platform, Metric: key.metric, Current: current, Baseline: baseline, Change: change,
			StatWindows: sums.statWindows, StatWindowMixed: len(sums.statWindows) > 1,
		}
		if group.StatWindowMixed {
			b.limit(ruleStatWindowMixed)
		}
		facts.Groups = append(facts.Groups, group)
		b.ref(name + "/performance/" + key.platform + "/" + key.metric)
	}
	return b.ok(facts)
}

// ---------------------------------------------------------------- audience feedback (§5.5, FR-024)

// AudienceFeedbackFacts count the excerpts in the window by where they came
// from and by the tags a person gave them. Tags are only trimmed and put in
// NFC: no case folding, no merging of words that mean the same, no sentiment
// and no keywords taken out of the text.
type AudienceFeedbackFacts struct {
	Excerpts int                   `json:"excerpts"`
	BySource []AudienceSourceCount `json:"by_source"`
	ByTag    []AudienceTagCount    `json:"by_tag"`
	Untagged int                   `json:"untagged"`
}

// AudienceSourceCount is one (platform, source type) and its excerpts.
type AudienceSourceCount struct {
	Platform   string `json:"platform"`
	SourceType string `json:"source_type"`
	Excerpts   int    `json:"excerpts"`
}

// AudienceTagCount is one tag and the excerpts that carry it. An excerpt
// with two tags is in both, so these do not add up to the excerpts.
type AudienceTagCount struct {
	Tag      string `json:"tag"`
	Excerpts int    `json:"excerpts"`
}

// normalizeTag is the one normalization a tag gets.
func normalizeTag(tag string) string {
	return strings.TrimSpace(norm.NFC.String(tag))
}

func calculateAudienceFeedback(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	b.limit(ruleTagsNotAdditive)
	name := sectionRefName(section)
	sources := p.dimensions[DimensionAudienceFeedback].Sources
	type sourceKey struct{ platform, source string }
	bySource, byTag := map[sourceKey]int{}, map[string]int{}
	facts := AudienceFeedbackFacts{BySource: []AudienceSourceCount{}, ByTag: []AudienceTagCount{}}
	publications := []string{}
	for _, id := range sortedKeys(set.excerpts) {
		excerpt := set.excerpts[id]
		publication, ok := set.publications[excerpt.PublicationRecordID]
		if !ok || !inSection(section, set.publicationAccount(excerpt.PublicationRecordID)) ||
			!p.inWindow(diagParseTime(excerpt.OccurredAt)) ||
			(len(sources) > 0 && !slices.Contains(sources, ExcerptSource(excerpt.SourceType))) {
			continue
		}
		facts.Excerpts++
		b.record(refExcerpt, id)
		if !slices.Contains(publications, excerpt.PublicationRecordID) {
			publications = append(publications, excerpt.PublicationRecordID)
		}
		bySource[sourceKey{publication.Channel, excerpt.SourceType}]++
		tags := []string{}
		for _, tag := range excerpt.Tags {
			if tag = normalizeTag(tag); tag != "" && !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
		for _, tag := range tags {
			byTag[tag]++
		}
		if len(tags) == 0 {
			facts.Untagged++
			b.gap(newGap(DimensionAudienceFeedback, GapExcerptUntagged, DiagnosisRecordRef{Kind: refExcerpt, ID: id}, section.AccountID))
		}
	}
	slices.Sort(publications)
	b.unresolved(set, section, publications)
	b.expected, b.present = facts.Excerpts, facts.Excerpts-facts.Untagged
	if facts.Excerpts == 0 {
		// No excerpt is not "0 excerpts, all fine": nothing was recorded.
		return b.notComputable(DimensionNoData)
	}
	for key, excerpts := range bySource {
		facts.BySource = append(facts.BySource, AudienceSourceCount{Platform: key.platform, SourceType: key.source, Excerpts: excerpts})
	}
	slices.SortFunc(facts.BySource, func(left, right AudienceSourceCount) int {
		return cmp.Or(cmp.Compare(indexOr(Platforms, left.Platform), indexOr(Platforms, right.Platform)),
			cmp.Compare(left.Platform, right.Platform),
			cmp.Compare(indexOr(ExcerptSources, left.SourceType), indexOr(ExcerptSources, right.SourceType)),
			cmp.Compare(left.SourceType, right.SourceType))
	})
	for _, tag := range sortedKeys(byTag) {
		facts.ByTag = append(facts.ByTag, AudienceTagCount{Tag: tag, Excerpts: byTag[tag]})
		b.ref(name + "/audience_feedback/tag/" + tag)
	}
	return b.ok(facts)
}

// ---------------------------------------------------------------- execution flow (§5.7, FR-025)

// ExecutionFlowFacts list what is waiting, held, due or went wrong, as of
// the report's generation. Nothing is compared with a number of days: the
// waiting time is stated, and nothing here decides it is too long.
type ExecutionFlowFacts struct {
	ReviewsPending                        ReviewWaitList      `json:"reviews_pending"`
	ReviewsChangesRequested               ReviewWaitList      `json:"reviews_changes_requested"`
	ReviewsRejectedInWindow               ReviewList          `json:"reviews_rejected_in_window"`
	DeliveriesHeld                        DeliveryTaskList    `json:"deliveries_held"`
	DeliveriesDue                         DeliveryTaskList    `json:"deliveries_due"`
	PublicationsFailed                    PublicationList     `json:"publications_failed"`
	PublicationsUnknown                   PublicationList     `json:"publications_unknown"`
	PublicationsRemoved                   PublicationList     `json:"publications_removed"`
	PublicationsMissingMetricsAfterWindow ObservationListFact `json:"publications_missing_metrics_after_window"`
}

// ReviewWaitList is open reviews, each with the whole days it has waited in
// the brand's timezone.
type ReviewWaitList struct {
	Count int              `json:"count"`
	Items []ReviewWaitItem `json:"items"`
}

// ReviewWaitItem is one open review.
type ReviewWaitItem struct {
	ReviewRequestID string `json:"review_request_id"`
	AccountID       string `json:"account_id"`
	WaitingDays     int    `json:"waiting_days"`
}

// ReviewList is reviews decided in the window.
type ReviewList struct {
	Count int          `json:"count"`
	Items []ReviewItem `json:"items"`
}

// ReviewItem is one review.
type ReviewItem struct {
	ReviewRequestID string `json:"review_request_id"`
	AccountID       string `json:"account_id"`
}

// DeliveryTaskList is delivery tasks in one state.
type DeliveryTaskList struct {
	Count int                `json:"count"`
	Items []DeliveryTaskItem `json:"items"`
}

// DeliveryTaskItem is one delivery task.
type DeliveryTaskItem struct {
	DeliveryTaskID  string `json:"delivery_task_id"`
	ReviewRequestID string `json:"review_request_id"`
}

// PublicationList is publication records at one latest status.
type PublicationList struct {
	Count int               `json:"count"`
	Items []PublicationItem `json:"items"`
}

// PublicationItem is one publication record.
type PublicationItem struct {
	PublicationRecordID string `json:"publication_record_id"`
	Channel             string `json:"channel"`
}

// ObservationListFact is the published records whose observation window has
// passed with no metric recorded, per channel. A channel with no observation
// window cannot answer, and then neither can the whole fact: a partial list
// would read as a complete one.
type ObservationListFact struct {
	Status   string               `json:"status"`
	Reason   string               `json:"reason,omitempty"`
	Channels []ObservationChannel `json:"channels"`
}

// ObservationChannel is one channel of that fact.
type ObservationChannel struct {
	Channel string            `json:"channel"`
	Status  string            `json:"status"`
	Reason  string            `json:"reason,omitempty"`
	Count   int               `json:"count"`
	Items   []PublicationItem `json:"items"`
}

// executionFlowFactNames are the facts a reference key may name.
var executionFlowFactNames = []string{
	"reviews_pending", "reviews_changes_requested", "reviews_rejected_in_window", "deliveries_held",
	"deliveries_due", "publications_failed", "publications_unknown", "publications_removed",
	"publications_missing_metrics_after_window",
}

// wholeDays is the number of calendar days between two times in the
// brand's timezone: the difference of their local dates. It is a length, not
// a limit.
func wholeDays(from, to time.Time, location *time.Location) int {
	date := func(at time.Time) time.Time {
		year, month, day := at.In(location).Date()
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	return int(date(to).Sub(date(from)) / (time.Hour * 24))
}

func calculateExecutionFlow(set *diagnosisInputSet, p preparedDiagnosis, section DiagnosisSection) dimensionOutput {
	b := &dimensionBuilder{}
	b.limit(ruleNoThreshold)
	name := sectionRefName(section)
	facts := ExecutionFlowFacts{
		ReviewsPending:                        ReviewWaitList{Items: []ReviewWaitItem{}},
		ReviewsChangesRequested:               ReviewWaitList{Items: []ReviewWaitItem{}},
		ReviewsRejectedInWindow:               ReviewList{Items: []ReviewItem{}},
		DeliveriesHeld:                        DeliveryTaskList{Items: []DeliveryTaskItem{}},
		DeliveriesDue:                         DeliveryTaskList{Items: []DeliveryTaskItem{}},
		PublicationsFailed:                    PublicationList{Items: []PublicationItem{}},
		PublicationsUnknown:                   PublicationList{Items: []PublicationItem{}},
		PublicationsRemoved:                   PublicationList{Items: []PublicationItem{}},
		PublicationsMissingMetricsAfterWindow: ObservationListFact{Status: dimensionOK, Channels: []ObservationChannel{}},
	}
	unknownSection := section.Section == sectionUnknownAccount

	for _, id := range sortedKeys(set.reviews) {
		review := set.reviews[id]
		if !inSection(section, review.AccountID) {
			continue
		}
		wait := func(list *ReviewWaitList) {
			item := ReviewWaitItem{ReviewRequestID: id, AccountID: review.AccountID}
			if requested := diagParseTime(review.RequestedAt); requested != nil {
				item.WaitingDays = wholeDays(*requested, p.generatedAt, p.location)
			}
			list.Items = append(list.Items, item)
			list.Count++
		}
		switch review.Status {
		case openReviewStatuses[0]:
			wait(&facts.ReviewsPending)
		case openReviewStatuses[1]:
			wait(&facts.ReviewsChangesRequested)
		case rejectedReviewStatus:
			if !p.inWindow(diagParseTimePtr(review.DecidedAt)) {
				continue
			}
			facts.ReviewsRejectedInWindow.Items = append(facts.ReviewsRejectedInWindow.Items,
				ReviewItem{ReviewRequestID: id, AccountID: review.AccountID})
			facts.ReviewsRejectedInWindow.Count++
		default:
			continue
		}
		b.record(refReview, id)
		if unknownSection {
			b.gap(newGap("", GapAccountUnresolved, DiagnosisRecordRef{Kind: refReview, ID: id}, ""))
		}
	}

	for _, id := range sortedKeys(set.tasks) {
		task := set.tasks[id]
		if !inSection(section, set.taskAccount(id)) {
			continue
		}
		item := DeliveryTaskItem{DeliveryTaskID: id, ReviewRequestID: task.ReviewRequestID}
		listed := false
		if task.Status == "held" {
			facts.DeliveriesHeld.Items = append(facts.DeliveriesHeld.Items, item)
			facts.DeliveriesHeld.Count++
			listed = true
		}
		// Due is review-delivery's own read-time derivation, taken as it
		// answered and never worked out again here.
		if task.Due {
			facts.DeliveriesDue.Items = append(facts.DeliveriesDue.Items, item)
			facts.DeliveriesDue.Count++
			listed = true
		}
		if !listed {
			continue
		}
		b.record(refDeliveryTask, id)
		if unknownSection {
			b.gap(newGap("", GapAccountUnresolved, DiagnosisRecordRef{Kind: refDeliveryTask, ID: id}, ""))
		}
	}

	listed := []string{}
	for _, id := range sortedKeys(set.publications) {
		publication := set.publications[id]
		if !inSection(section, set.publicationAccount(id)) {
			continue
		}
		var list *PublicationList
		switch publication.Status {
		case "failed":
			list = &facts.PublicationsFailed
		case "unknown":
			list = &facts.PublicationsUnknown
			b.gap(newGap(DimensionExecutionFlow, GapPublicationStatusUnknown,
				DiagnosisRecordRef{Kind: refPublication, ID: id}, section.AccountID))
		case "removed":
			list = &facts.PublicationsRemoved
		default:
			continue
		}
		list.Items = append(list.Items, PublicationItem{PublicationRecordID: id, Channel: publication.Channel})
		list.Count++
		listed = append(listed, id)
		b.record(refPublication, id)
	}

	// Published in the window, no metric at all, and past the brand's
	// observation window for its channel - asked of workspace-core, the one
	// definition of "due", with the generation time as now.
	rules := set.operatingRules()
	withMetric := map[string]bool{}
	for _, metric := range set.metrics {
		withMetric[metric.PublicationRecordID] = true
	}
	channels := map[string]*ObservationChannel{}
	window := set.windowPublications(p, section)
	for _, id := range window {
		publication := set.publications[id]
		channel := channels[publication.Channel]
		if channel == nil {
			channel = &ObservationChannel{Channel: publication.Channel, Status: dimensionOK, Items: []PublicationItem{}}
			channels[publication.Channel] = channel
			if _, source := workspacecore.ReadObservation(rules, publication.Channel); source == workspacecore.ObservationUnset {
				channel.Status, channel.Reason = dimensionNotComputable, string(DimensionMissingObservationWindow)
				b.gap(newGap(DimensionExecutionFlow, GapObservationUnset, DiagnosisRecordRef{Kind: refChannel, ID: publication.Channel}, ""))
			}
		}
		b.expected++
		if withMetric[id] {
			b.present++
			continue
		}
		if channel.Status != dimensionOK {
			continue
		}
		due := workspacecore.ObservationDueFor(rules, publication.Channel, diagParseTimePtr(publication.PublishedAt), p.generatedAt)
		if due == workspacecore.DuePassed {
			channel.Items = append(channel.Items, PublicationItem{PublicationRecordID: id, Channel: publication.Channel})
			channel.Count++
			b.record(refPublication, id)
		}
	}
	missing := &facts.PublicationsMissingMetricsAfterWindow
	for _, channel := range sortedKeys(channels) {
		fact := channels[channel]
		missing.Channels = append(missing.Channels, *fact)
		if fact.Status != dimensionOK {
			missing.Status, missing.Reason = dimensionNotComputable, string(DimensionMissingObservationWindow)
		}
	}
	slices.SortFunc(missing.Channels, func(left, right ObservationChannel) int {
		return cmp.Or(cmp.Compare(indexOr(Platforms, left.Channel), indexOr(Platforms, right.Channel)),
			cmp.Compare(left.Channel, right.Channel))
	})
	b.unresolved(set, section, append(listed, window...))
	for _, fact := range executionFlowFactNames {
		b.ref(name + "/execution_flow/" + fact)
	}
	return b.ok(facts)
}
