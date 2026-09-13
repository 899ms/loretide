#!/usr/bin/env bash
# Refuse handler fixture tests unless their process has an explicit, isolated DB.
set -euo pipefail

expected_db="${LORETIDE_HANDLER_TEST_DB:?LORETIDE_HANDLER_TEST_DB is required}"
expected_role="${LORETIDE_HANDLER_TEST_ROLE:?LORETIDE_HANDLER_TEST_ROLE is required}"
: "${DATABASE_URL:?DATABASE_URL is required}"

case "$expected_db" in
  loretide_handler_test_*) ;;
  *) echo "refusing handler tests: non-isolated database name" >&2; exit 2 ;;
esac

actual_db="$(psql "$DATABASE_URL" -Atc 'SELECT current_database()')"
actual_role="$(psql "$DATABASE_URL" -Atc 'SELECT current_user')"

if [[ "$actual_db" != "$expected_db" || "$actual_role" != "$expected_role" ]]; then
  echo "refusing handler tests: DATABASE_URL does not identify the required isolated role/database" >&2
  exit 2
fi

if [[ "$actual_db" == "loretide_dev" ]]; then
  echo "refusing handler tests: development database" >&2
  exit 2
fi

echo "handler-test-db=isolated"
