#!/usr/bin/env bash
# Contract tests for test-go-db.sh.
#
# The point of every case below is the same: the wrapper must not invent
# configuration. A wrapper that filled in a default database, or that passed a
# partial environment through to go, would restore the behaviour this change
# removed — so the fake go records whether it was called at all, and a refusal
# that still called go is a failure even if its message was right.
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/loretide-test-go-db.XXXXXX")
BIN_DIR="$TEST_DIR/bin"
CALLS_FILE="$TEST_DIR/go-calls.log"
OUTPUT_FILE="$TEST_DIR/output.log"

cleanup() {
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT

mkdir -p "$BIN_DIR"
export LORETIDE_TEST_GO_CALLS="$CALLS_FILE"
: >"$CALLS_FILE"

cat >"$BIN_DIR/go" <<'FAKE'
#!/usr/bin/env bash
set -eu
# Records the invocation and the suite the wrapper exported, so a case can
# assert both the command line and the environment the suite would have run in.
printf '%s|suite=%s|topic-url=%s\n' "$*" "${LORETIDE_DB_TEST_SUITE:-}" \
  "${LORETIDE_TOPIC_TEST_DATABASE_URL:-}" >>"$LORETIDE_TEST_GO_CALLS"
FAKE
chmod 755 "$BIN_DIR/go"

# A complete, syntactically valid environment. No case connects anywhere: the
# fake go never runs a test binary.
complete_env() {
  LORETIDE_DB_TESTS=1 \
  LORETIDE_DB_TEST_DATABASE_URL="postgres://r:p@127.0.0.1:5432/loretide_db?sslmode=disable" \
  LORETIDE_DB_TEST_DATABASE=loretide_db \
  LORETIDE_DB_TEST_ROLE=loretide_role \
  LORETIDE_DB_TEST_RUN_ID=12345_1 \
  "$@"
}

# $1: label, $2: expected recorded call. Clears the log.
expect_call() {
  actual=$(cat "$CALLS_FILE")
  if [ "$actual" != "$2" ]; then
    echo "$1: unexpected go call:" >&2
    printf 'got:  %s\nwant: %s\n' "$actual" "$2" >&2
    exit 1
  fi
  : >"$CALLS_FILE"
}

# $1: label, $2: expected exit status, rest: wrapper arguments. Asserts that go
# was never called — a refusal that still shelled out has not refused.
expect_refusal() {
  label=$1
  want=$2
  shift 2
  set +e
  PATH="$BIN_DIR:$PATH" bash "$SCRIPT_DIR/test-go-db.sh" "$@" >"$OUTPUT_FILE" 2>&1
  status=$?
  set -e
  if [ "$status" -ne "$want" ]; then
    echo "$label returned $status, want $want" >&2
    cat "$OUTPUT_FILE" >&2
    exit 1
  fi
  if [ -s "$CALLS_FILE" ]; then
    echo "$label refused but still invoked go:" >&2
    cat "$CALLS_FILE" >&2
    exit 1
  fi
}

# Exact command construction, and the suite the wrapper sets for the child.
PATH="$BIN_DIR:$PATH" complete_env bash "$SCRIPT_DIR/test-go-db.sh" --suite handler
expect_call "handler" "test -count=1 ./internal/handler|suite=handler|topic-url="

PATH="$BIN_DIR:$PATH" complete_env bash "$SCRIPT_DIR/test-go-db.sh" --suite cmd-server
expect_call "cmd-server" "test -count=1 ./cmd/server|suite=cmd-server|topic-url="

PATH="$BIN_DIR:$PATH" complete_env bash "$SCRIPT_DIR/test-go-db.sh" --suite topic-planning
expect_call "topic-planning" "test -count=1 ./internal/content/topic-planning|suite=topic-planning|topic-url=postgres://r:p@127.0.0.1:5432/loretide_db?sslmode=disable"

PATH="$BIN_DIR:$PATH" complete_env bash "$SCRIPT_DIR/test-go-db.sh" --suite handler --json
expect_call "handler --json" "test --json -count=1 ./internal/handler|suite=handler|topic-url="

PATH="$BIN_DIR:$PATH" complete_env bash "$SCRIPT_DIR/test-go-db.sh" --suite handler -- -run TestOne
expect_call "extra go args" "test -count=1 -run TestOne ./internal/handler|suite=handler|topic-url="

# The suite is set by the wrapper, not inherited: an environment built for the
# other suite must not steer this command at the other suite's database.
PATH="$BIN_DIR:$PATH" LORETIDE_DB_TEST_SUITE=cmd-server complete_env \
  bash "$SCRIPT_DIR/test-go-db.sh" --suite handler
expect_call "inherited suite is overridden" "test -count=1 ./internal/handler|suite=handler|topic-url="

PATH="$BIN_DIR:$PATH" LORETIDE_TOPIC_TEST_DATABASE_URL=postgres://stale complete_env \
  bash "$SCRIPT_DIR/test-go-db.sh" --suite handler
expect_call "inherited topic target is cleared" "test -count=1 ./internal/handler|suite=handler|topic-url="

# No opt-in, no run.
expect_refusal "absent opt-in" 2 --suite handler

# Every required variable, one at a time.
for variable in \
  LORETIDE_DB_TEST_DATABASE_URL \
  LORETIDE_DB_TEST_DATABASE \
  LORETIDE_DB_TEST_ROLE \
  LORETIDE_DB_TEST_RUN_ID
do
  set +e
  env LORETIDE_DB_TESTS=1 \
    LORETIDE_DB_TEST_DATABASE_URL="postgres://r:p@127.0.0.1:5432/loretide_db" \
    LORETIDE_DB_TEST_DATABASE=loretide_db \
    LORETIDE_DB_TEST_ROLE=loretide_role \
    LORETIDE_DB_TEST_RUN_ID=12345_1 \
    "$variable=" \
    PATH="$BIN_DIR:$PATH" bash "$SCRIPT_DIR/test-go-db.sh" --suite handler >"$OUTPUT_FILE" 2>&1
  status=$?
  set -e
  if [ "$status" -ne 2 ]; then
    echo "missing $variable returned $status, want 2" >&2
    cat "$OUTPUT_FILE" >&2
    exit 1
  fi
  if ! grep -q "$variable" "$OUTPUT_FILE"; then
    echo "missing $variable was refused without naming it" >&2
    cat "$OUTPUT_FILE" >&2
    exit 1
  fi
  if [ -s "$CALLS_FILE" ]; then
    echo "missing $variable still invoked go" >&2
    exit 1
  fi
done

# An opt-in that is not exactly 1. "0" and "true" are both things people type.
for value in 0 true yes; do
  set +e
  env LORETIDE_DB_TESTS="$value" \
    LORETIDE_DB_TEST_DATABASE_URL="postgres://r:p@127.0.0.1:5432/loretide_db" \
    LORETIDE_DB_TEST_DATABASE=loretide_db \
    LORETIDE_DB_TEST_ROLE=loretide_role \
    LORETIDE_DB_TEST_RUN_ID=12345_1 \
    PATH="$BIN_DIR:$PATH" bash "$SCRIPT_DIR/test-go-db.sh" --suite handler >"$OUTPUT_FILE" 2>&1
  status=$?
  set -e
  if [ "$status" -ne 2 ] || [ -s "$CALLS_FILE" ]; then
    echo "LORETIDE_DB_TESTS=$value was accepted (status $status)" >&2
    cat "$OUTPUT_FILE" >&2
    exit 1
  fi
done

# Argument errors never reach go either.
expect_refusal "no suite" 2
expect_refusal "unknown suite" 2 --suite everything
expect_refusal "unknown option" 2 --suite handler --unknown

echo "test-go-db.test.sh: PASS"
