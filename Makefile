.PHONY: help build run test clean docker-build-app docker-build-docreader docker-build-frontend docker-build-all migrate-up migrate-down build-images build-images-app build-images-docreader build-images-frontend clean-images show-platform server-dev-up server-dev-update server-dev-down server-dev-status server-dev-logs production-up production-update production-down production-status production-logs dev-all dev-all-stop dev-start dev-stop dev-restart dev-logs dev-status dev-app dev-auth dev-frontend dev-frontend-default dev-frontend-admin dev-frontend-app docs install-swagger

# Show help
help:
	@echo "RocheKAP Makefile 帮助"
	@echo ""
	@echo "基础命令:"
	@echo "  build             构建应用"
	@echo "  run               运行应用"
	@echo "  test              运行测试"
	@echo "  clean             清理构建文件"
	@echo ""
	@echo "Docker 命令:"
	@echo "  docker-build-app               构建应用 Docker 镜像 (roche/knowledge-agent-platform-app)"
	@echo "  docker-build-docreader         构建文档读取器镜像 (roche/knowledge-agent-platform-docreader)"
	@echo "  docker-build-frontend          构建包含四套前端的单一镜像"
	@echo "  docker-build-all               构建所有 Docker 镜像"
	@echo "镜像构建:"
	@echo "  build-images      从源码构建所有镜像"
	@echo "  build-images-app  从源码构建应用镜像"
	@echo "  build-images-docreader 从源码构建文档读取器镜像"
	@echo "  build-images-frontend        从源码构建包含四套前端的单一镜像"
	@echo "  clean-images      清理本地镜像"
	@echo ""
	@echo "数据库:"
	@echo "  migrate-up        执行数据库迁移"
	@echo "  migrate-down      回滚数据库迁移"
	@echo ""
	@echo "开发工具:"
	@echo "  fmt               格式化代码"
	@echo "  lint              代码检查"
	@echo "  deps              安装依赖"
	@echo "  docs              生成 Swagger API 文档"
	@echo "  install-swagger   安装 swag 工具"
	@echo ""
	@echo "部署入口（四套）:"
	@echo "  dev-all               启动本地源码开发环境"
	@echo "  dev-all-stop          停止本地源码开发环境"
	@echo "  server-dev-up         启动服务器源码开发环境"
	@echo "  server-dev-down       停止服务器源码开发环境"
	@echo "  production-up         启动服务器正式环境"
	@echo "  production-down       停止服务器正式环境"
	@echo "  show-platform     显示当前构建平台"
	@echo "  server-dev-update     拉取 Git 更新并重建开发服务"
	@echo "  production-update     拉取固定标签镜像并更新"
	@echo ""
	@echo "开发模式（推荐）:"
	@echo "  dev-all           一键启动完整源码开发环境（前端、后端及全部常用依赖）"
	@echo "  dev-start         启动开发环境基础设施（仅启动依赖服务）"
	@echo "                    可选: make dev-start DEV_ARGS=--odl-hybrid"
	@echo "  dev-stop          停止开发环境"
	@echo "  dev-restart       重启开发环境"
	@echo "  dev-logs          查看开发环境日志"
	@echo "  dev-status        查看开发环境状态"
	@echo "  dev-app           启动后端应用（本地运行，需先运行 dev-start）"
	@echo "  dev-frontend             启动 frontend 前端（默认入口，本地运行，端口 5173，需先运行 dev-start）"
	@echo "  dev-frontend-default     启动 frontend-default 前端（新版，挂在 /default/，本地运行，端口 5174，需先运行 dev-start）"
	@echo "  dev-frontend-admin       启动 frontend-admin 前端（挂在 /admin/，本地运行，端口 5175，需先运行 dev-start）"
	@echo "  dev-frontend-app         启动 frontend-app 前端（挂在 /app/，本地运行，端口 5176，需先运行 dev-start）"

# Go related variables
BINARY_NAME=RocheKAP
MAIN_PATH=./cmd/server

# Docker related variables
DOCKER_IMAGE=roche/knowledge-agent-platform-app
DOCKER_TAG=latest

# Platform detection
ifeq ($(shell uname -m),x86_64)
    PLATFORM=linux/amd64
else ifeq ($(shell uname -m),aarch64)
    PLATFORM=linux/arm64
else ifeq ($(shell uname -m),arm64)
    PLATFORM=linux/arm64
else
    PLATFORM=linux/amd64
endif

# Build the application
build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

# Run the application
run: build
	./$(BINARY_NAME)

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)

# Build Docker image
docker-build-app:
	@echo "获取版本信息..."
	@eval $$(./scripts/get_version.sh env); \
	./scripts/get_version.sh info; \
	docker build --platform $(PLATFORM) \
		--build-arg VERSION_ARG="$$VERSION" \
		--build-arg COMMIT_ID_ARG="$$COMMIT_ID" \
		--build-arg BUILD_TIME_ARG="$$BUILD_TIME" \
		--build-arg GO_VERSION_ARG="$$GO_VERSION" \
		-f docker/Dockerfile.app -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Build docreader Docker image
docker-build-docreader:
	docker build --platform $(PLATFORM) -f docker/Dockerfile.docreader -t roche/knowledge-agent-platform-docreader:latest .

