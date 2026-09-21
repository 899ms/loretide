package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/idempotency"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// SOP 3.3's historical import, driven end to end through the real router and
// the real middleware (specs/031, workflow step 12).
//
// There is no import endpoint and no import module: the four writes are
// existing endpoints called in order by an adapter. What this test proves is
// that the order works over HTTP as a person would drive it, and that the
// facts §3.3 requires survive the round trip.

// importedWork returns the ids the import chain produces: work, artifact,
// version, publication record.
func runTheImportChain(t *testing.T, fx *testutil.Fixture) (string, string, string, string) {
	t.Helper()

	// Step 1: a work with NO topic card. This is the request that used to be
	// refused.
	created := historicalImportRequest(t, "work", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","snapshot_id":"","title":"两年前发的那篇","historical_import":true}`)
	defer created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(created.Body)
		t.Fatalf("create imported work = %d, want 201: %s", created.StatusCode, body)
	}
	var work struct {
		WorkID           string `json:"work_id"`
		TopicCardID      string `json:"topic_card_id"`
		HistoricalImport bool   `json:"historical_import"`
	}
	if err := json.NewDecoder(created.Body).Decode(&work); err != nil {
		t.Fatal(err)
	}
	if work.TopicCardID != "" || !work.HistoricalImport {
		t.Fatalf("created work topic_card_id=%q historical_import=%v, want empty and true",
			work.TopicCardID, work.HistoricalImport)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, work.WorkID)

	// Step 2: the document, and the pasted body into its editing copy.
	artifactID := createHistoricalArtifactThroughTheAPI(t, work.WorkID, "artifact")
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID,
		`{"draft_body":"这是两年前发出去的正文"}`)
	patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("paste the body = %d, want 200", patch.StatusCode)
	}

	// Step 3: the import endpoint. Not the save endpoint with a flag.
	imported := historicalImportRequest(t, "version", http.MethodPost,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID+"/versions/import", "")
	defer imported.Body.Close()
	if imported.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(imported.Body)
		t.Fatalf("import version = %d, want 201: %s", imported.StatusCode, body)
	}
	var version struct {
		VersionID    string `json:"version_id"`
		ArtifactID   string `json:"artifact_id"`
		WorkID       string `json:"work_id"`
		Source       string `json:"source"`
		Action       string `json:"action"`
		RestoredFrom string `json:"restored_from"`
		AdoptedFrom  string `json:"adopted_from"`
		Revision     int64  `json:"revision"`
	}
	if err := json.NewDecoder(imported.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	// Step 12: the response is about the ids the PATH named, not about
	// whatever the middleware happened to put in the context.
	if version.WorkID != work.WorkID || version.ArtifactID != artifactID {
		t.Fatalf("import answered for work %q artifact %q, path named %q / %q",
			version.WorkID, version.ArtifactID, work.WorkID, artifactID)
	}
	if version.Source != "edited" || version.Action != "imported" {
		t.Errorf("source/action = %q/%q, want edited/imported", version.Source, version.Action)
	}
	if version.RestoredFrom != "" || version.AdoptedFrom != "" {
		t.Errorf("restored_from=%q adopted_from=%q; an import points at nothing (不虚构版本链)",
			version.RestoredFrom, version.AdoptedFrom)
	}
	if version.Revision != 1 {
		t.Errorf("revision = %d, want 1: an imported document starts at its only version", version.Revision)
	}

	// Step 4: the publication record, pointing straight at that version, with
	// no review request and no delivery task anywhere.
	fx.Cleanup(t, `DELETE FROM content_publication_record WHERE workspace_id=$1`, testWorkspaceID)
	recorded := historicalImportRequest(t, "publication", http.MethodPost, "/api/content-publications",
		fmt.Sprintf(`{"artifact_id":%q,"channel":"xiaohongshu","status":"reported_published",
			"declared_by":"导入者本人","page_url_or_content_id":"https://example.test/note/31",
			"published_at":"2024-05-01T08:00:00Z","platform_account":"我的号",
			"version_id":%q,"historical_import":true}`, artifactID, version.VersionID))
	defer recorded.Body.Close()
	if recorded.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(recorded.Body)
		t.Fatalf("record publication = %d, want 201: %s", recorded.StatusCode, body)
	}
	var record struct {
		PublicationRecordID string `json:"publication_record_id"`
		DeliveryTaskID      string `json:"delivery_task_id"`
		VersionID           string `json:"version_id"`
		VersionMatch        string `json:"version_match"`
		HistoricalImport    bool   `json:"historical_import"`
		WorkID              string `json:"work_id"`
	}
	if err := json.NewDecoder(recorded.Body).Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.DeliveryTaskID != "" {
		t.Errorf("delivery_task_id = %q, want empty: nothing was handed over", record.DeliveryTaskID)
	}
	if record.VersionID != version.VersionID {
		t.Errorf("version_id = %q, want %q (SOP 3.3 发布后快照)", record.VersionID, version.VersionID)
	}
	if record.VersionMatch != "unknown" {
		t.Errorf("version_match = %q, want unknown", record.VersionMatch)
	}
	if !record.HistoricalImport {
		t.Error("the publication record is not marked as a historical import")
	}
	if record.WorkID != work.WorkID {
		t.Errorf("work_id = %q, want %q: the record resolved the wrong work", record.WorkID, work.WorkID)
	}
	return work.WorkID, artifactID, version.VersionID, record.PublicationRecordID
}

