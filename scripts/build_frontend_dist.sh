#!/usr/bin/env bash
# Build a frontend static asset bundle for Docker / release packaging.
# Usage: build_frontend_dist.sh [frontend|frontend-default|frontend-admin|frontend-app]
# Default target is "frontend" for backward compatibility.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Target directory name under PROJECT_ROOT. Accepts: frontend | frontend-default | frontend-admin | frontend-app.
TARGET_DIR="${1:-frontend}"
case "$TARGET_DIR" in
	frontend|frontend-default|frontend-admin|frontend-app) ;;
	*)
		echo "ERROR: unsupported target '$TARGET_DIR' (expected: frontend | frontend-default | frontend-admin | frontend-app)" >&2
		exit 1
		;;
esac

if [ -z "${VITE_FRONTEND_COMMIT:-}" ]; then
	# shellcheck source=/dev/null
	eval "$("$PROJECT_ROOT/scripts/get_version.sh" env)"
	export VITE_FRONTEND_COMMIT="${COMMIT_ID:-unknown}"
fi

export VITE_IS_DOCKER="${VITE_IS_DOCKER:-true}"

cd "$PROJECT_ROOT/${TARGET_DIR}"
if [ -f package-lock.json ]; then
	npm ci
else
	npm install --no-package-lock
fi
npm run build
