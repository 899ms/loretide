package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Costs, leads, touches, deals and refunds/adjustments (specs/034 PR 1).

var contentROIRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-roi/costs", "/api/content-roi/costs"},
	{http.MethodPost, "/api/content-roi/costs", "/api/content-roi/costs"},
	{http.MethodGet, "/api/content-roi/costs/{costId}", "/api/content-roi/costs/cost-1"},
	{http.MethodPost, "/api/content-roi/costs/{costId}/revisions", "/api/content-roi/costs/cost-1/revisions"},
	{http.MethodGet, "/api/content-roi/leads", "/api/content-roi/leads"},
	{http.MethodPost, "/api/content-roi/leads", "/api/content-roi/leads"},
	{http.MethodGet, "/api/content-roi/leads/{leadId}", "/api/content-roi/leads/lead-1"},
	{http.MethodPost, "/api/content-roi/leads/{leadId}/revisions", "/api/content-roi/leads/lead-1/revisions"},
	{http.MethodPost, "/api/content-roi/leads/{leadId}/merge", "/api/content-roi/leads/lead-1/merge"},
	{http.MethodPost, "/api/content-roi/leads/{leadId}/touches", "/api/content-roi/leads/lead-1/touches"},
	{http.MethodPost, "/api/content-roi/leads/{leadId}/touches/{touchId}/revisions", "/api/content-roi/leads/lead-1/touches/touch-1/revisions"},
	{http.MethodGet, "/api/content-roi/deals", "/api/content-roi/deals"},
	{http.MethodPost, "/api/content-roi/deals", "/api/content-roi/deals"},
	{http.MethodGet, "/api/content-roi/deals/{dealId}", "/api/content-roi/deals/deal-1"},
	{http.MethodPost, "/api/content-roi/deals/{dealId}/revisions", "/api/content-roi/deals/deal-1/revisions"},
	{http.MethodPost, "/api/content-roi/deals/{dealId}/adjustments", "/api/content-roi/deals/deal-1/adjustments"},
	{http.MethodPost, "/api/content-roi/deals/{dealId}/adjustments/{adjustmentId}/revisions", "/api/content-roi/deals/deal-1/adjustments/adj-1/revisions"},
	// PR 2.
	{http.MethodPost, "/api/content-roi/deals/{dealId}/attribution", "/api/content-roi/deals/deal-1/attribution"},
	{http.MethodPost, "/api/content-roi/preview", "/api/content-roi/preview"},
	// PR 3.
	{http.MethodPost, "/api/content-roi/imports", "/api/content-roi/imports"},
	{http.MethodGet, "/api/content-roi/imports", "/api/content-roi/imports"},
	{http.MethodGet, "/api/content-roi/imports/{batchId}", "/api/content-roi/imports/batch-1"},
}

// T032 / FR-074: every PR 1 and PR 2 endpoint is mounted, and - every record being
// append-only - no route can rewrite or remove one.
func TestContentROIEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentROIRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	for route := range mounted {
		for _, verb := range []string{http.MethodDelete, http.MethodPatch, http.MethodPut} {
			if strings.HasPrefix(route, verb+" /api/content-roi/") {
				t.Errorf("a %s route exists on append-only ROI records: %s", verb, route)
			}
		}
	}
}

func TestContentROIEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentROIRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentROIEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-roi-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content ROI Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content ROI Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentROIRoutes {
		req, reqErr := http.NewRequest(route.method, testServer.URL+route.path, strings.NewReader("{}"))
		if reqErr != nil {
			t.Fatal(reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		response, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			t.Fatal(doErr)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s as outsider = %d, want 404", route.method, route.path, response.StatusCode)
		}
	}
}

func roiAPI(t *testing.T, method, path, body string, want int) map[string]any {
	t.Helper()
	response := accountAPIRequest(t, method, path, body)
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, response.StatusCode, want, raw)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return decoded
}

func roiString(t *testing.T, body map[string]any, key string) string {
	t.Helper()
	value, _ := body[key].(string)
	if value == "" {
		t.Fatalf("no %s in %v", key, body)
	}
	return value
}

