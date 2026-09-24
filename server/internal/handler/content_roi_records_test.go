package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/034 PR 1 against the real schema: costs, leads, touches, deals and
// refunds/adjustments. These run in the handler database suite
// (scripts/test-go-db.sh --suite handler), which CI provisions.
//
// Contract: specs/034-roi-review/contracts/roi-review.md

var roiTableNames = []string{
	"content_roi_cost_revision",
	"content_roi_lead_revision",
	"content_roi_touch_revision",
	"content_roi_deal_revision",
	"content_roi_adjustment_revision",
}

// roiWorkspace makes a brand the test user owns, with one content account and
// one publication record to point touches at.
func roiWorkspace(t *testing.T, slug string) (wsID, accountID, publicationID string) {
	t.Helper()
	slug = fmt.Sprintf("%s-%d", slug, time.Now().UnixNano())
	wsID = dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "ROI " + slug, "slug": slug, "description": "roi test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)
	accountID = "acct-" + slug
	dbfx.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name)
		VALUES ($1, $2, 'douyin', '品牌主号')`, accountID, wsID)
	publicationID = "pub-" + slug
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a','','douyin','verified_published',$3,'https://example.invalid/p/1', now())`,
		publicationID, wsID, testUserID)
	t.Cleanup(func() {
		background := context.Background()
		for _, table := range roiTableNames {
			_, _ = testPool.Exec(background, `DELETE FROM `+table+` WHERE workspace_id = $1`, wsID)
		}
		for _, statement := range []string{
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
			`DELETE FROM content_account WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id::text = $1`,
		} {
			_, _ = testPool.Exec(background, statement, wsID)
		}
	})
	return wsID, accountID, publicationID
}

func roiCall(t *testing.T, handler http.HandlerFunc, wsID, method, path, body string, params ...string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest(method, path, body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	if len(params) > 0 {
		req = testutil.WithURLParams(req, params...)
	}
	return testutil.Call(t, handler, req)
}

func costJSON(category, amount string, extra string) string {
	return fmt.Sprintf(`{"category":%q,"pricing":"amount","amount":%q,"currency":"CNY",
		"incurred_at":"2026-09-10T02:00:00Z","ad_spend":false,"account_id":"","work_id":"",
		"campaign_label":"","evidence_note":"发票 0412","note":""%s}`, category, amount, extra)
}

func roiCreated(t *testing.T, response *testutil.Response, idField string) (string, map[string]any) {
	t.Helper()
	response.Want(http.StatusCreated)
	body := response.Map()
	id, _ := body[idField].(string)
	if id == "" {
		t.Fatalf("no %s in %s", idField, response.Text())
	}
	return id, body
}

func assertROIField(t *testing.T, response *testutil.Response, status int, field string) {
	t.Helper()
	response.Want(status)
	if got := response.Map()["field"]; got != field {
		t.Fatalf("refusal named %v, want %q: %s", got, field, response.Text())
	}
}

// rowJSON is a revision row as the database holds it, for byte-for-byte
// comparisons across later writes.
func rowJSON(t *testing.T, table, idColumn, id string, revision int) string {
	t.Helper()
	var text string
	if err := testPool.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM `+table+` r
		WHERE `+idColumn+` = $1 AND revision = $2`, id, revision).Scan(&text); err != nil {
		t.Fatalf("read %s %s r%d: %v", table, id, revision, err)
	}
	return text
}

func auditSteps(t *testing.T, wsID, objectID, step string) int {
	t.Helper()
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_operation_audit
		WHERE workspace_id = $1 AND payload->>'object_id' = $2 AND payload->>'step' = $3`, wsID, objectID, step).Scan(&count); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	return count
}

