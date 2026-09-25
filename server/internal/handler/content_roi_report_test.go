package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/034 PR 4 against the real schema: report versions (T073 to T076,
// T078, T080; SC-009 to SC-011; FR-041, FR-053 to FR-058, FR-061). These run
// in the handler database suite (scripts/test-go-db.sh --suite handler).
//
// Contract: specs/034-roi-review/contracts/roi-review.md §1.10, §6, §7

// roiReportParams is contract §5.1's params for September 2026, with extra
// JSON members appended to the object.
func roiReportParams(extra string) string {
	return `{"window":{"start":"2026-09-01","end":"2026-09-30"},"report_currency":"CNY",
		"rates":[],"attribution_method":"even_split","conversion":{"from_stage":"咨询","to_stage":"预约"},
		"booking_stage":"预约","scope":{"account_ids":[],"work_ids":[],"campaign_labels":[]}` + extra + `}`
}

func roiReportRequest(title, params string) string {
	return fmt.Sprintf(`{"title":%q,"params":%s}`, title, params)
}

// roiStoredVersion is one stored row, the three JSON columns as the
// database gives them back.
type roiStoredVersion struct {
	calcVersion, params, inputs, result, row string
}

func roiReportRow(t *testing.T, reportID string, versionNo int) roiStoredVersion {
	t.Helper()
	var stored roiStoredVersion
	if err := testPool.QueryRow(t.Context(), `SELECT calc_version, params::text, inputs::text, result::text,
		row_to_json(r)::text FROM content_roi_report_version r WHERE report_id=$1 AND version_no=$2`,
		reportID, versionNo).Scan(&stored.calcVersion, &stored.params, &stored.inputs, &stored.result, &stored.row); err != nil {
		t.Fatalf("read report %s v%d: %v", reportID, versionNo, err)
	}
	return stored
}

func roiCanonical(t *testing.T, document []byte) string {
	t.Helper()
	out, err := feedbacklearning.CanonicalJSON(document)
	if err != nil {
		t.Fatalf("not JSON: %v\n%s", err, document)
	}
	return string(out)
}

// roiJudgedDeal records a deal on a new lead with one platform-linked touch
// and a "confirmed" judgement on it; it answers the deal id.
func roiJudgedDeal(t *testing.T, h *Handler, wsID, accountID, publicationID, customerRef, deal string) string {
	t.Helper()
	leadID := roiLead(t, h, wsID, customerRef)
	touchID, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"))
	dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		withLead(deal, leadID)), "deal_id")
	roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("confirmed", fmt.Sprintf(`[%q]`, touchID), `[]`, 0), "dealId", dealID), "deal_id")
	return dealID
}

