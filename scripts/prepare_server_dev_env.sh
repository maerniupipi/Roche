#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_FILE="${1:-$PROJECT_ROOT/.env.server-dev}"
PUBLIC_URL="${2:-}"
BASE_FILE="${3:-$PROJECT_ROOT/.env.server}"

if [[ ! "$PUBLIC_URL" =~ ^https?://[^/]+$ ]]; then
    echo "Public URL must be an HTTP(S) origin without a trailing slash." >&2
    exit 1
fi

if [[ ! -f "$OUTPUT_FILE" ]]; then
    if [[ -f "$BASE_FILE" ]]; then
        cp "$BASE_FILE" "$OUTPUT_FILE"
    else
        cp "$PROJECT_ROOT/.env.server-dev.example" "$OUTPUT_FILE"
    fi
fi

set_env() {
    local key="$1" value="$2" tmp
    tmp="$(mktemp)"
    awk -v key="$key" -v value="$value" '
        BEGIN { found = 0 }
        index($0, key "=") == 1 {
            print key "=" value
            found = 1
            next
        }
        { print }
        END {
            if (!found) {
                print key "=" value
            }
        }
    ' "$OUTPUT_FILE" > "$tmp"
    cat "$tmp" > "$OUTPUT_FILE"
    rm -f "$tmp"
}

# Preserve COMPOSE_PROJECT_NAME, DB_*, storage credentials and application
# secrets from the existing server file. Only development integration values
# are normalized here.
set_env SERVER_DEV_PUBLIC_URL "$PUBLIC_URL"
set_env GIN_MODE debug
set_env AUTH_PASSWORD_LOGIN_ENABLE true
set_env AUTH_REGISTRATION_ENABLE true
set_env AUTH_REGISTRATION_DEFAULT_ROLE viewer
set_env AUTH_REGISTRATION_DEV_ROLE_SELECTION true
set_env OIDC_AUTH_ENABLE false
set_env SAML_AUTH_ENABLE true
set_env SAML_AUTH_PROVIDER_DISPLAY_NAME '"Mock SAML"'
set_env SAML_AUTH_IDP_METADATA_URL ''
set_env SAML_AUTH_AUTO_PROVISION true
set_env SAML_AUTH_SIGN_REQUEST false
set_env SAML_AUTH_ALLOW_EPHEMERAL_CERT true
set_env SAML_INTERNAL_SP_ENTITY_ID urn:rochekap:internal:sp
set_env SAML_EXTERNAL_SP_ENTITY_ID urn:rochekap:external:sp
set_env SAML_INTERNAL_SP_CERT_FILE ''
set_env SAML_INTERNAL_SP_KEY_FILE ''
set_env SAML_EXTERNAL_SP_CERT_FILE ''
set_env SAML_EXTERNAL_SP_KEY_FILE ''
set_env MOCK_SAML_USERNAME admin
set_env MOCK_SAML_PASSWORD 'Admin123!'
set_env MOCK_SAML_EMAIL admin@rochekap.local
set_env MOCK_SAML_DEVELOPER_COUNT 100
set_env MOCK_SAML_DEVELOPER_PASSWORD 'Dev12345!'
set_env MOCK_SAML_DEVELOPER_EMAIL_DOMAIN rochekap.local
set_env WORKDAY_ENABLE true
set_env WORKDAY_PROVIDER mock
set_env WORKDAY_CONNECTION_KEY server-dev-mock
set_env WORKDAY_MOCK_FILE config/mock/workday.json
set_env SERVER_DEV_PROFILES '"minio langfuse neo4j milvus mock-saml"'

chmod 600 "$OUTPUT_FILE"
echo "Prepared $OUTPUT_FILE for server development at $PUBLIC_URL"
