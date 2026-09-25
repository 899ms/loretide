package feedbacklearning

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// specs/035 PR 1 without a database: the controlled sets, the params, the
// collection order, the stored copy and its fingerprint, the scope-only
// calculator and the public summary's allow-list (T009 to T014, T021, T023,
// T026).
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md

// ---------------------------------------------------------------- fakes

// diagLog records every adapter call in order.
type diagLog struct{ calls []string }

func (l *diagLog) add(call string) { l.calls = append(l.calls, call) }

type fakeDiagAccounts struct {
	log      *diagLog
	accounts map[string]DiagAccount
	profiles map[string]DiagProfile
}

func (f fakeDiagAccounts) AccountExists(_ context.Context, _, accountID string) error {
	f.log.add("exists:" + accountID)
	if _, ok := f.accounts[accountID]; !ok {
		return ErrNotFound
	}
	return nil
}

func (f fakeDiagAccounts) Account(_ context.Context, _, accountID string) (DiagAccount, error) {
	f.log.add("account:" + accountID)
	return f.accounts[accountID], nil
}

func (f fakeDiagAccounts) CurrentProfile(_ context.Context, _, accountID string) (DiagProfile, error) {
	f.log.add("profile:" + accountID)
	return f.profiles[accountID], nil
}

type fakeDiagRules struct{ log *diagLog }

func (f fakeDiagRules) OperatingRules(context.Context, string) (DiagOperatingRules, error) {
	f.log.add("rules")
	return DiagOperatingRules{Cadence: map[string]int64{"xiaohongshu": 2}}, nil
}

func (f fakeDiagRules) Location(context.Context, string) *time.Location {
	location, _ := time.LoadLocation("Asia/Shanghai")
	return location
}

type fakeDiagTopics struct {
	log   *diagLog
	cards map[string]string
}

func (f fakeDiagTopics) TopicCardAccount(_ context.Context, _, _, topicCardID string) (string, error) {
	f.log.add("topic:" + topicCardID)
	accountID, ok := f.cards[topicCardID]
	if !ok {
		return "", ErrNotFound
	}
	return accountID, nil
}

type fakeDiagWorks struct {
	log   *diagLog
	works map[string]DiagWork
}

func (f fakeDiagWorks) WorkExists(_ context.Context, _, _, workID string) error {
	f.log.add("work-exists:" + workID)
	if _, ok := f.works[workID]; !ok {
		return ErrNotFound
	}
	return nil
}

func (f fakeDiagWorks) Work(_ context.Context, _, _, workID string) (DiagWork, error) {
	f.log.add("work:" + workID)
	work, ok := f.works[workID]
	if !ok {
		return DiagWork{}, ErrNotFound
	}
	return work, nil
}

type fakeDiagDelivery struct {
	log          *diagLog
	publications []DiagPublication
	reviews      []DiagReview
	tasks        []DiagDeliveryTask
}

func (f fakeDiagDelivery) Publications(context.Context, string, string) ([]DiagPublication, error) {
	f.log.add("publications")
	return slices.Clone(f.publications), nil
}

func (f fakeDiagDelivery) Reviews(context.Context, string, string) ([]DiagReview, error) {
	f.log.add("reviews")
	return slices.Clone(f.reviews), nil
}

func (f fakeDiagDelivery) Tasks(context.Context, string, string) ([]DiagDeliveryTask, error) {
	f.log.add("tasks")
	return slices.Clone(f.tasks), nil
}

type fakeDiagOwn struct {
	log      *diagLog
	metrics  []ManualMetric
	excerpts []FeedbackExcerpt
	marks    []WorkMark
}

func (f fakeDiagOwn) ownDiagnosisRecords(_ context.Context, _ string, publicationIDs, workIDs []string) (
	[]ManualMetric, []FeedbackExcerpt, []WorkMark, error) {
	f.log.add("own")
	metrics, excerpts, marks := []ManualMetric{}, []FeedbackExcerpt{}, []WorkMark{}
	for _, metric := range f.metrics {
		if slices.Contains(publicationIDs, metric.PublicationRecordID) {
			metrics = append(metrics, metric)
		}
	}
	for _, excerpt := range f.excerpts {
		if slices.Contains(publicationIDs, excerpt.PublicationRecordID) {
			excerpts = append(excerpts, excerpt)
		}
	}
	for _, mark := range f.marks {
		if slices.Contains(workIDs, mark.WorkID) {
			marks = append(marks, mark)
		}
	}
	return metrics, excerpts, marks, nil
}

func diagAt(text string) *time.Time {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return &at
}