// T017 / FR-013: a revision is a new row; the old one does not change by a
// byte; a stale base is 409 naming base_revision; voiding is a revision.
func TestContentROICostRevisionsAreAppendOnly(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-cost-revisions")
	h := feedbackHandler(t)

	costID, created := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "3000.00", "")), "cost_id")
	if created["revision"] != float64(1) || created["amount_minor"] != "300000" || created["amount"] != "3000.00" {
		t.Fatalf("revision 1 = %s", roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", costID).Text())
	}
	if created["recorded_by"] != testUserID || created["source_type"] != "manual" {
		t.Fatalf("server-written fields: %v %v", created["recorded_by"], created["source_type"])
	}
	first := rowJSON(t, "content_roi_cost_revision", "cost_id", costID, 1)

	_, second := roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3200.00", `,"base_revision":1`), "costId", costID), "cost_id")
	if second["revision"] != float64(2) || second["amount_minor"] != "320000" {
		t.Fatalf("revision 2 = %v", second)
	}
	if got := rowJSON(t, "content_roi_cost_revision", "cost_id", costID, 1); got != first {
		t.Fatalf("revision 1 changed:\n%s\n%s", first, got)
	}

	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3300.00", `,"base_revision":1`), "costId", costID), http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3300.00", ""), "costId", costID), http.StatusBadRequest, "base_revision")

	_, voided := roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3200.00", `,"base_revision":2,"voided":true`), "costId", costID), "cost_id")
	if voided["revision"] != float64(3) || voided["voided"] != true {
		t.Fatalf("void = %v", voided)
	}

	var history struct {
		Revisions []struct {
			Revision int  `json:"revision"`
			Voided   bool `json:"voided"`
		} `json:"revisions"`
	}
	roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", costID).Want(http.StatusOK).JSON(&history)
	if len(history.Revisions) != 3 || !history.Revisions[2].Voided || history.Revisions[0].Voided {
		t.Fatalf("history = %+v", history.Revisions)
	}
	// A voided cost leaves the default list and stays in the inactive one.
	var list struct {
		Costs []map[string]any `json:"costs"`
	}
	roiCall(t, h.ListContentROICosts, wsID, "GET", "/api/content-roi/costs", "").Want(http.StatusOK).JSON(&list)
	if len(list.Costs) != 0 {
		t.Fatalf("a voided cost is still listed: %v", list.Costs)
	}
	roiCall(t, h.ListContentROICosts, wsID, "GET", "/api/content-roi/costs?include_inactive=true", "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Costs) != 1 {
		t.Fatalf("include_inactive lost the voided cost: %v", list.Costs)
	}
}

