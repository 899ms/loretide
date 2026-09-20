package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"strings"
	"testing"
	"time"
)

func localDirRef(t *testing.T, path, daemonID, mode string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{
		"local_path": path, "daemon_id": daemonID, "execution_mode": mode,
	})
	if err != nil {
		t.Fatalf("marshal ref: %v", err)
	}
	return raw
}

func runtimeWithVersion(daemonID, cliVersion string) db.AgentRuntime {
	rt := db.AgentRuntime{DaemonID: pgtype.Text{String: daemonID, Valid: daemonID != ""}}
	if cliVersion != "" {
		rt.Metadata, _ = json.Marshal(map[string]string{"cli_version": cliVersion})
	}
	return rt
}

// The save-time gate cannot cover a machine downgraded after the resource was
// written, and an old daemon json-skips execution_mode entirely — it would run
// the task IN PLACE, editing the working copy the user asked to isolate. This
// is the last point where that can be stopped.
func TestWorktreeClaimBlockReason(t *testing.T) {
	const daemon = "daemon-a"

	worktreeRes := []ProjectResourceData{{
		ID: "r1", ResourceType: "local_directory",
		ResourceRef: localDirRef(t, "/Users/dev/game", daemon, "worktree"),
	}}

	t.Run("blocks a runtime that does not advertise the capability", func(t *testing.T) {
		reason := worktreeClaimBlockReason(worktreeRes, runtimeWithVersion(daemon, "0.4.10"), false)
		if reason == "" {
			t.Fatal("an outdated runtime was allowed to claim a worktree task")
		}
		if !strings.Contains(reason, "/Users/dev/game") || !strings.Contains(reason, "Update the Multica app") {
			t.Errorf("reason should name the directory and tell the user to update, got: %q", reason)
		}
	})

	t.Run("blocks a runtime that advertises nothing at all", func(t *testing.T) {
		// Fail closed: "no version" is what a daemon far older than the field
		// looks like, which is exactly the dangerous case.
		if worktreeClaimBlockReason(worktreeRes, runtimeWithVersion(daemon, ""), false) == "" {
			t.Error("a runtime advertising nothing was allowed to claim")
		}
	})

	t.Run("allows a runtime that advertises the capability", func(t *testing.T) {
		if reason := worktreeClaimBlockReason(worktreeRes, runtimeWithVersion(daemon, "9.9.9"), true); reason != "" {
			t.Errorf("a capable runtime was blocked: %q", reason)
		}
	})

	t.Run("ignores in_place and absent modes", func(t *testing.T) {
		for _, mode := range []string{"in_place", ""} {
			res := []ProjectResourceData{{
				ID: "r1", ResourceType: "local_directory",
				ResourceRef: localDirRef(t, "/Users/dev/game", daemon, mode),
			}}
			if reason := worktreeClaimBlockReason(res, runtimeWithVersion(daemon, "0.1.0"), false); reason != "" {
				t.Errorf("mode %q blocked a daemon that can run it fine: %q", mode, reason)
			}
		}
	})

	// A project may carry one local_directory per machine. Another machine's
	// worktree resource says nothing about this runtime's ability to run.
	t.Run("ignores a resource bound to a different daemon", func(t *testing.T) {
		other := []ProjectResourceData{{
			ID: "r1", ResourceType: "local_directory",
			ResourceRef: localDirRef(t, "/Users/dev/game", "daemon-b", "worktree"),
		}}
		if reason := worktreeClaimBlockReason(other, runtimeWithVersion(daemon, "0.1.0"), false); reason != "" {
			t.Errorf("another machine's resource blocked this claim: %q", reason)
		}
	})

	t.Run("ignores non-local_directory resources", func(t *testing.T) {
		repo := []ProjectResourceData{{
			ID: "r1", ResourceType: "github_repo",
			ResourceRef: json.RawMessage(`{"url":"https://github.com/a/b"}`),
		}}
		if reason := worktreeClaimBlockReason(repo, runtimeWithVersion(daemon, "0.1.0"), false); reason != "" {
			t.Errorf("github_repo resource blocked a claim: %q", reason)
		}
	})

	t.Run("ignores a runtime with no daemon id", func(t *testing.T) {
		if reason := worktreeClaimBlockReason(worktreeRes, runtimeWithVersion("", "0.1.0"), false); reason != "" {
			t.Errorf("cloud runtime blocked: %q", reason)
		}
	})

	t.Run("survives a malformed ref", func(t *testing.T) {
		bad := []ProjectResourceData{{
			ID: "r1", ResourceType: "local_directory",
			ResourceRef: json.RawMessage(`{"local_path": 42}`),
		}}
		if reason := worktreeClaimBlockReason(bad, runtimeWithVersion(daemon, "0.1.0"), false); reason != "" {
			t.Errorf("malformed ref produced a block: %q", reason)
		}
	})
}

