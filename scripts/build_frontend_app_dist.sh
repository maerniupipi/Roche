#!/usr/bin/env bash
# Wrapper to build the frontend-app bundle using the shared script.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/build_frontend_dist.sh" frontend-app