func historicalImportRequest(t *testing.T, key, method, path, body string) *http.Response {
	t.Helper()
	response, err := historicalImportHTTP(key, method, path, body, testToken, testWorkspaceID)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

func historicalImportHTTP(key, method, path, body, token, workspace string) (*http.Response, error) {
	req, err := http.NewRequest(method, testServer.URL+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", workspace)
	req.Header.Set("Idempotency-Key", key)
	return http.DefaultClient.Do(req)
}

func createHistoricalArtifactThroughTheAPI(t *testing.T, workID, key string) string {
	t.Helper()
	response := historicalImportRequest(t, key, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts", `{"kind":"body","title":"正文","position":1}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create historical artifact = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created.ArtifactID
}

func TestAHistoricalImportGoesThroughTheExistingEndpoints(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)
	workID, artifactID, versionID, _ := runTheImportChain(t, fx)

	// One version and only one. §3.3: 不虚构版本链.
	list := accountAPIRequest(t, http.MethodGet,
		"/api/content-works/"+workID+"/artifacts/"+artifactID+"/versions", "")
	defer list.Body.Close()
	var versions struct {
		Versions []struct {
			VersionID string `json:"version_id"`
			Action    string `json:"action"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(list.Body).Decode(&versions); err != nil {
		t.Fatal(err)
	}
	if len(versions.Versions) != 1 {
		t.Fatalf("the imported document has %d versions, want exactly 1", len(versions.Versions))
	}
	if versions.Versions[0].VersionID != versionID || versions.Versions[0].Action != "imported" {
		t.Errorf("the one version is %+v, want %q/imported", versions.Versions[0], versionID)
	}

	// No review request and no delivery task were created along the way.
	for _, probe := range []struct {
		path string
		key  string
	}{
		{"/api/content-reviews", "reviews"},
		{"/api/content-deliveries", "deliveries"},
	} {
		response := accountAPIRequest(t, http.MethodGet, probe.path, "")
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		var generic map[string][]json.RawMessage
		if err := json.Unmarshal(payload, &generic); err != nil {
			t.Fatalf("decode %s: %v", probe.path, err)
		}
		if len(generic[probe.key]) != 0 {
			t.Errorf("%s returned %d rows; an import creates neither", probe.path, len(generic[probe.key]))
		}
	}
}

func TestHistoricalImportWriteReplaysAndRejectsChangedInput(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_import_idempotency WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)
	body := `{"topic_card_id":"","title":"可重放历史作品","historical_import":true}`
	first := historicalImportRequest(t, "replay-work", http.MethodPost, "/api/content-works", body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(first.Body)
		t.Fatalf("first import work = %d, want 201: %s", first.StatusCode, payload)
	}
	var firstWork struct {
		WorkID string `json:"work_id"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstWork); err != nil {
		t.Fatal(err)
	}
	second := historicalImportRequest(t, "replay-work", http.MethodPost, "/api/content-works", body)
	defer second.Body.Close()
	if second.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(second.Body)
		t.Fatalf("replayed import work = %d, want 201: %s", second.StatusCode, payload)
	}
	var secondWork struct {
		WorkID string `json:"work_id"`
	}
	if err := json.NewDecoder(second.Body).Decode(&secondWork); err != nil {
		t.Fatal(err)
	}
	if secondWork.WorkID != firstWork.WorkID {
		t.Fatalf("replay work_id=%q, want original %q", secondWork.WorkID, firstWork.WorkID)
	}
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_work
		WHERE workspace_id=$1 AND title='可重放历史作品'`, testWorkspaceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("historical work side effects=%d, want 1", count)
	}
	conflict := historicalImportRequest(t, "replay-work", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"changed","historical_import":true}`)
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(conflict.Body)
		t.Fatalf("changed replay = %d, want 409: %s", conflict.StatusCode, payload)
	}
}

