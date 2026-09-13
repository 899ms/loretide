package executionpolicy

import (
	"errors"
	"os"
	"testing"
)

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
