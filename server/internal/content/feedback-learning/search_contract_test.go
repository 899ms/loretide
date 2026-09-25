package feedbacklearning

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// specs/036 PR 4 without a database (T083, T084, T085, T086): the controlled
// sets, strict decoding, every validation's accepting and refusing side, the
// list filter, and the shape of what is answered.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	fieldErr, ok := errors.AsType[FieldError](err)
	if !ok || !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want a FieldError naming %q", err, field)
	}
	if fieldErr.Field != field {
		t.Fatalf("refusal named %q (%s), want %q", fieldErr.Field, fieldErr.Reason, field)
	}
}

// ---------------------------------------------------------------- sets

// T083 / FR-071 / FR-074 / contract §3: exactly two metrics, none of them a
// volume, a rank or 'other'; exactly two result kinds; exactly one source.
func TestSearchControlledSetsAreExactly(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  []string
		want []string
	}{
		{"SearchMetrics", toStrings(SearchMetrics), []string{"search_impression", "search_visit"}},
		{"RankResultKinds", toStrings(RankResultKinds), []string{"position", "not_found"}},
		{"SearchMetricSources", toStrings(SearchMetricSources), []string{"manual"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s = %v, want exactly %v", tc.name, tc.got, tc.want)
		}
	}
	for _, forbidden := range []string{"search_rank", "search_volume", "competition", "rank", "other"} {
		if slices.Contains(toStrings(SearchMetrics), forbidden) {
			t.Errorf("SearchMetrics holds %q; only what a platform offers may be recorded (FR-071)", forbidden)
		}
	}
	// 027's eleven are untouched: the search metrics are not in Metrics and
	// Metrics is not in the search set (Q2).
	if len(Metrics) != 11 {
		t.Errorf("027's Metrics has %d names, want its eleven", len(Metrics))
	}
	for _, metric := range SearchMetrics {
		if slices.Contains(toStrings(Metrics), string(metric)) {
			t.Errorf("%s was added to 027's Metrics", metric)
		}
	}
}