func TestHistoricalArtifactWithoutIdempotencyKeyRemainsCompatible(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)
	created := historicalImportRequest(t, "compatible-historical-work", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"兼容既有文稿创建","historical_import":true}`)
	defer created.Body.Close()
	var work struct {
		WorkID string `json:"work_id"`
	}
	if created.StatusCode != http.StatusCreated || json.NewDecoder(created.Body).Decode(&work) != nil {
		t.Fatalf("create historical work = %d", created.StatusCode)
	}

	// Existing editor callers add a document to a historical work without this
	// new header. They remain ordinary creates; a supplied key opts into replay.
	artifact := accountAPIRequest(t, http.MethodPost,
		"/api/content-works/"+work.WorkID+"/artifacts", `{"kind":"body","title":"既有调用方正文","position":1}`)
	defer artifact.Body.Close()
	if artifact.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(artifact.Body)
		t.Fatalf("headerless historical artifact = %d, want 201: %s", artifact.StatusCode, payload)
	}
}

// This is deliberately one real-router chain rather than four store-only
// examples. The four historical-import writes are an adapter workflow, so each
// one needs a live database proof of the failure mode it is responsible for:
// a lost create response, concurrent artifact callers, a mutable draft after a
// version response, and a changed publication request.
func TestHistoricalImportIdempotencyCoversTheFourWrites(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_import_idempotency WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_publication_record WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)

	// Step 1: the process committed but the client never read the first body.
	// Retrying the same key must recover that original response, not make work 2.
	firstWork := historicalImportRequest(t, "lost-work-response", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"丢失响应也可恢复","historical_import":true}`)
	if firstWork.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(firstWork.Body)
		firstWork.Body.Close()
		t.Fatalf("first work = %d, want 201: %s", firstWork.StatusCode, payload)
	}
	firstWork.Body.Close() // Simulate the response being lost after commit.
	replayedWork := historicalImportRequest(t, "lost-work-response", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"丢失响应也可恢复","historical_import":true}`)
	defer replayedWork.Body.Close()
	var work struct {
		WorkID string `json:"work_id"`
	}
	if err := json.NewDecoder(replayedWork.Body).Decode(&work); err != nil || work.WorkID == "" {
		t.Fatalf("lost-response replay = %d, decode=%v, work=%+v", replayedWork.StatusCode, err, work)
	}

	// Step 2: simultaneous retries must all receive the one original artifact.
	const callers = 6
	type artifactResult struct {
		status int
		body   []byte
		err    error
	}
	results := make(chan artifactResult, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := historicalImportHTTP("concurrent-artifact", http.MethodPost,
				"/api/content-works/"+work.WorkID+"/artifacts", `{"kind":"body","title":"并发导入正文","position":1}`,
				testToken, testWorkspaceID)
			if err != nil {
				results <- artifactResult{err: err}
				return
			}
			payload, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			results <- artifactResult{status: response.StatusCode, body: payload, err: readErr}
		}()
	}
	wg.Wait()
	close(results)
	artifactIDs := map[string]bool{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent artifact request: %v", result.err)
		}
		if result.status != http.StatusCreated {
			t.Fatalf("concurrent artifact = %d, want 201: %s", result.status, result.body)
		}
		var artifact struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal(result.body, &artifact); err != nil || artifact.ArtifactID == "" {
			t.Fatalf("decode concurrent artifact: %v, %s", err, result.body)
		}
		artifactIDs[artifact.ArtifactID] = true
	}
	if len(artifactIDs) != 1 {
		t.Fatalf("concurrent artifact ids = %v, want one", artifactIDs)
	}
	var artifactID string
	for artifactID = range artifactIDs {
	}
	var artifactCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_artifact
		WHERE workspace_id=$1 AND work_id=$2`, testWorkspaceID, work.WorkID).Scan(&artifactCount); err != nil {
		t.Fatal(err)
	}
	if artifactCount != 1 {
		t.Fatalf("concurrent artifact side effects=%d, want 1", artifactCount)
	}

	// Step 3: version import chooses the first snapshot as its replay contract.
	// After the response, a person may continue editing; a retry must not turn
	// that later draft into another imported version.
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID, `{"draft_body":"首次导入快照"}`)
	patch.Body.Close()
	firstVersion := historicalImportRequest(t, "version-first-snapshot", http.MethodPost,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID+"/versions/import", "")
	defer firstVersion.Body.Close()
	var version struct {
		VersionID string `json:"version_id"`
		Body      string `json:"body"`
	}
	if firstVersion.StatusCode != http.StatusCreated || json.NewDecoder(firstVersion.Body).Decode(&version) != nil {
		t.Fatalf("first import version = %d", firstVersion.StatusCode)
	}
	patch = accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID, `{"draft_body":"后来修改的草稿"}`)
	patch.Body.Close()
	replayedVersion := historicalImportRequest(t, "version-first-snapshot", http.MethodPost,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID+"/versions/import", "")
	defer replayedVersion.Body.Close()
	var replayed struct {
		VersionID string `json:"version_id"`
		Body      string `json:"body"`
	}
	if replayedVersion.StatusCode != http.StatusCreated || json.NewDecoder(replayedVersion.Body).Decode(&replayed) != nil {
		t.Fatalf("replayed import version = %d", replayedVersion.StatusCode)
	}
	if replayed.VersionID != version.VersionID || replayed.Body != "首次导入快照" {
		t.Fatalf("changed-draft replay = %+v, want original %+v", replayed, version)
	}

	// Step 4: unlike the version's stable first-snapshot contract, publication
	// input is immutable statement data. Changing it under one key is a 409.
	publicationBody := fmt.Sprintf(`{"artifact_id":%q,"channel":"xiaohongshu","status":"reported_published",
		"declared_by":"导入者","page_url_or_content_id":"https://example.test/idempotency",
		"version_id":%q,"historical_import":true}`, artifactID, version.VersionID)
	firstPublication := historicalImportRequest(t, "publication-conflict", http.MethodPost,
		"/api/content-publications", publicationBody)
	firstPublication.Body.Close()
	changedPublication := historicalImportRequest(t, "publication-conflict", http.MethodPost,
		"/api/content-publications", strings.Replace(publicationBody, "导入者", "另一位导入者", 1))
	defer changedPublication.Body.Close()
	if changedPublication.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(changedPublication.Body)
		t.Fatalf("changed publication = %d, want 409: %s", changedPublication.StatusCode, payload)
	}

	// A non-member cannot use a known key to discover its completed response.
	email := fmt.Sprintf("historical-import-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Historical Import Outsider", email)
	token, err := generateTestJWT(outsider, email, "Historical Import Outsider")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := historicalImportHTTP("lost-work-response", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"丢失响应也可恢复","historical_import":true}`, token, testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Body.Close()
	if foreign.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(foreign.Body)
		t.Fatalf("outsider replay = %d, want 404: %s", foreign.StatusCode, payload)
	}
}

func TestHistoricalImportIdempotencyClaimRollsBackWithItsWrite(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	request, err := idempotency.NewRequest("create-artifact", "rollback-work", "rollback-key",
		map[string]string{"title": "回滚正文"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, replayed, err := idempotency.Claim(t.Context(), tx, testWorkspaceID, request); err != nil || replayed {
		_ = tx.Rollback(t.Context())
		t.Fatalf("claim = replayed=%v, err=%v", replayed, err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_import_idempotency
		WHERE workspace_id=$1 AND operation='create-artifact' AND resource_scope='rollback-work'
		  AND idempotency_key='rollback-key'`, testWorkspaceID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("rolled-back import claim left %d replay rows", rows)
	}
}

