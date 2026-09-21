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

var contentWorkRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-works/", "/api/content-works"},
	{http.MethodPost, "/api/content-works/", "/api/content-works"},
	{http.MethodGet, "/api/content-works/{id}/", "/api/content-works/work-1"},
	{http.MethodPatch, "/api/content-works/{id}/", "/api/content-works/work-1"},
	{http.MethodGet, "/api/content-works/{id}/artifacts", "/api/content-works/work-1/artifacts"},
	{http.MethodPost, "/api/content-works/{id}/artifacts", "/api/content-works/work-1/artifacts"},
	{http.MethodPatch, "/api/content-works/{id}/artifacts/{artifactId}", "/api/content-works/work-1/artifacts/art-1"},
	{http.MethodGet, "/api/content-works/{id}/artifacts/{artifactId}/versions", "/api/content-works/work-1/artifacts/art-1/versions"},
	{http.MethodPost, "/api/content-works/{id}/artifacts/{artifactId}/versions", "/api/content-works/work-1/artifacts/art-1/versions"},
	// specs/031: a separate entry point, because which one was called is what
	// the server records as the version's provenance.
	{http.MethodPost, "/api/content-works/{id}/artifacts/{artifactId}/versions/import", "/api/content-works/work-1/artifacts/art-1/versions/import"},
	{http.MethodGet, "/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}", "/api/content-works/work-1/artifacts/art-1/versions/ver-1"},
	{http.MethodPost, "/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}/restore", "/api/content-works/work-1/artifacts/art-1/versions/ver-1/restore"},
	{http.MethodPost, "/api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}/adopt", "/api/content-works/work-1/artifacts/art-1/versions/ver-1/adopt"},
}

func TestContentWorkEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentWorkRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	// There must be no way to delete a version. SOP 7.1 keeps approved, handed
	// over and published versions, and the way to keep them is for no path to
	// exist that could remove one.
	for route := range mounted {
		if strings.HasPrefix(route, http.MethodDelete+" /api/content-works/") {
			t.Errorf("a delete route exists on the work editor: %s", route)
		}
		if route == http.MethodPatch+" /api/content-works/{id}/artifacts/{artifactId}/versions/{versionId}" {
			t.Errorf("a version can be patched: %s", route)
		}
	}
}

func TestContentWorkEndpointsRejectUnauthenticatedCallers(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	for _, route := range contentWorkRoutes {
		req := testutil.JSONRequest(route.method, route.path, `{}`)
		testutil.Call(t, router.ServeHTTP, req).Want(http.StatusUnauthorized)
	}
}

