package main

import (
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