// The bug this gate was rebuilt for: a dev-built daemon reports a git-describe
// version that the version floor deliberately exempts, so the old version-based
// gate waved through a binary with no worktree implementation and two tasks ran
// in the user's own directory (MUL-5707). The capability signal is immune to
// how the version string happens to be spelled.
func TestWorktreeClaimGateIgnoresVersionStrings(t *testing.T) {
	const daemon = "daemon-a"
	res := []ProjectResourceData{{
		ID: "r1", ResourceType: "local_directory",
		ResourceRef: localDirRef(t, "/Users/dev/game", daemon, "worktree"),
	}}

	// Every one of these is a version string that the old floor check would
	// have ALLOWED. Without the capability, all must now be blocked.
	for _, version := range []string{
		"v0.4.21-24-gcd3c0bb89", // the exact dev-describe build that got through
		"0.4.24",                // at the floor
		"9.9.9",                 // far above it
		"",                      // none reported
	} {
		if worktreeClaimBlockReason(res, runtimeWithVersion(daemon, version), false) == "" {
			t.Errorf("version %q was allowed to claim without advertising the capability", version)
		}
	}

	// And the converse: a capable daemon runs regardless of how old its version
	// string looks, so the gate can never strand a runtime that actually works.
	for _, version := range []string{"v0.4.21-24-gcd3c0bb89", "0.0.1", ""} {
		if reason := worktreeClaimBlockReason(res, runtimeWithVersion(daemon, version), true); reason != "" {
			t.Errorf("version %q blocked a capable runtime: %q", version, reason)
		}
	}
}

// The stored-capability read backs the save-time gate and the UI, which cannot
// see the live request. Absent metadata is an older daemon: fail closed.
func TestRuntimeHasCapability(t *testing.T) {
	withCaps := func(caps ...string) []byte {
		raw, err := json.Marshal(map[string]any{"capabilities": caps})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return raw
	}

	if !runtimeHasCapability(withCaps("skill-bundles-v1", "local-worktree-v1"), "local-worktree-v1") {
		t.Error("advertised capability not detected")
	}
	if runtimeHasCapability(withCaps("skill-bundles-v1"), "local-worktree-v1") {
		t.Error("unadvertised capability reported as present")
	}
	for _, metadata := range [][]byte{
		nil,
		[]byte(`{}`),
		[]byte(`{"cli_version":"9.9.9"}`), // an old daemon: version, no capabilities
		[]byte(`not json`),
	} {
		if runtimeHasCapability(metadata, "local-worktree-v1") {
			t.Errorf("metadata %q reported the capability as present", string(metadata))
		}
	}
}

func runtimeRow(daemonID string, seenAt time.Time, caps ...string) db.AgentRuntime {
	rt := db.AgentRuntime{
		DaemonID:   pgtype.Text{String: daemonID, Valid: daemonID != ""},
		LastSeenAt: pgtype.Timestamptz{Time: seenAt, Valid: !seenAt.IsZero()},
	}
	payload := map[string]any{"cli_version": "9.9.9"}
	if len(caps) > 0 {
		payload["capabilities"] = caps
	}
	rt.Metadata, _ = json.Marshal(payload)
	return rt
}

// Deregistering a runtime only flips the row to offline — its metadata,
// capabilities included, survives — and ListAgentRuntimes returns every row. So
// "any row advertised it" keeps answering yes long after the machine downgraded,
// and the save gate and UI would keep offering a mode the claim gate then
// refuses. Newest-seen row wins.
func TestDaemonAdvertisesWorktreeUsesNewestRow(t *testing.T) {
	const daemon = "daemon-a"
	older := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)

	t.Run("stale capable row does not rescue a downgraded daemon", func(t *testing.T) {
		rows := []db.AgentRuntime{
			runtimeRow(daemon, older, "local-worktree-v1"), // left behind by the old capable build
			runtimeRow(daemon, newer),                      // what the machine runs now
		}
		if daemonAdvertisesWorktree(rows, daemon) {
			t.Error("a stale capable row was allowed to vouch for a downgraded daemon")
		}
	})

	t.Run("newest capable row wins over an older incapable one", func(t *testing.T) {
		rows := []db.AgentRuntime{
			runtimeRow(daemon, older),
			runtimeRow(daemon, newer, "local-worktree-v1"), // the upgrade
		}
		if !daemonAdvertisesWorktree(rows, daemon) {
			t.Error("an upgraded daemon was not recognised")
		}
	})

	t.Run("row order does not matter", func(t *testing.T) {
		rows := []db.AgentRuntime{
			runtimeRow(daemon, newer),
			runtimeRow(daemon, older, "local-worktree-v1"),
		}
		if daemonAdvertisesWorktree(rows, daemon) {
			t.Error("result depended on slice order")
		}
	})

	t.Run("a row that never reported loses to one that did", func(t *testing.T) {
		var never time.Time
		rows := []db.AgentRuntime{
			runtimeRow(daemon, never, "local-worktree-v1"),
			runtimeRow(daemon, newer),
		}
		if daemonAdvertisesWorktree(rows, daemon) {
			t.Error("a never-seen row outvoted a live one")
		}
	})

	t.Run("ignores other daemons and empty ids", func(t *testing.T) {
		rows := []db.AgentRuntime{runtimeRow("daemon-b", newer, "local-worktree-v1")}
		if daemonAdvertisesWorktree(rows, daemon) {
			t.Error("another machine's row vouched for this daemon")
		}
		if daemonAdvertisesWorktree(rows, "") {
			t.Error("empty daemon id matched a row")
		}
		if daemonAdvertisesWorktree(nil, daemon) {
			t.Error("no rows at all reported capable")
		}
	})
}
