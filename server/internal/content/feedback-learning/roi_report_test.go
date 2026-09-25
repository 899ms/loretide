package feedbacklearning

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// specs/034 PR 4: report versions, the pure half (T075 to T078; SC-009,
// SC-011; FR-041, FR-048a, FR-057, FR-060 to FR-063). The same functions the
// store calls, with no database: the database half is in internal/handler's
// content_roi_report_test.go.

// reportFixtures is contract §5.4, each sample under its contract name, as the
// input a report version would store.
func reportFixtures(t *testing.T) map[string]ReportInput {
	t.Helper()
	fixtures := map[string]ReportInput{}
	with := func(deal DealRevision, costs ...CostRevision) ReportInput {
		deals, touches, judgements := oneTouchDeal(deal)
		return ReportInput{Params: sampleParams(), Costs: costs, Deals: deals, Touches: touches, Attributions: judgements}
	}
	fixtures["D14-V12/roi"] = with(statedDeal("d-1", "l-1", 1000000, 300000), sampleCost("c-1", 100000))
	fixtures["D14-V12/revenue"] = with(sampleDeal("d-1", "l-1", 1000000), sampleCost("c-1", 100000))
	fixtures["negative"] = with(statedDeal("d-1", "l-1", 90000, 50000), sampleCost("c-1", 100000))
	fixtures["zero-spend"] = with(statedDeal("d-1", "l-1", 1000000, 300000))
	usd := sampleCost("c-usd", 10000)
	usd.Currency = "USD"
	fixtures["unconverted"] = ReportInput{Params: sampleParams(), Costs: []CostRevision{usd}}
	converted := sampleParams()
	converted.Rates = []ExchangeRate{{From: "USD", To: "CNY", Rate: "7.1234", Note: "9 月 30 日中行牌价",
		EnteredBy: "u-1", EnteredAt: "2026-10-01T00:00:00Z"}}
	fixtures["converted"] = ReportInput{Params: converted, Costs: []CostRevision{usd}}
	fixtures["split-10001"] = ReportInput{
		Params: sampleParams(), Deals: []DealRevision{sampleDeal("d-1", "l-1", 10001)},
		Touches: []TouchRevision{
			sampleTouch("t-b", "l-1", "w-b", "", inWindow),
			sampleTouch("t-a", "l-1", "w-a", "", inWindow.Add(time.Hour)),
		},
		Attributions: []AttributionRevision{judged("d-1", JudgementMulti, "t-a", "t-b")},
	}
	amount := int64(10000)
	allocations, err := ResolveAllocations(&amount, "CNY", weightShares("work", "w-3", "work", "w-1", "work", "w-2"))
	if err != nil {
		t.Fatal(err)
	}
	shared := sampleCost("c-1", amount)
	shared.Allocations = allocations
	fixtures["alloc-10000"] = ReportInput{Params: sampleParams(), Costs: []CostRevision{shared}}
	fixtures["one-booking-two-works"] = oneBookingTwoWorks()
	return fixtures
}

// asStored is what a jsonb column gives back: the same document with its
// keys reordered and its spacing changed. CanonicalJSON sorts; the indent is
// there so a comparison that leaned on the original bytes would fail.
func asStored(t *testing.T, document []byte) []byte {
	t.Helper()
	canonical, err := CanonicalJSON(document)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(canonical, &value); err != nil {
		t.Fatal(err)
	}
	indented, err := json.MarshalIndent(value, "", "   ")
	if err != nil {
		t.Fatal(err)
	}
	return indented
}

func canonical(t *testing.T, document []byte) string {
	t.Helper()
	out, err := CanonicalJSON(document)
	if err != nil {
		t.Fatalf("not JSON: %v\n%s", err, document)
	}
	return string(out)
}

