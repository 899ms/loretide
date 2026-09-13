#!/usr/bin/env bash
# Recover a preserved native Loretide dev instance after Grok Bot Update.
# Never initializes/resets a database, never deletes source, never uses Docker.
#
# Exit codes:
#   0  success (services healthy)
#   1  precondition failure (missing preserved data, packages, toolchains, Tailscale)
#   2  usage error or refused destructive option
#   3  post-check failure (API/web/postgres not ready after repair)
#   4  another restore holds the lock
#   5  supervisord active but one or more programs failed to reach RUNNING
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
root="$PWD"
config="$root/scripts/native-supervisor.conf"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p data/native/logs data/native/bin data/native/socket
chmod 700 data/native/socket || true
logfile="data/native/logs/restore-${stamp}.log"

usage() {
  echo 'Usage: bash scripts/native-restore.sh --repair'
  echo 'Safe recovery only: reinstall missing runtime packages if needed, rebuild API binary,'
  echo 'refresh network.env from Tailscale, start supervisord if inactive, or supervisorctl-start'
  echo 'any FATAL/STOPPED programs when supervisord is already running (never wipe).'
  echo "Exit codes: 0 ok | 1 precondition | 2 usage/refused | 3 post-check | 4 locked | 5 program start"
  exit 2
}
[[ "${1:-}" == --repair ]] || usage

for arg in "$@"; do
  case "$arg" in
    --reset|--wipe|--drop|--docker|--initdb)
      echo "Refusing destructive option: $arg" >&2
      exit 2
      ;;
  esac
done

# Lock under logs/; never let supervisord inherit fd 9.
lockfile="data/native/logs/restore.lock"
exec 9>"$lockfile"
flock -n 9 || { echo 'Another recovery is running.' >&2; exit 4; }

log() {
  local line="[restore $(date -u +%H:%M:%SZ)] $*"
  printf '%s\n' "$line" | tee -a "$logfile"
}

log "logfile=$logfile lock=$lockfile"

for file in data/native/env data/native/pg/PG_VERSION; do
  [[ -s "$file" ]] || { log "Missing preserved file: $file. Restore backup first."; exit 1; }
done
[[ "$(cat data/native/pg/PG_VERSION)" == 17 ]] || { log 'Expected PostgreSQL 17 data directory.'; exit 1; }

