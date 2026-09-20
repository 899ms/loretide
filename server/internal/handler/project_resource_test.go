package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
	"time"
)

func TestIsValidGitRepoURL(t *testing.T) {
	good := []string{
		"https://github.com/multica-ai/multica",
		"https://github.com/multica-ai/multica.git",
		"http://github.example.com/x/y",
		"ssh://git@github.com/multica-ai/multica.git",
		"ssh://git@github.com:22/multica-ai/multica.git",
		"git@github.com:multica-ai/multica.git",
		"git@gitlab.example.com:group/sub/repo.git",
	}
	bad := []string{
		"",
		"not-a-url",
		"github.com/multica-ai/multica", // no scheme, no scp-style colon
		"https://",                      // empty host
		"git@github.com",                // missing :path
		"git@:foo/bar",                  // missing host
		"git@github.com:",               // missing path
		"ftp://example.com/repo",        // unsupported scheme
		"file:///tmp/repo",              // unsupported scheme
		"some random text with spaces",
		"github.com:org/repo@branch", // '@' after ':' belongs to the path, not user
		"foo:bar@baz",                // '@' after ':' with no scheme
		":foo/bar",                   // leading ':' with no host
	}
	for _, s := range good {
		if !isValidGitRepoURL(s) {
			t.Errorf("isValidGitRepoURL(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if isValidGitRepoURL(s) {
			t.Errorf("isValidGitRepoURL(%q) = true, want false", s)
		}
	}
}

func TestIsAbsoluteLocalPath(t *testing.T) {
	good := []string{
		"/Users/foo/work",
		"/",
		"/a",
		`C:\Users\foo`,
		`C:/Users/foo`,
		`d:\code\repo`,
		`\\server\share\path`,
	}
	bad := []string{
		"",
		"work/my-game",
		"./relative",
		"../relative",
		"~/work",
		"C:relative",
		"C:",
		`\foo`,
		"file:///tmp",
	}
	for _, s := range good {
		if !isAbsoluteLocalPath(s) {
			t.Errorf("isAbsoluteLocalPath(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if isAbsoluteLocalPath(s) {
			t.Errorf("isAbsoluteLocalPath(%q) = true, want false", s)
		}
	}
}

// execution_mode selects between the historical exclusive in-place run and
// worktree mode. It is validated at the API boundary so a typo is caught at
// save time; a daemon new enough to know the field refuses unknown values
// rather than falling back to in_place, so the two checks together keep a
// mistyped mode from ever running.
func TestValidateLocalDirectoryRefExecutionMode(t *testing.T) {
	accepted := []struct {
		name string
		mode string
		want string
	}{
		{"absent means in_place", "", ""},
		{"explicit in_place", "in_place", "in_place"},
		{"worktree", "worktree", "worktree"},
		{"surrounding whitespace is trimmed", "  worktree  ", "worktree"},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			ref := map[string]any{"local_path": "/Users/foo/work", "daemon_id": "d1"}
			if tc.mode != "" {
				ref["execution_mode"] = tc.mode
			}
			raw, err := json.Marshal(ref)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			out, err := validateLocalDirectoryRef(raw)
			if err != nil {
				t.Fatalf("validateLocalDirectoryRef: %v", err)
			}
			var got localDirectoryRef
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("unmarshal normalized ref: %v", err)
			}
			if got.ExecutionMode != tc.want {
				t.Errorf("ExecutionMode = %q, want %q", got.ExecutionMode, tc.want)
			}
		})
	}

	rejected := []string{"snapshot", "WORKTREE", "in-place", "true"}
	for _, mode := range rejected {
		t.Run("rejects "+mode, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{
				"local_path":     "/Users/foo/work",
				"daemon_id":      "d1",
				"execution_mode": mode,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if _, err := validateLocalDirectoryRef(raw); err == nil {
				t.Errorf("execution_mode %q was accepted, want a validation error", mode)
			}
		})
	}
}

// latestDaemonCLIVersion feeds the worktree-mode save gate: an old daemon
// json-skips execution_mode and would run tasks in place, so the gate has to
// read the version of the binary actually running on the machine — the
// freshest version-bearing row for that daemon_id, never a stale sibling row.
func TestLatestDaemonCLIVersion(t *testing.T) {
	row := func(daemonID, version string, seen time.Time) db.AgentRuntime {
		rt := db.AgentRuntime{
			DaemonID:   pgtype.Text{String: daemonID, Valid: daemonID != ""},
			LastSeenAt: pgtype.Timestamptz{Time: seen, Valid: !seen.IsZero()},
		}
		if version != "" {
			rt.Metadata = []byte(`{"cli_version":"` + version + `"}`)
		}
		return rt
	}
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		runtimes []db.AgentRuntime
		daemonID string
		want     string
	}{
		{
			name:     "no rows at all",
			runtimes: nil,
			daemonID: "d1",
			want:     "",
		},
		{
			name:     "only other daemons",
			runtimes: []db.AgentRuntime{row("d2", "0.9.9", base)},
			daemonID: "d1",
			want:     "",
		},
		{
			name:     "single match",
			runtimes: []db.AgentRuntime{row("d1", "0.4.24", base)},
			daemonID: "d1",
			want:     "0.4.24",
		},
		{
			name: "freshest row wins regardless of order",
			runtimes: []db.AgentRuntime{
				row("d1", "0.4.30", base.Add(2*time.Hour)),
				row("d1", "0.4.20", base),
			},
			daemonID: "d1",
			want:     "0.4.30",
		},
		{
			name: "stale-but-fresher row without a version does not mask an older versioned row",
			runtimes: []db.AgentRuntime{
				row("d1", "", base.Add(2*time.Hour)),
				row("d1", "0.4.24", base),
			},
			daemonID: "d1",
			want:     "0.4.24",
		},
		{
			name:     "match with no version anywhere",
			runtimes: []db.AgentRuntime{row("d1", "", base)},
			daemonID: "d1",
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := latestDaemonCLIVersion(tc.runtimes, tc.daemonID); got != tc.want {
				t.Errorf("latestDaemonCLIVersion = %q, want %q", got, tc.want)
			}
		})
	}
}
