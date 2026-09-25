package feedbacklearning

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// specs/035 PR 2 without a database: the six dimensions, completeness, the
// 补录待办 and the ROI reference (T038 to T050, T051 in part, T053 in part;
// SC-001 to SC-003, SC-005; D14-V04).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §5

// ---------------------------------------------------------------- building inputs

// diagSample builds stored input copies directly, the way a version holds
// them, so each case says exactly what the calculator is given.
type diagSample struct {
	t      *testing.T
	inputs []DiagnosisInput
	seen   map[string]bool
}

func newDiagSample(t *testing.T) *diagSample {
	return &diagSample{t: t, seen: map[string]bool{}}
}

func (s *diagSample) add(kind, id string, fields any) *diagSample {
	s.t.Helper()
	if s.seen[kind+"/"+id] {
		return s
	}
	s.seen[kind+"/"+id] = true
	input, err := newDiagnosisInput(kind, id, fields)
	if err != nil {
		s.t.Fatal(err)
	}
	s.inputs = append(s.inputs, input)
	return s
}

func stamp(text string) string { return diagTime(*diagAt(text)) }

func stampPtr(text string) *string {
	if text == "" {
		return nil
	}
	value := stamp(text)
	return &value
}

func diagCount(n int64) *int64 { return &n }

// account adds an account and its profile: every item confirmed except the
// ones named pending.
func (s *diagSample) account(id, platform, name string, pending ...string) *diagSample {
	status := map[string]string{}
	for _, key := range profileFieldKeys {
		status[key] = ProfileFieldConfirmed
		if slices.Contains(pending, key) {
			status[key] = "pending"
		}
	}
	s.add(inputAccount, id, diagAccountFields{Platform: platform, DisplayName: name})
	return s.add(inputProfile, id, diagProfileFields{RevisionID: "pr-" + id, FieldStatus: status})
}

func (s *diagSample) rules(rules DiagOperatingRules) *diagSample {
	if rules.Cadence == nil {
		rules.Cadence = map[string]int64{}
	}
	return s.add(inputOperatingRules, operatingRulesInputID, rules)
}

// published adds a publication record with its work and, when accountID is
// not "", a topic card for that account. publishedAt "" is no published_at.
func (s *diagSample) published(id, workID, accountID, channel, status, publishedAt string) *diagSample {
	card := ""
	if accountID != "" {
		card = "card-" + workID
		s.add(inputTopicCard, card, diagTopicCardFields{AccountID: accountID})
	}
	s.add(inputWork, workID, diagWorkFields{TopicCardID: card, Title: workID})
	return s.add(inputPublication, id, diagPublicationFields{WorkID: workID, Channel: channel, Status: status,
		PublishedAt: stampPtr(publishedAt)})
}

func (s *diagSample) historical(id, workID, channel, publishedAt string) *diagSample {
	s.add(inputWork, workID, diagWorkFields{HistoricalImport: true, Title: workID})
	return s.add(inputPublication, id, diagPublicationFields{WorkID: workID, Channel: channel,
		Status: "verified_published", PublishedAt: stampPtr(publishedAt)})
}

func (s *diagSample) metric(id, publicationID, platform, accountID string, metric Metric, value *int64, statWindow, sampledAt string) *diagSample {
	return s.add(inputMetric, id, diagMetricFields{PublicationRecordID: publicationID, Platform: platform,
		AccountID: accountID, Metric: string(metric), Value: value, StatWindow: statWindow,
		SampledAt: stamp(sampledAt), CreatedAt: stamp(sampledAt)})
}

func (s *diagSample) excerpt(id, publicationID string, source ExcerptSource, occurredAt string, tags ...string) *diagSample {
	if tags == nil {
		tags = []string{}
	}
	return s.add(inputExcerpt, id, diagExcerptFields{PublicationRecordID: publicationID, SourceType: string(source),
		Tags: tags, OccurredAt: stamp(occurredAt)})
}

func (s *diagSample) mark(id, workID string, kind MarkKind, item string, verdict MarkVerdict, accountID, revision, createdAt string) *diagSample {
	return s.add(inputWorkMark, id, diagWorkMarkFields{WorkID: workID, Kind: string(kind), Item: item,
		Verdict: string(verdict), AccountID: accountID, ProfileRevisionID: revision, CreatedAt: stamp(createdAt)})
}

func (s *diagSample) review(id, accountID, status, requestedAt, decidedAt string) *diagSample {
	return s.add(inputReview, id, diagReviewFields{AccountID: accountID, Status: status,
		RequestedAt: stamp(requestedAt), DecidedAt: stampPtr(decidedAt)})
}

func (s *diagSample) task(id, reviewID, status string, due bool) *diagSample {
	return s.add(inputDeliveryTask, id, diagDeliveryTaskFields{ReviewRequestID: reviewID, Status: status, Due: due})
}

// sampleParams is September 2026 in Asia/Shanghai against August, over the
// given dimensions, generated at 2026-10-02 09:00 local.
func opdiagParams(kind DiagnosisScope, dimensions []DiagnosisDimensionParam, accountIDs ...string) DiagnosisParams {
	params := diagParams(kind, accountIDs...)
	params.Dimensions = dimensions
	params.ComparisonWindow = &DiagnosisDateRange{Start: "2026-08-01", End: "2026-08-31"}
	return params
}

func allSix() []DiagnosisDimensionParam {
	return []DiagnosisDimensionParam{
		{Key: DimensionConsistency, Items: []string{"positioning", "expression_style"}},
		{Key: DimensionCoverage, Pillars: []string{"穿搭", "面料知识"}},
		{Key: DimensionCadence},
		{Key: DimensionPerformance, Metrics: []Metric{MetricImpression, MetricPlay, MetricFavorite}},
		{Key: DimensionAudienceFeedback},
		{Key: DimensionExecutionFlow},
	}
}

func only(dimension DiagnosisDimensionParam) []DiagnosisDimensionParam {
	return []DiagnosisDimensionParam{dimension}
}

