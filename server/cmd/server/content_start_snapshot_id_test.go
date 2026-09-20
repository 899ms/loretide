package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Workflow step 12 for EP-04b's three new path parameters.
//
// Route-existence is not enough: a handler that read the workspace id where it
// meant to read the path parameter would still be mounted, still answer, and
// still look right in a test whose two values happen to be equal. Every case
// here uses a path value that differs from the context value.
//
// Contract: specs/023-ep04b-start-snapshot/contracts/start-snapshot.md

// startTopicWithBrief creates a topic card through the API and freezes its
// first brief revision, returning both server-chosen ids.
func startTopicWithBrief(t *testing.T) (string, string) {
	t.Helper()
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
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create topic = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		TopicCardID string `json:"topic_card_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	action := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+created.TopicCardID+"/actions", `{
		"action":"start",
		"brief":{
			"audience":"new managers","core_problem":"focus","claim_and_boundaries":"narrow first",
			"channels":["zhihu"],"format":"post","structure":"three parts",
			"citation_requirements":"none","source_scope":"手头已有的材料",
			"deliverable":"one post","time_limit":"two hours","cost_limit":"none"
		}}`)
	defer action.Body.Close()
	if action.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(action.Body)
		t.Fatalf("freeze brief = %d, want 200: %s", action.StatusCode, body)
	}
	var result struct {
		Brief struct {
			BriefRevisionID string `json:"brief_revision_id"`
		} `json:"brief_revision"`
	}
	if err := json.NewDecoder(action.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Brief.BriefRevisionID == "" {
		t.Fatal("the start action returned no brief revision id")
	}
	return created.TopicCardID, result.Brief.BriefRevisionID
}

func TestStartSnapshotPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	cardID, briefID := startTopicWithBrief(t)
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, cardID)
	fx.Cleanup(t, `DELETE FROM content_start_snapshot WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, cardID)

	// Three ids, none of them the workspace id, and all three different from
	// each other. A handler that reached for the wrong one cannot pass.
	for name, id := range map[string]string{"topic card": cardID, "brief revision": briefID} {
		if id == "" || id == testWorkspaceID {
			t.Fatalf("%s id %q cannot prove path/context separation", name, id)
		}
	}
	if cardID == briefID {
		t.Fatal("the card and revision ids are equal; the two parameters cannot be told apart")
	}

	accountID := createAccountThroughTheAPI(t, fmt.Sprintf("start-snapshot-%s", briefID[:8]))
	fx.Cleanup(t, `DELETE FROM content_account_revision WHERE account_id=$1`, accountID)
	fx.Cleanup(t, `DELETE FROM content_account WHERE account_id=$1`, accountID)
	confirmMinimumProfile(t, accountID)

	start := accountAPIRequest(t, http.MethodPost,
		"/api/content-topics/"+cardID+"/briefs/"+briefID+"/start",
		fmt.Sprintf(`{"account_id":%q,"source_scope":"web","project_id":""}`, accountID))
	defer start.Body.Close()
	if start.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(start.Body)
		t.Fatalf("start = %d, want 201: %s", start.StatusCode, body)
	}
	var snapshot struct {
		SnapshotID      string `json:"snapshot_id"`
		TopicCardID     string `json:"topic_card_id"`
		BriefRevisionID string `json:"brief_revision_id"`
		WorkspaceID     string `json:"workspace_id"`
	}
	if err := json.NewDecoder(start.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.TopicCardID != cardID {
		t.Errorf("path named card %q, snapshot recorded %q", cardID, snapshot.TopicCardID)
	}
	if snapshot.BriefRevisionID != briefID {
		t.Errorf("path named revision %q, snapshot recorded %q", briefID, snapshot.BriefRevisionID)
	}
	if snapshot.WorkspaceID != testWorkspaceID {
		t.Errorf("snapshot workspace %q, want the context workspace %q", snapshot.WorkspaceID, testWorkspaceID)
	}
	if snapshot.SnapshotID == "" || snapshot.SnapshotID == cardID ||
		snapshot.SnapshotID == briefID || snapshot.SnapshotID == testWorkspaceID {
		t.Fatalf("snapshot id %q cannot prove path/context separation", snapshot.SnapshotID)
	}

	// {snapshotId} is a new class of id and gets its own case: reading it back
	// must return the one the path names, and nothing else.
	read := accountAPIRequest(t, http.MethodGet,
		"/api/content-topics/"+cardID+"/snapshots/"+snapshot.SnapshotID, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(read.Body)
		t.Fatalf("GET snapshot = %d, want 200: %s", read.StatusCode, body)
	}
	var got struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(read.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.SnapshotID != snapshot.SnapshotID {
		t.Fatalf("path named %q, response returned %q", snapshot.SnapshotID, got.SnapshotID)
	}

	// A snapshot id that is real but belongs to a different card must be
	// refused exactly as a missing one: an id must not be usable to probe.
	otherCard, _ := startTopicWithBrief(t)
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, otherCard)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, otherCard)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, otherCard)
	foreign := accountAPIRequest(t, http.MethodGet,
		"/api/content-topics/"+otherCard+"/snapshots/"+snapshot.SnapshotID, "")
	defer foreign.Body.Close()
	if foreign.StatusCode != http.StatusNotFound {
		t.Fatalf("a snapshot read under another card = %d, want 404", foreign.StatusCode)
	}
	missing := accountAPIRequest(t, http.MethodGet,
		"/api/content-topics/"+cardID+"/snapshots/no-such-snapshot", "")
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown snapshot = %d, want 404", missing.StatusCode)
	}

	// The two refusals must be byte-identical apart from the trace id, or the
	// difference itself says whether the id exists somewhere.
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}
}

// confirmMinimumProfile confirms exactly SOP 3.1's four minimum fields, so the
// account can start.
func confirmMinimumProfile(t *testing.T, accountID string) {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost,
		"/api/content-accounts/"+accountID+"/profile", `{
		"audience":{"value":"new managers","status":"confirmed"},
		"common_questions":{"value":"","status":"pending"},
		"experience":{"value":"","status":"pending"},
		"positioning":{"value":"","status":"pending"},
		"content_pillars":{"value":"one-on-ones","status":"confirmed"},
		"expression_style":{"value":"","status":"pending"},
		"forbidden_expressions":{"value":"","status":"pending"},
		"content_goals":{"value":"","status":"pending"},
		"primary_channels":{"values":["zhihu"],"status":"confirmed"},
		"weekly_hours":{"value":6,"status":"confirmed"},
		"style_samples":{"values":[],"status":"pending"}
	}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("confirm profile = %d, want 201: %s", response.StatusCode, body)
	}
}

func refusalsMatchApartFromTrace(t *testing.T, first, second *http.Response) bool {
	t.Helper()
	return decodedRefusal(t, first) == decodedRefusal(t, second)
}

func decodedRefusal(t *testing.T, response *http.Response) string {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	// The trace id is per-request by design and is the one field that must
	// differ; everything else has to be identical.
	delete(body, "trace_id")
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
