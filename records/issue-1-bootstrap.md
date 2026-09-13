# Issue #1 bootstrap record

## Scope

- Issue: https://github.com/899ms/loretide/issues/1
- Executor: Terra-DEV01
- Branch: `task/issue-1-windows-bootstrap`
- Baseline: `bb3a4b4616bf48a52e075bbe5e23e38112bbf6aa` on `app-main`

## Implemented artifacts

| Artifact | Purpose |
| --- | --- |
| `scripts/bootstrap-local-windows.ps1` | Isolated native Windows bootstrap/start/stop/status entry point. |
| `scripts/bootstrap-local-windows.test.ps1` | Fake-boundary behavior tests for lifecycle and safety contracts. |
| `docs/development/windows-bootstrap.md` | Operator prerequisites, commands, safety boundary, and evidence guidance. |

## Safety contract

The bootstrap writes only to its selected output directory, creates fresh random secrets and a random private verification code, uses a dedicated non-superuser role/database, keeps execution policy disabled, and retains state/logs after failure or stop. It neither reads the existing `data/windows` configuration nor operates the shared development instance. Before migration/build/start, a read-only SQL probe verifies the actual application role, database, database owner, and five forbidden privilege attributes; existing roles are never automatically altered. Stop validates both stored root identities and all Web descendants before acting, then revalidates each target immediately before requesting its stop. PID reuse fails closed, while operating-system-level races after the final identity read remain possible. Status is read-only.

## Verification record

Commands actually run before the review revision:

```powershell
pnpm install --frozen-lockfile
pwsh -NoProfile -File scripts/bootstrap-local-windows.test.ps1
pwsh -NoProfile -File scripts/bootstrap-local-windows.ps1 -Action bootstrap -RuntimePath F:\loretide-runtime -OutputPath C:\loretide-bootstrap-issue-1b -WebPort 13101 -ApiPort 18101 -PostgresPort 15401 -Database loretide_issue1_dev -Role loretide_issue1_app
```

- Dependency installation completed with pnpm 10.28.2 and did not change `pnpm-lock.yaml`.
- The pre-review script initialized an isolated PostgreSQL 17 cluster, applied migrations, started the API on 18101, and `/health` returned HTTP 200.
- Its first Web run incorrectly used the repository-root Next.js 15 entry. The page compiled and then returned HTTP 500 for a missing `.next/required-server-files.json`. This is failed evidence, not browser acceptance.
- The reviewed implementation now selects `apps/web/node_modules/next` 16.3.4 with `dev --webpack` and the correct `REMOTE_API_URL`. That revised live path has **not** been started or browser-verified because the local execution policy rejected restart/retest.

The current no-real-process test command is:

```powershell
pwsh -NoProfile -File scripts/bootstrap-local-windows.test.ps1
```

It passed 19 behavior cases for PID reuse rejection, all-root preflight and per-target stop revalidation, read-only status, caller environment restoration, repository-scoped tool detection from an external working directory, caller-directory restoration on successful and failed tool probes, healthy repeated start, partial Web failure, path quoting/Web upstream, stale config rejection, PostgreSQL identity mismatch, unattributed Web children, untracked listener rejection, application-role privilege/owner/query-failure rejection, and enforced tool versions. The external-directory cases exercise `Invoke-BootstrapMain` and the real `Get-ToolVersions` path with only the individual command boundary injected; they do not replace the entire version object with the `ToolVersions` hook. The privilege, owner, and query-failure cases prove no migration, build, or process launch occurs after a failed identity check. These tests do not start or stop real services.

The pnpm regression was also checked against the actual local command resolver without starting the instance: calling `Get-ToolVersions` from `C:\Users\13900` returned the repository-pinned pnpm `10.28.2` and left the caller working directory unchanged. Before the fix, the same directory resolved pnpm `10.33.0` while the checkout resolved `10.28.2`; the version probe had inherited the caller's directory instead of entering the repository.

## Outstanding evidence and local artifacts

Browser login and diagnostics acceptance remain unverified. Tool policy also rejected exact cleanup, isolated database-password rotation, and restart commands. No alternative tool or window was used. Four artifacts created by the pre-review PID/password-path bug remain untracked at the repository root and are excluded from the PR: `41224`, `60468`, `7094867fae1811578ae9b070b7283e84a645c89a2a0a95252c67cbaea5a9c364`, and `8a025e75f47b7d387ca9f95d7897c1eaffab0b7d6bf2e0eef58258fbc7c0cbed`. Their contents are not recorded here.

The current process state is intentionally not claimed. Once the execution-policy block is resolved, verify exact process ownership before any stop/restart, rotate only the isolated test role password, then run synthetic login and diagnostics browser acceptance. Rollback uses `-Action stop` only after the revised identity record exists; PostgreSQL data is retained by default.
