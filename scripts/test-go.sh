#!/usr/bin/env bash
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
GUARD_SCRIPT="$SCRIPT_DIR/go-test-with-agent-cli-guard.sh"

usage() {
  echo "usage: $0 [--race] [--only regular|agent]" >&2
}

# The suite is two `go test` invocations: every package outside pkg/agent at
# the default parallelism, then pkg/agent throttled (see below). `--only`
# selects one half so CI can give each its own runner; the default still runs
# both for `make test`, check.sh, and the release workflow.
go_test_args=(test)
only=all
while [ "$#" -gt 0 ]; do
  case "$1" in
    --race)
      go_test_args+=(-race)
      shift
      ;;
    --only)
      case "${2:-}" in
        regular|agent) only=$2 ;;
        *)
          usage
          exit 2
          ;;
      esac
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

cd "$REPO_ROOT/server"

if [ "$only" != agent ]; then
  packages=$(go list ./...)
  regular_packages=()
  for package in $packages; do
    case "$package" in
      */pkg/agent|*/pkg/agent/*) ;;
      # cmd/server is still a package-wide database suite: it fails closed
      # without an isolated database (internal/testutil/dbtest), so running it
      # here would turn this wrapper red on every machine that has no such
      # database. It runs through test-go-db.sh instead, against a database
      # provisioned for one run.
      #
      # internal/handler is no longer excluded. Its database-dependent test
      # files carry the dbtest build tag (Phase 2), so without that tag the
      # package builds to its database-free tests only and this wrapper runs
      # them. cmd/server gets the same treatment in the second half of Phase 2;
      # until then its exclusion is a real coverage gap, not a claim that its
      # database-free tests still run.
      */cmd/server) ;;
      *) regular_packages+=("$package") ;;
    esac
  done
  "$GUARD_SCRIPT" -- go "${go_test_args[@]}" "${regular_packages[@]}"
fi

if [ "$only" != regular ]; then
  # Subprocess-backed agent tests have hard deadlines. Limit both package and
  # within-package parallelism so race builds do not starve their parent loops.
  "$GUARD_SCRIPT" -- go "${go_test_args[@]}" -p 2 -parallel 2 ./pkg/agent/...
fi
