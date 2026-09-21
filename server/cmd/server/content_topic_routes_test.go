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

var contentTopicRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-topics/", "/api/content-topics"},
	{http.MethodPost, "/api/content-topics/", "/api/content-topics"},
	{http.MethodGet, "/api/content-topics/{id}/", "/api/content-topics/topic-1"},
	{http.MethodPost, "/api/content-topics/{id}/account", "/api/content-topics/topic-1/account"},
	{http.MethodPost, "/api/content-topics/{id}/sources", "/api/content-topics/topic-1/sources"},
	{http.MethodPost, "/api/content-topics/{id}/actions", "/api/content-topics/topic-1/actions"},
	{http.MethodGet, "/api/content-topics/{id}/briefs", "/api/content-topics/topic-1/briefs"},
	{http.MethodPost, "/api/content-topics/{id}/briefs", "/api/content-topics/topic-1/briefs"},
	{http.MethodGet, "/api/content-topics/{id}/briefs/{revisionId}", "/api/content-topics/topic-1/briefs/revision-1"},
	{http.MethodPost, "/api/content-topics/{id}/briefs/{revisionId}/start", "/api/content-topics/topic-1/briefs/revision-1/start"},
	{http.MethodGet, "/api/content-topics/{id}/briefs/{revisionId}/snapshots", "/api/content-topics/topic-1/briefs/revision-1/snapshots"},
	{http.MethodGet, "/api/content-topics/{id}/snapshots/{snapshotId}", "/api/content-topics/topic-1/snapshots/snapshot-1"},
}

func TestContentTopicEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentTopicRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
}

func TestContentTopicEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentTopicRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

// workspaceCoreRefusal matches the row content/workspace-core writes when it
// refuses a request. It cannot match on the component name: Sanitize is an
// allowlist and carries no content-module name, so every module's refusal is
// stored as component "unknown". What survives sanitization still identifies
// this writer uniquely - the api middleware logs object_type "diagnostics" with
// action "execute", and no other writer pairs object_type "account" with action
// "query" and step "not_member".
const workspaceCoreRefusal = `payload->>'error_code'='AUTHORIZATION_DENIED'
	AND payload->>'object_type'='account'
	AND payload->>'action'='query'
	AND payload->>'outcome'='failed'
	AND payload->>'severity'='warn'
	AND payload->>'step'='not_member'`

func TestContentTopicEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-topic-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Topic Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Topic Outsider")
	if err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_technical_log
		WHERE workspace_id=$1 AND payload->>'actor_id'=$2 AND `+workspaceCoreRefusal,
		testWorkspaceID, outsider)
	denialsBefore := fx.Count(t, `SELECT count(*) FROM content_technical_log
		WHERE workspace_id=$1 AND payload->>'actor_id'=$2 AND `+workspaceCoreRefusal,
		testWorkspaceID, outsider)
	for _, route := range contentTopicRoutes {
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
	denialsAfter := fx.Count(t, `SELECT count(*) FROM content_technical_log
		WHERE workspace_id=$1 AND payload->>'actor_id'=$2 AND `+workspaceCoreRefusal,
		testWorkspaceID, outsider)
	if got := denialsAfter - denialsBefore; got != len(contentTopicRoutes) {
		t.Fatalf("workspace-core recorded %d topic refusals, want %d", got, len(contentTopicRoutes))
	}
}

func TestContentTopicPathIDSurvivesTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	response := accountAPIRequest(t, http.MethodPost, "/api/content-topics", `{
		"audience_problem_judgment":"audience/problem/judgment",
		"ip_fit":"fit",
		"timing":"没有时效依据",
		"existing_content_relation":"没有",
		"evidence_gaps_and_investment":"没有现成证据",
		"channels":["zhihu"],
		"recommended_action":"start"
	}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create topic = %d, want 201", response.StatusCode)
	}
	var created struct {
		TopicCardID string `json:"topic_card_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.TopicCardID == "" || created.TopicCardID == testWorkspaceID {
		t.Fatalf("topic id %q cannot prove path/context separation", created.TopicCardID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, created.TopicCardID)

	read := accountAPIRequest(t, http.MethodGet, "/api/content-topics/"+created.TopicCardID, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		t.Fatalf("GET path topic = %d, want 200", read.StatusCode)
	}
	var got struct {
		TopicCardID string `json:"topic_card_id"`
	}
	if err := json.NewDecoder(read.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TopicCardID != created.TopicCardID {
		t.Fatalf("path named %q, response returned %q", created.TopicCardID, got.TopicCardID)
	}
}

// The account link endpoint carries the topic card id in the path and the
// account id in the body, and both have to arrive as themselves. Behind the
// real router the workspace id is in the request context, which is what the
// account endpoints once read instead of their path parameter — every one of
// them answered 404 for accounts that existed, and handler tests could not see
// it because the context is empty when a handler is called directly (workflow
// step 12).
func TestContentTopicAccountLinkUsesThePathIDBehindTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	accountID := "topic-link-account-" + fmt.Sprint(time.Now().UnixNano())
	fx.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name, settings)
		VALUES ($1, $2, 'zhihu', '关联用账号', '{}'::jsonb)`, accountID, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id = $1`, accountID)

	response := accountAPIRequest(t, http.MethodPost, "/api/content-topics", `{
		"audience_problem_judgment":"audience/problem/judgment",
		"ip_fit":"fit",
		"timing":"没有时效依据",
		"existing_content_relation":"没有",
		"evidence_gaps_and_investment":"没有现成证据",
		"channels":["zhihu"],
		"recommended_action":"start"
	}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create topic = %d, want 201", response.StatusCode)
	}
	var created struct {
		TopicCardID string `json:"topic_card_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.TopicCardID == "" || created.TopicCardID == testWorkspaceID {
		t.Fatalf("topic id %q cannot prove path/context separation", created.TopicCardID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, created.TopicCardID)

	linked := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+created.TopicCardID+"/account",
		`{"account_id":"`+accountID+`"}`)
	defer linked.Body.Close()
	if linked.StatusCode != http.StatusOK {
		t.Fatalf("link account = %d, want 200", linked.StatusCode)
	}
	var got struct {
		TopicCardID string  `json:"topic_card_id"`
		AccountID   *string `json:"account_id"`
	}
	if err := json.NewDecoder(linked.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TopicCardID != created.TopicCardID {
		t.Fatalf("path named %q, response returned %q", created.TopicCardID, got.TopicCardID)
	}
	if got.AccountID == nil || *got.AccountID != accountID {
		t.Fatalf("linked account = %v, want %s", got.AccountID, accountID)
	}

	// Detaching is the same endpoint with a null account, and it must not be
	// read as "leave it alone".
	detached := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+created.TopicCardID+"/account", `{"account_id":null}`)
	defer detached.Body.Close()
	if detached.StatusCode != http.StatusOK {
		t.Fatalf("detach = %d, want 200", detached.StatusCode)
	}
	var cleared struct {
		AccountID *string `json:"account_id"`
	}
	if err := json.NewDecoder(detached.Body).Decode(&cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.AccountID != nil {
		t.Fatalf("detached account = %v, want null", cleared.AccountID)
	}
}

func TestContentTopicSourcesLinkUsesThePathIDBehindTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	sourceID := "topic-link-source-" + fmt.Sprint(time.Now().UnixNano())
	fx.Exec(t, `INSERT INTO content_source (source_id, workspace_id, capture_method, source_type, title, raw_content, state)
		VALUES ($1, $2, 'manual', 'text', '素材标题', '素材正文', 'active')`, sourceID, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source WHERE source_id = $1`, sourceID)

	response := accountAPIRequest(t, http.MethodPost, "/api/content-topics", `{
		"audience_problem_judgment":"audience/problem/judgment",
		"ip_fit":"fit",
		"timing":"没有时效依据",
		"existing_content_relation":"没有",
		"evidence_gaps_and_investment":"没有现成证据",
		"channels":["zhihu"],
		"recommended_action":"start"
	}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create topic = %d, want 201", response.StatusCode)
	}
	var created struct {
		TopicCardID string `json:"topic_card_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.TopicCardID == "" || created.TopicCardID == testWorkspaceID {
		t.Fatalf("topic id %q cannot prove path/context separation", created.TopicCardID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, created.TopicCardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, created.TopicCardID)

	linked := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+created.TopicCardID+"/sources",
		`{"fit_source_ids":["`+sourceID+`"]}`)
	defer linked.Body.Close()
	if linked.StatusCode != http.StatusOK {
		t.Fatalf("link sources = %d, want 200", linked.StatusCode)
	}
	var got struct {
		TopicCardID       string   `json:"topic_card_id"`
		FitSourceIDs      []string `json:"fit_source_ids"`
		EvidenceSourceIDs []string `json:"evidence_source_ids"`
	}
	if err := json.NewDecoder(linked.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.TopicCardID != created.TopicCardID {
		t.Fatalf("path named %q, response returned %q", created.TopicCardID, got.TopicCardID)
	}
	if len(got.FitSourceIDs) != 1 || got.FitSourceIDs[0] != sourceID {
		t.Fatalf("linked fit sources = %v, want [%s]", got.FitSourceIDs, sourceID)
	}

	cleared := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+created.TopicCardID+"/sources", `{"fit_source_ids":[]}`)
	defer cleared.Body.Close()
	if cleared.StatusCode != http.StatusOK {
		t.Fatalf("clear = %d, want 200", cleared.StatusCode)
	}
	var clearedGot struct {
		FitSourceIDs []string `json:"fit_source_ids"`
	}
	if err := json.NewDecoder(cleared.Body).Decode(&clearedGot); err != nil {
		t.Fatal(err)
	}
	if len(clearedGot.FitSourceIDs) != 0 {
		t.Fatalf("cleared fit sources = %v, want empty", clearedGot.FitSourceIDs)
	}
}

