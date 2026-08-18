# ============================================================
# Go Builder 镜像：预装所有编译依赖，避免每次 apt-get install
# 首次构建后缓存，后续 CI 直接复用
# ============================================================
FROM golang:1.26.0-bookworm

# 替换阿里云源 + 安装 CGO 编译依赖
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        build-essential \
        libsqlite3-dev \
        protobuf-compiler \
    && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 预安装 migrate 工具（Go module 缓存进镜像层）
ENV GOPROXY=https://goproxy.cn,direct
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# 预创建 Go 缓存目录（供 volume 挂载用）
RUN mkdir -p /root/.cache/go-build /root/.duckdb && \
    chmod 777 /root/.cache/go-build /root/.duckdb

WORKDIR /go/src