// T075 / SC-011 / FR-057: every contract §5.4 sample, stored as a report
// version and read back, recomputes from its stored inputs and calc version
// to exactly the stored result.
func TestStoredReportsRecomputeToTheSameBytes(t *testing.T) {
	fixtures := reportFixtures(t)
	if len(fixtures) != 9 {
		t.Fatalf("%d fixtures, contract §5.4 has 9", len(fixtures))
	}
	for name, input := range fixtures {
		t.Run(name, func(t *testing.T) {
			row, err := buildReportVersion(input)
			if err != nil {
				t.Fatal(err)
			}
			recomputed, err := RecomputeReport(row.calcVersion, asStored(t, row.params), asStored(t, row.inputs))
			if err != nil {
				t.Fatal(err)
			}
			fresh, _ := json.Marshal(recomputed)
			if stored := canonical(t, asStored(t, row.result)); stored != canonical(t, fresh) {
				t.Fatalf("the stored result is not what its inputs recompute to:\nstored     %s\nrecomputed %s", stored, canonical(t, fresh))
			}
			// And the stored result is the calculator's, not something else
			// that merely recomputes consistently.
			direct, _ := json.Marshal(calculate(t, input))
			if canonical(t, row.result) != canonical(t, direct) {
				t.Fatal("the stored result differs from the calculator's result for the same input")
			}
		})
	}
	// The two D14-V12 samples, read from the stored bytes.
	for name, want := range map[string]map[MetricID]string{
		"D14-V12/roi":     {MetricBusinessROI: "200.00%"},
		"D14-V12/revenue": {MetricRevenueToSpend: "10.00 倍"},
		"negative":        {MetricBusinessROI: "−50.00%"},
	} {
		row, err := buildReportVersion(fixtures[name])
		if err != nil {
			t.Fatal(err)
		}
		var stored Result
		if err = json.Unmarshal(asStored(t, row.result), &stored); err != nil {
			t.Fatal(err)
		}
		for metric, display := range want {
			if got := stored.Metrics[metric].Display; got != display {
				t.Errorf("%s: stored %s = %q, want %q", name, metric, got, display)
			}
		}
	}
}

// The stored inputs name the included set - the latest revision of each
// record the calculator used - and carry a full copy of every revision read,
// including the text the calculator does not use: the copy has to be enough
// without the record tables.
func TestStoredInputsCarryEveryRevisionInFull(t *testing.T) {
	input := oneBookingTwoWorks()
	input.Leads[0].CustomerRef, input.Leads[0].Note = "客户-甲", "朋友介绍"
	input.Deals[0].OrderRef = "TB-0412"
	row, err := buildReportVersion(input)
	if err != nil {
		t.Fatal(err)
	}
	var stored ReportInputs
	if err = json.Unmarshal(row.inputs, &stored); err != nil {
		t.Fatal(err)
	}
	want := []ReportRecordKey{
		{"attribution", "d-1", 1}, {"cost", "c-1", 1}, {"deal", "d-1", 1},
		{"lead", "l-1", 2}, {"touch", "t-1", 1}, {"touch", "t-2", 1},
	}
	if !slices.Equal(stored.Records, want) {
		t.Fatalf("records = %v, want %v", stored.Records, want)
	}
	if len(stored.Leads) != 2 || stored.Leads[0].CustomerRef != "客户-甲" || stored.Deals[0].OrderRef != "TB-0412" {
		t.Fatalf("the copy is not complete: %+v", stored)
	}
	if stored.Adjustments == nil || stored.Attributions == nil {
		t.Fatal("empty kinds are stored as null, not []")
	}
}

// T076 / FR-041: the calc version and every metric's formula are in the
// stored result, and a version stored under another calc version is never
// recomputed by this one.
func TestAStoredVersionKeepsItsCalcVersion(t *testing.T) {
	row, err := buildReportVersion(reportFixtures(t)["D14-V12/roi"])
	if err != nil {
		t.Fatal(err)
	}
	var stored Result
	if err = json.Unmarshal(row.result, &stored); err != nil {
		t.Fatal(err)
	}
	if row.calcVersion != CalcVersion || stored.CalcVersion != CalcVersion || CalcVersion != "roi-calc/1" {
		t.Fatalf("calc versions: row %q, result %q, const %q", row.calcVersion, stored.CalcVersion, CalcVersion)
	}
	for _, metric := range MetricIDs {
		if got := stored.Metrics[metric].Formula; got != FormulaID(metric) {
			t.Errorf("%s formula = %q, want %q", metric, got, FormulaID(metric))
		}
	}
	// A version written by roi-calc/2 (or an older one) is not this build's
	// to recompute: it is refused, never quietly recomputed with roi-calc/1.
	for _, other := range []string{"roi-calc/0", "roi-calc/2", ""} {
		if _, err = RecomputeReport(other, row.params, row.inputs); !errors.Is(err, ErrCalcVersionUnavailable) {
			t.Errorf("recompute under %q = %v, want ErrCalcVersionUnavailable", other, err)
		}
	}
}