func mustCalculate(t *testing.T, params DiagnosisParams, inputs []DiagnosisInput) DiagnosisResult {
	t.Helper()
	result, err := CalculateDiagnosis(params, inputs)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// sectionOf finds a section by its reference name.
func sectionOf(t *testing.T, result DiagnosisResult, name string) DiagnosisSection {
	t.Helper()
	for _, section := range result.Sections {
		if sectionRefName(section) == name {
			return section
		}
	}
	t.Fatalf("no section %s in %+v", name, result.Sections)
	return DiagnosisSection{}
}

func dimensionOf(t *testing.T, result DiagnosisResult, section string, dimension DiagnosisDimension) DimensionResult {
	t.Helper()
	found, ok := sectionOf(t, result, section).Dimensions[dimension]
	if !ok {
		t.Fatalf("section %s has no %s", section, dimension)
	}
	return found
}

func factsOf[T any](t *testing.T, dimension DimensionResult) T {
	t.Helper()
	var facts T
	if dimension.Status != dimensionOK {
		t.Fatalf("dimension is %s/%s, not ok", dimension.Status, dimension.Reason)
	}
	if err := json.Unmarshal(dimension.Facts, &facts); err != nil {
		t.Fatal(err)
	}
	return facts
}

func wantNotComputable(t *testing.T, dimension DimensionResult, reason DimensionReason) {
	t.Helper()
	if dimension.Status != dimensionNotComputable || dimension.Reason != string(reason) || len(dimension.Facts) != 0 {
		t.Fatalf("dimension = %s/%s %s, want not_computable/%s and no facts", dimension.Status, dimension.Reason,
			dimension.Facts, reason)
	}
}

func hasGap(result DiagnosisResult, kind GapKind, refID string) bool {
	return slices.ContainsFunc(result.Gaps, func(gap DiagnosisGap) bool { return gap.Kind == kind && gap.Ref.ID == refID })
}

func onlyGroup(t *testing.T, facts PerformanceFacts, platform, metric string) PerformanceGroup {
	t.Helper()
	for _, group := range facts.Groups {
		if group.Platform == platform && group.Metric == metric {
			return group
		}
	}
	t.Fatalf("no group %s/%s in %+v", platform, metric, facts.Groups)
	return PerformanceGroup{}
}

// ---------------------------------------------------------------- T038: contract §5.10, verbatim

type fixedSample struct {
	name   string
	build  func(t *testing.T) (DiagnosisParams, []DiagnosisInput)
	expect func(t *testing.T, result DiagnosisResult)
}

// fixedSamples are contract §5.10's cases, named as it names them. The
// fixtures are September 2026 in Asia/Shanghai: 09-01 is a Tuesday, so the
// first ISO week (W36) is incomplete and W37 is 09-07 to 09-13.
func fixedSamples() []fixedSample {
	performance := func(metrics ...Metric) []DiagnosisDimensionParam {
		return only(DiagnosisDimensionParam{Key: DimensionPerformance, Metrics: metrics})
	}
	return []fixedSample{
		{
			name: "empty-scope",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号", "positioning").rules(DiagOperatingRules{})
				return opdiagParams(ScopeAccount, allSix(), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				for _, dimension := range []DiagnosisDimension{DimensionConsistency, DimensionCoverage, DimensionCadence,
					DimensionPerformance, DimensionAudienceFeedback} {
					wantNotComputable(t, dimensionOf(t, result, "account:a1", dimension), DimensionNoData)
				}
				facts := factsOf[ExecutionFlowFacts](t, dimensionOf(t, result, "account:a1", DimensionExecutionFlow))
				if facts.ReviewsPending.Count+facts.DeliveriesHeld.Count+facts.PublicationsFailed.Count != 0 ||
					len(facts.PublicationsMissingMetricsAfterWindow.Channels) != 0 {
					t.Fatalf("execution flow = %+v", facts)
				}
				if !hasGap(result, GapProfileFieldPending, "a1/positioning") {
					t.Fatalf("no configuration gap: %+v", result.Gaps)
				}
			},
		},
		{
			name: "nil-vs-zero",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
					published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-11T10:00:00+08:00").
					metric("m1", "p1", "xiaohongshu", "a1", MetricFavorite, nil, "发布后 7 天", "2026-09-17T10:00:00+08:00").
					metric("m2", "p2", "xiaohongshu", "a1", MetricFavorite, diagCount(0), "发布后 7 天", "2026-09-18T10:00:00+08:00")
				return opdiagParams(ScopeAccount, performance(MetricFavorite), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				group := onlyGroup(t, factsOf[PerformanceFacts](t, dimensionOf(t, result, "account:a1", DimensionPerformance)),
					"xiaohongshu", "favorite")
				current := group.Current
				if current.Publications != 2 || current.WithValue != 1 || current.Unknown != 1 || current.Sum == nil ||
					*current.Sum != "0" || current.Mean.Display != "0.00" || current.Mean.Status != dimensionOK {
					t.Fatalf("current = %+v", current)
				}
				// Nothing in August: not a zero, an unknown - and so is the change.
				if group.Baseline.Sum != nil || group.Baseline.Mean.Status != dimensionNotComputable ||
					group.Change.Status != dimensionNotComputable {
					t.Fatalf("baseline %+v, change %+v", group.Baseline, group.Change)
				}
			},
		},
		{
			name: "latest-sample",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-01T10:00:00+08:00").
					metric("m-0902", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(10), "", "2026-09-02T10:00:00+08:00").
					metric("m-0909", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(25), "", "2026-09-09T10:00:00+08:00")
				params := opdiagParams(ScopeAccount, performance(MetricFavorite), "a1")
				params.GeneratedAt = "2026-09-05T01:00:00Z"
				return params, s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				group := onlyGroup(t, factsOf[PerformanceFacts](t, dimensionOf(t, result, "account:a1", DimensionPerformance)),
					"xiaohongshu", "favorite")
				if group.Current.Sum == nil || *group.Current.Sum != "10" || group.Current.Publications != 1 {
					t.Fatalf("current = %+v, want the 09-02 sample only", group.Current)
				}
			},
		},
		{
			name: "two-platforms",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p-xhs", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
					published("p-dy", "w2", "a1", "douyin", "verified_published", "2026-09-10T11:00:00+08:00").
					metric("m1", "p-xhs", "xiaohongshu", "a1", MetricImpression, diagCount(100), "", "2026-09-17T10:00:00+08:00").
					metric("m2", "p-dy", "douyin", "a1", MetricPlay, diagCount(300), "", "2026-09-17T10:00:00+08:00")
				return opdiagParams(ScopeAccount, performance(MetricImpression, MetricPlay), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionPerformance)
				facts := factsOf[PerformanceFacts](t, dimension)
				if len(facts.Groups) != 2 {
					t.Fatalf("groups = %+v, want two", facts.Groups)
				}
				if got := onlyGroup(t, facts, "xiaohongshu", "impression").Current.Sum; got == nil || *got != "100" {
					t.Fatalf("xiaohongshu impression sum = %v", got)
				}
				if got := onlyGroup(t, facts, "douyin", "play").Current.Sum; got == nil || *got != "300" {
					t.Fatalf("douyin play sum = %v", got)
				}
				// Xiaohongshu is listed first because it is first in Platforms,
				// not because of any number.
				if facts.Groups[0].Platform != "xiaohongshu" {
					t.Fatalf("groups in order %+v", facts.Groups)
				}
				// And nowhere is there a sum over the two.
				if strings.Contains(string(dimension.Facts), `"400"`) {
					t.Fatalf("the two platforms were added together: %s", dimension.Facts)
				}
			},
		},
		{
			name: "stat-window-mixed",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
					published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-11T10:00:00+08:00").
					metric("m1", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(5), "发布后 7 天", "2026-09-17T10:00:00+08:00").
					metric("m2", "p2", "xiaohongshu", "a1", MetricFavorite, diagCount(6), "发布后 14 天", "2026-09-25T10:00:00+08:00")
				return opdiagParams(ScopeAccount, performance(MetricFavorite), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionPerformance)
				group := onlyGroup(t, factsOf[PerformanceFacts](t, dimension), "xiaohongshu", "favorite")
				if !group.StatWindowMixed || !slices.Equal(group.StatWindows, []string{"发布后 14 天", "发布后 7 天"}) ||
					!slices.Contains(dimension.Limits, "performance.stat_window_mixed") {
					t.Fatalf("group = %+v, limits %v", group, dimension.Limits)
				}
			},
		},
		{
			name: "change",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p-aug", "w1", "a1", "xiaohongshu", "verified_published", "2026-08-10T10:00:00+08:00").
					published("p-sep", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
					metric("m1", "p-aug", "xiaohongshu", "a1", MetricFavorite, diagCount(20), "", "2026-08-17T10:00:00+08:00").
					metric("m2", "p-sep", "xiaohongshu", "a1", MetricFavorite, diagCount(15), "", "2026-09-17T10:00:00+08:00")
				return opdiagParams(ScopeAccount, performance(MetricFavorite), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionPerformance)
				group := onlyGroup(t, factsOf[PerformanceFacts](t, dimension), "xiaohongshu", "favorite")
				if group.Change.Display != "−5.00" || group.Change.Value != "-5" || group.Baseline.Mean.Display != "20.00" {
					t.Fatalf("change = %+v, baseline %+v", group.Change, group.Baseline)
				}
				if !slices.Contains(dimension.Limits, "performance.difference_is_not_cause") {
					t.Fatalf("limits = %v", dimension.Limits)
				}
			},
		},
		{
			name: "cadence-unset-vs-zero",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				// xiaohongshu has no target, douyin has a target of 0; neither
				// published in W37. One wechat_mp record elsewhere in the month
				// makes the dimension computable.
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").
					rules(DiagOperatingRules{Cadence: map[string]int64{"douyin": 0}}).
					published("p1", "w1", "a1", "wechat_mp", "verified_published", "2026-09-22T10:00:00+08:00")
				return opdiagParams(ScopeBrand, only(DiagnosisDimensionParam{Key: DimensionCadence}), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				facts := factsOf[CadenceFacts](t, dimensionOf(t, result, "brand", DimensionCadence))
				week := func(channel string) (CadenceChannelFacts, CadenceWeekFacts) {
					for _, c := range facts.Channels {
						if c.Channel == channel {
							for _, w := range c.Weeks {
								if w.ISOWeek == "2026-W37" {
									return c, w
								}
							}
						}
					}
					t.Fatalf("no %s W37 in %+v", channel, facts)
					return CadenceChannelFacts{}, CadenceWeekFacts{}
				}
				unset, unsetWeek := week("xiaohongshu")
				if unset.Target.Set || unset.Target.PerWeek != nil || unsetWeek.Compared || unsetWeek.Met != nil ||
					!hasGap(result, GapCadenceUnset, "xiaohongshu") {
					t.Fatalf("unset channel = %+v, week %+v", unset, unsetWeek)
				}
				zero, zeroWeek := week("douyin")
				if !zero.Target.Set || zero.Target.PerWeek == nil || *zero.Target.PerWeek != 0 || !zeroWeek.Compared ||
					zeroWeek.Met == nil || !*zeroWeek.Met || hasGap(result, GapCadenceUnset, "douyin") {
					t.Fatalf("zero channel = %+v, week %+v", zero, zeroWeek)
				}
			},
		},
		{
			name: "incomplete-week",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").
					rules(DiagOperatingRules{Cadence: map[string]int64{"xiaohongshu": 1}}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-04T10:00:00+08:00")
				params := opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionCadence}), "a1")
				params.Window.Start = "2026-09-03"
				return params, s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionCadence)
				first := factsOf[CadenceFacts](t, dimension).Channels[0].Weeks[0]
				if first.ISOWeek != "2026-W36" || first.Complete || first.Compared || first.Met != nil || first.Published != 1 {
					t.Fatalf("first week = %+v", first)
				}
				if !slices.Contains(dimension.Limits, "cadence.incomplete_week_not_compared") {
					t.Fatalf("limits = %v", dimension.Limits)
				}
			},
		},
		{
			name: "pillar-untagged",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
					published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-11T10:00:00+08:00").
					published("p3", "w3", "a1", "xiaohongshu", "verified_published", "2026-09-12T10:00:00+08:00").
					mark("k1", "w1", MarkPillar, "穿搭", VerdictTagged, "", "", "2026-09-13T10:00:00+08:00").
					mark("k2", "w2", MarkPillar, "穿搭", VerdictTagged, "", "", "2026-09-13T10:00:00+08:00").
					mark("k3", "w2", MarkPillar, "穿搭", VerdictUntagged, "", "", "2026-09-14T10:00:00+08:00").
					mark("k4", "w2", MarkPillar, "面料知识", VerdictTagged, "", "", "2026-09-15T10:00:00+08:00")
				return opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionCoverage,
					Pillars: []string{"穿搭", "面料知识"}}), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionCoverage)
				facts := factsOf[CoverageFacts](t, dimension)
				if facts.Works != 3 || facts.Pillars[0] != (CoveragePillarFact{Pillar: "穿搭", Works: 1}) ||
					facts.Pillars[1] != (CoveragePillarFact{Pillar: "面料知识", Works: 1}) || facts.Untagged != 1 ||
					!hasGap(result, GapWorkUntagged, "w3") ||
					!slices.Contains(dimension.Limits, "coverage.multi_pillar_not_additive") {
					t.Fatalf("coverage = %+v, limits %v", facts, dimension.Limits)
				}
			},
		},
		{
			name: "pillars-empty",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00")
				return opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionCoverage, Pillars: []string{}}), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				wantNotComputable(t, dimensionOf(t, result, "account:a1", DimensionCoverage), DimensionMissingConfig)
			},
		},
		{
			name: "observation-unset",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00")
				return opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionExecutionFlow}), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				dimension := dimensionOf(t, result, "account:a1", DimensionExecutionFlow)
				fact := factsOf[ExecutionFlowFacts](t, dimension).PublicationsMissingMetricsAfterWindow
				if fact.Status != dimensionNotComputable || fact.Reason != "missing_observation_window" ||
					!hasGap(result, GapObservationUnset, "xiaohongshu") ||
					!slices.Contains(dimension.Limits, "execution_flow.no_threshold") {
					t.Fatalf("fact = %+v", fact)
				}
			},
		},
		{
			name: "unknown-account",
			build: func(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
				s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
					historical("p-history", "w-history", "xiaohongshu", "2026-09-05T10:00:00+08:00")
				return opdiagParams(ScopeBrand, allSix(), "a1"), s.inputs
			},
			expect: func(t *testing.T, result DiagnosisResult) {
				section := sectionOf(t, result, "unknown_account")
				if len(section.Dimensions) != 6 {
					t.Fatalf("unknown_account dimensions = %v", section.Dimensions)
				}
				for key, dimension := range section.Dimensions {
					if !slices.Contains(dimension.Limits, "scope.historical_import_account_unknown") {
						t.Errorf("unknown_account %s limits = %v", key, dimension.Limits)
					}
				}
				if !hasGap(result, GapAccountUnresolved, "p-history") ||
					!slices.Contains(result.Rules, "scope.historical_import_account_unknown") ||
					result.Scope.HistoricalImportPublications != 1 {
					t.Fatalf("gaps %+v, rules %v, historical %d", result.Gaps, result.Rules, result.Scope.HistoricalImportPublications)
				}
			},
		},
	}
}

