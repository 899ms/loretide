package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	reviewdelivery "github.com/multica-ai/multica/server/internal/content/review-delivery"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md

// reviewWorkspace makes a brand with a work, one channel draft and one saved
// version, and returns the workspace, the document and the version.
func reviewWorkspace(t *testing.T, slug, kind string) (string, string, string) {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "review test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)
	workID := fmt.Sprintf("work-%s", slug)
	artifactID := fmt.Sprintf("artifact-%s", slug)
	versionID := fmt.Sprintf("version-%s", slug)
	dbfx.Exec(t, `INSERT INTO content_work (work_id, workspace_id, topic_card_id, snapshot_id, title)
		VALUES ($1,$2,'card-1','','第一篇')`, workID, wsID)
	dbfx.Exec(t, `INSERT INTO content_artifact
		(artifact_id, work_id, workspace_id, kind, title, position)
		VALUES ($1,$2,$3,$4,'稿',1)`, artifactID, workID, wsID, kind)
	dbfx.Exec(t, `INSERT INTO content_artifact_version
		(version_id, artifact_id, work_id, workspace_id, revision, source, action, body, actor_id)
		VALUES ($1,$2,$3,$4,1,'edited','saved','第三版正文',$5)`,
		versionID, artifactID, workID, wsID, testUserID)
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
			`DELETE FROM content_delivery_task WHERE workspace_id = $1`,
			`DELETE FROM content_review_transition WHERE workspace_id = $1`,
			`DELETE FROM content_review_request WHERE workspace_id = $1`,
			`DELETE FROM content_artifact_version WHERE workspace_id = $1`,
			`DELETE FROM content_artifact WHERE workspace_id = $1`,
			`DELETE FROM content_work WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, wsID)
		}
	})
	return wsID, artifactID, versionID
}

// reviewHandler is testHandler with the content diagnostics service attached.
//
// Without it the module has no audit sink, begin() answers ErrStorage and every
// case below would fail as 503 - which is a real failure mode, just not the one
// these tests are about.
func reviewHandler(t *testing.T) *Handler {
	t.Helper()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	return &h
}

func reviewPost(t *testing.T, handler http.HandlerFunc, wsID, path, body string, params ...string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest("POST", path, body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	if len(params) > 0 {
		req = withURLParams(req, params...)
	}
	return testutil.Call(t, handler, req)
}

// The contract's decision order, one case per step, first failure winning.
//
// Steps 1-2 answer identically - a caller must not be able to tell a foreign
// row from a missing one - and the assertion below is byte-for-byte, not "both
// were 404". Steps 3-6 are the caller's own input and say which field is wrong:
// "参数错误" would leave the person filling the form with nothing to act on.
func TestTheReviewDecisionOrderRefusesAtTheFirstFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, artifactID, versionID := reviewWorkspace(t, "review-order", "channel_draft")

	// Step 1: not a member. The refusal is recorded and says nothing.
	outsiderWS := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Other", "slug": fmt.Sprintf("review-order-other-%d", time.Now().UnixNano()),
	})
	notMember := reviewPost(t, reviewHandler(t).SubmitContentReview, outsiderWS, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"a","channel":"xiaohongshu"}`,
			artifactID, versionID))
	notMember.Want(http.StatusNotFound)

	// Step 2: a member, but the version does not exist here.
	missing := reviewPost(t, reviewHandler(t).SubmitContentReview, wsID, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":"no-such-version","account_id":"a","channel":"xiaohongshu"}`,
			artifactID))
	missing.Want(http.StatusNotFound)

	if !sameRefusalApartFromTrace(t, notMember, missing) {
		t.Error("a non-member and a missing version answered differently; the response can be used to probe")
	}

	// Step 3: a value outside a controlled set, named.
	reviewPost(t, reviewHandler(t).SubmitContentReview, wsID, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"a","channel":"weibo"}`,
			artifactID, versionID)).
		Want(http.StatusBadRequest)
	assertNamedField(t, reviewHandler(t).SubmitContentReview, wsID, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"a","channel":"weibo"}`,
			artifactID, versionID), "channel")

	// Step 4: the type precondition. SOP 8 reviews a channel's version.
	bodyWS, bodyArtifact, bodyVersion := reviewWorkspace(t, "review-order-body", "body")
	assertNamedField(t, reviewHandler(t).SubmitContentReview, bodyWS, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"a","channel":"xiaohongshu"}`,
			bodyArtifact, bodyVersion), "kind")

	// Steps 5 and 6 are on the delivery side.
	review := submitReview(t, wsID, artifactID, versionID)
	task := createDelivery(t, wsID, artifactID, review)
	// Step 5: a conditionally required field, named.
	assertNamedField(t, reviewHandler(t).AdvanceContentDelivery,
		wsID, "/api/content-deliveries/"+task+"/status",
		`{"status":"cancelled"}`, "reason", "deliveryId", task)
	// Step 6: an illegal move, naming both ends.
	decide(t, wsID, review, "approved", "看过了")
	advance(t, wsID, task, `{"status":"ready"}`, http.StatusOK)
	advance(t, wsID, task, `{"status":"handed_off","handoff_method":"export"}`, http.StatusOK)
	illegal := reviewPost(t, reviewHandler(t).AdvanceContentDelivery,
		wsID, "/api/content-deliveries/"+task+"/status", `{"status":"ready"}`, "deliveryId", task)
	illegal.Want(http.StatusBadRequest)
	var refused map[string]any
	illegal.JSON(&refused)
	if refused["from"] != "handed_off" || refused["to"] != "ready" {
		t.Errorf("the refusal says %v -> %v, want handed_off -> ready", refused["from"], refused["to"])
	}
}

func assertNamedField(t *testing.T, handler http.HandlerFunc, wsID, path, body, field string, params ...string) {
	t.Helper()
	response := reviewPost(t, handler, wsID, path, body, params...)
	response.Want(http.StatusBadRequest)
	var got map[string]any
	response.JSON(&got)
	if got["field"] != field {
		t.Errorf("the refusal named %v, want %q (body was %s)", got["field"], field, body)
	}
}

func sameRefusalApartFromTrace(t *testing.T, left, right *testutil.Response) bool {
	t.Helper()
	strip := func(response *testutil.Response) string {
		var body map[string]any
		response.JSON(&body)
		delete(body, "trace_id")
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	return strip(left) == strip(right)
}

func submitReview(t *testing.T, wsID, artifactID, versionID string) string {
	t.Helper()
	var created map[string]any
	reviewPost(t, reviewHandler(t).SubmitContentReview, wsID, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":"acct-1","channel":"xiaohongshu"}`,
			artifactID, versionID)).
		Want(http.StatusCreated).JSON(&created)
	return created["review_request_id"].(string)
}

