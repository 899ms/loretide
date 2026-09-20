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
	sourceinbox "github.com/multica-ai/multica/server/internal/content/source-inbox"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The material inbox (specs/028).

var contentSourceRoutes = []struct {
	method  string
	pattern string
	path    string
}{
	{http.MethodGet, "/api/content-sources/", "/api/content-sources"},
	{http.MethodPost, "/api/content-sources/", "/api/content-sources"},
	{http.MethodGet, "/api/content-sources/duplicates", "/api/content-sources/duplicates"},
	{http.MethodPost, "/api/content-sources/bulk", "/api/content-sources/bulk"},
	{http.MethodGet, "/api/content-sources/{sourceId}/", "/api/content-sources/source-1"},
	{http.MethodPatch, "/api/content-sources/{sourceId}/", "/api/content-sources/source-1"},
	{http.MethodGet, "/api/content-sources/{sourceId}/revisions", "/api/content-sources/source-1/revisions"},
}

func TestContentSourceEndpointsAreMounted(t *testing.T) {
	router := NewRouter(nil, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil)
	mounted := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSourceRoutes {
		if !mounted[route.method+" "+route.pattern] {
			t.Errorf("%s %s is not mounted", route.method, route.pattern)
		}
	}
	// Archiving is a status and R-011 forbids removing a duplicate's own
	// context, so there must be no route that could delete one. Keeping that
	// is not a promise anybody has to remember if the route does not exist.
	for route := range mounted {
		if strings.HasPrefix(route, http.MethodDelete+" /api/content-sources") {
			t.Errorf("a DELETE route exists on the inbox: %s", route)
		}
	}
}

