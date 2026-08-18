#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMAND="${1:-up}"
ENV_FILE="${PRODUCTION_ENV_FILE:-$PROJECT_ROOT/.env.production}"
export SERVER_ENV_FILE="$ENV_FILE"

if docker compose version >/dev/null 2>&1; then
    COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
else
    echo "Docker Compose is required." >&2
    exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Missing $ENV_FILE" >&2
    echo "Create it from .env.production.example and replace every CHANGE_ME value." >&2
    exit 1
fi

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

# Production identity is owned by PingIdentity SAML through Auth Service.
# Password registration and development role selection are always disabled.
AUTH_REGISTRATION_ENABLE=false
AUTH_REGISTRATION_DEV_ROLE_SELECTION=false
AUTH_PASSWORD_LOGIN_ENABLE=false
export AUTH_REGISTRATION_ENABLE AUTH_REGISTRATION_DEV_ROLE_SELECTION AUTH_PASSWORD_LOGIN_ENABLE

# CI builds immutable images on the deployment host. Allow it to override only
# the application image references while keeping every other production
# setting and validation rule sourced from the selected environment file.
APP_IMAGE="${PRODUCTION_APP_IMAGE:-${APP_IMAGE:-}}"
FRONTEND_IMAGE="${PRODUCTION_FRONTEND_IMAGE:-${FRONTEND_IMAGE:-}}"
DOCREADER_IMAGE="${PRODUCTION_DOCREADER_IMAGE:-${DOCREADER_IMAGE:-}}"
AUTH_SERVICE_IMAGE="${PRODUCTION_AUTH_SERVICE_IMAGE:-${AUTH_SERVICE_IMAGE:-}}"
API_GATEWAY_IMAGE="${PRODUCTION_API_GATEWAY_IMAGE:-${API_GATEWAY_IMAGE:-}}"
export APP_IMAGE FRONTEND_IMAGE DOCREADER_IMAGE AUTH_SERVICE_IMAGE API_GATEWAY_IMAGE