# Build the combined frontend Docker image
docker-build-frontend:
	COMMIT_ID=$$(git rev-parse --short HEAD); \
	docker build --platform $(PLATFORM) --build-arg COMMIT_ID_ARG="$$COMMIT_ID" -f frontend/Dockerfile -t roche/knowledge-agent-platform-ui:latest .

# Build all Docker images
docker-build-all: docker-build-app docker-build-docreader docker-build-frontend

# 从源码构建镜像相关命令
build-images:
	./scripts/build_images.sh

build-images-app:
	./scripts/build_images.sh --app

build-images-docreader:
	./scripts/build_images.sh --docreader

build-images-frontend:
	./scripts/build_images.sh --frontend

clean-images:
	./scripts/build_images.sh --clean

# Database migrations
migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down

migrate-version:
	./scripts/migrate.sh version

migrate-create:
	@if [ -z "$(name)" ]; then \
		echo "Error: migration name is required"; \
		echo "Usage: make migrate-create name=your_migration_name"; \
		exit 1; \
	fi
	./scripts/migrate.sh create $(name)

migrate-force:
	@if [ -z "$(version)" ]; then \
		echo "Error: version is required"; \
		echo "Usage: make migrate-force version=4"; \
		exit 1; \
	fi
	./scripts/migrate.sh force $(version)

migrate-goto:
	@if [ -z "$(version)" ]; then \
		echo "Error: version is required"; \
		echo "Usage: make migrate-goto version=3"; \
		exit 1; \
	fi
	./scripts/migrate.sh goto $(version)

# Generate API documentation (Swagger)
docs:
	@echo "生成 Swagger API 文档..."
	swag init -g $(MAIN_PATH)/main.go -o ./docs --parseDependency --parseInternal
	@echo "文档已生成到 ./docs 目录"
	@echo "启动服务后访问 http://localhost:8080/swagger/index.html 查看文档"

# Install swagger tool
install-swagger:
	go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Install dependencies
deps:
	go mod download

# Build for production
# google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn for qdrant milvus proto conflict
build-prod:
	VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo "$${VERSION:-unknown}"); \
	COMMIT_ID=$${COMMIT_ID:-unknown}; \
	CGO_ENABLED=1 \
	CGO_CFLAGS="-Wno-deprecated-declarations" \
	CGO_LDFLAGS="$$(if [ "$$(uname)" = 'Darwin' ]; then echo '-Wl,-no_warn_duplicate_libraries'; fi)" \
	BUILD_TIME=$${BUILD_TIME:-unknown}; \
	GO_VERSION=$${GO_VERSION:-unknown}; \
	LDFLAGS="-X 'roche.local/knowledge-agent-platform/internal/handler.Version=$$VERSION' -X 'roche.local/knowledge-agent-platform/internal/handler.Edition=standard' -X 'roche.local/knowledge-agent-platform/internal/handler.CommitID=$$COMMIT_ID' -X 'roche.local/knowledge-agent-platform/internal/handler.BuildTime=$$BUILD_TIME' -X 'roche.local/knowledge-agent-platform/internal/handler.GoVersion=$$GO_VERSION' -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"; \
	go build -buildvcs=false -ldflags="-w -s $$LDFLAGS" -o $(BINARY_NAME) $(MAIN_PATH)

download_spatial:
	go run cmd/download/duckdb/duckdb.go

clean-db:
	@echo "Cleaning database..."
	@if [ $$(docker volume ls -q -f name=roche_kap_postgres-data) ]; then \
		docker volume rm roche_kap_postgres-data; \
	fi
	@if [ $$(docker volume ls -q -f name=roche_kap_minio_data) ]; then \
		docker volume rm roche_kap_minio_data; \
	fi
	@if [ $$(docker volume ls -q -f name=roche_kap_redis_data) ]; then \
		docker volume rm roche_kap_redis_data; \
	fi

# Single-host source-mounted Docker development server
server-dev-up:
	bash ./scripts/server_dev.sh up

server-dev-update:
	bash ./scripts/server_dev.sh update

server-dev-down:
	bash ./scripts/server_dev.sh down

server-dev-status:
	bash ./scripts/server_dev.sh status

server-dev-logs:
	bash ./scripts/server_dev.sh logs

# Immutable-image production deployment
production-up:
	bash ./scripts/production_server.sh up

production-update:
	bash ./scripts/production_server.sh update

production-down:
	bash ./scripts/production_server.sh down

production-status:
	bash ./scripts/production_server.sh status

production-logs:
	bash ./scripts/production_server.sh logs

# Show current platform
show-platform:
	@echo "当前系统架构: $(shell uname -m)"
	@echo "Docker构建平台: $(PLATFORM)"

# Development mode commands
dev-all:
	bash ./scripts/dev-all.sh $(DEV_ARGS)

dev-all-stop:
	bash ./scripts/dev-all.sh stop

dev-start:
	./scripts/dev.sh start $(DEV_ARGS)

dev-stop:
	./scripts/dev.sh stop

dev-restart:
	./scripts/dev.sh restart

dev-logs:
	./scripts/dev.sh logs

dev-frontend-default:
	./scripts/dev.sh frontend-default

dev-frontend-admin:
	./scripts/dev.sh frontend-admin

dev-frontend-app:
	./scripts/dev.sh frontend-app

dev-status:
	./scripts/dev.sh status

dev-app:
	./scripts/dev.sh app

dev-auth:
	./scripts/dev.sh auth

dev-frontend:
	./scripts/dev.sh frontend