func TestContentWorkEndpointsHideTheWorkspaceFromNonMembers(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-work-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Work Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Work Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentWorkRoutes {
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

// Workflow step 12 for the three path parameters this feature adds.
//
// Route existence is not enough: a handler that read the workspace id where it
// meant to read a path parameter would still be mounted, still answer, and
// still look right in a test whose two values happen to be equal. Every id here
// is server-chosen, none equals the workspace id, and all three differ from
// each other.
func TestWorkEditorPathIDsSurviveTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	cardID, _ := startTopicWithBrief(t)
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, cardID)
	fx.Cleanup(t, `DELETE FROM content_brief_revision WHERE topic_card_id=$1`, cardID)
	fx.Cleanup(t, `DELETE FROM content_topic_card WHERE topic_card_id=$1`, cardID)

	workID := createWorkThroughTheAPI(t, fx, cardID)
	artifactID := createArtifactThroughTheAPI(t, workID)

	// Type something, then save it as a version.
	patch := accountAPIRequest(t, http.MethodPatch,
		"/api/content-works/"+workID+"/artifacts/"+artifactID,
		`{"draft_body":"第一稿"}`)
	defer patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patch.Body)
		t.Fatalf("autosave = %d, want 200: %s", patch.StatusCode, body)
	}
	var patched struct {
		ArtifactID  string `json:"artifact_id"`
		WorkID      string `json:"work_id"`
		DraftStatus string `json:"draft_status"`
	}
	if err := json.NewDecoder(patch.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.ArtifactID != artifactID || patched.WorkID != workID {
		t.Fatalf("autosave answered for work %q artifact %q, path named %q / %q",
			patched.WorkID, patched.ArtifactID, workID, artifactID)
	}
	if patched.DraftStatus != "working" {
		t.Errorf("after typing draft_status = %q, want working", patched.DraftStatus)
	}

	saved := accountAPIRequest(t, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts/"+artifactID+"/versions", "")
	defer saved.Body.Close()
	if saved.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(saved.Body)
		t.Fatalf("save version = %d, want 201: %s", saved.StatusCode, body)
	}
	var version struct {
		VersionID  string `json:"version_id"`
		ArtifactID string `json:"artifact_id"`
		WorkID     string `json:"work_id"`
		Source     string `json:"source"`
		Action     string `json:"action"`
	}
	if err := json.NewDecoder(saved.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	fx.Cleanup(t, `DELETE FROM content_artifact_version WHERE work_id=$1`, workID)
	fx.Cleanup(t, `DELETE FROM content_artifact WHERE work_id=$1`, workID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE work_id=$1`, workID)

	for name, id := range map[string]string{
		"work": workID, "artifact": artifactID, "version": version.VersionID,
	} {
		if id == "" || id == testWorkspaceID {
			t.Fatalf("%s id %q cannot prove path/context separation", name, id)
		}
	}
	if workID == artifactID || artifactID == version.VersionID || workID == version.VersionID {
		t.Fatal("two of the three ids are equal; the parameters cannot be told apart")
	}
	if version.Source != "edited" || version.Action != "saved" {
		t.Errorf("an ordinary save is (%q,%q), want (edited,saved)", version.Source, version.Action)
	}

	// {versionId} gets its own cases: it is a new class of id, and a real
	// version read under a different work must be refused exactly like an
	// unknown one.
	read := accountAPIRequest(t, http.MethodGet,
		"/api/content-works/"+workID+"/artifacts/"+artifactID+"/versions/"+version.VersionID, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(read.Body)
		t.Fatalf("GET version = %d, want 200: %s", read.StatusCode, body)
	}
	var got struct {
		VersionID string `json:"version_id"`
	}
	if err := json.NewDecoder(read.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.VersionID != version.VersionID {
		t.Fatalf("path named %q, response returned %q", version.VersionID, got.VersionID)
	}

	otherWorkID := createWorkThroughTheAPI(t, fx, cardID)
	fx.Cleanup(t, `DELETE FROM content_work WHERE work_id=$1`, otherWorkID)
	foreign := accountAPIRequest(t, http.MethodGet,
		"/api/content-works/"+otherWorkID+"/artifacts/"+artifactID+"/versions/"+version.VersionID, "")
	defer foreign.Body.Close()
	if foreign.StatusCode != http.StatusNotFound {
		t.Fatalf("a version read under another work = %d, want 404", foreign.StatusCode)
	}
	missing := accountAPIRequest(t, http.MethodGet,
		"/api/content-works/"+workID+"/artifacts/"+artifactID+"/versions/no-such-version", "")
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("an unknown version = %d, want 404", missing.StatusCode)
	}
	if !refusalsMatchApartFromTrace(t, foreign, missing) {
		t.Error("the two refusals differ; the response can be used to probe")
	}
}

func createWorkThroughTheAPI(t *testing.T, fx *testutil.Fixture, topicCardID string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost, "/api/content-works",
		fmt.Sprintf(`{"topic_card_id":%q,"snapshot_id":"","title":"手写稿"}`, topicCardID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create work = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		WorkID     string `json:"work_id"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	// SOP 6.2: a work that never went through "start" is ordinary.
	if created.SnapshotID != "" {
		t.Fatalf("snapshot_id = %q, want empty", created.SnapshotID)
	}
	fx.Cleanup(t, `DELETE FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2`, testWorkspaceID, created.WorkID)
	return created.WorkID
}

func createArtifactThroughTheAPI(t *testing.T, workID string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts", `{"kind":"body","title":"正文","position":1}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create artifact = %d, want 201: %s", response.StatusCode, body)
	}
	var created struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created.ArtifactID
}
