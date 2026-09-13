# Loretide diagnostics CI (content contracts workflow)

This document describes the `Loretide content contracts` workflow
(`.github/workflows/loretide-content.yml`): what it verifies, how the
diagnostics database is provisioned with least privilege, and how to reproduce
its checks locally.

The workflow runs on `push` and `pull_request` targeting `app-main`. It is a
regression gate for the Loretide content-diagnostics slice; it is not a full
product acceptance suite, and it does not fix unknown failures. Real agent
execution stays disabled — the workflow only verifies the execution-policy
gates, it never runs a real executor.

## What runs

All steps live in a single `boundaries` job on `ubuntu-latest`.

| Check | Command | Purpose |
| --- | --- | --- |
| Content module boundaries (unit) | `node --test scripts/check-content-boundaries.test.mjs` | Boundary script's own tests |
| Content module boundaries (scan) | `node scripts/check-content-boundaries.mjs` | Enforces import boundaries |
| Diagnostics contract | `pnpm --filter @multica/core exec vitest run content/diagnostics/contract.test.ts` | Schema/contract parsing (non-UI, node) |
| Locale parity | `pnpm --filter @multica/views exec vitest run locales/parity.test.ts` | Translation key parity (non-UI, node) |
| Web typecheck | `pnpm --filter @multica/web typecheck` | TypeScript strict checks |
| Diagnostics DB integration + execution gates | `go test -race ./internal/content/diagnostics` and targeted `go test` runs | Postgres integration, execution-policy gates, migration invariants |
| API build | `go build ./cmd/server` | Server compiles |

### UI unit tests are not run in CI (Loretide policy)

Per Loretide policy, **UI unit tests are not written or run — neither in CI nor
locally** — and are not acceptance evidence; UI changes are accepted through
manual verification by the user (see `docs/development/README.md`). The policy
classifies tests by behavior, not by file extension: a `.test.ts(x)` that drives
React hooks or the DOM is a UI test.

The following two suites were previously run here and have been **removed from
CI execution**. Their source files remain in the repository (they are not
deleted), but they are not run — not in CI and not locally as part of
verification:

| Suite | Why it is a UI test |
| --- | --- |
| `packages/core content/diagnostics/queries.test.tsx` | jsdom + `renderHook`; exercises the `useDiagnosticStream` React hook |
| `apps/web platform/content-diagnostics.test.ts` | jsdom; drives `downloadDiagnosticBundle` through a DOM anchor and `URL.createObjectURL` |

Their absence from a CI run is a deliberate policy decision, **not a pass** — no
automated coverage runs for these behaviors, in CI or locally. `contract.test.ts`
(pure schema parsing, node environment) and `locales/parity.test.ts`
(`// @vitest-environment node`) are non-UI and remain in CI, as do typecheck, the
Go `-race` diagnostics, execution-policy gates, migration invariants,
module-boundary checks, the least-privilege database provisioning, and the build.

## Database identity and least privilege

The Go diagnostics integration test
(`server/internal/content/diagnostics/store_integration_test.go`) reads its
connection string from `LORETIDE_DIAG_TEST_DATABASE_URL`. It creates a
temporary schema, applies migrations `468`–`473`, and drops the schema on
cleanup. Nothing in that test needs cluster-wide privileges.

The workflow therefore provisions, on the runner's **native** PostgreSQL (no
Docker, never a development business database):

1. A dedicated database `loretide_diag_ci`.
2. A dedicated login role `loretide_diag` created
   `NOSUPERUSER NOCREATEDB NOCREATEROLE`, owning only that database.

The admin (`postgres`) superuser is used **only** to provision these. The
integration test connects **as `loretide_diag`**. Owning a single database lets
the role create and drop its own temporary schema and objects without any
cluster-wide power; it still cannot create other databases or roles and is not
a superuser.

### Fail-closed identity check

Before any test runs, a read-only step connects as the diagnostics role and
asserts its attributes:

```sql
SELECT rolsuper,
       rolcreatedb,
       rolcreaterole,
       pg_get_userbyid((SELECT datdba FROM pg_database WHERE datname = current_database())) = current_user
FROM pg_roles WHERE rolname = current_user;
```

The step requires exactly `false false false true`: the role is not a
superuser, cannot create databases or roles, and owns the database it is
connected to (which is what lets it manage its own temporary schema). Any
deviation — most importantly a role that is a superuser — stops the job with an
error before the integration suite executes. Test failures are never swallowed: every scripted
step uses `set -euo pipefail`, and the Go steps exit non-zero on failure.

### Secret handling

The role password is generated per run with `openssl rand` and is never
echoed. Both the raw password and the full connection string are registered
with `::add-mask::` so the runner redacts them even if a later step prints the
environment. The identity check prints only the four boolean/relationship
attributes above — never the user, host, database name, or connection string.

## Reproducing locally

You need a local PostgreSQL you are willing to write to (do **not** point this
at a development business database). Provision an equivalent least-privilege
role and database, then run the same commands:

```bash
# As a Postgres admin, provision (choose your own password):
psql -v ON_ERROR_STOP=1 <<'SQL'
DROP DATABASE IF EXISTS loretide_diag_ci;
DROP ROLE IF EXISTS loretide_diag;
CREATE ROLE loretide_diag LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD 'choose-a-password';
CREATE DATABASE loretide_diag_ci OWNER loretide_diag;
SQL

export LORETIDE_DIAG_TEST_DATABASE_URL='postgres://loretide_diag:choose-a-password@127.0.0.1:5432/loretide_diag_ci?sslmode=disable'

# Non-UI checks (as CI runs them):
pnpm --filter @multica/core exec vitest run content/diagnostics/contract.test.ts
pnpm --filter @multica/views exec vitest run locales/parity.test.ts

# Backend integration + gates:
(cd server && go test -race ./internal/content/diagnostics -count=1)
```

UI unit tests (`queries.test.tsx`, `content-diagnostics.test.ts`) are not part of
this list: per Loretide policy they are not run, in CI or locally. UI behavior is
verified manually by the user (see `docs/development/README.md`).

Local success does not substitute for a passing GitHub Actions run; the CI run
against the PR head SHA is the acceptance evidence for the non-UI checks.
