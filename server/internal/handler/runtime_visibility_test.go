package handler

import (
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

// TestCanUseRuntimeForAgent_Pure exercises the pure predicate behind the
// CreateAgent / UpdateAgent runtime gate. The truth table is the single
// product contract shared with the clients (MUL-6126): a private runtime is
// usable only by its owner — workspace owners and admins included, since a
// private runtime is another member's own machine — while a public runtime is
// usable by anyone in the workspace. An ownerless runtime is usable by nobody,
// public included: it cannot run tasks either (task claim requires a runtime
// owner to mint the agent token, MUL-3292), so the gate refuses it up front
// instead of letting the binding succeed and every task on it fail.
func TestCanUseRuntimeForAgent_Pure(t *testing.T) {
	ownerUserID := "11111111-1111-1111-1111-111111111111"
	otherUserID := "22222222-2222-2222-2222-222222222222"

	privateRT := db.AgentRuntime{
		OwnerID:    util.MustParseUUID(ownerUserID),
		Visibility: "private",
	}
	publicRT := db.AgentRuntime{
		OwnerID:    util.MustParseUUID(ownerUserID),
		Visibility: "public",
	}
	ownerlessPrivateRT := db.AgentRuntime{Visibility: "private"}
	ownerlessPublicRT := db.AgentRuntime{Visibility: "public"}

	cases := []struct {
		name   string
		userID string
		role   string
		rt     db.AgentRuntime
		want   bool
	}{
		// no workspace owner / admin override: role never widens access
		{"workspace owner on private runtime owned by another", otherUserID, "owner", privateRT, false},
		{"workspace admin on private runtime owned by another", otherUserID, "admin", privateRT, false},
		{"workspace admin on someone else's public runtime", otherUserID, "admin", publicRT, true},
		// runtime owner
		{"runtime owner on own private runtime", ownerUserID, "member", privateRT, true},
		{"runtime owner on own public runtime", ownerUserID, "member", publicRT, true},
		{"runtime owner who is also a workspace admin", ownerUserID, "admin", privateRT, true},
		// public runtime allows anyone in workspace
		{"plain member on someone else's public runtime", otherUserID, "member", publicRT, true},
		// private runtime owned by someone else is denied to everyone
		{"plain member on someone else's private runtime", otherUserID, "member", privateRT, false},
		{"plain member with empty role on private runtime", otherUserID, "", privateRT, false},
		// ownerless runtimes are refused whatever their visibility — nobody
		// can be issued a task token for them (MUL-3292)
		{"workspace owner on an ownerless private runtime", otherUserID, "owner", ownerlessPrivateRT, false},
		{"plain member on an ownerless public runtime", otherUserID, "member", ownerlessPublicRT, false},
		{"workspace admin on an ownerless public runtime", otherUserID, "admin", ownerlessPublicRT, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member := db.Member{
				UserID: util.MustParseUUID(tc.userID),
				Role:   tc.role,
			}
			got := canUseRuntimeForAgent(member, tc.rt)
			if got != tc.want {
				t.Fatalf("canUseRuntimeForAgent(role=%s, visibility=%s, owner=%s, caller=%s) = %v; want %v",
					tc.role, tc.rt.Visibility, ownerUserID, tc.userID, got, tc.want)
			}
		})
	}
}

// TestCanSetRuntimeVisibility_Pure pins the narrower gate: only the runtime
// owner may flip private↔public. Workspace owners/admins keep canEditRuntime
// for rename and delete, but sharing a machine is the owner's call — otherwise
// the MUL-6126 bind rule would be one PATCH away from being bypassed.
func TestCanSetRuntimeVisibility_Pure(t *testing.T) {
	ownerUserID := "11111111-1111-1111-1111-111111111111"
	otherUserID := "22222222-2222-2222-2222-222222222222"

	ownedRT := db.AgentRuntime{
		OwnerID:    util.MustParseUUID(ownerUserID),
		Visibility: "private",
	}
	ownerlessRT := db.AgentRuntime{Visibility: "private"}

	cases := []struct {
		name   string
		userID string
		role   string
		rt     db.AgentRuntime
		want   bool
	}{
		{"runtime owner", ownerUserID, "member", ownedRT, true},
		{"workspace owner who does not own the runtime", otherUserID, "owner", ownedRT, false},
		{"workspace admin who does not own the runtime", otherUserID, "admin", ownedRT, false},
		{"plain member", otherUserID, "member", ownedRT, false},
		{"workspace owner on an ownerless runtime", otherUserID, "owner", ownerlessRT, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			member := db.Member{
				UserID: util.MustParseUUID(tc.userID),
				Role:   tc.role,
			}
			if got := canSetRuntimeVisibility(member, tc.rt); got != tc.want {
				t.Fatalf("canSetRuntimeVisibility(role=%s, caller=%s) = %v; want %v",
					tc.role, tc.userID, got, tc.want)
			}
		})
	}
}
