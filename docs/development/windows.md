# Windows development readiness

LT-002 inspection, 2026-09-13. Source: [source-baseline.md](source-baseline.md). Inventory completion does not mean the application runs.

| Tool | Observed | Requirement / action |
| --- | --- | --- |
| Node | v24.14.0 | Meets package.json >=22; build untested |
| Go | 1.26.5 windows/amd64 | go.mod requires 1.26.6; resolve a project toolchain before compiling |
| Go toolchain policy | GOTOOLCHAIN=auto | Download and execution of required version not verified |
| pnpm | 10.33.0 from parent workspace | Project pins 10.28.2; resolve pinned version in app |
| Corepack | 0.34.6 | Available; avoid replacing global pnpm |
| PostgreSQL | psql/pg_isready absent from PATH; no standard Program Files installation or service found | Portable installations elsewhere are not ruled out; no working database verified |
| Docker / Podman | Absent from PATH and standard installation directories | No working container engine verified |
| WSL | Queries fail with Wsl/0x80070422; WslService stopped and disabled | Not currently a usable database host |
| Make | Absent from PATH | Upstream make commands are not a verified Windows startup method |
| Git Bash | Explicit Program Files/Git/bin/bash.exe reports 5.2.37 | Default bash resolves to a WindowsApps shim; full script compatibility unverified |

No listeners were returned for 5432, 6379, 3000 or 8080 at inspection time. Recheck ports before startup.

## Setup sequence (not executed)

1. Resolve Go 1.26.6 in an isolated toolchain. Run `go version` inside server and record the resolved version before compiling. Keep the global installation unchanged.
2. From app, run `corepack pnpm --version`, verify 10.28.2, then use `corepack pnpm install --frozen-lockfile`. Version resolution may require downloading the package manager.
3. LT-003 selects a dedicated database. First evaluate project-local native PostgreSQL given the unavailable container environment. Verify distribution source, version, extension compatibility and lifecycle commands. Upstream docker-compose.yml and CI use pgvector/pgvector:pg17; plain Windows PostgreSQL must not be presumed equivalent. Redis is absent from the default Compose file; optional server Redis paths and broader CI dependencies need separate evaluation.
4. If native compatibility fails, document the exact blocker before deciding on container setup. No WSL enablement or service installation occurred in LT-002.
5. Define independent database/data paths and ports before startup; add PowerShell lifecycle scripts under LT-008. Browser testing does not require Electron.

No dependencies, database, migrations, API, Web, daemon or model processes were started. No application tests ran. LT-002 establishes tool availability and actionable gaps; LT-003 through LT-008 must validate actual execution.
