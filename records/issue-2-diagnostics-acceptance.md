# Issue #2 diagnostics acceptance matrix

Date: 2026-09-13
Executor: Terra-DIAG02
Issue: https://github.com/899ms/loretide/issues/2
Application baseline: `app-main` / `bb3a4b4616bf48a52e075bbe5e23e38112bbf6aa`
Branch: `task/issue-2-diagnostics-acceptance` (local branch before push: `terra-diag02-issue-2`)

## Isolation and evidence rules

Reserved acceptance resources are Web `13102`, API `18102`, and PostgreSQL `15402`.
The preflight observed no listeners on those ports. The existing development Web `13000`, API `18000`, and PostgreSQL `15332` were not read, stopped, changed, or used.

`tests/diagnostics-acceptance/verify.ps1` is the repeatable verifier. It runs source-level assertions and an explicitly selected, database-free Go diagnostic subset. Supplying `-DatabaseUrl` is optional and rejected unless it targets `localhost:15402`; only then does it run the existing PostgreSQL diagnostic integration tests. It does not start Docker, create an environment, inspect model credentials, contact a VPS, or enable a real executor.

Run from the application checkout:

```powershell
pwsh -File tests/diagnostics-acceptance/verify.ps1
pwsh -File tests/diagnostics-acceptance/verify.ps1 -DatabaseUrl 'postgres://USER:PASSWORD@localhost:15402/DB?sslmode=disable'
```

The second command is intentionally not represented as passed until an isolated database exists on the reserved port. Do not place its connection string in a committed log or PR description.

Executed result: `pwsh -NoProfile -File tests/diagnostics-acceptance/verify.ps1` exited `0` on 2026-09-13. Its seven static assertions and selected Go unit/transport suite passed (`github.com/multica-ai/multica/server/internal/content/diagnostics`, `0.131s`). The optional PostgreSQL suite was explicitly skipped because no isolated `15402` database URL was supplied. `node_modules` was absent, so no Node/React test or browser test was run.

## Acceptance matrix

| Requirement | Evidence location | Status | Evidence / boundary |
| --- | --- | --- | --- |
| Actual download is JSON, not merely a click | `apps/web/platform/content-diagnostics.ts`; its focused test | Static verified | Blob is constructed as `application/json` before the anchor click; the focused frontend test parses the blob. Browser download was not executed in this checkout. |
| Export matches selected authorized run | `server/internal/content/diagnostics/service.go:Export`; PostgreSQL integration test | Not executed | The service loads the requested run through scoped access. Requires isolated DB and browser/API evidence to mark pass. |
| Cross workspace/account denial and redaction | `contract_test.go`, `log_test.go`, `store_integration_test.go` | Unit static verified; DB integration not executed | Unit coverage checks scope isolation and secret removal. No real account/browser result is claimed. |
| Stream pause, resume cursor, de-duplication | `packages/core/content/diagnostics/queries.ts`; focused React test | Static verified | Code retains cursor, merges by event ID, and stops work when paused. Node dependencies were absent, so the React test was not run. |
| Network loss / reconnect notice | `queries.ts`; focused React test | Static verified | The client sets a disconnected notice and reconnects from saved cursor. No browser websocket run occurred. |
| Retention gap | `store_integration_test.go`; `queries.ts` | Not executed | DB test asserts an expired cursor produces a gap; UI renders a gap notice. Requires isolated PostgreSQL/API/browser proof. |
| Permission-scoped diagnostics export | `service.go:Export`, `store.go:GetRun` | Static verified | Export loads its run through `GetRun(scope, id)` and uses scoped audit/technical queries. Runtime access result is not claimed. |
| Real executor boundary | `service.go`, `simulator_test.go`, `transport_test.go` | Unit verified | Simulation is test-gated and the export declares real replay unavailable. This is not evidence that any real executor works. |

## Test design and remaining work

1. Create only an isolated PostgreSQL database on `15402`, then run the optional verifier command. Retain only its redacted exit/result summary.
2. Install the locked Node dependencies in this checkout, run `apps/web/platform/content-diagnostics.test.ts` and `packages/core/content/diagnostics/queries.test.tsx`, and record exact exit codes.
3. Start only the reserved isolated API/Web pair (`18102`/`13102`) and perform browser evidence: choose a known synthetic run, preview/export it, parse the downloaded JSON, verify run ID, test forbidden workspace/account, pause/resume, reconnect, duplicate event, and retention-gap indication.
4. Keep every browser result `blocked` or `not executed` until it has its own screenshot, downloaded artifact assertion, and endpoint/operation evidence. Never mark the real executor as passed for this Issue.

## Defects observed during static review

No production code was changed by this Issue. The following requires a separate repair Issue if accepted as a defect: a complete diagnostic export downloads the `Export` object as JSON but the current focused browser-side test only asserts JSON readability, not that a UI-selected run ID equals the downloaded bundle's run ID. This task records the gap and does not modify production code or existing tests.

## Changed-file map and rollback

| File | Purpose |
| --- | --- |
| `tests/diagnostics-acceptance/verify.ps1` | Safe repeatable static/unit verifier; optional isolated PostgreSQL gate. |
| `records/issue-2-diagnostics-acceptance.md` | This matrix, status boundaries, reproduction commands, evidence plan, and rollback record. |

Rollback is limited to deleting these two Issue-owned files from this branch. No existing code, environment, database, or evidence was altered.