// FR-048a: the free-text-stage limitation is part of every stored result
// and of its public summary.
func TestEveryStoredReportStatesTheStageLimitation(t *testing.T) {
	const limitation = "阶段是自由文本，没有先后顺序；转化率只按线索是否到达过某阶段计算，不反映阶段之间的先后。"
	for name, input := range reportFixtures(t) {
		row, err := buildReportVersion(input)
		if err != nil {
			t.Fatal(err)
		}
		var stored Result
		if err = json.Unmarshal(row.result, &stored); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(stored.Rules, limitation) {
			t.Errorf("%s: stored rules %v lack the stage limitation", name, stored.Rules)
		}
		var params ReportParams
		_ = json.Unmarshal(row.params, &params)
		if summary := summarize(ReportVersionHeader{ReportID: "r-1", VersionNo: 1}, params, stored); !slices.Contains(summary.Rules, limitation) {
			t.Errorf("%s: the summary lacks the stage limitation", name)
		}
	}
}

// summaryFieldTags walks a type and every type inside it - struct fields,
// slice and map elements - and answers each JSON field name with where it
// was found.
func summaryFieldTags(typ reflect.Type, path string, seen map[reflect.Type]bool, found map[string]string) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] || typ == reflect.TypeOf(time.Time{}) {
		return
	}
	seen[typ] = true
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" && field.Anonymous {
			summaryFieldTags(field.Type, path, seen, found)
			continue
		}
		if tag == "" {
			tag = field.Name
		}
		found[tag] = path + "." + tag
		summaryFieldTags(field.Type, path+"."+tag, seen, found)
	}
}

// T077 / SC-009 / FR-058 / FR-062: the public summary, nested fields
// included, holds only contract §7's allowed fields - and none of the
// customer or evidence text.
func TestReportSummaryHasOnlyTheAllowedFields(t *testing.T) {
	found := map[string]string{}
	summaryFieldTags(reflect.TypeOf(ReportSummary{}), "", map[reflect.Type]bool{}, found)
	if len(found) < 30 {
		t.Fatalf("found only %d fields; the scan would pass vacuously", len(found))
	}
	for _, forbidden := range []string{
		"customer_ref", "order_ref", "note", "evidence_note", "recorded_by", "created_by",
		"entered_by", "stage", "lead_id", "dedupe_key", "not_duplicate_of", "category", "campaign_label",
	} {
		if where, ok := found[forbidden]; ok {
			t.Errorf("ReportSummary has %q at %s", forbidden, where)
		}
	}
	for tag, where := range found {
		if !slices.Contains(ReportSummaryFields, tag) {
			t.Errorf("ReportSummary field %s (%s) is not on contract §7's allow-list", tag, where)
		}
	}
}

// The summary copies the allowed fields only: a rate's note and who entered
// it do not survive, and neither does any record text in the inputs.
func TestReportSummaryDropsRateNotesAndRecordText(t *testing.T) {
	input := reportFixtures(t)["converted"]
	input.Costs[0].Note, input.Costs[0].EvidenceNote = "内部备注-甲", "发票-乙"
	row, err := buildReportVersion(input)
	if err != nil {
		t.Fatal(err)
	}
	var params ReportParams
	var result Result
	if json.Unmarshal(row.params, &params) != nil || json.Unmarshal(row.result, &result) != nil {
		t.Fatal("stored columns do not decode")
	}
	summary := summarize(ReportVersionHeader{ReportID: "r-1", VersionNo: 1, CreatedBy: "u-author"}, params, result)
	encoded, _ := json.Marshal(summary)
	for _, secret := range []string{"中行牌价", "u-1", "u-author", "内部备注-甲", "发票-乙"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("the summary carries %q: %s", secret, encoded)
		}
	}
	if len(summary.Params.Rates) != 1 || summary.Params.Rates[0].Rate != "7.1234" ||
		summary.Metrics[MetricSpendTotal].Display != "712.34" {
		t.Fatalf("the summary lost an allowed field: %s", encoded)
	}
}

