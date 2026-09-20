package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// slowProbeLocalSkillListStore wraps a LocalSkillListStore but blocks inside
// HasPending until the provided context is cancelled. PopPending delegates
// to the underlying store. Used to verify that a stalled probe cannot wedge
// the heartbeat — the bound context must cut it short — while the ack-safe
// PopPending path is never reached because HasPending returns an error, not
// true.
type slowProbeLocalSkillListStore struct{ LocalSkillListStore }

func (s slowProbeLocalSkillListStore) HasPending(ctx context.Context, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

type slowProbeLocalSkillImportStore struct{ LocalSkillImportStore }

func (s slowProbeLocalSkillImportStore) HasPending(ctx context.Context, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

// popRecordingLocalSkillListStore counts PopPending calls so a test can assert
// that the handler never reaches the ack-unsafe side-effecting claim path
// when HasPending reports an empty queue.
type popRecordingLocalSkillListStore struct {
	LocalSkillListStore
	popCalls int
}

func (s *popRecordingLocalSkillListStore) PopPending(ctx context.Context, runtimeID string) (*RuntimeLocalSkillListRequest, error) {
	s.popCalls++
	return s.LocalSkillListStore.PopPending(ctx, runtimeID)
}

type popRecordingLocalSkillImportStore struct {
	LocalSkillImportStore
	popCalls int
}

func (s *popRecordingLocalSkillImportStore) PopPending(ctx context.Context, runtimeID string) (*RuntimeLocalSkillImportRequest, error) {
	s.popCalls++
	return s.LocalSkillImportStore.PopPending(ctx, runtimeID)
}

// newDaemonTokenRequest creates an HTTP request with daemon token context set
// (simulating DaemonAuth middleware for mdt_ tokens).
func newDaemonTokenRequest(method, path string, body any, workspaceID, daemonID string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	// No X-User-ID — daemon tokens don't set it.
	ctx := middleware.WithDaemonContext(req.Context(), workspaceID, daemonID)
	return req.WithContext(ctx)
}

// TestGetTaskStatus_TransientDBError_Returns500 verifies that a transient DB
// error from GetAgentTask is reported as 500 rather than 404. The daemon
// uses 404+"task not found" as a hard cancel signal; a transient lookup
// failure must therefore not be smuggled into that body, otherwise a single
// DB hiccup would kill an in-flight agent.
func TestGetTaskStatus_TransientDBError_Returns500(t *testing.T) {
	h := &Handler{}
	h.Queries = db.New(&mockDB{getUserErr: errors.New("connection reset by peer")})

	req := newDaemonTokenRequest("GET", "/api/daemon/tasks/00000000-0000-0000-0000-000000000001/status", nil,
		"00000000-0000-0000-0000-000000000000", "test-daemon")
	req = withURLParam(req, "taskId", "00000000-0000-0000-0000-000000000001")
	w := testutil.Call(t, h.GetTaskStatus, req).Want(http.StatusInternalServerError)
	if strings.Contains(w.Body.String(), "task not found") {
		t.Fatalf("transient DB error must not surface as %q (daemon treats that as a deletion): %s", "task not found", w.Body.String())
	}
}

// TestGetTaskStatus_ErrNoRows_Returns404 verifies that an actually-missing
// task row still returns the 404+"task not found" body the daemon relies on
// to interrupt the running agent.
func TestGetTaskStatus_ErrNoRows_Returns404(t *testing.T) {
	h := &Handler{}
	h.Queries = db.New(&mockDB{getUserErr: pgx.ErrNoRows})

	req := newDaemonTokenRequest("GET", "/api/daemon/tasks/00000000-0000-0000-0000-000000000001/status", nil,
		"00000000-0000-0000-0000-000000000000", "test-daemon")
	req = withURLParam(req, "taskId", "00000000-0000-0000-0000-000000000001")
	w := testutil.Call(t, h.GetTaskStatus, req).Want(http.StatusNotFound)
	if !strings.Contains(w.Body.String(), "task not found") {
		t.Fatalf("ErrNoRows: expected body to contain %q, got %s", "task not found", w.Body.String())
	}
}

// withURLParams merges the given chi URL parameters into the request context.
// Unlike calling withURLParam twice (which replaces the whole chi.RouteContext
// and loses earlier params), this preserves previously-added params.
func withURLParams(req *http.Request, kv ...string) *http.Request {
	rctx := chi.NewRouteContext()
	if existing, ok := req.Context().Value(chi.RouteCtxKey).(*chi.Context); ok && existing != nil {
		for i, key := range existing.URLParams.Keys {
			rctx.URLParams.Add(key, existing.URLParams.Values[i])
		}
	}
	for i := 0; i+1 < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestListTaskMessagesByUser_InvalidTaskIDReturnsBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/optimistic-optimistic-1778739487737/messages", nil)
	req = withURLParams(req, "taskId", "optimistic-optimistic-1778739487737")

	(&Handler{}).ListTaskMessagesByUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid task id, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "task_id") {
		t.Fatalf("expected task_id validation error, got %s", w.Body.String())
	}
}

func TestClaimResponseAgentIdentityMatches(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		resp AgentTaskResponse
		want bool
	}{
		{name: "matching", resp: AgentTaskResponse{AgentID: "agent-a", Agent: &TaskAgentData{ID: "agent-a"}}, want: true},
		{name: "mismatched", resp: AgentTaskResponse{AgentID: "agent-a", Agent: &TaskAgentData{ID: "agent-b"}}},
		{name: "missing top-level identity", resp: AgentTaskResponse{Agent: &TaskAgentData{ID: "agent-a"}}},
		{name: "missing response agent", resp: AgentTaskResponse{AgentID: "agent-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimResponseAgentIdentityMatches(tc.resp); got != tc.want {
				t.Fatalf("claimResponseAgentIdentityMatches() = %v, want %v", got, tc.want)
			}
		})
	}
}

