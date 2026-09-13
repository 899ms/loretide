# Loretide native env reliability — 2026-09-13

Operator timezone notes: wall timestamps below are UTC unless labeled CST (UTC+8).

## 1. Scope

Independent **environment** reliability for `/home/box/loretide-dev` on the Grok Bot shared Linux computer (Tailscale `100.109.104.61`). Not brand/account/content business acceptance. **No Docker. No DB wipe. No business module / apps/ / server internal business package edits.**

Touched only: `scripts/native-*.sh|py|conf`, `docs/development/native-*.md`, `data/native/acceptance/` artifacts, and restore/doctor logs under `data/native/logs/`.

## 2. Topology (current)

| Layer | Detail |
| --- | --- |
| PID 1 | `tini` (no systemd; no in-guest reboot binary) |
| Process manager | `supervisord` as `box`, `scripts/native-supervisor.conf` |
| PostgreSQL | `data/native/pg`, `127.0.0.1:15332` |
| API | `data/native/bin/api` on `*:18000` |
| Web | Next `next dev` on Tailscale `:13000` |
| Recovery | `bash scripts/native-restore.sh --repair` |
| Status / doctor | `native-status.sh`, `native-doctor.sh` |

## 3. PART1 baseline

Artifact: `data/native/acceptance/part1-baseline-20260913T083629Z.txt`

- All three RUNNING; ports 15332/18000/13000 single listeners
- `diagnostic_runs=3`

## 4. PART2 reboot / cold-cycle

See `data/native/acceptance/part2-reboot-result.md`.

| Exercise | Status |
| --- | --- |
| TRUE machine reboot | **未演练** (no reboot binary; PID1=tini; platform Update/Reset only) |
| Supervisor cold-cycle simulation | **Passed** — stop all → kill supervisord → `native-restore.sh --repair` |

Cold-cycle evidence (`part2-supervisor-cold-cycle.txt`):

- Restore wall **57708 ms**
- diagnostics HTTP 200; listeners 1/1/1
- `diag_count=3`, marker `1625628ce1b3d0033892832f1789034e` unchanged

**Auto vs manual:** `autorestart=true` recovers child crashes while supervisord lives; after supervisord death or Update, **manual** `native-restore.sh --repair` is required.

## 5. PART3 restore reliability

### Script improvements

`scripts/native-restore.sh`:

- When supervisord already running → `supervisorctl start` for FATAL/STOPPED/EXITED/BACKOFF only (no wipe)
- Clearer exit codes 0/1/2/3/4/5
- Logs to `data/native/logs/restore-*.log` (+ `restore-last.txt`, `restore-latest.log`)
- Lock at `data/native/logs/restore.lock`; **close fd before spawning supervisord** (avoids inherited flock)
- Post-check diagnostics timeout extended to 120s for Next cold compile

`scripts/native-status.sh`: ports/listener counts, data presence, overall healthy/degraded exit codes.

### Drills

| Drill | Result |
| --- | --- |
| `native-status.sh` healthy | overall=healthy, listeners 1/1/1 |
| `--repair` ×2 while healthy | exit 0; **same PIDs**; no duplicate listeners; data preserved |
| stop only `web` → `--repair` | web restarted (new pid); **api/postgres PIDs unchanged**; diag 3 + same marker; diagnostics 200 |

Note: an early harness used `supervisorctl status && repair`; status exits 3 when any STOPPED, which skipped repair. Restore itself correctly starts STOPPED programs (verified).

## 6. PART4 Update runbook

Documented in `docs/development/native-update-runbook.md`. **No real Update executed (未模拟).**

## 7. PART5 observability

`scripts/native-doctor.sh` — read-only aggregator: supervisor, health, web diagnostics HTTP, pg_isready, ports, data presence, disk/mem, RSS snapshot, recent error log tails with secret redaction. Never prints `data/native/env` / `pgpassword` values.

## 8. PART6 cold-start & resources

Artifacts:

- `data/native/acceptance/part6-cold-start-timings.txt`
- `part6-resources-before-cold.txt` / `part6-resources-after-cold.txt`

### Clean supervisorctl cold-start (stop all → start postgres → api → web)

| Probe | Time |
| --- | --- |
| postgres `pg_isready` from start | **3187 ms** |
| API `/health` from T0 | **8378 ms** (~5.2 s after postgres ready) |
| Web diagnostics HTTP 200 from web start | **56709 ms** |
| Total to all ready | **65091 ms** |

No stress tests.

### Stable resource snapshot (after cold-cycle restore)

| Process | RSS (KiB) approx | %CPU |
| --- | --- | --- |
| postgres | ~28608 | ~0.0 |
| api | ~37584 | ~0.1 |
| web (node) | ~75484 | ~0.4 |
| Disk root | 126G total, ~29G used (~25%) |
| `data/native` | ~928M |
| Mem | 15Gi total; ~5.0Gi available |

## 9. Success criteria (final)

- postgres / api / web **RUNNING**
- diagnostics HTTP **200**
- `diagnostic_runs=3` unchanged; marker unchanged
- No business code changes

## 10. Unfinished / 未演练 items

1. TRUE platform reboot / Reset (**未演练**)
2. Real **Update Grok Bot's Computer** (**未模拟**)
3. Full `/home/box` overlay loss recovery (needs off-host backup)
4. Supervisor boot integration (not available on tini)
5. Postgres crash drill not re-run in this session (older `recovery-check.json` exists)

## 11. Commands cheat-sheet

```bash
cd /home/box/loretide-dev
bash scripts/native-status.sh
bash scripts/native-doctor.sh
bash scripts/native-restore.sh --repair
supervisorctl -c scripts/native-supervisor.conf status
```

## 12. Related docs / artifacts

- `docs/development/native-linux.md`
- `docs/development/native-env-acceptance-2026-09-13.md`
- `docs/development/native-update-runbook.md`
- `data/native/acceptance/*`
