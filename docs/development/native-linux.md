# Native Linux development

Current candidate environment: `box@100.109.104.61` (Tailscale SSH), `/home/box/loretide-dev`. No Docker or OpenSSH Server installation is required for this workflow. Existing platform services are outside this project's lifecycle.

Project toolchains: Go 1.26.6, Node 22.23.2 and pnpm 10.28.2. Go/Node archives were downloaded from official release endpoints and SHA256 checked. They live in `data/native/toolchains`; system Go/Node remain unchanged. PostgreSQL 17.11 and pgvector 0.8.0 were installed from Debian's signed repository using a temporary HTTPS source list. The package-created `17/main` cluster remains down.

The project-owned PostgreSQL cluster lives in `data/native/pg`, listens on loopback port 15332 and uses `data/native/socket` (mode 700). TCP uses SCRAM authentication; local socket connections trust users able to traverse that private directory. The role `loretide` is the cluster's development superuser. Credentials in `data/native/env` and `pgpassword` are mode 600 and ignored by Git.

Enter the environment:

```bash
cd /home/box/loretide-dev
source scripts/native-env.sh
go version
node --version
pnpm --version
pg_ctl -D data/native/pg status
curl -f http://127.0.0.1:18000/health
```

Web listens only on Tailscale IP port 13000. The upstream API listens on all interfaces port 18000; database remains loopback-only. No public ingress or daemon has been configured by this project. Application authentication is still required; Tailscale connectivity is not application authorization.

## Manual lifecycle

The current API and Web run as `box` background processes, with PID files `data/native/api.pid` and `web.pid`. Before stopping either, verify `/proc/<pid>/cmdline`, `/proc/<pid>/cwd` and process ownership match this project; do not blindly trust a stale PID file. Do not launch duplicates when the ports are occupied.

After stopping the matching services, start PostgreSQL if needed:

```bash
pg_ctl -D data/native/pg -l data/native/logs/postgres.log \
  -o "-p 15332 -h 127.0.0.1 -k $LORETIDE_ROOT/data/native/socket" start
```

Build API in `server` using `go run ./cmd/migrate up`, then `go build -o ../data/native/bin/api ./cmd/server`. Launch that binary from `server` with the sourced environment and redirect output to `data/native/logs/api.log`. Start Web from `apps/web` using `pnpm exec next dev --webpack --hostname 100.109.104.61 --port 13000`, redirecting output to `data/native/logs/web.log`. PostgreSQL can be stopped with `pg_ctl -D data/native/pg stop -m fast` after stopping application services. No automatic boot service is configured yet.

## Backup and acceptance boundary

`data/native/acceptance.dump` is a custom-format pg_dump, restored successfully to separate database `loretide_restore_check_20260913`. An off-host copy is held on the operator's Windows computer; never commit it. Preserve local application Git history and attachment backups as well. Web/API/DB restart survival does not establish persistence after the platform destroys or recreates its overlay filesystem. Do not delete the fallback VPS based solely on this smoke test.

This is environment acceptance, not acceptance of the planned Loretide business features or all upstream tests. Browser evidence and any typecheck failures are recorded in the parent repository's dated acceptance report.
