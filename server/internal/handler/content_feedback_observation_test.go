package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// SOP 3.2's 反馈观察时点, through the boundary (specs/029 PR 3).
//
// The module's own tests prove the three-valued rule; these prove it survives a
// real brand: the window is read out of the workspace's settings, and the
// answer changes what the fifth workbench item lists.

// observationWorkspace makes a brand with one published record whose
// publication time is `publishedDaysAgo` days in the past, and optionally an
// observation window in its settings.
func observationWorkspace(t *testing.T, slug string, publishedDaysAgo int, window string) (string, string) {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "observation test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)
	if window != "" {
		dbfx.Exec(t, `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, wsID, window)
	}

	publicationID := fmt.Sprintf("pub-%s", slug)
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a','','xiaohongshu','verified_published',$3,
		        'https://example.invalid/p/1', now() - make_interval(days => $4))`,
		publicationID, wsID, testUserID, publishedDaysAgo)

	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_manual_metric WHERE workspace_id = $1`,
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, wsID)
		}
	})
	return wsID, publicationID
}

func pendingCount(t *testing.T, h *Handler, wsID string) int {
	t.Helper()
	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-feedback/pending", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	var body struct {
		Count int `json:"count"`
	}
	testutil.Call(t, h.ListContentFeedbackPending, req).Want(http.StatusOK).JSON(&body)
	return body.Count
}

func pendingDue(t *testing.T, h *Handler, wsID string) string {
	t.Helper()
	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-feedback/pending", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	var body struct {
		Pending []struct {
			Due string `json:"due"`
		} `json:"pending"`
	}
	testutil.Call(t, h.ListContentFeedbackPending, req).Want(http.StatusOK).JSON(&body)
	if len(body.Pending) == 0 {
		return ""
	}
	return body.Pending[0].Due
}

const window14 = `{"loretide.operating_rules":{"observation":{"default":14}}}`

// The window elapsed: the record is waiting for its numbers.
func TestPendingListsARecordWhoseWindowHasPassed(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, _ := observationWorkspace(t, "obs-passed", 20, window14)

	if got := pendingCount(t, h, wsID); got != 1 {
		t.Fatalf("a record published 20 days ago with a 14-day window gave %d, want 1", got)
	}
	if got := pendingDue(t, h, wsID); got != "passed" {
		t.Errorf("due = %q, want passed", got)
	}
}

// Too early: the piece went out three days ago and the brand waits fourteen.
// Nothing is wrong with it, and listing it would be asking for numbers the
// platform has not produced yet.
func TestPendingHidesARecordWhoseWindowHasNotPassed(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, _ := observationWorkspace(t, "obs-notyet", 3, window14)

	if got := pendingCount(t, h, wsID); got != 0 {
		t.Errorf("a record published 3 days ago with a 14-day window gave %d, want 0", got)
	}
}

// The third state, and the one this whole card exists to get right.
//
// A brand that has set no window cannot be told whether anything is due - and
// that is NOT a reason to empty the workbench. Before specs/029 every record
// was listed; a brand that has not opted into a window keeps exactly that.
func TestPendingKeepsARecordWhenTheBrandHasSetNoWindow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	// Published three days ago: under any window a brand might plausibly set,
	// this would be "not yet". With no window set, it stays on the list.
	wsID, _ := observationWorkspace(t, "obs-nowindow", 3, "")

	if got := pendingCount(t, h, wsID); got != 1 {
		t.Fatalf("a brand with no observation window gave %d, want 1 - "+
			"records must not vanish because nobody has chosen a window", got)
	}
	if got := pendingDue(t, h, wsID); got != "unknown" {
		t.Errorf("due = %q, want unknown", got)
	}
}

// The other way to be unknown: 025 allows a publication record with no
// publication time, and without one there is no clock to start.
func TestPendingKeepsARecordWithNoPublicationTime(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, publicationID := observationWorkspace(t, "obs-notime", 3, window14)
	dbfx.Exec(t, `UPDATE content_publication_record SET published_at = NULL
		WHERE publication_record_id = $1`, publicationID)

	if got := pendingCount(t, h, wsID); got != 1 {
		t.Fatalf("a record with no publication time gave %d, want 1", got)
	}
	if got := pendingDue(t, h, wsID); got != "unknown" {
		t.Errorf("due = %q, want unknown", got)
	}
}

// A channel window overrides the brand's, both ways round.
func TestPendingUsesTheChannelWindowWhenThereIsOne(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	// Both directions, because a channel override that is merely ignored would
	// still look right in one of them. Same publication age in each case; only
	// the channel's number decides.
	//
	// Brand waits 14 days, Xiaohongshu waits 2: the channel's window has
	// elapsed although the brand's has not.
	shorter, _ := observationWorkspace(t, "obs-channel-short", 3,
		`{"loretide.operating_rules":{"observation":{"default":14,"by_channel":{"xiaohongshu":2}}}}`)
	if got := pendingCount(t, h, shorter); got != 1 {
		t.Errorf("the channel's 2-day window was not used: count %d, want 1", got)
	}
	if got := pendingDue(t, h, shorter); got != "passed" {
		t.Errorf("due = %q, want passed", got)
	}

	// Brand waits 2 days, Xiaohongshu waits 14: the brand's window has elapsed
	// and the channel's has not, so the record is not due yet.
	longer, _ := observationWorkspace(t, "obs-channel-long", 3,
		`{"loretide.operating_rules":{"observation":{"default":2,"by_channel":{"xiaohongshu":14}}}}`)
	if got := pendingCount(t, h, longer); got != 0 {
		t.Errorf("the channel's 14-day window was not used: count %d, want 0", got)
	}
}

// Recording a number still takes the record off the list, window or no window.
func TestRecordingAMetricStillClearsAPendingRecord(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, publicationID := observationWorkspace(t, "obs-recorded", 20, window14)

	if got := pendingCount(t, h, wsID); got != 1 {
		t.Fatalf("setup gave %d, want 1", got)
	}
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "10")).Want(http.StatusCreated)
	if got := pendingCount(t, h, wsID); got != 0 {
		t.Errorf("after recording a metric the count is %d, want 0", got)
	}
}

// The window is read from the brand's settings on every call, so changing it
// changes the list without anything being rewritten.
func TestChangingTheWindowChangesTheList(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, _ := observationWorkspace(t, "obs-changed", 5, window14)

	if got := pendingCount(t, h, wsID); got != 0 {
		t.Fatalf("published 5 days ago with a 14-day window gave %d, want 0", got)
	}
	dbfx.Exec(t, `UPDATE workspace SET settings =
		'{"loretide.operating_rules":{"observation":{"default":1}}}'::jsonb WHERE id = $1`, wsID)
	if got := pendingCount(t, h, wsID); got != 1 {
		t.Errorf("after shortening the window to 1 day the count is %d, want 1", got)
	}
}

// countingRulesDB wraps the pool and counts settings reads.
type countingRulesDB struct {
	inner workspacecore.Database
	reads int
}

func (c *countingRulesDB) Begin(ctx context.Context) (pgx.Tx, error) { return c.inner.Begin(ctx) }

func (c *countingRulesDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.reads++
	return c.inner.QueryRow(ctx, sql, args...)
}

// The pending list asks about the window once per row. Reading the brand's
// settings once per row would turn one page into a query per published piece,
// so the adapter reads them once and reuses the answer for the rest of the
// request.
func TestTheObservationAdapterReadsTheSettingsOncePerRequest(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _ := observationWorkspace(t, "obs-memo", 20, window14)

	counting := &countingRulesDB{inner: testPool}
	observation := feedbackObservation{
		store: &workspacecore.Store{DB: counting},
		state: newObservationState(),
	}

	published := time.Now().UTC().AddDate(0, 0, -20)
	for range 5 {
		if got := observation.DueFor(context.Background(), wsID, "xiaohongshu", &published); got != workspacecore.DuePassed {
			t.Fatalf("due = %q, want passed", got)
		}
	}
	if counting.reads != 1 {
		t.Errorf("the settings were read %d times for 5 rows, want 1", counting.reads)
	}
}

// A settings read that failed is remembered too: retrying it once per row
// would turn one database problem into a burst of them, and the answer would
// be DueUnknown every time anyway.
func TestTheObservationAdapterDoesNotRetryAFailedRead(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	counting := &countingRulesDB{inner: testPool}
	observation := feedbackObservation{
		store: &workspacecore.Store{DB: counting},
		state: newObservationState(),
	}

	published := time.Now().UTC().AddDate(0, 0, -20)
	for range 4 {
		// No such workspace, so the read cannot succeed. The answer must still
		// be "cannot tell" rather than "not yet" - a record must not leave the
		// workbench because a lookup failed.
		got := observation.DueFor(context.Background(), "00000000-0000-0000-0000-000000000000",
			"xiaohongshu", &published)
		if got != workspacecore.DueUnknown {
			t.Fatalf("due = %q, want unknown", got)
		}
	}
	if counting.reads != 1 {
		t.Errorf("a failing read was attempted %d times, want 1", counting.reads)
	}
}
