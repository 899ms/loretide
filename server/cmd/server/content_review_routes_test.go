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

// Review requests, delivery tasks and publication records (specs/025).

var contentReviewRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-reviews/", "/api/content-reviews"},
	{http.MethodPost, "/api/content-reviews/", "/api/content-reviews"},
	{http.MethodGet, "/api/content-reviews/{reviewId}/", "/api/content-reviews/review-1"},
	{http.MethodPost, "/api/content-reviews/{reviewId}/decision", "/api/content-reviews/review-1/decision"},
	{http.MethodGet, "/api/content-deliveries/", "/api/content-deliveries"},
	{http.MethodPost, "/api/content-deliveries/", "/api/content-deliveries"},
	{http.MethodPost, "/api/content-deliveries/{deliveryId}/status", "/api/content-deliveries/delivery-1/status"},
	{http.MethodGet, "/api/content-publications/", "/api/content-publications"},
	{http.MethodPost, "/api/content-publications/", "/api/content-publications"},
}

func TestContentReviewEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentReviewRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	// Publication records and the transition log are append-only, so there must
	// be no way to change or remove one. Keeping them is not a promise anybody
	// has to remember if no route exists that could break it.
	for route := range mounted {
		for _, prefix := range []string{"/api/content-reviews/", "/api/content-deliveries/", "/api/content-publications/"} {
			if strings.HasPrefix(route, http.MethodDelete+" "+prefix) {
				t.Errorf("a delete route exists on review delivery: %s", route)
			}
		}
		if strings.HasPrefix(route, http.MethodPatch+" /api/content-publications/") ||
			strings.HasPrefix(route, http.MethodPut+" /api/content-publications/") {
			t.Errorf("a publication record can be rewritten: %s", route)
		}
		// SOP 9.2: "系统不保存平台发布密钥，也不提供发布执行接口". No route may
		// be the thing that publishes.
		for _, forbidden := range []string{"/publish", "/platform", "/schedule-publish"} {
			if strings.Contains(route, forbidden) && strings.Contains(route, "content-") {
				t.Errorf("a publish execution route exists: %s", route)
			}
		}
	}
}

func TestContentReviewEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentReviewRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentReviewEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-review-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Review Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Review Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentReviewRoutes {
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

// Workflow step 12 for the two path parameters this feature adds.
//
// Route existence is not enough: a handler that read the workspace id where it
// meant to read a path parameter would still be mounted, still answer, and
// still look right in a test whose two values happen to be equal. Both ids here
// are server-chosen, neither equals the workspace id, and they differ from each
// other.
//
// Publication records add no third parameter: they are append-only, so there is
// no single-record path at all.
func TestReviewDeliveryPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	artifactID, versionID, workID, cardID := seedChannelDraftThroughTheAPI(t, fx)

	review := submitReviewThroughTheAPI(t, artifactID, versionID)
	fx.Cleanup(t, `DELETE FROM content_review_transition WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_review_request WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_delivery_task WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_publication_record WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE work_id=$1`, workID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE work_id=$1`, workID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE work_id=$1`, workID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	// {reviewId}: the response has to be about the request the path named.
	read := accountAPIRequest(t, http.MethodGet, "/api/content-reviews/"+review, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(read.Body)
		t.Fatalf("GET review = %d, want 200: %s", read.StatusCode, body)
	}
	var got struct {
		Review struct {
			ReviewRequestID string `json:"review_request_id"`
			Snapshot        struct {
				Attachments    []string          `json:"attachments"`
				DeliveryConfig map[string]string `json:"delivery_config"`
				VersionID      string            `json:"version_id"`
			} `json:"snapshot"`
		} `json:"review"`
		Transitions []struct {
			FromStatus string `json:"from_status"`
			ToStatus   string `json:"to_status"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(read.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Review.ReviewRequestID != review {
		t.Fatalf("path named %q, response returned %q", review, got.Review.ReviewRequestID)
	}
	if got.Review.Snapshot.VersionID != versionID {
		t.Errorf("the frozen snapshot names version %q, want %q", got.Review.Snapshot.VersionID, versionID)
	}
	// SOP 8's snapshot survives the round trip as an array, not null: a
	// consumer should not have to know which of the two it received.
	if got.Review.Snapshot.Attachments == nil {
		t.Error("attachments came back as null; W-03 has not landed and it must be an empty array")
	}
	if len(got.Review.Snapshot.Attachments) != 0 {
		t.Errorf("attachments carried %v", got.Review.Snapshot.Attachments)
	}
	if len(got.Transitions) != 1 || got.Transitions[0].ToStatus != "pending" {
		t.Errorf("the creation transition is missing: %+v", got.Transitions)
	}

	decide := accountAPIRequest(t, http.MethodPost, "/api/content-reviews/"+review+"/decision",
		`{"status":"approved","note":"看过了"}`)
	defer decide.Body.Close()
	if decide.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(decide.Body)
		t.Fatalf("decide = %d, want 200: %s", decide.StatusCode, body)
	}

	// {deliveryId}: a second class of id, created through the API so its value
	// is the server's and not the test's.
	delivery := createDeliveryThroughTheAPI(t, artifactID, review)
	for name, id := range map[string]string{"review": review, "delivery": delivery} {
		if id == "" || id == testWorkspaceID {
			t.Fatalf("%s id %q cannot prove path/context separation", name, id)
		}
	}
	if review == delivery {
		t.Fatal("the two ids are equal; the parameters cannot be told apart")
	}
	advance := accountAPIRequest(t, http.MethodPost, "/api/content-deliveries/"+delivery+"/status",
		`{"status":"ready"}`)
	defer advance.Body.Close()
	if advance.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(advance.Body)
		t.Fatalf("advance = %d, want 200: %s", advance.StatusCode, body)
	}
	var advanced struct {
		DeliveryTaskID string `json:"delivery_task_id"`
		Status         string `json:"status"`
	}
	if err := json.NewDecoder(advance.Body).Decode(&advanced); err != nil {
		t.Fatal(err)
	}
	if advanced.DeliveryTaskID != delivery {
		t.Fatalf("path named %q, response answered for %q", delivery, advanced.DeliveryTaskID)
	}
	if advanced.Status != "ready" {
		t.Errorf("status = %q, want ready", advanced.Status)
	}

	// An id from another workspace and an unknown one answer identically, so
	// the response cannot be used to probe for other people's rows.
	foreignReview := reviewRequestInAnotherWorkspace(t, fx)
	foreign := accountAPIRequest(t, http.MethodGet, "/api/content-reviews/"+foreignReview, "")
	defer foreign.Body.Close()
	missing := accountAPIRequest(t, http.MethodGet, "/api/content-reviews/no-such-review", "")
	defer missing.Body.Close()
	if foreign.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign = %d, missing = %d, want 404 and 404", foreign.StatusCode, missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}

	foreignDelivery := accountAPIRequest(t, http.MethodPost,
		"/api/content-deliveries/no-such-delivery/status", `{"status":"ready"}`)
	defer foreignDelivery.Body.Close()
	if foreignDelivery.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown delivery = %d, want 404", foreignDelivery.StatusCode)
	}
	// A fresh request: refusalsMatchApartFromTrace reads the body, so the one
	// compared above is already drained.
	missingAgain := accountAPIRequest(t, http.MethodGet, "/api/content-reviews/no-such-review", "")
	defer missingAgain.Body.Close()
	if !refusalsMatchApartFromTrace(t, missingAgain, foreignDelivery) {
		t.Error("the review and delivery refusals differ from each other")
	}
}

// seedChannelDraftThroughTheAPI builds a work with one channel draft and one
// saved version, all through the real API.
func seedChannelDraftThroughTheAPI(t *testing.T, fx *testutil.Fixture) (string, string, string, string) {
	t.Helper()
	cardID, _ := startTopicWithBrief(t)
	workID := createWorkThroughTheAPI(t, fx, cardID)

	response := accountAPIRequest(t, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts",
		`{"kind":"channel_draft","title":"小红书稿","position":1}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create channel draft = %d, want 201: %s", response.StatusCode, body)
	}
	var artifact struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&artifact); err != nil {
		t.Fatal(err)
	}
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+workID+"/artifacts/"+artifact.ArtifactID, `{"draft_body":"第三版正文"}`)
	patch.Body.Close()
	saved := accountAPIRequest(t, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts/"+artifact.ArtifactID+"/versions", "")
	defer saved.Body.Close()
	if saved.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(saved.Body)
		t.Fatalf("save version = %d, want 201: %s", saved.StatusCode, body)
	}
	var version struct {
		VersionID string `json:"version_id"`
	}
	if err := json.NewDecoder(saved.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	return artifact.ArtifactID, version.VersionID, workID, cardID
}

func submitReviewThroughTheAPI(t *testing.T, artifactID, versionID string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"acct-1","channel":"xiaohongshu"}`,
			artifactID, versionID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("submit review = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		ReviewRequestID string `json:"review_request_id"`
		Status          string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "pending" {
		t.Fatalf("a new request is %q, want pending", created.Status)
	}
	return created.ReviewRequestID
}

func createDeliveryThroughTheAPI(t *testing.T, artifactID, reviewID string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost, "/api/content-deliveries",
		fmt.Sprintf(`{"artifact_id":%q,"channel":"xiaohongshu","review_request_id":%q}`,
			artifactID, reviewID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create delivery = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		DeliveryTaskID string `json:"delivery_task_id"`
		Status         string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "draft" {
		t.Fatalf("a new task is %q, want draft", created.Status)
	}
	return created.DeliveryTaskID
}

// reviewRequestInAnotherWorkspace writes a row directly, because the point is
// to have an id that is real somewhere else.
func reviewRequestInAnotherWorkspace(t *testing.T, fx *testutil.Fixture) string {
	t.Helper()
	id := fmt.Sprintf("foreign-review-%d", time.Now().UnixNano())
	other := fmt.Sprintf("foreign-ws-%d", time.Now().UnixNano())
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_review_request
		(review_request_id, workspace_id, work_id, artifact_id, version_id, account_id,
		 channel, snapshot, status, requested_by)
		VALUES ($1,$2,'w','a','v','acct','xiaohongshu','{}'::jsonb,'pending','someone')`,
		id, other); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_review_request WHERE review_request_id=$1`, id)
	return id
}
