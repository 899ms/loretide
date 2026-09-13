# Loretide native env acceptance — 2026-09-13

Scope: independent environment acceptance and recovery for `/home/box/loretide-dev` on this Grok Bot cloud computer. Not brand/account/content business acceptance. No Docker. No database wipe. Business modules were not modified.

Operator: 开发测试bot. Instance Tailscale IPv4 at time of test: `100.109.104.61`.

## Current run model (read-only)

| Layer | Detail |
| --- | --- |
| Process manager | `supervisord` as user `box`, config `scripts/native-supervisor.conf` |
| Control socket | `data/native/supervisor.sock` (0600), no HTTP admin port |
| PostgreSQL | Project cluster `data/native/pg`, loopback `127.0.0.1:15332`, socket dir `data/native/socket` |
| API | `data/native/bin/api` via `scripts/native-service.sh api`, listen `*:18000` |
| Web | Next.js `next dev` via `scripts/native-service.sh web`, bind Tailscale IP `:13000` |
| Env | `source scripts/native-env.sh` → `data/native/env` + `data/native/network.env` |
| Toolchains | `data/native/toolchains/{go,node}` (preserved under `/home/box`) |
| Auto-restart | `autorestart=true` for postgres/api/web; supervisor itself is **not** platform-boot integrated (tini host) |

## Commands executed

```bash
# Inventory
ls /home/box/loretide-dev
supervisorctl -c scripts/native-supervisor.conf status
ss -tlnp | rg ':(15332|18000|13000)\\b'
curl -sS http://127.0.0.1:18000/health
curl -sS -o /tmp/diag.html -w '%{http_code}' http://100.109.104.61:13000/loretide-dev-check/diagnostics

# Diagnostic row baseline (preserve check)
psql -h 127.0.0.1 -p 15332 -U loretide -d loretide_dev \
  -Atc 'SELECT count(*) FROM content_diagnostic_run;'
# before_count=3 ; marker md5=1625628ce1b3d0033892832f1789034e

# API/Web-only crash recovery (PostgreSQL left running)
python3 scripts/native-api-web-recovery-check.py

# Post status
bash scripts/native-status.sh
```

## Results

### Service daemon & auto-recovery

| Service | Before kill | Recovered | HTTP | Notes |
| --- | --- | --- | --- | --- |
| api | pid 198736 | pid 216535 in 6.37s | `/health` 200 in 6.38s | SIGKILL process group only |
| web | pid 77979 | pid 216660 in 6.39s | diagnostics 200 in 58.36s | Next cold compile dominates ready time |
| postgres | pid 77976 | **unchanged** 77976 | `pg_isready` accepting | Intentionally not killed in this run |

Artifact: `data/native/api-web-recovery-check.json`.

Prior full three-service check (existing): `data/native/recovery-check.json` (includes postgres kill from earlier operator run).

### Browser / diagnostics accessibility

- `http://100.109.104.61:13000/loretide-dev-check/diagnostics` → HTTP 200 after web recovery (~318 KB HTML).
- This confirms environment reachability of the diagnostics route, **not** full business feature acceptance.

### Diagnostic data retention

| Check | Before | After API/Web crash recovery |
| --- | --- | --- |
| `content_diagnostic_run` count | 3 | 3 |
| row-set md5 marker | `1625628ce1b3d0033892832f1789034e` | identical |
| `data/native/env` sha256 | unchanged | unchanged |
| `data/native/pg/PG_VERSION` | unchanged | unchanged |
| `data/native/bin/api` sha256 | unchanged | unchanged |

### `/home/box` persistence (daily restart vs Update)

| Event | Expected for this layout | Verified this session? |
| --- | --- | --- |
| Ordinary session / process restart | `/home/box/loretide-dev` source, `data/native/pg`, env, toolchains remain | Yes — survived API/Web SIGKILL and supervisor respawn |
| Full Grok Bot computer reboot | Platform docs: files under `/home/box` and `/workspace` persist across turns | **Not** force-rebooted this session (would interrupt other bots on shared computer) |
| Update Grok Bot's Computer | Files/logins kept; apt packages / CLI may be lost → run `bash scripts/native-restore.sh --repair` | Script reviewed and hardened; full Update not executed |

## One-click recovery

```bash
cd /home/box/loretide-dev
bash scripts/native-restore.sh --repair
bash scripts/native-status.sh
```

Guarantees / refusals:

- Does **not** initdb, drop DB, git reset/clean, or use Docker.
- Requires preserved `data/native/env`, `data/native/pg/PG_VERSION=17`, Go/Node toolchains, Tailscale IPv4.
- Reinstalls only missing `postgresql-17`, `postgresql-17-pgvector`, `supervisor` packages.
- Rebuilds API binary; `pnpm --frozen-lockfile` for web filter; starts supervisord only if not already active.
- Post-checks: API health, diagnostics HTTP, `pg_isready`.

Related helpers:

- `scripts/native-status.sh` — non-destructive status
- `scripts/native-api-web-recovery-check.py` — API/Web crash drill (leaves postgres alone)
- `scripts/native-recovery-check.py` — older drill that also kills postgres

## Log locations

| Log | Path |
| --- | --- |
| supervisord | `data/native/logs/supervisor.log` |
| postgres (supervised) | `data/native/logs/postgres-supervised.log` |
| api | `data/native/logs/api-supervised.log` |
| web | `data/native/logs/web-supervised.log` |
| last restore summary | `data/native/logs/restore-last.txt` (after `--repair`) |
| status snapshot | `data/native/logs/status-after-acceptance.txt` |

## Uncovered risks

1. **Supervisor is not boot-persistent** on this platform (PID 1 = tini). After a full computer recreate/boot, someone must run `native-restore.sh --repair` or start `supervisord` manually.
2. **Tailscale authorization** is external; restore fails closed without `100.x` address.
3. **Web ready time ~1 minute** after crash; monitors should wait longer than API.
4. **Shared cloud computer** with other bots — resource pressure and accidental cross-impact possible.
5. **No `.git` directory** present in this tree at check time — source backup/off-host copy remains important.
6. **API listens on `*:18000`** — rely on Tailscale/network policy; not claimed as public-hardening acceptance.
7. This run did **not** simulate postgres crash (by request). Older `recovery-check.json` covers that path separately.
8. “Service starts” ≠ business diagnostics workflow correctness beyond HTTP 200 and row retention.

## Rollback

- Services: `supervisorctl -c scripts/native-supervisor.conf restart api|web|postgres` or `stop`/`start`.
- Do **not** use `drop-database.sh` for env recovery rollback.
- DB dumps available locally: `data/native/acceptance.dump`, `before-diagnostics-20260913.dump`, `migration.dump` (operator off-host copy also referenced in `native-linux.md`).
- Binaries: `data/native/bin/api.before-diagnostics` exists as a prior binary snapshot if needed.

## Final state (end of acceptance)

```
api       RUNNING
postgres  RUNNING  (same pid as pre-API/Web drill)
web       RUNNING
curl /health → 200
diagnostics URL → 200
content_diagnostic_run count → 3 (preserved)
```

## Next suggestions

1. After any Grok Bot **Update**, run `bash scripts/native-restore.sh --repair` before declaring the env ready.
2. Optionally add a tiny routine that only runs `native-status.sh` and pings on FAILURE (not on healthy silence).
3. Keep off-host copies of `data/native/pg` dumps and the project tree; do not treat this computer as sole backup.
4. If business diagnostics behavior regresses, file evidence separately — out of scope for this env acceptance.
