package handler

import (
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

// recordingWorkspaceRefreshNotifier captures the daemon wakeups a workspace
// write fans out to its members.
type recordingWorkspaceRefreshNotifier struct {
	userIDs []string
}

func (n *recordingWorkspaceRefreshNotifier) NotifyWorkspacesChanged(userID string) {
	n.userIDs = append(n.userIDs, userID)
}

// revocationFixture is a minimal (workspace, member-to-revoke, runtime,
// agent, queued-task, daemon-token) bundle used to drive the revocation
// tests. The "requester" is always testUserID (owner of the workspace) so
// `newRequest` passes the existing fixtures' auth context unchanged.
type revocationFixture struct {
	WorkspaceID  string
	TargetUserID string
	MemberID     string
	RuntimeID    string
	AgentID      string
	TaskID       string
	DaemonID     string
	TokenHash    string
}

// TestDefaultIssuePrefixFromSlug pins the derivation new workspaces get
// (MUL-6050): alphanumerics of the slug, first 4, uppercased. The Chinese
// cases are the point of the change — under the old name-based derivation
// every one of them collapsed to "WS".
//
// Keep this table in sync with the client-side preview
// (packages/views/onboarding/steps/step-workspace.tsx). If the two ever
// disagree, the create screen lies about the identifier the user will get.
func TestDefaultIssuePrefixFromSlug(t *testing.T) {
	cases := []struct {
		slug string
		want string
	}{
		{"acme", "ACME"},
		{"front-end", "FRON"},
		{"growth", "GROW"},
		{"team-2", "TEAM"},
		{"a1b2c3", "A1B2"},
		{"ab", "AB"},
		{"x", "X"},
		// Slugs the create handler would have rejected anyway; the function
		// stays total so no caller can persist an empty prefix.
		{"", "WS"},
		{"--", "WS"},
	}

	for _, tc := range cases {
		if got := defaultIssuePrefixFromSlug(tc.slug); got != tc.want {
			t.Errorf("defaultIssuePrefixFromSlug(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}
}

// TestLegacyIssuePrefixFromName_Frozen guards the read-time fallback for
// workspaces whose stored prefix is empty. Issue identifiers are computed
// from the current prefix on every read, so changing what this returns would
// silently rewrite the identifier of every historical issue in those
// workspaces. The product decision on MUL-6050 was explicit: no backfill,
// existing workspaces are left exactly as they are — which means this
// function must keep returning what it always did, including "WS" for
// non-ASCII names.
func TestLegacyIssuePrefixFromName_Frozen(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Jiayuan's Workspace", "JIA"},
		{"My Team", "MYT"},
		{"AB", "AB"},
		{"Team 2", "TEA"},
		{"前端团队", "WS"},
		{"", "WS"},
	}

	for _, tc := range cases {
		if got := legacyIssuePrefixFromName(tc.name); got != tc.want {
			t.Errorf("legacyIssuePrefixFromName(%q) = %q, want %q — this function is frozen; changing it rewrites existing issue identifiers", tc.name, got, tc.want)
		}
	}
}

// TestIssuePrefixForWorkspace_LegacyFallbackFrozen guards the resolution rule
// itself, at the seam every read-time caller goes through (getIssuePrefix and
// the GitHub close-intent scan, which holds the row already).
//
// A stored prefix always wins; only an empty one falls back, and that fallback
// must stay on the old name-based derivation. Pointing it at
// defaultIssuePrefixFromSlug would rewrite the identifier of every issue in
// those legacy workspaces on the next read — the exact outcome the "no
// backfill, leave existing workspaces alone" decision on MUL-6050 rules out.
func TestIssuePrefixForWorkspace_LegacyFallbackFrozen(t *testing.T) {
	cases := []struct {
		label string
		ws    db.Workspace
		want  string
	}{
		{"stored prefix wins", db.Workspace{Name: "前端团队", Slug: "frontend", IssuePrefix: "FE"}, "FE"},
		{"empty prefix, CJK name", db.Workspace{Name: "前端团队", Slug: "frontend"}, "WS"},
		{"empty prefix, ASCII name", db.Workspace{Name: "My Team", Slug: "my-team"}, "MYT"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if got := issuePrefixForWorkspace(tc.ws); got != tc.want {
				t.Fatalf("issuePrefixForWorkspace(%+v) = %q, want %q — legacy workspaces must keep the identifiers they already have", tc.ws, got, tc.want)
			}
		})
	}
}

func TestNormalizeIssuePrefix(t *testing.T) {
	cases := []struct {
		raw   string
		want  string
		valid bool
	}{
		{"acme", "ACME", true},
		{"  acme  ", "ACME", true},
		{"AB12", "AB12", true},
		{"ABCDEFGHIJ", "ABCDEFGHIJ", true},
		// Absent / blank means "use the default", not "invalid".
		{"", "", true},
		{"   ", "", true},
		// Rejections the API accepted before MUL-6050.
		{"ABCDEFGHIJK", "", false},
		{"前端", "", false},
		{"AB-CD", "", false},
		{"AB CD", "", false},
		{"AB_CD", "", false},
	}

	for _, tc := range cases {
		got, ok := normalizeIssuePrefix(tc.raw)
		if ok != tc.valid {
			t.Errorf("normalizeIssuePrefix(%q) valid = %v, want %v", tc.raw, ok, tc.valid)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeIssuePrefix(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