func decide(t *testing.T, wsID, reviewID, status, note string) {
	t.Helper()
	reviewPost(t, reviewHandler(t).DecideContentReview, wsID, "/api/content-reviews/"+reviewID+"/decision",
		fmt.Sprintf(`{"status":%q,"note":%q}`, status, note), "reviewId", reviewID).
		Want(http.StatusOK)
}

func createDelivery(t *testing.T, wsID, artifactID, reviewID string) string {
	t.Helper()
	var created map[string]any
	reviewPost(t, reviewHandler(t).CreateContentDelivery, wsID, "/api/content-deliveries",
		fmt.Sprintf(`{"artifact_id":%q,"channel":"xiaohongshu","review_request_id":%q}`,
			artifactID, reviewID)).
		Want(http.StatusCreated).JSON(&created)
	return created["delivery_task_id"].(string)
}

func advance(t *testing.T, wsID, taskID, body string, want int) map[string]any {
	t.Helper()
	var got map[string]any
	response := reviewPost(t, reviewHandler(t).AdvanceContentDelivery, wsID,
		"/api/content-deliveries/"+taskID+"/status", body, "deliveryId", taskID)
	response.Want(want)
	if want < 300 {
		response.JSON(&got)
	}
	return got
}

// SOP 8's personal mode: "个人模式可用'确认并提交审核'或'审核通过并安排交接'的连续
// 操作减少跳转，后台仍分别保存记录."
//
// The page may join the two clicks; the backend keeps two records with their
// own transition rows. A combined write would make "who approved this" and
// "who scheduled it" the same fact, which they are not.
func TestApprovingAndSchedulingStayTwoSeparateRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, artifactID, versionID := reviewWorkspace(t, "review-personal-mode", "channel_draft")
	review := submitReview(t, wsID, artifactID, versionID)
	decide(t, wsID, review, "approved", "可以发")
	task := createDelivery(t, wsID, artifactID, review)
	advance(t, wsID, task, `{"status":"ready"}`, http.StatusOK)

	var reviews, tasks int
	if err := testPool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM content_review_request WHERE workspace_id=$1),
		(SELECT count(*) FROM content_delivery_task WHERE workspace_id=$1)`, wsID).
		Scan(&reviews, &tasks); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 || tasks != 1 {
		t.Fatalf("got %d review requests and %d tasks, want 1 and 1", reviews, tasks)
	}
	for _, item := range []struct {
		kind    string
		subject string
		want    int
	}{
		{"review_request", review, 2}, // created, approved
		{"delivery_task", task, 2},    // created, ready
	} {
		var rows int
		if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_review_transition
			WHERE workspace_id=$1 AND subject_kind=$2 AND subject_id=$3`,
			wsID, item.kind, item.subject).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != item.want {
			t.Errorf("%s has %d transition rows, want %d", item.kind, rows, item.want)
		}
	}
}