command -v tailscale >/dev/null || {
  log 'Restore Tailscale and authorize this device from the Bot console first.'
  exit 1
}
ip="$(tailscale ip -4 2>/dev/null || true)"
[[ "$ip" =~ ^100\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  log 'No Tailscale IPv4 address. Authorize this device, then re-run.'
  exit 1
}

missing=()
for package in postgresql-17 postgresql-17-pgvector supervisor; do
  state="$(dpkg-query -W -f='${Status}' "$package" 2>/dev/null || true)"
  [[ "$state" == 'install ok installed' ]] || missing+=("$package")
done
if (( ${#missing[@]} )); then
  [[ -r /etc/apt/sources.list.d/debian.sources ]] || {
    log 'Platform OS package sources missing; review before installing.'
    exit 1
  }
  log "Installing missing packages: ${missing[*]}"
  sed 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources > data/native/debian.sources
  apt_options=(-o "Dir::Etc::sourcelist=$root/data/native/debian.sources" -o Dir::Etc::sourceparts=-)
  sudo -n apt-get "${apt_options[@]}" update -qq
  sudo -n env DEBIAN_FRONTEND=noninteractive apt-get "${apt_options[@]}" install --no-upgrade -y "${missing[@]}"
else
  log 'Required OS packages already installed.'
fi

for binary in data/native/toolchains/go/bin/go data/native/toolchains/node/bin/node; do
  [[ -x "$binary" ]] || {
    log "Preserved toolchain missing: $binary. Restore the project tree under /home/box first."
    exit 1
  }
done

printf 'export LORETIDE_TAILSCALE_IP=%q\n' "$ip" > data/native/network.env
# shellcheck disable=SC1091
source scripts/native-env.sh
log 'Refreshing web deps (frozen lockfile) and rebuilding API binary (data preserved).'
corepack enable --install-directory "$root/data/native/toolchains/node/bin"
# Capture noisy tool output into the restore log without printing secrets via xtrace.
{
  corepack pnpm --filter @multica/web... install --frozen-lockfile
  (cd server && go build -o ../data/native/bin/api ./cmd/server)
} 2>&1 | tee -a "$logfile"

wait_all_running() {
  local deadline=$((SECONDS + 180))
  local status_out
  while (( SECONDS < deadline )); do
    status_out="$(supervisorctl -c "$config" status 2>/dev/null || true)"
    if grep -c 'RUNNING' <<<"$status_out" | grep -qx 3; then
      if ! grep -Eq 'FATAL|STOPPED|EXITED|BACKOFF|STARTING|STOPPING' <<<"$status_out"; then
        return 0
      fi
    fi
    sleep 2
  done
  return 1
}

start_stopped_or_fatal() {
  local status_out name state started=()
  # Do not use supervisorctl status exit code (nonzero when any not RUNNING).
  status_out="$(supervisorctl -c "$config" status 2>/dev/null || true)"
  log "Supervisor already active; starting FATAL/STOPPED programs only (no wipe)."
  while read -r name state _; do
    [[ -n "${name:-}" ]] || continue
    case "$state" in
      FATAL|STOPPED|EXITED|BACKOFF)
        log "supervisorctl start $name (was $state)"
        supervisorctl -c "$config" start "$name" || true
        started+=("$name")
        ;;
      RUNNING|STARTING) ;;
      *) log "Unhandled program state for $name: $state" ;;
    esac
  done <<<"$status_out"

  if wait_all_running; then
    supervisorctl -c "$config" status 2>/dev/null | tee -a "$logfile" || true
    if ((${#started[@]})); then
      log "Started programs: ${started[*]}"
    else
      log 'All programs already RUNNING; nothing to start.'
    fi
    return 0
  fi
  supervisorctl -c "$config" status 2>/dev/null | tee -a "$logfile" || true
  log 'One or more programs did not reach RUNNING.'
  return 5
}

rc=0
if supervisorctl -c "$config" pid >/dev/null 2>&1; then
  start_stopped_or_fatal || rc=$?
else
  log 'Starting supervisord (was not running). Close restore lock fd before spawn.'
  supervisord -c "$config" 9>&-
  sleep 8
  if ! wait_all_running; then
    log 'Supervisord started but programs not all RUNNING.'
    rc=5
  fi
  supervisorctl -c "$config" status 2>/dev/null | tee -a "$logfile" || true
fi

post_ok=1
if ! curl --fail --silent --show-error --max-time 20 http://127.0.0.1:18000/health >/tmp/loretide-restore-api.health; then
  log 'Post-check failed: API /health'
  post_ok=0
fi
# Web may need a long cold compile after start.
if ! curl --fail --silent --show-error --max-time 120 "http://$ip:13000/loretide-dev-check/diagnostics" -o /tmp/loretide-restore-diag.html; then
  log 'Post-check failed: diagnostics HTTP'
  post_ok=0
fi
if ! pg_isready -h 127.0.0.1 -p 15332 >/dev/null; then
  log 'Post-check failed: postgres not ready'
  post_ok=0
fi

{
  echo "recovered_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "tailscale_ip=$ip"
  echo "logfile=$logfile"
  echo "api_health=$(tr -d '\n' </tmp/loretide-restore-api.health 2>/dev/null || echo missing)"
  echo "diagnostics_http=$([[ $post_ok -eq 1 ]] && echo ok || echo fail)"
  echo "postgres=$(pg_isready -h 127.0.0.1 -p 15332 2>/dev/null | tr -d '\n' || echo not-ready)"
  echo "note=database and source were not wiped"
  echo "exit_hint=rc=$rc post_ok=$post_ok"
} | tee data/native/logs/restore-last.txt | tee -a "$logfile"

ln -sfn "$(basename "$logfile")" data/native/logs/restore-latest.log

if (( post_ok != 1 )); then
  log "Recovery post-checks failed. See $logfile"
  flock -u 9 || true
  exit 3
fi
if (( rc != 0 )); then
  log "Recovery finished with program-start issues (exit $rc). See $logfile"
  flock -u 9 || true
  exit "$rc"
fi

log "Recovered. Browser: http://$ip:13000/loretide-dev-check/diagnostics"
log "Log: $logfile"
flock -u 9 || true
exit 0
