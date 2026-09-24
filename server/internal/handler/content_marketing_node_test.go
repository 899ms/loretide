package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/033 PR 1: the marketing node handlers against a real database
// (T027: 400 names the field, 404 is one shape, 409 on a stale revision) and
// the workspace delete fence on every node write path (T022).

// nodeWorkspace makes a brand the test user owns and removes it, with its
// node rows, when the test ends.
func nodeWorkspace(t *testing.T, slug string) string {
	t.Helper()
	slug = fmt.Sprintf("%s-%d", slug, time.Now().UnixNano())
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "marketing node test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, wsID, testUserID)
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_marketing_node_candidate WHERE workspace_id = $1`,
			`DELETE FROM content_marketing_node_revision WHERE workspace_id = $1`,
			`DELETE FROM content_marketing_node WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, wsID)
		}
	})
	return wsID
}

func nodeHandler(t *testing.T) *Handler {
	t.Helper()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	return &h
}

func nodeCall(t *testing.T, handler http.HandlerFunc, method, wsID, nodeID, body string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest(method, "/api/content-marketing-nodes", body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	if nodeID != "" {
		req = testutil.WithURLParams(req, "nodeId", nodeID)
	}
	return testutil.Call(t, handler, req)
}

func nodeBody(extra string) string {
	body := `{"name":"双十一","kind":"marketing","starts_on":"2026-11-11","ends_on":"2026-11-11",
		"timezone":"Asia/Shanghai","lead_days":14,"accounts":[],"goal":"清库存",
		"material_source_ids":[],"date_certainty":"confirmed","date_basis":""`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

func TestContentMarketingNodeHandlersAnswerTheContract(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := nodeHandler(t)
	brandA := nodeWorkspace(t, "node-brand-a")
	brandB := nodeWorkspace(t, "node-brand-b")

	var created topicplanning.MarketingNode
	nodeCall(t, h.CreateContentMarketingNode, "POST", brandA, "", nodeBody(`"note":"首版"`)).
		Want(http.StatusCreated).JSON(&created)
	if created.NodeID == "" || created.Status != topicplanning.NodeStatusActive ||
		created.Origin != topicplanning.NodeOriginManual || created.Timing.Timezone != "Asia/Shanghai" ||
		created.Timing.Today == "" {
		t.Fatalf("created = %+v", created)
	}

	// 400 names the field.
	for _, tc := range []struct{ body, field string }{
		{strings.Replace(nodeBody(""), `"ends_on":"2026-11-11"`, `"ends_on":"2026-11-10"`, 1), "ends_on"},
		{strings.Replace(nodeBody(""), `"Asia/Shanghai"`, `"Local"`, 1), "timezone"},
		{strings.Replace(nodeBody(""), `"lead_days":14`, `"lead_days":1.5`, 1), "lead_days"},
	} {
		body := nodeCall(t, h.CreateContentMarketingNode, "POST", brandA, "", tc.body).
			Want(http.StatusBadRequest).Map()
		if body["field"] != tc.field {
			t.Errorf("400 field = %v, want %s", body["field"], tc.field)
		}
	}
	nodeCall(t, h.CreateContentMarketingNode, "POST", brandA, "", nodeBody(`"origin":"import"`)).
		Want(http.StatusBadRequest)
	nodeCall(t, h.ListContentMarketingNodes, "GET", brandA, "", "").Want(http.StatusOK)

	rows := make([]string, 101)
	for i := range rows {
		rows[i] = nodeBody("")
	}
	tooMany := nodeCall(t, h.ImportContentMarketingNodes, "POST", brandA, "",
		`{"rows":[`+strings.Join(rows, ",")+`]}`).Want(http.StatusBadRequest).Map()
	if tooMany["field"] != "rows" {
		t.Fatalf("101 rows field = %v, want rows", tooMany["field"])
	}

	// 404: brand A's node seen from brand B is byte-identical to a random id.
	missingID := diagnostics.NewID()
	for name, call := range map[string]struct {
		handler http.HandlerFunc
		method  string
		body    string
	}{
		"get":     {h.GetContentMarketingNode, "GET", ""},
		"history": {h.ListContentMarketingNodeRevisions, "GET", ""},
		"revise":  {h.ReviseContentMarketingNode, "POST", nodeBody(`"base_revision":1`)},
		"confirm": {h.ConfirmContentMarketingNode, "POST", `{"base_revision":1}`},
		"cancel":  {h.CancelContentMarketingNode, "POST", `{"base_revision":1}`},
	} {
		foreign := nodeCall(t, call.handler, call.method, brandB, created.NodeID, call.body).Want(http.StatusNotFound).Text()
		missing := nodeCall(t, call.handler, call.method, brandB, missingID, call.body).Want(http.StatusNotFound).Text()
		if foreign != missing {
			t.Errorf("%s: foreign 404 %q differs from missing 404 %q", name, foreign, missing)
		}
	}

	// 409 on a stale base; the node is unchanged by the refused write.
	edited := strings.Replace(nodeBody(`"base_revision":1`), `"清库存"`, `"拉新"`, 1)
	nodeCall(t, h.ReviseContentMarketingNode, "POST", brandA, created.NodeID, edited).Want(http.StatusOK)
	conflict := nodeCall(t, h.ReviseContentMarketingNode, "POST", brandA, created.NodeID,
		strings.Replace(edited, `"拉新"`, `"再改"`, 1)).Want(http.StatusConflict).Map()
	if conflict["field"] != "base_revision" {
		t.Fatalf("409 body = %v", conflict)
	}
	var history struct {
		Revisions []topicplanning.NodeRevision `json:"revisions"`
	}
	nodeCall(t, h.ListContentMarketingNodeRevisions, "GET", brandA, created.NodeID, "").
		Want(http.StatusOK).JSON(&history)
	if len(history.Revisions) != 2 || history.Revisions[1].Goal != "拉新" {
		t.Fatalf("history after a 409 = %+v", history.Revisions)
	}

	// Confirming an active node is refused on status, not by 409.
	status := nodeCall(t, h.ConfirmContentMarketingNode, "POST", brandA, created.NodeID, `{"base_revision":2}`).
		Want(http.StatusBadRequest).Map()
	if status["field"] != "status" {
		t.Fatalf("confirm active = %v, want 400 on status", status)
	}
}

// The node write paths take LockWorkspaceForContentDiagnosticWrite first, so a
// write after a committed workspace deletion is refused as not found and
// leaves no row - node, revision or audit (FR-048, Issue #104).
func TestContentMarketingNodeWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	store := h.topicPlanningStore()

	slug := fmt.Sprintf("node-fence-%d", time.Now().UnixNano())
	var workspaceID string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ($1, $2) RETURNING id`,
		slug, slug).Scan(&workspaceID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_marketing_node_revision WHERE workspace_id = $1`,
			`DELETE FROM content_marketing_node WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, workspaceID)
		}
	})

	content := topicplanning.NodeContent{
		Name: "双十一", Kind: topicplanning.NodeKindMarketing, StartsOn: "2026-11-11", EndsOn: "2026-11-11",
		Timezone: "Asia/Shanghai", DateCertainty: topicplanning.DateConfirmed,
	}
	created, err := store.CreateNode(ctx, workspaceID, testUserID, topicplanning.CreateNodeRequest{Content: content})
	if err != nil {
		t.Fatalf("create while the workspace exists: %v", err)
	}

	count := func(table string) int {
		var total int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, workspaceID).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}
	nodesBefore, revisionsBefore, auditBefore := count("content_marketing_node"),
		count("content_marketing_node_revision"), count("content_operation_audit")
	if nodesBefore != 1 || revisionsBefore != 1 {
		t.Fatalf("setup wrote %d nodes and %d revisions, want 1 and 1", nodesBefore, revisionsBefore)
	}

	if _, err = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	edited := content
	edited.Goal = "改"
	importRows, err := topicplanning.DecodeImport([]byte(`{"rows":[{"name":"年货节","kind":"marketing",
		"starts_on":"2027-01-15","ends_on":"2027-01-15","timezone":"Asia/Shanghai","date_certainty":"tentative"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, write := range []struct {
		name string
		call func() error
	}{
		{"create", func() error {
			_, err := store.CreateNode(ctx, workspaceID, testUserID, topicplanning.CreateNodeRequest{Content: content})
			return err
		}},
		{"import", func() error {
			_, err := store.ImportNodes(ctx, workspaceID, testUserID, importRows)
			return err
		}},
		{"revise", func() error {
			_, err := store.ReviseNode(ctx, workspaceID, testUserID, created.NodeID,
				topicplanning.ReviseNodeRequest{BaseRevision: 1, Content: edited})
			return err
		}},
		{"confirm", func() error {
			_, err := store.ConfirmNode(ctx, workspaceID, testUserID, created.NodeID, topicplanning.TransitionRequest{BaseRevision: 1})
			return err
		}},
		{"cancel", func() error {
			_, err := store.CancelNode(ctx, workspaceID, testUserID, created.NodeID, topicplanning.TransitionRequest{BaseRevision: 1})
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, topicplanning.ErrNotFound) {
				t.Fatalf("%s after the delete committed = %v, want ErrNotFound", write.name, err)
			}
		})
	}

	if got := count("content_marketing_node"); got != nodesBefore {
		t.Errorf("content_marketing_node = %d after the refused writes, want %d", got, nodesBefore)
	}
	if got := count("content_marketing_node_revision"); got != revisionsBefore {
		t.Errorf("content_marketing_node_revision = %d after the refused writes, want %d", got, revisionsBefore)
	}
	if got := count("content_operation_audit"); got != auditBefore {
		t.Errorf("content_operation_audit = %d after the refused writes, want %d", got, auditBefore)
	}
}