// T018: two writers both at base_revision=1. The test seam parks both after
// every check has passed, so the read check cannot be what stops the second
// one - the unique index on (workspace_id, cost_id, revision) must, and it
// must come back as the same 409.
func TestContentROIConcurrentRevisionsLeaveExactlyOneWinner(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-cost-race")
	h := feedbackHandler(t)
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("投放", "800.00", "")), "cost_id")

	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	store := h.roiStore()
	store.BeforeRevisionInsert = func(context.Context, string, string) {
		arrived.Done()
		<-release
	}
	go func() {
		waited := make(chan struct{})
		go func() { arrived.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(10 * time.Second):
			t.Error("both writers never reached the insert together; the index layer was not exercised")
		}
		close(release)
	}()

	amount := "900.00"
	base := 1
	input := feedbacklearning.CostInput{
		Category: "投放", Pricing: "amount", Amount: &amount, Currency: "CNY",
		IncurredAt: "2026-09-10T02:00:00Z",
	}
	errs := make([]error, 2)
	var done sync.WaitGroup
	for i := range 2 {
		done.Add(1)
		go func() {
			defer done.Done()
			_, errs[i] = store.ReviseCost(context.Background(), wsID, testUserID, costID, input,
				feedbacklearning.Revision{BaseRevision: &base})
		}()
	}
	done.Wait()

	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.As(err, new(feedbacklearning.RevisionConflict)):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d (%v), want exactly one of each", wins, conflicts, errs)
	}
	var revisions int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_roi_cost_revision
		WHERE workspace_id=$1 AND cost_id=$2`, wsID, costID).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 {
		t.Fatalf("%d revisions stored, want 2", revisions)
	}
}

func roiLead(t *testing.T, h *Handler, wsID, customerRef string) string {
	t.Helper()
	id, _ := roiCreated(t, roiCall(t, h.CreateContentROILead, wsID, "POST", "/api/content-roi/leads",
		fmt.Sprintf(`{"customer_ref":%q,"stage":"咨询","qualified":true,
			"first_seen_at":"2026-09-10T02:00:00Z","note":""}`, customerRef)), "lead_id")
	return id
}

func roiTouch(t *testing.T, h *Handler, wsID, leadID, body string) (string, map[string]any) {
	t.Helper()
	return roiCreated(t, roiCall(t, h.AddContentROITouch, wsID, "POST", "/touches", body, "leadId", leadID), "touch_id")
}

func touchBody(evidence, platform, accountID, workID, publicationID, role string) string {
	return fmt.Sprintf(`{"evidence_type":%q,"platform":%q,"account_id":%q,"work_id":%q,
		"publication_record_id":%q,"role":%q,"paid":false,"occurred_at":"2026-09-10T03:00:00Z",
		"evidence_note":"","note":""}`, evidence, platform, accountID, workID, publicationID, role)
}

func leadTouchIDs(t *testing.T, h *Handler, wsID, leadID string) []string {
	t.Helper()
	var detail struct {
		Touches []struct {
			TouchID string `json:"touch_id"`
		} `json:"touches"`
	}
	roiCall(t, h.GetContentROILead, wsID, "GET", "/", "", "leadId", leadID).Want(http.StatusOK).JSON(&detail)
	ids := []string{}
	for _, touch := range detail.Touches {
		ids = append(ids, touch.TouchID)
	}
	return ids
}

// T019 / FR-022: merging B into A is a revision of B with an audit event; A
// then reads both leads' touches, each once; A→B→A is refused naming
// merged_into; unmerging restores the split.
func TestContentROILeadMergeIsARevisionAndReadsTouchesOnce(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-merge")
	h := feedbackHandler(t)
	leadA := roiLead(t, h, wsID, "客户-A")
	leadB := roiLead(t, h, wsID, "客户-B")
	touchA, _ := roiTouch(t, h, wsID, leadA, touchBody("customer_statement", "", "", "", "", "first_touch"))
	touchB, _ := roiTouch(t, h, wsID, leadB, touchBody("unknown", "", "", "", "", "pre_booking"))

	_, merged := roiCreated(t, roiCall(t, h.MergeContentROILead, wsID, "POST", "/merge",
		fmt.Sprintf(`{"target_lead_id":%q,"note":"同一位客户","base_revision":1}`, leadA), "leadId", leadB), "lead_id")
	if merged["merged_into"] != leadA || merged["revision"] != float64(2) {
		t.Fatalf("merge revision = %v", merged)
	}
	if auditSteps(t, wsID, leadB, "merge-lead") != 1 {
		t.Error("the merge left no audit event")
	}
	got := leadTouchIDs(t, h, wsID, leadA)
	if len(got) != 2 || !slices.Contains(got, touchA) || !slices.Contains(got, touchB) {
		t.Fatalf("A reads touches %v, want %s and %s once each", got, touchA, touchB)
	}
	// The touch rows themselves were not touched.
	if rowJSON(t, "content_roi_touch_revision", "touch_id", touchB, 1) == "" {
		t.Fatal("touch B vanished")
	}

	var list struct {
		Leads []struct {
			LeadID string `json:"lead_id"`
		} `json:"leads"`
	}
	roiCall(t, h.ListContentROILeads, wsID, "GET", "/api/content-roi/leads", "").Want(http.StatusOK).JSON(&list)
	if len(list.Leads) != 1 || list.Leads[0].LeadID != leadA {
		t.Fatalf("a merged lead still counts: %+v", list.Leads)
	}

	assertROIField(t, roiCall(t, h.MergeContentROILead, wsID, "POST", "/merge",
		fmt.Sprintf(`{"target_lead_id":%q,"note":"","base_revision":1}`, leadB), "leadId", leadA),
		http.StatusBadRequest, "merged_into")

	roiCreated(t, roiCall(t, h.MergeContentROILead, wsID, "POST", "/merge",
		`{"target_lead_id":"","note":"其实不是同一人","base_revision":2}`, "leadId", leadB), "lead_id")
	if got = leadTouchIDs(t, h, wsID, leadA); len(got) != 1 || got[0] != touchA {
		t.Fatalf("after unmerge A reads %v", got)
	}
	if auditSteps(t, wsID, leadB, "unmerge-lead") != 1 {
		t.Error("the unmerge left no audit event")
	}
}

// T020 / SC-005 (first half): five kinds of source, each read back exactly as
// written - an empty work stays empty, the evidence type stays the one given.
func TestContentROIFiveSourcesReadBackAsWritten(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, accountID, publicationID := roiWorkspace(t, "roi-sources")
	h := feedbackHandler(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"platform-linked", touchBody("platform_linked_content", "douyin", accountID, "", publicationID, "first_touch"), "platform_linked_content"},
		{"customer-statement", touchBody("customer_statement", "douyin", accountID, "", "", "first_touch"), "customer_statement"},
		{"account-only", touchBody("account_only", "douyin", accountID, "", "", "pre_booking"), "account_only"},
		{"unknown", touchBody("unknown", "", "", "", "", "other"), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			leadID := roiLead(t, h, wsID, "")
			touchID, _ := roiTouch(t, h, wsID, leadID, tc.body)
			var detail struct {
				Touches []struct {
					Current map[string]any `json:"current"`
				} `json:"touches"`
			}
			roiCall(t, h.GetContentROILead, wsID, "GET", "/", "", "leadId", leadID).Want(http.StatusOK).JSON(&detail)
			if len(detail.Touches) != 1 {
				t.Fatalf("touches = %v", detail.Touches)
			}
			current := detail.Touches[0].Current
			if current["touch_id"] != touchID || current["evidence_type"] != tc.want || current["work_id"] != "" {
				t.Fatalf("read back %v", current)
			}
		})
	}

	t.Run("multi-touch", func(t *testing.T) {
		leadID := roiLead(t, h, wsID, "")
		first, _ := roiTouch(t, h, wsID, leadID, touchBody("customer_statement", "", "", "", "", "first_touch"))
		second, _ := roiTouch(t, h, wsID, leadID, touchBody("dedicated_channel", "", "", "", "", "pre_booking"))
		got := leadTouchIDs(t, h, wsID, leadID)
		if len(got) != 2 || !slices.Contains(got, first) || !slices.Contains(got, second) {
			t.Fatalf("multi-touch lead reads %v", got)
		}
	})

	// A reference that does not exist here answers like the lead not
	// existing (contract §6 step 2).
	leadID := roiLead(t, h, wsID, "")
	missing := roiCall(t, h.AddContentROITouch, wsID, "POST", "/touches",
		touchBody("account_only", "douyin", "no-such-account", "", "", "other"), "leadId", leadID)
	missing.Want(http.StatusNotFound)
	absentLead := roiCall(t, h.AddContentROITouch, wsID, "POST", "/touches",
		touchBody("unknown", "", "", "", "", "other"), "leadId", "no-such-lead")
	absentLead.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, missing, absentLead) {
		t.Error("a missing account and a missing lead answered differently")
	}
}

func dealJSON(orderRef, amount, basis, extra string) string {
	return fmt.Sprintf(`{"lead_id":"","order_ref":%q,"amount":%q,"currency":"CNY",
		"closed_at":"2026-09-12T02:00:00Z","gross_basis":%q,"note":""%s}`, orderRef, amount, basis, extra)
}

func refundBody(amount, currency, extra string) string {
	return fmt.Sprintf(`{"kind":"refund","amount":%q,"currency":%q,
		"occurred_at":"2026-09-20T02:00:00Z","note":""%s}`, amount, currency, extra)
}

// T021: both gross bases read back; a refund past the net is refused naming
// amount; a refund in another currency is refused naming currency.
func TestContentROIDealsAndRefundsKeepTheirBasisAndTheirNet(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-deals")
	h := feedbackHandler(t)

	cogsDeal, cogs := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-1", "10000.00", "cogs", `,"cogs":"7000.00"`)), "deal_id")
	if cogs["cogs_minor"] != "700000" || cogs["gross_profit_minor"] != nil || cogs["amount_minor"] != "1000000" {
		t.Fatalf("cogs deal = %v", cogs)
	}
	_, stated := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-2", "10000.00", "stated_gross_profit", `,"gross_profit":"3000.00"`)), "deal_id")
	if stated["gross_profit_minor"] != "300000" || stated["cogs_minor"] != nil {
		t.Fatalf("stated deal = %v", stated)
	}

	refundID, refund := roiCreated(t, roiCall(t, h.AddContentROIAdjustment, wsID, "POST", "/adjustments",
		refundBody("1000.00", "CNY", `,"gross_delta":"300.00"`), "dealId", cogsDeal), "adjustment_id")
	if refund["revenue_delta_minor"] != "100000" || refund["gross_delta_minor"] != "30000" {
		t.Fatalf("refund = %v", refund)
	}
	// 10000 - 1000 = 9000 left; 9000.01 more takes it below zero.
	assertROIField(t, roiCall(t, h.AddContentROIAdjustment, wsID, "POST", "/adjustments",
		refundBody("9000.01", "CNY", ""), "dealId", cogsDeal), http.StatusBadRequest, "amount")
	assertROIField(t, roiCall(t, h.AddContentROIAdjustment, wsID, "POST", "/adjustments",
		refundBody("10.00", "USD", ""), "dealId", cogsDeal), http.StatusBadRequest, "currency")
	// Revising the refund replaces it rather than adding to it.
	roiCreated(t, roiCall(t, h.ReviseContentROIAdjustment, wsID, "POST", "/revisions",
		refundBody("10000.00", "CNY", `,"base_revision":1`), "dealId", cogsDeal, "adjustmentId", refundID), "adjustment_id")
	// Lowering the deal below its refunds is refused the same way.
	assertROIField(t, roiCall(t, h.ReviseContentROIDeal, wsID, "POST", "/revisions",
		dealJSON("TB-1", "9999.99", "cogs", `,"cogs":"7000.00","base_revision":1`), "dealId", cogsDeal),
		http.StatusBadRequest, "amount")
	// An adjustment reached through another deal's path is missing.
	roiCall(t, h.ReviseContentROIAdjustment, wsID, "POST", "/revisions",
		refundBody("1.00", "CNY", `,"base_revision":2`), "dealId", "no-such-deal", "adjustmentId", refundID).
		Want(http.StatusNotFound)

	var detail struct {
		Adjustments []struct {
			AdjustmentID string           `json:"adjustment_id"`
			Revisions    []map[string]any `json:"revisions"`
		} `json:"adjustments"`
	}
	roiCall(t, h.GetContentROIDeal, wsID, "GET", "/", "", "dealId", cogsDeal).Want(http.StatusOK).JSON(&detail)
	if len(detail.Adjustments) != 1 || len(detail.Adjustments[0].Revisions) != 2 {
		t.Fatalf("deal detail adjustments = %+v", detail.Adjustments)
	}
}

// T022 / FR-026: a manual entry that looks like an existing one is held with
// the ids it matched; confirming "not a duplicate" writes it, keeps the ids
// on the row and audits the confirmation. Voided and merged records take no
// part.
func TestContentROIPossibleDuplicatesAreHeldUntilConfirmed(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-dupes")
	h := feedbackHandler(t)

	firstDeal, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-77", "500.00", "none", "")), "deal_id")
	held := roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-77", "650.00", "none", ""))
	held.Want(http.StatusConflict)
	body := held.Map()
	matches, _ := body["matches"].([]any)
	if body["code"] != "possible_duplicate" || len(matches) != 1 || matches[0] != firstDeal {
		t.Fatalf("held = %s", held.Text())
	}
	secondDeal, second := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-77", "650.00", "none", fmt.Sprintf(`,"not_duplicate_of":[%q]`, firstDeal))), "deal_id")
	if confirmations, _ := second["not_duplicate_of"].([]any); len(confirmations) != 1 || confirmations[0] != firstDeal {
		t.Fatalf("not_duplicate_of = %v", second["not_duplicate_of"])
	}
	if auditSteps(t, wsID, secondDeal, "confirm-not-duplicate") != 1 {
		t.Error("the confirmation left no audit event")
	}

	// A voided cost is not a duplicate of anything.
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("剪辑", "900.00", "")), "cost_id")
	roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("剪辑", "900.00", "")).Want(http.StatusConflict)
	roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("剪辑", "900.00", `,"base_revision":1,"voided":true`), "costId", costID), "cost_id")
	roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("剪辑", "900.00", "")), "cost_id")

	// A merged lead is not a duplicate of anything either.
	merged := roiLead(t, h, wsID, "客户-0412")
	target := roiLead(t, h, wsID, "客户-0413")
	roiCreated(t, roiCall(t, h.MergeContentROILead, wsID, "POST", "/merge",
		fmt.Sprintf(`{"target_lead_id":%q,"note":"","base_revision":1}`, target), "leadId", merged), "lead_id")
	roiLead(t, h, wsID, "客户-0412")
}

// T023 / SC-015: once a workspace deletion has committed, every write path is
// refused by the fence and leaves no row behind.
func TestContentROIWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	wsID, _, _ := roiWorkspace(t, "roi-fence")
	h := feedbackHandler(t)
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "1.00", "")), "cost_id")
	leadID := roiLead(t, h, wsID, "客户-fence")
	otherLead := roiLead(t, h, wsID, "客户-fence-2")
	touchID, _ := roiTouch(t, h, wsID, leadID, touchBody("unknown", "", "", "", "", "other"))
	dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-fence", "100.00", "none", "")), "deal_id")
	adjustmentID, _ := roiCreated(t, roiCall(t, h.AddContentROIAdjustment, wsID, "POST", "/adjustments",
		refundBody("1.00", "CNY", ""), "dealId", dealID), "adjustment_id")

	counts := func() map[string]int {
		result := map[string]int{}
		for _, table := range roiTableNames {
			var n int
			if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, wsID).Scan(&n); err != nil {
				t.Fatal(err)
			}
			result[table] = n
		}
		return result
	}
	before := counts()

	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, wsID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	store := h.roiStore()
	amount, one := "2.00", 1
	cost := feedbacklearning.CostInput{Category: "拍摄", Pricing: "amount", Amount: &amount, Currency: "CNY", IncurredAt: "2026-09-10T02:00:00Z"}
	lead := feedbacklearning.LeadInput{CustomerRef: "客户-new", FirstSeenAt: "2026-09-10T02:00:00Z"}
	touch := feedbacklearning.TouchInput{EvidenceType: "unknown", Role: "other", OccurredAt: "2026-09-10T02:00:00Z"}
	deal := feedbacklearning.DealInput{OrderRef: "TB-new", Amount: &amount, Currency: "CNY", ClosedAt: "2026-09-10T02:00:00Z", GrossBasis: "none"}
	refund := feedbacklearning.AdjustmentInput{Kind: "refund", Amount: &amount, Currency: "CNY", OccurredAt: "2026-09-10T02:00:00Z"}
	base := feedbacklearning.Revision{BaseRevision: &one}
	for _, write := range []struct {
		name string
		call func() error
	}{
		{"create-cost", func() error {
			_, err := store.CreateCost(ctx, wsID, testUserID, cost, feedbacklearning.RecordManual)
			return err
		}},
		{"revise-cost", func() error { _, err := store.ReviseCost(ctx, wsID, testUserID, costID, cost, base); return err }},
		{"create-lead", func() error {
			_, err := store.CreateLead(ctx, wsID, testUserID, lead, feedbacklearning.RecordManual)
			return err
		}},
		{"revise-lead", func() error { _, err := store.ReviseLead(ctx, wsID, testUserID, leadID, lead, base); return err }},
		{"merge-lead", func() error {
			_, err := store.MergeLead(ctx, wsID, testUserID, otherLead, feedbacklearning.MergeInput{TargetLeadID: leadID}, base)
			return err
		}},
		{"add-touch", func() error { _, err := store.AddTouch(ctx, wsID, testUserID, leadID, touch); return err }},
		{"revise-touch", func() error {
			_, err := store.ReviseTouch(ctx, wsID, testUserID, leadID, touchID, touch, base)
			return err
		}},
		{"create-deal", func() error {
			_, err := store.CreateDeal(ctx, wsID, testUserID, deal, feedbacklearning.RecordManual)
			return err
		}},
		{"revise-deal", func() error { _, err := store.ReviseDeal(ctx, wsID, testUserID, dealID, deal, base); return err }},
		{"add-adjustment", func() error { _, err := store.AddAdjustment(ctx, wsID, testUserID, dealID, refund); return err }},
		{"revise-adjustment", func() error {
			_, err := store.ReviseAdjustment(ctx, wsID, testUserID, dealID, adjustmentID, refund, base)
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, feedbacklearning.ErrNotFound) {
				t.Fatalf("%s after the delete committed = %v, want ErrNotFound", write.name, err)
			}
		})
	}
	after := counts()
	for table, want := range before {
		if after[table] != want {
			t.Errorf("%s has %d rows after the fenced writes, want %d", table, after[table], want)
		}
	}
}

// T025: the real owner-delete path removes every ROI row of the deleted
// workspace and none of its neighbour's.
func TestDeleteWorkspaceRemovesROIRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	target, _, _ := roiWorkspace(t, "roi-delete-target")
	neighbor, _, _ := roiWorkspace(t, "roi-delete-neighbor")
	h := feedbackHandler(t)
	for _, wsID := range []string{target, neighbor} {
		roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
			costJSON("拍摄", "1.00", "")), "cost_id")
		leadID := roiLead(t, h, wsID, "客户-delete")
		roiTouch(t, h, wsID, leadID, touchBody("unknown", "", "", "", "", "other"))
		dealID, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
			dealJSON("TB-delete", "100.00", "none", "")), "deal_id")
		roiCreated(t, roiCall(t, h.AddContentROIAdjustment, wsID, "POST", "/adjustments",
			refundBody("1.00", "CNY", ""), "dealId", dealID), "adjustment_id")
	}

	request := newRequest(http.MethodDelete, "/api/workspaces/"+target, nil)
	request = withURLParam(request, "id", target)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)

	for _, table := range roiTableNames {
		var targetRows, neighborRows int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, target).Scan(&targetRows); err != nil {
			t.Fatal(err)
		}
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, neighbor).Scan(&neighborRows); err != nil {
			t.Fatal(err)
		}
		if targetRows != 0 {
			t.Errorf("%s kept %d rows of the deleted workspace", table, targetRows)
		}
		if neighborRows != 1 {
			t.Errorf("%s has %d neighbour rows, want 1", table, neighborRows)
		}
	}
}

// T026 / contract §6: the decision order, first failure winning, one case per
// step. Steps 1 and 2 answer byte for byte alike apart from the trace id.
func TestContentROIDecisionOrderRefusesAtTheFirstFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-order")
	outsider, _, _ := roiWorkspace(t, "roi-order-outsider")
	dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, outsider, testUserID)
	h := feedbackHandler(t)
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "1.00", "")), "cost_id")

	// 1: not a member of the workspace in the header.
	notMember := roiCall(t, h.ReviseContentROICost, outsider, "POST", "/revisions",
		costJSON("拍摄", "1.00", `,"base_revision":1`), "costId", costID)
	notMember.Want(http.StatusNotFound)
	// 2: a member, but the path id does not exist here.
	missing := roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "1.00", `,"base_revision":1`), "costId", "no-such-cost")
	missing.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, notMember, missing) {
		t.Error("not-a-member and not-found answered differently")
	}
	// 2 before 3: a missing reference wins over a bad controlled value.
	both := strings.Replace(costJSON("拍摄", "1.00", ""), `"account_id":""`, `"account_id":"no-such-account"`, 1)
	both = strings.Replace(both, `"pricing":"amount"`, `"pricing":"fixed"`, 1)
	roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", both).Want(http.StatusNotFound)
	// 3: a controlled set.
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		strings.Replace(costJSON("拍摄", "1.00", ""), `"pricing":"amount"`, `"pricing":"fixed"`, 1)),
		http.StatusBadRequest, "pricing")
	// 4: amount format, before the too-long category of step 5.
	long := strings.Repeat("拍", feedbacklearning.MaxShortRunes+1)
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON(long, "1.001", "")), http.StatusBadRequest, "amount")
	// 5: length.
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON(long, "1.00", "")), http.StatusBadRequest, "category")
	// 6: base_revision.
	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "1.00", `,"base_revision":7`), "costId", costID), http.StatusConflict, "base_revision")
	// 7: possible duplicate.
	dup := roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", costJSON("拍摄", "1.00", ""))
	dup.Want(http.StatusConflict)
	if dup.Map()["code"] != "possible_duplicate" {
		t.Fatalf("step 7 = %s", dup.Text())
	}
}

// SC-008: every endpoint refuses a non-member exactly as it refuses a missing
// record.
func TestEveryContentROIEndpointRefusesANonMemberLikeAMissingRecord(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-scope")
	outsider, _, _ := roiWorkspace(t, "roi-scope-outsider")
	dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, outsider, testUserID)
	h := feedbackHandler(t)
	reference := roiCall(t, h.GetContentROICost, wsID, "GET", "/", "", "costId", "no-such-cost")
	reference.Want(http.StatusNotFound)

	for _, endpoint := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
		params  []string
	}{
		{"list-costs", h.ListContentROICosts, "GET", nil},
		{"create-cost", h.CreateContentROICost, "POST", nil},
		{"get-cost", h.GetContentROICost, "GET", []string{"costId", "c"}},
		{"revise-cost", h.ReviseContentROICost, "POST", []string{"costId", "c"}},
		{"list-leads", h.ListContentROILeads, "GET", nil},
		{"create-lead", h.CreateContentROILead, "POST", nil},
		{"get-lead", h.GetContentROILead, "GET", []string{"leadId", "l"}},
		{"revise-lead", h.ReviseContentROILead, "POST", []string{"leadId", "l"}},
		{"merge-lead", h.MergeContentROILead, "POST", []string{"leadId", "l"}},
		{"add-touch", h.AddContentROITouch, "POST", []string{"leadId", "l"}},
		{"revise-touch", h.ReviseContentROITouch, "POST", []string{"leadId", "l", "touchId", "t"}},
		{"list-deals", h.ListContentROIDeals, "GET", nil},
		{"create-deal", h.CreateContentROIDeal, "POST", nil},
		{"get-deal", h.GetContentROIDeal, "GET", []string{"dealId", "d"}},
		{"revise-deal", h.ReviseContentROIDeal, "POST", []string{"dealId", "d"}},
		{"add-adjustment", h.AddContentROIAdjustment, "POST", []string{"dealId", "d"}},
		{"revise-adjustment", h.ReviseContentROIAdjustment, "POST", []string{"dealId", "d", "adjustmentId", "a"}},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			response := roiCall(t, endpoint.handler, outsider, endpoint.method, "/", "{}", endpoint.params...)
			response.Want(http.StatusNotFound)
			if !sameRefusalApartFromTrace(t, response, reference) {
				t.Errorf("%s refused a non-member differently from a missing record: %s", endpoint.name, response.Text())
			}
		})
	}
}

// T027 / FR-003: an amount sent as a JSON number is refused naming it; the
// server-written fields in a body are ignored, not obeyed.
func TestContentROIAmountsMustBeStringsAndServerFieldsAreIgnored(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-strings")
	h := feedbackHandler(t)
	numeric := strings.Replace(costJSON("拍摄", "3000.00", ""), `"amount":"3000.00"`, `"amount":3000.00`, 1)
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs", numeric),
		http.StatusBadRequest, "amount")
	numericDeal := strings.Replace(dealJSON("TB-n", "1.00", "none", ""), `"amount":"1.00"`, `"amount":1`, 1)
	assertROIField(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals", numericDeal),
		http.StatusBadRequest, "amount")

	_, cost := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "3000.00", `,"recorded_by":"someone-else","source_type":"import"`)), "cost_id")
	if cost["recorded_by"] != testUserID || cost["source_type"] != "manual" {
		t.Fatalf("the body set server-written fields: %v %v", cost["recorded_by"], cost["source_type"])
	}
}

// T028: allocations arrive with PR 2. Until then a non-empty one is refused
// by name on both endpoints; null and [] are no allocation at all.
func TestContentROIAllocationsAreRefusedUntilPR2(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-allocations")
	h := feedbackHandler(t)
	allocation := `,"allocations":[{"target_kind":"work","target_id":"w1","weight":1}]`
	assertROIField(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "3000.00", allocation)), http.StatusBadRequest, "allocations")
	costID, _ := roiCreated(t, roiCall(t, h.CreateContentROICost, wsID, "POST", "/api/content-roi/costs",
		costJSON("拍摄", "3000.00", `,"allocations":[]`)), "cost_id")
	assertROIField(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3000.00", `,"base_revision":1`+allocation), "costId", costID),
		http.StatusBadRequest, "allocations")
	roiCreated(t, roiCall(t, h.ReviseContentROICost, wsID, "POST", "/revisions",
		costJSON("拍摄", "3100.00", `,"base_revision":1,"allocations":null`), "costId", costID), "cost_id")
}

// A value of the wrong JSON type is refused naming the JSON field alone -
// never the Go struct path encoding/json reports for an embedded input
// ("CostInput.amount"). Every body type, every field a caller could send with
// the wrong type. No database: this is the decode step only.
func TestContentROIDecodeErrorsNameOnlyTheJSONField(t *testing.T) {
	cases := []struct {
		name   string
		target func() any
		body   string
		field  string
	}{
		{"cost amount", func() any { return &roiCostBody{} }, `{"amount":3000}`, "amount"},
		{"cost labor_rate", func() any { return &roiCostBody{} }, `{"labor_rate":150}`, "labor_rate"},
		{"cost labor_minutes", func() any { return &roiCostBody{} }, `{"labor_minutes":"360"}`, "labor_minutes"},
		{"cost category", func() any { return &roiCostBody{} }, `{"category":1}`, "category"},
		{"cost not_duplicate_of", func() any { return &roiCostBody{} }, `{"not_duplicate_of":"c1"}`, "not_duplicate_of"},
		{"cost base_revision", func() any { return &roiCostBody{} }, `{"base_revision":"1"}`, "base_revision"},
		{"cost voided", func() any { return &roiCostBody{} }, `{"voided":"yes"}`, "voided"},
		{"lead qualified", func() any { return &roiLeadBody{} }, `{"qualified":"true"}`, "qualified"},
		{"lead first_seen_at", func() any { return &roiLeadBody{} }, `{"first_seen_at":1}`, "first_seen_at"},
		{"merge target_lead_id", func() any { return &roiMergeBody{} }, `{"target_lead_id":7}`, "target_lead_id"},
		{"merge base_revision", func() any { return &roiMergeBody{} }, `{"base_revision":"2"}`, "base_revision"},
		{"touch paid", func() any { return &roiTouchBody{} }, `{"paid":1}`, "paid"},
		{"touch work_id", func() any { return &roiTouchBody{} }, `{"work_id":1}`, "work_id"},
		{"deal amount", func() any { return &roiDealBody{} }, `{"amount":1}`, "amount"},
		{"deal gross_profit", func() any { return &roiDealBody{} }, `{"gross_profit":300}`, "gross_profit"},
		{"deal cogs", func() any { return &roiDealBody{} }, `{"cogs":700}`, "cogs"},
		{"adjustment amount", func() any { return &roiAdjustmentBody{} }, `{"amount":10}`, "amount"},
		{"adjustment gross_delta", func() any { return &roiAdjustmentBody{} }, `{"gross_delta":3}`, "gross_delta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.JSONRequest("POST", "/", tc.body)
			err := decodeROIBody(httptest.NewRecorder(), req, tc.target())
			fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
			if !ok {
				t.Fatalf("err = %v, want a FieldError naming %q", err, tc.field)
			}
			if fieldErr.Field != tc.field {
				t.Fatalf("named %q, want %q", fieldErr.Field, tc.field)
			}
		})
	}
	for field, want := range map[string]string{
		"CostInput.amount": "amount", "amount": "amount",
		"roiRevisionFields.base_revision": "base_revision",
		"Outer.inner.value":               "inner.value", "Only": "",
	} {
		if got := jsonFieldPath(field); got != want {
			t.Errorf("jsonFieldPath(%q) = %q, want %q", field, got, want)
		}
	}
}
