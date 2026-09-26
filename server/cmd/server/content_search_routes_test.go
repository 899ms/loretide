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

// Platform search optimization (specs/036 PR 1): search themes under
// /api/content-search (T022, T026; FR-107).
// PR 2 adds suggestions, comparison and abandoning (T046, T050).

var contentSearchRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-search/themes", "/api/content-search/themes"},
	{http.MethodPost, "/api/content-search/themes", "/api/content-search/themes"},
	{http.MethodGet, "/api/content-search/themes/{themeId}", "/api/content-search/themes/theme-1"},
	{http.MethodGet, "/api/content-search/themes/{themeId}/revisions", "/api/content-search/themes/theme-1/revisions"},
	{http.MethodPost, "/api/content-search/themes/{themeId}/revisions", "/api/content-search/themes/theme-1/revisions"},
	// PR 2: suggestions, comparison and abandoning.
	{http.MethodGet, "/api/content-search/suggestions", "/api/content-search/suggestions"},
	{http.MethodPost, "/api/content-search/suggestions", "/api/content-search/suggestions"},
	{http.MethodGet, "/api/content-search/suggestions/compare", "/api/content-search/suggestions/compare?ids=a,b"},
	{http.MethodGet, "/api/content-search/suggestions/{suggestionId}", "/api/content-search/suggestions/sug-1"},
	{http.MethodPost, "/api/content-search/suggestions/{suggestionId}/revisions", "/api/content-search/suggestions/sug-1/revisions"},
	{http.MethodPost, "/api/content-search/suggestions/{suggestionId}/decisions", "/api/content-search/suggestions/sug-1/decisions"},
	{http.MethodPost, "/api/content-search/suggestions/decisions/{decisionId}/retry", "/api/content-search/suggestions/decisions/decision-1/retry"},
}

// T026 / FR-107: every PR 1 endpoint is mounted, and - a theme changing only
// by a new revision - no route rewrites or removes one.
func TestContentSearchEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSearchRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	for route := range mounted {
		for _, verb := range []string{http.MethodDelete, http.MethodPatch, http.MethodPut} {
			if strings.HasPrefix(route, verb+" /api/content-search/") {
				t.Errorf("a %s route exists on append-only search themes: %s", verb, route)
			}
		}
	}
}

func TestContentSearchEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentSearchRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentSearchEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-search-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Search Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Search Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSearchRoutes {
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

// T022 / FR-107: the three {themeId} endpoints, through the real router and
// middleware, with a server-chosen theme id that is not the workspace id.
// Every answer has to be about the theme in the path, read with
// chi.URLParam; a theme that does not exist answers 404 on all three, and
// the same as another missing theme.
func TestContentSearchThemePathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_search_theme_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	body := `{"name":"羊绒大衣怎么洗","platform":"xiaohongshu","keywords":["羊绒"],"intent":"solve","origin":"manual_keyword"}`
	themeID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-search/themes", body, http.StatusCreated), "theme_id")
	if themeID == testWorkspaceID {
		t.Fatal("the theme id equals the workspace id; the two cannot be told apart")
	}
	second := roiAPI(t, http.MethodPost, "/api/content-search/themes/"+themeID+"/revisions",
		strings.Replace(body, `"name":"羊绒大衣怎么洗"`, `"base_revision":1,"name":"羊绒大衣护理"`, 1), http.StatusCreated)
	if second["theme_id"] != themeID || second["revision"] != float64(2) || second["name"] != "羊绒大衣护理" {
		t.Fatalf("revise answered %v %v %v", second["theme_id"], second["revision"], second["name"])
	}
	current := roiAPI(t, http.MethodGet, "/api/content-search/themes/"+themeID, "", http.StatusOK)
	if current["theme_id"] != themeID || current["revision"] != float64(2) {
		t.Fatalf("get answered %v %v", current["theme_id"], current["revision"])
	}
	revisions := roiAPI(t, http.MethodGet, "/api/content-search/themes/"+themeID+"/revisions", "", http.StatusOK)
	if listed, _ := revisions["revisions"].([]any); revisions["theme_id"] != themeID || len(listed) != 2 {
		t.Fatalf("revisions answered %v", revisions)
	}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/content-search/themes/no-such-theme", ""},
		{http.MethodGet, "/api/content-search/themes/no-such-theme/revisions", ""},
		{http.MethodPost, "/api/content-search/themes/no-such-theme/revisions", `{"base_revision":1}`},
	} {
		missing := accountAPIRequest(t, tc.method, tc.path, tc.body)
		other := accountAPIRequest(t, http.MethodGet, "/api/content-search/themes/another-missing-theme", "")
		if missing.StatusCode != http.StatusNotFound || other.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", tc.method, tc.path, missing.StatusCode)
		}
		if !refusalsMatchApartFromTrace(t, missing, other) {
			t.Errorf("%s %s answers differently from another missing theme", tc.method, tc.path)
		}
		missing.Body.Close()
		other.Body.Close()
	}
}

