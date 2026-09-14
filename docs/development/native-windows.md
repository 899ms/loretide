# Native Windows development

Current development runs on this Windows computer, without Docker. Use the existing checkout; do not restore an old upstream tree over uncommitted changes.

## Start and stop

Run from the application checkout in PowerShell:

```powershell
./scripts/local-windows.ps1 start
./scripts/local-windows.ps1 status
./scripts/local-windows.ps1 status -Json
./scripts/local-windows.ps1 stop
./scripts/local-windows.ps1 stop -TimeoutSec 60
./scripts/local-windows.ps1 build
```

PowerShell 7 or newer is required; the script declares `#Requires -Version 7.0` and refuses to run under Windows PowerShell 5.

`start` checks its preconditions before launching anything: private configuration present, `api.exe` built, PostgreSQL running (started if needed), and none of the three ports held by a process outside this checkout. A failed check exits non-zero and prints the component, the log to read and the command to run next, instead of leaving a supervisor crash-looping into a log file. A `supervisor.pid` left by a dead process is cleared rather than believed. Starting an instance that is already running reports it and exits 0 without opening a second listener.

`status` reports postgres, supervisor, api and web on one line each: state, pid, whether the process belongs to this checkout, and — for a component that is actually running — its health probe. A process on one of our ports that is not one of our pids is listed as a conflict, never as healthy, because a health probe against another checkout's server returns 200 and proves nothing. Nothing is probed over HTTP unless it is running and owned, so `status` on a stopped instance returns immediately instead of waiting out timeouts. `-Json` prints the same facts as one object for scripted checks; its shape is fixed by `specs/003-lt008-windows-instance-lifecycle/contracts/status-json.md`.

| `status` exit code | Meaning |
|---|---|
| 0 | all four components running, owned, no conflicts |
| 1 | a component is not running, or a port conflict exists |
| 2 | the script cannot check (for example `pg_ctl.exe` is missing) |

`stop` requests API/Web shutdown and leaves PostgreSQL running, then waits for api, web and supervisor to actually exit (`-TimeoutSec`, default 30) and reports each one. It never force-kills: on timeout it lists the components still alive and exits non-zero so you can decide. Waiting is what makes the next `build` safe — a rebuild fails while the old `api.exe` is still held. Stopping an instance that is not running exits 0.

No command prints the contents of `data/windows/secrets.json`, the database password or the JWT secret; failures name the file path only.

Browser: http://localhost:13000/loretide-dev-check/diagnostics

API health: http://127.0.0.1:18000/health

The existing development account uses the application's development email fallback: its verification code appears in the private local API log when no email provider is configured. Do not publish codes or logs containing them. This does not use Codex, Claude or Gemini authentication.

## Paths and runtime

| Item | Location |
|---|---|
| Application | `F:\GJ\内容创作工作台\app` |
| PostgreSQL binaries | `F:\loretide-runtime\pgsql` |
| PostgreSQL data | `F:\loretide-runtime\pg` |
| PostgreSQL log | `F:\loretide-runtime\postgres.log` |
| Private configuration, executable, PIDs and API/Web logs | `app/data/windows/` |
| Source snapshots, previous local files and database dump | `app/data/local-migration/` |

PostgreSQL uses an ASCII runtime path because initialization failed with the binary under the Chinese checkout path. PostgreSQL binds to loopback port 15332; Web binds to loopback port 13000. Do not expose the development API to the Internet.

Installed baseline: Node 24.14.0, repository pnpm 10.28.2, PostgreSQL 17.11. Go 1.26.5 is installed; `GOTOOLCHAIN=auto` satisfies the project's Go 1.26.6 requirement. This launcher uses the existing installed dependencies and private configuration; it is not a clean-machine installer.

## Supervision and limits

The local Node supervisor restarts API/Web after process exit. PostgreSQL is started by the launcher and is not monitored by that supervisor. Windows login/reboot auto-start is not configured or verified. After reboot run `start` manually. Do not claim full unattended recovery.

