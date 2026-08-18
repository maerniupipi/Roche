# RocheKAP 数据库表全景目录

## 1. 文档基线

本文档目录以 2026-07-29 当前 `feature/tjj` 分支和本地运行中的 PostgreSQL 为基线：

- 数据库：`WeKnora`
- PostgreSQL 镜像：`postgres:17.10-bookworm`
- 数据库迁移版本：`79`
- 迁移状态：`dirty = false`
- `public` Schema 实表：40 张
- 应用及运行表：39 张
- 迁移框架表：1 张（`schema_migrations`）

字段清单通过以下三处交叉确认：

1. `migrations/versioned/*.up.sql`
2. `internal/types/*.go`
3. PostgreSQL `information_schema.columns`

因此，本目录描述的是“所有 up migration 执行完毕后的最终状态”，不是历史上曾经出现过的表。

## 2. 功能分类

| 功能目录 | 表数量 | 内容 |
|---|---:|---|
| [01-identity-and-access](./01-identity-and-access/README.md) | 6 | 用户、登录令牌、OIDC 身份、知识域管理员、知识库和文档授权 |
| [02-organization-and-integration](./02-organization-and-integration/README.md) | 6 | 企业组织树、员工归属、Workday 等外部组织与同步记录 |
| [03-platform-configuration](./03-platform-configuration/README.md) | 7 | 知识域、模型、向量存储、平台运行参数、Web 搜索配置 |
| [04-knowledge-core](./04-knowledge-core/README.md) | 7 | 知识库、目录、文档、Chunk、标签和构建过程 Span |
| [05-agent-and-conversation](./05-agent-and-conversation/README.md) | 3 | 智能体、会话、消息和引用 |
| [06-data-source-sync](./06-data-source-sync/README.md) | 2 | Google Drive 等数据源及同步日志 |
| [07-mcp](./07-mcp/README.md) | 4 | MCP 服务、工具审批和 OAuth 数据 |
| [08-user-experience-and-audit](./08-user-experience-and-audit/README.md) | 3 | 收藏、知识库置顶和审计日志 |
| [09-operations](./09-operations/README.md) | 2 | 异步任务死信和数据库迁移版本 |

## 3. 全部表清单

| # | 表名 | 核心用途 |
|---:|---|---|
| 1 | `users` | 本地用户主档 |
| 2 | `auth_tokens` | 登录/刷新令牌状态 |
| 3 | `sso_identities` | OIDC/PingIdentity 身份与本地用户映射 |
| 4 | `knowledge_domain_admins` | 用户与知识域的管理员分配 |
| 5 | `knowledge_base_grants` | 整个知识库的显式读取授权 |
| 6 | `knowledge_grants` | 单个知识文档的显式读取授权 |
| 7 | `org_units` | 企业组织树 |
| 8 | `user_org_memberships` | 用户与企业组织节点的关系 |
| 9 | `external_org_units` | Workday 等外部组织投影 |
| 10 | `external_workers` | Workday 等外部员工投影 |
| 11 | `integration_sync_runs` | 企业集成同步批次和游标 |
| 12 | `integration_events` | 外部事件幂等及处理状态 |
| 13 | `knowledge_domains` | 知识管理域，不是企业 HR 部门 |
| 14 | `knowledge_domain_storage` | 知识域存储配额和已用空间 |
| 15 | `platform_runtime_configs` | 全平台唯一的解析、存储和检索配置 |
| 16 | `models` | LLM、Embedding、Rerank、VLM、ASR 模型配置 |
| 17 | `vector_stores` | Milvus 等向量存储连接配置 |
| 18 | `web_search_providers` | Web 搜索服务配置 |
| 19 | `system_settings` | 可在运行时覆盖的系统设置 |
| 20 | `knowledge_bases` | 知识库主表 |
| 21 | `knowledge_folders` | 知识库内部目录树 |
| 22 | `knowledges` | 文档、FAQ 或手工知识主记录 |
| 23 | `chunks` | 文档切片及图片、父子关系 |
| 24 | `knowledge_tags` | 知识库内标签定义 |
| 25 | `knowledge_tag_relations` | 文档与标签多对多关系 |
| 26 | `knowledge_processing_spans` | 文档构建流水线阶段明细 |
| 27 | `custom_agents` | 内置或自定义智能体配置 |
| 28 | `sessions` | 对话会话 |
| 29 | `messages` | 用户和智能体消息、引用、工具步骤 |
| 30 | `data_sources` | Google Drive 等外部内容源配置 |
| 31 | `sync_logs` | 数据源同步执行结果 |
| 32 | `mcp_services` | MCP 服务连接配置 |
| 33 | `mcp_tool_approvals` | MCP 工具是否需要人工审批 |
| 34 | `mcp_oauth_clients` | MCP OAuth 客户端注册信息 |
| 35 | `mcp_oauth_tokens` | MCP OAuth 的主体级令牌 |
| 36 | `user_resource_favorites` | 用户收藏知识库或智能体 |
| 37 | `user_kb_pins` | 用户个人知识库置顶 |
| 38 | `audit_logs` | 权限及管理操作审计 |
| 39 | `task_dead_letters` | 异步任务最终失败归档 |
| 40 | `schema_migrations` | 数据库迁移版本和 dirty 状态 |

## 4. 最重要的关系

```text
knowledge_domains
  ├─ knowledge_bases
  │    ├─ knowledge_folders
  │    ├─ knowledges
  │    │    ├─ chunks
  │    │    ├─ knowledge_tag_relations ── knowledge_tags
  │    │    ├─ knowledge_processing_spans
  │    │    └─ knowledge_grants
  │    └─ knowledge_base_grants
  ├─ knowledge_domain_admins ── users
  ├─ custom_agents
  ├─ sessions ── messages
  └─ data_sources ── sync_logs

org_units
  ├─ org_units（父子树）
  ├─ user_org_memberships ── users
  └─ external_org_units

external_workers ── users
```

## 5. `knowledge_domain_id` 的含义

所有 `knowledge_domain_id` 字段均指向知识管理域：

```text
knowledge_domain_id 实际表示 knowledge_domains.id
```

它不等于企业组织部门。企业组织使用：

```text
org_units.id
user_org_memberships.org_unit_id
```

## 6. 不属于 PostgreSQL table 的数据

| 存储 | 保存内容 | 为什么不在本表目录中 |
|---|---|---|
| Milvus | Chunk 的索引文本、向量及检索过滤字段 | 是 Collection，不是 PostgreSQL table |
| Neo4j | 知识图谱实体、关系和来源 Chunk 关联 | 是图节点和边，不是关系表 |
| S3/MinIO/本地存储 | 原始文件、解析图片及附件对象 | 是对象，不是数据库行 |
| Redis | 异步队列、流和缓存 | 是内存键值数据 |
| DuckDB | Excel/CSV 数据分析时的临时表 | 会话结束后通常释放 |
| Langfuse PostgreSQL/ClickHouse | 独立 Langfuse 服务的数据 | 不属于本应用数据库 |

## 7. Mock 数据约定

- 所有 ID、邮箱、Token、密钥均为虚构示例。
- 时间统一使用 ISO 8601。
- 加密字段用 `enc:v1:mock-ciphertext` 表示，不提供真实密钥。
- Mock 行主要用于解释字段协作，不保证可以不经关联数据准备直接执行 INSERT。
