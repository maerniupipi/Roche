# 测试与调试

## 1. 后端验证

```bash
go test ./...
go vet ./...
go build ./cmd/server
```

只运行一个包：

```bash
go test ./internal/application/service -run TestName -v
```

## 2. 前端验证

```bash
cd frontend
npm run type-check
npm test
npm run build
```

## 3. VS Code 调试后端

创建 `.vscode/launch.json`：

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug RocheKAP backend",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/server",
      "cwd": "${workspaceFolder}",
      "envFile": "${workspaceFolder}/.env.local"
    }
  ]
}
```

先运行：

```bash
make dev-start
```

再按 F5。不要同时运行 `make dev-app`，否则 8080 端口冲突。

## 4. 调试异步知识构建

异步不代表不能调试。Worker 与 API 默认在同一个 Go 进程，断点可以设置在：

- `internal/application/service/knowledge_create.go`
- `internal/application/service/knowledge_process.go`
  - `ProcessDocument`
  - `ProcessSummaryGeneration`
  - `ProcessQuestionGeneration`
- `internal/application/service/image_multimodal.go`
- `internal/application/service/extract.go`
- `internal/application/repository/retriever/milvus/repository.go`

上传请求返回后，任务进入 Redis。保持调试进程运行，Worker 取到任务时会命中断点。
如果任务已被其他进程消费，先停止其他 app/worker 实例再重试。

## 5. 调试 Agent

建议断点：

- `internal/handler/session/qa.go`
- `internal/application/service/session_agent_qa.go`
- `internal/agent/` 中 Agent loop
- `internal/agent/tools/` 中目标工具
- `internal/application/service/session_knowledge_qa.go`

检查顺序：

1. 当前用户身份是否正确。
2. 会话是否属于该用户。
3. Agent 配置和工具白名单。
4. 有效知识授权范围。
5. 工具输入参数。
6. Milvus/Neo4j 返回。
7. 重排和引用。
8. 写入 `messages.agent_steps` 和 Langfuse。

## 6. 数据库调试

本地 PostgreSQL：

```text
host: 127.0.0.1
port: 5432
database: RocheKAP
user: postgres
password: 读取 .env.local
schema: public
```

表位于 `RocheKAP -> Schemas -> public -> Tables`。`postgres`、`langfuse` 等是同一
PostgreSQL 实例中的不同数据库，不是应用重复表。

## 7. Swagger 验证

```bash
make install-swagger
make docs
```

然后验证：

```bash
go test ./...
```

开发模式访问 `/swagger/index.html`。如 JSON 无法解析，优先检查 Swag 注释中的
未转义引号、重复 method/path 和错误的 `/api/v1` 前缀。

## 8. 常见故障

| 现象 | 检查 |
|---|---|
| 登录显示网络错误 | 后端 8080、Vite proxy、CORS、前端 API base |
| app 长时间 Waiting | `docker logs roche-kap-app` 和健康检查 start period |
| 文档一直 processing | Redis、Worker、processing spans、DocReader |
| 文档解析失败 | DocReader 日志、文件大小、parser rule、MinerU 配置 |
| 检索无结果 | 用户授权、knowledge enable、Chunk、Milvus collection、模型维度 |
| 引用加载失败 | chunk/knowledge 权限、软删除、引用 ID、文件签名 URL |
| SAML 登录提示 ACS/Audience 不匹配 | IdP 中注册的 ACS URL、SP Entity ID 与当前 Auth Service 配置必须逐字符匹配 |
| 授权弹窗显示纯文本 404 | Vite 已更新但 Go 后端仍是旧进程；重启后端并确认迁移版本至少为 2 |
| 目录删除返回 409 | 删除过程中出现活动文档/并发上传；刷新目录并重试，检查上传任务 |

### 判断后端是否为最新代码

`GET /health` 只能证明端口上的进程活着。修改 Go 路由后，Vite 的热更新不会自动
替换旧后端。可用已登录请求检查新路由：

```text
GET /api/v1/knowledge-bases/{kb_id}/resources/knowledge_base/{kb_id}/grants
```

- `200`：新路由存在且有管理权限。
- `401/403`：新路由存在，但身份或权限不满足。
- 纯文本 `404 page not found`：通常仍是旧 Go 进程。

数据库迁移表 `schema_migrations` 的版本应至少为 `2`，且 `dirty=false`。
