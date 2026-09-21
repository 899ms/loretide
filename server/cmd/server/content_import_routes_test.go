package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

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
	created := accountAPIRequest(t, http.MethodPost, "/api/content-works",
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
	artifactID := createArtifactThroughTheAPI(t, work.WorkID)
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID,
		`{"draft_body":"这是两年前发出去的正文"}`)
	patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("paste the body = %d, want 200", patch.StatusCode)
	}

	// Step 3: the import endpoint. Not the save endpoint with a flag.
	imported := accountAPIRequest(t, http.MethodPost,
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
	recorded := accountAPIRequest(t, http.MethodPost, "/api/content-publications",
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

	created := accountAPIRequest(t, http.MethodPost, "/api/content-works",
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
	artifactID := createArtifactThroughTheAPI(t, work.WorkID)
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+work.WorkID+"/artifacts/"+artifactID, `{"draft_body":"正文"}`)
	patch.Body.Close()

	response := accountAPIRequest(t, http.MethodPost,
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
