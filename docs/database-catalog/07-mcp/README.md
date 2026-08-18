# 07 MCP 服务与 OAuth

本组共 4 张表：

```text
mcp_services
├─ mcp_tool_approvals
├─ mcp_oauth_clients
└─ mcp_oauth_tokens
```

MCP 功能可以在产品层关闭，但这些表当前仍属于数据库最终 Schema。

## 1. `mcp_services`

**用途：** 注册一个可供智能体连接的 MCP Server，包括传输方式、地址、认证和进程启动参数。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | MCP 服务 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `name` | varchar | 服务显示名称。 |
| `description` | text | 服务用途说明。 |
| `enabled` | boolean | 是否允许发现和调用该服务工具。 |
| `transport_type` | varchar | `sse`、`http-streamable` 或 `stdio`。 |
| `url` | text | HTTP/SSE 服务地址；`stdio` 模式可为空。 |
| `headers` | jsonb | 自定义 HTTP Header，敏感值应加密或引用 Secret。 |
| `auth_config` | jsonb | Bearer、OAuth 等认证配置。 |
| `advanced_config` | jsonb | 超时、重试、TLS 等扩展配置。 |
| `stdio_config` | jsonb | `stdio` 服务的命令和参数。 |
| `env_vars` | jsonb | `stdio` 子进程环境变量。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `is_builtin` | boolean | 是否为系统内置 MCP 服务。 |

**Mock 数据：**

```json
{
  "id": "mcp-service-docops-001",
  "knowledge_domain_id": 1,
  "name": "Document Operations",
  "description": "企业文档审批查询服务",
  "enabled": true,
  "transport_type": "http-streamable",
  "url": "https://mcp.example.com/mcp",
  "headers": {"X-Client": "knowledge-hub"},
  "auth_config": {"type": "oauth2", "client_id_ref": "mcp-client-docops-001"},
  "advanced_config": {"timeout_seconds": 30, "max_retries": 2},
  "stdio_config": null,
  "env_vars": {},
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null,
  "is_builtin": false
}
```

## 2. `mcp_tool_approvals`

**用途：** 为某个 MCP 服务的具体工具保存“调用前是否需要用户批准”的策略。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK | 审批策略记录 ID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `service_id` | varchar(36), FK | 对应 `mcp_services.id`，服务删除时级联删除。 |
| `tool_name` | varchar | MCP Server 暴露的工具名。 |
| `require_approval` | boolean | `true` 表示每次或按客户端策略请求用户确认。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |

**Mock 数据：**

```json
{
  "id": 71,
  "knowledge_domain_id": 1,
  "service_id": "mcp-service-docops-001",
  "tool_name": "submit_approval",
  "require_approval": true,
  "created_at": "2026-07-29T09:05:00+08:00",
  "updated_at": "2026-07-29T09:05:00+08:00"
}
```

## 3. `mcp_oauth_clients`

**用途：** 保存平台作为 OAuth Client 访问某个 MCP 服务时使用的客户端注册信息。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | OAuth Client 配置 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `service_id` | varchar(36), FK | 对应 MCP 服务，服务删除时级联删除。 |
| `client_id` | text | OAuth Provider 分配的 Client ID。 |
| `client_secret` | text | 加密保存的 Client Secret。 |
| `redirect_uri` | text | OAuth 回调地址。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |

**Mock 数据：**

```json
{
  "id": "mcp-client-docops-001",
  "knowledge_domain_id": 1,
  "service_id": "mcp-service-docops-001",
  "client_id": "knowledge-hub-mcp-client",
  "client_secret": "<encrypted>",
  "redirect_uri": "https://knowledge.example.com/api/mcp/oauth/callback",
  "created_at": "2026-07-29T09:10:00+08:00",
  "updated_at": "2026-07-29T09:10:00+08:00"
}
```

## 4. `mcp_oauth_tokens`

**用途：** 保存完成 OAuth 授权后获得的 Access Token 和 Refresh Token，可按用户或其他主体隔离。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | Token 记录 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `user_id` | varchar(36), nullable | 兼容的用户归属字段。 |
| `service_id` | varchar(36), FK | 对应 MCP 服务，服务删除时级联删除。 |
| `access_token` | text | 加密后的 OAuth Access Token。 |
| `refresh_token` | text | 加密后的 OAuth Refresh Token。 |
| `token_type` | varchar | 通常为 `Bearer`。 |
| `expires_at` | timestamptz | Access Token 过期时间。 |
| `created_at` | timestamptz | 首次授权时间。 |
| `updated_at` | timestamptz | 刷新 Token 后的更新时间。 |
| `principal_type` | varchar | Token 所属主体类型，例如 `user`。 |
| `principal_id` | varchar(36) | 对应主体 ID。 |

**Mock 数据：**

```json
{
  "id": "mcp-token-001",
  "knowledge_domain_id": 1,
  "user_id": "usr-viewer-001",
  "service_id": "mcp-service-docops-001",
  "access_token": "<encrypted>",
  "refresh_token": "<encrypted>",
  "token_type": "Bearer",
  "expires_at": "2026-07-29T12:00:00+08:00",
  "created_at": "2026-07-29T10:00:00+08:00",
  "updated_at": "2026-07-29T10:00:00+08:00",
  "principal_type": "user",
  "principal_id": "usr-viewer-001"
}
```

## 安全提示

- `client_secret`、`access_token`、`refresh_token` 不应以明文写日志。
- MCP 服务删除后，审批、Client 和 Token 记录会随外键级联删除。
- `require_approval` 是操作确认策略，不替代服务端鉴权和知识权限检查。