// T038 / SC-001 / SC-002 / D14-V04: every fixed sample of contract §5.10,
// and - its last row, order-independent - each one gives the same bytes in
// any input order and recomputes to the same bytes from its stored JSON
// (SC-005's calculator half; T053 does it through the database).
func TestTheFixedDiagnosisSamples(t *testing.T) {
	for _, sample := range fixedSamples() {
		t.Run(sample.name, func(t *testing.T) {
			params, inputs := sample.build(t)
			result := mustCalculate(t, params, inputs)
			sample.expect(t, result)
			checkResultInvariants(t, result)
		})
	}
	t.Run("order-independent", func(t *testing.T) {
		for _, sample := range fixedSamples() {
			params, inputs := sample.build(t)
			want, _ := json.Marshal(mustCalculate(t, params, inputs))
			for seed := range uint64(20) {
				got, _ := json.Marshal(mustCalculate(t, params, shuffled(inputs, seed)))
				if string(got) != string(want) {
					t.Fatalf("%s: seed %d changed the result", sample.name, seed)
				}
			}
			storedParams, _ := json.Marshal(params)
			storedInputs, _ := json.Marshal(inputs)
			again, err := RecomputeDiagnosis(DiagnosisCalcVersion, storedParams, storedInputs)
			if err != nil {
				t.Fatalf("%s: %v", sample.name, err)
			}
			if got, _ := json.Marshal(again); string(got) != string(want) {
				t.Fatalf("%s recomputed differently:\n%s\n%s", sample.name, got, want)
			}
		}
	})
}

// checkResultInvariants holds what every result must be, whatever went in:
// each dimension ok with facts or not computable with a reason from the set;
// every completeness gap key is in the report's list, once; every list is
// there, empty rather than null; every reference key is unique.
func checkResultInvariants(t *testing.T, result DiagnosisResult) {
	t.Helper()
	keys := map[string]bool{}
	for _, gap := range result.Gaps {
		if keys[gap.GapKey] {
			t.Errorf("gap %s is listed twice", gap.GapKey)
		}
		keys[gap.GapKey] = true
		if !oneOf(string(gap.Kind), GapKinds) || gap.FixRoute == "" {
			t.Errorf("gap %+v", gap)
		}
	}
	refs := map[string]bool{}
	for _, ref := range result.Refs {
		if refs[ref] {
			t.Errorf("reference %s is listed twice", ref)
		}
		refs[ref] = true
	}
	for _, section := range result.Sections {
		for key, dimension := range section.Dimensions {
			switch dimension.Status {
			case dimensionOK:
				if dimension.Reason != "" || len(dimension.Facts) == 0 {
					t.Errorf("%s %s ok = %+v", sectionRefName(section), key, dimension)
				}
			case dimensionNotComputable:
				if !oneOf(dimension.Reason, DimensionReasons) || len(dimension.Facts) != 0 {
					t.Errorf("%s %s not computable = %+v", sectionRefName(section), key, dimension)
				}
			default:
				t.Errorf("%s %s status %q", sectionRefName(section), key, dimension.Status)
			}
			if dimension.Limits == nil || dimension.Records == nil || dimension.Completeness.GapKeys == nil {
				t.Errorf("%s %s has a null list", sectionRefName(section), key)
			}
			for _, key := range dimension.Completeness.GapKeys {
				if !keys[key] {
					t.Errorf("completeness gap %s is not in the report's list", key)
				}
			}
			if !refs[sectionRefName(section)+"/"+string(key)] {
				t.Errorf("no reference for %s/%s", sectionRefName(section), key)
			}
		}
	}
	if result.Gaps == nil || result.Rules == nil || result.Refs == nil {
		t.Error("a report-level list is null")
	}
}

