#!/usr/bin/env bash
# Read-only native env diagnostics. Aggregates supervisor, health, ports, pg,
# recent error log tails (redacted), disk/mem, data dir presence.
# Never prints passwords/tokens from data/native/env or pgpassword.
# Exit: 0 healthy; 1 issues found
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
config=scripts/native-supervisor.conf
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ok=1

redact() {
  # Strip common secret patterns from log lines before display
  sed -E \
    -e 's/(password|passwd|pwd|token|secret|api[_-]?key|authorization|bearer)(=|:|[[:space:]]+)[^[:space:]]+/\1=***REDACTED***/Ig' \
    -e 's|postgres(ql)?://[^[:space:]]+|postgres://***REDACTED***|Ig' \
    -e 's|(DATABASE_URL=)[^[:space:]]+|\1***REDACTED***|Ig' \
    -e 's/(PGPASSWORD=)[^[:space:]]+/\1***REDACTED***/Ig'
}

echo "======== native-doctor $stamp ========"

echo
echo '## supervisor'
if supervisorctl -c "$config" pid >/dev/null 2>&1; then
  spid="$(supervisorctl -c "$config" pid 2>/dev/null || echo unknown)"
  echo "supervisord_pid=$spid"
  supervisorctl -c "$config" status || ok=0
  running="$(supervisorctl -c "$config" status 2>/dev/null | grep -c 'RUNNING' || true)"
  echo "running_count=$running/3"
  (( running == 3 )) || ok=0
else
  echo 'supervisord: NOT RUNNING'
  ok=0
fi

echo
echo '## health'
api_code="$(curl -sS -o /tmp/loretide-doctor-api.health -w '%{http_code}' --max-time 5 http://127.0.0.1:18000/health || true)"
echo "api_http=$api_code body=$(tr -d '\n' </tmp/loretide-doctor-api.health 2>/dev/null || true)"
[[ "$api_code" == 200 ]] || ok=0

ip="$(tailscale ip -4 2>/dev/null || true)"
if [[ -n "$ip" ]]; then
  web_code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "http://$ip:13000/loretide-dev-check/diagnostics" || true)"
  echo "web_diagnostics_http=$web_code url=http://$ip:13000/loretide-dev-check/diagnostics"
  [[ "$web_code" == 200 ]] || ok=0
else
  echo 'web_diagnostics_http=unavailable (no Tailscale IPv4)'
  ok=0
fi

echo
echo '## postgres'
if pg_isready -h 127.0.0.1 -p 15332; then
  echo 'pg_isready=ok'
else
  echo 'pg_isready=FAIL'
  ok=0
fi
if [[ -f data/native/pg/PG_VERSION ]]; then
  echo "pg_version_file=$(tr -d '\n' < data/native/pg/PG_VERSION)"
fi

echo
echo '## ports'
if command -v ss >/dev/null; then
  ss -tlnp 2>/dev/null | rg ':(15332|18000|13000)\b' || echo 'no expected listeners'
  for p in 15332 18000 13000; do
    n="$(ss -tlnp 2>/dev/null | rg -c ":${p}\\b" || true)"
    echo "listeners_$p=${n:-0}"
    if [[ "${n:-0}" != 1 ]]; then
      echo "WARN: expected 1 listener on $p"
      ok=0
    fi
  done
fi

echo
echo '## data dirs / files (presence only)'
for path in \
  data/native \
  data/native/env \
  data/native/pg \
  data/native/pg/PG_VERSION \
  data/native/socket \
  data/native/bin/api \
  data/native/logs \
  data/native/toolchains/go/bin/go \
  data/native/toolchains/node/bin/node \
  scripts/native-supervisor.conf \
  scripts/native-restore.sh \
  scripts/native-status.sh
do
  if [[ -e "$path" ]]; then
    if [[ -f "$path" ]]; then
      echo "OK file $path size=$(wc -c <"$path" | tr -d ' ') mode=$(stat -c '%a' "$path" 2>/dev/null || echo '?')"
    else
      echo "OK dir  $path"
    fi
  else
    echo "MISSING $path"
    ok=0
  fi
done
# Never cat env/pgpassword
echo 'note=data/native/env and pgpassword contents intentionally omitted'

echo
echo '## disk / memory'
df -h / /home/box 2>/dev/null | uniq || df -h /
free -h || true

echo
echo '## process RSS/CPU (stable snapshot, no stress)'
ps -o pid,rss,pcpu,comm -p "$(supervisorctl -c "$config" pid postgres 2>/dev/null || echo 0)" 2>/dev/null || true
ps -o pid,rss,pcpu,comm -p "$(supervisorctl -c "$config" pid api 2>/dev/null || echo 0)" 2>/dev/null || true
# web is a process group; show supervisor child + next-server if present
web_pid="$(supervisorctl -c "$config" pid web 2>/dev/null || echo 0)"
ps -o pid,rss,pcpu,comm -p "$web_pid" 2>/dev/null || true
pgrep -a 'next-server' 2>/dev/null | head -3 || true

echo
echo '## recent log errors (redacted tails)'
for log in \
  data/native/logs/supervisor.log \
  data/native/logs/postgres-supervised.log \
  data/native/logs/api-supervised.log \
  data/native/logs/web-supervised.log
do
  echo "--- $log ---"
  if [[ -f "$log" ]]; then
    # last 200 lines, keep only error-ish, redact, max 15 lines
    tail -n 200 "$log" | rg -i 'error|fatal|panic|emerg|exception|denied|refused' | redact | tail -n 15 || echo '(no recent error-matching lines)'
  else
    echo '(missing)'
  fi
done

echo
if (( ok == 1 )); then
  echo '======== doctor overall=healthy ========'
  exit 0
fi
echo '======== doctor overall=issues ========'
exit 1
