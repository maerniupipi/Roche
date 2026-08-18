#!/bin/bash
set -e

# Get the script directory and project root
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Percent-encode database credentials inside Bash instead of interpolating the
# plaintext password into a Python `-c` command. LC_ALL=C makes Bash iterate
# over UTF-8 bytes so every non-URL-safe byte is encoded deterministically.
urlencode() {
    local LC_ALL=C
    local input="$1"
    local output=""
    local char encoded
    local i

    for ((i = 0; i < ${#input}; i++)); do
        char="${input:i:1}"
        case "$char" in
            [a-zA-Z0-9.~_-]) output+="$char" ;;
            *)
                printf -v encoded '%%%02X' "'$char"
                output+="$encoded"
                ;;
        esac
    done

    printf '%s' "$output"
}

# Local migrations use .env.local by default. Set ENV_FILE=.env.server-dev or
# ENV_FILE=.env.production for the corresponding server environment.
ENV_FILE="${ENV_FILE:-$PROJECT_ROOT/.env.local}"
if [ -f "$ENV_FILE" ]; then
    echo "Loading environment file from $ENV_FILE"
    set -a
    source "$ENV_FILE"
    set +a
fi

# Database connection details (can be overridden by environment variables)
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres}
DB_NAME=${DB_NAME:-RocheKAP}

# Use versioned migrations directory
MIGRATIONS_DIR="${MIGRATIONS_DIR:-migrations/versioned}"

# Check if migrate tool is installed
if ! command -v migrate &> /dev/null; then
    echo "Error: migrate tool is not installed"
    echo "Install it with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1"
    exit 1
fi

# Construct the database URL. Vector data is stored in Milvus; PostgreSQL
# migrations contain only relational application data.
if [ -n "$DB_URL" ]; then
    # If DB_URL already exists, ensure sslmode=disable is set (unless sslmode is already specified)
    if [[ "$DB_URL" != *"sslmode="* ]]; then
        # Add sslmode=disable if not present
        if [[ "$DB_URL" == *"?"* ]]; then
            DB_URL="${DB_URL}&sslmode=disable"
        else
            DB_URL="${DB_URL}?sslmode=disable"
        fi
    elif [[ "$DB_URL" == *"sslmode=require"* ]] || [[ "$DB_URL" == *"sslmode=prefer"* ]]; then
        # Replace sslmode=require/prefer with sslmode=disable for local dev
        DB_URL="${DB_URL//sslmode=require/sslmode=disable}"
        DB_URL="${DB_URL//sslmode=prefer/sslmode=disable}"
    fi
else
    ENCODED_USER=$(urlencode "$DB_USER")
    ENCODED_PASSWORD=$(urlencode "$DB_PASSWORD")
    DB_URL="postgres://${ENCODED_USER}:${ENCODED_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
fi

# Execute migration based on command
case "$1" in
    up)
        echo "Running migrations up..."
        echo "Database connection: configured (credentials redacted)"
        echo "MIGRATIONS_DIR: ${MIGRATIONS_DIR}"
        migrate -path "${MIGRATIONS_DIR}" -database "${DB_URL}" up
        ;;
    down)
        echo "Running migrations down..."
        migrate -path "${MIGRATIONS_DIR}" -database "${DB_URL}" down
        ;;
    create)
        if [ -z "$2" ]; then
            echo "Error: Migration name is required"
            echo "Usage: $0 create <migration_name>"
            exit 1
        fi
        echo "Creating migration files for $2..."
        migrate create -ext sql -dir "${MIGRATIONS_DIR}" -seq "$2"
        echo "Created:"
        echo "  - ${MIGRATIONS_DIR}/$(ls -t "${MIGRATIONS_DIR}" | head -1)"
        echo "  - ${MIGRATIONS_DIR}/$(ls -t "${MIGRATIONS_DIR}" | head -2 | tail -1)"
        ;;
    version)
        echo "Checking current migration version..."
        migrate -path "${MIGRATIONS_DIR}" -database "${DB_URL}" version
        ;;
    force)
        if [ -z "$2" ]; then
            echo "Error: Version number is required"
            echo "Usage: $0 force <version>"
            echo "Note: Use -1 to reset to no version (allows re-running all migrations)"
            exit 1
        fi
        VERSION="$2"
        echo "Forcing migration version to $VERSION..."
        # Use env to pass the command, avoiding shell flag parsing issues with negative numbers
        env migrate -path "${MIGRATIONS_DIR}" -database "${DB_URL}" force -- "$VERSION"
        ;;
    goto)
        if [ -z "$2" ]; then
            echo "Error: Version number is required"
            echo "Usage: $0 goto <version>"
            exit 1
        fi
        echo "Migrating to version $2..."
        migrate -path "${MIGRATIONS_DIR}" -database "${DB_URL}" goto "$2"
        ;;
    *)
        echo "Usage: $0 {up|down|create <migration_name>|version|force <version>|goto <version>}"
        exit 1
        ;;
esac

echo "Migration command completed successfully"
