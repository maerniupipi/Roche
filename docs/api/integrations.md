# Integration API

## 1. Workday

Workday 适配器把外部组织和员工投影到本地企业目录，不直接创建知识权限。

### 触发同步

```http
POST /api/v1/system/admin/integrations/workday/sync
Authorization: Bearer <system-admin-token>
Content-Type: application/json
X-Trace-ID: optional-trace-id

{
  "mode": "incremental"
}
```

`mode` 允许：

| 值 | 含义 |
|---|---|
| `incremental` | 使用上次成功游标继续拉取 |
| `full` | 从空游标开始，并在成功结束后停用本次未出现的外部投影 |

返回 `202 Accepted` 和一条 `integration_sync_runs` 记录。任务由异步 Worker 执行。

### 查询运行

```text
GET /system/admin/integrations/workday/runs?offset=0&limit=20
GET /system/admin/integrations/workday/runs/{run_id}
```

状态为 `pending`、`running`、`succeeded` 或 `failed`。`counters` 记录读取、变化、
成功关联用户和未匹配员工数量。

### 事件入口

```http
POST /api/v1/system/admin/integrations/workday/events
Authorization: Bearer <system-admin-token>
Content-Type: application/json

{
  "external_event_id": "wd-event-1001",
  "event_type": "worker.changed",
  "payload": {}
}
```

当前端点仍要求系统管理员 JWT，并不是可直接暴露给 Workday 的公网 Webhook。
接入真实 Webhook 前必须增加供应商签名校验，再决定是否加入认证白名单。

## 2. 数据源与 Google Drive

Google Drive 使用通用数据源 API：

```text
GET    /datasource/types
POST   /datasource/validate-credentials
POST   /datasource
GET    /datasource
GET    /datasource/{id}
PUT    /datasource/{id}
DELETE /datasource/{id}
PUT    /datasource/{id}/credentials
DELETE /datasource/{id}/credentials/{field}
POST   /datasource/{id}/validate
GET    /datasource/{id}/resources
POST   /datasource/{id}/resource-ancestors
POST   /datasource/{id}/sync
POST   /datasource/{id}/pause
POST   /datasource/{id}/resume
GET    /datasource/{id}/logs
GET    /datasource/logs/{log_id}
```

标准流程：

1. `GET /datasource/types` 获取 Google Drive 所需配置模式。
2. `POST /datasource/validate-credentials` 临时验证服务账号或 OAuth 凭据。
3. `POST /datasource` 保存非敏感配置并绑定目标知识库。
4. `PUT /datasource/{id}/credentials` 写入加密凭据。
5. 查询资源并选择同步范围。
6. `POST /datasource/{id}/sync` 触发首次同步。
7. 通过 logs 接口观察每次同步结果。

同步拉取的文件进入与本地上传相同的解析、切片和 Milvus 索引流水线。删除或覆盖策略
以数据源配置和当前实现为准，不反向删除 Google Drive 源文件。

## 3. MinerU 与 DocReader

MinerU 没有直接暴露给浏览器的业务 API。文档上传后由后台 Worker 根据知识库解析
规则调用 DocReader 或 MinerU：

```text
HTTP upload
  -> object storage
  -> PostgreSQL knowledge
  -> Redis/Asynq task
  -> parser adapter
  -> normalized ReadResult
  -> chunk/index pipeline
```

外部解析凭据和地址必须配置在服务端。浏览器只负责上传文件和查询处理状态。

## 4. Langfuse

Langfuse 是可观测性集成，没有平台业务 CRUD API。启用环境变量后，Gin 中间件、
LLM 调用、检索和异步任务会记录 trace/span/generation。

建议用以下键关联一次请求：

| 键 | 来源 |
|---|---|
| `trace_id` | `X-Trace-ID`、`X-Request-ID` 或服务端生成值 |
| `session_id` | 对话 URL 路径 |
| `user_id` | JWT 解析后的本地用户 |
| `knowledge_id` | 文档处理任务 |
| `run_id` | Workday 或其他集成运行 |

Langfuse 失败不得阻断知识问答主链路。

## 5. 外部调用安全

- 所有外部 URL 必须经过 SSRF 校验。
- 凭据只通过专用 credentials 接口写入，不进入普通 GET 响应。
- 生产流量必须使用 TLS。
- 回调和 Webhook 必须校验 state、nonce、签名或共享密钥。
- 连接测试和同步操作必须审计。
- 原始 Workday 敏感 payload 不写普通业务表。