// The migration CHECK lists are a backstop; the Go sets are the authority,
// and this holds the two to each other. Found by table name, not number, so
// a renumbering at merge keeps it pointed at the right files.
func TestSearchMigrationChecksAgreeWithTheGoSets(t *testing.T) {
	migrations := filepath.Join(moduleDir(t), "..", "..", "..", "migrations")
	read := func(table string) string {
		matches, err := filepath.Glob(filepath.Join(migrations, "*_"+table+".up.sql"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("expected one create migration for %s, found %v (%v)", table, matches, err)
		}
		body, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	checkList := func(sql, column string) []string {
		match := regexp.MustCompile(`(?s)` + column + `\s+text NOT NULL CHECK \(` + column + ` IN \(([^)]*)\)\)`).FindStringSubmatch(sql)
		if match == nil {
			t.Fatalf("no CHECK list for %s", column)
		}
		values := []string{}
		for _, quoted := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(match[1], -1) {
			values = append(values, quoted[1])
		}
		return values
	}
	metric := read("content_search_metric")
	observation := read("content_search_rank_observation_revision")
	for _, tc := range []struct {
		name      string
		got, want []string
	}{
		{"metric.platform", checkList(metric, "platform"), toStrings(Platforms)},
		{"metric.metric", checkList(metric, "metric"), toStrings(SearchMetrics)},
		{"metric.source_type", checkList(metric, "source_type"), toStrings(SearchMetricSources)},
		{"observation.platform", checkList(observation, "platform"), toStrings(Platforms)},
		{"observation.result_kind", checkList(observation, "result_kind"), toStrings(RankResultKinds)},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s CHECK = %v, Go = %v", tc.name, tc.got, tc.want)
		}
	}
	// Q5: nothing about volume, competition or a combined rank is stored.
	for _, sql := range []string{metric, observation} {
		code := regexp.MustCompile(`(?m)--.*$`).ReplaceAllString(sql, "")
		for _, forbidden := range []string{"search_volume", "competition", "avg", "best", "current_rank", "median"} {
			if strings.Contains(code, forbidden) {
				t.Errorf("a search migration stores %q", forbidden)
			}
		}
	}
}

// ---------------------------------------------------------------- decoding

// FR-002 / FR-014 / Q5 / contract §4: an unknown member is refused by its own
// name - search volume and competition first of all - and a wrongly typed
// value by its JSON path, never by a Go type name.
func TestSearchBodiesAreDecodedStrictly(t *testing.T) {
	metric := `{"publication_record_id":"p1","platform":"xiaohongshu","account_id":"","metric":"search_impression",
		"value":1240,"unit":"次","stat_window":"发布后 7 天累计","sampled_at":"2026-10-02T13:30:00Z","evidence_note":"截图"%s}`
	observation := `{"platform":"xiaohongshu","account_id":"","query":"羊绒大衣能机洗吗","theme_id":"","publication_record_id":"",
		"observed_at":"2026-10-02T13:30:00Z","conditions":"未登录 上海 综合排序","result_kind":"position","position":7,
		"scanned_depth":null,"evidence_note":"截图 1002.png"%s}`
	for _, tc := range []struct{ name, extra, field string }{
		{"search volume", `,"search_volume":12000`, "search_volume"},
		{"competition", `,"competition":"low"`, "competition"},
		{"rank", `,"rank":3`, "rank"},
		{"best position", `,"best_position":1`, "best_position"},
		{"scope", `,"scope":"online"`, "scope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeSearchMetric([]byte(strings.Replace(metric, "%s", tc.extra, 1)))
			wantField(t, err, tc.field)
			_, err = DecodeRankObservation([]byte(strings.Replace(observation, "%s", tc.extra, 1)))
			wantField(t, err, tc.field)
			_, err = DecodeRankObservationRevision([]byte(strings.Replace(observation, "%s", `,"base_revision":1`+tc.extra, 1)))
			wantField(t, err, tc.field)
		})
	}
	// Wrong JSON types, named by the member only.
	_, err := DecodeSearchMetric([]byte(strings.Replace(strings.Replace(metric, "%s", "", 1), `"value":1240`, `"value":"1240"`, 1)))
	wantField(t, err, "value")
	_, err = DecodeRankObservation([]byte(strings.Replace(strings.Replace(observation, "%s", "", 1), `"position":7`, `"position":"7"`, 1)))
	wantField(t, err, "position")
	// A new observation has no base.
	_, err = DecodeRankObservation([]byte(strings.Replace(observation, "%s", `,"base_revision":1`, 1)))
	wantField(t, err, "base_revision")
	// A revision needs one.
	_, err = DecodeRankObservationRevision([]byte(strings.Replace(observation, "%s", "", 1)))
	wantField(t, err, "base_revision")
	// Two values, or none, are not a body.
	if _, err = DecodeSearchMetric([]byte(`{} {}`)); !errors.Is(err, ErrInvalid) {
		t.Errorf("two JSON values = %v, want ErrInvalid", err)
	}

	// The accepting side: the same bodies decode, and server-written fields
	// echoed back are accepted and dropped (contract §4).
	echoed := `,"recorded_by":"someone else","source_type":"csv_import","data_origin":"ai","rule":"x","created_at":"x"`
	decoded, err := DecodeSearchMetric([]byte(strings.Replace(metric, "%s", echoed+`,"search_metric_id":"m9"`, 1)))
	if err != nil || decoded.Value == nil || *decoded.Value != 1240 || decoded.StatWindow != "发布后 7 天累计" {
		t.Fatalf("metric decoded = %+v, %v", decoded, err)
	}
	revision, err := DecodeRankObservationRevision([]byte(strings.Replace(observation, "%s",
		echoed+`,"observation_id":"o9","revision":4,"base_revision":2,"voided":true`, 1)))
	if err != nil || revision.BaseRevision != 2 || !revision.Voided || revision.Input.Position == nil || *revision.Input.Position != 7 {
		t.Fatalf("revision decoded = %+v, %v", revision, err)
	}
}

// ---------------------------------------------------------------- metrics

func validSearchMetric() SearchMetricInput {
	value := int64(1240)
	return SearchMetricInput{
		PublicationRecordID: "p1", Platform: PlatformXiaohongshu, Metric: SearchMetricImpression,
		Value: &value, Unit: "次", StatWindow: "发布后 7 天累计", SampledAt: "2026-10-02T13:30:00+08:00",
		EvidenceNote: "笔记数据页截图 0928-1.png",
	}
}

