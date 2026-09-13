# Issue #2 diagnostics acceptance matrix

Date: 2026-09-13
Executor: Terra-DIAG02
Issue: https://github.com/899ms/loretide/issues/2
Application baseline: `app-main` / `bb3a4b4616bf48a52e075bbe5e23e38112bbf6aa`
Branch: `task/issue-2-diagnostics-acceptance` (local branch before push: `terra-diag02-issue-2`)

## Isolation and evidence rules

Reserved acceptance resources are Web `13102`, API `18102`, and PostgreSQL `15402`.
The preflight observed no listeners on those ports. The existing development Web `13000`, API `18000`, and PostgreSQL `15332` were not read, stopped, changed, or used.

`tests/diagnostics-acceptance/verify.ps1` is the repeatable verifier. Its source-presence checks only confirm that expected wiring remains present; they never prove behavior. Behavior is accepted only from the named Go and Vitest suites: Go JSON must show every requested test exactly once as both `run` and `pass`, with no `skip` or `fail`; Vitest JSON must show both requested files/two tests passing. Supplying `-DatabaseUrl` is optional and rejected unless it names a credentialed, loopback `localhost:15402` database with the `loretide_diag_acceptance_*` naming convention and no URI query override other than `sslmode`. Before any database test it requires a PostgreSQL protocol probe proving the requested `current_database`, `current_user`, and non-superuser role; its temporary environment variables and caller working directory are restored in `finally`. It does not echo the URL, password, or raw test error output, start Docker, create an environment, inspect model credentials, contact a VPS, or enable a real executor.

Run from the application checkout:

```powershell
pwsh -File tests/diagnostics-acceptance/verify.ps1
pwsh -File tests/diagnostics-acceptance/verify.ps1 -DatabaseUrl 'postgres://USER:PASSWORD@localhost:15402/loretide_diag_acceptance_example?sslmode=disable'
```

The second command is intentionally not represented as passed until an isolated database exists on the reserved port. Do not place its connection string in a committed log or PR description.

Executed result: `pnpm install --frozen-lockfile` completed successfully on 2026-09-13. `pwsh -NoProfile -File tests/diagnostics-acceptance/verify.ps1` then exited `0`: five source-presence checks passed; Go JSON reported each of eight named unit/transport tests exactly once; Vitest JSON reported exactly two selected tests in exactly two selected files, both passing. The optional PostgreSQL suite was explicitly skipped because no isolated `15402` database URL was supplied. No browser test was run.

## Acceptance matrix

| Requirement | Evidence location | Status | Evidence / boundary |
| --- | --- | --- | --- |
| Actual download is JSON, not merely a click | `apps/web/platform/content-diagnostics.ts`; `apps/web/platform/content-diagnostics.test.ts` | Behavior test passed; browser not executed | Vitest ran the focused test, which parses the generated blob after the download helper triggers it. The source-presence checks only establish wiring. A browser download artifact was not obtained in this checkout. |
| Export matches selected authorized run | `server/internal/content/diagnostics/service.go:Export`; PostgreSQL integration test | Not executed | The service loads the requested run through scoped access. Requires isolated DB and browser/API evidence to mark pass. |
| Cross workspace/account denial and redaction | `contract_test.go`, `log_test.go`, `store_integration_test.go` | Go behavior test passed; DB integration not executed | Named Go tests exercised scope isolation and secret removal. No real account/browser result is claimed. |
| Stream pause, resume cursor, de-duplication | `packages/core/content/diagnostics/queries.ts`; focused React test | Vitest behavior test passed | Named Vitest coverage retained cursor, de-duplicated resumed events, and made pause stop reconnects. No browser websocket run occurred. |
| Network loss / reconnect notice | `queries.ts`; focused React test | Vitest behavior test passed | The named test verified reconnect from the stored cursor after a closed stream. No browser websocket run occurred. |
| Retention gap | `store_integration_test.go`; `queries.ts` | Not executed | DB test asserts an expired cursor produces a gap; UI renders a gap notice. Requires isolated PostgreSQL/API/browser proof. |
| Permission-scoped diagnostics export | `service.go:Export`, `store.go:GetRun` | Source presence only | Export appears to load its run through `GetRun(scope, id)` and scoped queries. Runtime access is not claimed until the isolated database/API check passes. |
| Real executor boundary | `service.go`, `simulator_test.go`, `transport_test.go` | Unit verified | Simulation is test-gated and the export declares real replay unavailable. This is not evidence that any real executor works. |

## Test design and remaining work

1. Create only an isolated, non-superuser PostgreSQL database on `15402` whose name starts with `loretide_diag_acceptance_`, then run the optional verifier command. Retain only its redacted exit/result summary.
2. Start only the reserved isolated API/Web pair (`18102`/`13102`) and perform browser evidence: choose a known synthetic run, preview/export it, parse the downloaded JSON, verify run ID, test forbidden workspace/account, pause/resume, reconnect, duplicate event, and retention-gap indication.
3. Keep every browser result `blocked` or `not executed` until it has its own screenshot, downloaded artifact assertion, and endpoint/operation evidence. Never mark the real executor as passed for this Issue.

## Defects observed during static review

No production code was changed by this Issue. The following requires a separate repair Issue if accepted as a defect: a complete diagnostic export downloads the `Export` object as JSON but the current focused browser-side test only asserts JSON readability, not that a UI-selected run ID equals the downloaded bundle's run ID. This task records the gap and does not modify production code or existing tests.

## Changed-file map and rollback

| File | Purpose |
| --- | --- |
| `tests/diagnostics-acceptance/verify.ps1` | Repeatable source-presence, Go/Vitest behavior verifier with JSON test-count gates and an optional isolated PostgreSQL identity gate. |
| `records/issue-2-diagnostics-acceptance.md` | This matrix, status boundaries, reproduction commands, evidence plan, and rollback record. |

Rollback is limited to deleting these two Issue-owned files from this branch. No existing code, environment, database, or evidence was altered.
