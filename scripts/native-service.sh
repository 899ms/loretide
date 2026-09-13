#!/usr/bin/env bash
set -eu
source "$(dirname "$0")/native-env.sh"
cd "$LORETIDE_ROOT"
case "${1:-}" in
  postgres)
    exec postgres -D data/native/pg -p 15332 -h 127.0.0.1 -k "$LORETIDE_ROOT/data/native/socket"
    ;;
  api)
    cd server
    exec ../data/native/bin/api
    ;;
  web)
    cd apps/web
    exec node node_modules/next/dist/bin/next dev --webpack --hostname "$LORETIDE_TAILSCALE_IP" --port 13000
    ;;
  *) echo 'Usage: native-service.sh postgres|api|web' >&2; exit 2 ;;
esac
