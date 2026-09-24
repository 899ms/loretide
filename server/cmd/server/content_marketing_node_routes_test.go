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

// specs/033 PR 1, T026: the marketing node endpoints behind the real router
// and middleware (workflow step 12, FR-049, FR-050, SC-012).

var contentMarketingNodeRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-marketing-nodes/", "/api/content-marketing-nodes"},
	{http.MethodPost, "/api/content-marketing-nodes/", "/api/content-marketing-nodes"},
	{http.MethodPost, "/api/content-marketing-nodes/import", "/api/content-marketing-nodes/import"},
	{http.MethodGet, "/api/content-marketing-nodes/{nodeId}/", "/api/content-marketing-nodes/node-1"},
	{http.MethodGet, "/api/content-marketing-nodes/{nodeId}/revisions", "/api/content-marketing-nodes/node-1/revisions"},
	{http.MethodPost, "/api/content-marketing-nodes/{nodeId}/revisions", "/api/content-marketing-nodes/node-1/revisions"},
	{http.MethodPost, "/api/content-marketing-nodes/{nodeId}/confirm", "/api/content-marketing-nodes/node-1/confirm"},
	{http.MethodPost, "/api/content-marketing-nodes/{nodeId}/cancel", "/api/content-marketing-nodes/node-1/cancel"},
}

func TestContentMarketingNodeEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentMarketingNodeRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
}

func TestContentMarketingNodeEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentMarketingNodeRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentMarketingNodeEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("marketing-node-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Marketing Node Outsider", email)
	token, err := generateTestJWT(outsider, email, "Marketing Node Outsider")
	if err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_technical_log
		WHERE workspace_id=$1 AND payload->>'actor_id'=$2 AND `+workspaceCoreRefusal,
		testWorkspaceID, outsider)
	count := func() int {
		return fx.Count(t, `SELECT count(*) FROM content_technical_log
			WHERE workspace_id=$1 AND payload->>'actor_id'=$2 AND `+workspaceCoreRefusal,
			testWorkspaceID, outsider)
	}
	before := count()
	for _, route := range contentMarketingNodeRoutes {
		req, err := http.NewRequest(route.method, testServer.URL+route.path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s as outsider = %d, want 404", route.method, route.path, response.StatusCode)
		}
	}
	if got := count() - before; got != len(contentMarketingNodeRoutes) {
		t.Fatalf("workspace-core recorded %d marketing node refusals, want %d", got, len(contentMarketingNodeRoutes))
	}
}

type routedNode struct {
	NodeID          string `json:"node_id"`
	WorkspaceID     string `json:"workspace_id"`
	Status          string `json:"status"`
	CurrentRevision int64  `json:"current_revision"`
	Current         struct {
		ChangeKind string `json:"change_kind"`
		Goal       string `json:"goal"`
	} `json:"current"`
}

func decodeRouted(t *testing.T, response *http.Response, want int, target any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("%s %s = %d, want %d", response.Request.Method, response.Request.URL.Path, response.StatusCode, want)
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
	}
}

const routedNodeBody = `"kind":"marketing","starts_on":"2026-11-11","ends_on":"2026-11-11",
	"timezone":"Asia/Shanghai","lead_days":14,"accounts":[],"goal":"%s",
	"material_source_ids":[],"date_certainty":"confirmed","date_basis":""`

func cleanupRoutedNodes(t *testing.T, fx *testutil.Fixture, nodeIDs ...string) {
	t.Helper()
	for _, id := range nodeIDs {
		fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, id)
		fx.Cleanup(t, `DELETE FROM content_marketing_node_revision WHERE node_id=$1`, id)
		fx.Cleanup(t, `DELETE FROM content_marketing_node WHERE node_id=$1`, id)
	}
}

// Every {nodeId} endpoint answers for the node named in the URL. Behind the
// real router the workspace id sits in the request context; an endpoint that
// read that instead of its path parameter would answer 404 for a node that
// exists, and a handler test calling the function directly could not see it.
func TestContentMarketingNodePathIDSurvivesTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	name := fmt.Sprintf("路由节点-%d", time.Now().UnixNano())

	var created routedNode
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, "/api/content-marketing-nodes",
		`{"name":"`+name+`",`+fmt.Sprintf(routedNodeBody, "初版")+`,"note":""}`), http.StatusCreated, &created)
	cleanupRoutedNodes(t, fx, created.NodeID)
	if created.NodeID == "" || created.NodeID == testWorkspaceID {
		t.Fatalf("node id %q cannot prove path/context separation", created.NodeID)
	}
	path := "/api/content-marketing-nodes/" + created.NodeID

	var read routedNode
	decodeRouted(t, accountAPIRequest(t, http.MethodGet, path, ""), http.StatusOK, &read)
	if read.NodeID != created.NodeID || read.WorkspaceID != testWorkspaceID {
		t.Fatalf("GET %s returned node %q in %q", path, read.NodeID, read.WorkspaceID)
	}

	var revised routedNode
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, path+"/revisions",
		`{"base_revision":1,"name":"`+name+`",`+fmt.Sprintf(routedNodeBody, "改版")+`,"note":"改目标"}`),
		http.StatusOK, &revised)
	if revised.NodeID != created.NodeID || revised.CurrentRevision != 2 || revised.Current.Goal != "改版" {
		t.Fatalf("revise via path = %+v", revised)
	}

	// Same base again: the node has moved on, and the answer is 409.
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, path+"/revisions",
		`{"base_revision":1,"name":"`+name+`",`+fmt.Sprintf(routedNodeBody, "再改")+`}`),
		http.StatusConflict, nil)

	var history struct {
		Revisions []struct {
			NodeID   string `json:"node_id"`
			Revision int64  `json:"revision"`
		} `json:"revisions"`
	}
	decodeRouted(t, accountAPIRequest(t, http.MethodGet, path+"/revisions", ""), http.StatusOK, &history)
	if len(history.Revisions) != 2 || history.Revisions[0].NodeID != created.NodeID || history.Revisions[1].Revision != 2 {
		t.Fatalf("history via path = %+v", history)
	}

	var cancelled routedNode
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, path+"/cancel", `{"base_revision":2,"note":"取消"}`),
		http.StatusOK, &cancelled)
	if cancelled.NodeID != created.NodeID || cancelled.Status != "cancelled" {
		t.Fatalf("cancel via path = %+v", cancelled)
	}
}

// POST /import is a static segment registered before /{nodeId}: it must reach
// the import handler, and confirm must then work on the id the import made.
func TestContentMarketingNodeImportIsNotANodeID(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	name := fmt.Sprintf("导入节点-%d", time.Now().UnixNano())
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1 AND payload->>'step'='import-marketing-nodes'
		AND payload->>'actor_id'=$2`, testWorkspaceID, testUserID)

	var imported struct {
		Results []struct {
			Row     int    `json:"row"`
			Outcome string `json:"outcome"`
			NodeID  string `json:"node_id"`
		} `json:"results"`
	}
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, "/api/content-marketing-nodes/import",
		`{"rows":[{"name":"`+name+`",`+fmt.Sprintf(routedNodeBody, "")+`}]}`), http.StatusOK, &imported)
	if len(imported.Results) != 1 || imported.Results[0].Outcome != "created" || imported.Results[0].NodeID == "" {
		t.Fatalf("import = %+v", imported)
	}
	nodeID := imported.Results[0].NodeID
	cleanupRoutedNodes(t, fx, nodeID)
	if n := fx.Count(t, `SELECT count(*) FROM content_marketing_node WHERE node_id='import'`); n != 0 {
		t.Fatalf("a node with id \"import\" exists")
	}

	var confirmed routedNode
	decodeRouted(t, accountAPIRequest(t, http.MethodPost, "/api/content-marketing-nodes/"+nodeID+"/confirm",
		`{"base_revision":1}`), http.StatusOK, &confirmed)
	if confirmed.NodeID != nodeID || confirmed.Status != "active" || confirmed.Current.ChangeKind != "confirm" {
		t.Fatalf("confirm via path = %+v", confirmed)
	}
}
