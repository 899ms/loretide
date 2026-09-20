//go:build dbtest

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHydrateTaskAttributionsFillsUserRef verifies the batch name hydration resolves
// initiator/originator refs from the GLOBAL user table (departed-safe) and leaves an
// unknown id un-filled without erroring (MUL-4302 §9).
func TestHydrateTaskAttributionsFillsUserRef(t *testing.T) {
	known := &AttributionUser{ID: testUserID}
	unknown := &AttributionUser{ID: "44444444-4444-4444-4444-444444444444"}
	attrs := []*TaskAttribution{
		{Initiator: known, Originator: known}, // same id in both refs
		{Initiator: unknown},
		nil, // must be skipped without panic
	}

	testHandler.hydrateTaskAttributions(context.Background(), attrs)

	if known.Name != handlerTestName || known.Email != handlerTestEmail {
		t.Errorf("known ref = %q/%q, want %q/%q", known.Name, known.Email, handlerTestName, handlerTestEmail)
	}
	if unknown.Name != "" {
		t.Errorf("unknown user id must stay un-filled, got name %q", unknown.Name)
	}
}

// TestListTasksByIssueHydratesAttribution is the MUL-4302 §9 regression guard for
// the execution-log surface: the issue task list (api.listTasksByIssue) drives the
// "on behalf of <member>" badge, so it must resolve the initiator's display NAME,
// not just its id. Before the fix ListTasksByIssue returned taskToResponse without
// hydration and the badge fell back to "someone" on issue detail.
func TestListTasksByIssueHydratesAttribution(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	agentID := createHandlerTestAgent(t, "AttributionListAgent", []byte("[]"))

	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_id, creator_type, number, position)
		VALUES ($1, 'attribution-list-issue', 'todo', 'medium', $2, 'member', 92777, 0)
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })

	// direct_human: accountable == originator == the fixture user, whose name
	// lives in the global user table — so hydration has a name to fill.
	var taskID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, issue_id, originator_source, originator_user_id, accountable_user_id)
		VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', 0, $2, 'direct_human', $3, $3)
		RETURNING id
	`, agentID, issueID, testUserID).Scan(&taskID); err != nil {
		t.Fatalf("create attributed task: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })

	req := newRequest("GET", "/api/issues/"+issueID+"/tasks", nil)
	req = withURLParam(req, "id", issueID)
	w := httptest.NewRecorder()
	testHandler.ListTasksByIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []AgentTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode task list: %v", err)
	}

	var got *TaskAttribution
	for i := range resp {
		if resp[i].ID == taskID {
			got = resp[i].Attribution
			break
		}
	}
	if got == nil {
		t.Fatalf("task %s missing from response or has no attribution: %s", taskID, w.Body.String())
	}
	if got.Source != "direct_human" || !got.Precise {
		t.Fatalf("source/precise = %q/%v, want direct_human/true", got.Source, got.Precise)
	}
	if got.Initiator == nil {
		t.Fatal("initiator missing")
	}
	if got.Initiator.ID != testUserID {
		t.Errorf("initiator id = %q, want %q", got.Initiator.ID, testUserID)
	}
	if got.Initiator.Name != handlerTestName {
		t.Errorf("initiator name = %q, want %q (list endpoint must hydrate names)", got.Initiator.Name, handlerTestName)
	}
}
