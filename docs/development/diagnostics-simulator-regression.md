# Diagnostics Simulator Regression Tests

This document outlines the non-UI regression tests for the diagnostics simulation engine in `server/internal/content/diagnostics/simulator_regression_test.go`.

These tests satisfy the contract and determinism regression requirements of Issue #14 (DIAG-SIM-TEST-01).

## Scope & Boundaries

- **File under test**: `server/internal/content/diagnostics/simulator.go` (`Simulate`, `Receiver`, `Scenarios`, `Evaluate`).
- **Test file**: `server/internal/content/diagnostics/simulator_regression_test.go`
- **Exclusions**: Real persistence layer and database rollback tests are excluded and owned by Issue #2 (`store_integration_test.go`). Real agent execution and external model calls are unconditionally disabled.

## Covered Behaviors and Assertions

### 1. Standalone Receiver Contract (`TestSimulatorRegressionReceiverContracts`)
- **Strictly Increasing Sequences**: Monotonic sequences (`1, 2, 3, ...`) are accepted cleanly (returns `""`).
- **Duplicate Sequences**: Re-receiving an already seen sequence yields `"DUPLICATE"`.
- **Out-of-Order Sequences**: Sequences arriving behind the high-water mark yield `"LATE_RESULT"`.
- **Cancellation**: Once `Cancelled = true`, any subsequent events are rejected as `"LATE_RESULT"`.

### 2. Independent Scenario Failure Assertions (`TestSimulatorRegressionScenarioContracts`)
- Verifies contract failure codes without deriving expected values from `Scenarios[i].Expected`, preventing tautological test passes if scenarios are renamed or altered.
- **Exact Sequence and Termination Point**: Asserts the exact slice of component names and lengths for every scenario (e.g. `timeout` and `cancel` terminate immediately after `executor` with 6 events, `late` terminates after `result` with 8 events, `normal`/`reconnect` run full 9 events).
- **Exact Outcomes**: Distinguishes precise event outcomes — `"cancelled"` for cancel, `"ignored"` for duplicate/late, `"failed"` for error conditions, and `"success"` for non-failing steps.
- **Attempt Transitions**: Verifies `reconnect` transitions from `attempt = 1` for steps preceding `daemon` to `attempt = 2` from `daemon` onwards.

### 3. Virtual Determinism & Identifier Correlation (`TestSimulatorRegressionDeterministicTimeAndIdentifiers`)
- Runs identical seeds to verify exact event duration and virtual `Occurred` timestamps.
- Checks that runtime UUIDs (`ID`, `Trace`, `Span`, `Operation`) are unique across distinct runs, without asserting wall-clock `Created` inequality.
- Confirms that within a run, all steps correlate under the identical `Operation` and `Trace` context.

### 4. Gate Rejection and Scope Authorization (`TestSimulatorRegressionRejectionAndSecurity`)
- `testEnabled = false` rejects immediately with `ErrDenied`.
- Scope rejection when account is not authorized rejects with `ErrDenied`.
- Unknown scenario identifiers reject with `ErrConflict`.
- Database scenario requirement boundaries are verified without mocking fake persistence coverage.

## Running Tests

From `server/`:

```bash
# Direct regression run
go test -v ./internal/content/diagnostics -run TestSimulatorRegression -count=1

# Race detector
go test -race -v ./internal/content/diagnostics -run TestSimulatorRegression -count=1
```
