#!/usr/bin/env bash
# One-command source development supervisor.
# Docker runs infrastructure; Go and Vite run from the local checkout.

set -u

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="$PROJECT_ROOT/logs"
PID_DIR="$PROJECT_ROOT/tmp"
BACKEND_LOG="$LOG_DIR/dev-backend.log"
AUTH_LOG="$LOG_DIR/dev-auth-service.log"
FRONTEND_LOG="$LOG_DIR/dev-frontend.log"
BACKEND_PID_FILE="$PID_DIR/dev-backend.pid"
AUTH_PID_FILE="$PID_DIR/dev-auth-service.pid"
FRONTEND_PID_FILE="$PID_DIR/dev-frontend.pid"
SUPERVISOR_PID_FILE="$PID_DIR/dev-all.pid"
BACKEND_URL="http://127.0.0.1:8080"
AUTH_URL="http://127.0.0.1:8081"

BACKEND_PID=""
AUTH_PID=""
FRONTEND_PID=""
CLEANING_UP=0

log_info() { printf "%b\n" "${BLUE}[INFO]${NC} $1"; }
log_success() { printf "%b\n" "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { printf "%b\n" "${YELLOW}[WARNING]${NC} $1"; }
log_error() { printf "%b\n" "${RED}[ERROR]${NC} $1"; }

is_windows_shell() {
    case "$(uname -s)" in
        MINGW*|MSYS*|CYGWIN*) return 0 ;;
        *) return 1 ;;
    esac
}

