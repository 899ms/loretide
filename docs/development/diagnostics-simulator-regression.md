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
- Checks:
  - Exact failure code and status.
  - Failing component attribution (e.g. `timeout` fails at `executor`, `file_missing` fails at `tool`, `reconnect` fails at `daemon`).
  - Attempt retry incrementation on reconnection scenarios.
  - Event sequence length and outcomes (`failed`, `cancelled`, `ignored`).

### 3. Virtual Determinism & Identifier Correlation (`TestSimulatorRegressionDeterministicTimeAndIdentifiers`)
- Runs identical seeds to verify exact event duration and virtual `Occurred` timestamps.
- Confirms wall-clock `Created` time and runtime UUIDs (`ID`, `Trace`, `Span`, `Operation`) are not verbatim duplicates across distinct runs.
- Verifies that all steps within a run share the same `Operation` and `Trace` context.

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
