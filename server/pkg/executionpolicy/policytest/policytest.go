// Package policytest is the test-side counterpart of the Loretide execution
// gate. It exists so an upstream Multica test that cannot run while the gate is
// closed says so, instead of failing with an error that reads like a product
// regression.
package policytest

import (
	"testing"

	"github.com/multica-ai/multica/server/pkg/executionpolicy"
)

// SkipIfExecutionGated skips the calling test when the Loretide execution gate
// is closed.
//
// It is for upstream tests only: those that reach LoadConfig, execenv.Prepare or
// execenv.Reuse, all of which fail closed on executionpolicy.Check. Keeping them
// as failures buried the signal - 108 red lines that never named a real defect -
// and the ones that fail downstream of the gate are actively misleading (a
// runTask test reports "StartTask placement is missing" when runTask simply
// returned at prepare).
//
// This is NOT a bypass. It queries the gate and never changes it; there is no
// value of any flag or environment variable that makes Check succeed. When a
// reviewed adapter opens the gate, every call here becomes a no-op and the tests
// run again, unedited. See constitution principle IX.
func SkipIfExecutionGated(t *testing.T) {
	t.Helper()
	if err := executionpolicy.Check(); err != nil {
		t.Skipf("upstream test requires the execution gate to be open: %v (constitution principle IX)", err)
	}
}
