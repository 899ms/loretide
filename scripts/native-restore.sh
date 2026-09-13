#!/usr/bin/env bash
# Recover a preserved project after platform Update; never initialize a database.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
root="$PWD"
[[ "${1:-}" == --repair ]] || { echo 'Usage: bash scripts/native-restore.sh --repair'; exit 2; }
for file in data/native/env data/native/pg/PG_VERSION; do
  [[ -s "$file" ]] || { echo "Missing preserved file: $file. Restore backup first." >&2; exit 1; }
done
[[ "$(cat data/native/pg/PG_VERSION)" == 17 ]] || { echo 'Expected PostgreSQL 17 data.' >&2; exit 1; }
command -v tailscale >/dev/null || { echo 'Restore Tailscale and authorize this device from the Bot console first.' >&2; exit 1; }
ip="$(tailscale ip -4)"
[[ "$ip" =~ ^100\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'No Tailscale IPv4 address.' >&2; exit 1; }
mkdir -p data/native/logs data/native/bin
exec 9>data/native/restore.lock
flock -n 9 || { echo 'Another recovery is running.' >&2; exit 1; }
missing=()
for package in postgresql-17 postgresql-17-pgvector supervisor; do
  state="$(dpkg-query -W -f='${Status}' "$package" 2>/dev/null || true)"
  [[ "$state" == 'install ok installed' ]] || missing+=("$package")
done
if (( ${#missing[@]} )); then
  [[ -r /etc/apt/sources.list.d/debian.sources ]] || { echo 'Platform OS changed; review package sources.' >&2; exit 1; }
  sed 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources > data/native/debian.sources
  apt_options=(-o "Dir::Etc::sourcelist=$root/data/native/debian.sources" -o Dir::Etc::sourceparts=-)
  sudo -n apt-get "${apt_options[@]}" update -qq
  sudo -n env DEBIAN_FRONTEND=noninteractive apt-get "${apt_options[@]}" install --no-upgrade -y "${missing[@]}"
fi
# These isolated runtimes are in /home/box, not system package directories.
for binary in data/native/toolchains/go/bin/go data/native/toolchains/node/bin/node; do
  [[ -x "$binary" ]] || { echo "Preserved toolchain missing: $binary. Restore the project backup first." >&2; exit 1; }
done
config="$root/scripts/native-supervisor.conf"
if supervisorctl -c "$config" pid >/dev/null 2>&1; then
  echo 'Supervisor already active; configuration and running dependencies unchanged.'
  supervisorctl -c "$config" status
  exit 0
fi
printf 'export LORETIDE_TAILSCALE_IP=%q\n' "$ip" > data/native/network.env
source scripts/native-env.sh
corepack enable --install-directory "$root/data/native/toolchains/node/bin"
corepack pnpm --filter @multica/web... install --frozen-lockfile
(cd server && go build -o ../data/native/bin/api ./cmd/server)
supervisord -c "$config"
sleep 8
supervisorctl -c "$config" status
curl --fail --max-time 15 http://127.0.0.1:18000/health
echo "Recovered. Browser: http://$ip:13000/loretide-dev-check/issues"
