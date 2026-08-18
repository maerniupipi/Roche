#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir -p \
    "$TEST_ROOT/project/scripts" \
    "$TEST_ROOT/project/migrations/versioned" \
    "$TEST_ROOT/bin"
cp "$SCRIPT_DIR/migrate.sh" "$TEST_ROOT/project/scripts/migrate.sh"

cat > "$TEST_ROOT/bin/migrate" <<'EOF'
#!/bin/bash
printf '%s\n' "$@" > "$MIGRATE_ARGS_FILE"
EOF
chmod +x "$TEST_ROOT/bin/migrate"

password="pa ss!@#'\$"
export MIGRATE_ARGS_FILE="$TEST_ROOT/migrate-args"

output=$(
    PATH="$TEST_ROOT/bin:$PATH" \
    DB_URL='' \
    DB_USER='ops user' \
    DB_PASSWORD="$password" \
    DB_HOST='db.internal' \
    DB_PORT='5432' \
    DB_NAME='knowledge' \
    MIGRATIONS_DIR="$TEST_ROOT/project/migrations/versioned" \
    "$TEST_ROOT/project/scripts/migrate.sh" up
)

if [[ "$output" == *"$password"* ]]; then
    echo "migration output leaked the plaintext database password" >&2
    exit 1
fi
if [[ "$output" == *"postgres://"* ]]; then
    echo "migration output leaked the database URL" >&2
    exit 1
fi

mapfile -t args < "$MIGRATE_ARGS_FILE"
expected_url='postgres://ops%20user:pa%20ss%21%40%23%27%24@db.internal:5432/knowledge?sslmode=disable'
if [[ "${args[3]:-}" != "$expected_url" ]]; then
    echo "migration URL encoding changed unexpectedly" >&2
    exit 1
fi

echo "migration credential redaction test passed"