// T084 / SC-009 / SC-010 / US7: each rule's accepting side and refusing side.
func TestSearchMetricValidation(t *testing.T) {
	sampled, err := ValidateSearchMetric(validSearchMetric())
	if err != nil {
		t.Fatalf("a valid metric is refused: %v", err)
	}
	if want := time.Date(2026, 10, 2, 5, 30, 0, 0, time.UTC); !sampled.Equal(want) || sampled.Location() != time.UTC {
		t.Fatalf("sampled_at = %v, want %v in UTC", sampled, want)
	}
	zero := int64(0)
	for name, value := range map[string]*int64{"unknown (null)": nil, "confirmed zero": &zero} {
		input := validSearchMetric()
		input.Value = value
		if _, err := ValidateSearchMetric(input); err != nil {
			t.Errorf("%s is refused: %v", name, err)
		}
	}
	for _, platform := range Platforms {
		input := validSearchMetric()
		input.Platform = platform
		if _, err := ValidateSearchMetric(input); err != nil {
			t.Errorf("platform %s is refused: %v", platform, err)
		}
	}
	input := validSearchMetric()
	input.Metric = SearchMetricVisit
	if _, err := ValidateSearchMetric(input); err != nil {
		t.Errorf("search_visit is refused: %v", err)
	}

	negative := int64(-1)
	for _, tc := range []struct {
		name   string
		change func(*SearchMetricInput)
		field  string
	}{
		{"no publication record", func(in *SearchMetricInput) { in.PublicationRecordID = "" }, "publication_record_id"},
		{"zhihu (Q8)", func(in *SearchMetricInput) { in.Platform = "zhihu" }, "platform"},
		{"search_rank", func(in *SearchMetricInput) { in.Metric = "search_rank" }, "metric"},
		{"search_volume", func(in *SearchMetricInput) { in.Metric = "search_volume" }, "metric"},
		{"027 metric", func(in *SearchMetricInput) { in.Metric = "impression" }, "metric"},
		{"negative", func(in *SearchMetricInput) { in.Value = &negative }, "value"},
		{"no stat_window", func(in *SearchMetricInput) { in.StatWindow = "" }, "stat_window"},
		{"blank stat_window", func(in *SearchMetricInput) { in.StatWindow = "  " }, "stat_window"},
		{"long stat_window", func(in *SearchMetricInput) { in.StatWindow = strings.Repeat("天", MaxShortRunes+1) }, "stat_window"},
		{"no sampled_at", func(in *SearchMetricInput) { in.SampledAt = "" }, "sampled_at"},
		{"bad sampled_at", func(in *SearchMetricInput) { in.SampledAt = "2026-10-02" }, "sampled_at"},
		{"long unit", func(in *SearchMetricInput) { in.Unit = strings.Repeat("次", MaxShortRunes+1) }, "unit"},
		{"long evidence", func(in *SearchMetricInput) { in.EvidenceNote = strings.Repeat("证", MaxNoteRunes+1) }, "evidence_note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := validSearchMetric()
			tc.change(&input)
			_, err := ValidateSearchMetric(input)
			wantField(t, err, tc.field)
		})
	}
	// At the limits, accepted.
	input = validSearchMetric()
	input.StatWindow = strings.Repeat("天", MaxShortRunes)
	input.EvidenceNote = strings.Repeat("证", MaxNoteRunes)
	if _, err := ValidateSearchMetric(input); err != nil {
		t.Errorf("values at the limits are refused: %v", err)
	}
}

// ---------------------------------------------------------------- observations

var observationNow = time.Date(2026, 10, 2, 14, 0, 0, 0, time.UTC)

func intPointer(value int) *int { return &value }

func validObservation() RankObservationInput {
	return RankObservationInput{
		Platform: PlatformXiaohongshu, Query: "羊绒大衣能机洗吗", ObservedAt: "2026-10-02T21:30:00+08:00",
		Conditions: "自己手机，未登录，定位上海，综合排序", ResultKind: RankPosition, Position: intPointer(7),
		EvidenceNote: "截图 1002-2130.png",
	}
}