// specs/031 FR-034 over HTTP: the imported work is in the workspace list and
// not in any card's list.
//
// The module test asserts the same thing against the store. This one asserts
// the query parameter the adapter actually sends arrives as a filter, which is
// a different failure - ListContentWorks reading the wrong query key would
// pass the module test and fail here.
func TestAnImportedWorkIsNotListedUnderATopicCardOverHTTP(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)

	cardID, _ := startTopicWithBrief(t)
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, cardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, cardID)

	onCardID := createWorkThroughTheAPI(t, fx, cardID)
	importedID, _, _, _ := runTheImportChain(t, fx)

	byCard := listWorkIDs(t, "/api/content-works?topic_card_id="+cardID)
	if contains(byCard, importedID) {
		t.Error("the imported work is listed under a topic card it does not belong to")
	}
	// The positive half: without it this passes against a filter that returns
	// nothing at all.
	if !contains(byCard, onCardID) {
		t.Fatal("the work that IS on this card is missing; the assertion above proves nothing")
	}

	all := listWorkIDs(t, "/api/content-works")
	if !contains(all, importedID) {
		t.Error("the imported work is missing from the workspace list; only the BY-CARD reading excludes it")
	}
	if !contains(all, onCardID) {
		t.Error("the ordinary work is missing from the workspace list")
	}
}

