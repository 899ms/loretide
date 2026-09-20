package handler

import (
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

// TestCommentMentionsAnyone covers the pure helper that drives the
// "skip leader on @<anyone>" behavior. Routing-style mentions
// (agent/member/squad/all) count; issue cross-references do not. Kept as a
// unit test so it runs without a database connection.
func TestCommentMentionsAnyone(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "empty", content: "", want: false},
		{name: "plain text", content: "please take a look", want: false},
		{name: "literal at sign only", content: "ping @alice", want: false},
		{name: "agent mention", content: "[@A](mention://agent/11111111-1111-1111-1111-111111111111) handle this", want: true},
		{name: "member mention", content: "[@Bob](mention://member/22222222-2222-2222-2222-222222222222)", want: true},
		{name: "squad mention", content: "[@Squad](mention://squad/44444444-4444-4444-4444-444444444444)", want: true},
		{name: "mention all", content: "[@all](mention://all/all)", want: true},
		{name: "issue mention only", content: "see [MUL-1](mention://issue/33333333-3333-3333-3333-333333333333)", want: false},
		{name: "issue + plain text", content: "see [MUL-1](mention://issue/33333333-3333-3333-3333-333333333333) for context", want: false},
		{name: "agent plus member", content: "[@A](mention://agent/11111111-1111-1111-1111-111111111111) cc [@B](mention://member/22222222-2222-2222-2222-222222222222)", want: true},
		{name: "issue plus member", content: "blocks [MUL-1](mention://issue/33333333-3333-3333-3333-333333333333) — [@Bob](mention://member/22222222-2222-2222-2222-222222222222)", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commentMentionsAnyone(tc.content); got != tc.want {
				t.Fatalf("commentMentionsAnyone(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}

func triggersContainIssueAssigneeSquadLeader(triggers []commentAgentTrigger) bool {
	for _, trigger := range triggers {
		if trigger.Source == commentTriggerSourceIssueAssignee && trigger.Squad != nil {
			return true
		}
	}
	return false
}

// squadCommentTriggerFixture wires a squad assigned to a fresh issue and
// returns the loaded db.Issue plus the leader agent UUID for use in
// cascade integration tests.
type squadCommentTriggerFixture struct {
	Issue    db.Issue
	SquadID  string
	LeaderID string
	OtherID  string // second agent in workspace (with runtime), used as a non-leader @mention target
}