func TestAnEmptyParamListIsMissingConfigNotARefusal(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00")
	params := opdiagParams(ScopeAccount, []DiagnosisDimensionParam{
		{Key: DimensionConsistency}, {Key: DimensionCoverage}, {Key: DimensionPerformance},
	}, "a1")
	result := mustCalculate(t, params, s.inputs)
	for _, dimension := range []DiagnosisDimension{DimensionConsistency, DimensionCoverage, DimensionPerformance} {
		wantNotComputable(t, dimensionOf(t, result, "account:a1", dimension), DimensionMissingConfig)
	}
	// No comparison window: performance cannot compare, and says so; the
	// other dimensions are still read.
	params = opdiagParams(ScopeAccount, []DiagnosisDimensionParam{
		{Key: DimensionPerformance, Metrics: []Metric{MetricLike}}, {Key: DimensionCadence},
	}, "a1")
	params.ComparisonWindow = nil
	result = mustCalculate(t, params, s.inputs)
	wantNotComputable(t, dimensionOf(t, result, "account:a1", DimensionPerformance), DimensionMissingComparisonWindow)
	if dimensionOf(t, result, "account:a1", DimensionCadence).Status != dimensionOK {
		t.Fatal("cadence was not computed beside it")
	}
}

// ---------------------------------------------------------------- T039: order does not matter

func newSeeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x5bd1e995)) }

// randomSample is a brand of two accounts and records of every kind drawn
// from a seed: published or not, dated or not, in either window, with nil,
// zero and other values, tags or none, marks, reviews and tasks.
func randomSample(t *testing.T, seed uint64) []DiagnosisInput {
	random := newSeeded(seed)
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号", "positioning").account("a2", "douyin", "副号").
		rules(DiagOperatingRules{Cadence: map[string]int64{"xiaohongshu": 2, "douyin": 0}})
	accounts := []string{"a1", "a2", ""}
	statuses := []string{"verified_published", "reported_published", "failed", "unknown", "removed"}
	days := []string{"", "2026-08-05T10:00:00+08:00", "2026-08-30T23:30:00+08:00", "2026-09-01T00:00:00+08:00",
		"2026-09-14T09:00:00+08:00", "2026-09-30T23:30:00+08:00", "2026-10-01T00:10:00+08:00"}
	for i := range random.IntN(8) + 1 {
		id, work := "p"+string(rune('a'+i)), "w"+string(rune('a'+i%4))
		account := accounts[random.IntN(len(accounts))]
		channel := string(Platforms[random.IntN(len(Platforms))])
		if account == "" && random.IntN(2) == 0 {
			s.historical(id, "h"+work, channel, days[1+random.IntN(len(days)-1)])
		} else {
			s.published(id, work+account, account, channel, statuses[random.IntN(len(statuses))], days[random.IntN(len(days))])
		}
		for j := range random.IntN(4) {
			var value *int64
			if n := random.IntN(4); n > 0 {
				value = diagCount(int64(n - 1))
			}
			metric := []Metric{MetricImpression, MetricPlay, MetricFavorite}[random.IntN(3)]
			s.metric(id+"-m"+string(rune('0'+j)), id, channel, account, metric, value,
				[]string{"发布后 7 天", " 发布后 7 天", "发布后 14 天"}[random.IntN(3)], days[4+random.IntN(2)])
		}
		if random.IntN(2) == 0 {
			tags := [][]string{{}, {"价格"}, {" 价格 ", "Price"}, {"price", "尺码"}}[random.IntN(4)]
			s.excerpt(id+"-e", id, ExcerptSources[random.IntN(3)], "2026-09-15T10:00:00+08:00", tags...)
		}
		if random.IntN(2) == 0 {
			s.mark(id+"-k", work+account, MarkPillar, []string{"穿搭", "面料知识"}[random.IntN(2)],
				[]MarkVerdict{VerdictTagged, VerdictUntagged}[random.IntN(2)], "", "", "2026-09-16T10:00:00+08:00")
		}
		if random.IntN(2) == 0 {
			s.mark(id+"-c", work+account, MarkConsistency, "positioning",
				[]MarkVerdict{VerdictConsistent, VerdictInconsistent, VerdictUnsure}[random.IntN(3)],
				[]string{"a1", "a2"}[random.IntN(2)], []string{"pr-a1", "pr-old"}[random.IntN(2)], "2026-09-16T10:00:00+08:00")
		}
	}
	for i := range random.IntN(3) {
		id := "r" + string(rune('0'+i))
		s.review(id, accounts[random.IntN(len(accounts))], []string{"pending", "changes_requested", "rejected"}[random.IntN(3)],
			"2026-09-20T10:00:00+08:00", "2026-09-21T10:00:00+08:00")
		s.task("t"+id, id, []string{"held", "scheduled"}[random.IntN(2)], random.IntN(2) == 0)
	}
	return s.inputs
}

// T039 / FR-010: 200 random brands, each in 200 orders... well, in a fresh
// shuffle each: the result is the same bytes.
func TestTheDiagnosisDoesNotDependOnInputOrder(t *testing.T) {
	params := opdiagParams(ScopeBrand, allSix(), "a2", "a1")
	for seed := range uint64(200) {
		inputs := randomSample(t, seed)
		result := mustCalculate(t, params, inputs)
		checkResultInvariants(t, result)
		want, _ := json.Marshal(result)
		got, _ := json.Marshal(mustCalculate(t, params, shuffled(inputs, seed+1000)))
		if string(got) != string(want) {
			t.Fatalf("seed %d: input order changed the result", seed)
		}
	}
}

// ---------------------------------------------------------------- T040

// T040: stat_window spellings are compared after NFC and trimming; two real
// spellings are mixed, listed in byte order.
func TestStatWindowSpellingsAreNormalizedBeforeComparing(t *testing.T) {
	build := func(second string) PerformanceGroup {
		s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
			published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
			published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-11T10:00:00+08:00").
			metric("m1", "p1", "xiaohongshu", "a1", MetricLike, diagCount(1), "7 天 café", "2026-09-17T10:00:00+08:00").
			metric("m2", "p2", "xiaohongshu", "a1", MetricLike, diagCount(2), second, "2026-09-17T10:00:00+08:00")
		result := mustCalculate(t, opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionPerformance,
			Metrics: []Metric{MetricLike}}), "a1"), s.inputs)
		return onlyGroup(t, factsOf[PerformanceFacts](t, dimensionOf(t, result, "account:a1", DimensionPerformance)), "xiaohongshu", "like")
	}
	if group := build("  7 天 cafe\u0301 "); group.StatWindowMixed || !slices.Equal(group.StatWindows, []string{"7 天 café"}) {
		t.Fatalf("one spelling read as two: %+v", group)
	}
	if group := build("14 天"); !group.StatWindowMixed || !slices.Equal(group.StatWindows, []string{"14 天", "7 天 café"}) {
		t.Fatalf("two spellings = %+v", group)
	}
}

// ---------------------------------------------------------------- T041

