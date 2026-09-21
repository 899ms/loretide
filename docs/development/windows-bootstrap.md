# Isolated native Windows bootstrap

`scripts/bootstrap-local-windows.ps1` creates a separate local Loretide instance without reading the existing `data/windows` configuration, its development database, or model-client credentials. It does not use Docker, VPS access, firewall changes, or automatic restart.

## Prerequisites and enforced versions

- Node 22 or newer, Go 1.26.6 or newer, PostgreSQL 17 or newer, repository pnpm 10.28.2, and workspace Next.js 16.3.4 or newer. Bootstrap fails before startup when a version is missing or too old.
- Toolchain detection runs from the checkout (Go from `server`) and restores the caller's working directory even when a probe fails. You may invoke the script by absolute path from another directory; the pnpm check still resolves the repository's `packageManager` pin rather than an unrelated parent-directory installation.
- The bootstrap runs `pnpm install --frozen-lockfile` when the checkout has no installed frontend dependencies.
- A native PostgreSQL runtime at an ASCII-only path containing `pgsql/bin/initdb.exe`, `pg_ctl.exe` and `psql.exe`.
- Three free loopback ports. The defaults are Web `13101`, API `18101`, PostgreSQL `15401`.

Choose an ASCII output path outside the repository if the checkout path is not ASCII. The output directory contains the private generated database, random secrets, PIDs and logs; do not commit it.

```powershell
./scripts/bootstrap-local-windows.ps1 -Action bootstrap `
  -RuntimePath C:\loretide-runtime `
  -OutputPath C:\loretide-bootstrap-dev
```

The same command may be launched outside the checkout by using the script's absolute path. There is no need to change the global pnpm version:

```powershell
& C:\path\to\loretide\scripts\bootstrap-local-windows.ps1 -Action start `
  -OutputPath C:\loretide-bootstrap-dev
```

The first run initializes a native PostgreSQL cluster, creates a dedicated non-superuser application role/database, generates new secrets and a random private development verification code, runs migrations, builds the API, and starts API/Web. Repeating the command validates and reuses only that output directory and database. A healthy owned instance is returned as already running without rebuilding or launching duplicates.

The configuration binds its repository, output path, runtime path, PostgreSQL data path, ports, database and role. PostgreSQL is queried after startup to prove that its actual data directory and port match. Before every migration, build, or application start, the script connects with the generated application-role credentials and performs a read-only identity query. It requires the selected role and database, requires that role to own the database, and rejects `SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, or `BYPASSRLS`. Existing roles are never repaired with `ALTER ROLE`; unexpected privilege or ownership fails closed. A conflicting explicit argument, stale configuration, or failed identity query also fails closed.

The script deliberately sets `LORETIDE_EXECUTION_POLICY=disabled`. Do not change that setting in the generated state to obtain a real-agent run.

## Operation and evidence

```powershell
./scripts/bootstrap-local-windows.ps1 -Action status -OutputPath C:\loretide-bootstrap-dev
./scripts/bootstrap-local-windows.ps1 -Action stop -OutputPath C:\loretide-bootstrap-dev
./scripts/bootstrap-local-windows.ps1 -Action start -OutputPath C:\loretide-bootstrap-dev
```

`status` is read-only: it verifies recorded API/Web ownership and reports PostgreSQL state without starting a stopped service. `stop` validates both tracked roots before acting, then immediately revalidates each planned child/root identity before its stop request. It ends only API/Web processes whose PID, creation time, executable and complete command line still match the stored instance record. Web child processes are also checked before any stop occurs; a reused PID or unattributed child makes the operation fail closed. This narrows identified PID-reuse windows but cannot eliminate every operating-system scheduling race between the final identity read and the stop syscall. PostgreSQL and its data remain available for investigation or restart.

At each API/Web start, the launcher sets `LORETIDE_BUILD` to the checkout's full Git object ID. It appends `-dirty` when `git status --porcelain=v1 --untracked-files=normal` reports tracked or untracked changes. If either Git query fails or HEAD is not a full object ID, startup fails before service boundaries instead of reporting a fabricated or `unknown` build. An already-running process keeps the environment from its original launch; this identifier update takes effect on its next normal start and is not a reason by itself to restart a service under active inspection.

Logs remain under `<OutputPath>/logs`; they may contain operational detail and must be reviewed/redacted before sharing. Database passwords are passed in process-local environment variables, and role-password SQL is supplied through a short-lived private file rather than command-line arguments. A missing runtime, dependency, port collision or build/migration failure stops with an actionable error and leaves the non-secret diagnostic logs in place.

For synthetic local login, use a synthetic address and the random development verification code in the private `<OutputPath>/state/instance.json`. Do not publish that file or code. The Web process receives `REMOTE_API_URL=http://127.0.0.1:<ApiPort>` for same-origin rewrites; it does not substitute `NEXT_PUBLIC_API_URL` for that upstream. Browser acceptance must include the isolated Web URL and diagnostics route and must not use the existing development application or database as evidence.

If API startup succeeds but Web startup or health fails, the script records the real partial state and exits with an error. Inspect the retained process records and logs; it never prints a full-start success message for a partial start.
