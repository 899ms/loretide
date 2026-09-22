# Issue #235 — 诊断 503 早退结构化错误

- Issue: https://github.com/899ms/loretide/issues/235
- Base: `app-main` `01618357fb665fbf99412b696248b85bd846a229`
- Branch: `codex/diagnostic-early-errors`

## Change

`diagnosticScope` now returns the structured `INTERNAL` error when the
diagnostics service is absent. `ContentDiagnosticStream` preserves
authorization first and, only after it succeeds, returns the same structured
`INTERNAL` error when the response writer cannot flush.

`INTERNAL` is the existing non-database sanitized server-failure code. The
diagnostics code allowlist is intentionally closed by
`TestSanitizeCodeTableIsUnchanged`, so this change does not add new error-code
shapes merely to distinguish the two transport failures. Both responses use
the `diagnostics` component and the existing `INTERNAL` contract
(`retryable=false`, `next_action=inspect_trace`). Existing `diagnosticError`
behavior for authorization, input conflict, and database errors is unchanged;
no non-database failure is mapped to `DATABASE_UNAVAILABLE`.

## Tests

`TestContentDiagnosticEarlyErrorsStayStructured` covers the two actual handler
paths. The stream case uses an authorized workspace fixture before reaching the
non-Flusher branch, so it cannot pass by changing authorization order.

The handler package is a fail-closed database suite. Per Issue #235, it is not
run against a local database; GitHub CI must run it with its isolated hosted
database. Local verification is limited to compilation/static checks and the
exact closed-code-table guard that do not open a database.

## Manual Todo

After CI, the user may verify in an independent instance that diagnostics
overview and an unavailable real-time-log connection display the stable error
code and next action without revealing internal or business content. This is
not a UI-unit-test substitute.

## Rollback

Revert this task's commit. No migration, environment, service, or data change
is involved.
