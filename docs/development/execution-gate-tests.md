# Execution-gate regression tests (Loretide real-executor disable)

Loretide keeps real agent execution unconditionally disabled until a separately
reviewed adapter lands. The gate is `executionpolicy.Check()`
(`server/pkg/executionpolicy/policy.go`), which always returns the sentinel
`executionpolicy.ErrDisabled`. Production entry points call it before any
provider discovery, credential preparation, or subprocess creation.

These tests are the **non-UI** regression for that gate. They relate to the gate
verification part of LT-004 and do **not** constitute a real-executor security
acceptance. This document and the tests cover static/interface behavior only;
they do not prove runtime privilege containment.

## Files

| Test file | Entry point under test |
| --- | --- |
| `server/pkg/executionpolicy/policy_test.go` | `Check()` across all configs; sentinel-through-`%w` contract |
| `server/pkg/agent/loretide_policy_test.go` | `New`, `NewRuntime`, `ResolveBackend` for every registered factory |
| `server/internal/daemon/execenv/loretide_policy_test.go` | `Prepare` and `Reuse` |

## What they assert, and why err != nil is not enough

A bare `err != nil` assertion is satisfied by *any* failure — an "unknown agent
type" error, a missing-required-field error, a filesystem error — so it keeps
passing even if the gate is removed and the call fails later for an unrelated
reason. These tests instead assert the specific sentinel with
`errors.Is(err, executionpolicy.ErrDisabled)`, which fails if the gate is
bypassed or replaced by a different rejection:

- **executionpolicy**: `Check()` returns `ErrDisabled` for unset, `disabled`,
  `enabled` and `unknown` values of `LORETIDE_EXECUTION_POLICY` (the gate
  ignores the variable). A separate test wraps the error with `%w` and confirms
  `errors.Is` still recovers it — the contract the agent and execenv gates rely
  on — and that an unrelated error does not match.
- **agent**: every protocol family in `agent.SupportedTypes` and every built-in
  runtime in `agent.BuiltinRuntimes` is rejected with `ErrDisabled` through
  `New`, `NewRuntime` and the production `ResolveBackend` router. The lists are
  iterated from the real registries, so a newly registered factory is covered
  automatically. An unregistered type is also asserted to return `ErrDisabled`,
  proving the gate runs before type validation. `NewRuntime` wraps the gate
  error with `%w`; `errors.Is` matching confirms the wrap is preserved.
- **execenv**: `Prepare` is called with otherwise fully-valid synthetic params
  (all required fields set). With the gate removed it would pass validation and
  begin building the environment tree, so asserting `ErrDisabled` **and** that
  the workspaces root was never created detects a bypass that a missing-field
  error would mask. `Reuse` has no error channel; its gate returns `nil`. The
  gate runs before `Reuse`'s `os.Stat(WorkDir)` check, so the test passes an
  **existing** WorkDir: without the gate, `Reuse` would proceed and create a
  `multica-config` sidecar under the workdir's parent and return a non-nil
  environment. Asserting `nil` and the absence of that sidecar detects a bypass.

All inputs are synthetic and use `t.TempDir()`. The tests never read the user's
`HOME`, never touch real model credentials, never invoke a real provider CLI,
and never start a service.

## Running

From `server/`:

```bash
go test ./pkg/executionpolicy -run 'TestConfiguredPolicy|TestMissingPolicy|TestErrDisabledMatchesThroughWrapping' -count=1 -v
go test ./pkg/agent -run 'TestLoretideGateRejectsEveryRegisteredFactory' -count=1 -v
go test ./internal/daemon/execenv -run 'TestLoretidePrepareRejectsWithErrDisabledAndNoSideEffects|TestLoretideReuseReturnsNilWithoutSideEffects' -count=1 -v
```

## Bypass detection (mutation check)

To confirm the assertions detect a fail-open gate, temporarily change
`Check()` to `return nil` (a throwaway local edit, never committed) and re-run
the commands above: all six tests fail — the executionpolicy tests report "gate
failed open", the agent test reports a non-nil backend, and the execenv tests
report a non-nil environment plus the created filesystem state. Reverting
`policy.go` restores green. This demonstrates the tests fail closed on a real
bypass rather than merely re-stating the constant implementation.