// diagFixture is one brand: two accounts of its own (a1 "品牌主号" on
// xiaohongshu, a2 "副号" on douyin), a third account a3 that is also its
// own but not in the report, and publication records around September 2026
// in Asia/Shanghai.
func diagFixture() (*DiagnosisStore, *diagLog) {
	log := &diagLog{}
	confirmed := map[string]string{}
	for _, key := range profileFieldKeys {
		confirmed[key] = ProfileFieldConfirmed
	}
	partly := map[string]string{"audience": ProfileFieldConfirmed, "positioning": "pending"}
	store := &DiagnosisStore{
		Store: &Store{},
		Accounts: fakeDiagAccounts{log: log,
			accounts: map[string]DiagAccount{
				"a1": {AccountID: "a1", Platform: "xiaohongshu", DisplayName: "品牌主号"},
				"a2": {AccountID: "a2", Platform: "douyin", DisplayName: "副号"},
				"a3": {AccountID: "a3", Platform: "douyin", DisplayName: "第三个号"},
			},
			profiles: map[string]DiagProfile{
				"a1": {RevisionID: "pr-a1-9", FieldStatus: partly},
				"a2": {RevisionID: "pr-a2-1", FieldStatus: confirmed},
			}},
		Rules: fakeDiagRules{log: log},
		Topics: fakeDiagTopics{log: log, cards: map[string]string{
			"card-a1": "a1", "card-a2": "a2", "card-a3": "a3", "card-none": "",
		}},
		Works: fakeDiagWorks{log: log, works: map[string]DiagWork{
			"w-a1":       {WorkID: "w-a1", TopicCardID: "card-a1", Title: "面料对比"},
			"w-a2":       {WorkID: "w-a2", TopicCardID: "card-a2", Title: "穿搭"},
			"w-a3":       {WorkID: "w-a3", TopicCardID: "card-a3", Title: "别的号"},
			"w-history":  {WorkID: "w-history", Title: "历史导入", HistoricalImport: true},
			"w-lostcard": {WorkID: "w-lostcard", TopicCardID: "card-gone", Title: "卡已删"},
		}},
		Delivery: fakeDiagDelivery{log: log,
			publications: []DiagPublication{
				{PublicationRecordID: "p-a1", WorkID: "w-a1", Channel: "xiaohongshu", Status: "verified_published",
					PublishedAt: diagAt("2026-09-10T10:00:00+08:00")},
				{PublicationRecordID: "p-a2", WorkID: "w-a2", Channel: "douyin", Status: "reported_published",
					PublishedAt: diagAt("2026-09-30T23:30:00+08:00")},
				{PublicationRecordID: "p-a3", WorkID: "w-a3", Channel: "douyin", Status: "verified_published",
					PublishedAt: diagAt("2026-09-12T10:00:00+08:00")},
				{PublicationRecordID: "p-history", WorkID: "w-history", Channel: "xiaohongshu", Status: "verified_published",
					PublishedAt: diagAt("2026-09-05T10:00:00+08:00")},
				{PublicationRecordID: "p-lostcard", WorkID: "w-lostcard", Channel: "xiaohongshu", Status: "verified_published",
					PublishedAt: diagAt("2026-09-06T10:00:00+08:00")},
				{PublicationRecordID: "p-late", WorkID: "w-a1", Channel: "xiaohongshu", Status: "verified_published",
					PublishedAt: diagAt("2026-10-01T00:10:00+08:00")},
				{PublicationRecordID: "p-undated", WorkID: "w-a1", Channel: "xiaohongshu", Status: "verified_published"},
				{PublicationRecordID: "p-failed", WorkID: "w-a1", Channel: "xiaohongshu", Status: "failed",
					PublishedAt: diagAt("2026-09-10T10:00:00+08:00")},
			},
			reviews: []DiagReview{
				{ReviewRequestID: "r-a1", AccountID: "a1", Status: "pending", RequestedAt: *diagAt("2026-09-20T10:00:00+08:00")},
				{ReviewRequestID: "r-a3", AccountID: "a3", Status: "pending", RequestedAt: *diagAt("2026-09-20T10:00:00+08:00")},
				{ReviewRequestID: "r-approved", AccountID: "a2", Status: "approved", RequestedAt: *diagAt("2026-09-01T10:00:00+08:00")},
			},
			tasks: []DiagDeliveryTask{
				{DeliveryTaskID: "t-a2", ReviewRequestID: "r-approved", Status: "held"},
				{DeliveryTaskID: "t-done", ReviewRequestID: "r-approved", Status: "handed_off"},
			}},
		own: fakeDiagOwn{log: log,
			metrics: []ManualMetric{
				{ManualMetricID: "m-1", PublicationRecordID: "p-a1", Platform: "xiaohongshu", AccountID: "a1",
					Metric: MetricFavorite, StatWindow: "发布后 7 天", EvidenceNote: "截图 0412",
					SampledAt: *diagAt("2026-09-17T10:00:00+08:00"), CreatedAt: *diagAt("2026-09-17T10:00:00+08:00")},
			},
			excerpts: []FeedbackExcerpt{
				{FeedbackExcerptID: "e-1", PublicationRecordID: "p-a1", SourceType: ExcerptComment,
					RedactedExcerpt: "价格有点贵", Interpretation: "价格敏感", Tags: []string{"价格"},
					OccurredAt: *diagAt("2026-09-11T10:00:00+08:00")},
			},
			marks: []WorkMark{
				{MarkID: "mk-1", WorkID: "w-a1", Kind: MarkPillar, Item: "面料知识", Verdict: VerdictTagged,
					Note: "这是人写的说明", RecordedBy: "u1", CreatedAt: *diagAt("2026-09-12T10:00:00+08:00")},
			},
		},
	}
	return store, log
}

