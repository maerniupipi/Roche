#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_UNDER_TEST="$PROJECT_ROOT/scripts/production_server.sh"
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
    printf 'registration=%s\n' "${AUTH_REGISTRATION_ENABLE:-unset}"
    printf 'role_selection=%s\n' "${AUTH_REGISTRATION_DEV_ROLE_SELECTION:-unset}"
    printf 'password_login=%s\n' "${AUTH_PASSWORD_LOGIN_ENABLE:-unset}"
    exit 0
fi
echo "Unexpected docker invocation: $*" >&2
exit 1
FAKE_DOCKER
chmod +x "$FAKE_BIN/docker"

printf 'internal-cert' > "$TEST_ROOT/internal.crt"
printf 'internal-key' > "$TEST_ROOT/internal.key"
printf 'external-cert' > "$TEST_ROOT/external.crt"
printf 'external-key' > "$TEST_ROOT/external.key"

VALID_ENV="$TEST_ROOT/production.env"
cat > "$VALID_ENV" <<EOF
COMPOSE_PROJECT_NAME=rochekap-production
APP_IMAGE=rochekap/app:test-sha
FRONTEND_IMAGE=rochekap/frontend:test-sha
DOCREADER_IMAGE=rochekap/docreader:test-sha
AUTH_SERVICE_IMAGE=rochekap/auth-service:test-sha
API_GATEWAY_IMAGE=rochekap/api-gateway:test-sha
AUTH_REGISTRATION_ENABLE=false
AUTH_REGISTRATION_DEV_ROLE_SELECTION=false
AUTH_PASSWORD_LOGIN_ENABLE=false
AUTH_INTERNAL_SERVICE_SECRET=internal-0123456789012345678901234567890123456789
AUTH_EXTERNAL_SERVICE_SECRET=external-0123456789012345678901234567890123456789
AUTH_REFRESH_COOKIE_SECURE=true
AUTH_INTERNAL_ALLOWED_REDIRECT_URIS=https://internal.example.test/
AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS=https://external.example.test/
OIDC_AUTH_ENABLE=false
SAML_AUTH_ENABLE=true
SAML_AUTH_ALLOW_EPHEMERAL_CERT=false
SAML_AUTH_IDP_METADATA_URL=https://ping.example.test/metadata
SAML_INTERNAL_SP_ENTITY_ID=urn:test:internal
SAML_INTERNAL_ACS_URL=https://internal.example.test/api/v1/auth/saml/acs
SAML_EXTERNAL_SP_ENTITY_ID=urn:test:external
SAML_EXTERNAL_ACS_URL=https://external.example.test/api/v1/auth/saml/acs
SAML_INTERNAL_SP_CERT_HOST_PATH=$TEST_ROOT/internal.crt
SAML_INTERNAL_SP_KEY_HOST_PATH=$TEST_ROOT/internal.key
SAML_EXTERNAL_SP_CERT_HOST_PATH=$TEST_ROOT/external.crt
SAML_EXTERNAL_SP_KEY_HOST_PATH=$TEST_ROOT/external.key
SYSTEM_AES_KEY=0123456789abcdef0123456789abcdef
LANGFUSE_ENCRYPTION_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
WORKDAY_ENABLE=false
PRODUCTION_PROFILES=minio
EOF

run_script() {
    PATH="$FAKE_BIN:$PATH" PRODUCTION_ENV_FILE="$1" bash "$SCRIPT_UNDER_TEST" config 2>&1
}

output="$(run_script "$VALID_ENV")"
grep -Fq -- "--project-name rochekap-production" <<< "$output"
grep -Fq "registration=false" <<< "$output"
grep -Fq "role_selection=false" <<< "$output"
grep -Fq "password_login=false" <<< "$output"
echo "production SAML Auth Service contract test passed"

LATEST_ENV="$TEST_ROOT/latest.env"
sed 's#AUTH_SERVICE_IMAGE=rochekap/auth-service:test-sha#AUTH_SERVICE_IMAGE=rochekap/auth-service:latest#' \
    "$VALID_ENV" > "$LATEST_ENV"
if run_script "$LATEST_ENV" >"$TEST_ROOT/latest.out"; then
    echo "Expected a latest-tagged Auth Service image to be rejected." >&2
    exit 1
fi
grep -Fq "AUTH_SERVICE_IMAGE must use an explicit immutable tag" "$TEST_ROOT/latest.out"
echo "production immutable image enforcement test passed"

OIDC_ENV="$TEST_ROOT/oidc.env"
sed 's/^OIDC_AUTH_ENABLE=false$/OIDC_AUTH_ENABLE=true/' "$VALID_ENV" > "$OIDC_ENV"
if run_script "$OIDC_ENV" >"$TEST_ROOT/oidc.out"; then
    echo "Expected OIDC to be rejected by the PingIdentity SAML production profile." >&2
    exit 1
fi
grep -Fq "OIDC_AUTH_ENABLE must be false" "$TEST_ROOT/oidc.out"
echo "production SAML-only enforcement test passed"

DEV_ADMIN_ENV="$TEST_ROOT/dev-admin.env"
sed '/^SAML_AUTH_ENABLE=true$/a SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS=developer001@example.test' \
    "$VALID_ENV" > "$DEV_ADMIN_ENV"
if run_script "$DEV_ADMIN_ENV" >"$TEST_ROOT/dev-admin.out"; then
    echo "Expected development SAML administrator bootstrap to be rejected in production." >&2
    exit 1
fi
grep -Fq "SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS must be empty" "$TEST_ROOT/dev-admin.out"
echo "production development-role bootstrap enforcement test passed"

MISSING_KEY_ENV="$TEST_ROOT/missing-key.env"
sed "s#SAML_EXTERNAL_SP_KEY_HOST_PATH=$TEST_ROOT/external.key#SAML_EXTERNAL_SP_KEY_HOST_PATH=$TEST_ROOT/missing.key#" \
    "$VALID_ENV" > "$MISSING_KEY_ENV"
if run_script "$MISSING_KEY_ENV" >"$TEST_ROOT/missing-key.out"; then
    echo "Expected a missing external SAML key to be rejected." >&2
    exit 1
fi
grep -Fq "SAML_EXTERNAL_SP_KEY_HOST_PATH must reference" "$TEST_ROOT/missing-key.out"
echo "production zone-specific SAML key enforcement test passed"
