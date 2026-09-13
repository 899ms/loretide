# Native Windows development

Current development runs on this Windows computer, without Docker. Use the existing checkout; do not restore an old upstream tree over uncommitted changes.

## Start and stop

Run from the application checkout in PowerShell:

```powershell
./scripts/local-windows.ps1 start
./scripts/local-windows.ps1 status
./scripts/local-windows.ps1 stop
./scripts/local-windows.ps1 build
```

`stop` requests API/Web shutdown and leaves PostgreSQL running. Stop before rebuilding the API executable, then start again. `start` starts PostgreSQL if needed and avoids launching a second project supervisor.

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

Real AI execution remains disabled by `LORETIDE_EXECUTION_POLICY=disabled`. The daemon and real executor have not been accepted. Imported workspace-deletion concurrency changes remain pending review; compilation and a working page do not establish their correctness.

Never run handler fixture tests against `loretide_dev`. Use a dedicated test database and role, verify the actual connection, then run tests. Integration tests that skip without their database environment are not integration acceptance.

The local isolated database is now provisioned. Its private configuration is `data/windows/test-db.json`. Run `./scripts/test-local-windows.ps1 diagnostics` or `./scripts/test-local-windows.ps1 handler`. The runner checks the database/role identity and rejects superusers, serializes its test runs with a file lock, and writes JSON test evidence under `data/windows/`. It does not provision or clear databases. External-service skips remain separate from passing cases.

## Preservation and rollback

The restored development database contained 2 workspaces and 3 diagnostic runs. The former VPS and its data remain intact; its uploads directory contained no files at migration inspection. Local pre-import file copies and the remote dump are retained under the private migration directory.

To stop using this environment, run `stop`; do not delete either database. Review individual source differences against `data/local-migration/local-before/` before any rollback. Database restore is a separate, explicit operation and must target a new database first. Never commit private configuration, dumps, raw logs or runtime files.
