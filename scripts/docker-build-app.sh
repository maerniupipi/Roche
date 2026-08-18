#!/bin/bash
# ============================================================
# 预编译 Go 二进制 + 构建精简镜像
# 
# 策略：
# 1. 用 docker run 在容器中编译二进制（挂载主机 Go 缓存目录）
# 2. 用精简 Dockerfile 打包预编译好的二进制
#
# Go build cache (/root/.cache/go-build) 和 Go mod cache (/go/pkg/mod)
# 挂载到主机，实现跨 CI 运行持久化，增量编译极快
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CACHE_DIR="${CACHE_DIR:-/tmp/rochekap-build-cache}"
BINARY_NAME="RocheKAP"

mkdir -p "${CACHE_DIR}/go-build"
mkdir -p "${CACHE_DIR}/go-mod"
mkdir -p "${CACHE_DIR}/go-bin"
mkdir -p "${CACHE_DIR}/duckdb"

# ---- 首次构建或更新 Builder 镜像 ----
BUILDER_IMAGE="rochekap/go-builder:latest"
if ! docker image inspect "${BUILDER_IMAGE}" > /dev/null 2>&1; then
  echo "=== 首次构建：创建 Go Builder 镜像（仅此一次） ==="
  docker build \
    --platform linux/amd64 \
    -f docker/Dockerfile.builder \
    -t "${BUILDER_IMAGE}" \
    "${PROJECT_DIR}"
  echo "=== Builder 镜像创建完成 ==="
else
  echo "=== 复用已有 Builder 镜像 ==="
fi

echo "=== Go 缓存目录 ==="
echo "  go-build:  ${CACHE_DIR}/go-build"
echo "  go-mod:    ${CACHE_DIR}/go-mod"
echo "  duckdb:    ${CACHE_DIR}/duckdb"

echo "=== 编译 Go 二进制 ==="
# 从 Builder 镜像提取 migrate 工具到缓存（镜像已预装）
if [ ! -f "${CACHE_DIR}/go-bin/migrate" ]; then
  echo "提取 migrate 工具..."
  docker create --name tmp-builder-extract "${BUILDER_IMAGE}" > /dev/null 2>&1
  docker cp tmp-builder-extract:/go/bin/migrate "${CACHE_DIR}/go-bin/migrate" 2>/dev/null || true
  docker rm tmp-builder-extract > /dev/null 2>&1
fi

docker run --rm \
  --platform linux/amd64 \
  -v "${PROJECT_DIR}:/app" \
  -v "${CACHE_DIR}/go-build:/root/.cache/go-build" \
  -v "${CACHE_DIR}/go-mod:/go/pkg/mod" \
  -v "${CACHE_DIR}/duckdb:/root/.duckdb" \
  -w /app \
  -e GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  -e GOSUMDB="${GOSUMDB:-off}" \
  "${BUILDER_IMAGE}" \
  bash -c "
    # 允许容器内 git 访问挂载的仓库
    git config --global --add safe.directory /app

    # 下载依赖（首次较慢，后续从 /go/pkg/mod 缓存读取）
    go mod download

    # duckdb 下载
    go run cmd/download/duckdb/duckdb.go 2>/dev/null || true

    # 复制 yanyiwu 模块到项目目录（供精简 Dockerfile 使用）
    cp -r /go/pkg/mod/github.com/yanyiwu/ /app/yanyiwu/ 2>/dev/null || true

    # 编译
    make build-prod
  "

echo "=== 编译完成: ${PROJECT_DIR}/${BINARY_NAME} ==="
ls -lh "${PROJECT_DIR}/${BINARY_NAME}"

# 从缓存目录复制 migrate 工具到项目根（供 Dockerfile COPY 使用）
if [ -f "${CACHE_DIR}/go-bin/migrate" ]; then
  cp "${CACHE_DIR}/go-bin/migrate" "${PROJECT_DIR}/migrate"
  echo "migrate 工具已就绪"
fi

# 确保 duckdb 缓存目录可复制到镜像
if [ -d "${CACHE_DIR}/duckdb" ] && [ "$(ls -A ${CACHE_DIR}/duckdb 2>/dev/null)" ]; then
  cp -r "${CACHE_DIR}/duckdb" "${PROJECT_DIR}/.duckdb"
  echo "duckdb 缓存已就绪"
fi

echo "=== 构建精简 Docker 镜像 ==="
docker build \
  --platform linux/amd64 \
  -f docker/Dockerfile.app.fast \
  -t rochekap/app:latest \
  "${PROJECT_DIR}"

echo "=== App 镜像构建完成 ==="
