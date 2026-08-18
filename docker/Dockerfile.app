# syntax=docker/dockerfile:1

# ============================================================
# Stage 1: Go 构建器（原 go-builder，无需外部 Registry）
# ============================================================
FROM golang:1.26.0-bookworm AS go-builder

RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        build-essential \
        libsqlite3-dev \
        protobuf-compiler \
    && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 预安装工具（使用国内代理）
ENV GOPROXY=https://goproxy.cn,direct
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# ============================================================
# Stage 2: 应用构建
# ============================================================
FROM go-builder AS builder

WORKDIR /app

ARG GOPRIVATE_ARG
ARG GOPROXY_ARG
ARG GOSUMDB_ARG=off

ENV GOPRIVATE=${GOPRIVATE_ARG}
# 未显式传入 GOPROXY_ARG 时回退到国内代理，避免 go 使用默认的 proxy.golang.org（国内直连极慢）
ENV GOPROXY=${GOPROXY_ARG:-https://goproxy.cn,direct}
ENV GOSUMDB=${GOSUMDB_ARG:-off}

# go.mod/go.sum 不变时，依赖下载层直接命中镜像层缓存。
COPY go.mod go.sum ./
RUN go mod download

# duckdb 扩展：产物落到 /app/.duckdb（供运行时阶段 COPY），
# 该层只依赖 cmd/download 目录，几乎不会失效，可稳定复用层缓存。
COPY cmd/download cmd/download
RUN go run cmd/download/duckdb/duckdb.go && cp -r /root/.duckdb /app/.duckdb

COPY . .

ARG VERSION_ARG
ARG COMMIT_ID_ARG
ARG BUILD_TIME_ARG
ARG GO_VERSION_ARG

ENV VERSION=${VERSION_ARG}
ENV COMMIT_ID=${COMMIT_ID_ARG}
ENV BUILD_TIME=${BUILD_TIME_ARG}
ENV GO_VERSION=${GO_VERSION_ARG}

# 注意：不要在 Docker Desktop (Windows) 上给 go mod download / go build
# 加 BuildKit cache mount——gRPC-FUSE 对大量小文件写入性能极差（实测 go mod
# download 卡死 25+ 分钟）。本 Dockerfile 走镜像层缓存，适合 CI/服务器；
# 日常改代码迭代请使用 scripts/docker-build-app.ps1：
#   docker run -v 挂载主机缓存目录增量编译（快）→ Dockerfile.app.fast 秒级打包。
RUN make build-prod && cp -r /go/pkg/mod/github.com/yanyiwu/ /app/yanyiwu/

# ============================================================
# Stage 3: 运行时（原 app-base，无需外部 Registry）
# ============================================================
FROM debian:12.12-slim

RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources

RUN useradd -m -s /bin/bash appuser

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential \
        postgresql-client \
        default-mysql-client \
        tzdata \
        sed \
        curl \
        bash \
        vim \
        wget \
        libsqlite3-0 \
        python3 \
        python3-pip \
        python3-dev \
        libffi-dev \
        libssl-dev \
        nodejs \
        npm \
        gosu \
        ffmpeg \
    && python3 -m pip install --break-system-packages --upgrade pip setuptools wheel \
    && mkdir -p /home/appuser/.local/bin \
    && curl -LsSf https://astral.sh/uv/install.sh | CARGO_HOME=/home/appuser/.cargo UV_INSTALL_DIR=/home/appuser/.local/bin sh \
    && chown -R appuser:appuser /home/appuser \
    && ln -sf /home/appuser/.local/bin/uvx /usr/local/bin/uvx \
    && chmod +x /usr/local/bin/uvx \
    && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /data/files && \
    chown -R appuser:appuser /data/files

WORKDIR /app

COPY --from=builder /go/bin/migrate /usr/local/bin/
COPY --from=builder /app/yanyiwu/ /go/pkg/mod/github.com/yanyiwu/
COPY --from=builder /app/config ./config
COPY --from=builder /app/scripts ./scripts
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/dataset/samples ./dataset/samples
COPY --from=builder /app/skills/preloaded ./skills/preloaded
COPY --from=builder /app/skills/preloaded ./skills/_builtin
COPY --from=builder /app/.duckdb /home/appuser/.duckdb
COPY --from=builder /app/RocheKAP .
COPY --from=builder /app/scripts/docker-entrypoint.sh ./scripts/docker-entrypoint.sh

RUN chmod +x ./scripts/*.sh

EXPOSE 8080

ENTRYPOINT ["./scripts/docker-entrypoint.sh"]
CMD ["./RocheKAP"]
