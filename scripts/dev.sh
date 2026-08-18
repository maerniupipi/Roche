#!/bin/bash
# 开发环境启动脚本 - 只启动基础设施，app 和 frontend 需要手动在本地运行

# 设置颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # 无颜色

# 获取项目根目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
DEV_ENV_FILE="${DEV_ENV_FILE:-.env.local}"

# 日志函数
log_info() {
    printf "%b\n" "${BLUE}[INFO]${NC} $1"
}

log_success() {
    printf "%b\n" "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    printf "%b\n" "${RED}[ERROR]${NC} $1"
}

log_warning() {
    printf "%b\n" "${YELLOW}[WARNING]${NC} $1"
}

# 选择可用的 Docker Compose 命令
DOCKER_COMPOSE_BIN=""
DOCKER_COMPOSE_SUBCMD=""

detect_compose_cmd() {
    if docker compose version &> /dev/null; then
        DOCKER_COMPOSE_BIN="docker"
        DOCKER_COMPOSE_SUBCMD="compose"
        return 0
    fi
    if command -v docker-compose &> /dev/null; then
        if docker-compose version &> /dev/null; then
            DOCKER_COMPOSE_BIN="docker-compose"
            DOCKER_COMPOSE_SUBCMD=""
            return 0
        fi
    fi
    return 1
}

# 显示帮助信息
show_help() {
    printf "%b\n" "${GREEN}RocheKAP 开发环境脚本${NC}"
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  start      启动基础设施服务（postgres, redis, docreader, langfuse, Mock SAML）"
    echo "  stop       停止所有服务"
    echo "  restart    重启所有服务"
    echo "  logs       查看服务日志"
    echo "  status     查看服务状态"
    echo "  app        启动后端应用（本地运行）"
    echo "  frontend   启动 frontend 前端开发服务器（本地运行，端口 5174）"
    echo "  frontend-default 启动 frontend-default 前端开发服务器（本地运行，端口 5173，默认入口）"
    echo "  frontend-admin 启动 frontend-admin 前端开发服务器（本地运行，端口 5175，挂在 /admin/）"
    echo "  frontend-app 启动 frontend-app 前端开发服务器（本地运行，端口 5176，挂在 /app/）"
    echo "  help       显示此帮助信息"
    echo ""
    echo "可选 Profile（用于 start 命令）:"
    echo "  --minio       启动 MinIO 对象存储"
    echo "  --qdrant      启动 Qdrant 向量数据库"
    echo "  --milvus      启动 Milvus 向量数据库"
    echo "  --neo4j       启动 Neo4j 图数据库"
    echo "  --langfuse    启动 Langfuse（默认已开启）"
    echo "  --no-langfuse 不启动 Langfuse"
    echo "  --mock-saml      启动本地 Mock SAML（默认已开启）"
    echo "  --no-mock-saml   不启动本地 Mock SAML"
    echo "  --odl-hybrid  启动 OpenDataLoader hybrid（Docling，镜像较大，按需启用）"
    echo "  --full        启动所有可选服务（不含 odl-hybrid，需另加 --odl-hybrid）"
    echo ""
    echo "示例："
    echo "  $0 start                    # 启动基础服务"
    echo "  $0 start --qdrant           # 启动基础服务 + Qdrant"
    echo "  $0 start --milvus           # 启动基础服务 + Milvus"
    echo "  $0 start --odl-hybrid       # 启动基础服务 + OpenDataLoader hybrid"
    echo "  $0 start --full             # 启动所有服务"
    echo "  make dev-start DEV_ARGS=--odl-hybrid   # 同上（Makefile 传参）"
    echo "  $0 app                      # 在另一个终端启动后端"
    echo "  $0 frontend-default         # 在另一个终端启动 frontend-default 前端（默认入口，5173）"
    echo "  $0 frontend                 # 在另一个终端启动 legacy frontend 前端（5174）"
    echo "  $0 frontend-admin           # 在另一个终端启动 frontend-admin 前端（5175）"
    echo "  $0 frontend-app             # 在另一个终端启动 frontend-app 前端（5176）"
}

