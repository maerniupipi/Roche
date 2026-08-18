#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMAND="${1:-up}"
ENV_FILE="${SERVER_DEV_ENV_FILE:-$PROJECT_ROOT/.env.server-dev}"
export SERVER_ENV_FILE="$ENV_FILE"
export DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}"
export COMPOSE_DOCKER_CLI_BUILD="${COMPOSE_DOCKER_CLI_BUILD:-1}"

if docker compose version >/dev/null 2>&1; then
    COMPOSE=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
else
    echo "Docker Compose is required. Install the Docker Compose v2 plugin first." >&2
    exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
    echo "Missing $ENV_FILE" >&2
    echo "Create it from .env.server-dev.example before starting server development." >&2
    exit 1
fi

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

PUBLIC_URL="${SERVER_DEV_PUBLIC_URL:-}"
if [[ ! "$PUBLIC_URL" =~ ^https?://[^/]+$ ]] || [[ "$PUBLIC_URL" == *"SERVER_IP_OR_HOST"* ]]; then
    echo "SERVER_DEV_PUBLIC_URL must be a browser-reachable URL without a trailing slash." >&2
    echo "Example: SERVER_DEV_PUBLIC_URL=http://192.168.10.25" >&2
    exit 1
fi

PUBLIC_URL="${PUBLIC_URL%/}"
if [[ "$PUBLIC_URL" =~ ^(https?://)([^/:]+)(:[0-9]+)?$ ]]; then
    URL_SCHEME="${BASH_REMATCH[1]}"
    URL_HOST="${BASH_REMATCH[2]}"
else
    echo "SERVER_DEV_PUBLIC_URL must use a hostname or IPv4 address." >&2
    exit 1
fi

MOCK_SAML_PORT="${MOCK_SAML_PORT:-8091}"
APP_PORT="${APP_PORT:-8080}"
GATEWAY_INTERNAL_PORT="${GATEWAY_INTERNAL_PORT:-8088}"
GATEWAY_EXTERNAL_PORT="${GATEWAY_EXTERNAL_PORT:-8089}"
GATEWAY_INTERNAL_URL="${URL_SCHEME}${URL_HOST}:${GATEWAY_INTERNAL_PORT}"
GATEWAY_EXTERNAL_URL="${URL_SCHEME}${URL_HOST}:${GATEWAY_EXTERNAL_PORT}"
export MOCK_SAML_PUBLIC_URL="${MOCK_SAML_PUBLIC_URL:-http://${URL_HOST}:${MOCK_SAML_PORT}}"
export OIDC_AUTH_ENABLE=false
export SAML_AUTH_ENABLE=true
export SAML_AUTH_IDP_METADATA_URL="${SAML_AUTH_IDP_METADATA_URL:-http://mock-saml-idp:8091/metadata}"
export SAML_AUTH_PROVIDER_DISPLAY_NAME="${SAML_AUTH_PROVIDER_DISPLAY_NAME:-Mock SAML}"
export SAML_AUTH_AUTO_PROVISION="${SAML_AUTH_AUTO_PROVISION:-true}"
# The development IdP accepts unsigned AuthnRequests. Production keeps its
# independent strict setting in .env.production.
export SAML_AUTH_SIGN_REQUEST="${SAML_AUTH_SIGN_REQUEST:-false}"
export AUTH_INTERNAL_ALLOWED_ORIGINS="${AUTH_INTERNAL_ALLOWED_ORIGINS:-$GATEWAY_INTERNAL_URL}"
export AUTH_EXTERNAL_ALLOWED_ORIGINS="${AUTH_EXTERNAL_ALLOWED_ORIGINS:-$GATEWAY_EXTERNAL_URL}"
export AUTH_INTERNAL_ALLOWED_REDIRECT_URIS="${AUTH_INTERNAL_ALLOWED_REDIRECT_URIS:-$GATEWAY_INTERNAL_URL/,$GATEWAY_INTERNAL_URL/default/,$GATEWAY_INTERNAL_URL/admin/,$GATEWAY_INTERNAL_URL/app/}"
export AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS="${AUTH_EXTERNAL_ALLOWED_REDIRECT_URIS:-$GATEWAY_EXTERNAL_URL/,$GATEWAY_EXTERNAL_URL/default/,$GATEWAY_EXTERNAL_URL/admin/,$GATEWAY_EXTERNAL_URL/app/}"
export SAML_INTERNAL_ACS_URL="${SAML_INTERNAL_ACS_URL:-$GATEWAY_INTERNAL_URL/api/v1/auth/saml/acs}"
export SAML_EXTERNAL_ACS_URL="${SAML_EXTERNAL_ACS_URL:-$GATEWAY_EXTERNAL_URL/api/v1/auth/saml/acs}"
export SSRF_WHITELIST_EXTRA="${SSRF_WHITELIST_EXTRA:+${SSRF_WHITELIST_EXTRA},}${URL_HOST},mock-saml-idp,searxng,qdrant,milvus,weaviate,doris-fe"
export LANGFUSE_NEXTAUTH_URL="${SERVER_DEV_LANGFUSE_URL:-http://${URL_HOST}:${LANGFUSE_WEB_PORT:-3000}}"
export LANGFUSE_S3_MEDIA_UPLOAD_ENDPOINT="${SERVER_DEV_LANGFUSE_S3_URL:-http://${URL_HOST}:${LANGFUSE_MINIO_S3_PORT:-9100}}"

# The server development environment starts its standard infrastructure and
# both enterprise mocks by default.
# Override with a space-separated list, for example
# SERVER_DEV_PROFILES="milvus neo4j mock-saml".
read -r -a profiles <<< "${SERVER_DEV_PROFILES:-minio langfuse neo4j milvus mock-saml}"
compose_args=(
    --project-name "${COMPOSE_PROJECT_NAME:-rochekap-server-dev}"
    --env-file "$ENV_FILE"
    -f docker-compose.server-dev.yml
)
for profile in "${profiles[@]}"; do
    compose_args+=(--profile "$profile")
done

compose() {
    "${COMPOSE[@]}" "${compose_args[@]}" "$@"
}

warn_if_no_registry_mirror() {
    local mirrors
    mirrors="$(docker info --format '{{json .RegistryConfig.Mirrors}}' 2>/dev/null || true)"
    if [[ -z "$mirrors" || "$mirrors" == "null" || "$mirrors" == "[]" ]]; then
        echo "WARNING: Docker daemon has no registry mirror configured." >&2
        echo "Docker Hub image pulls may be slow in mainland China; see docs/development/deployment-modes.md." >&2
    fi
}

cd "$PROJECT_ROOT"

case "$COMMAND" in
    up)
        warn_if_no_registry_mirror
        compose up -d --build
        ;;
    update)
        warn_if_no_registry_mirror
        git pull --ff-only
        compose up -d --build --force-recreate
        ;;
    ci-update)
        # CI has already checked out the exact commit on the deployment host.
        # Start any missing infrastructure first, then restart only the
        # source-mounted application tier so persistent data services keep
        # their containers and named volumes intact.
        warn_if_no_registry_mirror
        compose up -d --build
        # Keep dependency ordering enabled here. In particular, both Gateway
        # instances must wait until the source-built frontend is healthy.
        compose up -d --force-recreate \
            app \
            docreader \
            frontend \
            auth-service-internal \
            auth-service-external \
            api-gateway-internal \
            api-gateway-external \
            mock-saml-idp
        ;;
    down)
        compose down --remove-orphans
        ;;
    status)
        compose ps
        ;;
    config)
        compose config --quiet
        ;;
    logs)
        shift || true
        compose logs -f --tail 200 "$@"
        ;;
    *)
        echo "Usage: $0 {up|update|ci-update|down|status|config|logs [service...]}" >&2
        exit 2
        ;;
esac
