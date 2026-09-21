package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// SOP 3.3's 发布后快照: a publication record can name its version directly,
// and the resolver reads that before walking the two hops.
//
// Contract: specs/031-historical-import/contracts/historical-import.md
// (specs/031 FR-011a, FR-011b)

// importedPublication makes a brand with one publication record that states
// its version directly. directVersion is what the record names; taskVersion,
// when non-empty, is what a delivery task and review request behind it would
// resolve to instead.
func importedPublication(t *testing.T, slug, directVersion, taskVersion string) (string, string) {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "import test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)

	publicationID := fmt.Sprintf("pub-%s", slug)
	deliveryTaskID := ""
	if taskVersion != "" {
		deliveryTaskID = fmt.Sprintf("task-%s", slug)
		reviewID := fmt.Sprintf("review-%s", slug)
		dbfx.Exec(t, `INSERT INTO content_review_request
			(review_request_id, workspace_id, work_id, artifact_id, version_id,
			 account_id, channel, snapshot, status, requested_by)
			VALUES ($1,$2,'w','a',$3,'acct-1','xiaohongshu','{}'::jsonb,'approved',$4)`,
			reviewID, wsID, taskVersion, testUserID)
		dbfx.Exec(t, `INSERT INTO content_delivery_task
			(delivery_task_id, workspace_id, work_id, artifact_id, review_request_id,
			 channel, status)
			VALUES ($1,$2,'w','a',$3,'xiaohongshu','handed_off')`,
			deliveryTaskID, wsID, reviewID)
	}
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at,
		 version_id, historical_import)
		VALUES ($1,$2,'w','a',$3,'xiaohongshu','reported_published',$4,
			'https://example.invalid/p/31', now(), $5, true)`,
		publicationID, wsID, deliveryTaskID, testUserID, directVersion)

	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
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
	return wsID, publicationID
}

// A record that states its version is answered with it, with no delivery task
// to walk. This is the whole point of the column: an import has no review
// request and no handover task, and §3.3 forbids inventing them.
func TestAStatedVersionIsResolvedWithoutAnyHops(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, publicationID := importedPublication(t, "import-direct", "version-imported", "")

	var written map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "1200")).Want(http.StatusCreated).JSON(&written)
	if written["version_id"] != "version-imported" {
		t.Errorf("version resolved to %v, want the stated version-imported", written["version_id"])
	}
}

// FR-011a: when both exist, the STATED version wins.
//
// The order is the whole assertion. A resolver that walked the hops first
// would pass every other test here - the two only disagree on a record that
// has both, which is exactly what this builds.
func TestAStatedVersionBeatsTheTwoHopsWhenBothExist(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	wsID, publicationID := importedPublication(t, "import-both", "version-stated", "version-from-task")

	var written map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "1200")).Want(http.StatusCreated).JSON(&written)
	if written["version_id"] == "version-from-task" {
		t.Fatal("the two hops answered; the stated version is what somebody said about THIS publication")
	}
	if written["version_id"] != "version-stated" {
		t.Errorf("version resolved to %v, want version-stated", written["version_id"])
	}
}

// FR-011b: the fallback still works, and "" is still never a refusal.
//
// Without this, the change above could have replaced the two hops rather than
// preceding them, and every record made before migration 534 would silently
// stop resolving.
func TestTheTwoHopsStillAnswerWhenNoVersionIsStated(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)

	wsID, publicationID := importedPublication(t, "import-fallback", "", "version-from-task")
	var viaHops map[string]any
	feedbackPost(t, h.RecordContentMetric, wsID, "/api/content-metrics",
		metricBody(publicationID, "read", "10")).Want(http.StatusCreated).JSON(&viaHops)
	if viaHops["version_id"] != "version-from-task" {
		t.Errorf("version resolved to %v, want version-from-task", viaHops["version_id"])
	}

	// Neither route answers. The recording still succeeds: the numbers are
	// real, we just do not know which version they belong to.
	emptyWS, emptyPub := importedPublication(t, "import-neither", "", "")
	var unresolved map[string]any
	feedbackPost(t, h.RecordContentMetric, emptyWS, "/api/content-metrics",
		metricBody(emptyPub, "read", "10")).Want(http.StatusCreated).JSON(&unresolved)
	if unresolved["version_id"] != "" {
		t.Errorf("version resolved to %v, want empty", unresolved["version_id"])
	}
}
