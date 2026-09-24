package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/034 PR 2 against the real schema: cost allocations, attribution
// judgements and the preview. These run in the handler database suite
// (scripts/test-go-db.sh --suite handler), which CI provisions. The
// calculator itself is tested without a database in feedback-learning.
//
// Contract: specs/034-roi-review/contracts/roi-review.md

// T044 (T028 turned around): a cost's split is stored with the revision that
// carries it. Weights are worked out by the largest-remainder rule; amounts
// must add up; a revision that says nothing keeps the split, recomputed on
// its own amount; [] ends it; and the earlier revision's shares never change.
func TestContentROIAllocationsAreStoredWithTheirRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, accountID, _ := roiWorkspace(t, "roi-allocations")
	h := feedbackHandler(t)
	split := fmt.Sprintf(`,"allocations":[
		{"target_kind":"period","target_id":"2026-09","weight":1},
		{"target_kind":"account","target_id":%q,"weight":1},
		{"target_kind":"campaign_label","target_id":"国庆","weight":1}]`, accountID)
	costID, created := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "100.00", split)), "cost_id")
	shares := allocationsOf(t, created)
	// Sorted by target key; the extra unit to the smallest key.
	if len(shares) != 3 || shares["account:"+accountID] != "3334" || shares["campaign_label:国庆"] != "3333" ||
		shares["period:2026-09"] != "3333" {
		t.Fatalf("shares = %v", shares)
	}
	first := allocationRows(t, costID, 1)

	// Absent: the split carries over, recomputed on the new amount.
	_, second := roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "100.01", `,"base_revision":1`), "costId", costID), "cost_id")
	if shares = allocationsOf(t, second); shares["account:"+accountID] != "3334" || shares["campaign_label:国庆"] != "3334" {
		t.Fatalf("carried shares = %v", shares)
	}
	if got := allocationRows(t, costID, 1); got != first {
		t.Fatalf("revision 1's shares changed:\n%s\n%s", first, got)
	}

	// Amounts that do not add up are refused naming allocations; ones that
	// do are stored as given.
	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "100.01", `,"base_revision":2,"allocations":[
			{"target_kind":"period","target_id":"2026-09","amount":"60.00"},
			{"target_kind":"period","target_id":"2026-10","amount":"40.00"}]`), "costId", costID),
		http.StatusBadRequest, "allocations")
	_, third := roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "100.01", `,"base_revision":2,"allocations":[
			{"target_kind":"period","target_id":"2026-09","amount":"60.01"},
			{"target_kind":"period","target_id":"2026-10","amount":"40.00"}]`), "costId", costID), "cost_id")
	if shares = allocationsOf(t, third); shares["period:2026-09"] != "6001" || shares["period:2026-10"] != "4000" {
		t.Fatalf("amount shares = %v", shares)
	}
	// Carrying amounts that no longer add up is refused, not rescaled.
	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "200.00", `,"base_revision":3`), "costId", costID), http.StatusBadRequest, "allocations")
	// [] ends the split.
	_, fourth := roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "200.00", `,"base_revision":3,"allocations":[]`), "costId", costID), "cost_id")
	if shares = allocationsOf(t, fourth); len(shares) != 0 {
		t.Fatalf("[] left shares %v", shares)
	}

	// A share to an account that is not here answers like a missing record.
	roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("剪辑", "1.00", `,"allocations":[{"target_kind":"account","target_id":"no-such-account","weight":1}]`)).
		Want(http.StatusNotFound)
	// A zero weight is refused by name.
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("剪辑", "1.00", `,"allocations":[{"target_kind":"period","target_id":"2026-09","weight":0}]`)),
		http.StatusBadRequest, "allocations")

	// The history reads every revision with its own shares.
	var history struct {
		Revisions []map[string]any `json:"revisions"`
	}
	roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", costID).Want(http.StatusOK).JSON(&history)
	if len(history.Revisions) != 4 || len(allocationsOf(t, history.Revisions[0])) != 3 ||
		len(allocationsOf(t, history.Revisions[3])) != 0 {
		t.Fatalf("history = %v", history.Revisions)
	}
}

