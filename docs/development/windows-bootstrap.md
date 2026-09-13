# Isolated native Windows bootstrap

`scripts/bootstrap-local-windows.ps1` creates a separate local Loretide instance without reading the existing `data/windows` configuration, its development database, or model-client credentials. It does not use Docker, VPS access, firewall changes, or automatic restart.

## Prerequisites

- Node, pnpm and Go on `PATH`.
- The bootstrap runs `pnpm install --frozen-lockfile` when the checkout has no installed frontend dependencies.
- A native PostgreSQL runtime at an ASCII-only path containing `pgsql/bin/initdb.exe`, `pg_ctl.exe` and `psql.exe`.
- Three free loopback ports. The defaults are Web `13101`, API `18101`, PostgreSQL `15401`.

Choose an ASCII output path outside the repository if the checkout path is not ASCII. The output directory contains the private generated database, random secrets, PIDs and logs; do not commit it.

```powershell
./scripts/bootstrap-local-windows.ps1 -Action bootstrap `
  -RuntimePath C:\loretide-runtime `
  -OutputPath C:\loretide-bootstrap-dev
```

The first run initializes a native PostgreSQL cluster, creates a dedicated non-superuser application role/database, generates new secrets, runs migrations, builds the API, and starts API/Web. Repeating the command reuses only that output directory and database; it neither replaces the existing development instance nor changes its ports.

The script deliberately sets `LORETIDE_EXECUTION_POLICY=disabled`. Do not change that setting in the generated state to obtain a real-agent run.

## Operation and evidence

```powershell
./scripts/bootstrap-local-windows.ps1 -Action status -OutputPath C:\loretide-bootstrap-dev
./scripts/bootstrap-local-windows.ps1 -Action stop -OutputPath C:\loretide-bootstrap-dev
./scripts/bootstrap-local-windows.ps1 -Action start -OutputPath C:\loretide-bootstrap-dev
```

`stop` ends only the API/Web processes created by this instance and retains PostgreSQL/data for investigation or restart. Logs remain under `<OutputPath>/logs`; they may contain operational detail and must be reviewed/redacted before sharing. A missing runtime, dependency, port collision or build/migration failure stops with an actionable error and leaves the generated logs in place.

For synthetic local login, use a synthetic address and the development verification code `888888`. It is configured only in the isolated child process environment, never in a repository file. Browser acceptance should include the isolated Web URL and its diagnostics route; do not use the existing development application or database as evidence.
