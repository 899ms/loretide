#!/usr/bin/env bash
# Run one package-wide database test suite against an explicitly provisioned,
# isolated database.
#
# The wrapper requires the full LORETIDE_DB_TEST_* contract for all three
# suites. internal/handler and cmd/server repeat that check in Go; the older
# topic-planning fixture itself skips on a direct go test without its private
# URL, so it must be invoked through this wrapper in CI.
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
  echo "usage: $0 --suite handler|cmd-server|topic-planning [--json] [-- go-test-args...]" >&2
}

suite=
json=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --suite)
      case "${2:-}" in
        handler|cmd-server|topic-planning) suite=$2 ;;
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
# surfacing as a Go panic. handler and cmd/server check them again in Go; the
# topic-planning fixture is older and only receives its private URL below.
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
unset LORETIDE_TOPIC_TEST_DATABASE_URL

case "$suite" in
  handler) package=./internal/handler ;;
  cmd-server) package=./cmd/server ;;
  # The store fixture creates an isolated schema inside this already
  # per-run-provisioned database. Keep its historical variable private to this
  # one process so a developer shell can never inherit an opt-in target.
  topic-planning)
    package=./internal/content/topic-planning
    export LORETIDE_TOPIC_TEST_DATABASE_URL="$LORETIDE_DB_TEST_DATABASE_URL"
    ;;
esac

cd "$REPO_ROOT/server"

# -count=1 because a cached result is not evidence that anything ran, which is
# the failure mode this whole change exists to remove.
exec go test ${json:+$json} -count=1 "$@" "$package"
