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

Verifier self-test result: `pwsh -NoProfile -File tests/diagnostics-acceptance/verify.behavior.tests.ps1` exited `0`. It injects fake Go, Vitest, and PostgreSQL command runners—no real database, browser, subprocess cleanup, or service is used—and proves that missing/skip/fail Go tests, missing/failed Vitest cases, invalid database targets, mismatched database and current-user identities, and superuser identities are rejected. A fake `pnpm` default-runner entry writes a JSON report while emitting an ordinary log line; the verifier returns only the structured report. It also proves malformed URI errors do not echo a fake secret, encoded credential components are split before decoding, missing passwords are rejected, caller working directory and diagnostic/password environment variables are restored on successful and exceptional paths, and a fake secret from a command failure is not included in the thrown message.

Post-refactor non-database result: `pwsh -NoProfile -File tests/diagnostics-acceptance/verify.ps1` exited `0`; the same five source-presence checks, eight exact Go passes, and two exact Vitest passes completed. PostgreSQL remained explicitly skipped because no isolated URL was supplied; no service or browser was started.

## User-supplied manual UI observation (not agent UI acceptance)

The user manually observed the eight diagnostics panels on the separate `miranda-qu6a` instance at Web `13101`; API and database showed healthy, build showed `unknown`, Web heartbeat showed `unverified`, the sample count was zero, and the stream was paused. The user had not simulated a run or downloaded a bundle. This is user-provided UI evidence only: this Issue did not run a browser/UI test, computer-use check, API request, database query, or process inspection against `13101`/`18101`/`15401`, and it does not establish Issue #1 completion.

Read-only source attribution:

- `server/cmd/server/router.go` constructs diagnostics with `os.Getenv("LORETIDE_BUILD")`; `diagnostics.NewService` preserves an empty value because `safeToken("")` is valid. Therefore a blank launcher environment reaches the overview, and the UI's `build || "unknown"` fallback displays `unknown`. The appropriate owner is the isolated-environment launcher/configuration work in Issue #1 (set a safe build identifier); do not change production behavior under this acceptance Issue.
- `packages/core/content/diagnostics/queries.ts` calls `POST /api/content-diagnostics/client` once on diagnostics mount and every 10 seconds, but catches and suppresses failures. `server/internal/handler/content_diagnostics.go` records Web healthy only after that authenticated, owner/admin-gated request succeeds. `service.go` otherwise emits `unverified` for absent heartbeats. The observation can therefore mean the view was observed before a successful beat, or that the POST failed/scope was rejected; current evidence cannot distinguish those cases. Treat it as a remaining Issue #1/API integration check, not as an actual Web-health pass.

Heartbeat request contract, read from source: `ApiClient.contentDiagnosticRequest("client", "", {heartbeat:true})` sends `POST /api/content-diagnostics/client` with `Content-Type: application/json`, body `{"heartbeat":true}`, the current `X-Workspace-Slug` when available, and `credentials: include`. The shared client reads the browser `multica_csrf` cookie and emits `X-CSRF-Token` when present; the server's cookie-auth path validates CSRF for POSTs. The route is inside authenticated `RequireWorkspaceMember`, then requires a human actor and owner/admin membership before `ContentDiagnosticClient` records `Heartbeat("web", "browser", ...)`. This static path is internally consistent; it cannot establish which runtime condition happened.

Minimum user-owned Network Todo (one observation, no credentials): with the diagnostics page open, inspect the request whose URL path is exactly `/api/content-diagnostics/client` (its origin may be the Web rewrite or configured API origin), method `POST`, request body `{"heartbeat":true}`. Record only status and response shape: expected `200` with `{"ok":true}`. Do not copy cookies, Authorization, CSRF headers, request IDs, or response bodies beyond that shape. A non-200 status is evidence for the launcher/API owner: `401` means authentication, `403` can be CSRF, machine-actor, or owner/admin denial, and `404` can be workspace resolution/membership. This Todo is not agent-run UI acceptance.

Non-UI preparation remaining after the launcher is available: use a fresh `loretide_diag_acceptance_*` non-superuser database on the reserved acceptance port; pass the verifier's protocol/identity gate; then record endpoint results, a downloaded JSON artifact parsed against its selected run ID, scoped denial, stream resume/de-duplication, and retention-gap results. UI observations and manual Todo steps remain user-owned and are not substituted by agent tests.

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
| `tests/diagnostics-acceptance/verify.behavior.tests.ps1` | Fake-runner self-test for the verifier's reject gates, state restoration, and failure-output redaction. |
| `records/issue-2-diagnostics-acceptance.md` | This matrix, status boundaries, reproduction commands, evidence plan, and rollback record. |

Rollback is limited to deleting these two Issue-owned files from this branch. No existing code, environment, database, or evidence was altered.
