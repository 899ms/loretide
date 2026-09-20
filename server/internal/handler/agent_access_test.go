package handler

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
	"time"
)

func TestDeriveAgentRuntimeAvailability(t *testing.T) {
	now := time.Date(2026, time.April, 27, 12, 0, 0, 0, time.UTC)
	timestamp := func(at time.Time) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: at, Valid: true}
	}

	for _, tc := range []struct {
		name   string
		status string
		seen   pgtype.Timestamptz
		want   string
	}{
		{name: "online", status: "online", want: "online"},
		{name: "recent loss", status: "offline", seen: timestamp(now.Add(-time.Minute)), want: "unstable"},
		{name: "offline", status: "offline", seen: timestamp(now.Add(-10 * time.Minute)), want: "offline"},
		{name: "missing heartbeat", status: "offline", want: "offline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveAgentRuntimeAvailability(db.AgentRuntime{
				Status:     tc.status,
				LastSeenAt: tc.seen,
			}, now)
			if got != tc.want {
				t.Fatalf("deriveAgentRuntimeAvailability = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadAgentRuntimeAvailability_PrivilegedViewerSkipsRuntimeQuery(t *testing.T) {
	h := &Handler{}
	agents := []db.Agent{{
		RuntimeID: util.MustParseUUID("11111111-1111-1111-1111-111111111111"),
	}}

	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			got, err := h.loadAgentRuntimeAvailability(
				context.Background(),
				agents,
				"22222222-2222-2222-2222-222222222222",
				"33333333-3333-3333-3333-333333333333",
				role,
				time.Now(),
			)
			if err != nil {
				t.Fatalf("loadAgentRuntimeAvailability: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("availability = %v, want empty for %s", got, role)
			}
		})
	}
}

// TestMemberAllowedToViewAgent_Pure exercises the pure predicate that drives
// the private-agent VIEW gate. For a private agent it must allow:
//   - workspace owner / admin (regardless of agent ownership)
//   - the agent owner (regardless of role)
//
// And deny everyone else. This test runs without a database.
func TestMemberAllowedToViewAgent_Pure(t *testing.T) {
	ownerUserID := "11111111-1111-1111-1111-111111111111"
	otherUserID := "22222222-2222-2222-2222-222222222222"

	agent := db.Agent{
		OwnerID:        util.MustParseUUID(ownerUserID),
		PermissionMode: "private",
	}

	cases := []struct {
		name   string
		userID string
		role   string
		want   bool
	}{
		{"workspace owner, not agent owner", otherUserID, "owner", true},
		{"workspace admin, not agent owner", otherUserID, "admin", true},
		{"agent owner with member role", ownerUserID, "member", true},
		{"agent owner with admin role", ownerUserID, "admin", true},
		{"plain member, not agent owner", otherUserID, "member", false},
		{"plain member with no role string", otherUserID, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := memberAllowedToViewAgent(agent, nil, tc.userID, tc.role)
			if got != tc.want {
				t.Fatalf("memberAllowedToViewAgent(userID=%s, role=%s) = %v; want %v",
					tc.userID, tc.role, got, tc.want)
			}
		})
	}
}

func listContainsAgent(t *testing.T, body []byte, agentID string) bool {
	t.Helper()
	var resp []AgentResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode ListAgents response: %v", err)
	}
	for _, a := range resp {
		if a.ID == agentID {
			return true
		}
	}
	return false
}
