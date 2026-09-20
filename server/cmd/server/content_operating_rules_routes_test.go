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

// The brand's operating rules (specs/029, SOP 3.2).

var contentOperatingRulesRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/operating-rules/", "/api/operating-rules"},
	{http.MethodPut, "/api/operating-rules/", "/api/operating-rules"},
}

func TestContentOperatingRulesEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentOperatingRulesRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	if !mounted[http.MethodPut+" /api/content-accounts/{id}/homepage"] {
		t.Error("PUT /api/content-accounts/{id}/homepage is not mounted")
	}

	// The brand-scoped endpoints carry NO path parameter: which brand is
	// decided by the X-Workspace-ID header. Workflow step 12's "parameter is
	// not the context id" case therefore has no subject on these two, which is
	// a fact worth asserting rather than a step worth skipping silently. The
	// account endpoint below does have one, and does have that test.
	for route := range mounted {
		if strings.Contains(route, "/api/operating-rules") && strings.Contains(route, "{") {
			t.Errorf("a path parameter appeared on a brand-scoped endpoint: %s", route)
		}
	}

	// Settings are edited, never removed: clearing a field is a value, not a
	// deletion, so there is no DELETE for one to be confused with.
	for route := range mounted {
		if strings.HasPrefix(route, http.MethodDelete+" /api/operating-rules") {
			t.Errorf("a DELETE route exists on operating rules: %s", route)
		}
	}
}

func TestContentOperatingRulesEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentOperatingRulesRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
	req := testutil.JSONRequest(http.MethodPut, "/api/content-accounts/acct-1/homepage", `{}`)
	testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
}

// Workflow step 12, for the one endpoint on this card that has a path
// parameter.
//
// Through the REAL router and middleware, with the account id in the path
// deliberately different from the workspace id in the context: an account that
// belongs to another brand must answer exactly the way a missing one does.
// LT-011/012/013 all shipped a version of this and all three needed it.
func TestHomepageAccountIDComesFromThePathNotTheContext(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)

	mine := fmt.Sprintf("acct-home-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_account
		(account_id, workspace_id, platform, display_name, settings)
		VALUES ($1,$2,'xiaohongshu','Mine','{"loretide.scope":"all"}'::jsonb)`,
		mine, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id=$1`, mine)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	if mine == testWorkspaceID {
		t.Fatal("the account id equals the workspace id; the two cannot be told apart")
	}

	link := "https://www.xiaohongshu.com/user/profile/route-test"
	saved := accountAPIRequest(t, http.MethodPut,
		"/api/content-accounts/"+mine+"/homepage",
		fmt.Sprintf(`{"homepage":%q}`, link))
	defer saved.Body.Close()
	if saved.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(saved.Body)
		t.Fatalf("save homepage = %d, want 200: %s", saved.StatusCode, body)
	}
	var echoed struct {
		Homepage string `json:"homepage"`
	}
	if err := json.NewDecoder(saved.Body).Decode(&echoed); err != nil {
		t.Fatal(err)
	}
	if echoed.Homepage != link {
		t.Errorf("echoed %q, want %q", echoed.Homepage, link)
	}

	// It landed on the account the PATH named, and LT-014's scope on that same
	// row is untouched - the write sets one key, it does not replace settings.
	var stored, scope string
	if err := testPool.QueryRow(t.Context(),
		`SELECT COALESCE(settings->>'loretide.homepage',''), COALESCE(settings->>'loretide.scope','')
		 FROM content_account WHERE account_id=$1`, mine).Scan(&stored, &scope); err != nil {
		t.Fatal(err)
	}
	if stored != link {
		t.Errorf("stored %q, want %q", stored, link)
	}
	if scope != "all" {
		t.Errorf("the account's material scope became %q; saving a homepage wiped it", scope)
	}

	// An account in another brand, and one that does not exist, answer
	// identically. Neither response may confirm that an id is real.
	foreign := fmt.Sprintf("acct-foreign-%d", time.Now().UnixNano())
	otherWS := fmt.Sprintf("ws-foreign-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_account
		(account_id, workspace_id, platform, display_name)
		VALUES ($1,$2,'douyin','Theirs')`, foreign, otherWS); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id=$1`, foreign)

	foreignResponse := accountAPIRequest(t, http.MethodPut,
		"/api/content-accounts/"+foreign+"/homepage", fmt.Sprintf(`{"homepage":%q}`, link))
	foreignBody, _ := io.ReadAll(foreignResponse.Body)
	foreignResponse.Body.Close()

	missingResponse := accountAPIRequest(t, http.MethodPut,
		"/api/content-accounts/acct-does-not-exist/homepage", fmt.Sprintf(`{"homepage":%q}`, link))
	missingBody, _ := io.ReadAll(missingResponse.Body)
	missingResponse.Body.Close()

	if foreignResponse.StatusCode != http.StatusNotFound {
		t.Errorf("another brand's account = %d, want 404", foreignResponse.StatusCode)
	}
	if missingResponse.StatusCode != http.StatusNotFound {
		t.Errorf("a missing account = %d, want 404", missingResponse.StatusCode)
	}
	if withoutTrace(foreignBody) != withoutTrace(missingBody) {
		t.Errorf("the two refusals differ:\nforeign: %s\nmissing: %s", foreignBody, missingBody)
	}

	// And the other brand's row was not touched.
	var foreignHomepage string
	if err := testPool.QueryRow(t.Context(),
		`SELECT COALESCE(settings->>'loretide.homepage','') FROM content_account WHERE account_id=$1`,
		foreign).Scan(&foreignHomepage); err != nil {
		t.Fatal(err)
	}
	if foreignHomepage != "" {
		t.Errorf("another brand's account got a homepage: %q", foreignHomepage)
	}
}

// withoutTrace drops the one field two refusals are allowed to differ in.
func withoutTrace(body []byte) string {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return string(body)
	}
	delete(decoded, "trace_id")
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}