validate_production_environment() {
    local image image_var profile_list internal_auth_secret external_auth_secret
    if grep -Eq '^[A-Z_][A-Z0-9_]*=.*CHANGE_ME' "$ENV_FILE"; then
        echo "Production environment still contains CHANGE_ME placeholders." >&2
        return 1
    fi

    for image_var in APP_IMAGE FRONTEND_IMAGE DOCREADER_IMAGE AUTH_SERVICE_IMAGE API_GATEWAY_IMAGE; do
        image="${!image_var:-}"
        if [[ -z "$image" || "$image" != *:* || "$image" == *":latest" ]]; then
            echo "$image_var must use an explicit immutable tag, never latest." >&2
            return 1
        fi
    done

    if [[ "${AUTH_REGISTRATION_ENABLE:-}" != "false" ]]; then
        echo "AUTH_REGISTRATION_ENABLE must be false in production." >&2
        return 1
    fi
    if [[ "${AUTH_PASSWORD_LOGIN_ENABLE:-}" != "false" ]]; then
        echo "AUTH_PASSWORD_LOGIN_ENABLE must be false in production." >&2
        return 1
    fi
    if [[ "${AUTH_REGISTRATION_DEV_ROLE_SELECTION:-}" != "false" ]]; then
        echo "AUTH_REGISTRATION_DEV_ROLE_SELECTION must be false in production." >&2
        return 1
    fi
    if [[ -n "${SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS:-}" ]]; then
        echo "SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS must be empty in production." >&2
        return 1
    fi
    if [[ "${SAML_AUTH_ENABLE:-}" != "true" ]]; then
        echo "SAML_AUTH_ENABLE must be true in production." >&2
        return 1
    fi
    if [[ "${OIDC_AUTH_ENABLE:-false}" == "true" ]]; then
        echo "OIDC_AUTH_ENABLE must be false for the PingIdentity SAML production profile." >&2
        return 1
    fi
    internal_auth_secret="${AUTH_INTERNAL_SERVICE_SECRET:-}"
    external_auth_secret="${AUTH_EXTERNAL_SERVICE_SECRET:-}"
    if [[ ${#internal_auth_secret} -lt 48 || ${#external_auth_secret} -lt 48 ]]; then
        echo "Both zone-specific Auth Service secrets must contain at least 48 characters." >&2
        return 1
    fi
    if [[ "$internal_auth_secret" == "$external_auth_secret" ]]; then
        echo "Internal and external Gateway-to-Auth secrets must be different." >&2
        return 1
    fi
    if [[ "${AUTH_REFRESH_COOKIE_SECURE:-}" != "true" ]]; then
        echo "AUTH_REFRESH_COOKIE_SECURE must be true in production." >&2
        return 1
    fi
    if [[ "${SAML_AUTH_ALLOW_EPHEMERAL_CERT:-false}" == "true" ]]; then
        echo "Ephemeral SAML SP certificates are forbidden in production." >&2
        return 1
    fi
    if [[ "${WORKDAY_ENABLE:-false}" == "true" ]]; then
        if [[ "${WORKDAY_PROVIDER:-}" == "mock" ]]; then
            echo "WORKDAY_PROVIDER=mock is forbidden in production." >&2
            return 1
        fi
        if [[ ! "${WORKDAY_BASE_URL:-}" =~ ^https:// ]]; then
            echo "WORKDAY_BASE_URL must use HTTPS when Workday is enabled in production." >&2
            return 1
        fi
    fi
    if [[ ${#SYSTEM_AES_KEY} -ne 32 ]]; then
        echo "SYSTEM_AES_KEY must be exactly 32 ASCII characters." >&2
        return 1
    fi
    if [[ ! "${LANGFUSE_ENCRYPTION_KEY:-}" =~ ^[0-9a-fA-F]{64}$ ]]; then
        echo "LANGFUSE_ENCRYPTION_KEY must be exactly 64 hexadecimal characters." >&2
        return 1
    fi
    if [[ ! "${SAML_AUTH_IDP_METADATA_URL:-}" =~ ^https:// ]]; then
        echo "SAML_AUTH_IDP_METADATA_URL must use HTTPS in production." >&2
        return 1
    fi
    if [[ ! "${SAML_INTERNAL_ACS_URL:-}" =~ ^https:// || ! "${SAML_EXTERNAL_ACS_URL:-}" =~ ^https:// ]]; then
        echo "Both SAML ACS URLs must use HTTPS in production." >&2
        return 1
    fi
    if [[ -z "${SAML_INTERNAL_SP_ENTITY_ID:-}" || -z "${SAML_EXTERNAL_SP_ENTITY_ID:-}" ]]; then
        echo "Both internal and external SAML SP entity IDs are required." >&2
        return 1
    fi
    if [[ "${SAML_INTERNAL_SP_ENTITY_ID}" == "${SAML_EXTERNAL_SP_ENTITY_ID}" ]]; then
        echo "Internal and external SAML SP entity IDs must be different." >&2
        return 1
    fi
    if [[ "${SAML_INTERNAL_ACS_URL}" == "${SAML_EXTERNAL_ACS_URL}" ]]; then
        echo "Internal and external SAML ACS URLs must be different." >&2
        return 1
    fi
    local saml_file_var saml_file
    for saml_file_var in \
        SAML_INTERNAL_SP_CERT_HOST_PATH SAML_INTERNAL_SP_KEY_HOST_PATH \
        SAML_EXTERNAL_SP_CERT_HOST_PATH SAML_EXTERNAL_SP_KEY_HOST_PATH; do
        saml_file="${!saml_file_var:-}"
        if [[ -z "$saml_file" || ! -f "$saml_file" ]]; then
            echo "$saml_file_var must reference a stable zone-specific SAML file." >&2
            return 1
        fi
    done
    if [[ "${SAML_INTERNAL_SP_CERT_HOST_PATH}" == "${SAML_EXTERNAL_SP_CERT_HOST_PATH}" || \
          "${SAML_INTERNAL_SP_KEY_HOST_PATH}" == "${SAML_EXTERNAL_SP_KEY_HOST_PATH}" ]]; then
        echo "Internal and external SAML SP certificate/key paths must be different." >&2
        return 1
    fi
    if cmp -s "${SAML_INTERNAL_SP_CERT_HOST_PATH}" "${SAML_EXTERNAL_SP_CERT_HOST_PATH}" || \
       cmp -s "${SAML_INTERNAL_SP_KEY_HOST_PATH}" "${SAML_EXTERNAL_SP_KEY_HOST_PATH}"; then
        echo "Internal and external SAML SP certificate/key material must be different." >&2
        return 1
    fi
    if [[ -z "${AUTH_INTERNAL_ALLOWED_REDIRECT_URIS:-}" || -z "${AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS:-}" ]]; then
        echo "Both Auth Service redirect URI allowlists are required." >&2
        return 1
    fi
    profile_list=" ${PRODUCTION_PROFILES:-minio langfuse neo4j milvus} "
    if [[ "$profile_list" == *" dex "* || "$profile_list" == *" mock-saml "* || "$profile_list" == *" full "* ]]; then
        echo "Production profiles must not include dex, mock-saml or full." >&2
        return 1
    fi
}

validate_production_environment

read -r -a profiles <<< "${PRODUCTION_PROFILES:-minio langfuse neo4j milvus}"
compose_args=(
    --project-name "${COMPOSE_PROJECT_NAME:-rochekap}"
    --env-file "$ENV_FILE"
    -f docker-compose.server-dev.yml
    -f docker-compose.production.yml
)
for profile in "${profiles[@]}"; do
    compose_args+=(--profile "$profile")
done

compose() {
    "${COMPOSE[@]}" "${compose_args[@]}" "$@"
}

cd "$PROJECT_ROOT"

case "$COMMAND" in
    up)
        compose pull
        compose up -d --no-build
        ;;
    update)
        compose pull
        compose up -d --no-build --force-recreate
        ;;
    ci-update)
        compose up -d --no-build --no-deps --force-recreate app docreader frontend auth-service-internal auth-service-external api-gateway-internal api-gateway-external
        ;;
    down)
        compose down --remove-orphans
        ;;
    status)
        compose ps
        ;;
    logs)
        shift || true
        compose logs -f --tail 200 "$@"
        ;;
    config)
        compose config --quiet
        ;;
    *)
        echo "Usage: $0 {up|update|ci-update|down|status|config|logs [service...]}" >&2
        exit 2
        ;;
esac
