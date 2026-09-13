package executionpolicy

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

// TestErrDisabledMatchesThroughWrapping guards the sentinel contract that the
// agent and execenv gates depend on: they wrap Check()'s error with %w, and
// callers recover it with errors.Is. If Check stopped returning the ErrDisabled
// sentinel (e.g. a freshly constructed error per call), those errors.Is checks
// would silently fail open, so this locks the sentinel identity here.
func TestErrDisabledMatchesThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("runtime %q (family %q): %w", "omp", "pi", Check())
	if !errors.Is(wrapped, ErrDisabled) {
		t.Fatal("wrapped gate error no longer matches ErrDisabled")
	}
	if errors.Is(errors.New("unrelated failure"), ErrDisabled) {
		t.Fatal("an unrelated error matched ErrDisabled")
	}
}

func TestConfiguredPolicyCannotEnableExecution(t *testing.T) {
	for _, value := range []string{"disabled", "enabled", "", "unknown"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LORETIDE_EXECUTION_POLICY", value)
			if !errors.Is(Check(), ErrDisabled) {
				t.Fatal("gate failed open")
			}
		})
	}
}

func TestMissingPolicyCannotEnableExecution(t *testing.T) {
	previous, exists := os.LookupEnv("LORETIDE_EXECUTION_POLICY")
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv("LORETIDE_EXECUTION_POLICY", previous)
		} else {
			_ = os.Unsetenv("LORETIDE_EXECUTION_POLICY")
		}
	})
	_ = os.Unsetenv("LORETIDE_EXECUTION_POLICY")
	if !errors.Is(Check(), ErrDisabled) {
		t.Fatal("missing configuration enabled execution")
	}
}
