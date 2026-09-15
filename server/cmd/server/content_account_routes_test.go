package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The gap this file exists to close.
//
// #71 delivered the account handlers and #73 the persona revision handlers,
// each with its own handler tests, and both merged green. Neither PR mounted
// them on the router, and nothing noticed: a handler test calls the function
// directly, so it passes exactly the same whether or not a URL reaches it.
// Until LT-013 there was no caller, so the endpoints were unreachable in a
// running server and no test said so.
//
// These cases assert the property those handler tests structurally cannot:
// that the eight endpoints are on the route table, and that they sit inside
// the workspace-member group rather than being exposed.

// contentAccountRoutes is every endpoint LT-011 and LT-012 delivered. The two
// list-and-detail persona routes are here even though the LT-013 page uses
// only six of the eight: they belong to the same resource, and mounting them
// in a later card means editing router.go a second time for no reason.
var contentAccountRoutes = []struct {
	method  string
	pattern string
	// pattern is chi's own spelling as Walk reports it, trailing slash and
	// all; path is a concrete URL, used for the rejection cases.
	path string
}{
	{http.MethodGet, "/api/content-accounts/", "/api/content-accounts"},
	{http.MethodPost, "/api/content-accounts/", "/api/content-accounts"},
	{http.MethodGet, "/api/content-accounts/{id}/", "/api/content-accounts/acct-1"},
	{http.MethodPatch, "/api/content-accounts/{id}/", "/api/content-accounts/acct-1"},
	{http.MethodGet, "/api/content-accounts/{id}/persona", "/api/content-accounts/acct-1/persona"},
	{http.MethodPost, "/api/content-accounts/{id}/persona", "/api/content-accounts/acct-1/persona"},
	{http.MethodGet, "/api/content-accounts/{id}/persona/revisions", "/api/content-accounts/acct-1/persona/revisions"},
	{http.MethodGet, "/api/content-accounts/{id}/persona/{revisionId}", "/api/content-accounts/acct-1/persona/rev-1"},
}

func TestContentAccountEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)

	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	for _, route := range contentAccountRoutes {
		key := route.method + " " + route.pattern
		if !mounted[key] {
			t.Errorf("%s is not on the route table; the handler exists but nothing reaches it", key)
		}
	}
}

// Unauthenticated callers are turned away before the handler. A 404 here would
// be ambiguous — it could mean "no such route" — so this asserts the status the
// auth middleware produces, and separately that the route exists above.
func TestContentAccountEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)

	for _, route := range contentAccountRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want %d — an unauthenticated caller must be stopped by middleware, not reach the handler",
				route.method, route.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

// A signed-in user who is not a member of the workspace must be refused by
// RequireWorkspaceMember. This is what proves the routes were mounted INSIDE
// that group: mounting them one block higher would let this request through to
// the handler, and the handler's own workspacecore check would then be the only
// thing standing between an outsider and the brand's accounts.
func TestContentAccountEndpointsRejectNonMembers(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)

	email := fmt.Sprintf("content-account-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Account Outsider", email)
	// Deliberately NOT fx.Member: this user exists and can authenticate, but
	// belongs to no workspace.
	outsiderJWT, err := generateTestJWT(outsider, email, "Content Account Outsider")
	if err != nil {
		t.Fatalf("outsider jwt: %v", err)
	}

	for _, route := range contentAccountRoutes {
		req, err := http.NewRequest(route.method, testServer.URL+route.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+outsiderJWT)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Workspace-ID", testWorkspaceID)

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", route.method, route.path, err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()

		if response.StatusCode < 400 {
			t.Errorf("%s %s as a non-member = %d; a caller outside the workspace must be refused",
				route.method, route.path, response.StatusCode)
		}
		// Without this the case is vacuous: an unmounted route answers 404,
		// which is also >= 400, so the assertion above would pass while
		// nothing was being refused. chi's own no-route reply is the plain
		// text below; every middleware refusal is a JSON body.
		if strings.Contains(string(body), "404 page not found") {
			t.Errorf("%s %s answered chi's no-route 404, so nothing refused it — the route is not mounted",
				route.method, route.path)
		}
	}
}

// The routes that were already there stay there.
//
// Mounting the account group edits router.go, which is upstream Multica code,
// so this case exists to show the edit was additive. It names the routes
// registered around the insertion point - the ones a mis-scoped `r.Route` or a
// stray closing brace would swallow - plus the group the accounts sit beside.
// A sampled list rather than a route count on purpose: a count would have to be
// bumped by every unrelated PR that adds an endpoint, which is a tax on
// everyone else to protect one change of mine.
func TestMountingContentAccountsLeftNeighbouringRoutesAlone(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)

	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	for _, route := range []string{
		// Registered immediately after the inserted block.
		"GET /api/assignee-frequency",
		// The workspace-scoped group the accounts were mounted beside.
		"GET /api/content-diagnostics/overview",
		"GET /api/content-diagnostics/runs",
		"GET /api/content-diagnostics/events",
		"GET /api/content-diagnostics/stream",
		"POST /api/content-diagnostics/simulate",
		"POST /api/content-diagnostics/client",
		// Further down the same group, so a swallowed closing brace shows up.
		"GET /api/issues/search",
		"GET /api/issues/grouped",
	} {
		if !mounted[route] {
			t.Errorf("%s disappeared from the route table; mounting the account group was supposed to add routes, not move any", route)
		}
	}
}