// T078 / FR-060 / FR-061: the AI explanation of a report is pending_data,
// always, and nothing in this module produces anything else for it.
func TestAReportsAIExplanationIsPendingData(t *testing.T) {
	if got := ReportAIExplanation(); got != StatePendingData {
		t.Fatalf("AI explanation state = %q, want pending_data", got)
	}
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "roi_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"StateGenerated", "StateQueued", "StateGenerating", "ReviewStateFor("} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("roi_report.go uses %s", forbidden)
		}
	}
}

// T078 / FR-061: no migration builds a table for an AI explanation,
// suggestion or adoption, and every content_roi_ table a migration creates is
// one the append-only guard knows about.
func TestNoMigrationBuildsAnExplanationTable(t *testing.T) {
	migrations := filepath.Join(moduleDir(t), "..", "..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(migrations, "*.up.sql"))
	if err != nil || len(files) < 100 {
		t.Fatalf("read %d migrations (%v); the scan would pass vacuously", len(files), err)
	}
	create := regexp.MustCompile(`(?i)CREATE TABLE(?: IF NOT EXISTS)?\s+([a-z_]+)`)
	roiCreated := 0
	for _, file := range files {
		body, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, match := range create.FindAllStringSubmatch(string(body), -1) {
			table := strings.ToLower(match[1])
			if !strings.HasPrefix(table, "content_roi_") && !strings.HasPrefix(table, "content_feedback") {
				continue
			}
			for _, word := range []string{"explan", "suggest", "adopt", "insight", "_ai"} {
				if strings.Contains(table, word) {
					t.Errorf("%s creates %s, an AI explanation table", filepath.Base(file), table)
				}
			}
			if strings.HasPrefix(table, "content_roi_") {
				roiCreated++
				if !slices.Contains(roiTables, table) {
					t.Errorf("%s creates %s, which the append-only guard does not cover", filepath.Base(file), table)
				}
			}
		}
	}
	if roiCreated < len(roiTables) {
		t.Fatalf("found %d content_roi_ tables, the guard lists %d", roiCreated, len(roiTables))
	}
}

// T078 / FR-063: the ROI code writes to its own tables and nowhere else - no
// topic, no budget todo, no business memory, no adoption of anything.
func TestROICodeWritesOnlyItsOwnTables(t *testing.T) {
	insert := regexp.MustCompile(`(?i)INSERT\s+INTO\s+([a-z_]+)`)
	count := 0
	for name, body := range roiSourceFiles(t) {
		for _, match := range insert.FindAllStringSubmatch(body, -1) {
			count++
			if !strings.HasPrefix(strings.ToLower(match[1]), "content_roi_") {
				t.Errorf("%s inserts into %s", name, match[1])
			}
		}
		for _, forbidden := range []string{"content/topic", "topic-card", "content_topic", "todo", "memory"} {
			if strings.Contains(strings.ToLower(stripGoComments(body)), forbidden) {
				t.Errorf("%s mentions %q outside a comment", name, forbidden)
			}
		}
	}
	if count < len(roiTables) {
		t.Fatalf("found %d inserts; the guard would pass vacuously", count)
	}
}

// FR-055: what changed since a version, derived from two key lists.
func TestInputsChangedIsDerivedFromTheKeys(t *testing.T) {
	stored := []ReportRecordKey{{"cost", "c-1", 1}, {"cost", "c-2", 1}, {"lead", "l-1", 1}, {"lead", "l-1", 2}}
	if got := diffReportInputs(stored, stored); len(got) != 0 {
		t.Fatalf("the same keys differ: %+v", got)
	}
	current := []ReportRecordKey{{"cost", "c-1", 2}, {"deal", "d-9", 1}, {"lead", "l-1", 1}, {"lead", "l-1", 2}}
	got := diffReportInputs(stored, current)
	describe := func(change ChangedInput) string {
		text := change.Kind + ":" + change.ID
		for _, revision := range []*int{change.ReportRevision, change.CurrentRevision} {
			if revision == nil {
				text += " -"
			} else {
				text += " " + string(rune('0'+*revision))
			}
		}
		return text
	}
	descriptions := []string{}
	for _, change := range got {
		descriptions = append(descriptions, describe(change))
	}
	want := []string{"cost:c-1 1 2", "cost:c-2 1 -", "deal:d-9 - 1"}
	if !slices.Equal(descriptions, want) {
		t.Fatalf("changes = %v, want %v", descriptions, want)
	}
}

func TestVersionNumbersInThePath(t *testing.T) {
	for text, want := range map[string]int{"1": 1, "12": 12} {
		if got, ok := ParseVersionNo(text); !ok || got != want {
			t.Errorf("ParseVersionNo(%q) = %d, %v", text, got, ok)
		}
	}
	for _, text := range []string{"", "0", "-1", "+1", "abc", "1.0", "1e2", " 1", "99999999999999999999"} {
		if _, ok := ParseVersionNo(text); ok {
			t.Errorf("ParseVersionNo(%q) accepted", text)
		}
	}
}

// T073 / FR-054, structurally: no read path computes. A version is read as
// it was stored; reading one never runs the calculator over today's records
// and never rebuilds a stored column. Scans the read functions' bodies for
// any call that computes a result.
func TestReadingAVersionComputesNothing(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleDir(t), "roi_report.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]bool{
		"GetReportVersion": false, "readReportVersion": false, "ListReports": false,
		"ListReportVersions": false, "ReportSummary": false, "summarize": false,
	}
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if _, reader := readers[function.Name.Name]; !reader {
			continue
		}
		readers[function.Name.Name] = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if ident, isIdent := node.(*ast.Ident); isIdent {
				switch ident.Name {
				case "CalculateROI", "buildReportVersion", "RecomputeReport", "calculators", "Marshal":
					t.Errorf("%s uses %s; a stored version is read, not computed", function.Name.Name, ident.Name)
				}
			}
			return true
		})
	}
	for name, found := range readers {
		if !found {
			t.Errorf("read function %s is gone; this guard would pass vacuously", name)
		}
	}
}