is_process_running() {
    local pid="${1:-}"
    [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1
}

stop_process_tree() {
    local pid="${1:-}"
    [ -z "$pid" ] && return 0
    is_process_running "$pid" || return 0

    if is_windows_shell && command -v taskkill.exe >/dev/null 2>&1; then
        local win_pid
        win_pid="$(ps -lp "$pid" 2>/dev/null | awk 'NR == 2 { print $4 }')"
        if [[ "$win_pid" =~ ^[0-9]+$ ]]; then
            taskkill.exe //PID "$win_pid" //T //F >/dev/null 2>&1 || true
            return 0
        fi
    fi

    kill "$pid" >/dev/null 2>&1 || true
    for _ in 1 2 3 4 5; do
        is_process_running "$pid" || return 0
        sleep 1
    done
    kill -9 "$pid" >/dev/null 2>&1 || true
}

read_pid_file() {
    local file="$1"
    if [ -f "$file" ]; then
        tr -dc '0-9' < "$file"
    fi
}

stop_pid_file() {
    local file="$1"
    local pid
    pid="$(read_pid_file "$file")"
    if [ -n "$pid" ]; then
        stop_process_tree "$pid"
    fi
    rm -f "$file"
}

url_ready() {
    local url="$1"
    curl --silent --show-error --fail --max-time 4 --output /dev/null "$url" >/dev/null 2>&1
}

wait_for_url() {
    local name="$1"
    local url="$2"
    local timeout="$3"
    local elapsed=0

    log_info "等待 ${name}: ${url}"
    while [ "$elapsed" -lt "$timeout" ]; do
        if url_ready "$url"; then
            log_success "${name} 已就绪"
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    log_error "${name} 在 ${timeout}s 内未就绪: ${url}"
    return 1
}

show_log_tail() {
    local label="$1"
    local file="$2"
    if [ -f "$file" ]; then
        log_error "${label}最近日志："
        tail -n 40 "$file" || true
    fi
}

cleanup() {
    [ "$CLEANING_UP" -eq 1 ] && return 0
    CLEANING_UP=1

    echo ""
    log_info "正在停止源码开发环境..."
    stop_process_tree "$FRONTEND_PID"
    stop_process_tree "$AUTH_PID"
    stop_process_tree "$BACKEND_PID"
    rm -f "$FRONTEND_PID_FILE" "$AUTH_PID_FILE" "$BACKEND_PID_FILE" "$SUPERVISOR_PID_FILE"

    cd "$PROJECT_ROOT"
    ./scripts/dev.sh stop >/dev/null 2>&1 || true
    log_success "源码开发环境已停止"
}

stop_all_environment() {
    mkdir -p "$PID_DIR"
    log_info "Stopping the complete source development environment..."

    # Stop the supervisor first so it cannot race this explicit cleanup.
    stop_pid_file "$SUPERVISOR_PID_FILE"
    stop_pid_file "$FRONTEND_PID_FILE"
    stop_pid_file "$AUTH_PID_FILE"
    stop_pid_file "$BACKEND_PID_FILE"

    cd "$PROJECT_ROOT"
    ./scripts/dev.sh stop
    log_success "Complete source development environment stopped"
}

on_signal() {
    exit 130
}

start_local_services() {
    mkdir -p "$LOG_DIR" "$PID_DIR"

    if url_ready "$BACKEND_URL/health"; then
        log_warning "后端已经运行，跳过重复启动"
    else
        stop_pid_file "$BACKEND_PID_FILE"
        log_info "后台启动 Go 后端，日志: $BACKEND_LOG"
        (
            cd "$PROJECT_ROOT"
            exec ./scripts/dev.sh app
        ) >"$BACKEND_LOG" 2>&1 &
        BACKEND_PID=$!
        printf "%s" "$BACKEND_PID" > "$BACKEND_PID_FILE"
    fi

    if url_ready "$AUTH_URL/health"; then
        log_warning "Auth Service is already running; skipping duplicate start"
    else
        stop_pid_file "$AUTH_PID_FILE"
        log_info "Starting Auth Service in background; log: $AUTH_LOG"
        (
            cd "$PROJECT_ROOT"
            exec ./scripts/dev.sh auth
        ) >"$AUTH_LOG" 2>&1 &
        AUTH_PID=$!
        printf "%s" "$AUTH_PID" > "$AUTH_PID_FILE"
    fi

    if url_ready "http://127.0.0.1:5173"; then
        log_warning "前端已经运行，跳过重复启动"
    else
        stop_pid_file "$FRONTEND_PID_FILE"
        log_info "后台启动 Vite 前端，日志: $FRONTEND_LOG"
        (
            cd "$PROJECT_ROOT"
            exec ./scripts/dev.sh frontend
        ) >"$FRONTEND_LOG" 2>&1 &
        FRONTEND_PID=$!
        printf "%s" "$FRONTEND_PID" > "$FRONTEND_PID_FILE"
    fi
}

verify_all_services() {
    wait_for_url "后端健康检查" "$BACKEND_URL/health" 300 || {
        show_log_tail "后端" "$BACKEND_LOG"
        return 1
    }
    wait_for_url "前端" "http://127.0.0.1:5173" 180 || {
        show_log_tail "前端" "$FRONTEND_LOG"
        return 1
    }
    wait_for_url "Auth Service" "$AUTH_URL/health" 180 || {
        show_log_tail "Auth Service" "$AUTH_LOG"
        return 1
    }
    wait_for_url "API Gateway" "http://127.0.0.1:${GATEWAY_PORT:-8088}/health" 180 || return 1
    wait_for_url "Langfuse" "http://127.0.0.1:${LANGFUSE_WEB_PORT:-3000}" 240 || return 1
    wait_for_url "Mock SAML" "http://127.0.0.1:${MOCK_SAML_PORT:-8091}/healthz" 120 || return 1
    wait_for_url "Milvus WebUI" "http://127.0.0.1:9091/webui/" 180 || return 1
    wait_for_url "Neo4j" "http://127.0.0.1:7474" 180 || return 1
}

print_summary() {
    echo ""
    printf "%b\n" "${GREEN}项目已按源码开发模式启动完成：${NC}"
    echo "前端：http://localhost:5173"
    echo "后端健康检查：${BACKEND_URL}/health"
    echo "Auth Service：${AUTH_URL}/health"
    echo "API Gateway：http://localhost:${GATEWAY_PORT:-8088}"
    echo "Langfuse：http://localhost:${LANGFUSE_WEB_PORT:-3000}"
    echo "Milvus WebUI：http://localhost:9091/webui/"
    echo "Neo4j：http://localhost:7474"
    printf "%b\n" "${GREEN}前后端及所有依赖服务均已正常运行。${NC}"
    echo ""
    echo "后端日志：$BACKEND_LOG"
    echo "Auth Service 日志：$AUTH_LOG"
    echo "前端日志：$FRONTEND_LOG"
    echo "按 Ctrl+C 停止本命令启动的源码开发环境。"
}

monitor_local_services() {
    while true; do
        sleep 3
        if [ -n "$BACKEND_PID" ] && ! is_process_running "$BACKEND_PID"; then
            log_error "Go 后端进程已退出"
            show_log_tail "后端" "$BACKEND_LOG"
            return 1
        fi
        if [ -n "$AUTH_PID" ] && ! is_process_running "$AUTH_PID"; then
            log_error "Auth Service 进程已退出"
            show_log_tail "Auth Service" "$AUTH_LOG"
            return 1
        fi
        if [ -n "$FRONTEND_PID" ] && ! is_process_running "$FRONTEND_PID"; then
            log_error "Vite 前端进程已退出"
            show_log_tail "前端" "$FRONTEND_LOG"
            return 1
        fi
    done
}

main() {
    export NO_PROXY="${NO_PROXY:+$NO_PROXY,}localhost,127.0.0.1,::1"
    export no_proxy="${no_proxy:+$no_proxy,}localhost,127.0.0.1,::1"

    cd "$PROJECT_ROOT"

    # Keep the supervisor's health checks aligned with the application and
    # Vite processes, which load the same local environment files in dev.sh.
    for env_file in .env.local; do
        if [ -f "$env_file" ]; then
            set -a
            # shellcheck source=/dev/null
            source "$env_file"
            set +a
        fi
    done
    BACKEND_URL="${DEV_BACKEND_URL:-http://127.0.0.1:8080}"

    if [ "${1:-}" = "stop" ]; then
        stop_all_environment
        return
    fi

    mkdir -p "$PID_DIR"
    printf "%s" "$$" > "$SUPERVISOR_PID_FILE"
    trap on_signal INT TERM
    trap cleanup EXIT

    log_info "启动 Docker 开发依赖（Milvus、Neo4j、Langfuse）..."
    ./scripts/dev.sh start --milvus --neo4j "$@" || return 1

    start_local_services
    verify_all_services || return 1
    print_summary
    monitor_local_services
}

main "$@"