### One instance per machine

This machine supports one running instance. The ports (PostgreSQL 15332, API 18000, Web 13000) are fixed in the launcher and the supervisor, so a second checkout cannot run at the same time. That is a deliberate limit, not an oversight: a second checkout's `start` stops at the port check, names the pid holding the port, and changes nothing. Stop the first instance, or work in the checkout that owns it. Per-worktree port and database isolation — what `make up` provides on Linux — is a separate task and is not implemented here.

Real AI execution remains disabled by `LORETIDE_EXECUTION_POLICY=disabled`. The daemon and real executor have not been accepted. Imported workspace-deletion concurrency changes remain pending review; compilation and a working page do not establish their correctness.

Never run handler fixture tests against `loretide_dev`. Use a dedicated test database and role, verify the actual connection, then run tests. Integration tests that skip without their database environment are not integration acceptance.

The local isolated database is now provisioned. Its private configuration is `data/windows/test-db.json`. Run `./scripts/test-local-windows.ps1 diagnostics` or `./scripts/test-local-windows.ps1 handler`. The runner checks the database/role identity and rejects superusers, serializes its test runs with a file lock, and writes JSON test evidence under `data/windows/`. It does not provision or clear databases. External-service skips remain separate from passing cases.

## Preservation and rollback

The restored development database contained 2 workspaces and 3 diagnostic runs. The former VPS and its data remain intact; its uploads directory contained no files at migration inspection. Local pre-import file copies and the remote dump are retained under the private migration directory.

To stop using this environment, run `stop`; do not delete either database. Review individual source differences against `data/local-migration/local-before/` before any rollback. Database restore is a separate, explicit operation and must target a new database first. Never commit private configuration, dumps, raw logs or runtime files.

## Update log

| Date | Change | Verification evidence |
|---|---|---|
| 2026-09-14 | `status` now reports state / pid / ownership / build / health per component and supports `-Json`; `start` checks private configuration, `api.exe` and port ownership before launching and clears stale pid files; `stop` waits for processes to exit and reports each, without killing. Documented the one-instance-per-machine limit. | `pwsh -File scripts/local-windows.test.ps1` - 61 checks, exit 0, run on Linux pwsh 7.4.6 with the Windows calls stubbed. The real Windows end-to-end run (quickstart.md) was NOT executed by the change author and is recorded as not run. |
| 2026-09-14 | `start` still hung when it had to launch PostgreSQL: the first fix replaced a captured pipeline with `Start-Process -Wait`, but on Windows `-Wait` waits for the whole process tree and postgres.exe is pg_ctl's child, so it waited for the database to shut down by another route. It now waits on the pg_ctl process alone with a 60s bound, and proves the server is ready by polling `pg_ctl status` for up to 30s rather than assuming a zero exit code means ready. | `pwsh -File scripts/local-windows.test.ps1` - 92 checks, exit 0. The wrapper is now reachable from a test: shadowing the `Start-Process` cmdlet lets the suite assert that `-Wait` is not passed, that the wait is bounded, and that a non-returning pg_ctl is reported as a failure. Real-Windows reverification pending. |
| 2026-09-14 | Three defects found by the first real-Windows run, fixed: `pg_ctl start` no longer runs through a captured pipeline (the postgres.exe it spawns inherited the handles, so `start` never returned); `start` waits up to 10s to confirm the supervisor survived launch and fails with the error-log path instead of reporting success for a dead instance; a listener owned by a child of a component (a `next dev` worker holding 13000) is no longer reported as a port conflict. | `pwsh -File scripts/local-windows.test.ps1` - 76 checks, exit 0. Real-Windows run by the main task on pwsh 7.6.6: start/restart without new listeners, `status -Json` api and web health 200, `stop -TimeoutSec 60` reporting three exits with PostgreSQL still running. The pipeline deadlock cannot be reproduced from a stub; its fix is verified on Windows only, and the suite asserts the call shape. |