// T041 / FR-022: weeks are ISO weeks of the params timezone; an account
// whose platform is not a delivery channel cannot be held to a cadence; the
// target is always read as the brand's, per channel.
func TestCadenceIsCountedInISOWeeksOfTheBrandsTimezone(t *testing.T) {
	// 2026-09-13 23:30 in Shanghai is Sunday of W37; in UTC it is still the
	// same Sunday, but 2026-09-14 00:30 Shanghai (Monday, W38) is Sunday
	// 16:30 UTC - counted in W38, not W37.
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").account("a9", "bilibili", "别的平台").
		rules(DiagOperatingRules{Cadence: map[string]int64{"xiaohongshu": 1}}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-13T23:30:00+08:00").
		published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-14T00:30:00+08:00").
		published("p-undated", "w3", "a1", "xiaohongshu", "verified_published", "")
	params := opdiagParams(ScopeBrand, only(DiagnosisDimensionParam{Key: DimensionCadence}), "a1", "a9")
	result := mustCalculate(t, params, s.inputs)
	dimension := dimensionOf(t, result, "account:a1", DimensionCadence)
	facts := factsOf[CadenceFacts](t, dimension)
	weeks := map[string]CadenceWeekFacts{}
	for _, week := range facts.Channels[0].Weeks {
		weeks[week.ISOWeek] = week
	}
	if len(facts.Channels) != 1 || weeks["2026-W37"].Published != 1 || weeks["2026-W38"].Published != 1 ||
		weeks["2026-W37"].Start != "2026-09-07" || !*weeks["2026-W37"].Met || facts.PublishedAtMissing != 1 {
		t.Fatalf("cadence = %+v", facts)
	}
	if !hasGap(result, GapPublishedAtMissing, "p-undated") || dimension.Completeness.Expected != 3 ||
		dimension.Completeness.Present != 2 {
		t.Fatalf("gaps %+v, completeness %+v", result.Gaps, dimension.Completeness)
	}
	if dimension.Limits[0] != "cadence.target_is_brand_channel_level" {
		t.Fatalf("limits = %v", dimension.Limits)
	}
	wantNotComputable(t, dimensionOf(t, result, "account:a9", DimensionCadence), DimensionNoDeliveryChannel)
	if !slices.Contains(dimensionOf(t, result, "account:a9", DimensionCadence).Limits, "cadence.target_is_brand_channel_level") {
		t.Fatal("the brand-level limit is missing where the dimension is not computable")
	}
}

// ---------------------------------------------------------------- T042

// T042 / FR-025: waiting days are whole calendar days in the params
// timezone; due is whatever review-delivery answered; an unset observation
// window is a gap and makes that fact not computable.
func TestTheExecutionFlowStatesWhatIsWaitingWithoutAThreshold(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").
		rules(DiagOperatingRules{Observation: observationDays(7)}).
		// Requested 09-30 23:30 Shanghai, generated 10-02 09:00 Shanghai: two
		// calendar days there, although it is under 34 hours.
		review("r-pending", "a1", "pending", "2026-09-30T23:30:00+08:00", "").
		review("r-changes", "a1", "changes_requested", "2026-10-01T08:00:00+08:00", "").
		review("r-rejected", "a1", "rejected", "2026-09-01T08:00:00+08:00", "2026-09-02T08:00:00+08:00").
		review("r-rejected-old", "a1", "rejected", "2026-08-01T08:00:00+08:00", "2026-08-02T08:00:00+08:00").
		// A task review-delivery says is due - even one scheduled in the
		// future is taken as it answered, not worked out again here.
		task("t-due", "r-pending", "scheduled", true).
		task("t-held", "r-pending", "held", false).
		published("p-old", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		published("p-new", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-30T10:00:00+08:00").
		published("p-measured", "w3", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		metric("m1", "p-measured", "xiaohongshu", "a1", MetricLike, nil, "", "2026-09-17T10:00:00+08:00").
		published("p-unknown", "w4", "a1", "xiaohongshu", "unknown", "").
		published("p-failed", "w5", "a1", "xiaohongshu", "failed", "")
	params := opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionExecutionFlow}), "a1")
	params.GeneratedAt = "2026-10-02T01:00:00Z"
	result := mustCalculate(t, params, s.inputs)
	facts := factsOf[ExecutionFlowFacts](t, dimensionOf(t, result, "account:a1", DimensionExecutionFlow))
	if facts.ReviewsPending.Count != 1 || facts.ReviewsPending.Items[0].WaitingDays != 2 ||
		facts.ReviewsChangesRequested.Items[0].WaitingDays != 1 {
		t.Fatalf("reviews = %+v %+v", facts.ReviewsPending, facts.ReviewsChangesRequested)
	}
	if facts.ReviewsRejectedInWindow.Count != 1 || facts.ReviewsRejectedInWindow.Items[0].ReviewRequestID != "r-rejected" {
		t.Fatalf("rejected in window = %+v", facts.ReviewsRejectedInWindow)
	}
	if facts.DeliveriesDue.Count != 1 || facts.DeliveriesDue.Items[0].DeliveryTaskID != "t-due" ||
		facts.DeliveriesHeld.Count != 1 {
		t.Fatalf("deliveries = %+v %+v", facts.DeliveriesDue, facts.DeliveriesHeld)
	}
	if facts.PublicationsUnknown.Count != 1 || facts.PublicationsFailed.Count != 1 ||
		!hasGap(result, GapPublicationStatusUnknown, "p-unknown") {
		t.Fatalf("publications = %+v %+v", facts.PublicationsUnknown, facts.PublicationsFailed)
	}
	// Seven days after 09-10 has passed by 10-02; after 09-30 it has not;
	// p-measured has a metric - an unknown one, which still counts as
	// recorded.
	missing := facts.PublicationsMissingMetricsAfterWindow
	if missing.Status != dimensionOK || len(missing.Channels) != 1 || missing.Channels[0].Count != 1 ||
		missing.Channels[0].Items[0].PublicationRecordID != "p-old" {
		t.Fatalf("missing metrics = %+v", missing)
	}
}

func observationDays(days int64) workspacecore.Observation {
	return workspacecore.Observation{Default: &days}
}

// T042: no number of days is written into the diagnosis - the waiting days
// are stated and never compared, and the observation window comes from the
// brand's settings (027's TestThePendingDerivationReadsItsWindowFromSettings
// does the same for the pending derivation).
func TestTheDiagnosisHasNoDayThreshold(t *testing.T) {
	sources := opdiagSourceFiles(t)
	checked := 0
	for name, source := range sources {
		code := stripGoComments(source)
		checked++
		for _, pattern := range []string{
			`WaitingDays\s*[<>]`, `wholeDays\([^)]*\)\s*[<>]`, `\b\d+\s*\*\s*time\.Hour`,
			`AddDate\(`, `time\.Duration\(\s*\d+\s*\)`,
		} {
			if match := regexp.MustCompile(pattern).FindString(code); match != "" {
				t.Errorf("%s has a day threshold or a fixed window: %q", name, match)
			}
		}
	}
	if checked == 0 || !strings.Contains(sources["opdiag_dimensions.go"], "workspacecore.ObservationDueFor(") {
		t.Fatal("the execution flow no longer asks workspace-core; this guard would pass vacuously")
	}
}

// ---------------------------------------------------------------- T043

// T043 / FR-020: marks made against an older profile revision are counted
// and named; a mark for another account is not a check of this one.
func TestConsistencyCountsTheCurrentMarksOfThisAccount(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号", "expression_style").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-11T10:00:00+08:00").
		mark("c1", "w1", MarkConsistency, "positioning", VerdictConsistent, "a1", "pr-a1", "2026-09-12T10:00:00+08:00").
		mark("c2", "w1", MarkConsistency, "expression_style", VerdictInconsistent, "a1", "pr-old", "2026-09-12T10:00:00+08:00").
		mark("c3", "w2", MarkConsistency, "positioning", VerdictUnsure, "a1", "pr-a1", "2026-09-12T10:00:00+08:00").
		// w2's positioning was checked for a1 first, then for another
		// account: its current mark is not a check of a1.
		mark("c4", "w2", MarkConsistency, "positioning", VerdictConsistent, "a-other", "pr-x", "2026-09-13T10:00:00+08:00")
	result := mustCalculate(t, opdiagParams(ScopeAccount, allSix()[:1], "a1"), s.inputs)
	dimension := dimensionOf(t, result, "account:a1", DimensionConsistency)
	facts := factsOf[ConsistencyFacts](t, dimension)
	positioning, style := facts.Items[0], facts.Items[1]
	if facts.Works != 2 || positioning != (ConsistencyItemFacts{Item: "positioning", ProfileStatus: "confirmed", Consistent: 1, Unchecked: 1}) ||
		style != (ConsistencyItemFacts{Item: "expression_style", ProfileStatus: "pending", Inconsistent: 1, Unchecked: 1}) {
		t.Fatalf("items = %+v", facts.Items)
	}
	if facts.MarksOnOlderRevision != 1 || !slices.Contains(dimension.Limits, "consistency.marks_on_older_profile") {
		t.Fatalf("older marks = %d, limits %v", facts.MarksOnOlderRevision, dimension.Limits)
	}
	if !hasGap(result, GapWorkUnchecked, "w2") || !hasGap(result, GapProfileFieldPending, "a1/expression_style") ||
		dimension.Completeness.Expected != 4 || dimension.Completeness.Present != 2 {
		t.Fatalf("gaps %+v, completeness %+v", result.Gaps, dimension.Completeness)
	}
}

