# Source this file from Bash; credentials are generated outside version control.
LORETIDE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export LORETIDE_ROOT
export PATH="$LORETIDE_ROOT/data/native/toolchains/go/bin:$LORETIDE_ROOT/data/native/toolchains/node/bin:/usr/lib/postgresql/17/bin:$PATH"
if [[ ! -f "$LORETIDE_ROOT/data/native/env" ]]; then
  echo "Missing native credentials: $LORETIDE_ROOT/data/native/env" >&2
  return 1
fi
source "$LORETIDE_ROOT/data/native/env"
if [[ -f "$LORETIDE_ROOT/data/native/network.env" ]]; then
  source "$LORETIDE_ROOT/data/native/network.env"
fi
export APP_ENV=development PORT=18000 FRONTEND_PORT=13000
export LORETIDE_TAILSCALE_IP="${LORETIDE_TAILSCALE_IP:-100.109.104.61}"
export FRONTEND_ORIGIN="http://$LORETIDE_TAILSCALE_IP:13000"
export CORS_ALLOWED_ORIGINS="$FRONTEND_ORIGIN"
export REMOTE_API_URL=http://127.0.0.1:18000
export LOCAL_UPLOAD_DIR="$LORETIDE_ROOT/server/data/uploads"
export GOMAXPROCS=4 GOFLAGS=-p=4 GOTOOLCHAIN=local
export NODE_OPTIONS=--max-old-space-size=4096 NEXT_TELEMETRY_DISABLED=1