func allocationsOf(t *testing.T, cost map[string]any) map[string]string {
	t.Helper()
	list, ok := cost["allocations"].([]any)
	if !ok {
		t.Fatalf("no allocations list in %v", cost)
	}
	shares := map[string]string{}
	for _, item := range list {
		share := item.(map[string]any)
		shares[fmt.Sprint(share["target_kind"], ":", share["target_id"])] = fmt.Sprint(share["allocated_minor"])
	}
	return shares
}

func allocationRows(t *testing.T, costID string, revision int) string {
	t.Helper()
	var text string
	if err := testPool.QueryRow(t.Context(), `SELECT coalesce(json_agg(row_to_json(a) ORDER BY target_kind, target_id)::text, '')
		FROM content_roi_cost_allocation a WHERE cost_id=$1 AND cost_revision=$2`, costID, revision).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatalf("cost %s revision %d has no shares", costID, revision)
	}
	return text
}

func attributionBody(judgement, touchIDs, weights string, base int) string {
	return fmt.Sprintf(`{"judgement":%q,"touch_ids":%s,"weights":%s,"note":"","base_revision":%d}`,
		judgement, touchIDs, weights, base)
}

func touchRows(t *testing.T, wsID string) string {
	t.Helper()
	var text string
	if err := testPool.QueryRow(t.Context(), `SELECT coalesce(json_agg(row_to_json(r) ORDER BY touch_id, revision)::text, '')
		FROM content_roi_touch_revision r WHERE workspace_id=$1`, wsID).Scan(&text); err != nil {
		t.Fatal(err)
	}
	return text
}

// T045 / SC-005 (second half), FR-020: a judgement is its own revision on
// the deal. Writing it leaves every touch row exactly as it was; "unknown"
// with a touch is refused; a touch of another lead answers like one that does
// not exist; weights must be one per touch.
func TestContentROIAttributionIsARevisionAndLeavesTouchesAlone(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, accountID, publicationID := roiWorkspace(t, "roi-attribution")
	h := feedbackHandler(t)
	leadID := roiLead(t, h, wsID, "客户-判断")
	otherLead := roiLead(t, h, wsID, "客户-别人")
	first, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"))
	second, _ := roiTouch(t, h, wsID, leadID, touchBody("account_only", "douyin", accountID, "", "", "pre_booking"))
	foreign, _ := roiTouch(t, h, wsID, otherLead, touchBody("unknown", "", "", "", "", "other"))
	dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		withLead(dealJSON("TB-attr", "10000.00", "none", ""), leadID)), "deal_id")
	before := touchRows(t, wsID)

	_, judged := roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("multi_touch", fmt.Sprintf(`[%q,%q]`, first, second), `[2,1]`, 0), "dealId", dealID), "deal_id")
	if judged["revision"] != float64(1) || judged["judgement"] != "multi_touch" || judged["recorded_by"] != testUserID {
		t.Fatalf("judgement = %v", judged)
	}
	if got := touchRows(t, wsID); got != before {
		t.Fatalf("writing a judgement changed the touches:\n%s\n%s", before, got)
	}
	if auditSteps(t, wsID, dealID, "record-attribution") != 1 {
		t.Error("the judgement left no audit event")
	}

	// A stale base, and a missing one.
	assertROIField(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("confirmed", fmt.Sprintf(`[%q]`, first), `[]`, 0), "dealId", dealID), http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		`{"judgement":"confirmed","touch_ids":[]}`, "dealId", dealID), http.StatusBadRequest, "base_revision")
	// "unknown" accepts no touch.
	assertROIField(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("unknown", fmt.Sprintf(`[%q]`, first), `[]`, 1), "dealId", dealID), http.StatusBadRequest, "touch_ids")
	// Weights one per touch.
	assertROIField(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("multi_touch", fmt.Sprintf(`[%q,%q]`, first, second), `[1]`, 1), "dealId", dealID),
		http.StatusBadRequest, "weights")
	// Another lead's touch, and no touch at all, answer alike.
	foreignRefusal := roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("confirmed", fmt.Sprintf(`[%q]`, foreign), `[]`, 1), "dealId", dealID)
	foreignRefusal.Want(http.StatusNotFound)
	missingRefusal := roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("confirmed", `["no-such-touch"]`, `[]`, 1), "dealId", dealID)
	missingRefusal.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, foreignRefusal, missingRefusal) {
		t.Error("another lead's touch and a missing touch answered differently")
	}
	roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("unknown", `[]`, `[]`, 0), "dealId", "no-such-deal").Want(http.StatusNotFound)

	// The next revision; unknown with nothing accepted is a real answer.
	_, revised := roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("unknown", `[]`, `[]`, 1), "dealId", dealID), "deal_id")
	if revised["revision"] != float64(2) || auditSteps(t, wsID, dealID, "revise-attribution") != 1 {
		t.Fatalf("second judgement = %v", revised)
	}
	if got := touchRows(t, wsID); got != before {
		t.Fatal("a second judgement changed the touches")
	}
	var detail struct {
		Attributions []map[string]any `json:"attributions"`
	}
	roiCall(t, h.GetContentROIDeal, wsID, "GET", "/", "", "dealId", dealID).Want(http.StatusOK).JSON(&detail)
	if len(detail.Attributions) != 2 || detail.Attributions[0]["judgement"] != "multi_touch" ||
		detail.Attributions[1]["judgement"] != "unknown" {
		t.Fatalf("deal detail attributions = %v", detail.Attributions)
	}
}

