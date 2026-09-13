#!/usr/bin/env bash
# Non-destructive native env status. No secrets printed.
# Exit: 0 all healthy; 1 degraded/down
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
config=scripts/native-supervisor.conf
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ok=1

echo "== native-status $stamp =="
echo '== supervisor =='
if supervisorctl -c "$config" pid >/dev/null 2>&1; then
  supervisorctl -c "$config" status || ok=0
  running="$(supervisorctl -c "$config" status 2>/dev/null | grep -c 'RUNNING' || true)"
  echo "running_count=$running/3"
  (( running == 3 )) || ok=0
else
  echo 'supervisord not running'
  ok=0
fi

echo '== ports (expected 15332/18000/13000; warn on duplicates) =='
if command -v ss >/dev/null; then
  ports="$(ss -tlnp 2>/dev/null | rg ':(15332|18000|13000)\b' || true)"
  echo "${ports:-none}"
  for p in 15332 18000 13000; do
    n="$(ss -tlnp 2>/dev/null | rg -c ":${p}\\b" || true)"
    n="${n:-0}"
    echo "listeners_$p=$n"
    if (( n != 1 )); then
      echo "WARN: expected exactly 1 listener on $p (got $n)"
      ok=0
    fi
  done
else
  echo 'ss unavailable'
fi

echo '== health =='
api_body="$(curl -sS --max-time 5 http://127.0.0.1:18000/health 2>/dev/null || true)"
if [[ -n "$api_body" ]]; then
  echo "api_health=$api_body"
else
  echo 'api_health=down'
  ok=0
fi

ip="$(tailscale ip -4 2>/dev/null || true)"
if [[ -n "$ip" ]]; then
  code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "http://$ip:13000/loretide-dev-check/diagnostics" || true)"
  echo "diagnostics_http=$code url=http://$ip:13000/loretide-dev-check/diagnostics"
  [[ "$code" == 200 ]] || ok=0
else
  echo 'tailscale ip unavailable'
  ok=0
fi

echo '== postgres =='
if pg_isready -h 127.0.0.1 -p 15332; then
  :
else
  ok=0
fi

echo '== data presence (no secret values) =='
for path in data/native/env data/native/pg/PG_VERSION data/native/bin/api data/native/toolchains/go/bin/go data/native/toolchains/node/bin/node; do
  if [[ -e "$path" ]]; then
    echo "present $path"
  else
    echo "MISSING $path"
    ok=0
  fi
done
[[ -f data/native/pg/PG_VERSION ]] && echo "pg_version=$(tr -d '\n' < data/native/pg/PG_VERSION)"

if (( ok == 1 )); then
  echo 'overall=healthy'
  exit 0
fi
echo 'overall=degraded'
exit 1