// ---------------------------------------------------------------- T044

// T044 / FR-024: tags are trimmed and put in NFC and nothing more; with no
// excerpt, the dimension is not computable.
func TestAudienceTagsAreOnlyTrimmedAndNFC(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		excerpt("e1", "p1", ExcerptComment, "2026-09-11T10:00:00+08:00", "价格", "Price").
		excerpt("e2", "p1", ExcerptPrivateMessage, "2026-09-12T10:00:00+08:00", " 价格 ", "price").
		excerpt("e3", "p1", ExcerptComment, "2026-09-12T10:00:00+08:00").
		excerpt("e-late", "p1", ExcerptComment, "2026-10-01T00:10:00+08:00", "价格")
	result := mustCalculate(t, opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionAudienceFeedback}), "a1"), s.inputs)
	dimension := dimensionOf(t, result, "account:a1", DimensionAudienceFeedback)
	facts := factsOf[AudienceFeedbackFacts](t, dimension)
	if facts.Excerpts != 3 || facts.Untagged != 1 || !slices.Equal(facts.ByTag, []AudienceTagCount{
		{Tag: "Price", Excerpts: 1}, {Tag: "price", Excerpts: 1}, {Tag: "价格", Excerpts: 2},
	}) || !hasGap(result, GapExcerptUntagged, "e3") {
		t.Fatalf("audience = %+v", facts)
	}
	if !slices.Equal(facts.BySource, []AudienceSourceCount{
		{Platform: "xiaohongshu", SourceType: "comment", Excerpts: 2}, {Platform: "xiaohongshu", SourceType: "private_message", Excerpts: 1},
	}) || !slices.Contains(dimension.Limits, "audience_feedback.tags_not_additive") {
		t.Fatalf("by source = %+v", facts.BySource)
	}
	s = newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00")
	result = mustCalculate(t, opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionAudienceFeedback}), "a1"), s.inputs)
	wantNotComputable(t, dimensionOf(t, result, "account:a1", DimensionAudienceFeedback), DimensionNoData)
}

// ---------------------------------------------------------------- T045

// allGapsSample finds every kind of gap at least once.
func allGapsSample(t *testing.T) (DiagnosisParams, []DiagnosisInput) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号", "positioning").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		published("p-undated", "w2", "a1", "xiaohongshu", "verified_published", "").
		published("p-unknown", "w3", "a1", "xiaohongshu", "unknown", "").
		excerpt("e1", "p1", ExcerptComment, "2026-09-11T10:00:00+08:00").
		historical("p-history", "w-history", "xiaohongshu", "2026-09-05T10:00:00+08:00").
		add(inputPublication, "p-nowork", diagPublicationFields{WorkID: "w-gone", Channel: "xiaohongshu",
			Status: "verified_published", PublishedAt: stampPtr("2026-09-06T10:00:00+08:00")})
	params := opdiagParams(ScopeBrand, allSix(), "a1")
	params.Dimensions[0].Items = []string{"positioning"}
	return params, s.inputs
}

// T045 / FR-030 / FR-031: every kind of gap can be found; a key is made of
// the dimension, the kind and the record, the same every time; a gap two
// dimensions find is in the report once.
func TestGapsHaveStableKeysAndAppearOnce(t *testing.T) {
	params, inputs := allGapsSample(t)
	result := mustCalculate(t, params, inputs)
	checkResultInvariants(t, result)
	kinds := map[GapKind]bool{}
	for _, gap := range result.Gaps {
		kinds[gap.Kind] = true
	}
	for _, kind := range GapKinds {
		if !kinds[kind] {
			t.Errorf("no sample finds a %s gap", kind)
		}
	}
	for _, want := range []string{
		"performance/metric_missing/publication/p1",
		"cadence/cadence_unset/channel/xiaohongshu",
		"cadence/published_at_missing/publication/p-undated",
		"consistency/work_unchecked/work/w1",
		"coverage/work_untagged/work/w1",
		"audience_feedback/excerpt_untagged/excerpt/e1",
		"execution_flow/observation_unset/channel/xiaohongshu",
		"execution_flow/publication_status_unknown/publication/p-unknown",
		"scope/profile_field_pending/profile_field/a1/positioning",
		"scope/account_unresolved/publication/p-history",
		"scope/work_missing/publication/p-nowork",
	} {
		if !slices.ContainsFunc(result.Gaps, func(gap DiagnosisGap) bool { return gap.GapKey == want }) {
			t.Errorf("no gap %s", want)
		}
	}
	// p-history has no account: cadence, performance, coverage and the rest
	// all find it, and the report lists it once.
	found := 0
	for _, section := range result.Sections {
		for _, dimension := range section.Dimensions {
			if slices.Contains(dimension.Completeness.GapKeys, "scope/account_unresolved/publication/p-history") {
				found++
			}
		}
	}
	if found < 2 {
		t.Fatalf("the unresolved gap was found by %d dimensions; this case needs two or more", found)
	}
	again := mustCalculate(t, params, shuffled(inputs, 3))
	if !slices.Equal(result.Gaps, again.Gaps) {
		t.Fatal("gap keys are not deterministic")
	}
}

// ---------------------------------------------------------------- T046

// T046 / contract §5.8: every shape of reference key, and one per fact -
// each performance group, tag, pillar and consistency item has exactly one.
func TestEveryCitableFactHasAReferenceKey(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00").
		metric("m1", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(3), "", "2026-09-17T10:00:00+08:00").
		excerpt("e1", "p1", ExcerptComment, "2026-09-11T10:00:00+08:00", "价格").
		published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-12T10:00:00+08:00")
	s.add(inputROISummary, "roi-1/3", sampleROISummary())
	params := opdiagParams(ScopeBrand, allSix(), "a1")
	params.ROIReportRef = &DiagnosisROIRef{ReportID: "roi-1", VersionNo: 3}
	result := mustCalculate(t, params, s.inputs)
	for _, want := range []string{
		"scope", "account:a1/cadence", "account:a1/consistency/positioning", "account:a1/coverage/穿搭",
		"brand/cadence/xiaohongshu/2026-W37", "account:a1/performance/xiaohongshu/favorite",
		"account:a1/audience_feedback/tag/价格", "brand/execution_flow/reviews_pending",
		"gap/performance/metric_missing/publication/p2", "roi_reference",
	} {
		if !slices.Contains(result.Refs, want) {
			t.Errorf("no reference %s", want)
		}
	}
	checkResultInvariants(t, result)
	// One reference per group, and a group per reference.
	performanceRefs := 0
	for _, ref := range result.Refs {
		if strings.HasPrefix(ref, "account:a1/performance/") {
			performanceRefs++
		}
	}
	groups := factsOf[PerformanceFacts](t, dimensionOf(t, result, "account:a1", DimensionPerformance)).Groups
	if performanceRefs != len(groups) {
		t.Fatalf("%d performance references for %d groups", performanceRefs, len(groups))
	}
	// A dimension that is not computable has nothing to cite but itself.
	for _, ref := range result.Refs {
		if strings.HasPrefix(ref, "account:a1/consistency/") && dimensionOf(t, result, "account:a1", DimensionConsistency).Status != dimensionOK {
			t.Fatalf("a not-computable dimension has fact references: %s", ref)
		}
	}
}

