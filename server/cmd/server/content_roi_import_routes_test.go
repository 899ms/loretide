package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/034 PR 3 through the real router and middleware.

func roiImportRequest(t *testing.T, key, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/content-roi/imports", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, raw
}

// T068 workflow step 12, and FR-028 end to end: the Idempotency-Key header
// reaches the module through the real middleware - a replay answers the same
// bytes - and GET /imports/{batchId} answers for the batch in the path, which
// is a server-chosen id, not the workspace id. Another workspace's batch
// answers exactly like a missing one.
func TestContentROIImportThroughTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	for _, table := range []string{
		"content_roi_cost_revision", "content_roi_import_batch", "content_roi_import_claim",
	} {
		fx.Cleanup(t, `DELETE FROM `+table+` WHERE workspace_id=$1`, testWorkspaceID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	stamp := time.Now().UnixNano()
	key := fmt.Sprintf("route-key-%d", stamp)
	body := fmt.Sprintf(`{"record_kind":"cost","rows":[{"category":"拍摄-%d","pricing":"amount",
		"amount":"3000.00","currency":"CNY","incurred_at":"2026-09-10T02:00:00Z"}]}`, stamp)
	status, first := roiImportRequest(t, key, body)
	if status != http.StatusCreated {
		t.Fatalf("import = %d: %s", status, first)
	}
	status, replay := roiImportRequest(t, key, body)
	if status != http.StatusCreated || !bytes.Equal(first, replay) {
		t.Fatalf("replay = %d:\n%s\nwant\n%s", status, replay, first)
	}
	status, conflict := roiImportRequest(t, key, `{"record_kind":"cost","rows":[{"category":"other","pricing":"amount",
		"amount":"1.00","currency":"CNY","incurred_at":"2026-09-10T02:00:00Z"}]}`)
	if status != http.StatusConflict || !bytes.Contains(conflict, []byte(`"field":"Idempotency-Key"`)) {
		t.Fatalf("same key, other rows = %d: %s", status, conflict)
	}

	var batch struct {
		ImportBatchID string `json:"import_batch_id"`
		WorkspaceID   string `json:"workspace_id"`
	}
	if err := json.Unmarshal(first, &batch); err != nil || batch.ImportBatchID == "" {
		t.Fatalf("no batch id in %s (%v)", first, err)
	}
	if batch.ImportBatchID == testWorkspaceID {
		t.Fatal("the batch id equals the workspace id; the two cannot be told apart")
	}
	got := roiAPI(t, http.MethodGet, "/api/content-roi/imports/"+batch.ImportBatchID, "", http.StatusOK)
	if got["import_batch_id"] != batch.ImportBatchID || got["workspace_id"] != testWorkspaceID {
		t.Fatalf("GET /imports/{batchId} answered %v", got)
	}

	foreignBatch := fmt.Sprintf("batch-foreign-%d", stamp)
	if _, err := testPool.Exec(t.Context(), `INSERT INTO content_roi_import_batch
		(import_batch_id, workspace_id, record_kind, row_count, written_count, skipped_count, rows, recorded_by)
		VALUES ($1,$2,'cost',0,0,0,'[]','someone')`, foreignBatch, fmt.Sprintf("ws-foreign-%d", stamp)); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_roi_import_batch WHERE import_batch_id=$1`, foreignBatch)
	foreign := accountAPIRequest(t, http.MethodGet, "/api/content-roi/imports/"+foreignBatch, "")
	defer foreign.Body.Close()
	missing := accountAPIRequest(t, http.MethodGet, "/api/content-roi/imports/no-such-batch", "")
	defer missing.Body.Close()
	if foreign.StatusCode != http.StatusNotFound || missing.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign = %d, missing = %d, want 404 and 404", foreign.StatusCode, missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}
}