// The review delivery write paths follow the workspace delete/write protocol
// the diagnostics module, topic planning and the work editor follow: the write
// transaction takes LockWorkspaceForContentDiagnosticWrite before it touches a
// row, so a delete either waits for the write and sweeps it, or commits first
// and leaves the write with no workspace to attach to.
//
// All four tables carry a text workspace id and no foreign key, so nothing else
// would stop an orphan from being written after the delete commits (Issue #104).
//
// This is also what the module's own fixture stands in for: it runs in an
// isolated schema with no workspace table, so the fence is proven here.
func TestReviewDeliveryWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	store := h.reviewDeliveryStore()

	wsID, artifactID, versionID := reviewWorkspace(t, "review-fence", "channel_draft")

	request, err := store.Submit(ctx, wsID, testUserID, reviewdelivery.SubmitRequest{
		ArtifactID: artifactID, VersionID: versionID,
		AccountID: "acct-1", Channel: reviewdelivery.ChannelXiaohongshu,
	})
	if err != nil {
		t.Fatalf("submit while the workspace exists: %v", err)
	}
	task, err := store.CreateTask(ctx, wsID, testUserID, reviewdelivery.CreateTaskRequest{
		ArtifactID: artifactID, Channel: reviewdelivery.ChannelXiaohongshu,
		ReviewRequestID: request.ReviewRequestID,
	})
	if err != nil {
		t.Fatalf("create task while the workspace exists: %v", err)
	}
	if _, err = store.Record(ctx, wsID, testUserID, reviewdelivery.RecordRequest{
		ArtifactID: artifactID, Channel: reviewdelivery.ChannelXiaohongshu,
		Status: reviewdelivery.PublicationReported, PageURLOrContentID: "https://example.invalid/p/1",
	}); err != nil {
		t.Fatalf("record while the workspace exists: %v", err)
	}

	count := func(table string) int {
		var total int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, wsID).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}
	before := map[string]int{}
	for _, table := range []string{
		"content_review_request", "content_review_transition",
		"content_delivery_task", "content_publication_record",
	} {
		before[table] = count(table)
	}
	if before["content_review_request"] != 1 || before["content_delivery_task"] != 1 ||
		before["content_publication_record"] != 1 {
		t.Fatalf("setup wrote %v", before)
	}

	// The delete commits on its own connection, exactly as a workspace deletion
	// that finished just before the next request arrived.
	if _, err = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, wsID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	for _, write := range []struct {
		name string
		call func() error
	}{
		{"submit-review", func() error {
			_, err := store.Submit(ctx, wsID, testUserID, reviewdelivery.SubmitRequest{
				ArtifactID: artifactID, VersionID: versionID,
				AccountID: "acct-1", Channel: reviewdelivery.ChannelXiaohongshu,
			})
			return err
		}},
		{"decide-review", func() error {
			_, err := store.Decide(ctx, wsID, testUserID, request.ReviewRequestID,
				reviewdelivery.ReviewApproved, "可以发")
			return err
		}},
		{"create-task", func() error {
			_, err := store.CreateTask(ctx, wsID, testUserID, reviewdelivery.CreateTaskRequest{
				ArtifactID: artifactID, Channel: reviewdelivery.ChannelXiaohongshu,
			})
			return err
		}},
		{"advance-task", func() error {
			_, err := store.Advance(ctx, wsID, testUserID, task.DeliveryTaskID,
				reviewdelivery.DeliveryAdvance{To: reviewdelivery.DeliveryCancelled, Reason: "不做了"})
			return err
		}},
		{"record-publication", func() error {
			_, err := store.Record(ctx, wsID, testUserID, reviewdelivery.RecordRequest{
				ArtifactID: artifactID, Channel: reviewdelivery.ChannelXiaohongshu,
				Status: reviewdelivery.PublicationRemoved, ReceiptNote: "链接失效",
			})
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, reviewdelivery.ErrNotFound) {
				t.Fatalf("%s after the delete committed = %v, want ErrNotFound", write.name, err)
			}
		})
	}

	// Nothing landed. A single extra row here is an orphan nothing will ever
	// clean up, because the workspace it belonged to is gone.
	for table, want := range before {
		if got := count(table); got != want {
			t.Errorf("%s has %d rows, want %d", table, got, want)
		}
	}
}