// ---------------------------------------------------------------- T047 / SC-003

var causalWords = []string{"growth", "caused", "because", "增长", "提升", "下降", "导致", "带来", "因为"}

// T047 / FR-015 / FR-028 / SC-003 (server half): no rule id and no string
// literal in the diagnosis code states a cause or a trend, and the
// performance dimension always says its difference is not a cause.
func TestTheDiagnosisConcludesNothing(t *testing.T) {
	literals := 0
	for name, source := range opdiagSourceFiles(t) {
		file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			literals++
			for _, word := range causalWords {
				if strings.Contains(strings.ToLower(literal.Value), word) {
					t.Errorf("%s: the literal %s states a cause or a trend", name, literal.Value)
				}
			}
			return true
		})
	}
	if literals < 100 {
		t.Fatalf("read only %d literals; the guard would pass vacuously", literals)
	}
	for _, rule := range DiagnosisRuleIDs {
		for _, word := range causalWords {
			if strings.Contains(strings.ToLower(rule), word) {
				t.Errorf("rule %s states a cause or a trend", rule)
			}
		}
	}
	for _, sample := range fixedSamples() {
		params, inputs := sample.build(t)
		for _, section := range mustCalculate(t, params, inputs).Sections {
			if dimension, ok := section.Dimensions[DimensionPerformance]; ok &&
				!slices.Contains(dimension.Limits, "performance.difference_is_not_cause") {
				t.Errorf("%s: performance limits %v", sample.name, dimension.Limits)
			}
		}
	}
}

// SC-011 / FR-013 / FR-023: no facts type has a score-like field, and none
// has a field that adds or ranks across performance groups.
func TestTheFactsHaveNoScoreAndNoCrossGroupTotal(t *testing.T) {
	checked := 0
	for _, typ := range []reflect.Type{
		reflect.TypeFor[ConsistencyFacts](), reflect.TypeFor[CoverageFacts](), reflect.TypeFor[CadenceFacts](),
		reflect.TypeFor[PerformanceFacts](), reflect.TypeFor[AudienceFeedbackFacts](), reflect.TypeFor[ExecutionFlowFacts](),
	} {
		for _, name := range fieldNames(typ) {
			checked++
			for _, forbidden := range []string{"score", "grade", "rating", "rank", "level", "role", "industry", "percent", "trend"} {
				if strings.Contains(name, forbidden) {
					t.Errorf("%s has field %q", typ, name)
				}
			}
		}
	}
	if top := fieldNames(reflect.TypeFor[PerformanceFacts]()); top[0] != "groups" || slices.Contains(top, "total") ||
		slices.Contains(top, "overall") || slices.Contains(top, "all_platforms") {
		t.Errorf("performance facts = %v", top)
	}
	if checked < 50 {
		t.Fatalf("checked only %d fields", checked)
	}
}

// ---------------------------------------------------------------- T048

// T048 / FR-029: account sections by display name then id - never by a
// number; unknown_account only when something has no account; the brand
// section carries cadence and execution flow only.
func TestBrandSectionsAreInAFixedOrder(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "B 号").account("a2", "douyin", "A 号").
		account("a0", "douyin", "A 号").rules(DiagOperatingRules{})
	// "B 号" has the most records; it still comes after the two "A 号".
	for _, id := range []string{"p1", "p2", "p3"} {
		s.published(id, "w"+id, "a1", "xiaohongshu", "verified_published", "2026-09-10T10:00:00+08:00")
	}
	params := opdiagParams(ScopeBrand, allSix(), "a1", "a2", "a0")
	result := mustCalculate(t, params, s.inputs)
	names := []string{}
	for _, section := range result.Sections {
		names = append(names, sectionRefName(section))
	}
	if !slices.Equal(names, []string{"account:a0", "account:a2", "account:a1", "brand"}) {
		t.Fatalf("sections = %v", names)
	}
	brand := sectionOf(t, result, "brand")
	if len(brand.Dimensions) != 2 || brand.Dimensions[DimensionCadence].Status == "" ||
		brand.Dimensions[DimensionExecutionFlow].Status == "" {
		t.Fatalf("brand section = %+v", brand.Dimensions)
	}
	s.historical("p-h", "w-h", "douyin", "2026-09-10T10:00:00+08:00")
	result = mustCalculate(t, params, s.inputs)
	if sectionRefName(result.Sections[3]) != "unknown_account" || sectionRefName(result.Sections[4]) != "brand" {
		t.Fatalf("sections = %+v", result.Sections)
	}
}

// ---------------------------------------------------------------- T050 / Q6

type fakeROISummaries struct{ log *diagLog }

func sampleROISummary() ReportSummary {
	return ReportSummary{
		ReportID: "roi-1", VersionNo: 3, Title: "九月 ROI", CalcVersion: "roi-calc/1",
		Metrics: map[MetricID]SummaryMetric{MetricIDs[0]: {Status: "ok", Display: "1,200.00", Formula: "f"}},
		Rules:   []string{"roi.rule"}, CreatedAt: *diagAt("2026-09-30T10:00:00+08:00"),
	}
}

func (f fakeROISummaries) ReportSummary(_ context.Context, _, reportID string, versionNo int) (ReportSummary, error) {
	f.log.add("roi")
	if reportID == "roi-1" && versionNo == 3 {
		return sampleROISummary(), nil
	}
	return ReportSummary{}, ErrNotFound
}

