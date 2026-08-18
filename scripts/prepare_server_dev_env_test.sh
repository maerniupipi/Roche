#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$PROJECT_ROOT/scripts/prepare_server_dev_env.sh"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

BASE_FILE="$TEST_ROOT/.env.server"
OUTPUT_FILE="$TEST_ROOT/.env.server-dev"

cat > "$BASE_FILE" <<'EOF'
COMPOSE_PROJECT_NAME=existing-project
DB_NAME=ExistingDB
DB_PASSWORD=existing-secret
SAML_AUTH_IDP_METADATA_URL=https://ping.example/metadata
WORKDAY_ENABLE=false
WORKDAY_PROVIDER=mulesoft
EOF

bash "$SCRIPT" "$OUTPUT_FILE" "http://10.3.97.217" "$BASE_FILE"

grep -Fqx 'COMPOSE_PROJECT_NAME=existing-project' "$OUTPUT_FILE"
grep -Fqx 'DB_NAME=ExistingDB' "$OUTPUT_FILE"
grep -Fqx 'DB_PASSWORD=existing-secret' "$OUTPUT_FILE"
grep -Fqx 'SERVER_DEV_PUBLIC_URL=http://10.3.97.217' "$OUTPUT_FILE"
grep -Fqx 'SAML_AUTH_IDP_METADATA_URL=' "$OUTPUT_FILE"
grep -Fqx 'WORKDAY_ENABLE=true' "$OUTPUT_FILE"
grep -Fqx 'WORKDAY_PROVIDER=mock' "$OUTPUT_FILE"
grep -Fqx 'SERVER_DEV_PROFILES="minio langfuse neo4j milvus mock-saml"' "$OUTPUT_FILE"

echo "server development environment preparation test passed"
