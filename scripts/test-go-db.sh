#!/usr/bin/env bash
# Run one package-wide database test suite against an explicitly provisioned,
# isolated database.
#
# The two suites it can run — internal/handler and cmd/server — refuse to start
# without the full LORETIDE_DB_TEST_* contract (server/internal/testutil/dbtest).
# This wrapper exists so the contract is spelled once, in a file that is
# reviewed, rather than retyped into every job and every shell.
#
# It never invents configuration. If a required variable is missing it exits
# without calling go, because a wrapper that filled in a default would restore
# exactly the behaviour this replaced.
#
# Contract: docs/development/testing-database-suites.md
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

usage() {
  echo "usage: $0 --suite handler|cmd-server [--json] [-- go-test-args...]" >&2
}

suite=
json=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --suite)
      case "${2:-}" in
        handler|cmd-server) suite=$2 ;;
        *)
          usage
          exit 2
          ;;
      esac
      shift 2
      ;;
    --json)
      json=--json
      shift
      ;;
    --)
      shift
      break
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

if [ -z "$suite" ]; then
  usage
  exit 2
fi

# The opt-in and every identifier must already be in the environment. This
# wrapper checks them so the failure names the missing variable instead of
# surfacing as a Go panic, but the Go guard checks them again: the shell check
# and the Go check are defence in depth against the same mistake, not two
# independent proofs.
missing=
for variable in \
  LORETIDE_DB_TESTS \
  LORETIDE_DB_TEST_DATABASE_URL \
  LORETIDE_DB_TEST_DATABASE \
  LORETIDE_DB_TEST_ROLE \
  LORETIDE_DB_TEST_RUN_ID
do
  eval "value=\${$variable:-}"
  if [ -z "$value" ]; then
    missing="$missing $variable"
  fi
done

if [ -n "$missing" ]; then
  echo "refusing to run the $suite database suite: missing$missing" >&2
  echo "see docs/development/testing-database-suites.md" >&2
  exit 2
fi

if [ "${LORETIDE_DB_TESTS}" != "1" ]; then
  echo "refusing to run the $suite database suite: LORETIDE_DB_TESTS must be exactly 1" >&2
  exit 2
fi

# The suite name is set here from the selected suite rather than inherited.
# An inherited value is how a handler command ends up pointed at the other
# suite's database, and the Go guard compares this against its own constant.
export LORETIDE_DB_TEST_SUITE="$suite"

case "$suite" in
  handler) package=./internal/handler ;;
  cmd-server) package=./cmd/server ;;
esac

cd "$REPO_ROOT/server"

# -tags=dbtest selects the database half of the suite. Without it the package
# builds to its database-free tests only — which is exactly what the default
# wrapper runs — so a database job that forgot the tag would provision a
# database, connect to nothing, and still report green.
#
# -count=1 because a cached result is not evidence that anything ran, which is
# the failure mode this whole change exists to remove.
exec go test -tags=dbtest ${json:+$json} -count=1 "$@" "$package"