// T050 / FR-027 / Q6: the ROI summary a person names is copied whole into
// the inputs and shown as it is. It changes no dimension, and a version that
// is not there is refused like a missing account - before any account's data
// is read.
func TestTheROISummaryIsShownAsItIs(t *testing.T) {
	params := opdiagParams(ScopeBrand, allSix(), "a1", "a2")
	store, log := diagFixture()
	store.roiSummaries = fakeROISummaries{log: log}
	_, without, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
	if err != nil {
		t.Fatal(err)
	}
	withRef := params
	withRef.ROIReportRef = &DiagnosisROIRef{ReportID: "roi-1", VersionNo: 3}
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", withRef, false)
	if err != nil {
		t.Fatal(err)
	}
	copies := inputIDs(inputs, inputROISummary)
	if !slices.Equal(copies, []string{"roi-1/3"}) {
		t.Fatalf("roi inputs = %v", copies)
	}
	result := mustCalculate(t, withRef, inputs)
	shown, _ := json.Marshal(result.ROIReference)
	original, _ := json.Marshal(sampleROISummary())
	if string(shown) != string(original) || !slices.Contains(result.Rules, "roi_reference.shown_as_is") {
		t.Fatalf("roi_reference = %s, rules %v", shown, result.Rules)
	}
	plain := mustCalculate(t, params, without)
	withSections, _ := json.Marshal(result.Sections)
	plainSections, _ := json.Marshal(plain.Sections)
	if string(withSections) != string(plainSections) || !slices.Equal(result.Gaps, plain.Gaps) {
		t.Fatal("the ROI summary changed a dimension")
	}
	// It recomputes from the stored copy alone.
	storedParams, _ := json.Marshal(withRef)
	storedInputs, _ := json.Marshal(inputs)
	again, err := RecomputeDiagnosis(DiagnosisCalcVersion, storedParams, storedInputs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustJSON(t, again), mustJSON(t, result); got != want {
		t.Fatalf("recomputed:\n%s\n%s", got, want)
	}
	// A copy without its param, or a param without its copy, is not a
	// stored version.
	if _, err = CalculateDiagnosis(params, inputs); err == nil {
		t.Fatal("an ROI copy no param names was accepted")
	}
	if _, err = CalculateDiagnosis(withRef, without); err == nil {
		t.Fatal("an ROI param with no copy was accepted")
	}

	// A version that is not there: refused like a missing account, with
	// nothing about any account read.
	for _, ref := range []DiagnosisROIRef{{ReportID: "roi-1", VersionNo: 4}, {ReportID: "", VersionNo: 0}} {
		store, log = diagFixture()
		store.roiSummaries = fakeROISummaries{log: log}
		missing := params
		missing.ROIReportRef = &ref
		missing.Window.End = "2026-01-01" // and bad params besides: existence comes first
		if _, _, err = store.gatherDiagnosisInputs(t.Context(), "ws", "u1", missing, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%+v = %v, want ErrNotFound", ref, err)
		}
		if slices.ContainsFunc(log.calls, func(call string) bool { return !strings.HasPrefix(call, "exists:") && call != "roi" }) {
			t.Fatalf("account data was read: %v", log.calls)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// ---------------------------------------------------------------- collection

// What a selected dimension needs is read only when it is selected, and
// never changes another dimension: the comparison window's records for
// performance, undated records for cadence, failed / unknown / removed ones
// for the execution flow. With no dimension the inputs are PR 1's.
func TestTheCollectionReadsWhatTheSelectedDimensionsNeed(t *testing.T) {
	store, _ := diagFixture()
	delivery := store.Delivery.(fakeDiagDelivery)
	delivery.publications = append(delivery.publications, DiagPublication{PublicationRecordID: "p-august", WorkID: "w-a1",
		Channel: "xiaohongshu", Status: "verified_published", PublishedAt: diagAt("2026-08-20T10:00:00+08:00")})
	store.Delivery = delivery
	own := store.own.(fakeDiagOwn)
	own.excerpts = append(own.excerpts, FeedbackExcerpt{FeedbackExcerptID: "e-august", PublicationRecordID: "p-august",
		SourceType: ExcerptComment, Tags: []string{"八月"}, OccurredAt: *diagAt("2026-09-02T10:00:00+08:00")})
	store.own = own
	gather := func(dimensions ...DiagnosisDimensionParam) []DiagnosisInput {
		params := opdiagParams(ScopeAccount, dimensions, "a1")
		_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
		if err != nil {
			t.Fatal(err)
		}
		return inputs
	}
	plain := inputIDs(gather(), inputPublication)
	if slices.Contains(plain, "p-august") || slices.Contains(plain, "p-undated") || slices.Contains(plain, "p-failed") {
		t.Fatalf("with no dimension: %v", plain)
	}
	if got := inputIDs(gather(DiagnosisDimensionParam{Key: DimensionPerformance, Metrics: []Metric{MetricLike}}), inputPublication); !slices.Contains(got, "p-august") {
		t.Fatalf("performance: %v", got)
	}
	if got := inputIDs(gather(DiagnosisDimensionParam{Key: DimensionCadence}), inputPublication); !slices.Contains(got, "p-undated") {
		t.Fatalf("cadence: %v", got)
	}
	if got := inputIDs(gather(DiagnosisDimensionParam{Key: DimensionExecutionFlow}), inputPublication); !slices.Contains(got, "p-failed") {
		t.Fatalf("execution flow: %v", got)
	}
	// The comparison window's feedback is not this report's audience
	// feedback, with or without performance.
	audience := DiagnosisDimensionParam{Key: DimensionAudienceFeedback}
	alone := gather(audience)
	withPerformance := gather(audience, DiagnosisDimensionParam{Key: DimensionPerformance, Metrics: []Metric{MetricLike}})
	if slices.Contains(inputIDs(withPerformance, inputExcerpt), "e-august") ||
		!slices.Equal(inputIDs(alone, inputExcerpt), inputIDs(withPerformance, inputExcerpt)) {
		t.Fatalf("excerpts alone %v, with performance %v", inputIDs(alone, inputExcerpt), inputIDs(withPerformance, inputExcerpt))
	}
}

// ---------------------------------------------------------------- T051 (module half)

// T051: a preview is exactly what generating would store as the result,
// and it touches no database: this store has none.
func TestAPreviewIsTheCalculatorsResult(t *testing.T) {
	store, _ := diagFixture()
	params := opdiagParams(ScopeBrand, allSix(), "a1", "a2")
	params.Window.Timezone, params.GeneratedAt = "UTC", "1999-01-01T00:00:00Z"
	now := *diagAt("2026-10-02T01:00:00Z")
	preview, err := store.PreviewDiagnosis(t.Context(), "ws", "u1", &params, now)
	if err != nil {
		t.Fatal(err)
	}
	used := params
	used.Window.Timezone, used.GeneratedAt = "Asia/Shanghai", "2026-10-02T01:00:00Z"
	store, _ = diagFixture()
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", used, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mustJSON(t, preview), mustJSON(t, mustCalculate(t, used, inputs)); got != want {
		t.Fatalf("preview:\n%s\ncalculator:\n%s", got, want)
	}
	store, log := diagFixture()
	foreign := opdiagParams(ScopeBrand, allSix(), "a1", "b-foreign")
	if _, err = store.PreviewDiagnosis(t.Context(), "ws", "u1", &foreign, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign account = %v", err)
	}
	if slices.ContainsFunc(log.calls, func(call string) bool { return !strings.HasPrefix(call, "exists:") }) {
		t.Fatalf("account data was read: %v", log.calls)
	}
	if _, err = store.PreviewDiagnosis(t.Context(), "ws", "u1", nil, now); err == nil {
		t.Fatal("a preview with no params")
	}
}

// Ruling Q7, beyond contract §5.10's latest-sample (whose second sample is
// after generation, so "earliest" and "latest" agree there): of two samples
// before generation the later is taken, a tie on sampled_at goes to the
// later created_at, and then to the larger id.
func TestTheLastSampleBeforeGenerationIsTaken(t *testing.T) {
	s := newDiagSample(t).account("a1", "xiaohongshu", "主号").rules(DiagOperatingRules{}).
		published("p1", "w1", "a1", "xiaohongshu", "verified_published", "2026-09-01T10:00:00+08:00").
		published("p2", "w2", "a1", "xiaohongshu", "verified_published", "2026-09-01T11:00:00+08:00").
		metric("m-a", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(10), "", "2026-09-02T10:00:00+08:00").
		metric("m-b", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(12), "", "2026-09-04T10:00:00+08:00").
		metric("m-c", "p1", "xiaohongshu", "a1", MetricFavorite, diagCount(25), "", "2026-09-09T10:00:00+08:00")
	// p2: two samples at the same moment; m-e was typed in later.
	s.add(inputMetric, "m-e", diagMetricFields{PublicationRecordID: "p2", Platform: "xiaohongshu", AccountID: "a1",
		Metric: "favorite", Value: diagCount(7), SampledAt: stamp("2026-09-03T10:00:00+08:00"), CreatedAt: stamp("2026-09-04T10:00:00+08:00")})
	s.add(inputMetric, "m-f", diagMetricFields{PublicationRecordID: "p2", Platform: "xiaohongshu", AccountID: "a1",
		Metric: "favorite", Value: diagCount(3), SampledAt: stamp("2026-09-03T10:00:00+08:00"), CreatedAt: stamp("2026-09-03T10:00:00+08:00")})
	params := opdiagParams(ScopeAccount, only(DiagnosisDimensionParam{Key: DimensionPerformance, Metrics: []Metric{MetricFavorite}}), "a1")
	params.GeneratedAt = "2026-09-05T01:00:00Z"
	result := mustCalculate(t, params, s.inputs)
	group := onlyGroup(t, factsOf[PerformanceFacts](t, dimensionOf(t, result, "account:a1", DimensionPerformance)), "xiaohongshu", "favorite")
	// 12 (p1, 09-04) + 7 (p2, entered later): never 10, 25 or 3.
	if group.Current.Sum == nil || *group.Current.Sum != "19" || group.Current.Publications != 2 {
		t.Fatalf("current = %+v", group.Current)
	}
}