// T085 / FR-073 / FR-074 / SC-009 / US8 scenarios 1-3.
func TestRankObservationValidation(t *testing.T) {
	record, err := ValidateRankObservation(validObservation(), observationNow)
	if err != nil {
		t.Fatalf("a valid observation is refused: %v", err)
	}
	if !record.ObservedAt.Equal(time.Date(2026, 10, 2, 13, 30, 0, 0, time.UTC)) || record.ObservedAt.Location() != time.UTC ||
		record.Position == nil || *record.Position != 7 || record.ScannedDepth != nil {
		t.Fatalf("validated = %+v", record)
	}
	notFound := validObservation()
	notFound.ResultKind, notFound.Position, notFound.ScannedDepth = RankNotFound, nil, intPointer(30)
	if record, err = ValidateRankObservation(notFound, observationNow); err != nil || *record.ScannedDepth != 30 || record.Position != nil {
		t.Fatalf("not_found in the first 30 = %+v, %v", record, err)
	}

	// The four combinations of the two integers, for each kind.
	for _, tc := range []struct {
		name            string
		kind            RankResultKind
		position, depth *int
		field           string
	}{
		{"position, only the right one", RankPosition, intPointer(1), nil, ""},
		{"position, neither", RankPosition, nil, nil, "position"},
		{"position, both", RankPosition, intPointer(3), intPointer(30), "scanned_depth"},
		{"position, only the wrong one", RankPosition, nil, intPointer(30), "position"},
		{"position below 1", RankPosition, intPointer(0), nil, "position"},
		{"not_found, only the right one", RankNotFound, nil, intPointer(1), ""},
		{"not_found, neither", RankNotFound, nil, nil, "scanned_depth"},
		{"not_found, both", RankNotFound, intPointer(3), intPointer(30), "position"},
		{"not_found, only the wrong one", RankNotFound, intPointer(3), nil, "scanned_depth"},
		{"not_found depth below 1", RankNotFound, nil, intPointer(0), "scanned_depth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := validObservation()
			input.ResultKind, input.Position, input.ScannedDepth = tc.kind, tc.position, tc.depth
			_, err := ValidateRankObservation(input, observationNow)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			wantField(t, err, tc.field)
		})
	}

	for _, tc := range []struct {
		name   string
		change func(*RankObservationInput)
		field  string
	}{
		{"zhihu (Q8)", func(in *RankObservationInput) { in.Platform = "zhihu" }, "platform"},
		{"kind outside the set", func(in *RankObservationInput) { in.ResultKind = "top10" }, "result_kind"},
		{"empty query", func(in *RankObservationInput) { in.Query = "" }, "query"},
		{"blank query", func(in *RankObservationInput) { in.Query = " 　 " }, "query"},
		{"long query", func(in *RankObservationInput) { in.Query = strings.Repeat("羊", MaxRankQueryRunes+1) }, "query"},
		{"empty conditions", func(in *RankObservationInput) { in.Conditions = "" }, "conditions"},
		{"long conditions", func(in *RankObservationInput) { in.Conditions = strings.Repeat("条", MaxRankConditionsRunes+1) }, "conditions"},
		{"empty evidence", func(in *RankObservationInput) { in.EvidenceNote = "" }, "evidence_note"},
		{"long evidence", func(in *RankObservationInput) { in.EvidenceNote = strings.Repeat("证", MaxRankEvidenceRunes+1) }, "evidence_note"},
		{"no observed_at", func(in *RankObservationInput) { in.ObservedAt = "" }, "observed_at"},
		{"bad observed_at", func(in *RankObservationInput) { in.ObservedAt = "昨天晚上" }, "observed_at"},
		{"observed_at 11 minutes ahead", func(in *RankObservationInput) {
			in.ObservedAt = observationNow.Add(11 * time.Minute).Format(time.RFC3339)
		}, "observed_at"},
		{"long account id", func(in *RankObservationInput) { in.AccountID = strings.Repeat("a", MaxShortRunes+1) }, "account_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := validObservation()
			tc.change(&input)
			_, err := ValidateRankObservation(input, observationNow)
			wantField(t, err, tc.field)
		})
	}

	// The accepting side of each bound: a device clock nine minutes fast,
	// the longest query, conditions and evidence.
	input := validObservation()
	input.ObservedAt = observationNow.Add(9 * time.Minute).Format(time.RFC3339)
	input.Query = strings.Repeat("羊", MaxRankQueryRunes)
	input.Conditions = strings.Repeat("条", MaxRankConditionsRunes)
	input.EvidenceNote = strings.Repeat("证", MaxRankEvidenceRunes)
	if _, err := ValidateRankObservation(input, observationNow); err != nil {
		t.Fatalf("values at the limits are refused: %v", err)
	}
}

