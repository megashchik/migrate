#!/usr/bin/env bash
#
# Integration test: spins up a real PostgreSQL via Docker, runs the migrate
# tool against it, and verifies every command (up, list, last, check).
#
set -u

PG_IMAGE="postgres:16-alpine"
PG_CONTAINER="migrate-test-pg"
NETWORK="migrate-test-net"
MIGRATE_IMAGE="migrate-test:latest"

PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); printf "  \033[32m✓ %s\033[0m\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS="${ERRORS}  ✗ $1: $2\n"; printf "  \033[31m✗ %s: %s\033[0m\n" "$1" "$2"; }

cleanup() {
    docker rm -f "$PG_CONTAINER" >/dev/null 2>&1 || true
    docker rm -f migrate-test-run >/dev/null 2>&1 || true
    docker network rm "$NETWORK" >/dev/null 2>&1 || true
    docker rmi "$MIGRATE_IMAGE" >/dev/null 2>&1 || true
    rm -f "${MIGRATE_BINARY:-}"
    rm -rf "${TEST_MIGRATIONS_DIR:-}" 2>/dev/null || true
}
trap cleanup EXIT

# Ensure a clean slate also on re-runs: if a previous run was killed before
# its EXIT trap fired, a stale container/network would otherwise make
# "docker network create" and "docker run --name" fail silently below, and the
# script would keep running against leftover state (wrong or confusing results).
docker rm -f "$PG_CONTAINER" >/dev/null 2>&1 || true
docker rm -f migrate-test-run >/dev/null 2>&1 || true
docker network rm "$NETWORK" >/dev/null 2>&1 || true

# ---------- 0. prepare test migration files --------------------------------
TEST_MIGRATIONS_DIR=$(mktemp -d)
cat > "${TEST_MIGRATIONS_DIR}/20260906100000001_create_users.sql" <<'SQL'
CREATE TABLE users (
    id    SERIAL PRIMARY KEY,
    name  TEXT NOT NULL,
    email TEXT NOT NULL
);
SQL
cat > "${TEST_MIGRATIONS_DIR}/20260906100000002_add_phone.sql" <<'SQL'
ALTER TABLE users ADD COLUMN phone TEXT;
SQL

# ---------- 1. build the migrate binary via Docker --------------------------
printf "\n\033[1m── Building migrate image ──\033[0m\n"
docker build -t "$MIGRATE_IMAGE" . >/dev/null 2>&1
MIGRATE_CONTAINER=$(docker create "$MIGRATE_IMAGE")
MIGRATE_BINARY=$(mktemp)
docker cp "${MIGRATE_CONTAINER}:/migrate" "$MIGRATE_BINARY"
docker rm "$MIGRATE_CONTAINER" >/dev/null 2>&1
chmod +x "$MIGRATE_BINARY"

# ---------- 2. start PostgreSQL ---------------------------------------------
printf "\033[1m── Starting PostgreSQL ──\033[0m\n"
docker network create "$NETWORK" >/dev/null 2>&1
docker run -d --name "$PG_CONTAINER" --network "$NETWORK" \
    -e POSTGRES_USER=postgres \
    -e POSTGRES_PASSWORD=postgres \
    -e POSTGRES_DB=testdb \
    "$PG_IMAGE" >/dev/null 2>&1

printf "  waiting for postgres"
TRIES=0
until docker exec "$PG_CONTAINER" pg_isready -U postgres -q 2>/dev/null; do
    TRIES=$((TRIES + 1))
    if [ "$TRIES" -ge 30 ]; then
        printf " \033[31mtimeout\033[0m\n"
        docker logs "$PG_CONTAINER" >&2
        exit 1
    fi
    printf "."
    sleep 1
done
printf " ready\n"

# helper: run a command inside the postgres container
run() {
    docker exec "$PG_CONTAINER" "$@"
}

# helper: run the migrate tool via docker exec
migrate() {
    docker exec "$PG_CONTAINER" /tmp/migrate "$@"
}

# copy the binary and test migrations into the running postgres container
docker cp "$MIGRATE_BINARY" "${PG_CONTAINER}:/tmp/migrate"
docker exec "$PG_CONTAINER" chmod +x /tmp/migrate
docker exec "$PG_CONTAINER" mkdir -p /migrations
docker cp "$TEST_MIGRATIONS_DIR/." "${PG_CONTAINER}:/migrations"

# ==========================================================================
printf "\n\033[1m── Integration Tests ──\033[0m\n\n"

# ── up: apply migrations ---------------------------------------------------
printf "\033[1mup\033[0m\n"

OUT=$(migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations up 2>&1)
CODE=$?

if [ "$CODE" -ne 0 ]; then
    fail "up exit code" "expected 0, got $CODE"
    printf "    output:\n%s\n" "$OUT"
else
    pass "up exits 0"
fi

if echo "$OUT" | grep -q "migrated"; then
    pass "up prints 'migrated' for each file"
else
    fail "up output" "missing 'migrated' lines"
    printf "    output:\n%s\n" "$OUT"
fi

ROWS=$(run psql -U postgres -d testdb -tAc \
    "SELECT count(*) FROM test_migrations;" 2>&1)
if [ "$ROWS" = "2" ]; then
    pass "schema_migrations has 2 rows"
else
    fail "schema_migrations row count" "expected 2, got '${ROWS}'"
fi

TABLE=$(run psql -U postgres -d testdb -tAc \
    "SELECT count(*) FROM information_schema.tables WHERE table_name='users';" 2>&1)
if [ "$TABLE" = "1" ]; then
    pass "users table created"
else
    fail "users table" "not found"
fi

COLS=$(run psql -U postgres -d testdb -tAc \
    "SELECT count(*) FROM information_schema.columns WHERE table_name='users' AND column_name='phone';" 2>&1)
if [ "$COLS" = "1" ]; then
    pass "phone column added"
else
    fail "phone column" "not found"
fi

# ── up: idempotent re-run --------------------------------------------------
printf "\n\033[1mup (idempotent re-run)\033[0m\n"

migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations up >/dev/null 2>&1

ROWS=$(run psql -U postgres -d testdb -tAc \
    "SELECT count(*) FROM test_migrations;" 2>&1)
if [ "$ROWS" = "2" ]; then
    pass "idempotent re-run still 2 rows"
else
    fail "idempotent re-run" "expected 2 rows, got '${ROWS}'"
fi

# ── list --------------------------------------------------------------------
printf "\n\033[1mlist\033[0m\n"

OUT=$(migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations list 2>&1)

if echo "$OUT" | grep -q "20260906100000001"; then
    pass "list shows version 20260906100000001"
else
    fail "list" "missing version 20260906100000001"
    printf "    output:\n%s\n" "$OUT"
fi

if echo "$OUT" | grep -q "20260906100000002"; then
    pass "list shows version 20260906100000002"
else
    fail "list" "missing version 20260906100000002"
    printf "    output:\n%s\n" "$OUT"
fi

# ── last --------------------------------------------------------------------
printf "\n\033[1mlast\033[0m\n"

OUT=$(migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations last 2>&1)

if echo "$OUT" | grep -q "20260906100000002"; then
    pass "last shows latest version 20260906100000002"
else
    fail "last" "missing version 20260906100000002"
    printf "    output:\n%s\n" "$OUT"
fi

if ! echo "$OUT" | grep -q "20260906100000001"; then
    pass "last does not show older version"
else
    fail "last" "unexpectedly contains 20260906100000001"
    printf "    output:\n%s\n" "$OUT"
fi

# ── check -------------------------------------------------------------------
printf "\n\033[1mcheck\033[0m\n"

OUT=$(migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations check 2>&1)
CODE=$?

if [ "$CODE" -eq 0 ]; then
    pass "check clean exits 0"
else
    fail "check clean" "expected exit 0, got $CODE"
fi

if echo "$OUT" | grep -q "ok: no duplicate migration versions"; then
    pass "check prints ok message"
else
    fail "check output" "missing ok message"
    printf "    output:\n%s\n" "$OUT"
fi

# create a duplicate to test detection
cp "${TEST_MIGRATIONS_DIR}/20260906100000001_create_users.sql" \
   "${TEST_MIGRATIONS_DIR}/20260906100000001_create_users_dup.sql"
docker cp "${TEST_MIGRATIONS_DIR}/20260906100000001_create_users_dup.sql" \
    "${PG_CONTAINER}:/migrations/"

OUT=$(migrate --conn="postgres://postgres:postgres@localhost:5432/testdb?sslmode=disable" \
    --dir=/migrations --t=test_migrations check 2>&1)
CODE=$?

if [ "$CODE" -ne 0 ]; then
    pass "check duplicate exits non-zero"
else
    fail "check duplicate" "expected non-zero exit"
fi

if echo "$OUT" | grep -q "20260906100000001_create_users_dup.sql"; then
    pass "check reports duplicate file"
else
    fail "check duplicate output" "missing duplicate file name"
    printf "    output:\n%s\n" "$OUT"
fi

rm -f "${TEST_MIGRATIONS_DIR}/20260906100000001_create_users_dup.sql"
docker exec "$PG_CONTAINER" rm -f /migrations/20260906100000001_create_users_dup.sql

# ==========================================================================
printf "\n\033[1m── Results ──\033[0m\n\n"

TOTAL=$((PASS + FAIL))
printf "  %d/%d passed\n\n" "$PASS" "$TOTAL"

if [ "$FAIL" -gt 0 ]; then
    printf "\033[31mFailures:\033[0m\n"
    printf "$ERRORS"
    printf "\n"
    exit 1
fi

printf "\033[32m✔ Integration test passed\033[0m\n"
exit 0
