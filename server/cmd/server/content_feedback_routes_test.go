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

// Manual metrics and feedback excerpts (specs/027).

var contentFeedbackRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-metrics/", "/api/content-metrics"},
	{http.MethodPost, "/api/content-metrics/", "/api/content-metrics"},
	{http.MethodPost, "/api/content-metrics/import", "/api/content-metrics/import"},
	{http.MethodGet, "/api/content-feedback/", "/api/content-feedback"},
	{http.MethodPost, "/api/content-feedback/", "/api/content-feedback"},
	{http.MethodGet, "/api/content-feedback/pending", "/api/content-feedback/pending"},
}

func TestContentFeedbackEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentFeedbackRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	// Both tables are append-only, so there must be no way to change or remove
	// a row. R-044's "更新保留历史" is kept by there being no route that could
	// break it.
	for route := range mounted {
		for _, prefix := range []string{"/api/content-metrics/", "/api/content-feedback/"} {
			for _, verb := range []string{http.MethodDelete, http.MethodPatch, http.MethodPut} {
				if strings.HasPrefix(route, verb+" "+prefix) {
					t.Errorf("a %s route exists on feedback learning: %s", verb, route)
				}
			}
		}
	}
	// This feature has NO path parameters at all: append-only means there is
	// no single-record path to hang one on. Workflow step 12's "parameter is
	// not the context id" case therefore has no subject here, which is a fact
	// worth asserting rather than a step worth skipping silently.
	for route := range mounted {
		if !strings.Contains(route, "/api/content-metrics") && !strings.Contains(route, "/api/content-feedback") {
			continue
		}
		if strings.Contains(route, "{") {
			t.Errorf("a path parameter appeared on an append-only feature: %s", route)
		}
	}
}

func TestContentFeedbackEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentFeedbackRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentFeedbackEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-feedback-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Feedback Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Feedback Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentFeedbackRoutes {
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

// Through the real router and middleware: the workspace comes from the header
// and the publication record from the body, and the response has to be about
// the record the body named - not about whatever id the context happened to
// hold. This is workflow step 12's substance for a feature whose ids all
// travel in the body.
func TestFeedbackRecordIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	publicationID := fmt.Sprintf("pub-route-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a','','xiaohongshu','verified_published',$3,'https://example.invalid/p/1', now())`,
		publicationID, testWorkspaceID, testUserID); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_manual_metric WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_feedback_excerpt WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_publication_record WHERE publication_record_id=$1`, publicationID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	if publicationID == testWorkspaceID {
		t.Fatal("the record id equals the workspace id; the two cannot be told apart")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	created := accountAPIRequest(t, http.MethodPost, "/api/content-metrics",
		fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu","account_id":"acct-1",
			"metric":"read","value":null,"unit":"次","stat_window":"首日","sampled_at":%q,
			"evidence_note":""}`, publicationID, now))
	defer created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(created.Body)
		t.Fatalf("record metric = %d, want 201: %s", created.StatusCode, body)
	}
	var metric struct {
		PublicationRecordID string   `json:"publication_record_id"`
		WorkspaceID         string   `json:"workspace_id"`
		Value               *float64 `json:"value"`
		SourceType          string   `json:"source_type"`
		RecordedBy          string   `json:"recorded_by"`
	}
	if err := json.NewDecoder(created.Body).Decode(&metric); err != nil {
		t.Fatal(err)
	}
	if metric.PublicationRecordID != publicationID {
		t.Fatalf("body named %q, response answered for %q", publicationID, metric.PublicationRecordID)
	}
	if metric.WorkspaceID != testWorkspaceID {
		t.Errorf("the row landed in workspace %q", metric.WorkspaceID)
	}
	// An unknown value survives the whole boundary as null, not 0.
	if metric.Value != nil {
		t.Errorf("an unknown value came back as %v", *metric.Value)
	}
	if metric.SourceType != "manual" {
		t.Errorf("source_type = %q, want manual", metric.SourceType)
	}
	if metric.RecordedBy != testUserID {
		t.Errorf("recorded_by = %q, want the session's user", metric.RecordedBy)
	}

	// A record from another workspace and one that does not exist answer
	// identically, so the response cannot be used to probe.
	foreignID := fmt.Sprintf("pub-foreign-%d", time.Now().UnixNano())
	otherWS := fmt.Sprintf("ws-foreign-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id)
		VALUES ($1,$2,'w','a','','xiaohongshu','verified_published','someone')`,
		foreignID, otherWS); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_publication_record WHERE publication_record_id=$1`, foreignID)

	foreign := accountAPIRequest(t, http.MethodPost, "/api/content-metrics",
		fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu","account_id":"",
			"metric":"read","value":1,"unit":"","stat_window":"","sampled_at":%q,
			"evidence_note":""}`, foreignID, now))
	defer foreign.Body.Close()
	missing := accountAPIRequest(t, http.MethodPost, "/api/content-metrics",
		fmt.Sprintf(`{"publication_record_id":"no-such-record","platform":"xiaohongshu","account_id":"",
			"metric":"read","value":1,"unit":"","stat_window":"","sampled_at":%q,
			"evidence_note":""}`, now))
	defer missing.Body.Close()
	if foreign.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign = %d, missing = %d, want 404 and 404", foreign.StatusCode, missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}

	// And the pending list drops the record once a number is in.
	pending := accountAPIRequest(t, http.MethodGet, "/api/content-feedback/pending", "")
	defer pending.Body.Close()
	if pending.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(pending.Body)
		t.Fatalf("pending = %d, want 200: %s", pending.StatusCode, body)
	}
	var pendingBody struct {
		Pending []struct {
			PublicationRecordID string `json:"publication_record_id"`
		} `json:"pending"`
	}
	if err := json.NewDecoder(pending.Body).Decode(&pendingBody); err != nil {
		t.Fatal(err)
	}
	for _, item := range pendingBody.Pending {
		if item.PublicationRecordID == publicationID {
			t.Error("a record with a metric was still listed as pending")
		}
	}
}