// T073 / T074 / SC-010 / FR-054 to FR-056: version 1 is never touched again.
// Revising a cost it read leaves every stored byte of it as it was, and
// reading it says - derived, not stored - that the inputs have moved on and
// which record did. Version 2 is a new row with version 1's parameters, and
// both stay.
func TestContentROIReportVersionsAreNeverRewritten(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	wsID, accountID, publicationID := roiWorkspace(t, "roi-report")
	h := feedbackHandler(t)
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("投放", "1000.00", "")), "cost_id")
	roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-报告甲",
		dealJSON("TB-report-0412", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`))

	params := roiReportParams("")
	params = strings.Replace(params, `"rates":[]`,
		`"rates":[{"from":"USD","to":"CNY","rate":"7.1234","note":"九月牌价","entered_by":"someone-else"}]`, 1)
	created := roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports", roiReportRequest("九月复盘", params))
	reportID, first := roiCreated(t, created, "report_id")
	if first["version_no"] != float64(1) || first["inputs_changed"] != false || first["ai_explanation"] != "pending_data" {
		t.Fatalf("version 1 = %v", first)
	}
	var firstResult feedbacklearning.Result
	firstRaw, _ := json.Marshal(first["result"])
	if err := json.Unmarshal(firstRaw, &firstResult); err != nil {
		t.Fatal(err)
	}
	if firstResult.Metrics[feedbacklearning.MetricBusinessROI].Display != "200.00%" {
		t.Fatalf("version 1 business_roi = %+v", firstResult.Metrics[feedbacklearning.MetricBusinessROI])
	}
	v1 := roiReportRow(t, reportID, 1)
	var storedParams feedbacklearning.ReportParams
	if err := json.Unmarshal([]byte(v1.params), &storedParams); err != nil {
		t.Fatal(err)
	}
	if storedParams.Rates[0].EnteredBy != testUserID || storedParams.Window.Timezone != "Asia/Shanghai" || storedParams.GeneratedAt == "" {
		t.Fatalf("the server-written params were not the server's: %+v", storedParams)
	}

	// A new revision of the cost version 1 read.
	roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("投放", "2000.00", `,"base_revision":1`), "costId", costID), "cost_id")
	if got := roiReportRow(t, reportID, 1); got != v1 {
		t.Fatalf("version 1 changed after its cost was revised:\n%s\n%s", v1.row, got.row)
	}
	var read struct {
		VersionNo     int              `json:"version_no"`
		InputsChanged bool             `json:"inputs_changed"`
		ChangedInputs []map[string]any `json:"changed_inputs"`
		AIExplanation string           `json:"ai_explanation"`
		Result        json.RawMessage  `json:"result"`
		Params        json.RawMessage  `json:"params"`
	}
	roiCall(t, h.GetContentROIReportVersion, wsID, "GET", "/", "", "reportId", reportID, "versionNo", "1").
		Want(http.StatusOK).JSON(&read)
	if !read.InputsChanged || len(read.ChangedInputs) != 1 || read.ChangedInputs[0]["kind"] != "cost" ||
		read.ChangedInputs[0]["id"] != costID || read.ChangedInputs[0]["report_revision"] != float64(1) ||
		read.ChangedInputs[0]["current_revision"] != float64(2) {
		t.Fatalf("version 1 read after the revision: inputs_changed %v, changes %v", read.InputsChanged, read.ChangedInputs)
	}
	if read.AIExplanation != "pending_data" {
		t.Fatalf("ai_explanation = %q", read.AIExplanation)
	}
	if roiCanonical(t, read.Result) != roiCanonical(t, []byte(v1.result)) || roiCanonical(t, read.Result) != roiCanonical(t, firstRaw) {
		t.Fatal("reading version 1 answered a different result from the one stored")
	}
	// "Inputs have been updated" is derived: the table has no column for it.
	var flagColumns int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'content_roi_report_version' AND (column_name LIKE '%changed%' OR column_name LIKE '%stale%')`).
		Scan(&flagColumns); err != nil {
		t.Fatal(err)
	}
	if flagColumns != 0 {
		t.Fatalf("the report table stores %d changed/stale column(s)", flagColumns)
	}

	// Version 2, from the records as they are now, with version 1's params.
	second := roiCall(t, h.GenerateContentROIReportVersion, wsID, "POST", "/versions", `{}`, "reportId", reportID)
	_, v2Body := roiCreated(t, second, "report_id")
	if v2Body["version_no"] != float64(2) || v2Body["title"] != "九月复盘" || v2Body["report_id"] != reportID {
		t.Fatalf("version 2 = %v", v2Body)
	}
	v2 := roiReportRow(t, reportID, 2)
	var secondParams feedbacklearning.ReportParams
	if err := json.Unmarshal([]byte(v2.params), &secondParams); err != nil {
		t.Fatal(err)
	}
	if secondParams.Window != storedParams.Window || secondParams.AttributionMethod != storedParams.AttributionMethod ||
		secondParams.Rates[0] != storedParams.Rates[0] || secondParams.Conversion != storedParams.Conversion {
		t.Fatalf("version 2 did not reuse version 1's params:\n%+v\n%+v", storedParams, secondParams)
	}
	var secondResult feedbacklearning.Result
	if err := json.Unmarshal([]byte(v2.result), &secondResult); err != nil {
		t.Fatal(err)
	}
	// (3000.00 - 2000.00) / 2000.00.
	if got := secondResult.Metrics[feedbacklearning.MetricBusinessROI].Display; got != "50.00%" {
		t.Fatalf("version 2 business_roi = %q, want 50.00%%", got)
	}
	if got := roiReportRow(t, reportID, 1); got != v1 {
		t.Fatal("generating version 2 changed version 1")
	}
	roiCall(t, h.GetContentROIReportVersion, wsID, "GET", "/", "", "reportId", reportID, "versionNo", "2").
		Want(http.StatusOK).JSON(&read)
	if read.InputsChanged || len(read.ChangedInputs) != 0 {
		t.Fatalf("version 2 is already out of date: %v", read.ChangedInputs)
	}

	// Both versions listed; the report list answers the latest.
	var versions struct {
		ReportID string           `json:"report_id"`
		Versions []map[string]any `json:"versions"`
	}
	roiCall(t, h.ListContentROIReportVersions, wsID, "GET", "/", "", "reportId", reportID).Want(http.StatusOK).JSON(&versions)
	if versions.ReportID != reportID || len(versions.Versions) != 2 || versions.Versions[0]["version_no"] != float64(1) ||
		versions.Versions[1]["version_no"] != float64(2) {
		t.Fatalf("versions = %+v", versions)
	}
	var reports struct {
		Reports []map[string]any `json:"reports"`
	}
	roiCall(t, h.ListContentROIReports, wsID, "GET", "/", "").Want(http.StatusOK).JSON(&reports)
	if len(reports.Reports) != 1 || reports.Reports[0]["version_no"] != float64(2) {
		t.Fatalf("reports = %+v", reports.Reports)
	}
	if auditSteps(t, wsID, reportID, "generate-report") != 1 || auditSteps(t, wsID, reportID, "generate-report-version") != 1 {
		t.Fatal("each generation is not one audit event")
	}

	// FR-058 / SC-009: the public summary of version 1 carries the figures
	// and none of the customer, order or evidence text.
	summary, err := h.feedbackStore().ReportSummary(ctx, wsID, reportID, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(summary)
	if summary.Metrics[feedbacklearning.MetricBusinessROI].Display != "200.00%" || summary.VersionNo != 1 {
		t.Fatalf("summary = %s", encoded)
	}
	for _, secret := range []string{"客户-报告甲", "TB-report-0412", "发票 0412", "九月牌价", testUserID} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("the summary carries %q", secret)
		}
	}
	if _, err = h.feedbackStore().ReportSummary(ctx, wsID, reportID, 3); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("summary of a missing version = %v", err)
	}
}