func listWorkIDs(t *testing.T, path string) []string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodGet, path, "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s = %d, want 200: %s", path, response.StatusCode, body)
	}
	var listed struct {
		Works []struct {
			WorkID string `json:"work_id"`
		} `json:"works"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(listed.Works))
	for _, work := range listed.Works {
		ids = append(ids, work.WorkID)
	}
	return ids
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// The version's own provenance is not something a caller can state.
//
// A handler that accepted {"action":"adopted"} on the import endpoint would
// let any caller claim any version had been approved as a baseline. The body
// is ignored entirely, which is what makes "which endpoint was called" the
// record of what happened.
func TestTheImportEndpointIgnoresAnyProvenanceInTheBody(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE workspace_id=$1`, testWorkspaceID)

	created := historicalImportRequest(t, "ignored-provenance-work", http.MethodPost, "/api/content-works",
		`{"topic_card_id":"","title":"历史作品","historical_import":true}`)
	var work struct {
		WorkID string `json:"work_id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&work); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, work.WorkID)
	artifactID := createHistoricalArtifactThroughTheAPI(t, work.WorkID, "ignored-provenance-artifact")
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID, `{"draft_body":"正文"}`)
	patch.Body.Close()

	response := historicalImportRequest(t, "ignored-provenance-version", http.MethodPost,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID+"/versions/import",
		`{"source":"generated","action":"adopted","created_at":"2024-01-01T00:00:00Z"}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("import = %d, want 201: %s", response.StatusCode, body)
	}
	var version struct {
		Source string `json:"source"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if version.Source != "edited" || version.Action != "imported" {
		t.Errorf("the body was believed: source/action = %q/%q, want edited/imported",
			version.Source, version.Action)
	}
}
