package agent

import (
	"errors"
	"os"
	"testing"

	"github.com/multica-ai/multica/server/pkg/executionpolicy"
)

// policyEnvValues exercises the gate under every LORETIDE_EXECUTION_POLICY
// configuration: unset, the documented disabled/enabled values, and an
// unrecognised one. The gate ignores this variable, so every case must still
// fail closed; iterating them proves no configuration can flip it.
var policyEnvValues = []string{"__unset__", "disabled", "enabled", "unknown"}

// setPolicyEnv installs a setter that applies one policyEnvValues entry and a
// t.Cleanup that restores the original value, so the test never leaks state.
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

// TestLoretideGateRejectsEveryRegisteredFactory asserts the disabled-execution
// gate rejects every registered construction entry point with
// executionpolicy.ErrDisabled — not merely with some non-nil error. Matching
// the sentinel (rather than asserting err != nil) is what distinguishes the
// gate firing from an unrelated failure such as "unknown agent type" or a
// config validation error, so this test fails if the gate is bypassed or
// replaced by a different rejection.
func TestLoretideGateRejectsEveryRegisteredFactory(t *testing.T) {
	setPolicy := setPolicyEnv(t)
	assertDisabled := func(label string, b Backend, err error) {
		t.Helper()
		if b != nil {
			t.Fatalf("%s returned a non-nil backend while execution is disabled", label)
		}
		if !errors.Is(err, executionpolicy.ErrDisabled) {
			t.Fatalf("%s error = %v; want executionpolicy.ErrDisabled", label, err)
		}
	}
	for _, env := range policyEnvValues {
		setPolicy(env)
		// Every protocol family the New() switch registers.
		for _, family := range SupportedTypes {
			b, err := New(family, Config{})
			assertDisabled("New("+family+") ["+env+"]", b, err)
			rb, rerr := ResolveBackend(family, Config{})
			assertDisabled("ResolveBackend("+family+") ["+env+"]", rb, rerr)
		}
		// The gate must precede type validation: even an unregistered type is
		// rejected with ErrDisabled, not "unknown agent type". If the gate were
		// removed this call would surface the type error instead and errors.Is
		// would fail.
		b, err := New("definitely-not-a-registered-provider", Config{})
		assertDisabled("New(unregistered) ["+env+"]", b, err)
		// Every built-in runtime identity, through both NewRuntime and the
		// production ResolveBackend router. NewRuntime wraps the gate error with
		// %w, so errors.Is must still recover it.
		for _, rt := range BuiltinRuntimes {
			b, err := NewRuntime(rt.ID, Config{})
			assertDisabled("NewRuntime("+rt.ID+") ["+env+"]", b, err)
			rb, rerr := ResolveBackend(rt.ID, Config{})
			assertDisabled("ResolveBackend("+rt.ID+") ["+env+"]", rb, rerr)
		}
	}
}