// roiReadChanges reads one version and answers its derived flag and change
// list as "kind:id report current".
func roiReadChanges(t *testing.T, h *Handler, wsID, reportID, versionNo string) (bool, []string) {
	t.Helper()
	var read struct {
		InputsChanged bool `json:"inputs_changed"`
		ChangedInputs []struct {
			Kind            string `json:"kind"`
			ID              string `json:"id"`
			ReportRevision  *int   `json:"report_revision"`
			CurrentRevision *int   `json:"current_revision"`
		} `json:"changed_inputs"`
	}
	roiCall(t, h.GetContentROIReportVersion, wsID, "GET", "/", "", "reportId", reportID, "versionNo", versionNo).
		Want(http.StatusOK).JSON(&read)
	described := []string{}
	for _, change := range read.ChangedInputs {
		text := change.Kind + ":" + change.ID
		for _, revision := range []*int{change.ReportRevision, change.CurrentRevision} {
			if revision == nil {
				text += " -"
			} else {
				text += fmt.Sprintf(" %d", *revision)
			}
		}
		described = append(described, text)
	}
	return read.InputsChanged, described
}

// Controller ruling on PR #266 (FR-055), against the real schema: revising a
// lead first seen outside the window raises no "inputs have been updated";
// revising a deal the version used does, and names it.
func TestContentROIInputsChangedIgnoresRecordsTheVersionDidNotUse(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, accountID, publicationID := roiWorkspace(t, "roi-report-included")
	h := feedbackHandler(t)
	roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("投放", "1000.00", "")), "cost_id")
	dealID := roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-included",
		dealJSON("TB-included", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`))
	augustLead, _ := roiCreated(t, roiCall(t, h.CreateContentROILead, wsID, "POST", "/api/content-roi/leads",
		`{"customer_ref":"客户-八月","stage":"咨询","qualified":false,"first_seen_at":"2026-08-05T02:00:00Z","note":""}`), "lead_id")
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest("九月", roiReportParams(""))), "report_id")
	if changed, changes := roiReadChanges(t, h, wsID, reportID, "1"); changed || len(changes) != 0 {
		t.Fatalf("a fresh version reads as changed: %v", changes)
	}

	// Out of the window: revised, and nothing is flagged.
	roiCreated(t, roiCall(t, h.ReviseContentROILead, wsID, "POST", "/revisions",
		`{"customer_ref":"客户-八月","stage":"预约","qualified":true,"first_seen_at":"2026-08-05T02:00:00Z","note":"","base_revision":1}`,
		"leadId", augustLead), "lead_id")
	if changed, changes := roiReadChanges(t, h, wsID, reportID, "1"); changed || len(changes) != 0 {
		t.Fatalf("an out-of-window lead revision flagged the version: %v", changes)
	}

	// Used by the version: revised (same lead, new amount), and flagged by
	// name.
	var detail struct {
		Current struct {
			LeadID string `json:"lead_id"`
		} `json:"current"`
	}
	roiCall(t, h.GetContentROIDeal, wsID, "GET", "/", "", "dealId", dealID).Want(http.StatusOK).JSON(&detail)
	roiCreated(t, roiCall(t, h.ReviseContentROIDeal, wsID, "POST", "/revisions",
		withLead(dealJSON("TB-included", "12000.00", "stated_gross_profit", `,"gross_profit":"3000.00","base_revision":1`),
			detail.Current.LeadID), "dealId", dealID), "deal_id")
	changed, changes := roiReadChanges(t, h, wsID, reportID, "1")
	if !changed || !slices.Equal(changes, []string{"deal:" + dealID + " 1 2"}) {
		t.Fatalf("an included deal revision: inputs_changed %v, changes %v", changed, changes)
	}
}

// T075 / SC-011 / FR-057: each contract §5.4 sample, recorded through the
// endpoints and generated as a report, is read back from the database and
// recomputed from its stored inputs and calc version: the same bytes as the
// stored result. Works are replaced by accounts and campaign labels where a
// sample splits across them, because a work needs work-editor's own rows;
// what is under test is the round trip, and the shape of each sample is
// kept.
func TestContentROIStoredReportsRecomputeFromTheDatabase(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	usdCost := strings.Replace(costJSON("投放-美元", "100.00", ""), `"currency":"CNY"`, `"currency":"USD"`, 1)
	samples := []struct {
		name    string
		record  func(t *testing.T, wsID, accountID, publicationID string)
		params  string
		metric  feedbacklearning.MetricID
		display string
	}{
		{"D14-V12/roi", func(t *testing.T, wsID, accountID, publicationID string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", costJSON("投放", "1000.00", "")), "cost_id")
			roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-roi", dealJSON("TB-roi", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`))
		}, roiReportParams(""), feedbacklearning.MetricBusinessROI, "200.00%"},
		{"D14-V12/revenue", func(t *testing.T, wsID, accountID, publicationID string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", costJSON("投放", "1000.00", "")), "cost_id")
			roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-revenue", dealJSON("TB-revenue", "10000.00", "none", ""))
		}, roiReportParams(""), feedbacklearning.MetricRevenueToSpend, "10.00 倍"},
		{"negative", func(t *testing.T, wsID, accountID, publicationID string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", costJSON("投放", "1000.00", "")), "cost_id")
			roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-negative", dealJSON("TB-negative", "900.00", "stated_gross_profit", `,"gross_profit":"500.00"`))
		}, roiReportParams(""), feedbacklearning.MetricBusinessROI, "−50.00%"},
		{"zero-spend", func(t *testing.T, wsID, accountID, publicationID string) {
			roiJudgedDeal(t, h, wsID, accountID, publicationID, "客户-zero", dealJSON("TB-zero", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`))
		}, roiReportParams(""), feedbacklearning.MetricBusinessROI, ""},
		{"unconverted", func(t *testing.T, wsID, _, _ string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", usdCost), "cost_id")
		}, roiReportParams(""), feedbacklearning.MetricSpendTotal, ""},
		{"converted", func(t *testing.T, wsID, _, _ string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", usdCost), "cost_id")
		}, strings.Replace(roiReportParams(""), `"rates":[]`, `"rates":[{"from":"USD","to":"CNY","rate":"7.1234","note":""}]`, 1),
			feedbacklearning.MetricSpendTotal, "712.34"},
		{"split-10001", func(t *testing.T, wsID, accountID, publicationID string) {
			leadID := roiLead(t, h, wsID, "客户-split")
			first, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"))
			second, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "pre_booking"))
			dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
				withLead(dealJSON("TB-split", "100.01", "none", ""), leadID)), "deal_id")
			roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
				attributionBody("multi_touch", fmt.Sprintf(`[%q,%q]`, first, second), `[]`, 0), "dealId", dealID), "deal_id")
		}, roiReportParams(""), feedbacklearning.MetricAttributedNetRevenue, "100.01"},
		{"alloc-10000", func(t *testing.T, wsID, _, _ string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
				costJSON("拍摄", "100.00", `,"allocations":[
					{"target_kind":"campaign_label","target_id":"国庆-3","weight":1},
					{"target_kind":"campaign_label","target_id":"国庆-1","weight":1},
					{"target_kind":"campaign_label","target_id":"国庆-2","weight":1}]`)), "cost_id")
		}, strings.Replace(roiReportParams(""), `"campaign_labels":[]`, `"campaign_labels":["国庆-1"]`, 1),
			feedbacklearning.MetricSpendTotal, "33.34"},
		{"one-booking-two-works", func(t *testing.T, wsID, accountID, publicationID string) {
			roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", costJSON("投放", "1000.00", "")), "cost_id")
			leadID := roiLead(t, h, wsID, "客户-booking")
			roiCreated(t, roiCall(t, h.ReviseContentROILead, wsID, "POST", "/revisions",
				`{"customer_ref":"客户-booking","stage":"预约","qualified":true,"first_seen_at":"2026-09-10T02:00:00Z","note":"","base_revision":1}`,
				"leadId", leadID), "lead_id")
			first, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"))
			second, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "pre_booking"))
			dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
				withLead(dealJSON("TB-booking", "10000.00", "none", ""), leadID)), "deal_id")
			roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
				attributionBody("multi_touch", fmt.Sprintf(`[%q,%q]`, first, second), `[]`, 0), "dealId", dealID), "deal_id")
		}, roiReportParams(""), feedbacklearning.MetricBookings, "1"},
	}
	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			wsID, accountID, publicationID := roiWorkspace(t, "roi-recompute")
			sample.record(t, wsID, accountID, publicationID)
			reportID, body := roiCreated(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
				roiReportRequest(sample.name, sample.params)), "report_id")
			stored := roiReportRow(t, reportID, 1)
			recomputed, err := feedbacklearning.RecomputeReport(stored.calcVersion, []byte(stored.params), []byte(stored.inputs))
			if err != nil {
				t.Fatal(err)
			}
			fresh, _ := json.Marshal(recomputed)
			if roiCanonical(t, fresh) != roiCanonical(t, []byte(stored.result)) {
				t.Fatalf("recomputed from the stored inputs:\n%s\nstored:\n%s", roiCanonical(t, fresh), roiCanonical(t, []byte(stored.result)))
			}
			responseResult, _ := json.Marshal(body["result"])
			if roiCanonical(t, responseResult) != roiCanonical(t, []byte(stored.result)) {
				t.Fatal("the response result is not the stored one")
			}
			metric := recomputed.Metrics[sample.metric]
			if sample.display == "" {
				if metric.Status != feedbacklearning.StatusNotComputable {
					t.Fatalf("%s = %+v, want not computable", sample.metric, metric)
				}
			} else if metric.Display != sample.display {
				t.Fatalf("%s = %+v, want %q", sample.metric, metric, sample.display)
			}
		})
	}
}

// T076 / FR-041: a version stored under another calc version is read exactly
// as stored - its calc version and its result - and is not recomputed by
// this build; the next version is this build's, and the old one stays.
func TestContentROIOldVersionsAreReadAsStored(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-report-old")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest("基线", roiReportParams(""))), "report_id")
	params := roiReportRow(t, reportID, 1).params
	oldID := "report-old-" + reportID
	oldResult := `{"calc_version":"roi-calc/0","metrics":{"business_roi":{"status":"ok","value":"1","display":"99.99%","formula":"business_roi/0","records":[]}},"rules":[]}`
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_roi_report_version
		(workspace_id, report_id, version_no, title, params, inputs, calc_version, result, created_by)
		VALUES ($1, $2, 1, '旧版', $3::jsonb, '{"records":[]}'::jsonb, 'roi-calc/0', $4::jsonb, $5)`,
		wsID, oldID, params, oldResult, testUserID); err != nil {
		t.Fatal(err)
	}
	var read struct {
		CalcVersion string          `json:"calc_version"`
		Result      json.RawMessage `json:"result"`
	}
	roiCall(t, h.GetContentROIReportVersion, wsID, "GET", "/", "", "reportId", oldID, "versionNo", "1").
		Want(http.StatusOK).JSON(&read)
	if read.CalcVersion != "roi-calc/0" || roiCanonical(t, read.Result) != roiCanonical(t, []byte(oldResult)) {
		t.Fatalf("the old version was not read as stored: %s %s", read.CalcVersion, read.Result)
	}
	stored := roiReportRow(t, oldID, 1)
	if _, err := feedbacklearning.RecomputeReport(stored.calcVersion, []byte(stored.params), []byte(stored.inputs)); !errors.Is(err, feedbacklearning.ErrCalcVersionUnavailable) {
		t.Fatalf("recomputing a roi-calc/0 version = %v", err)
	}
	_, next := roiCreated(t, roiCall(t, h.GenerateContentROIReportVersion, wsID, "POST", "/versions", `{}`, "reportId", oldID), "report_id")
	if next["version_no"] != float64(2) || next["calc_version"] != feedbacklearning.CalcVersion {
		t.Fatalf("the next version = %v", next)
	}
	if got := roiReportRow(t, oldID, 1); got != stored {
		t.Fatal("generating the next version changed the old one")
	}
}

// SC-015 for the PR 4 write: after a workspace deletion has committed,
// generating a report or a version is refused by the fence and writes no row.
func TestContentROIReportGenerationIsFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	wsID, _, _ := roiWorkspace(t, "roi-report-fence")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest("fence", roiReportParams(""))), "report_id")
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, wsID); err != nil {
		t.Fatal(err)
	}
	var params feedbacklearning.ReportParams
	if err := json.Unmarshal([]byte(roiReportParams("")), &params); err != nil {
		t.Fatal(err)
	}
	store := h.roiStore()
	if _, err := store.CreateReport(ctx, wsID, testUserID, feedbacklearning.ReportRequest{Params: &params}, time.Now()); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a new report after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.GenerateReportVersion(ctx, wsID, testUserID, reportID, feedbacklearning.ReportRequest{}, time.Now()); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a new version after the delete committed = %v, want ErrNotFound", err)
	}
	var rows int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM content_roi_report_version WHERE workspace_id=$1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("%d report rows after fenced writes, want the 1 from before", rows)
	}
}

