package handler

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http"
	"testing"
)

func withURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("failed to unmarshal got JSON %q: %v", string(got), err)
	}

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("failed to unmarshal want JSON %q: %v", want, err)
	}

	gotJSON, err := json.Marshal(gotValue)
	if err != nil {
		t.Fatalf("failed to marshal normalized got JSON: %v", err)
	}
	wantJSON, err := json.Marshal(wantValue)
	if err != nil {
		t.Fatalf("failed to marshal normalized want JSON: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("expected JSON %s, got %s", string(wantJSON), string(gotJSON))
	}
}

func TestBatchIssueGCCheckRejectsInvalidRequests(t *testing.T) {
	h := &Handler{}
	workspaceID := "00000000-0000-0000-0000-000000000001"
	tooMany := make([]string, maxIssueGCBatchSize+1)
	for i := range tooMany {
		tooMany[i] = "00000000-0000-0000-0000-000000000002"
	}

	tests := []struct {
		name        string
		workspaceID string
		body        any
	}{
		{name: "malformed workspace", workspaceID: "not-a-uuid", body: map[string]any{"issue_ids": []string{}}},
		{name: "malformed issue", workspaceID: workspaceID, body: map[string]any{"issue_ids": []string{"not-a-uuid"}}},
		{name: "too many issues", workspaceID: workspaceID, body: map[string]any{"issue_ids": tooMany}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newDaemonTokenRequest("POST", "/api/daemon/workspaces/"+tt.workspaceID+"/issues/gc-check", tt.body,
				tt.workspaceID, "test-daemon")
			req = withURLParam(req, "workspaceId", tt.workspaceID)
			testutil.Call(t, h.BatchIssueGCCheck, req).Want(http.StatusBadRequest)
		})
	}
}

func assertSkillIDsPresent(t *testing.T, skills []SkillSummaryResponse, wantIDs ...string) {
	t.Helper()
	got := make(map[string]bool, len(skills))
	for _, s := range skills {
		got[s.ID] = true
	}
	for _, want := range wantIDs {
		if !got[want] {
			t.Fatalf("response missing skill %s; got %+v", want, skills)
		}
	}
}

func strPtr(s string) *string {
	return &s
}
