#!/usr/bin/env bash
# Refuse a database test suite unless its process points at an isolated,
# least-privilege database provisioned for this run.
#
# It checks the same things the Go guard checks (internal/testutil/dbtest), in
# the shell, before Go starts. That is defence in depth against one mistake —
# a miswired job — not two independent proofs: both read the same environment,
# and anyone who can set it can satisfy both.
#
# It replaces verify-handler-test-db.sh, which required the generic
# DATABASE_URL and a hard-coded name prefix. The old script is left in place
# because an open PR still calls it; it should be deleted once that PR lands.
#
# Contract: docs/development/testing-database-suites.md
set -euo pipefail

: "${LORETIDE_DB_TESTS:?LORETIDE_DB_TESTS is required}"
: "${LORETIDE_DB_TEST_DATABASE_URL:?LORETIDE_DB_TEST_DATABASE_URL is required}"
expected_db="${LORETIDE_DB_TEST_DATABASE:?LORETIDE_DB_TEST_DATABASE is required}"
expected_role="${LORETIDE_DB_TEST_ROLE:?LORETIDE_DB_TEST_ROLE is required}"
: "${LORETIDE_DB_TEST_RUN_ID:?LORETIDE_DB_TEST_RUN_ID is required}"
suite="${LORETIDE_DB_TEST_SUITE:?LORETIDE_DB_TEST_SUITE is required}"

if [[ "$LORETIDE_DB_TESTS" != "1" ]]; then
  echo "refusing database tests: LORETIDE_DB_TESTS must be exactly 1" >&2
  exit 2
fi

case "$suite" in
  handler|cmd-server) ;;
  *) echo "refusing database tests: unknown suite '$suite'" >&2; exit 2 ;;
esac

# Plain lowercase identifiers only. Anything needing a quote is being injected
# rather than derived by the provisioner.
for name in "$expected_db" "$expected_role"; do
  if [[ ! "$name" =~ ^[a-z_][a-z0-9_]{0,62}$ ]]; then
    echo "refusing database tests: '$name' is not a plain identifier" >&2
    exit 2
  fi
done

# One round trip, so the database cannot be one thing for this check and
# another for the next. Nine values, compared as a single string: a partial
# match is a mismatch.
identity="$(psql "$LORETIDE_DB_TEST_DATABASE_URL" -X -A -t -F '|' -v ON_ERROR_STOP=1 -c \
  "SELECT current_database(), current_user, r.rolcanlogin::text, r.rolsuper::text, r.rolcreatedb::text, r.rolcreaterole::text, r.rolreplication::text, r.rolbypassrls::text, (d.datdba = r.oid)::text
     FROM pg_roles r
     JOIN pg_database d ON d.datname = current_database()
    WHERE r.rolname = current_user")"

expected_identity="${expected_db}|${expected_role}|true|false|false|false|false|false|true"

# Safe to print: names and booleans, never the URL.
echo "db-test target ($suite): $identity"

if [[ "$identity" != "$expected_identity" ]]; then
  echo "refusing database tests: the connected identity, role attributes or database owner do not match the run contract" >&2
  echo "expected: $expected_identity" >&2
  exit 2
fi

echo "db-test-target=verified suite=$suite database=$expected_db role=$expected_role"