// Contract §6 steps 1 to 5 on the report endpoints: a report or version that
// is not here - another brand's, a version number that is not one, a report
// that does not exist - answers exactly like a missing record; bad params are
// refused by name.
func TestContentROIReportPathsAnswerLikeMissingRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-report-paths")
	other, _, _ := roiWorkspace(t, "roi-report-paths-other")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest("paths", roiReportParams(""))), "report_id")
	foreignID, _ := roiCreated(t, roiCall(t, h.CreateContentROIReport, other, "POST", "/api/content-roi/reports",
		roiReportRequest("foreign", roiReportParams(""))), "report_id")
	reference := roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", "no-such-cost")
	reference.Want(http.StatusNotFound)

	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		method   string
		body     string
		reportID string
		version  string
	}{
		{"version abc", h.GetContentROIReportVersion, "GET", "", reportID, "abc"},
		{"version 0", h.GetContentROIReportVersion, "GET", "", reportID, "0"},
		{"version -1", h.GetContentROIReportVersion, "GET", "", reportID, "-1"},
		{"version 1.0", h.GetContentROIReportVersion, "GET", "", reportID, "1.0"},
		{"version 99", h.GetContentROIReportVersion, "GET", "", reportID, "99"},
		{"missing report", h.GetContentROIReportVersion, "GET", "", "no-such-report", "1"},
		{"foreign report", h.GetContentROIReportVersion, "GET", "", foreignID, "1"},
		{"foreign versions", h.ListContentROIReportVersions, "GET", "", foreignID, ""},
		{"missing versions", h.ListContentROIReportVersions, "GET", "", "no-such-report", ""},
		{"new version of a foreign report", h.GenerateContentROIReportVersion, "POST", "{}", foreignID, ""},
		{"new version of a missing report", h.GenerateContentROIReportVersion, "POST", "{}", "no-such-report", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := []string{"reportId", tc.reportID}
			if tc.version != "" {
				params = append(params, "versionNo", tc.version)
			}
			response := roiCall(t, tc.handler, wsID, tc.method, "/", tc.body, params...)
			response.Want(http.StatusNotFound)
			if !sameRefusalApartFromTrace(t, response, reference) {
				t.Errorf("%s answered differently from a missing record: %s", tc.name, response.Text())
			}
		})
	}
	var foreignRows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_roi_report_version WHERE report_id=$1`, foreignID).Scan(&foreignRows); err != nil {
		t.Fatal(err)
	}
	if foreignRows != 1 {
		t.Fatalf("the foreign report has %d versions, want 1", foreignRows)
	}

	assertROIField(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports", `{"title":"x"}`),
		http.StatusBadRequest, "params")
	assertROIField(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest("x", strings.Replace(roiReportParams(""), `"report_currency":"CNY"`, `"report_currency":"RMB"`, 1))),
		http.StatusBadRequest, "report_currency")
	assertROIField(t, roiCall(t, h.CreateContentROIReport, wsID, "POST", "/api/content-roi/reports",
		roiReportRequest(strings.Repeat("复", feedbacklearning.MaxReportTitleRunes+1), roiReportParams(""))),
		http.StatusBadRequest, "title")
	assertROIField(t, roiCall(t, h.GenerateContentROIReportVersion, wsID, "POST", "/versions",
		`{"params":`+strings.Replace(roiReportParams(""), `"even_split"`, `"time_decay"`, 1)+`}`, "reportId", reportID),
		http.StatusBadRequest, "attribution_method")
}

// Decode errors on the report bodies name the JSON field only. No database.
func TestContentROIReportBodiesNameOnlyTheJSONField(t *testing.T) {
	cases := []struct {
		name, body, field string
	}{
		{"title", `{"title":1}`, "title"},
		{"params window", `{"params":{"window":{"start":20260901}}}`, "params.window.start"},
		{"params rate", `{"params":{"rates":[{"rate":7.1234}]}}`, "params.rates.rate"},
		{"params scope", `{"params":{"scope":{"work_ids":"w"}}}`, "params.scope.work_ids"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.JSONRequest("POST", "/", tc.body)
			err := decodeROIBody(httptest.NewRecorder(), req, &roiReportBody{})
			fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
			if !ok || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want a FieldError naming %q", err, tc.field)
			}
		})
	}
}