type claimRuntimeGuardTask struct {
	PriorSessionID                string          `json:"prior_session_id"`
	PriorWorkDir                  string          `json:"prior_work_dir"`
	PriorSessionResumeUnavailable bool            `json:"prior_session_resume_unavailable"`
	ChatMessage                   string          `json:"chat_message"`
	ThreadName                    string          `json:"thread_name"`
	QuickCreateAttachmentIDs      []string        `json:"quick_create_attachment_ids"`
	QuickCreatePriority           string          `json:"quick_create_priority"`
	QuickCreateDueDate            string          `json:"quick_create_due_date"`
	ProjectID                     string          `json:"project_id"`
	ProjectDescription            string          `json:"project_description"`
	ParentIssueID                 string          `json:"parent_issue_id"`
	QuickCreateSourceContext      json.RawMessage `json:"quick_create_source_context"`
}

// ---------------------------------------------------------------------------
// Membership Cache Integration Tests
//
// These tests don't just exercise the cache primitive (that's covered in
// auth/membership_cache_test.go). They prove two things that the unit tests
// can't:
//
//  1. requireDaemonWorkspaceAccess actually short-circuits the DB on a cache
//     hit (the "ghost user" trick below).
//  2. Each handler that mutates membership actually calls
//     h.MembershipCache.Invalidate(...) — so a future refactor that drops
//     one of those calls will fail CI instead of silently leaking a stale
//     authorization grant for up to MembershipCacheTTL.
// ---------------------------------------------------------------------------

type claimCommentTaskResp struct {
	Task *struct {
		ID               string `json:"id"`
		PriorSessionID   string `json:"prior_session_id"`
		TriggerCommentID string `json:"trigger_comment_id"`
		NewCommentCount  int    `json:"new_comment_count"`
		NewCommentsSince string `json:"new_comments_since"`
		DeltaKnown       bool   `json:"new_comments_delta_known"`
	} `json:"task"`
}
