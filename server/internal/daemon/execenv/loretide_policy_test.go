package execenv

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/multica-ai/multica/server/pkg/executionpolicy"
)

var policyEnvValues = []string{"__unset__", "disabled", "enabled", "unknown"}

// setPolicyEnv installs a setter for LORETIDE_EXECUTION_POLICY and restores the
// original value on cleanup. The gate ignores this variable; the matrix proves
// no value can flip it at the execenv entry points.
func setPolicyEnv(t *testing.T) func(string) {
	t.Helper()
	const key = "LORETIDE_EXECUTION_POLICY"
	orig, had := os.LookupEnv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, orig)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	return func(value string) {
		if value == "__unset__" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, value)
		}
	}
}

// TestLoretidePrepareRejectsWithErrDisabledAndNoSideEffects verifies Prepare
// fails closed with executionpolicy.ErrDisabled before doing any work.
//
// The params are otherwise fully valid: WorkspacesRoot, WorkspaceID, TaskID and
// Provider are all set with synthetic values. With the gate removed, Prepare
// would pass its argument validation and begin building the environment tree,
// so a rejection here can only come from the gate — not from a missing-field
// error like "workspace ID is required". Asserting errors.Is(ErrDisabled) plus
// the absence of any filesystem side effect therefore detects a bypass, which a
// bare err != nil check would not.
func TestLoretidePrepareRejectsWithErrDisabledAndNoSideEffects(t *testing.T) {
	setPolicy := setPolicyEnv(t)
	for _, env := range policyEnvValues {
		setPolicy(env)
		base := t.TempDir()
		root := filepath.Join(base, "workspaces")
		params := PrepareParams{
			WorkspacesRoot: root,
			WorkspaceID:    "00000000-0000-0000-0000-000000000001",
			WorkspaceSlug:  "synthetic-ws",
			TaskID:         "00000000-0000-0000-0000-000000000002",
			Provider:       "claude",
		}
		environment, err := Prepare(params, slog.Default())
		if environment != nil {
			t.Fatalf("[%s] Prepare returned a non-nil environment while execution is disabled", env)
		}
		if !errors.Is(err, executionpolicy.ErrDisabled) {
			t.Fatalf("[%s] Prepare error = %v; want executionpolicy.ErrDisabled", env, err)
		}
		if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
			t.Fatalf("[%s] Prepare created filesystem state at %s before the gate rejected", env, root)
		}
	}
}

// TestLoretideReuseReturnsNilWithoutSideEffects verifies Reuse fails closed by
// returning a nil *Environment and touching nothing.
//
// Reuse has no error channel; its contract on the gate is a nil return. The
// gate runs before Reuse's os.Stat(WorkDir) check, so a WorkDir that actually
// exists means a nil result can only be the gate: without it, Reuse would move
// past os.Stat and create a "multica-config" sidecar directory under the
// workdir's parent before returning a non-nil environment. Asserting nil and
// the absence of that sidecar detects a bypass.
func TestLoretideReuseReturnsNilWithoutSideEffects(t *testing.T) {
	setPolicy := setPolicyEnv(t)
	for _, env := range policyEnvValues {
		setPolicy(env)
		base := t.TempDir()
		workDir := filepath.Join(base, "workdir")
		if err := os.MkdirAll(workDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if got := Reuse(ReuseParams{WorkDir: workDir}, slog.Default()); got != nil {
			t.Fatalf("[%s] Reuse returned a non-nil environment while execution is disabled", env)
		}
		if _, statErr := os.Stat(filepath.Join(base, "multica-config")); !os.IsNotExist(statErr) {
			t.Fatalf("[%s] Reuse created a sidecar directory before the gate rejected", env)
		}
	}
}