// T029 / T047 workflow step 12: each of the twelve path-parameter endpoints,
// through the real router and middleware, with server-chosen ids that are
// not the workspace id. Every answer has to be about the id in the path; a
// handler that read the context where it meant chi.URLParam would answer
// about the wrong thing or not at all. An id from another workspace answers
// exactly like one that does not exist.
func TestContentROIPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	for _, table := range []string{
		"content_roi_cost_revision", "content_roi_lead_revision", "content_roi_touch_revision",
		"content_roi_deal_revision", "content_roi_adjustment_revision",
		"content_roi_cost_allocation", "content_roi_attribution_revision",
	} {
		fx.Cleanup(t, `DELETE FROM `+table+` WHERE workspace_id=$1`, testWorkspaceID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	stamp := time.Now().UnixNano()
	costID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/costs",
		fmt.Sprintf(`{"category":"拍摄-%d","pricing":"amount","amount":"3000.00","currency":"CNY",
			"incurred_at":"2026-09-10T02:00:00Z"}`, stamp), http.StatusCreated), "cost_id")
	leadID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/leads",
		`{"customer_ref":"","stage":"咨询","first_seen_at":"2026-09-10T02:00:00Z"}`, http.StatusCreated), "lead_id")
	targetLead := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/leads",
		`{"customer_ref":"","stage":"预约","first_seen_at":"2026-09-10T02:00:00Z"}`, http.StatusCreated), "lead_id")
	touchID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/leads/"+leadID+"/touches",
		`{"evidence_type":"unknown","role":"first_touch","occurred_at":"2026-09-10T03:00:00Z"}`, http.StatusCreated), "touch_id")
	dealID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/deals",
		fmt.Sprintf(`{"order_ref":"TB-%d","amount":"10000.00","currency":"CNY",
			"closed_at":"2026-09-12T02:00:00Z","gross_basis":"none"}`, stamp), http.StatusCreated), "deal_id")
	adjustmentID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-roi/deals/"+dealID+"/adjustments",
		`{"kind":"refund","amount":"100.00","currency":"CNY","occurred_at":"2026-09-20T02:00:00Z"}`,
		http.StatusCreated), "adjustment_id")
	for _, id := range []string{costID, leadID, touchID, dealID, adjustmentID} {
		if id == testWorkspaceID {
			t.Fatalf("an id equals the workspace id; the two cannot be told apart")
		}
	}

	cases := []struct {
		name, method, path, body, idKey, want string
	}{
		{"get-cost", http.MethodGet, "/api/content-roi/costs/" + costID, "", "cost_id", costID},
		{"revise-cost", http.MethodPost, "/api/content-roi/costs/" + costID + "/revisions",
			fmt.Sprintf(`{"category":"拍摄-%d","pricing":"amount","amount":"3100.00","currency":"CNY",
				"incurred_at":"2026-09-10T02:00:00Z","base_revision":1}`, stamp), "cost_id", costID},
		{"get-lead", http.MethodGet, "/api/content-roi/leads/" + leadID, "", "lead_id", leadID},
		{"revise-lead", http.MethodPost, "/api/content-roi/leads/" + leadID + "/revisions",
			`{"customer_ref":"","stage":"预约","first_seen_at":"2026-09-10T02:00:00Z","base_revision":1}`, "lead_id", leadID},
		{"add-touch", http.MethodPost, "/api/content-roi/leads/" + leadID + "/touches",
			`{"evidence_type":"unknown","role":"other","occurred_at":"2026-09-10T04:00:00Z"}`, "lead_id", leadID},
		{"revise-touch", http.MethodPost, "/api/content-roi/leads/" + leadID + "/touches/" + touchID + "/revisions",
			`{"evidence_type":"unknown","role":"pre_booking","occurred_at":"2026-09-10T03:00:00Z","base_revision":1}`, "touch_id", touchID},
		{"merge-lead", http.MethodPost, "/api/content-roi/leads/" + leadID + "/merge",
			fmt.Sprintf(`{"target_lead_id":%q,"note":"同一人","base_revision":2}`, targetLead), "lead_id", leadID},
		{"get-deal", http.MethodGet, "/api/content-roi/deals/" + dealID, "", "deal_id", dealID},
		{"revise-deal", http.MethodPost, "/api/content-roi/deals/" + dealID + "/revisions",
			fmt.Sprintf(`{"order_ref":"TB-%d","amount":"10000.00","currency":"CNY",
				"closed_at":"2026-09-12T02:00:00Z","gross_basis":"none","base_revision":1}`, stamp), "deal_id", dealID},
		{"add-adjustment", http.MethodPost, "/api/content-roi/deals/" + dealID + "/adjustments",
			`{"kind":"refund","amount":"1.00","currency":"CNY","occurred_at":"2026-09-21T02:00:00Z"}`, "deal_id", dealID},
		{"revise-adjustment", http.MethodPost, "/api/content-roi/deals/" + dealID + "/adjustments/" + adjustmentID + "/revisions",
			`{"kind":"refund","amount":"200.00","currency":"CNY","occurred_at":"2026-09-20T02:00:00Z","base_revision":1}`,
			"adjustment_id", adjustmentID},
		// PR 2: the judgement answers for the deal in the path.
		{"record-attribution", http.MethodPost, "/api/content-roi/deals/" + dealID + "/attribution",
			`{"judgement":"unknown","touch_ids":[],"base_revision":0}`, "deal_id", dealID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := http.StatusCreated
			if tc.method == http.MethodGet {
				want = http.StatusOK
			}
			body := roiAPI(t, tc.method, tc.path, tc.body, want)
			if got := body[tc.idKey]; got != tc.want {
				t.Fatalf("path named %s=%q, response answered for %v", tc.idKey, tc.want, got)
			}
			if workspace, ok := body["workspace_id"]; ok && workspace != testWorkspaceID {
				t.Errorf("the row landed in workspace %v", workspace)
			}
		})
	}

	// A cost from another workspace and one that does not exist answer
	// identically.
	foreignCost := fmt.Sprintf("cost-foreign-%d", stamp)
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_roi_cost_revision
		(workspace_id, cost_id, revision, category, pricing, amount_minor, currency,
		 incurred_at, source_type, recorded_by)
		VALUES ($1,$2,1,'拍摄','amount',100,'CNY',now(),'manual','someone')`,
		fmt.Sprintf("ws-foreign-%d", stamp), foreignCost); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_roi_cost_revision WHERE cost_id=$1`, foreignCost)
	foreign := accountAPIRequest(t, http.MethodGet, "/api/content-roi/costs/"+foreignCost, "")
	defer foreign.Body.Close()
	missing := accountAPIRequest(t, http.MethodGet, "/api/content-roi/costs/no-such-cost", "")
	defer missing.Body.Close()
	if foreign.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign = %d, missing = %d, want 404 and 404", foreign.StatusCode, missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}
}