// changesOf describes what reading a version generated from before would
// flag, if the records were now after: "kind:id report current".
func changesOf(t *testing.T, before, after ReportInput) []string {
	t.Helper()
	stored, err := includedRecords(before)
	if err != nil {
		t.Fatal(err)
	}
	current, err := includedRecords(after)
	if err != nil {
		t.Fatal(err)
	}
	described := []string{}
	for _, change := range diffReportInputs(stored, current) {
		text := change.Kind + ":" + change.ID
		for _, revision := range []*int{change.ReportRevision, change.CurrentRevision} {
			if revision == nil {
				text += " -"
			} else {
				text += " " + strconv.Itoa(*revision)
			}
		}
		described = append(described, text)
	}
	return described
}

// Controller ruling on PR #266 (FR-055): "inputs have been updated" is raised
// only by a record that the version's calculation used, or one that has
// entered its window or scope. A revision of anything else raises nothing.
func TestInputsChangedOnlyForRecordsTheCalculationUses(t *testing.T) {
	outOfWindow := time.Date(2026, 8, 5, 2, 0, 0, 0, time.UTC)
	base := func() ReportInput {
		input := oneBookingTwoWorks()
		input.Leads = append(input.Leads, LeadRevision{LeadID: "l-august", Revision: 1, Stage: "咨询", FirstSeenAt: outOfWindow})
		august := sampleDeal("d-august", "l-august", 50000)
		august.ClosedAt = outOfWindow
		input.Deals = append(input.Deals, august)
		cost := sampleCost("c-august", 7000)
		cost.IncurredAt = outOfWindow
		input.Costs = append(input.Costs, cost)
		return input
	}
	for _, tc := range []struct {
		name   string
		change func(input *ReportInput)
		want   []string
	}{
		{"nothing changed", func(*ReportInput) {}, []string{}},
		{"an out-of-window lead revised", func(input *ReportInput) {
			input.Leads = append(input.Leads, LeadRevision{LeadID: "l-august", Revision: 2, Stage: "预约", Qualified: true,
				FirstSeenAt: outOfWindow})
		}, []string{}},
		{"an out-of-window deal and cost revised", func(input *ReportInput) {
			input.Deals[1].Revision, input.Deals[1].AmountMinor = 2, 60000
			input.Costs[1].Revision = 2
		}, []string{}},
		{"an included deal revised", func(input *ReportInput) {
			input.Deals[0].Revision, input.Deals[0].AmountMinor = 2, 900000
		}, []string{"deal:d-1 1 2"}},
		{"an included lead revised", func(input *ReportInput) {
			input.Leads = append(input.Leads, LeadRevision{LeadID: "l-1", Revision: 3, Stage: "成交", Qualified: true,
				FirstSeenAt: input.Leads[0].FirstSeenAt})
		}, []string{"lead:l-1 2 3"}},
		{"an accepted touch revised", func(input *ReportInput) {
			input.Touches[0].Revision = 2
		}, []string{"touch:t-1 1 2"}},
		{"a refund recorded on an included deal", func(input *ReportInput) {
			input.Adjustments = append(input.Adjustments, AdjustmentRevision{
				AdjustmentID: "r-1", Revision: 1, DealID: "d-1", Kind: AdjustmentRefund,
				RevenueDeltaMinor: 100, Currency: "CNY", OccurredAt: inWindow,
			})
		}, []string{"adjustment:r-1 - 1"}},
		{"a refund dated after the generation time", func(input *ReportInput) {
			input.Adjustments = append(input.Adjustments, AdjustmentRevision{
				AdjustmentID: "r-late", Revision: 1, DealID: "d-1", Kind: AdjustmentRefund,
				RevenueDeltaMinor: 100, Currency: "CNY", OccurredAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			})
		}, []string{}},
		{"a deal moved into the window", func(input *ReportInput) {
			input.Deals[1].Revision, input.Deals[1].ClosedAt = 2, inWindow
		}, []string{"deal:d-august - 2"}},
		{"a cost moved out of the window", func(input *ReportInput) {
			input.Costs[0].Revision, input.Costs[0].IncurredAt = 2, outOfWindow
		}, []string{"cost:c-1 1 -"}},
		{"an included cost voided", func(input *ReportInput) {
			input.Costs[0].Revision, input.Costs[0].Voided = 2, true
		}, []string{"cost:c-1 1 -"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := base()
			tc.change(&after)
			if got := changesOf(t, base(), after); !slices.Equal(got, tc.want) {
				t.Fatalf("changes = %v, want %v", got, tc.want)
			}
		})
	}
}

// The same rule for scope: a cost outside the report's scope is not used,
// so revising it raises nothing; one revised into the scope does.
func TestInputsChangedFollowsTheScope(t *testing.T) {
	scoped := func(label string) ReportInput {
		params := sampleParams()
		params.Scope.CampaignLabels = []string{"国庆"}
		inScope, outOfScope := sampleCost("c-in", 10000), sampleCost("c-out", 20000)
		inScope.CampaignLabel, outOfScope.CampaignLabel = "国庆", label
		return ReportInput{Params: params, Costs: []CostRevision{inScope, outOfScope}}
	}
	revised := scoped("中秋")
	revised.Costs[1].Revision, revised.Costs[1].Note = 2, "改了备注"
	if got := changesOf(t, scoped("中秋"), revised); len(got) != 0 {
		t.Fatalf("an out-of-scope cost raised %v", got)
	}
	entered := scoped("国庆")
	entered.Costs[1].Revision = 2
	if got := changesOf(t, scoped("中秋"), entered); !slices.Equal(got, []string{"cost:c-out - 2"}) {
		t.Fatalf("a cost revised into the scope raised %v", got)
	}
}