func diagParams(kind DiagnosisScope, accountIDs ...string) DiagnosisParams {
	return DiagnosisParams{
		Scope:       DiagnosisScopeParams{Kind: kind, AccountIDs: accountIDs},
		Window:      DiagnosisWindow{Start: "2026-09-01", End: "2026-09-30", Timezone: "Asia/Shanghai"},
		Dimensions:  []DiagnosisDimensionParam{},
		GeneratedAt: "2026-10-02T01:00:00Z",
	}
}

func inputIDs(inputs []DiagnosisInput, kind string) []string {
	ids := []string{}
	for _, input := range inputs {
		if input.Kind == kind {
			ids = append(ids, input.ID)
		}
	}
	return ids
}

func fieldErrorNaming(t *testing.T, err error, field string) {
	t.Helper()
	fieldErr, ok := errors.AsType[FieldError](err)
	if !ok || fieldErr.Field != field {
		t.Fatalf("err = %v, want a FieldError naming %q", err, field)
	}
}

// ---------------------------------------------------------------- T009

// T009 / FR-003 / FR-073 / contract §3: each set is exactly its values.
func TestDiagnosisControlledSetsAreExactly(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  []string
		want []string
	}{
		{"DiagnosisDimensions", toStrings(DiagnosisDimensions),
			[]string{"consistency", "coverage", "cadence", "performance", "audience_feedback", "execution_flow"}},
		{"DiagnosisScopes", toStrings(DiagnosisScopes), []string{"account", "brand"}},
		{"MarkKinds", toStrings(MarkKinds), []string{"pillar", "consistency"}},
		{"MarkVerdicts", toStrings(MarkVerdicts), []string{"tagged", "untagged", "consistent", "inconsistent", "unsure"}},
		{"DataOrigins", toStrings(DataOrigins), []string{"manual_only"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s = %v, want exactly %v", tc.name, tc.got, tc.want)
		}
	}
	// Outside the set, each is refused naming its field.
	params := diagParams("region", "a1")
	_, err := prepareDiagnosisParams(params)
	fieldErrorNaming(t, err, "scope.kind")
	params = diagParams(ScopeAccount, "a1")
	params.Dimensions = []DiagnosisDimensionParam{{Key: "growth"}}
	_, err = prepareDiagnosisParams(params)
	fieldErrorNaming(t, err, "dimensions")
	_, err = validateWorkMark(WorkMarkInput{WorkID: "w", Kind: "topic", Item: "x", Verdict: VerdictTagged})
	fieldErrorNaming(t, err, "kind")
	_, err = validateWorkMark(WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: "x", Verdict: "yes"})
	fieldErrorNaming(t, err, "verdict")
}

func toStrings[T ~string](values []T) []string {
	out := []string{}
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// ---------------------------------------------------------------- T010

// T010: profileFieldKeys are ip-profile's eleven JSON names in its order,
// and profileTextKeys are exactly its TextField items - read out of
// ip-profile's source, not imported.
func TestTheProfileKeysAgreeWithIPProfiles(t *testing.T) {
	path := filepath.Join(moduleDir(t), "..", "ip-profile", "profile.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names, textNames []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "ExpressionProfile" {
			return true
		}
		for _, field := range spec.Type.(*ast.StructType).Fields.List {
			tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
			name := strings.Split(tag, ",")[0]
			names = append(names, name)
			if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "TextField" {
				textNames = append(textNames, name)
			}
		}
		return false
	})
	if len(names) == 0 {
		t.Fatal("read no fields out of ip-profile's ExpressionProfile; the comparison would pass vacuously")
	}
	if !slices.Equal(names, profileFieldKeys) {
		t.Errorf("profileFieldKeys = %v, ip-profile has %v", profileFieldKeys, names)
	}
	if !slices.Equal(textNames, profileTextKeys) {
		t.Errorf("profileTextKeys = %v, ip-profile's TextField items are %v", profileTextKeys, textNames)
	}
}

// ---------------------------------------------------------------- T011

