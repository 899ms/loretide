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
