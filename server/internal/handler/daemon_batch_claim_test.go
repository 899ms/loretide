package handler

import (
	"net/http"
	"testing"
	"time"
)

// batchClaimResponse mirrors the {"tasks":[...]} envelope ClaimTasksByRuntime
// returns, with the few fields these tests assert on.
type batchClaimResponse struct {
	Tasks []struct {
		ID        string `json:"id"`
		RuntimeID string `json:"runtime_id"`
		AuthToken string `json:"auth_token"`
	} `json:"tasks"`
	ClaimPollHintSupported      bool  `json:"claim_poll_hint_supported"`
	NextDeferredTaskAfterMillis int64 `json:"next_deferred_task_after_ms"`
}

func batchClaimRequest(workspaceID string, runtimeIDs []string, maxTasks int, capabilities string) *http.Request {
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/tasks/claim",
		map[string]any{"daemon_id": batchClaimTestDaemonID, "runtime_ids": runtimeIDs, "max_tasks": maxTasks}, workspaceID, batchClaimTestDaemonID)
	if capabilities != "" {
		req.Header.Set("X-Client-Capabilities", capabilities)
	}
	return req
}

// batchClaimTestDaemonID is the daemon id used by both the mdt_ token context
// and the request body in batch-claim handler tests, so the daemon_id
// consistency check passes on the happy path.
const batchClaimTestDaemonID = "batch-claim-review"

func TestClaimPollHintDelayHasOneSecondFloor(t *testing.T) {
	now := time.Unix(1_000, 0)
	for _, tt := range []struct {
		name   string
		fireAt time.Time
		want   time.Duration
	}{
		{name: "overdue", fireAt: now.Add(-time.Minute), want: time.Second},
		{name: "sub-second", fireAt: now.Add(100 * time.Millisecond), want: time.Second},
		{name: "future", fireAt: now.Add(5 * time.Second), want: 5 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimPollHintDelay(now, tt.fireAt); got != tt.want {
				t.Fatalf("claimPollHintDelay() = %s, want %s", got, tt.want)
			}
		})
	}
}
