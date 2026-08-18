#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_UNDER_TEST="$PROJECT_ROOT/scripts/server_dev.sh"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_BIN="$TEST_ROOT/bin"
mkdir -p "$FAKE_BIN"
cat > "$FAKE_BIN/docker" <<'FAKE_DOCKER'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "compose" && "${2:-}" == "version" ]]; then
    exit 0
fi
if [[ "${1:-}" == "compose" ]]; then
    printf 'compose arguments=%s\n' "$*"
    printf 'mock_saml_public_url=%s\n' "${MOCK_SAML_PUBLIC_URL:-unset}"
    printf 'saml_metadata_url=%s\n' "${SAML_AUTH_IDP_METADATA_URL:-unset}"
    printf 'internal_redirects=%s\n' "${AUTH_INTERNAL_ALLOWED_REDIRECT_URIS:-unset}"
    printf 'external_redirects=%s\n' "${AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS:-unset}"
    printf 'internal_acs=%s\n' "${SAML_INTERNAL_ACS_URL:-unset}"
    printf 'external_acs=%s\n' "${SAML_EXTERNAL_ACS_URL:-unset}"
    exit 0
fi
echo "Unexpected docker invocation: $*" >&2
exit 1
FAKE_DOCKER
chmod +x "$FAKE_BIN/docker"

ENV_FILE="$TEST_ROOT/server-dev.env"
cat > "$ENV_FILE" <<'EOF'
SERVER_DEV_PUBLIC_URL=http://10.20.30.40
COMPOSE_PROJECT_NAME=rochekap-server-dev-test
MOCK_SAML_PORT=8091
GATEWAY_INTERNAL_PORT=8088
GATEWAY_EXTERNAL_PORT=8089
AUTH_INTERNAL_ALLOWED_ORIGINS=
AUTH_EXTERNAL_ALLOWED_ORIGINS=
AUTH_INTERNAL_ALLOWED_REDIRECT_URIS=
AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS=
SAML_INTERNAL_ACS_URL=
SAML_EXTERNAL_ACS_URL=
EOF

output="$({
    PATH="$FAKE_BIN:$PATH" \
    SERVER_DEV_ENV_FILE="$ENV_FILE" \
    SERVER_DEV_RENDER_DIR="$TEST_ROOT/rendered" \
        bash "$SCRIPT_UNDER_TEST" config
} 2>&1)"

grep -Fq 'mock_saml_public_url=http://10.20.30.40:8091' <<< "$output"
grep -Fq 'saml_metadata_url=http://mock-saml-idp:8091/metadata' <<< "$output"
grep -Fq 'internal_redirects=http://10.20.30.40:8088/,http://10.20.30.40:8088/default/,http://10.20.30.40:8088/admin/,http://10.20.30.40:8088/app/' <<< "$output"
grep -Fq 'external_redirects=http://10.20.30.40:8089/,http://10.20.30.40:8089/default/,http://10.20.30.40:8089/admin/,http://10.20.30.40:8089/app/' <<< "$output"
grep -Fq 'internal_acs=http://10.20.30.40:8088/api/v1/auth/saml/acs' <<< "$output"
grep -Fq 'external_acs=http://10.20.30.40:8089/api/v1/auth/saml/acs' <<< "$output"

# App and both zone-specific Auth Service deployments must receive the
# deployment-managed SSRF allowlist. Without it, Auth Service reports SAML as
# enabled but cannot fetch the bundled Mock IdP metadata.
test "$(grep -c 'SSRF_WHITELIST_EXTRA=${SSRF_WHITELIST_EXTRA:-' "$PROJECT_ROOT/docker-compose.server-dev.yml")" -eq 3

ci_output="$({
    PATH="$FAKE_BIN:$PATH" \
    SERVER_DEV_ENV_FILE="$ENV_FILE" \
        bash "$SCRIPT_UNDER_TEST" ci-update
} 2>&1)"

grep -Fq 'up -d --build' <<< "$ci_output"
grep -Fq 'up -d --force-recreate app docreader frontend auth-service-internal auth-service-external api-gateway-internal api-gateway-external mock-saml-idp' <<< "$ci_output"

# Gateways must not start while the source-mounted frontend is still running
# npm installs/builds. The frontend health endpoint is served only by nginx,
# after every bundle has completed successfully.
grep -Fq 'test: ["CMD", "curl", "-fsS", "http://localhost/health"]' "$PROJECT_ROOT/docker-compose.server-dev.yml"
test "$(grep -c 'frontend:' "$PROJECT_ROOT/docker-compose.server-dev.yml")" -ge 3
test "$(grep -c 'condition: service_healthy' "$PROJECT_ROOT/docker-compose.server-dev.yml")" -ge 8

echo "server development Mock SAML/Auth/Gateway contract test passed"