// T011 / FR-002 / FR-004 / FR-005 / SC-011: params that cannot be used are
// refused naming the field; in PR 1 any dimension is.
func TestDiagnosisParamsAreRefusedByName(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*DiagnosisParams)
		field string
	}{
		{"brand with no accounts", func(p *DiagnosisParams) { p.Scope = DiagnosisScopeParams{Kind: ScopeBrand, AccountIDs: []string{}} }, "scope.account_ids"},
		{"account with two", func(p *DiagnosisParams) { p.Scope.AccountIDs = []string{"a1", "a2"} }, "scope.account_ids"},
		{"account with none", func(p *DiagnosisParams) { p.Scope.AccountIDs = nil }, "scope.account_ids"},
		{"same account twice", func(p *DiagnosisParams) {
			p.Scope = DiagnosisScopeParams{Kind: ScopeBrand, AccountIDs: []string{"a1", "a1"}}
		}, "scope.account_ids"},
		{"ends before it starts", func(p *DiagnosisParams) { p.Window.End = "2026-08-31" }, "window"},
		{"not a date", func(p *DiagnosisParams) { p.Window.Start = "2026/09/01" }, "window.start"},
		{"comparison ends first", func(p *DiagnosisParams) {
			p.ComparisonWindow = &DiagnosisDateRange{Start: "2026-08-31", End: "2026-08-01"}
		}, "comparison_window"},
		{"a dimension twice", func(p *DiagnosisParams) {
			p.Dimensions = []DiagnosisDimensionParam{{Key: DimensionCadence}, {Key: DimensionCadence}}
		}, "dimensions"},
		{"an ROI reference", func(p *DiagnosisParams) { p.ROIReportRef = &DiagnosisROIRef{ReportID: "r", VersionNo: 1} }, "roi_report_ref"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := diagParams(ScopeAccount, "a1")
			tc.edit(&params)
			_, err := prepareDiagnosisParams(params)
			fieldErrorNaming(t, err, tc.field)
		})
	}
	// Every one of the six is well formed and still refused in PR 1: this
	// build registers no dimension calculator.
	for _, dimension := range DiagnosisDimensions {
		params := diagParams(ScopeAccount, "a1")
		params.Dimensions = []DiagnosisDimensionParam{{Key: dimension}}
		if _, err := prepareDiagnosisParams(params); err != nil {
			t.Fatalf("%s: %v", dimension, err)
		}
		_, err := CalculateDiagnosis(params, nil)
		fieldErrorNaming(t, err, "dimensions")
		store, log := diagFixture()
		_, _, err = store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
		fieldErrorNaming(t, err, "dimensions")
		if slices.ContainsFunc(log.calls, func(call string) bool { return !strings.HasPrefix(call, "exists:") }) {
			t.Fatalf("%s: account data was read before the dimension was refused: %v", dimension, log.calls)
		}
	}
}

// ---------------------------------------------------------------- T012

// T012 / FR-002 / US1 scenarios 2, 3: the window is [start 00:00, end+1
// 00:00) in the brand's timezone. 23:30 on the last day is in, 00:10 the next
// day is out, and a record with no published_at is in no window.
func TestTheDiagnosisWindowIsTheBrandsCalendarDays(t *testing.T) {
	prepared, err := prepareDiagnosisParams(diagParams(ScopeAccount, "a1"))
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]bool{
		"2026-09-30T23:30:00+08:00": true,
		"2026-10-01T00:10:00+08:00": false,
		"2026-09-01T00:00:00+08:00": true,
		"2026-08-31T23:59:59+08:00": false,
		// 2026-09-30T16:00Z is 10-01 00:00 in Shanghai: out, although it is
		// still September in UTC.
		"2026-09-30T16:00:00Z": false,
	} {
		if got := prepared.inWindow(diagAt(text)); got != want {
			t.Errorf("%s in window = %v, want %v", text, got, want)
		}
	}
	if prepared.inWindow(nil) {
		t.Error("a record with no published_at fell into the window")
	}
	store, _ := diagFixture()
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", diagParams(ScopeBrand, "a1", "a2"), false)
	if err != nil {
		t.Fatal(err)
	}
	// p-a2 is at 23:30 on the last day; p-late and p-undated are a1's too,
	// and would be here if the window let them in.
	if got := inputIDs(inputs, inputPublication); !slices.Contains(got, "p-a1") || !slices.Contains(got, "p-a2") ||
		slices.Contains(got, "p-late") || slices.Contains(got, "p-undated") {
		t.Fatalf("publications collected = %v", got)
	}
}

// ---------------------------------------------------------------- T013

// fieldNames lists every JSON field name reachable from a type, nested ones
// included. It stops at stop types, raw JSON and times.
func fieldNames(typ reflect.Type, stop ...reflect.Type) []string {
	var names []string
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || seen[t] || slices.Contains(stop, t) ||
			t == reflect.TypeFor[time.Time]() || t == reflect.TypeFor[json.RawMessage]() {
			return
		}
		seen[t] = true
		for i := range t.NumField() {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name != "" {
				names = append(names, name)
			} else if !field.Anonymous {
				names = append(names, field.Name)
			}
			walk(field.Type)
		}
	}
	walk(typ)
	return names
}

var diagnosisInputFieldTypes = []reflect.Type{
	reflect.TypeFor[DiagnosisInput](), reflect.TypeFor[diagAccountFields](), reflect.TypeFor[diagProfileFields](),
	reflect.TypeFor[DiagOperatingRules](), reflect.TypeFor[diagPublicationFields](), reflect.TypeFor[diagWorkFields](),
	reflect.TypeFor[diagTopicCardFields](), reflect.TypeFor[diagMetricFields](), reflect.TypeFor[diagExcerptFields](),
	reflect.TypeFor[diagReviewFields](), reflect.TypeFor[diagDeliveryTaskFields](), reflect.TypeFor[diagWorkMarkFields](),
}

