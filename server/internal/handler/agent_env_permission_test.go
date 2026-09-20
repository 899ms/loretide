package handler

import (
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

// TestCanManageAgentEnv_Pure exercises the predicate behind the env
// endpoints without a database. Ownership must count for a plain member
// (MUL-5438), workspace owner/admin must keep working, and a NULL
// agent.owner_id must never match anyone: the column is nullable and
// uuidToString renders a NULL UUID as "".
func TestCanManageAgentEnv_Pure(t *testing.T) {
	ownerUserID := "11111111-1111-1111-1111-111111111111"
	otherUserID := "22222222-2222-2222-2222-222222222222"

	owned := db.Agent{OwnerID: util.MustParseUUID(ownerUserID)}
	orphaned := db.Agent{} // owner_id IS NULL

	cases := []struct {
		name   string
		agent  db.Agent
		userID string
		role   string
		want   bool
	}{
		{"workspace owner, not agent owner", owned, otherUserID, "owner", true},
		{"workspace admin, not agent owner", owned, otherUserID, "admin", true},
		{"agent owner with member role", owned, ownerUserID, "member", true},
		{"agent owner with admin role", owned, ownerUserID, "admin", true},
		{"plain member, not agent owner", owned, otherUserID, "member", false},
		{"plain member with no role string", owned, otherUserID, "", false},
		{"null owner_id, plain member", orphaned, otherUserID, "member", false},
		{"null owner_id, workspace admin", orphaned, otherUserID, "admin", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member := db.Member{UserID: util.MustParseUUID(tc.userID), Role: tc.role}
			if got := canManageAgentEnv(tc.agent, member); got != tc.want {
				t.Fatalf("canManageAgentEnv(owner=%q, user=%s, role=%s) = %v; want %v",
					uuidToString(tc.agent.OwnerID), tc.userID, tc.role, got, tc.want)
			}
		})
	}
}
