package main

import (
	"encoding/json"
	"fmt"
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

// Brand/account operating diagnosis (specs/035 PR 1 and PR 2): report versions,
// work marks and the preview under /api/content-operating-diagnosis.

var contentOpDiagRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-operating-diagnosis/reports", "/api/content-operating-diagnosis/reports"},
	{http.MethodPost, "/api/content-operating-diagnosis/reports", "/api/content-operating-diagnosis/reports"},
	{http.MethodGet, "/api/content-operating-diagnosis/reports/{reportId}/versions", "/api/content-operating-diagnosis/reports/report-1/versions"},
	{http.MethodPost, "/api/content-operating-diagnosis/reports/{reportId}/versions", "/api/content-operating-diagnosis/reports/report-1/versions"},
	{http.MethodGet, "/api/content-operating-diagnosis/reports/{reportId}/versions/{versionNo}", "/api/content-operating-diagnosis/reports/report-1/versions/1"},
	{http.MethodGet, "/api/content-operating-diagnosis/work-marks", "/api/content-operating-diagnosis/work-marks"},
	{http.MethodPost, "/api/content-operating-diagnosis/work-marks", "/api/content-operating-diagnosis/work-marks"},
	// PR 2 (T052): compute without saving.
	{http.MethodPost, "/api/content-operating-diagnosis/preview", "/api/content-operating-diagnosis/preview"},
	// PR 3 (T080): judgements, suggestions, decisions, proposals, todos.
	{http.MethodGet, "/api/content-operating-diagnosis/reports/{reportId}/versions/{versionNo}/annotations", "/api/content-operating-diagnosis/reports/report-1/versions/1/annotations"},
	{http.MethodPost, "/api/content-operating-diagnosis/reports/{reportId}/versions/{versionNo}/judgements", "/api/content-operating-diagnosis/reports/report-1/versions/1/judgements"},
	{http.MethodPost, "/api/content-operating-diagnosis/judgements/{judgementId}/revisions", "/api/content-operating-diagnosis/judgements/judgement-1/revisions"},
	{http.MethodPost, "/api/content-operating-diagnosis/reports/{reportId}/versions/{versionNo}/suggestions", "/api/content-operating-diagnosis/reports/report-1/versions/1/suggestions"},
	{http.MethodPost, "/api/content-operating-diagnosis/suggestions/{suggestionId}/revisions", "/api/content-operating-diagnosis/suggestions/suggestion-1/revisions"},
	{http.MethodPost, "/api/content-operating-diagnosis/suggestions/{suggestionId}/decisions", "/api/content-operating-diagnosis/suggestions/suggestion-1/decisions"},
	{http.MethodPost, "/api/content-operating-diagnosis/decisions/{decisionId}/retry", "/api/content-operating-diagnosis/decisions/decision-1/retry"},
	{http.MethodGet, "/api/content-operating-diagnosis/profile-proposals", "/api/content-operating-diagnosis/profile-proposals"},
	{http.MethodPost, "/api/content-operating-diagnosis/profile-proposals/{proposalId}/confirm", "/api/content-operating-diagnosis/profile-proposals/proposal-1/confirm"},
	{http.MethodPost, "/api/content-operating-diagnosis/profile-proposals/{proposalId}/dismiss", "/api/content-operating-diagnosis/profile-proposals/proposal-1/dismiss"},
	{http.MethodGet, "/api/content-operating-diagnosis/todos", "/api/content-operating-diagnosis/todos"},
	{http.MethodPost, "/api/content-operating-diagnosis/todos", "/api/content-operating-diagnosis/todos"},
	{http.MethodPost, "/api/content-operating-diagnosis/todos/{todoId}/revisions", "/api/content-operating-diagnosis/todos/todo-1/revisions"},
}

// T031 / FR-088: every PR 1 endpoint is mounted, and - both tables being
// append-only - no route can rewrite or remove anything. The route prefix is
// not the development diagnostics' (FR-092).
func TestContentOpDiagEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentOpDiagRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
		if strings.Contains(route.pattern, "/diagnostics") {
			t.Errorf("%s shares the development diagnostics' name", route.pattern)
		}
	}
	for route := range mounted {
		for _, verb := range []string{http.MethodDelete, http.MethodPatch, http.MethodPut} {
			if strings.HasPrefix(route, verb+" /api/content-operating-diagnosis/") {
				t.Errorf("a %s route exists on append-only diagnosis records: %s", verb, route)
			}
		}
	}
}

func TestContentOpDiagEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentOpDiagRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentOpDiagEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-opdiag-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Opdiag Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Opdiag Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentOpDiagRoutes {
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

// T028 workflow step 12: the three endpoints with {reportId} and
// {versionNo}, through the real router and middleware, with a server-chosen
// report id that is not the workspace id. Every answer has to be about the
// report and version in the path, read with chi.URLParam; a version number
// that is not a number and a report that does not exist answer exactly
// alike.
func TestContentOpDiagReportPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	accountID := fmt.Sprintf("acct-opdiag-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_account (account_id, workspace_id, platform, display_name)
		VALUES ($1, $2, 'xiaohongshu', '品牌主号')`, accountID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id=$1`, accountID)
	fx.Cleanup(t, `DELETE FROM content_opdiag_report_version WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	params := fmt.Sprintf(`{"scope":{"kind":"account","account_ids":[%q]},
		"window":{"start":"2026-09-01","end":"2026-09-30"},"dimensions":[]}`, accountID)
	reportID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-operating-diagnosis/reports",
		`{"title":"step 12","params":`+params+`}`, http.StatusCreated), "report_id")
	if reportID == testWorkspaceID {
		t.Fatal("the report id equals the workspace id; the two cannot be told apart")
	}

	second := roiAPI(t, http.MethodPost, "/api/content-operating-diagnosis/reports/"+reportID+"/versions", `{}`, http.StatusCreated)
	if second["report_id"] != reportID || second["version_no"] != float64(2) {
		t.Fatalf("generate-version answered %v %v", second["report_id"], second["version_no"])
	}
	versions := roiAPI(t, http.MethodGet, "/api/content-operating-diagnosis/reports/"+reportID+"/versions", "", http.StatusOK)
	if listed, _ := versions["versions"].([]any); versions["report_id"] != reportID || len(listed) != 2 {
		t.Fatalf("list-versions answered %v", versions)
	}
	for _, versionNo := range []string{"1", "2"} {
		version := roiAPI(t, http.MethodGet, "/api/content-operating-diagnosis/reports/"+reportID+"/versions/"+versionNo, "", http.StatusOK)
		if version["report_id"] != reportID || fmt.Sprint(version["version_no"]) != versionNo {
			t.Fatalf("get-version %s answered %v %v", versionNo, version["report_id"], version["version_no"])
		}
	}

	notANumber := accountAPIRequest(t, http.MethodGet, "/api/content-operating-diagnosis/reports/"+reportID+"/versions/abc", "")
	defer notANumber.Body.Close()
	missing := accountAPIRequest(t, http.MethodGet, "/api/content-operating-diagnosis/reports/no-such-report/versions/1", "")
	defer missing.Body.Close()
	if notANumber.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("not a number = %d, missing = %d, want 404 and 404", notANumber.StatusCode, missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, notANumber, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}
	missingVersions := accountAPIRequest(t, http.MethodGet, "/api/content-operating-diagnosis/reports/no-such-report/versions", "")
	defer missingVersions.Body.Close()
	missingNext := accountAPIRequest(t, http.MethodPost, "/api/content-operating-diagnosis/reports/no-such-report/versions", "{}")
	defer missingNext.Body.Close()
	if missingVersions.StatusCode != http.StatusNotFound || missingNext.StatusCode != http.StatusNotFound {
		t.Fatalf("missing report: list = %d, generate = %d, want 404 and 404", missingVersions.StatusCode, missingNext.StatusCode)
	}
}

// T077 workflow step 12 (PR 3): every PR 3 endpoint with a path parameter,
// through the real router and middleware, with server-chosen ids that are
// not the workspace id. Each answer is about the id in the path - read with
// chi.URLParam - and an id that is not here answers exactly like any other
// missing record.
func TestContentOpDiagDecisionPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	accountID := fmt.Sprintf("acct-opdiag-pr3-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_account (account_id, workspace_id, platform, display_name)
		VALUES ($1, $2, 'xiaohongshu', '品牌主号')`, accountID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id=$1`, accountID)
	fx.Cleanup(t, `DELETE FROM content_account_revision WHERE account_id=$1`, accountID)
	for _, table := range []string{
		"content_opdiag_report_version", "content_opdiag_judgement_revision", "content_opdiag_suggestion_revision",
		"content_opdiag_decision", "content_opdiag_effect", "content_opdiag_profile_proposal_revision",
		"content_opdiag_todo_revision", "content_operation_audit",
	} {
		fx.Cleanup(t, `DELETE FROM `+table+` WHERE workspace_id=$1`, testWorkspaceID)
	}
	const base = "/api/content-operating-diagnosis"

	params := fmt.Sprintf(`{"scope":{"kind":"account","account_ids":[%q]},
		"window":{"start":"2026-09-01","end":"2026-09-30"},"dimensions":[]}`, accountID)
	reportID := roiString(t, roiAPI(t, http.MethodPost, base+"/reports", `{"title":"pr3 step 12","params":`+params+`}`,
		http.StatusCreated), "report_id")
	if reportID == testWorkspaceID {
		t.Fatal("the report id equals the workspace id; the two cannot be told apart")
	}
	version := roiAPI(t, http.MethodGet, base+"/reports/"+reportID+"/versions/1", "", http.StatusOK)
	result, _ := version["result"].(map[string]any)
	gaps, _ := result["gaps"].([]any)
	if len(gaps) == 0 {
		t.Fatalf("a scope-only report of an account with no profile has no gap: %v", result)
	}
	gapKey, _ := gaps[0].(map[string]any)["gap_key"].(string)

	// Judgement: created on the version in the path, revised by the id in
	// the path.
	judgement := roiAPI(t, http.MethodPost, base+"/reports/"+reportID+"/versions/1/judgements",
		`{"kind":"judgement","basis":"evidence","evidence_refs":["scope"],"body":"范围只有一个账号"}`, http.StatusCreated)
	judgementID := roiString(t, judgement, "judgement_id")
	if judgement["report_id"] != reportID || judgement["version_no"] != float64(1) {
		t.Fatalf("judgement landed on %v v%v", judgement["report_id"], judgement["version_no"])
	}
	revised := roiAPI(t, http.MethodPost, base+"/judgements/"+judgementID+"/revisions",
		`{"base_revision":1,"kind":"limitation","basis":"qualitative","evidence_refs":[],"body":"定性"}`, http.StatusCreated)
	if revised["judgement_id"] != judgementID || revised["revision"] != float64(2) {
		t.Fatalf("judgement revision answered %v r%v", revised["judgement_id"], revised["revision"])
	}

	// Suggestions: a todo and a profile proposal, each revised and decided
	// by the id in the path.
	todoSuggestion := roiString(t, roiAPI(t, http.MethodPost, base+"/reports/"+reportID+"/versions/1/suggestions",
		`{"body":"补录配置","target_kind":"todo","target":{"title":"补定位","account_id":""}}`, http.StatusCreated), "suggestion_id")
	next := roiAPI(t, http.MethodPost, base+"/suggestions/"+todoSuggestion+"/revisions",
		`{"base_revision":1,"body":"补录配置项","target_kind":"todo","target":{"title":"补定位","account_id":""}}`, http.StatusCreated)
	if next["suggestion_id"] != todoSuggestion || next["revision"] != float64(2) {
		t.Fatalf("suggestion revision answered %v r%v", next["suggestion_id"], next["revision"])
	}
	decided := roiAPI(t, http.MethodPost, base+"/suggestions/"+todoSuggestion+"/decisions",
		`{"suggestion_revision":2,"decision":"adopt"}`, http.StatusCreated)
	decisionID := roiString(t, decided, "decision_id")
	if decided["suggestion_id"] != todoSuggestion || decided["effect_state"] != "done" {
		t.Fatalf("decision answered %v %v", decided["suggestion_id"], decided["effect_state"])
	}
	// The retry reads the decision in the path: it is a todo adoption, so
	// there is nothing to retry - 409 naming decision_id, not the 404 a lost
	// path parameter would give.
	retry := accountAPIRequest(t, http.MethodPost, base+"/decisions/"+decisionID+"/retry", `{}`)
	var retryBody map[string]any
	_ = json.NewDecoder(retry.Body).Decode(&retryBody)
	retry.Body.Close()
	if retry.StatusCode != http.StatusConflict || retryBody["field"] != "decision_id" {
		t.Fatalf("retry of %s = %d %v, want 409 decision_id", decisionID, retry.StatusCode, retryBody)
	}

	proposalSuggestion := roiString(t, roiAPI(t, http.MethodPost, base+"/reports/"+reportID+"/versions/1/suggestions",
		fmt.Sprintf(`{"body":"改定位","target_kind":"profile_proposal","target":{"account_id":%q,"patches":[{"field":"positioning","value":"面料专家"}]}}`,
			accountID), http.StatusCreated), "suggestion_id")
	adopted := roiAPI(t, http.MethodPost, base+"/suggestions/"+proposalSuggestion+"/decisions",
		`{"suggestion_revision":1,"decision":"adopt"}`, http.StatusCreated)
	effects, _ := adopted["effects"].([]any)
	if len(effects) != 1 {
		t.Fatalf("proposal adoption effects = %v", adopted["effects"])
	}
	proposalID, _ := effects[0].(map[string]any)["target_id"].(string)
	confirmed := roiAPI(t, http.MethodPost, base+"/profile-proposals/"+proposalID+"/confirm", `{"base_revision_id":""}`, http.StatusCreated)
	if confirmed["proposal_id"] != proposalID || confirmed["state"] != "confirmed" || confirmed["applied_revision_id"] == "" {
		t.Fatalf("confirm answered %v", confirmed)
	}
	// A second proposal, dismissed by its own id.
	second := roiString(t, roiAPI(t, http.MethodPost, base+"/reports/"+reportID+"/versions/1/suggestions",
		fmt.Sprintf(`{"body":"改受众","target_kind":"profile_proposal","target":{"account_id":%q,"patches":[{"field":"audience","value":"新手"}]}}`,
			accountID), http.StatusCreated), "suggestion_id")
	secondEffects, _ := roiAPI(t, http.MethodPost, base+"/suggestions/"+second+"/decisions",
		`{"suggestion_revision":1,"decision":"adopt"}`, http.StatusCreated)["effects"].([]any)
	if len(secondEffects) != 1 {
		t.Fatalf("second proposal adoption effects = %v", secondEffects)
	}
	secondProposal, _ := secondEffects[0].(map[string]any)["target_id"].(string)
	dismissed := roiAPI(t, http.MethodPost, base+"/profile-proposals/"+secondProposal+"/dismiss", `{}`, http.StatusCreated)
	if dismissed["proposal_id"] != secondProposal || dismissed["state"] != "dismissed" {
		t.Fatalf("dismiss answered %v", dismissed)
	}

	// Todo from a gap, revised by its id.
	todoID := roiString(t, roiAPI(t, http.MethodPost, base+"/todos",
		fmt.Sprintf(`{"origin_report_id":%q,"origin_version_no":1,"origin_gap_key":%q}`, reportID, gapKey), http.StatusCreated), "todo_id")
	done := roiAPI(t, http.MethodPost, base+"/todos/"+todoID+"/revisions", `{"base_revision":1,"state":"done"}`, http.StatusCreated)
	if done["todo_id"] != todoID || done["state"] != "done" {
		t.Fatalf("todo revision answered %v", done)
	}

	annotations := roiAPI(t, http.MethodGet, base+"/reports/"+reportID+"/versions/1/annotations", "", http.StatusOK)
	if annotations["report_id"] != reportID || annotations["version_no"] != float64(1) {
		t.Fatalf("annotations answered %v v%v", annotations["report_id"], annotations["version_no"])
	}
	if listed, _ := annotations["suggestions"].([]any); len(listed) != 3 {
		t.Fatalf("annotations list %d suggestions, want 3", len(listed))
	}

	// Ids that are not here, and a version number that is not one, answer
	// alike.
	reference := accountAPIRequest(t, http.MethodGet, base+"/reports/no-such-report/versions/1/annotations", "")
	defer reference.Body.Close()
	if reference.StatusCode != http.StatusNotFound {
		t.Fatalf("missing report annotations = %d", reference.StatusCode)
	}
	referenceBody := decodedRefusal(t, reference)
	for _, probe := range []struct{ method, path, body string }{
		{http.MethodGet, base + "/reports/" + reportID + "/versions/abc/annotations", ""},
		{http.MethodPost, base + "/reports/" + reportID + "/versions/9/judgements", `{"kind":"judgement","basis":"qualitative","body":"x"}`},
		{http.MethodPost, base + "/judgements/no-such-judgement/revisions", `{"base_revision":1,"voided":true}`},
		{http.MethodPost, base + "/reports/no-such-report/versions/1/suggestions", `{"body":"x","target_kind":"topic_card","target":{}}`},
		{http.MethodPost, base + "/suggestions/no-such-suggestion/revisions", `{"base_revision":1,"voided":true}`},
		{http.MethodPost, base + "/suggestions/no-such-suggestion/decisions", `{"suggestion_revision":1,"decision":"reject"}`},
		{http.MethodPost, base + "/decisions/no-such-decision/retry", `{}`},
		{http.MethodPost, base + "/profile-proposals/no-such-proposal/confirm", `{"base_revision_id":""}`},
		{http.MethodPost, base + "/profile-proposals/no-such-proposal/dismiss", `{}`},
		{http.MethodPost, base + "/todos/no-such-todo/revisions", `{"base_revision":1,"state":"done"}`},
	} {
		response := accountAPIRequest(t, probe.method, probe.path, probe.body)
		if response.StatusCode != http.StatusNotFound {
			response.Body.Close()
			t.Errorf("%s %s = %d, want 404", probe.method, probe.path, response.StatusCode)
			continue
		}
		if got := decodedRefusal(t, response); got != referenceBody {
			t.Errorf("%s %s answered %s, a missing record answers %s", probe.method, probe.path, got, referenceBody)
		}
		response.Body.Close()
	}
}
