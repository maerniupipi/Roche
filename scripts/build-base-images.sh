#!/bin/bash
# ============================================================
# 本地构建 Base 镜像脚本
# 
# 用法（本地有外网的机器）：
#   export CI_REGISTRY_USER=你的用户名
#   export CI_REGISTRY_PASSWORD=你的密码
#   bash scripts/build-base-images.sh
#
# 一次性构建 4 个 base 镜像并推送到内部 GitLab Registry。
# 这些镜像包含所有 OS 级 / npm 级依赖，CI 构建时无需外网。
#
# 注意：当 package.json / go.mod / pip 依赖有变化时，需重新运行此脚本。
# ============================================================
set -e

REGISTRY_HOST="${REGISTRY_HOST:-newgitlab.digitalchina.com}"
IMAGE_REGISTRY="${IMAGE_REGISTRY:-${REGISTRY_HOST}/tangjjg/rochekap}"
PLATFORM="${BUILD_PLATFORM:-linux/amd64}"

echo "=== 登录 Registry ==="
echo "${CI_REGISTRY_PASSWORD}" | docker login -u "${CI_REGISTRY_USER}" --password-stdin "${REGISTRY_HOST}"

echo ""
echo "=== [1/4] 构建 Go Builder Base 镜像 ==="
docker build \
    --platform ${PLATFORM} \
    -f docker/Dockerfile.go-builder \
    -t ${IMAGE_REGISTRY}/go-builder:latest \
    .
docker push ${IMAGE_REGISTRY}/go-builder:latest
echo "  -> ${IMAGE_REGISTRY}/go-builder:latest 完成"

echo ""
echo "=== [2/4] 构建 App Base 镜像 ==="
docker build \
    --platform ${PLATFORM} \
    -f docker/Dockerfile.app-base \
    -t ${IMAGE_REGISTRY}/app-base:latest \
    .
docker push ${IMAGE_REGISTRY}/app-base:latest
echo "  -> ${IMAGE_REGISTRY}/app-base:latest 完成"

echo ""
echo "=== [3/4] 构建 DocReader Base 镜像 ==="
docker build \
    --platform ${PLATFORM} \
    -f docker/Dockerfile.docreader-base \
    -t ${IMAGE_REGISTRY}/docreader-base:latest \
    .
docker push ${IMAGE_REGISTRY}/docreader-base:latest
echo "  -> ${IMAGE_REGISTRY}/docreader-base:latest 完成"

echo ""
echo "=== [4/4] 构建 Frontend Builder Base 镜像 ==="
docker build \
    --platform ${PLATFORM} \
    -f docker/Dockerfile.frontend-builder \
    -t ${IMAGE_REGISTRY}/frontend-builder:latest \
    .
docker push ${IMAGE_REGISTRY}/frontend-builder:latest
echo "  -> ${IMAGE_REGISTRY}/frontend-builder:latest 完成"

echo ""
echo "=== 全部完成！==="
echo "Base 镜像已推送到："
echo "  ${IMAGE_REGISTRY}/go-builder:latest"
echo "  ${IMAGE_REGISTRY}/app-base:latest"
echo "  ${IMAGE_REGISTRY}/docreader-base:latest"
echo "  ${IMAGE_REGISTRY}/frontend-builder:latest"
echo ""
echo "现在可以提交代码并在 GitLab 触发 CI 流水线。"
echo ""
echo "提醒：当 go.mod / pyproject.toml / package.json 依赖变更时，需重新运行此脚本。"