// withLead sets a deal body's lead_id.
func withLead(body, leadID string) string {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		panic(err)
	}
	decoded["lead_id"] = leadID
	encoded, _ := json.Marshal(decoded)
	return string(encoded)
}

// roiRowCounts counts every content_roi_ table's rows and the audit rows of
// one workspace.
func roiRowCounts(t *testing.T, wsID string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range append(append([]string{}, roiTableNames...), "content_operation_audit") {
		var n int
		if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, wsID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	return counts
}

// T054: the preview answers exactly what the calculator answers for the
// same input, writes nothing - no record, no audit - and refuses a
// non-member like a missing record. The generation time and timezone are the
// server's, whatever the body says.
func TestContentROIPreviewIsTheCalculatorAndWritesNothing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, accountID, publicationID := roiWorkspace(t, "roi-preview")
	outsider, _, _ := roiWorkspace(t, "roi-preview-outsider")
	dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, outsider, testUserID)
	h := feedbackHandler(t)
	roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("投放", "1000.00", "")), "cost_id")
	leadID := roiLead(t, h, wsID, "客户-预览")
	touchID, _ := roiTouch(t, h, wsID, leadID, touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"))
	dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		withLead(dealJSON("TB-preview", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`), leadID)), "deal_id")
	roiCreated(t, roiCall(t, h.RecordContentROIAttribution, wsID, "POST", "/attribution",
		attributionBody("confirmed", fmt.Sprintf(`[%q]`, touchID), `[]`, 0), "dealId", dealID), "deal_id")

	body := `{"window":{"start":"2026-09-01","end":"2026-09-30","timezone":"UTC"},"report_currency":"CNY",
		"rates":[{"from":"USD","to":"CNY","rate":"7.1234","note":"牌价","entered_by":"someone-else"}],
		"attribution_method":"even_split","conversion":{"from_stage":"咨询","to_stage":"预约"},
		"booking_stage":"预约","scope":{"account_ids":[],"work_ids":[],"campaign_labels":[]},
		"generated_at":"2000-01-01T00:00:00Z"}`
	before := roiRowCounts(t, wsID)
	response := roiCall(t, h.PreviewContentROI, wsID, "POST", "/api/content-roi/preview", body)
	response.Want(http.StatusOK)
	if after := roiRowCounts(t, wsID); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("the preview wrote something: before %v, after %v", before, after)
	}
	var result feedbacklearning.Result
	response.JSON(&result)
	if result.Metrics[feedbacklearning.MetricBusinessROI].Display != "200.00%" ||
		result.Metrics[feedbacklearning.MetricRevenueToSpend].Display != "10.00 倍" {
		t.Fatalf("preview metrics = %+v", result.Metrics)
	}
	if result.GeneratedAt == "2000-01-01T00:00:00Z" || result.Window.Timezone != "Asia/Shanghai" {
		t.Fatalf("the body set server-written parameters: generated_at %q, timezone %q", result.GeneratedAt, result.Window.Timezone)
	}

	// The same input, straight into the calculator, gives the same bytes.
	var params feedbacklearning.ReportParams
	if err := json.Unmarshal([]byte(body), &params); err != nil {
		t.Fatal(err)
	}
	params.Window.Timezone, params.GeneratedAt = result.Window.Timezone, result.GeneratedAt
	params.Rates[0].EnteredBy, params.Rates[0].EnteredAt = testUserID, result.GeneratedAt
	input, err := h.roiStore().LoadReportInput(context.Background(), wsID, testUserID, params)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := feedbacklearning.CalculateROI(input)
	if err != nil {
		t.Fatal(err)
	}
	directJSON, _ := json.Marshal(direct)
	var previewJSON any
	response.JSON(&previewJSON)
	reencoded, _ := json.Marshal(previewJSON)
	var directDecoded any
	_ = json.Unmarshal(directJSON, &directDecoded)
	directReencoded, _ := json.Marshal(directDecoded)
	if string(reencoded) != string(directReencoded) {
		t.Fatalf("preview and calculator differ:\n%s\n%s", reencoded, directReencoded)
	}

	// Parameters are refused by name.
	assertROIField(t, roiCall(t, h.PreviewContentROI, wsID, "POST", "/api/content-roi/preview",
		`{"window":{"start":"2026-09-01","end":"2026-09-30"},"report_currency":"RMB","attribution_method":"even_split"}`),
		http.StatusBadRequest, "report_currency")
	assertROIField(t, roiCall(t, h.PreviewContentROI, wsID, "POST", "/api/content-roi/preview",
		`{"window":{"start":"2026-09-01","end":"2026-09-30"},"report_currency":"CNY","attribution_method":"time_decay"}`),
		http.StatusBadRequest, "attribution_method")

	// A non-member is refused exactly like a missing record.
	refused := roiCall(t, h.PreviewContentROI, outsider, "POST", "/api/content-roi/preview", body)
	refused.Want(http.StatusNotFound)
	reference := roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", "no-such-cost")
	if !sameRefusalApartFromTrace(t, refused, reference) {
		t.Error("the preview refused a non-member differently from a missing record")
	}
}

// SC-015 for the PR 2 write: after a workspace deletion has committed, a
// judgement is refused by the fence and leaves no row.
func TestContentROIAttributionIsFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	wsID, _, _ := roiWorkspace(t, "roi-attribution-fence")
	h := feedbackHandler(t)
	dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-fence-attr", "100.00", "none", "")), "deal_id")
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, wsID); err != nil {
		t.Fatal(err)
	}
	zero := 0
	_, err := h.roiStore().RecordAttribution(ctx, wsID, testUserID, dealID,
		feedbacklearning.AttributionInput{Judgement: "unknown"}, feedbacklearning.Revision{BaseRevision: &zero})
	if !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a judgement after the delete committed = %v, want ErrNotFound", err)
	}
	var rows int
	if err = testPool.QueryRow(ctx, `SELECT count(*) FROM content_roi_attribution_revision WHERE workspace_id=$1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("%d judgement rows after a fenced write", rows)
	}
}

// Decode errors on the PR 2 bodies name the JSON field only. No database.
func TestContentROICalcBodiesNameOnlyTheJSONField(t *testing.T) {
	cases := []struct {
		name   string
		target func() any
		body   string
		field  string
	}{
		{"allocation weight", func() any { return &roiCostBody{} }, `{"allocations":[{"weight":"1"}]}`, "allocations.weight"},
		{"allocation amount", func() any { return &roiCostBody{} }, `{"allocations":[{"amount":10}]}`, "allocations.amount"},
		{"attribution weights", func() any { return &roiAttributionBody{} }, `{"weights":"1"}`, "weights"},
		{"attribution touch_ids", func() any { return &roiAttributionBody{} }, `{"touch_ids":"t"}`, "touch_ids"},
		{"attribution base_revision", func() any { return &roiAttributionBody{} }, `{"base_revision":"0"}`, "base_revision"},
		{"preview window", func() any { return &roiPreviewBody{} }, `{"window":{"start":20260901}}`, "window.start"},
		{"preview rate", func() any { return &roiPreviewBody{} }, `{"rates":[{"rate":7.1234}]}`, "rates.rate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.JSONRequest("POST", "/", tc.body)
			err := decodeROIBody(httptest.NewRecorder(), req, tc.target())
			fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
			if !ok || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want a FieldError naming %q", err, tc.field)
			}
		})
	}
}
