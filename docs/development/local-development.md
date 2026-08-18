# 本地源码开发

## 1. 适用场景

本地模式让 PostgreSQL、Redis、DocReader、Milvus、Neo4j、Langfuse 和 Mock SAML IdP
运行在 Docker 中，Go 后端和 Vite 前端直接运行当前工作区源码。修改 Go 或 Vue
代码后无需重建应用镜像。

知识库上传文件默认写入 `.local-data/files`。业务 MinIO 不在默认启动列表中；
Langfuse 使用的独立 MinIO 只保存 Langfuse 事件和媒体，不能当作知识库对象存储。

## 2. 环境要求

- Windows 10/11 或 Linux。
- Docker Desktop / Docker Engine 与 Compose v2。
- Git Bash。在 Windows 上推荐使用 `C:\Program Files\Git\bin\bash.exe`。
- Go 1.26。
- Node.js 24 和 npm。
- Windows 本地编译 Go 时需要支持 CGO 的 GCC，因为 DuckDB Go binding 使用 CGO。

DocReader 的 Python 依赖由 Docker 镜像提供，不要求在宿主机安装 Python 环境。

## 3. 首次配置

```bash
cp .env.local.example .env.local
```

开发凭据只允许用于本机。至少检查：

- `DB_PASSWORD`
- `REDIS_PASSWORD`
- `JWT_SECRET`
- `SYSTEM_AES_KEY`
- `RETRIEVE_DRIVER=milvus`
- `MILVUS_ADDRESS=milvus:19530`

本地 Go 进程由脚本将容器服务名转换为宿主机地址。

## 4. 一条命令启动

在 Git Bash 中：

```bash
bash ./scripts/dev-all.sh
```

PowerShell 中：

```powershell
& "C:\Program Files\Git\bin\bash.exe" "F:/RocheKAP/scripts/dev-all.sh"
```

启动结果：

| 服务 | 地址 |
|---|---|
| 前端 | `http://localhost:5173` |
| 后端健康检查 | `http://localhost:8080/health` |
| Swagger | `http://localhost:8080/swagger/index.html`，要求非 release 模式 |
| Mock SAML IdP | `http://127.0.0.1:8091` |
| Langfuse | `http://localhost:3000` |
| Milvus WebUI | `http://localhost:9091/webui/` |
| Neo4j | `http://localhost:7474` |

脚本前台作为 supervisor 运行。保持终端打开可监控前后端进程。

## 5. 一条命令停止

```bash
bash ./scripts/dev-all.sh stop
```

或：

```bash
make dev-all-stop
```

停止不会删除 Docker Volume，不会清空 PostgreSQL、Milvus、Neo4j、MinIO 或
Langfuse 数据。

## 6. 分开启动

启动基础设施：

```bash
make dev-start
```

新终端启动后端：

```bash
make dev-app
```

新终端启动前端：

```bash
make dev-frontend
```

只停止基础设施：

```bash
make dev-stop
```

### 6.1 连接共享服务器开发基础设施

当本地代码需要直接使用 `10.3.97.217` 上的开发 PostgreSQL、Redis、
Milvus、DocReader、Neo4j 和 Langfuse 时，使用独立环境文件，
不要修改 `.env.local`：

```powershell
Copy-Item .env.remote-dev.example .env.remote-dev
.\scripts\remote-dev.ps1 check
```

后端直接在 Windows 本机运行：

```powershell
.\scripts\remote-dev.ps1 app-host
```

后端运行在本地 Docker：

```powershell
.\scripts\remote-dev.ps1 app-docker
```

Docker 模式的日志和停止命令：

```powershell
.\scripts\remote-dev.ps1 logs-docker
.\scripts\remote-dev.ps1 down-docker
```

两种模式均直接连接服务器内网 IP，不需要 SSH 隧道；`AUTO_MIGRATE=true`
会在后端启动时对共享开发数据库执行自动迁移。服务器开发 Compose 需要公开
`5432`、`6379`、`19530`、`50051`、`7687` 和 `9000`，其中 DocReader
由 `DOCREADER_BIND=0.0.0.0`、`DOCREADER_PORT=50051` 控制。

当前共享服务器的业务文件存储实际为 `STORAGE_TYPE=local`，因此远程开发配置
也保持 local。PG/Milvus 中的问答数据可共享，但服务器已有文件的原文件预览不会
自动挂载到本地 Docker；如需完全共享原文件，应先将服务器业务存储统一迁移到
MinIO，而不是只修改本地 `STORAGE_TYPE`。

## 7. 代码修改后的生效方式

| 修改 | 操作 |
|---|---|
| Vue/TypeScript/CSS | Vite 自动热更新 |
| Go 后端（Windows Git Bash） | 停止 `make dev-app` 后重新运行 |
| Go 后端（Linux/macOS 且已安装 Air） | Air 自动重新编译并重启 |
| DocReader Python | 重建并重启 `docreader` 容器 |
| `.env.local` | 重启受影响的进程或容器 |
| 数据库迁移 | 重启后端，启动时自动执行 |
| Swagger 注释 | `make docs` 后刷新 Swagger UI |

后端和前端日志：

```text
logs/dev-backend.log
logs/dev-frontend.log
```

## 8. 常用诊断

```bash
curl http://127.0.0.1:8080/health
docker compose --env-file .env.local -f docker-compose.local.yml ps
docker compose --env-file .env.local -f docker-compose.local.yml logs -f docreader
```

## 9. 保留与清空数据

普通停止不会删除数据。删除 Volume 也不会删除默认位于
`.local-data/files` 的知识库文件；如果需要彻底重置，还应单独确认并处理该目录。
只有明确需要全新测试库时才执行带 Volume 删除的操作：

```bash
docker compose --env-file .env.local -f docker-compose.local.yml down -v
```

该命令会永久删除本地开发数据，执行前必须确认没有需要保留的文档和账号。

## 10. 可选使用 MinIO

需要测试与服务器一致的对象存储时：

1. 将 `.env.local` 中 `STORAGE_TYPE` 改为 `minio`。
2. 配置 `MINIO_ENDPOINT=localhost:9000`、访问密钥和 Bucket。
3. 启动时追加 `--minio`。

```bash
bash ./scripts/dev-all.sh --minio
```

此时业务 MinIO 控制台为 `http://localhost:9001`。