# Load the single local development environment file.
load_env_files() {
    local env_file="$DEV_ENV_FILE"
    if [[ "$env_file" != /* ]]; then
        env_file="$PROJECT_ROOT/$env_file"
    fi
    if [ -f "$env_file" ]; then
        set -a
        # shellcheck source=/dev/null
        source "$env_file"
        set +a
        export AUTH_SERVICE_INTERNAL_SECRET="${AUTH_SERVICE_INTERNAL_SECRET:-rochekap-local-gateway-auth-secret-32bytes}"
        export AUTH_REFRESH_COOKIE_SECURE="${AUTH_REFRESH_COOKIE_SECURE:-false}"
    else
        return 1
    fi
    return 0
}

# 检查 Docker
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "未安装Docker，请先安装Docker"
        return 1
    fi
    
    if ! detect_compose_cmd; then
        log_error "未检测到 Docker Compose"
        return 1
    fi
    
    if ! docker info &> /dev/null; then
        log_error "Docker服务未运行"
        return 1
    fi
    
    return 0
}

# 检查 .env.local 是否启用了 hybrid 模式（用于 --odl-hybrid 启动后重建 docreader）
_should_enable_odl_hybrid_from_env() {
    local hybrid="${DOCREADER_ODL_HYBRID:-off}"
    hybrid=$(printf '%s' "$hybrid" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')
    case "$hybrid" in
        off|"") return 1 ;;
        *) return 0 ;;
    esac
}

_enable_odl_hybrid_profile() {
    PROFILES="$PROFILES --profile odl-hybrid"
    ENABLED_SERVICES="$ENABLED_SERVICES odl-hybrid"
}

# 等待 odl-hybrid HTTP 健康检查通过（compose 启动后服务可能仍在拉依赖）
_wait_odl_hybrid_ready() {
    local port="${ODL_HYBRID_PORT:-5002}"
    local max_wait="${ODL_HYBRID_STARTUP_WAIT_SEC:-180}"
    local waited=0
    local interval=5

    if ! command -v curl &> /dev/null; then
        log_warning "未安装 curl，跳过 odl-hybrid 就绪等待；请手动检查 http://localhost:${port}/health"
        return 0
    fi

    log_info "等待 odl-hybrid 就绪（最多 ${max_wait}s，首次需构建镜像: docker compose ... build odl-hybrid）..."
    while [ "$waited" -lt "$max_wait" ]; do
        if curl -sf "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
            log_success "odl-hybrid 已就绪 (http://localhost:${port}/health)"
            return 0
        fi
        sleep "$interval"
        waited=$((waited + interval))
    done
    log_warning "odl-hybrid 在 ${max_wait}s 内未就绪，请查看: docker logs roche-kap-odl-hybrid"
    return 1
}

# 启动基础设施服务
start_services() {
    log_info "启动开发环境基础设施服务..."
    
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi

    cd "$PROJECT_ROOT"
    
    # Check the single local environment file.
    if [ ! -f ".env.local" ]; then
        log_error ".env.local 文件不存在，请从 .env.local.example 创建"
        return 1
    fi

    load_env_files
    if [ $? -ne 0 ]; then
        log_error ".env.local 文件不存在，请从 .env.local.example 创建"
        return 1
    fi

    if [ -n "${DEV_REMOTE_HOST:-}" ]; then
        log_warning "已配置 DEV_REMOTE_HOST=${DEV_REMOTE_HOST}，跳过本地 Docker 基础设施启动"
        log_info "远程服务: PostgreSQL/Redis/DocReader/Langfuse → ${DEV_REMOTE_HOST}"
        log_info "接下来: make dev-app（本地后端）或 make dev-frontend（前端）"
        return 0
    fi
    
    # 解析 profile 参数
    shift  # 移除 "start" 命令本身
    # 默认启动基础设施（postgres / redis / docreader）+ langfuse + Mock SAML，
    # 其余可选服务通过 --minio / --qdrant / --milvus / --neo4j / --full 按需开启。
    PROFILES="--profile langfuse --profile mock-saml --profile gateway"
    ENABLED_SERVICES="langfuse mock-saml gateway"
    while [ $# -gt 0 ]; do
        case "$1" in
            --minio)
                PROFILES="$PROFILES --profile minio"
                ENABLED_SERVICES="$ENABLED_SERVICES minio"
                ;;
            --qdrant)
                PROFILES="$PROFILES --profile qdrant"
                ENABLED_SERVICES="$ENABLED_SERVICES qdrant"
                ;;
            --milvus)
                PROFILES="$PROFILES --profile milvus"
                ENABLED_SERVICES="$ENABLED_SERVICES milvus"
                ;;
            --neo4j)
                PROFILES="$PROFILES --profile neo4j"
                ENABLED_SERVICES="$ENABLED_SERVICES neo4j"
                ;;
            --langfuse)
                PROFILES="$PROFILES --profile langfuse"
                ENABLED_SERVICES="$ENABLED_SERVICES langfuse"
                ;;
            --no-langfuse)
                PROFILES="${PROFILES//--profile langfuse/}"
                ENABLED_SERVICES="${ENABLED_SERVICES//langfuse/}"
                ;;
            --mock-saml)
                PROFILES="$PROFILES --profile mock-saml"
                ENABLED_SERVICES="$ENABLED_SERVICES mock-saml"
                ;;
            --no-mock-saml)
                PROFILES="${PROFILES//--profile mock-saml/}"
                ENABLED_SERVICES="${ENABLED_SERVICES//mock-saml/}"
                ;;
            --gateway)
                PROFILES="$PROFILES --profile gateway"
                ENABLED_SERVICES="$ENABLED_SERVICES gateway"
                ;;
            --no-gateway)
                PROFILES="${PROFILES//--profile gateway/}"
                ENABLED_SERVICES="${ENABLED_SERVICES//gateway/}"
                ;;
            --odl-hybrid)
                if [[ "$ENABLED_SERVICES" != *"odl-hybrid"* ]]; then
                    _enable_odl_hybrid_profile
                fi
                ;;
            --full)
                PROFILES="--profile full"
                ENABLED_SERVICES="minio qdrant milvus neo4j langfuse mock-saml gateway"
                break
                ;;
            *)
                log_warning "未知参数: $1"
                ;;
        esac
        shift
    done

    # 启动服务（odl-hybrid 单独 --build，避免每次重建 docreader）
    "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml $PROFILES up -d
    local compose_rc=$?
    if [ "$compose_rc" -eq 0 ] && [[ "$ENABLED_SERVICES" == *"odl-hybrid"* ]]; then
        log_info "构建/更新 odl-hybrid 镜像..."
        "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml $PROFILES up -d --build odl-hybrid
        _wait_odl_hybrid_ready || true
        # docreader 需读取 DOCREADER_ODL_HYBRID；若刚改 .env.local，强制重建以注入环境变量
        if _should_enable_odl_hybrid_from_env; then
            log_info "重建 docreader 以应用 DOCREADER_ODL_HYBRID=${DOCREADER_ODL_HYBRID} ..."
            "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml up -d --force-recreate docreader
        fi
    fi

    if [ "$compose_rc" -eq 0 ]; then
        log_success "基础设施服务已启动"
        echo ""
        log_info "服务访问地址:"
        echo "  - PostgreSQL:    localhost:5432"
        echo "  - Redis:         localhost:6379"
        echo "  - DocReader:     localhost:50051"
        
        # 根据启用的 profile 显示额外服务
        if [[ "$ENABLED_SERVICES" == *"minio"* ]]; then
            echo "  - MinIO:         localhost:9000 (Console: localhost:9001)"
        fi
        if [[ "$ENABLED_SERVICES" == *"qdrant"* ]]; then
            echo "  - Qdrant:        localhost:6333 (gRPC: localhost:6334)"
        fi
        if [[ "$ENABLED_SERVICES" == *"milvus"* ]]; then
            echo "  - Milvus:        localhost:19530 (WebUI: http://localhost:9091/webui/)"
        fi
        if [[ "$ENABLED_SERVICES" == *"neo4j"* ]]; then
            echo "  - Neo4j:         localhost:7474 (Bolt: localhost:7687)"
        fi
        if [[ "$ENABLED_SERVICES" == *"langfuse"* ]]; then
            echo "  - Langfuse:      http://localhost:${LANGFUSE_WEB_PORT:-3000}"
        fi
        if [[ "$ENABLED_SERVICES" == *"mock-saml"* ]]; then
            echo "  - Mock SAML:     http://127.0.0.1:${MOCK_SAML_PORT:-8091}"
            echo "                   admin / Admin123!"
        fi
        if [[ "$ENABLED_SERVICES" == *"odl-hybrid"* ]]; then
            echo "  - ODL Hybrid:    http://localhost:${ODL_HYBRID_PORT:-5002} (health: /health)"
            echo "                   docreader 需 DOCREADER_ODL_HYBRID=docling-fast"
        fi
        
        echo ""
        log_info "接下来的步骤:"
        printf "%b\n" "${YELLOW}1. 在新终端运行后端:${NC} make dev-app"
        printf "%b\n" "${YELLOW}2. 在新终端运行前端:${NC} make dev-frontend"
        return 0
    else
        log_error "服务启动失败"
        return 1
    fi
}

# 停止服务
stop_services() {
    log_info "停止开发环境服务..."
    
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi
    
    cd "$PROJECT_ROOT"
    # Include every optional profile so Milvus, Neo4j and Langfuse are
    # stopped together with the default PostgreSQL/Redis/DocReader services.
    "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml --profile "*" down
    
    if [ $? -eq 0 ]; then
        log_success "所有服务已停止"
        return 0
    else
        log_error "服务停止失败"
        return 1
    fi
}

# 重启服务
restart_services() {
    stop_services
    sleep 2
    start_services
}

# 查看日志
show_logs() {
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi

    cd "$PROJECT_ROOT"
    "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml logs -f
}

# 查看状态
show_status() {
    check_docker
    if [ $? -ne 0 ]; then
        return 1
    fi

    cd "$PROJECT_ROOT"
    "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD --env-file .env.local -f docker-compose.local.yml ps
}

# 远程开发模式下检查基础设施端口是否可达
check_remote_dev_connectivity() {
    local host="${DEV_REMOTE_HOST:-}"
    if [ -z "$host" ]; then
        return 0
    fi

    local db_port="${DB_PORT:-5432}"
    local redis_port
    redis_port="${REDIS_ADDR#*:}"
    if [ "$redis_port" = "$REDIS_ADDR" ]; then
        redis_port=6379
    fi
    local docreader_port="${DOCREADER_PORT:-50051}"

    log_info "检查远程基础设施连通性 (${host})..."
    local failed=0
    for spec in "PostgreSQL:${host}:${db_port}" "Redis:${host}:${redis_port}" "DocReader:${host}:${docreader_port}"; do
        local name="${spec%%:*}"
        local rest="${spec#*:}"
        local h="${rest%%:*}"
        local p="${rest##*:}"
        if command -v nc &> /dev/null; then
            if nc -z -G 3 "$h" "$p" 2>/dev/null; then
                log_success "${name} ${h}:${p} 可达"
            else
                log_error "${name} ${h}:${p} 不可达 (no route / connection refused)"
                failed=1
            fi
        else
            log_warning "未安装 nc，跳过 ${name} 连通性检查"
        fi
    done

    if [ "$failed" -ne 0 ]; then
        echo ""
        log_error "无法连接远程开发环境 ${host}"
        log_info "排查建议:"
        echo "  1. 确认远程机器 Docker 容器在运行 (postgres/redis/docreader)"
        echo "  2. 确认本机与 ${host} 在同一局域网 (本机: $(ipconfig getifaddr en0 2>/dev/null || echo '未知'))"
        echo "  3. 在远程检查端口映射: docker ps --format 'table {{.Names}}\t{{.Ports}}'"
        echo "  4. 检查远程防火墙是否放行 5432/6379/50051"
        return 1
    fi
    return 0
}

# 启动后端应用（本地）
start_app() {
    log_info "启动后端应用（本地开发模式）..."
    
    cd "$PROJECT_ROOT"
    
    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装"
        return 1
    fi
    
    log_info "加载环境配置..."
    if ! load_env_files; then
        log_error ".env.local 文件不存在，请从 .env.local.example 创建"
        return 1
    fi
    
    # Local Compose mode maps container service names to localhost. Remote
    # development preserves addresses from .env.local when DEV_REMOTE_HOST is set.
    if [ -n "${DEV_REMOTE_HOST:-}" ]; then
        log_info "远程开发模式: 基础设施 → ${DEV_REMOTE_HOST}"
        export DB_HOST="${DB_HOST:-$DEV_REMOTE_HOST}"
        export REDIS_ADDR="${REDIS_ADDR:-$DEV_REMOTE_HOST:6379}"
        export DOCREADER_ADDR="${DOCREADER_ADDR:-$DEV_REMOTE_HOST:50051}"
        export MINIO_ENDPOINT="${MINIO_ENDPOINT:-$DEV_REMOTE_HOST:9000}"
        export MILVUS_ADDRESS="${MILVUS_ADDRESS:-$DEV_REMOTE_HOST:19530}"
        export NEO4J_URI="${NEO4J_URI:-bolt://$DEV_REMOTE_HOST:7687}"
        export QDRANT_HOST="${QDRANT_HOST:-$DEV_REMOTE_HOST}"
        if [ -z "${LANGFUSE_HOST:-}" ] || [ "$LANGFUSE_HOST" = "http://langfuse-web:3000" ]; then
            export LANGFUSE_HOST="http://${DEV_REMOTE_HOST}:3000"
        fi
    else
        export DB_HOST=localhost
        export DOCREADER_ADDR=localhost:50051
        export MINIO_ENDPOINT=localhost:9000
        export REDIS_ADDR=localhost:6379
        export MILVUS_ADDRESS=localhost:19530
        export NEO4J_URI=bolt://localhost:7687
        export QDRANT_HOST=localhost
        # The backend runs on the host in dev mode, so the Docker-internal
        # service name is not resolvable here. Preserve Cloud/custom URLs.
        if [ -z "${LANGFUSE_HOST:-}" ] || [ "$LANGFUSE_HOST" = "http://langfuse-web:3000" ]; then
            export LANGFUSE_HOST="http://localhost:${LANGFUSE_WEB_PORT:-3000}"
        fi
    fi
    export DOCREADER_TRANSPORT="${DOCREADER_TRANSPORT:-grpc}"

    if ! check_remote_dev_connectivity; then
        return 1
    fi

    # .env.local.example uses /data/files for the Docker app container, where a
    # volume is mounted at that path. When the backend runs directly on the
    # host via dev-app, /data is often read-only or missing, so use a repo-local
    # writable directory unless the developer explicitly configured another
    # local storage path.
    if [ -z "${LOCAL_STORAGE_BASE_DIR:-}" ] || [ "$LOCAL_STORAGE_BASE_DIR" = "/data/files" ]; then
        export LOCAL_STORAGE_BASE_DIR="$PROJECT_ROOT/.local-data/files"
    fi
    mkdir -p "$LOCAL_STORAGE_BASE_DIR"
    
    # 确保必要的环境变量已设置
    if [ -z "$DB_DRIVER" ]; then
        log_error "DB_DRIVER 环境变量未设置，请检查 .env.local 文件"
        return 1
    fi
    
    log_info "环境变量已设置，启动应用..."
    log_info "数据库地址: $DB_HOST:${DB_PORT:-5432}"
    
    export CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant"
    if [[ "$(uname)" == "Darwin" ]]; then
      export CGO_LDFLAGS="-Wl,-no_warn_duplicate_libraries"
    fi

    # Air's configured output is a Unix-style binary path. Under Git Bash on
    # Windows, CMD refuses to execute it without an .exe suffix, so use the
    # reliable go-run path there while keeping hot reload on Linux/macOS.
    case "$(uname -s)" in
      MINGW*|MSYS*|CYGWIN*)
        log_info "Windows Git Bash 环境使用普通模式启动"
        LDFLAGS="$(./scripts/get_version.sh ldflags) -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"
        go run -ldflags="$LDFLAGS" ./cmd/server
        ;;
      *)
        if command -v air &> /dev/null; then
        log_success "检测到 Air，使用热重载模式启动..."
        log_info "修改 Go 代码后将自动重新编译和重启"
        air
        else
            log_info "未检测到 Air，使用普通模式启动"
            log_warning "提示: 安装 Air 可以实现代码修改后自动重启"
            log_info "安装命令: go install github.com/air-verse/air@latest"
            LDFLAGS="$(./scripts/get_version.sh ldflags) -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"
            go run -ldflags="$LDFLAGS" ./cmd/server
        fi
        ;;
    esac
}

# 启动 frontend 前端（本地，端口 5173，发布时的默认入口）
# Start the independent authentication process for host-side development.
start_auth_service() {
    log_info "Starting Auth Service (local source mode)..."
    cd "$PROJECT_ROOT"

    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        return 1
    fi
    if ! load_env_files; then
        log_error ".env.local is missing; create it from .env.local.example"
        return 1
    fi

    if [ -n "${DEV_REMOTE_HOST:-}" ]; then
        export DB_HOST="$DEV_REMOTE_HOST"
    else
        export DB_HOST=localhost
    fi
    export AUTH_SERVICE_HOST="${AUTH_SERVICE_HOST:-127.0.0.1}"
    export AUTH_SERVICE_PORT="${AUTH_SERVICE_PORT:-8081}"
    export AUTH_ALLOWED_REDIRECT_URIS="${AUTH_ALLOWED_REDIRECT_URIS:-http://127.0.0.1:5173/,http://localhost:5173/,http://127.0.0.1:5174/default/,http://localhost:5174/default/,http://127.0.0.1:5175/admin/,http://localhost:5175/admin/,http://127.0.0.1:5176/app/,http://localhost:5176/app/}"
    export AUTH_ALLOWED_ORIGINS="${AUTH_ALLOWED_ORIGINS:-http://127.0.0.1:5173,http://localhost:5173,http://127.0.0.1:5174,http://localhost:5174,http://127.0.0.1:5175,http://localhost:5175,http://127.0.0.1:5176,http://localhost:5176}"
    export OIDC_AUTH_ENABLE=false
    export SAML_AUTH_ENABLE="${SAML_AUTH_ENABLE:-true}"
    export SAML_AUTH_PROVIDER_DISPLAY_NAME="${SAML_AUTH_PROVIDER_DISPLAY_NAME:-Mock SAML}"
    export SAML_AUTH_IDP_METADATA_URL="${SAML_AUTH_IDP_METADATA_URL:-http://127.0.0.1:${MOCK_SAML_PORT:-8091}/metadata}"
    export SAML_AUTH_SP_ENTITY_ID="${SAML_AUTH_SP_ENTITY_ID:-urn:rochekap:local:sp}"
    export SAML_AUTH_ACS_URL="${SAML_AUTH_ACS_URL:-http://127.0.0.1:${GATEWAY_PORT:-8088}/api/v1/auth/saml/acs}"
    export SAML_AUTH_AUTO_PROVISION="${SAML_AUTH_AUTO_PROVISION:-true}"
    export SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS="${SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS:-developer001@rochekap.local,developer002@rochekap.local,developer003@rochekap.local,developer004@rochekap.local,developer005@rochekap.local,developer006@rochekap.local,developer007@rochekap.local,developer008@rochekap.local,developer009@rochekap.local,developer010@rochekap.local}"
    export SAML_AUTH_SIGN_REQUEST="${SAML_AUTH_SIGN_REQUEST:-false}"
    export SAML_AUTH_ALLOW_EPHEMERAL_CERT="${SAML_AUTH_ALLOW_EPHEMERAL_CERT:-true}"

    go run ./cmd/auth-service
}

start_frontend() {
    log_info "启动 frontend 前端开发服务器..."

    cd "$PROJECT_ROOT"
    if [ -f ".env.local" ]; then
        load_env_files >/dev/null 2>&1 || true
    fi

    cd "$PROJECT_ROOT/frontend"

    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装"
        return 1
    fi

    if [ ! -d "node_modules" ]; then
        log_warning "node_modules 不存在，正在安装依赖..."
        npm install
    fi

    log_info "启动 Vite 开发服务器..."
    log_info "frontend 前端将运行在 http://localhost:5173"
    log_info "前端 API 代理目标: ${VITE_DEV_PROXY_TARGET:-${FRONTEND_BACKEND_URL:-http://127.0.0.1:8088}}"

    npm run dev
}

# 启动 frontend-default 前端（本地，端口 5174，新版前端；发布后挂在 /default/ 下）
start_frontend_default() {
    log_info "启动 frontend-default (新版) 前端开发服务器..."

    cd "$PROJECT_ROOT"
    if [ -f ".env.local" ]; then
        load_env_files >/dev/null 2>&1 || true
    fi

    cd "$PROJECT_ROOT/frontend-default"

    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装"
        return 1
    fi

    if [ ! -d "node_modules" ]; then
        log_warning "node_modules 不存在，正在安装依赖..."
        npm install
    fi

    log_info "启动 Vite 开发服务器..."
    log_info "frontend-default 前端将运行在 http://localhost:5174"
    log_info "前端 API 代理目标: ${VITE_DEV_PROXY_TARGET:-${FRONTEND_BACKEND_URL:-http://127.0.0.1:8088}}"

    npm run dev
}

# 启动 frontend-admin 前端（本地，端口 5175；发布后挂在 /admin/ 下）
start_frontend_admin() {
    log_info "启动 frontend-admin 前端开发服务器..."

    cd "$PROJECT_ROOT"
    if [ -f ".env.local" ]; then
        load_env_files >/dev/null 2>&1 || true
    fi

    cd "$PROJECT_ROOT/frontend-admin"

    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装"
        return 1
    fi

    if [ ! -d "node_modules" ]; then
        log_warning "node_modules 不存在，正在安装依赖..."
        npm install
    fi

    log_info "启动 Vite 开发服务器..."
    log_info "frontend-admin 前端将运行在 http://localhost:5175/admin/"
    log_info "前端 API 代理目标: ${VITE_DEV_PROXY_TARGET:-${FRONTEND_BACKEND_URL:-http://127.0.0.1:8088}}"

    npm run dev
}

# 启动 frontend-app 前端（本地，端口 5176；发布后挂在 /app/ 下）
start_frontend_app() {
    log_info "启动 frontend-app 前端开发服务器..."

    cd "$PROJECT_ROOT"
    if [ -f ".env.local" ]; then
        load_env_files >/dev/null 2>&1 || true
    fi

    cd "$PROJECT_ROOT/frontend-app"

    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装"
        return 1
    fi

    if [ ! -d "node_modules" ]; then
        log_warning "node_modules 不存在，正在安装依赖..."
        npm install
    fi

    log_info "启动 Vite 开发服务器..."
    log_info "frontend-app 前端将运行在 http://localhost:5176/app/"
    log_info "前端 API 代理目标: ${VITE_DEV_PROXY_TARGET:-${FRONTEND_BACKEND_URL:-http://127.0.0.1:8088}}"

    npm run dev
}

# 解析命令
CMD="${1:-help}"
case "$CMD" in
    start)
        start_services "$@"
        ;;
    stop)
        stop_services
        ;;
    restart)
        restart_services
        ;;
    logs)
        show_logs
        ;;
    status)
        show_status
        ;;
    app)
        start_app
        ;;
    auth)
        start_auth_service
        ;;
    frontend)
        start_frontend
        ;;
    frontend-default)
        start_frontend_default
        ;;
    frontend-admin)
        start_frontend_admin
        ;;
    frontend-app)
        start_frontend_app
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        log_error "未知命令: $CMD"
        show_help
        exit 1
        ;;
esac

exit 0
