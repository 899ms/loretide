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
| `scripts/bootstrap-local-windows.test.ps1` | Static contract checks for safety and required bootstrap behavior. |
| `docs/development/windows-bootstrap.md` | Operator prerequisites, commands, safety boundary, and evidence guidance. |

## Safety contract

The bootstrap writes only to its selected output directory, creates fresh random secrets, uses a dedicated non-superuser role/database, keeps execution policy disabled, and retains state/logs after failure or stop. It neither reads the existing `data/windows` configuration nor operates the shared development instance.

## Verification record

Pending runtime execution in this isolated checkout. The implementation test is intended to run with:

```powershell
pwsh -NoProfile -File scripts/bootstrap-local-windows.test.ps1
```

Browser acceptance and synthetic login require the generated isolated database and are recorded separately after execution. Rollback is `-Action stop` for the selected output path; data is retained by default.