// Workflow step 12 for the path parameter this feature adds.
//
// Route existence is not enough: a handler that read the workspace id where it
// meant to read the path parameter would still be mounted, still answer, and
// still look right in a test whose two values happen to be equal. The source id
// here is server-chosen and does not equal the workspace id.
func TestSourceInboxPathIDSurvivesTheRealMiddleware(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_source_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source_snapshot WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	sourceID := createSourceThroughTheAPI(t, `{"kind":"pasted_text","content":"a note worth keeping","title":"note"}`)
	if sourceID == testWorkspaceID {
		t.Fatal("the source id equals the workspace id; this test could not tell the two apart")
	}

	// {sourceId}: the response has to be about the item the path named.
	read := accountAPIRequest(t, http.MethodGet, "/api/content-sources/"+sourceID, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(read.Body)
		t.Fatalf("GET source = %d, want 200: %s", read.StatusCode, body)
	}
	var got struct {
		Source struct {
			SourceID    string `json:"source_id"`
			WorkspaceID string `json:"workspace_id"`
			Status      string `json:"status"`
		} `json:"source"`
		Snapshot *struct {
			ContentHash string `json:"content_hash"`
		} `json:"snapshot"`
	}
	if err := json.NewDecoder(read.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Source.SourceID != sourceID {
		t.Fatalf("path named %q, response returned %q", sourceID, got.Source.SourceID)
	}
	if got.Source.WorkspaceID != testWorkspaceID {
		t.Errorf("workspace = %q, want %q", got.Source.WorkspaceID, testWorkspaceID)
	}
	if got.Snapshot == nil || got.Snapshot.ContentHash == "" {
		t.Error("a pasted text came back without its snapshot")
	}

	// {sourceId}/revisions: same parameter, a second handler.
	organize := accountAPIRequest(t, http.MethodPatch, "/api/content-sources/"+sourceID,
		`{"title":"renamed"}`)
	organize.Body.Close()
	if organize.StatusCode != http.StatusOK {
		t.Fatalf("PATCH source = %d, want 200", organize.StatusCode)
	}
	revisions := accountAPIRequest(t, http.MethodGet, "/api/content-sources/"+sourceID+"/revisions", "")
	defer revisions.Body.Close()
	var history struct {
		Revisions []struct {
			SourceID      string   `json:"source_id"`
			ChangedFields []string `json:"changed_fields"`
		} `json:"revisions"`
	}
	if err := json.NewDecoder(revisions.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history.Revisions) != 1 || history.Revisions[0].SourceID != sourceID {
		t.Fatalf("revisions = %+v, want one for %s", history.Revisions, sourceID)
	}
	if len(history.Revisions[0].ChangedFields) != 1 || history.Revisions[0].ChangedFields[0] != "title" {
		t.Errorf("changed fields = %v, want [title]", history.Revisions[0].ChangedFields)
	}

	// The static segments are not read as ids. Without the ordering in the
	// router, "duplicates" would arrive here as a source id and answer 404.
	duplicates := accountAPIRequest(t, http.MethodGet,
		"/api/content-sources/duplicates?content_hash="+contentHashOf("a note worth keeping"), "")
	defer duplicates.Body.Close()
	if duplicates.StatusCode != http.StatusOK {
		t.Fatalf("GET duplicates = %d, want 200 (is it being read as a source id?)", duplicates.StatusCode)
	}
	var hint struct {
		SourceIDs []string `json:"source_ids"`
	}
	if err := json.NewDecoder(duplicates.Body).Decode(&hint); err != nil {
		t.Fatal(err)
	}
	if len(hint.SourceIDs) != 1 || hint.SourceIDs[0] != sourceID {
		t.Errorf("duplicates = %v, want [%s]", hint.SourceIDs, sourceID)
	}
}

// R-011, through the API: the second collection is created, the hint names the
// first, and BOTH are still there afterwards.
func TestCollectingTheSameTextTwiceKeepsBothThroughTheAPI(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_source_revision WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source_snapshot WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	first := createSourceThroughTheAPI(t, `{"kind":"pasted_text","content":"duplicated body","annotation":"first time"}`)

	second := accountAPIRequest(t, http.MethodPost, "/api/content-sources",
		`{"kind":"pasted_text","content":"duplicated body","annotation":"second time"}`)
	defer second.Body.Close()
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("second collection = %d, want 201; a duplicate must not be refused", second.StatusCode)
	}
	var created struct {
		Source struct {
			SourceID string `json:"source_id"`
		} `json:"source"`
		Duplicates []string `json:"duplicates"`
	}
	if err := json.NewDecoder(second.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.Duplicates) != 1 || created.Duplicates[0] != first {
		t.Errorf("duplicates = %v, want [%s]", created.Duplicates, first)
	}

	list := accountAPIRequest(t, http.MethodGet, "/api/content-sources", "")
	defer list.Body.Close()
	var listed struct {
		Sources []struct {
			SourceID   string `json:"source_id"`
			Annotation string `json:"annotation"`
		} `json:"sources"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sources) != 2 {
		t.Fatalf("listed %d sources, want 2; neither collection may be removed", len(listed.Sources))
	}
	annotations := map[string]bool{}
	for _, source := range listed.Sources {
		annotations[source.Annotation] = true
	}
	if !annotations["first time"] || !annotations["second time"] {
		t.Errorf("an annotation was lost: %v", annotations)
	}
}

// FR-002 at the boundary: a url is stored with no snapshot and no hint.
func TestAURLIsStoredWithoutBeingFetched(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	fx.Cleanup(t, `DELETE FROM content_source_snapshot WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_source WHERE workspace_id=$1`, testWorkspaceID)
	fx.Cleanup(t, `DELETE FROM content_operation_audit WHERE workspace_id=$1`, testWorkspaceID)

	// example.invalid is reserved by RFC 2606 and resolves nowhere: if anything
	// tried to fetch it, this would hang or error rather than pass.
	sourceID := createSourceThroughTheAPI(t,
		`{"kind":"url","url":"https://example.invalid/post/1","annotation":"typed by hand"}`)

	var snapshots int
	if err := testPool.QueryRow(t.Context(),
		`SELECT count(*) FROM content_source_snapshot WHERE workspace_id=$1 AND source_id=$2`,
		testWorkspaceID, sourceID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 {
		t.Errorf("a url source produced %d snapshots, want 0", snapshots)
	}
}

// A value outside a controlled set is a 400 that names the field, not a 503 and
// not a silent accept.
func TestABadKindIsRefusedWithTheFieldNamed(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	response := accountAPIRequest(t, http.MethodPost, "/api/content-sources",
		`{"kind":"file","content":"x"}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad kind = %d, want 400", response.StatusCode)
	}
	var body struct {
		Field string `json:"field"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Field != "kind" {
		t.Errorf("field = %q, want kind", body.Field)
	}
}

// An outsider gets the same 404 everywhere, so the response cannot be used to
// learn that something exists here.
func TestSourceInboxRefusesAnOutsiderEverywhere(t *testing.T) {
	if testPool == nil || testServer == nil {
		t.Skip("database not available")
	}
	fx := testutil.New(testPool, testWorkspaceID, testUserID)
	email := fmt.Sprintf("content-source-outsider-%d@multica.test", time.Now().UnixNano())
	outsider := fx.User(t, "Content Source Outsider", email)
	token, err := generateTestJWT(outsider, email, "Content Source Outsider")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range contentSourceRoutes {
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

// contentHashOf mirrors the module's hash so the duplicate query can be built
// here. Imported rather than reimplemented: a second sha256 would be a second
// place for the definition to drift.
func contentHashOf(content string) string { return sourceinbox.ContentHash(content) }

func createSourceThroughTheAPI(t *testing.T, body string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost, "/api/content-sources", body)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("create source = %d, want 201: %s", response.StatusCode, raw)
	}
	var created struct {
		Source struct {
			SourceID string `json:"source_id"`
		} `json:"source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Source.SourceID == "" {
		t.Fatal("create source returned no id")
	}
	return created.Source.SourceID
}