// T046 / FR-107: /suggestions/compare is a static segment and is matched
// before {suggestionId}: a comparison is never read as a suggestion called
// "compare".
func TestContentSearchCompareIsNotASuggestionID(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for path, want := range map[string]string{
		"/api/content-search/suggestions/compare":                    "/api/content-search/suggestions/compare",
		"/api/content-search/suggestions/sug-1":                      "/api/content-search/suggestions/{suggestionId}",
		"/api/content-search/suggestions/compar":                     "/api/content-search/suggestions/{suggestionId}",
		"/api/content-search/suggestions/decisions/decision-1/retry": "/api/content-search/suggestions/decisions/{decisionId}/retry",
	} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/retry") {
			method = http.MethodPost
		}
		if got := router.Find(chi.NewRouteContext(), method, path); got != want {
			t.Errorf("%s %s matches %q, want %q", method, path, got, want)
		}
	}
}

// T046 / FR-107: the three {suggestionId} endpoints, through the real router
// and middleware, with a server-chosen suggestion id that is not the
// workspace id. Every answer has to be about the suggestion in the path,
// read with chi.URLParam; one that does not exist answers 404 on all three,
// the same as another missing one. The comparison reaches its own handler:
// one id is 400 naming ids, not the 404 a suggestion called "compare" gets.
func TestContentSearchSuggestionPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_search_suggestion_decision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_search_suggestion_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_search_theme_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)
	suffix := fmt.Sprint(time.Now().UnixNano())
	work, artifact, version := "work-sug-"+suffix, "artifact-sug-"+suffix, "version-sug-"+suffix
	fx.Exec(t, `INSERT INTO content_work (work_id, workspace_id, topic_card_id, snapshot_id, title)
		VALUES ($1, $2, '', '', '羊绒护理指南')`, work, testWorkspaceID)
	fx.Exec(t, `INSERT INTO content_artifact (artifact_id, work_id, workspace_id, kind, title, position)
		VALUES ($1, $2, $3, 'channel_draft', '小红书稿', 1)`, artifact, work, testWorkspaceID)
	fx.Exec(t, `INSERT INTO content_artifact_version (version_id, artifact_id, work_id, workspace_id,
		revision, source, action, body, actor_id) VALUES ($1, $2, $3, $4, 1, 'edited', 'saved', '原稿', $5)`,
		version, artifact, work, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE version_id=$1`, version)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE artifact_id=$1`, artifact)
	fx.Cleanup(t, `DELETE FROM content_work WHERE work_id=$1`, work)

	themeID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-search/themes",
		`{"name":"羊绒大衣怎么洗","platform":"xiaohongshu","keywords":["羊绒"],"intent":"solve","origin":"manual_keyword"}`,
		http.StatusCreated), "theme_id")
	body := fmt.Sprintf(`{"work_id":%q,"artifact_id":%q,"base_version_id":%q,"theme_id":%q,
		"target_question":"羊绒大衣能机洗吗","aspects":["body"],"rationale":"依据","proposed_body":"改过的稿"}`,
		work, artifact, version, themeID)
	suggestionID := roiString(t, roiAPI(t, http.MethodPost, "/api/content-search/suggestions", body, http.StatusCreated), "suggestion_id")
	if suggestionID == testWorkspaceID {
		t.Fatal("the suggestion id equals the workspace id; the two cannot be told apart")
	}
	current := roiAPI(t, http.MethodGet, "/api/content-search/suggestions/"+suggestionID, "", http.StatusOK)
	if current["suggestion_id"] != suggestionID || current["revision"] != float64(1) {
		t.Fatalf("get answered %v %v", current["suggestion_id"], current["revision"])
	}
	revised := roiAPI(t, http.MethodPost, "/api/content-search/suggestions/"+suggestionID+"/revisions",
		strings.Replace(body, `"rationale":"依据"`, `"base_revision":1,"rationale":"补充依据"`, 1), http.StatusCreated)
	if revised["suggestion_id"] != suggestionID || revised["revision"] != float64(2) || revised["rationale"] != "补充依据" {
		t.Fatalf("revise answered %v %v %v", revised["suggestion_id"], revised["revision"], revised["rationale"])
	}
	decided := roiAPI(t, http.MethodPost, "/api/content-search/suggestions/"+suggestionID+"/decisions",
		`{"decision":"abandon","revision":2}`, http.StatusCreated)
	if decided["suggestion_id"] != suggestionID || decided["state"] != "abandoned" {
		t.Fatalf("decide answered %v %v", decided["suggestion_id"], decided["state"])
	}
	compare := roiAPI(t, http.MethodGet, "/api/content-search/suggestions/compare?ids="+suggestionID, "", http.StatusBadRequest)
	if compare["field"] != "ids" {
		t.Fatalf("compare with one id answered %v, want a 400 naming ids", compare)
	}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/content-search/suggestions/no-such-suggestion", ""},
		{http.MethodPost, "/api/content-search/suggestions/no-such-suggestion/revisions", `{"base_revision":1}`},
		{http.MethodPost, "/api/content-search/suggestions/no-such-suggestion/decisions", `{"decision":"abandon","revision":1}`},
	} {
		missing := accountAPIRequest(t, tc.method, tc.path, tc.body)
		other := accountAPIRequest(t, http.MethodGet, "/api/content-search/suggestions/another-missing-suggestion", "")
		if missing.StatusCode != http.StatusNotFound || other.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", tc.method, tc.path, missing.StatusCode)
		}
		if !refusalsMatchApartFromTrace(t, missing, other) {
			t.Errorf("%s %s answers differently from another missing suggestion", tc.method, tc.path)
		}
		missing.Body.Close()
		other.Body.Close()
	}
}
