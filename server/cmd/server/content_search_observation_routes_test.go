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

// Search metrics and ranking observations (specs/036 PR 4) under
// /api/content-search (T089, T092; FR-107). Kept beside
// content_search_routes_test.go rather than in it, so the PR 2 and PR 4
// route lists do not edit the same lines.

var contentSearchObservationRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-search/metrics", "/api/content-search/metrics?publication_record_id=p1"},
	{http.MethodPost, "/api/content-search/metrics", "/api/content-search/metrics"},
	{http.MethodGet, "/api/content-search/rank-observations", "/api/content-search/rank-observations?theme_id=t1"},
	{http.MethodPost, "/api/content-search/rank-observations", "/api/content-search/rank-observations"},
	{http.MethodPost, "/api/content-search/rank-observations/{observationId}/revisions",
		"/api/content-search/rank-observations/observation-1/revisions"},
}

// T092 / FR-107: every PR 4 endpoint is mounted. An observation changes only
// by a new revision and a metric never changes: TestContentSearchEndpoints
// AreMounted already refuses any PUT, PATCH or DELETE under the prefix.
func TestContentSearchObservationEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSearchObservationRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	for route := range mounted {
		for _, prefix := range []string{"/api/content-search/metrics/", "/api/content-search/rank-observations/{observationId}"} {
			if strings.Contains(route, prefix) && route != http.MethodPost+" /api/content-search/rank-observations/{observationId}/revisions" {
				t.Errorf("an unexpected search observation route: %s", route)
			}
		}
	}
}

func TestContentSearchObservationEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentSearchObservationRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentSearchObservationEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-search-obs-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Search Observation Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Search Observation Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSearchObservationRoutes {
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

// T089 / FR-107: the one {observationId} endpoint, through the real router
// and middleware, with a server-chosen observation id that is not the
// workspace id. The revision has to be about the observation in the path,
// read with chi.URLParam; an observation that does not exist answers 404,
// the same as another missing one, whatever the body.
func TestContentSearchObservationPathIDSurvivesTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_search_rank_observation_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	body := `{"platform":"douyin","query":"羊绒大衣能机洗吗","observed_at":"2026-10-03T13:30:00Z",
		"conditions":"同事账号，北京，综合排序","result_kind":"not_found","scanned_depth":30,"evidence_note":"截图 1003.png"}`
	created := roiAPI(t, http.MethodPost, "/api/content-search/rank-observations", body, http.StatusCreated)
	observationID := roiString(t, created, "observation_id")
	if observationID == testWorkspaceID {
		t.Fatal("the observation id equals the workspace id; the two cannot be told apart")
	}
	if created["rule"] != "rank.single_observation" || created["revision"] != float64(1) {
		t.Fatalf("created = %v", created)
	}
	revised := roiAPI(t, http.MethodPost, "/api/content-search/rank-observations/"+observationID+"/revisions",
		strings.Replace(body, `"scanned_depth":30`, `"scanned_depth":20,"base_revision":1`, 1), http.StatusCreated)
	if revised["observation_id"] != observationID || revised["revision"] != float64(2) || revised["scanned_depth"] != float64(20) {
		t.Fatalf("revise answered %v %v %v", revised["observation_id"], revised["revision"], revised["scanned_depth"])
	}
	listed := roiAPI(t, http.MethodGet, "/api/content-search/rank-observations?query="+
		"%E7%BE%8A%E7%BB%92%E5%A4%A7%E8%A1%A3%E8%83%BD%E6%9C%BA%E6%B4%97%E5%90%97", "", http.StatusOK)
	if observations, _ := listed["observations"].([]any); len(observations) != 1 {
		t.Fatalf("list answered %v", listed)
	}

	missing := accountAPIRequest(t, http.MethodPost, "/api/content-search/rank-observations/no-such-observation/revisions", `{"rank":1}`)
	other := accountAPIRequest(t, http.MethodPost, "/api/content-search/rank-observations/another-missing-observation/revisions",
		strings.Replace(body, `"scanned_depth":30`, `"scanned_depth":30,"base_revision":1`, 1))
	defer missing.Body.Close()
	defer other.Body.Close()
	if missing.StatusCode != http.StatusNotFound || other.StatusCode != http.StatusNotFound {
		t.Fatalf("missing observations = %d, %d, want 404", missing.StatusCode, other.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, missing, other) {
		t.Error("two missing observations answer differently")
	}
}