// T013 / FR-042 / SC-010 (first half): what the stored inputs may never
// hold, by field name. A metric's own value is an input; a profile item's
// text is not, so the profile copy has no "value" at any depth.
func TestDiagnosisInputsHoldOnlyWhatTheCalculatorNeeds(t *testing.T) {
	checked := 0
	for _, typ := range diagnosisInputFieldTypes {
		for _, name := range fieldNames(typ) {
			checked++
			for _, forbidden := range []string{"redacted_excerpt", "interpretation", "evidence_note",
				"customer_ref", "order_ref", "recorded_by", "note", "persona_prompt", "decision_note"} {
				if name == forbidden {
					t.Errorf("%s has field %q", typ, name)
				}
			}
		}
	}
	if checked < 30 {
		t.Fatalf("checked only %d fields; the guard would pass vacuously", checked)
	}
	if names := fieldNames(reflect.TypeFor[diagProfileFields]()); slices.Contains(names, "value") ||
		slices.Contains(names, "values") {
		t.Errorf("the profile copy holds an item's text: %v", names)
	}
	// And the collected copy of a real fixture, by JSON key.
	store, _ := diagFixture()
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", diagParams(ScopeBrand, "a1", "a2"), false)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inputs)
	for _, forbidden := range []string{"价格有点贵", "价格敏感", "截图 0412", "这是人写的说明", `"recorded_by"`, `"note"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("the stored inputs contain %q", forbidden)
		}
	}
}

// T013: the fingerprint does not depend on field order and does depend on
// every field's value.
func TestTheInputFingerprintIsOrderFreeAndValueSensitive(t *testing.T) {
	type forward struct {
		A string `json:"account_id"`
		B string `json:"status"`
	}
	type backward struct {
		B string `json:"status"`
		A string `json:"account_id"`
	}
	one, err := newDiagnosisInput(inputReview, "r1", forward{A: "a1", B: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	two, _ := newDiagnosisInput(inputReview, "r1", backward{A: "a1", B: "pending"})
	three, _ := newDiagnosisInput(inputReview, "r1", map[string]string{"status": "pending", "account_id": "a1"})
	if one.Fingerprint != two.Fingerprint || one.Fingerprint != three.Fingerprint {
		t.Fatalf("field order changed the fingerprint: %s %s %s", one.Fingerprint, two.Fingerprint, three.Fingerprint)
	}
	for _, changed := range []forward{{A: "a2", B: "pending"}, {A: "a1", B: "approved"}, {A: "a1", B: "pending "}} {
		other, _ := newDiagnosisInput(inputReview, "r1", changed)
		if other.Fingerprint == one.Fingerprint {
			t.Errorf("%+v has the same fingerprint as the original", changed)
		}
	}
	// A nil metric value and a zero one are different inputs.
	zero := int64(0)
	unknown, _ := newDiagnosisInput(inputMetric, "m", diagMetricFields{Metric: "favorite"})
	known, _ := newDiagnosisInput(inputMetric, "m", diagMetricFields{Metric: "favorite", Value: &zero})
	if unknown.Fingerprint == known.Fingerprint {
		t.Error("a nil value and a 0 value have the same fingerprint")
	}
}

// ---------------------------------------------------------------- T014 / T019

func shuffled(inputs []DiagnosisInput, seed uint64) []DiagnosisInput {
	out := slices.Clone(inputs)
	random := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	random.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// T014 / FR-010 / FR-073: a scope-only diagnosis is the same bytes in any
// input order, says manual_only and opdiag-calc/1, and has the shape of
// contract §5.2: accounts by name, a section per account, the unknown
// account section only when something has no account, and the brand level.
func TestAScopeOnlyDiagnosisIsDeterministic(t *testing.T) {
	store, _ := diagFixture()
	params := diagParams(ScopeBrand, "a2", "a1")
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CalculateDiagnosis(params, inputs)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(result)
	for seed := range uint64(50) {
		again, err := CalculateDiagnosis(params, shuffled(inputs, seed))
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := json.Marshal(again); string(got) != string(want) {
			t.Fatalf("seed %d changed the result:\n%s\n%s", seed, want, got)
		}
	}
	if result.CalcVersion != "opdiag-calc/1" || result.DataOrigin != DataOriginManualOnly || result.ROIReference != nil {
		t.Fatalf("calc_version %q, data_origin %q, roi_reference %v", result.CalcVersion, result.DataOrigin, result.ROIReference)
	}
	// "副号" sorts before "品牌主号".
	if len(result.Scope.Accounts) != 2 || result.Scope.Accounts[0].AccountID != "a2" ||
		result.Scope.Accounts[1].ProfileRevisionID != "pr-a1-9" {
		t.Fatalf("scope accounts = %+v", result.Scope.Accounts)
	}
	// p-a1, p-a2, p-history and p-lostcard: a3's record is another account's,
	// p-late and p-undated are outside the window, p-failed is not published.
	counts := result.Scope.InputCounts
	if counts.Publications != 4 || counts.Works != 4 || counts.Metrics != 1 || counts.Excerpts != 1 ||
		counts.WorkMarks != 1 || counts.Reviews != 2 || counts.DeliveryTasks != 1 {
		t.Fatalf("input counts = %+v", counts)
	}
	if result.Scope.HistoricalImportPublications != 1 || !slices.Contains(result.Rules, "scope.historical_import_account_unknown") {
		t.Fatalf("historical imports = %d, rules = %v", result.Scope.HistoricalImportPublications, result.Rules)
	}
	sections := []string{}
	for _, section := range result.Sections {
		sections = append(sections, section.Section+":"+section.AccountID)
		if len(section.Dimensions) != 0 {
			t.Errorf("section %s has dimensions in PR 1: %v", section.Section, section.Dimensions)
		}
	}
	if !slices.Equal(sections, []string{"account:a2", "account:a1", "unknown_account:", "brand:"}) {
		t.Fatalf("sections = %v", sections)
	}
	// a1 has one item confirmed and ten not (one pending, nine never
	// answered); a2 has all eleven confirmed.
	if len(result.Gaps) != 10 {
		t.Fatalf("%d gaps, want 10: %+v", len(result.Gaps), result.Gaps)
	}
	for _, gap := range result.Gaps {
		if gap.Kind != GapProfileFieldPending || gap.AccountID != "a1" || gap.FixRoute != "account_settings" ||
			!slices.Contains(result.Refs, "gap/"+gap.GapKey) {
			t.Errorf("gap = %+v", gap)
		}
	}
	if result.Refs[0] != "scope" {
		t.Errorf("refs = %v", result.Refs)
	}

	// An account report has one section and no unknown_account section, but
	// still names the historical-import limitation.
	accountParams := diagParams(ScopeAccount, "a1")
	store, _ = diagFixture()
	_, accountInputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", accountParams, false)
	if err != nil {
		t.Fatal(err)
	}
	accountResult, err := CalculateDiagnosis(accountParams, accountInputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountResult.Sections) != 1 || accountResult.Sections[0].Section != "account" ||
		!slices.Contains(accountResult.Rules, "scope.historical_import_account_unknown") {
		t.Fatalf("account report sections %+v, rules %v", accountResult.Sections, accountResult.Rules)
	}
}

// T019 / FR-045 (the PR 1 part of SC-005): a stored scope-only version,
// written as JSON and read back, recomputes to the same bytes under its own
// calc version; an unknown calc version is refused, not recomputed with the
// current one.
func TestAStoredScopeOnlyVersionRecomputes(t *testing.T) {
	for _, params := range []DiagnosisParams{diagParams(ScopeBrand, "a1", "a2"), diagParams(ScopeAccount, "a2")} {
		store, _ := diagFixture()
		_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
		if err != nil {
			t.Fatal(err)
		}
		result, err := CalculateDiagnosis(params, inputs)
		if err != nil {
			t.Fatal(err)
		}
		storedParams, _ := json.Marshal(params)
		storedInputs, _ := json.Marshal(inputs)
		storedResult, _ := json.Marshal(result)
		again, err := RecomputeDiagnosis(result.CalcVersion, storedParams, storedInputs)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := json.Marshal(again); string(got) != string(storedResult) {
			t.Fatalf("recomputed:\n%s\nstored:\n%s", got, storedResult)
		}
		if _, err = RecomputeDiagnosis("opdiag-calc/0", storedParams, storedInputs); !errors.Is(err, ErrCalcVersionUnavailable) {
			t.Fatalf("an unknown calc version = %v", err)
		}
		// A copy that no longer matches its fingerprint is not an input.
		tampered := slices.Clone(inputs)
		tampered[0].Fields = json.RawMessage(`{"platform":"douyin","display_name":"改过"}`)
		if _, err = CalculateDiagnosis(params, tampered); err == nil {
			t.Fatal("a tampered input was calculated from")
		}
	}
}

// ---------------------------------------------------------------- T026

// T026 / FR-081 / SC-009 / D14-V01: every account in the params is checked
// before anything about any account is read, and a foreign one stops the
// collection there - the same refusal as a missing one, with no profile,
// work, metric or feedback read.
func TestTheCollectionOrderChecksEveryAccountBeforeReadingAny(t *testing.T) {
	store, log := diagFixture()
	_, _, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", diagParams(ScopeBrand, "a1", "b-foreign", "a2"), false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("a foreign account = %v, want ErrNotFound", err)
	}
	if !slices.Equal(log.calls, []string{"exists:a1", "exists:a2", "exists:b-foreign"}) {
		t.Fatalf("calls = %v, want the three existence checks and nothing else", log.calls)
	}

	// A foreign account with otherwise unusable params still answers as a
	// missing one: existence is decision step 2, the params are 3 to 5.
	store, log = diagFixture()
	bad := diagParams(ScopeBrand, "a1", "b-foreign")
	bad.Window.End = "2026-01-01"
	if _, _, err = store.gatherDiagnosisInputs(t.Context(), "ws", "u1", bad, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign account and a bad window = %v, want ErrNotFound", err)
	}
	if slices.ContainsFunc(log.calls, func(call string) bool { return !strings.HasPrefix(call, "exists:") }) {
		t.Fatalf("account data was read: %v", log.calls)
	}

	// The whole order when everything is here.
	store, log = diagFixture()
	if _, _, err = store.gatherDiagnosisInputs(t.Context(), "ws", "u1", diagParams(ScopeBrand, "a2", "a1"), false); err != nil {
		t.Fatal(err)
	}
	stage := func(call string) int {
		switch {
		case strings.HasPrefix(call, "exists:"):
			return 1
		case strings.HasPrefix(call, "account:"), strings.HasPrefix(call, "profile:"):
			return 2
		case call == "rules":
			return 3
		case call == "publications":
			return 4
		case strings.HasPrefix(call, "work:"):
			return 5
		case strings.HasPrefix(call, "topic:"):
			return 6
		case call == "reviews", call == "tasks":
			return 7
		case call == "own":
			return 8
		}
		return 99
	}
	for i := 1; i < len(log.calls); i++ {
		if stage(log.calls[i]) < stage(log.calls[i-1]) {
			t.Fatalf("%s came after %s: %v", log.calls[i], log.calls[i-1], log.calls)
		}
	}
	if log.calls[0] != "exists:a1" || log.calls[1] != "exists:a2" || log.calls[len(log.calls)-1] != "own" {
		t.Fatalf("calls = %v", log.calls)
	}
}

// ---------------------------------------------------------------- inputs_changed

// FR-043: the change list matches inputs by (kind, id) and compares their
// fingerprints; nothing else about it is stored.
func TestInputsChangedComparesFingerprints(t *testing.T) {
	store, _ := diagFixture()
	params := diagParams(ScopeAccount, "a1")
	_, before, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
	if err != nil {
		t.Fatal(err)
	}
	if changes := diffDiagnosisInputs(before, before); changes.Changed || len(changes.Added)+len(changes.Modified)+len(changes.Removed) != 0 {
		t.Fatalf("the same inputs differ: %+v", changes)
	}
	own := store.own.(fakeDiagOwn)
	five := int64(5)
	own.metrics = append(own.metrics, ManualMetric{ManualMetricID: "m-2", PublicationRecordID: "p-a1",
		Platform: "xiaohongshu", AccountID: "a1", Metric: MetricLike, Value: &five,
		SampledAt: *diagAt("2026-09-18T10:00:00+08:00"), CreatedAt: *diagAt("2026-09-18T10:00:00+08:00")})
	own.marks[0].Verdict = VerdictUntagged
	store.own = own
	_, after, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
	if err != nil {
		t.Fatal(err)
	}
	changes := diffDiagnosisInputs(before, after)
	if !changes.Changed || !slices.Equal(changes.Added, []DiagnosisRecordRef{{Kind: "metric", ID: "m-2"}}) ||
		!slices.Equal(changes.Modified, []DiagnosisRecordRef{{Kind: "work_mark", ID: "mk-1"}}) || len(changes.Removed) != 0 {
		t.Fatalf("changes = %+v", changes)
	}
	// An account gone since: reading leniently leaves it out and reports it.
	accounts := store.Accounts.(fakeDiagAccounts)
	delete(accounts.accounts, "a1")
	if _, _, err = store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("generating for a deleted account = %v, want ErrNotFound", err)
	}
	_, lenient, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed := diffDiagnosisInputs(before, lenient).Removed; !slices.Contains(removed, DiagnosisRecordRef{Kind: "account", ID: "a1"}) {
		t.Fatalf("removed = %v", removed)
	}
}

// ---------------------------------------------------------------- T021 (pure part)

// T021 / contract §1.2: a pillar takes tagged or untagged, a consistency
// mark takes the other three and an account and a profile item. The pillar
// name is stored normalized.
func TestAWorkMarksKindDecidesItsVerdictsAndFields(t *testing.T) {
	cases := []struct {
		name  string
		in    WorkMarkInput
		field string
	}{
		{"pillar with a consistency verdict", WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: "穿搭", Verdict: VerdictConsistent}, "verdict"},
		{"consistency with a pillar verdict", WorkMarkInput{WorkID: "w", Kind: MarkConsistency, Item: "positioning", Verdict: VerdictTagged, AccountID: "a1"}, "verdict"},
		{"consistency without an account", WorkMarkInput{WorkID: "w", Kind: MarkConsistency, Item: "positioning", Verdict: VerdictUnsure}, "account_id"},
		{"consistency on no profile item", WorkMarkInput{WorkID: "w", Kind: MarkConsistency, Item: "persona_prompt", Verdict: VerdictUnsure, AccountID: "a1"}, "item"},
		{"no work", WorkMarkInput{Kind: MarkPillar, Item: "穿搭", Verdict: VerdictTagged}, "work_id"},
		{"blank pillar", WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: "  ", Verdict: VerdictTagged}, "item"},
		{"long pillar", WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: strings.Repeat("穿", MaxPillarRunes+1), Verdict: VerdictTagged}, "item"},
		{"long note", WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: "穿搭", Verdict: VerdictTagged, Note: strings.Repeat("字", MaxMarkNoteRunes+1)}, "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateWorkMark(tc.in)
			fieldErrorNaming(t, err, tc.field)
		})
	}
	// "é" composed and decomposed, with spaces around: one pillar.
	item, err := validateWorkMark(WorkMarkInput{WorkID: "w", Kind: MarkPillar, Item: " café ", Verdict: VerdictTagged})
	if err != nil || item != "café" {
		t.Fatalf("item = %q, %v", item, err)
	}
	for _, verdict := range []MarkVerdict{VerdictConsistent, VerdictInconsistent, VerdictUnsure} {
		if _, err = validateWorkMark(WorkMarkInput{WorkID: "w", Kind: MarkConsistency, Item: "expression_style",
			Verdict: verdict, AccountID: "a1"}); err != nil {
			t.Errorf("%s: %v", verdict, err)
		}
	}
}

// T021: the current mark of a (work, kind, item) is the latest by
// (created_at, mark_id); earlier ones stay in the record.
func TestTheLatestWorkMarkIsByCreatedAtThenID(t *testing.T) {
	at := *diagAt("2026-09-12T10:00:00+08:00")
	later := at.Add(time.Minute)
	marks := []WorkMark{
		{MarkID: "b", WorkID: "w1", Kind: MarkPillar, Item: "穿搭", Verdict: VerdictTagged, CreatedAt: at},
		{MarkID: "a", WorkID: "w1", Kind: MarkPillar, Item: "穿搭", Verdict: VerdictUntagged, CreatedAt: later},
		{MarkID: "c", WorkID: "w1", Kind: MarkPillar, Item: "面料知识", Verdict: VerdictTagged, CreatedAt: at},
		{MarkID: "e", WorkID: "w1", Kind: MarkPillar, Item: "面料知识", Verdict: VerdictUntagged, CreatedAt: at},
		{MarkID: "d", WorkID: "w1", Kind: MarkPillar, Item: "面料知识", Verdict: VerdictTagged, CreatedAt: at},
	}
	for seed := range uint64(10) {
		shuffledMarks := slices.Clone(marks)
		random := rand.New(rand.NewPCG(seed, 7))
		random.Shuffle(len(shuffledMarks), func(i, j int) { shuffledMarks[i], shuffledMarks[j] = shuffledMarks[j], shuffledMarks[i] })
		current := latestWorkMarks(shuffledMarks)
		if len(current) != 2 || current[0].MarkID != "a" || current[1].MarkID != "e" {
			t.Fatalf("seed %d: current = %+v", seed, current)
		}
	}
}

// ---------------------------------------------------------------- T023 / SC-011

// T023 / FR-046 / SC-010 (second half): DiagnosisSummary holds only contract
// §8's allow-list, nested fields included, and none of what it must not.
func TestTheDiagnosisSummaryHoldsOnlyItsAllowList(t *testing.T) {
	names := fieldNames(reflect.TypeFor[DiagnosisSummary](), reflect.TypeFor[ReportSummary]())
	if len(names) < 30 {
		t.Fatalf("read only %d field names; the check would pass vacuously", len(names))
	}
	for _, name := range names {
		if !slices.Contains(DiagnosisSummaryFields, name) {
			t.Errorf("DiagnosisSummary has %q, which contract §8 does not allow", name)
		}
		for _, forbidden := range []string{"inputs", "note", "created_by", "recorded_by", "redacted_excerpt",
			"interpretation", "evidence_note", "value", "customer_ref", "order_ref", "fix_route", "title"} {
			if name == forbidden {
				t.Errorf("DiagnosisSummary has %q", name)
			}
		}
	}
	// It is built from a stored result by copying, not by passing through.
	result, err := CalculateDiagnosis(diagParams(ScopeAccount, "a1"), mustGather(t, diagParams(ScopeAccount, "a1")))
	if err != nil {
		t.Fatal(err)
	}
	summary := summarizeDiagnosis(DiagnosisReportHeader{ReportID: "r", VersionNo: 1, CalcVersion: result.CalcVersion,
		CreatedBy: "someone", Title: "九月"}, result)
	encoded, _ := json.Marshal(summary)
	for _, forbidden := range []string{"someone", "九月", "account_settings"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("the summary contains %q: %s", forbidden, encoded)
		}
	}
	if len(summary.Gaps) != len(result.Gaps) || summary.Scope.Accounts[0].ProfileRevisionID != "pr-a1-9" {
		t.Fatalf("summary = %s", encoded)
	}
}

func mustGather(t *testing.T, params DiagnosisParams) []DiagnosisInput {
	t.Helper()
	store, _ := diagFixture()
	_, inputs, err := store.gatherDiagnosisInputs(t.Context(), "ws", "u1", params, false)
	if err != nil {
		t.Fatal(err)
	}
	return inputs
}

// SC-011 / FR-004 / FR-013 / contract §9: no score, grade, rating, rank or
// level, and no industry or role, anywhere in the params, the result, a
// stored version or the summary.
func TestThereIsNoScoreAndNoRoleInTheDiagnosis(t *testing.T) {
	checked := 0
	for _, typ := range []reflect.Type{
		reflect.TypeFor[DiagnosisParams](), reflect.TypeFor[DiagnosisResult](), reflect.TypeFor[DiagnosisSummary](),
		reflect.TypeFor[DiagnosisReportVersion](), reflect.TypeFor[DiagnosisReportRequest](), reflect.TypeFor[WorkMark](),
	} {
		for _, name := range fieldNames(typ, reflect.TypeFor[ReportSummary]()) {
			checked++
			for _, forbidden := range []string{"score", "grade", "rating", "rank", "level", "role", "industry"} {
				if strings.Contains(strings.ToLower(name), forbidden) {
					t.Errorf("%s has field %q", typ, name)
				}
			}
		}
	}
	if checked < 40 {
		t.Fatalf("checked only %d fields; the guard would pass vacuously", checked)
	}
}
