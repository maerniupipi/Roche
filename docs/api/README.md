# API Reference

本文档描述 Gateway 后方的 Auth Service 与 Application Backend API。业务路由实现以
`internal/router/router.go` 为准，认证路由实现以 `internal/authserver/router.go` 为准。

## 1. 访问入口

| 项目 | 地址 |
|---|---|
| API 根路径 | `/api/v1` |
| 健康检查 | `GET /health` |
| Swagger UI | `GET /swagger/index.html`，仅非 `release` 模式开放 |
| OpenAPI JSON | `GET /swagger/doc.json`，仅非 `release` 模式开放 |

本地源码开发的默认地址：

```text
http://localhost:8088/api/v1
```

除开发注册/登录、SAML/OIDC 启动与回调、刷新令牌等公开认证接口外，请求都需要平台 JWT：

```http
Authorization: Bearer <access_token>
Content-Type: application/json
X-Request-ID: optional-client-request-id
```

`X-Request-ID` 用于日志与 Langfuse 链路关联。企业集成接口还接受
`X-Trace-ID`，若两者同时存在，Workday 同步优先使用 `X-Trace-ID`。

## 2. 通用响应

普通成功响应通常采用以下信封：

```json
{
  "success": true,
  "data": {}
}
```

列表接口可能同时返回分页字段：

```json
{
  "success": true,
  "data": [],
  "total": 0
}
```

失败响应由统一错误中间件生成：

```json
{
  "success": false,
  "error": {
    "code": 1002,
    "message": "permission denied",
    "details": null
  }
}
```

常用错误码：

| HTTP | 业务码 | 含义 |
|---|---:|---|
| 400 | 1000 / 1010 | 请求或字段校验失败 |
| 401 | 1001 | 未登录、JWT 无效或已过期 |
| 403 | 1002 | 已登录但没有资源权限 |
| 404 | 1003 | 资源不存在，或调用者不可见 |
| 409 | 1005 | 状态冲突或重复资源 |
| 429 | 1006 | 频率或配额限制 |
| 500 | 1007 | 服务内部错误 |
| 503 | 1008 | 下游服务暂不可用 |

客户端不得通过解析错误文本判断业务分支，应优先使用 HTTP 状态码和
`error.code`。

## 3. 权限模型

当前系统只有三类业务身份：

| 身份 | API 能力 |
|---|---|
| 系统管理员 | 平台配置、模型、知识域、组织目录、Workday、所有知识资源 |
| 知识域管理员 | 管理被指派知识域及其知识库、文档和授权 |
| 普通用户 | 读取被显式授权的知识库或文档，并使用 Agent 问答 |

知识访问来自统一的 `knowledge_resource_grants`。资源可以是知识库、目录或文档，
授权主体可以是 `user` 或 `org_unit`，支持 `read/manage` 与 `allow/deny`。企业
组织成员关系本身不自动产生访问权。

Agent 没有独立的知识授权。普通 RAG 请求中的 `knowledge_base_ids`、
`knowledge_ids` 和标签会与用户范围求交集。Agent 当前以用户全部有效授权范围
启动，智能体绑定知识库和本轮 `@知识库` 不参与初始范围；具体工具参数只能继续
缩小，不能扩大权限。

完整规则见 [Security and Permissions](../architecture/security-permissions.md)。

## 4. 接口分组

| 分组 | 说明 | 详细文档 |
|---|---|---|
| Authentication | 独立 Auth Service、PingIdentity SAML、OIDC Mock、平台 Token | [authentication.md](authentication.md) |
| Knowledge Management | 知识域、知识库、文档、Chunk、FAQ、标签 | [knowledge-management.md](knowledge-management.md) |
| Chat and Agent | 会话、消息、RAG、Agent SSE、检索工具 | [chat-agent.md](chat-agent.md) |
| Administration | 模型、运行配置、组织、显式授权、系统状态 | [administration.md](administration.md) |
| Integrations | Workday、Google Drive 数据源、外部解析、Langfuse | [integrations.md](integrations.md) |

## 5. 完整路由目录

以下是当前稳定业务接口。MCP 与 Skills 是可选功能，只有开关启用时才注册路由。

| 资源 | 方法与路径 |
|---|---|
| Auth | `GET /auth/registration/config`; `POST /auth/register`; `POST /auth/login`; `GET /auth/saml/config`; `GET /auth/saml/url`; `GET/POST /auth/saml/acs`; `GET /auth/saml/metadata`; OIDC development endpoints; refresh, validate, logout, current user and preferences |
| Knowledge domains | `POST/GET /knowledge-domains`; `GET /knowledge-domains/all`; `GET /knowledge-domains/search`; `GET/PUT/DELETE /knowledge-domains/{id}`; administrators and audit-log subresources |
| Knowledge bases | `POST/GET /knowledge-bases`; `GET/PUT/DELETE /knowledge-bases/{id}`; pin, hybrid-search, copy and move-target endpoints |
| Knowledge | file, URL and manual create endpoints; list, folder, detail, update, delete, preview, download, reparse, cancel, move and batch endpoints |
| Tags and FAQ | `/knowledge-bases/{id}/tags`; `/knowledge-bases/{id}/faq/*` |
| Chunks | `/chunks/{knowledge_id}`; `/chunks/by-id/{id}`; chunk update/delete and generated-question delete |
| Sessions | `POST/GET /sessions`; session detail, update, delete, message clear, title, stop, pin and stream continuation |
| Chat | `POST /knowledge-chat/{session_id}`; `POST /agent-chat/{session_id}`; `POST /knowledge-search` |
| Agents | `/agents`; `/agents/{id}`; copy, placeholders, presets and suggested questions |
| Enterprise directory | `/enterprise/org-units`; members; user memberships; grant-user search |
| Resource ACL | `/knowledge-bases/{id}/resources/{resource_type}/{resource_id}/grants`; grant subjects; grant revoke; recursive folder delete |
| Models | `/models`; providers, debug and credential subresources |
| Vector stores | `/vector-stores`; type and connectivity test endpoints |
| Data sources | `/datasource`; credential validation, resource selection, sync control and logs |
| Workday | `/system/admin/integrations/workday/sync`; runs and authenticated event endpoint |
| System | `/system/info`; parser and storage status; platform runtime config; system administrator settings and audit |
| Optional MCP | `/mcp-services`; tools, resources, credentials, OAuth and approvals |
| Optional Skills | `GET /skills` |

## 6. 异步操作

文档解析、知识库复制、文档批处理、数据源同步、Workday 同步均可能异步执行。
创建接口返回成功只表示任务已接收，不等于内容已经可检索。

客户端应通过相应状态接口轮询：

| 操作 | 状态接口 |
|---|---|
| 单文档解析 | `GET /knowledge/{id}`、`GET /knowledge/{id}/stages`、`GET /knowledge/{id}/spans` |
| 知识库复制 | `GET /knowledge-bases/copy/progress/{task_id}` |
| 文档移动 | `GET /knowledge/move/progress/{task_id}` |
| FAQ 导入 | `GET /faq/import/progress/{task_id}` |
| 数据源同步 | `GET /datasource/{id}/logs` |
| Workday 同步 | `GET /system/admin/integrations/workday/runs/{run_id}` |

## 7. 版本与变更规则

当前 API 主版本固定在 `/api/v1`。对现有字段增加可选字段属于兼容变更；删除字段、
改变字段语义或更改权限规则属于破坏性变更，应升级 API 主版本并同步更新：

1. `internal/router/router.go`
2. Handler Swagger 注解
3. 本目录中的调用说明
4. `docs/swagger.json`、`docs/swagger.yaml` 和 `docs/docs.go`
