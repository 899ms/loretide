// Package executionpolicy holds the Loretide development execution gate.
package executionpolicy

import (
	"errors"
)

var ErrDisabled = errors.New("LORETIDE_EXECUTION_DISABLED: real execution is not security-verified; use the diagnostics simulator")

// Check runs before provider discovery, credential preparation or subprocess creation.
// No enabled value is supported until a separately reviewed adapter is delivered.
func Check() error {
	return ErrDisabled
}
