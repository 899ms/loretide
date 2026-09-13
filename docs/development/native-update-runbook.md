# Native env runbook after Grok Bot Update

Scope: recover `/home/box/loretide-dev` native supervised stack after **Update Grok Bot's Computer**. This is environment recovery, not business-feature acceptance. **No real Update was executed in the 2026-09-13 reliability session** (items marked 未模拟).

## Likely losses after Update

| Asset | Likely kept under `/home/box` | Likely lost / broken |
| --- | --- | --- |
| Project tree `loretide-dev/` | Usually kept | — |
| `data/native/pg` cluster | Usually kept | — |
| `data/native/env`, `pgpassword` | Usually kept (mode 600) | — |
| Toolchains under `data/native/toolchains` | Usually kept | — |
| Debian packages `postgresql-17`, `pgvector`, `supervisor` | — | May be missing after image refresh |
| Running `supervisord` / child PIDs | — | Always gone after Update/reboot of the computer |
| Tailscale authorization | Device may need re-approve | Restore fails closed without `100.x` IPv4 |
| Apt source layout | — | May need HTTPS mirror rewrite (restore handles) |

Unsimulated: exact Update package delta on this image (**未模拟**).

## Restore command(s)

```bash
cd /home/box/loretide-dev
bash scripts/native-restore.sh --repair
bash scripts/native-status.sh
# optional deeper read-only check
bash scripts/native-doctor.sh
```

Refusals baked into restore: `--reset` / `--wipe` / `--drop` / `--docker` / `--initdb` → exit 2. Never initdb, never drop DB, never git reset/clean, never Docker.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success; post-checks passed |
| 1 | Precondition failure (missing env/pg/toolchains/Tailscale/packages sources) |
| 2 | Usage / refused destructive option |
| 3 | Post-check failure (API/web/postgres) |
| 4 | Another restore holds `data/native/logs/restore.lock` |
| 5 | Programs failed to reach RUNNING |

Logs: `data/native/logs/restore-YYYYMMDDTHHMMSSZ.log` and `restore-last.txt`.

## Expected results

1. Missing OS packages reinstalled (only if absent).
2. `data/native/network.env` refreshed from `tailscale ip -4`.
3. API binary rebuilt; web deps via `pnpm --frozen-lockfile`.
4. If supervisord **down** → started (lock fd not inherited).
5. If supervisord **up** → `supervisorctl start` only for FATAL/STOPPED/EXITED/BACKOFF (no wipe, no duplicate listeners).
6. Post-checks: `curl /health`, diagnostics HTTP, `pg_isready`.
7. DB rows preserved (`content_diagnostic_run` count unchanged when cluster intact).

## Failure modes

| Symptom | Likely cause | Action |
| --- | --- | --- |
| Exit 1 Tailscale | Device unauthorized | Approve in Bot console; re-run |
| Exit 1 missing `data/native/env` or `pg/PG_VERSION` | Overlay/data loss | Restore off-host backup **before** any initdb temptation |
| Exit 1 toolchain missing | Tree incomplete | Restore `/home/box/loretide-dev` from backup |
| Exit 3 diagnostics | Web still compiling / bind IP wrong | Wait; confirm `network.env` IP; `native-status.sh` |
| Exit 4 lock | Concurrent restore | Wait or inspect `data/native/logs/restore.lock` holders |
| Exit 5 | Program FATAL | `native-doctor.sh`; inspect `data/native/logs/*-supervised.log` (redact secrets) |
| Duplicate listeners | Manual + supervised mix | Stop orphans; use only supervisorctl |

## Success criteria

- `supervisorctl status` → postgres/api/web **RUNNING**
- Exactly one listener each on `15332`, `18000`, `13000`
- `curl -f http://127.0.0.1:18000/health`
- Diagnostics URL on Tailscale IP returns HTTP 200
- `pg_isready -h 127.0.0.1 -p 15332`
- Diagnostic row count/marker unchanged vs pre-Update snapshot
- No business code changes required for env recovery

## Unsimulated items (未模拟)

- Actual **Update Grok Bot's Computer** click-through on this host
- TRUE machine reboot (no reboot binary; PID1=tini) — see `data/native/acceptance/part2-reboot-result.md`
- Loss of entire `/home/box` overlay (would require off-host restore, out of band)

## Related

- `docs/development/native-linux.md`
- `docs/development/native-env-reliability-2026-09-13.md`
- `docs/development/native-env-acceptance-2026-09-13.md`