// FR-073: the query is saved NFC and trimmed, nothing else - no case
// folding, no splitting.
func TestRankQueryIsNFCAndTrimmedOnly(t *testing.T) {
	input := validObservation()
	input.Query = "  Café 羊绒 "
	record, err := ValidateRankObservation(input, observationNow)
	if err != nil {
		t.Fatal(err)
	}
	if record.Query != "Café 羊绒" {
		t.Fatalf("query = %q, want NFC and trimmed", record.Query)
	}
	if NormalizeRankQuery("Cashmere") == NormalizeRankQuery("cashmere") {
		t.Fatal("the query was case-folded")
	}
}

// T089 / contract §7: at least one of the three filters; any one alone is
// enough; include_voided is true or false.
func TestRankObservationFilter(t *testing.T) {
	_, err := ParseRankObservationFilter("", "", "", "")
	wantField(t, err, "theme_id")
	_, err = ParseRankObservationFilter("", "", "   ", "")
	wantField(t, err, "theme_id")
	for _, tc := range [][3]string{{"t1", "", ""}, {"", "p1", ""}, {"", "", "羊绒大衣能机洗吗"}} {
		filter, err := ParseRankObservationFilter(tc[0], tc[1], tc[2], "")
		if err != nil || filter.IncludeVoided {
			t.Errorf("%v = %+v, %v", tc, filter, err)
		}
	}
	filter, err := ParseRankObservationFilter("t1", "", " 羊绒 ", "true")
	if err != nil || !filter.IncludeVoided || filter.Query != "羊绒" {
		t.Fatalf("filter = %+v, %v", filter, err)
	}
	_, err = ParseRankObservationFilter("t1", "", "", "yes")
	wantField(t, err, "include_voided")
}

// ---------------------------------------------------------------- shape

// T086 / FR-075 / FR-082 / SC-010: nothing answered combines observations.
// No field of an observation, a metric or a list of either is an average, a
// best, a median, a "current" rank, a score or a stored volume.
func TestSearchResponsesHaveNoSummaryField(t *testing.T) {
	forbidden := []string{"avg", "average", "mean", "best", "median", "current_rank", "rank", "score",
		"search_volume", "competition", "density", "keyword_count", "seo", "total", "sum"}
	checked := 0
	for _, value := range []any{RankObservation{}, SearchMetricRecord{}, RankObservationInput{}, SearchMetricInput{}} {
		kind := reflect.TypeOf(value)
		for i := range kind.NumField() {
			tag := strings.Split(kind.Field(i).Tag.Get("json"), ",")[0]
			checked++
			for _, word := range forbidden {
				if tag == word || strings.HasPrefix(tag, word+"_") || strings.HasSuffix(tag, "_"+word) || strings.Contains(tag, "_"+word+"_") {
					t.Errorf("%s.%s (%q) is a summary field", kind.Name(), kind.Field(i).Name, tag)
				}
			}
		}
	}
	if checked < 30 {
		t.Fatalf("checked %d fields; the guard would pass vacuously", checked)
	}
}

// FR-075 / US9: every observation says it is one look, typed in by a person.
func TestEveryObservationCarriesTheSingleObservationRule(t *testing.T) {
	record, err := ValidateRankObservation(validObservation(), observationNow)
	if err != nil {
		t.Fatal(err)
	}
	view := ViewRankObservation(record)
	if view.Rule != "rank.single_observation" || view.DataOrigin != "manual_only" {
		t.Fatalf("view = %+v", view)
	}
	encoded, _ := json.Marshal(view)
	for _, want := range []string{`"rule":"rank.single_observation"`, `"data_origin":"manual_only"`, `"scanned_depth":null`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("%s lacks %s", encoded, want)
		}
	}
}
