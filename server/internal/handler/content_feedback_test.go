package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/027-feedback-manual/contracts/feedback-manual.md

// feedbackWorkspace makes a brand with one publication record, optionally with
// the delivery task and review request the version resolution walks.
func feedbackWorkspace(t *testing.T, slug, status string, withDelivery bool) (string, string, string) {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "feedback test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)

	publicationID := fmt.Sprintf("pub-%s", slug)
	deliveryTaskID := ""
	versionID := ""
	if withDelivery {
		deliveryTaskID = fmt.Sprintf("task-%s", slug)
		reviewID := fmt.Sprintf("review-%s", slug)
		versionID = fmt.Sprintf("version-%s", slug)
		dbfx.Exec(t, `INSERT INTO content_review_request
			(review_request_id, workspace_id, work_id, artifact_id, version_id,
			 account_id, channel, snapshot, status, requested_by)
			VALUES ($1,$2,'w','a',$3,'acct-1','xiaohongshu','{}'::jsonb,'approved',$4)`,
			reviewID, wsID, versionID, testUserID)
		dbfx.Exec(t, `INSERT INTO content_delivery_task
			(delivery_task_id, workspace_id, work_id, artifact_id, review_request_id,
			 channel, status)
			VALUES ($1,$2,'w','a',$3,'xiaohongshu','handed_off')`,
			deliveryTaskID, wsID, reviewID)
	}
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a',$3,'xiaohongshu',$4,$5,'https://example.invalid/p/1', now())`,
		publicationID, wsID, deliveryTaskID, status, testUserID)

	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_feedback_excerpt WHERE workspace_id = $1`,
			`DELETE FROM content_manual_metric WHERE workspace_id = $1`,
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
			`DELETE FROM content_delivery_task WHERE workspace_id = $1`,
			`DELETE FROM content_review_request WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, wsID)
		}
	})
	return wsID, publicationID, versionID
}

// feedbackHandler is testHandler with the content diagnostics service
// attached. Without it the module has no audit sink and every case below would
// fail as 503 - a real failure mode, just not the one these tests are about.
func feedbackHandler(t *testing.T) *Handler {
	t.Helper()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	return &h
}

func feedbackPost(t *testing.T, handler http.HandlerFunc, wsID, path, body string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest("POST", path, body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	return testutil.Call(t, handler, req)
}

func metricBody(publicationID, metric, value string) string {
	return fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu",
		"account_id":"acct-1","metric":%q,"value":%s,"unit":"次",
		"stat_window":"发布后 14 天累计","sampled_at":%q,"evidence_note":""}`,
		publicationID, metric, value, time.Now().UTC().Format(time.RFC3339))
}

// The contract's decision order, first failure winning.
//
// Steps 1-2 answer identically - a caller must not be able to tell a foreign
// row from a missing one - and the assertion is byte-for-byte, not "both were
// 404". Steps 3-5 are the caller's own input and say which field is wrong.
func TestTheFeedbackDecisionOrderRefusesAtTheFirstFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-order", "verified_published", true)
	h := feedbackHandler(t)

	// Step 1: not a member.
	outsiderWS := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Other", "slug": fmt.Sprintf("feedback-order-other-%d", time.Now().UnixNano()),
	})
	notMember := feedbackPost(t, h.RecordContentMetric, outsiderWS, "/api/content-metrics",
		metricBody(publicationID, "read", "10"))
	notMember.Want(http.StatusNotFound)

	// Step 2: a member, but no such publication record here.
	missing := feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody("no-such-record", "read", "10"))
	missing.Want(http.StatusNotFound)

	if !sameRefusalApartFromTrace(t, notMember, missing) {
		t.Error("a non-member and a missing record answered differently; the response can be used to probe")
	}

	// Step 3: a value outside a controlled set, named.
	assertFeedbackField(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "engagement", "10"), "metric")

	// Step 4: a required field missing, named.
	assertFeedbackField(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu","account_id":"",
			"metric":"read","value":null,"unit":"","stat_window":"","sampled_at":"","evidence_note":""}`,
			publicationID), "sampled_at")

	// Step 5: over the length limit, named.
	long := ""
	for range feedbacklearning.MaxShortRunes + 1 {
		long += "次"
	}
	assertFeedbackField(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu","account_id":"",
			"metric":"read","value":null,"unit":%q,"stat_window":"","sampled_at":%q,"evidence_note":""}`,
			publicationID, long, time.Now().UTC().Format(time.RFC3339)), "unit")
}

func assertFeedbackField(t *testing.T, handler http.HandlerFunc, wsID, path, body, field string) {
	t.Helper()
	response := feedbackPost(t, handler, wsID, path, body)
	response.Want(http.StatusBadRequest)
	var got map[string]any
	response.JSON(&got)
	if got["field"] != field {
		t.Errorf("the refusal named %v, want %q (body was %s)", got["field"], field, body)
	}
}

// SOP 10.1: "未知填空；0 只表示已确认的零值", all the way through the boundary.
func TestTheBoundaryKeepsAnUnknownValueApartFromZero(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-null", "verified_published", true)
	h := feedbackHandler(t)

	var unknown map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "conversion", "null")).
		Want(http.StatusCreated).JSON(&unknown)
	if unknown["value"] != nil {
		t.Errorf("an unknown value came back as %v (%T)", unknown["value"], unknown["value"])
	}

	var zero map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "follow", "0")).
		Want(http.StatusCreated).JSON(&zero)
	if zero["value"] == nil {
		t.Fatal("a confirmed zero came back as null")
	}
	if number, ok := zero["value"].(float64); !ok || number != 0 {
		t.Errorf("a confirmed zero came back as %v (%T)", zero["value"], zero["value"])
	}

	// And the list keeps them apart too - this is where a "default to 0" in a
	// serializer would show up.
	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-metrics?publication_record_id="+publicationID, ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	var listed struct {
		Metrics []struct {
			Metric string   `json:"metric"`
			Value  *float64 `json:"value"`
		} `json:"metrics"`
	}
	testutil.Call(t, h.ListContentMetrics, req).Want(http.StatusOK).JSON(&listed)
	byMetric := map[string]*float64{}
	for _, item := range listed.Metrics {
		byMetric[item.Metric] = item.Value
	}
	if byMetric["conversion"] != nil {
		t.Error("the unknown value read back as a number")
	}
	if byMetric["follow"] == nil || *byMetric["follow"] != 0 {
		t.Error("the confirmed zero read back as unknown")
	}
}

// The server decides the source from which endpoint was called. A body field
// saying otherwise is not even accepted - the decoder rejects unknown fields.
func TestTheSourceTypeIsWrittenByTheEndpointNotTheBody(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-source", "verified_published", true)
	h := feedbackHandler(t)

	var manual map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "like", "5")).
		Want(http.StatusCreated).JSON(&manual)
	if manual["source_type"] != "manual" {
		t.Errorf("the form endpoint stored %v", manual["source_type"])
	}

	var imported map[string]any
	feedbackPost(t, h.ImportContentMetrics, wsID, "/api/content-metrics/import",
		fmt.Sprintf(`{"metrics":[%s]}`, metricBody(publicationID, "share", "2"))).
		Want(http.StatusCreated).JSON(&imported)
	metrics, _ := imported["metrics"].([]any)
	if len(metrics) != 1 {
		t.Fatalf("import returned %v", imported)
	}
	first, _ := metrics[0].(map[string]any)
	if first["source_type"] != "csv_import" {
		t.Errorf("the import endpoint stored %v", first["source_type"])
	}

	// A body that tries to declare its own origin does not get through at all.
	body := fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu",
		"account_id":"","metric":"read","value":1,"unit":"","stat_window":"",
		"sampled_at":%q,"evidence_note":"","source_type":"manual"}`,
		publicationID, time.Now().UTC().Format(time.RFC3339))
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics", body).
		Want(http.StatusBadRequest)
}

// All or nothing, and the refusal says which line of the paste was wrong.
func TestAnImportWithOneBadRowWritesNothingAndNamesTheRow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-batch", "verified_published", true)
	h := feedbackHandler(t)

	body := fmt.Sprintf(`{"metrics":[%s,%s,%s]}`,
		metricBody(publicationID, "read", "100"),
		metricBody(publicationID, "engagement", "200"),
		metricBody(publicationID, "like", "300"))
	response := feedbackPost(t, h.ImportContentMetrics, wsID, "/api/content-metrics/import", body)
	response.Want(http.StatusBadRequest)
	var refused map[string]any
	response.JSON(&refused)
	if refused["field"] != "metric" {
		t.Errorf("the refusal named %v, want metric", refused["field"])
	}
	if row, ok := refused["row"].(float64); !ok || row != 2 {
		t.Errorf("the refusal says row %v, want 2", refused["row"])
	}

	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_manual_metric
		WHERE workspace_id=$1`, wsID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d rows landed from a batch that was refused", count)
	}
}

// SOP 2's fifth workbench item, through the boundary.
func TestThePendingListIsPublishedRecordsWithNoMetrics(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-pending", "verified_published", true)
	h := feedbackHandler(t)

	pending := func() int {
		req := testutil.WithHeaders(
			testutil.JSONRequest("GET", "/api/content-feedback/pending", ""),
			"X-User-ID", testUserID, "X-Workspace-ID", wsID)
		var body struct {
			Count int `json:"count"`
		}
		testutil.Call(t, h.ListContentFeedbackPending, req).Want(http.StatusOK).JSON(&body)
		return body.Count
	}
	if got := pending(); got != 1 {
		t.Fatalf("a published record with no metrics gave count %d, want 1", got)
	}
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "10")).Want(http.StatusCreated)
	if got := pending(); got != 0 {
		t.Errorf("after recording a metric the count is %d, want 0", got)
	}
}

// R-045: the quote and the reading arrive and leave as two fields, and the AI
// review placeholder says pending_data rather than leaving the caller to guess.
func TestAnExcerptKeepsItsTwoHalvesAndTheReviewStateIsPendingData(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-excerpt", "verified_published", true)
	h := feedbackHandler(t)

	body := fmt.Sprintf(`{"publication_record_id":%q,"source_type":"comment",
		"redacted_excerpt":"看完就去买了","interpretation":"转化点在第三段",
		"tags":["转化"],"occurred_at":%q}`,
		publicationID, time.Now().UTC().Format(time.RFC3339))
	var written map[string]any
	feedbackPost(t, h.RecordContentFeedback, wsID, "/api/content-feedback", body).
		Want(http.StatusCreated).JSON(&written)
	if written["redacted_excerpt"] != "看完就去买了" || written["interpretation"] != "转化点在第三段" {
		t.Errorf("the two halves did not stay apart: %v", written)
	}

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-feedback?publication_record_id="+publicationID, ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	var listed map[string]any
	testutil.Call(t, h.ListContentFeedback, req).Want(http.StatusOK).JSON(&listed)
	if listed["review_state"] != "pending_data" {
		t.Errorf("the AI review state is %v, want pending_data", listed["review_state"])
	}
	excerpts, _ := listed["excerpts"].([]any)
	if len(excerpts) != 1 {
		t.Fatalf("got %d excerpts", len(excerpts))
	}
	first, _ := excerpts[0].(map[string]any)
	if first["tags"] == nil {
		t.Error("tags came back as null rather than an array")
	}
}

// Ruling Q2: the version is resolved on read, two hops, and "" is a real
// answer for a record entered for history.
func TestTheVersionResolvesThroughTwoHopsOrComesBackEmpty(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)

	wsID, publicationID, versionID := feedbackWorkspace(t, "feedback-version", "verified_published", true)
	var written map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "10")).Want(http.StatusCreated).JSON(&written)
	if written["version_id"] != versionID {
		t.Errorf("version resolved to %v, want %q", written["version_id"], versionID)
	}

	historyWS, historyPub, _ := feedbackWorkspace(t, "feedback-history", "reported_published", false)
	var historical map[string]any
	feedbackPost(t, h.RecordContentMetric, historyWS, "/api/content-metrics",
		metricBody(historyPub, "read", "10")).Want(http.StatusCreated).JSON(&historical)
	if historical["version_id"] != "" {
		t.Errorf("a record with no delivery task resolved to %v, want empty", historical["version_id"])
	}
}

// The feedback write paths follow the workspace delete/write protocol the
// other content modules follow: the write transaction takes
// LockWorkspaceForContentDiagnosticWrite before it touches a row, so a delete
// either waits for the write and sweeps it, or commits first and leaves the
// write with no workspace to attach to.
//
// Both tables carry a text workspace id and no foreign key, so nothing else
// would stop an orphan from being written after the delete commits (#104).
//
// This is also what the module's own fixture stands in for: it runs in an
// isolated schema with no workspace table, so the fence is proven here.
func TestFeedbackWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := feedbackHandler(t)
	store := h.feedbackStore()

	wsID, publicationID, _ := feedbackWorkspace(t, "feedback-fence", "verified_published", true)
	now := time.Now().UTC().Format(time.RFC3339)
	input := feedbacklearning.MetricInput{
		PublicationRecordID: publicationID, Platform: feedbacklearning.PlatformXiaohongshu,
		Metric: feedbacklearning.MetricRead, SampledAt: now,
	}
	if _, err := store.Record(ctx, wsID, testUserID, input, feedbacklearning.SourceManual); err != nil {
		t.Fatalf("record while the workspace exists: %v", err)
	}
	if _, err := store.Excerpt(ctx, wsID, testUserID, feedbacklearning.ExcerptInput{
		PublicationRecordID: publicationID, SourceType: feedbacklearning.ExcerptComment,
		RedactedExcerpt: "很好", OccurredAt: now,
	}); err != nil {
		t.Fatalf("excerpt while the workspace exists: %v", err)
	}

	count := func(table string) int {
		var total int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, wsID).Scan(&total); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return total
	}
	before := map[string]int{
		"content_manual_metric":    count("content_manual_metric"),
		"content_feedback_excerpt": count("content_feedback_excerpt"),
	}
	if before["content_manual_metric"] != 1 || before["content_feedback_excerpt"] != 1 {
		t.Fatalf("setup wrote %v", before)
	}

	// The delete commits on its own connection, exactly as a workspace
	// deletion that finished just before the next request arrived.
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, wsID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	for _, write := range []struct {
		name string
		call func() error
	}{
		{"record-metric", func() error {
			_, err := store.Record(ctx, wsID, testUserID, input, feedbacklearning.SourceManual)
			return err
		}},
		{"import-metrics", func() error {
			_, err := store.RecordBatch(ctx, wsID, testUserID,
				[]feedbacklearning.MetricInput{input}, feedbacklearning.SourceCSVImport)
			return err
		}},
		{"record-excerpt", func() error {
			_, err := store.Excerpt(ctx, wsID, testUserID, feedbacklearning.ExcerptInput{
				PublicationRecordID: publicationID, SourceType: feedbacklearning.ExcerptLead,
				Interpretation: "后来的判断", OccurredAt: now,
			})
			return err
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if err := write.call(); !errors.Is(err, feedbacklearning.ErrNotFound) {
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
