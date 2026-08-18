# PostgreSQL 表结构

本文按功能列出执行全部版本化迁移后的 38 张应用表。结构基线是：

1. `migrations/versioned/000000_init.up.sql`
2. `migrations/versioned/000001_global_vector_stores.up.sql`
3. `migrations/versioned/000002_knowledge_resource_grants.up.sql`

迁移框架另维护 `schema_migrations`，不计入 38 张应用表。

## 1. 身份与登录

### `users`

系统用户主档。开发密码、Dex OIDC 或 PingIdentity SAML 登录最终都映射到这里。

字段：`id`、`username`、`email`、`password_hash`、`avatar`、`is_active`、
`preferences`、`is_system_admin`、`created_at`、`updated_at`、`deleted_at`。

- `password_hash` 只保存哈希；纯 SSO 用户使用不可用于生产密码登录的占位哈希。
- `is_system_admin` 是平台最高权限。
- 不再包含 tenant/default space 字段。

示例：

```json
{"id":"u-1","username":"Alice","email":"alice@example.com","is_active":true,"is_system_admin":false}
```

### `auth_tokens`

平台 Access/Refresh Token 状态。字段：`id`、`user_id`、`token`、`token_type`、
`expires_at`、`is_revoked`、`created_at`、`updated_at`。由登录、刷新和注销流程写入。

### `sso_identities`

SAML/OIDC 外部身份映射。字段：`id`、`user_id`、`provider`、`issuer`、`subject`、
`created_at`、`updated_at`、`last_login_at`。

唯一身份键包含 `provider + issuer + subject`；email 只用于首次辅助匹配，不应作为永久 SSO 主键。

## 2. 知识域与平台配置

### `knowledge_domains`

知识管理分组，不是企业组织。字段：`id`、`code`、`name`、`description`、
`status`、`created_at`、`updated_at`、`deleted_at`。由系统管理员创建。

### `knowledge_domain_admins`

知识域管理员任命。字段：`id`、`knowledge_domain_id`、`user_id`、`granted_by`、
`created_at`、`updated_at`。同一用户可管理多个知识域。

### `knowledge_domain_storage`

知识域存储配额。字段：`knowledge_domain_id`、`storage_quota`、`storage_used`、
`created_at`、`updated_at`。上传/删除文件时更新使用量。

### `platform_runtime_configs`

全平台单例运行配置，`id` 固定为 1。字段：

- `retriever_engines`：检索引擎定义。
- `context_config`：上下文扩展配置。
- `web_search_config`：网络搜索配置。
- `parser_engine_config`：DocReader/MinerU 等解析配置。
- `storage_engine_config`：local/MinIO/S3 等存储配置。
- `retrieval_config`：TopK、阈值、Rerank 和 RRF。
- `created_at`、`updated_at`。

只有系统管理员可修改。模型凭据不应明文写进普通配置响应。

### `system_settings`

通用平台设置。字段：`id`、`key`、`value`、`value_type`、`category`、
`description`、`is_secret`、`requires_restart`、`last_modified_by`、时间字段。

### `models`

全平台模型目录。字段：`id`、`name`、`display_name`、`type`、`source`、
`description`、`parameters`、`is_default`、`status`、`is_builtin`、`managed_by`、
时间和软删除字段。`parameters` 保存端点与非敏感参数，敏感字段由凭据接口处理。

### `vector_stores`

向量存储连接配置。字段：`id`、`name`、`engine_type`、`connection_config`、
`index_config`、`knowledge_domain_id`、时间和软删除字段。标准部署
`engine_type=milvus`。

### `web_search_providers`

网络搜索 Provider。字段：`id`、`knowledge_domain_id`、`name`、`provider`、
`description`、`parameters`、`is_default`、时间和软删除字段。

## 3. 企业组织与 Workday

### `org_units`

系统使用的企业组织树。字段：`id`、`parent_id`、`code`、`name`、`status`、
`source`、`external_id`、`sort_order`、`attributes`、`created_by`、时间和软删除。

`source` 只能是 `manual`、`workday`、`bootstrap`；组织成员关系本身不自动产生
知识权限。

### `user_org_memberships`

用户与组织多对多。字段：`id`、`user_id`、`org_unit_id`、`is_primary`、
`status`、`source`、`created_at`、`updated_at`。

调岗时 Workday 更新本表；基于组织的授权随有效成员关系变化，直接用户授权不变。

### `external_org_units`

Workday 原始组织投影。字段：`id`、`provider`、`external_org_id`、
`parent_external_org_id`、`org_unit_id`、`name`、`org_type`、`status`、
`attributes`、`checksum`、`effective_from`、`effective_to`、`last_seen_at`、
时间字段。

### `external_workers`

Workday 员工投影。字段：`id`、`provider`、`external_worker_id`、`user_id`、
`primary_org_external_id`、`manager_external_worker_id`、`corporate_email`、
`worker_status`、`attributes`、`checksum`、有效期、`last_seen_at` 和时间字段。

`external_worker_id` 是稳定就业身份键；企业邮箱只辅助关联本地 `users`。

### `integration_sync_runs`

企业同步运行记录。字段：`id`、`provider`、`connection_key`、`mode`、
`cursor_before`、`cursor_after`、`status`、`counters`、`trace_id`、`error_code`、
`error_summary`、`started_at`、`finished_at`、`created_at`。

### `integration_events`

Webhook/批次事件幂等与审计。字段：`id`、`provider`、`external_event_id`、
`event_type`、`payload_hash`、`status`、`attempt_count`、`trace_id`、
`received_at`、`processed_at`、`error_summary`。唯一键是
`provider + external_event_id`。

## 4. 知识授权

### `knowledge_resource_grants`

知识库、目录和文档的统一 ACL。迁移 2 会删除旧的 `knowledge_base_grants`、
`knowledge_grants`，当前代码不再读取旧表。

| 字段 | 含义 |
|---|---|
| `id` | 自增授权记录 ID |
| `knowledge_domain_id` | 知识资源所属知识域，删除知识域时级联删除 |
| `knowledge_base_id` | 物理知识库 ID，删除知识库时级联删除 |
| `resource_type` | `knowledge_base`、`folder`、`knowledge` |
| `resource_id` | 按资源类型保存知识库、目录或文档 ID |
| `subject_type` | `user` 或 `org_unit` |
| `subject_id` | 用户 ID 或企业组织节点 ID |
| `permission` | `read` 或 `manage` |
| `effect` | `allow` 或 `deny` |
| `inherit_to_children` | 知识库/目录规则是否影响后代 |
| `granted_by` | 授权操作人；用户删除时置空 |
| `created_at`、`updated_at` | 创建和最近更新规则的时间 |

`resource_id` 和 `subject_id` 是多态引用，Service 在写入前验证实际资源和主体。
唯一键由知识库、资源、主体和权限共同组成。权限算法见
[资源层级、授权与删除](../knowledge/resource-access-and-deletion.md)。

## 5. 知识库、文档与 Chunk

### `knowledge_bases`

知识库配置。字段：

- 标识：`id`、`name`、`description`、`knowledge_domain_id`、`creator_id`、`type`。
- 处理：`chunking_config`、`image_processing_config`、`vlm_config`、
  `extract_config`、`question_generation_config`、`asr_config`。
- 模型：`embedding_model_id`、`summary_model_id`。
- 索引：`vector_store_id`、`indexing_strategy`。
- 存储：`storage_provider_config`、兼容字段 `cos_config`。
- 其他：`faq_config`、`is_temporary`、时间和软删除字段。

`knowledge_count`、`chunk_count`、`is_processing`、`creator_name`、
`my_permission`、`is_pinned` 等是查询时计算的响应字段，不在本表。

### `knowledge_folders`

知识库内部目录树。字段：`id`、`knowledge_domain_id`、`knowledge_base_id`、
`parent_id`、`name`、`relative_path`、`created_at`、`updated_at`。

目录不是独立物理知识库，但可以拥有直接 ACL。删除目录会递归删除后代目录和文档，
并通过业务服务清理外部派生数据。

### `knowledges`

一份文档/URL/手工内容的主记录。字段：

- 归属：`id`、`knowledge_domain_id`、`knowledge_base_id`、`folder_id`。
- 内容身份：`type`、`title`、`description`、`source`、`channel`。
- 状态：`parse_status`、`pending_subtasks_count`、`summary_status`、
  `enable_status`、`error_message`。
- 文件：`file_name`、`file_type`、`file_size`、`file_path`、`file_hash`、
  `storage_size`。
- 处理：`embedding_model_id`、`metadata`、`last_faq_import_result`。
- 时间：`created_at`、`updated_at`、`processed_at`、`deleted_at`。

`file_path` 是对象存储引用，`metadata` 可包含解析器属性、数据源远端 ID、手工内容
配置和本次处理覆盖项。完整文件本体不存 PostgreSQL。

### `chunks`

检索和引用的正文单元。字段：

- 标识与归属：`id`、`seq_id`、`knowledge_domain_id`、`knowledge_base_id`、
  `knowledge_id`、`tag_id`。
- 内容与顺序：`content`、`chunk_index`、`start_at`、`end_at`、
  `pre_chunk_id`、`next_chunk_id`。
- 类型与关系：`chunk_type`、`parent_chunk_id`、`relation_chunks`、
  `indirect_relation_chunks`。
- 多模态：`image_info`、`video_info`。
- 检索状态：`is_enabled`、`status`、`flags`、`content_hash`。
- 扩展：`metadata`、时间和软删除字段。

`start_at/end_at` 是解析后 Markdown 的字符边界。向量不在本表，Milvus 通过
`chunk_id` 关联。

### `knowledge_processing_spans`

知识构建阶段 Trace。字段：`id`、`knowledge_id`、`attempt`、`span_id`、
`parent_span_id`、`name`、`kind`、`status`、`input`、`output`、`metadata`、
错误字段、开始/结束/耗时和时间字段。

### `task_dead_letters`

异步最终失败任务。字段：`id`、`knowledge_domain_id`、`task_type`、`scope`、
`scope_id`、`related_id`、`payload`、`last_error`、`fail_count`、`failed_at`。

### `knowledge_tags`

知识库内标签定义。字段：`id`、`seq_id`、`knowledge_domain_id`、
`knowledge_base_id`、`name`、`color`、`sort_order`、时间和软删除字段。

### `knowledge_tag_relations`

文档与标签多对多。字段：`knowledge_id`、`tag_id`、`created_at`。

### `user_kb_pins`

用户置顶知识库。字段：`knowledge_domain_id`、`user_id`、`kb_id`、`pinned_at`。

## 6. 会话与智能体

### `custom_agents`

平台智能体定义。字段：`id`、`name`、`description`、`avatar`、`is_builtin`、
`created_by`、`config`、时间和软删除字段。`config` 保存模型、Prompt、工具和
运行参数，不保存独立知识授权。

### `sessions`

字段：`id`、`user_id`、`title`、`description`、`last_request_state`、
`is_pinned`、`pinned_at`、时间和软删除字段。

### `messages`

字段：`id`、`request_id`、`session_id`、`role`、`content`、
`rendered_content`、`knowledge_references`、`agent_steps`、`mentioned_items`、
`images`、`attachments`、`channel`、`is_completed`、`is_fallback`、
`agent_duration_ms`、时间和软删除字段。

### `user_resource_favorites`

用户收藏。字段：`user_id`、`resource_type`、`resource_id`、`created_at`。

## 7. 数据源

### `data_sources`

外部数据源配置。字段：`id`、`knowledge_domain_id`、`knowledge_base_id`、`name`、
`type`、`config`、`sync_schedule`、`sync_mode`、`status`、
`conflict_strategy`、`sync_deletions`、`last_sync_at`、`last_sync_cursor`、
`last_sync_result`、`error_message`、`sync_log_retention_days`、时间和软删除字段。

### `sync_logs`

每次数据源同步结果。字段：`id`、`data_source_id`、`knowledge_domain_id`、
`status`、开始/结束时间、total/created/updated/deleted/skipped/failed 计数、
`error_message`、`result`、时间字段。

## 8. MCP 可选功能

以下表仅在 MCP 功能开启时使用：

- `mcp_services`：服务定义、Transport、URL、Header、认证和高级配置。
- `mcp_oauth_clients`：MCP OAuth Client 配置。
- `mcp_oauth_tokens`：用户/主体的 Access 与 Refresh Token。
- `mcp_tool_approvals`：按服务和工具名配置是否要求人工审批。

这些表都含 `knowledge_domain_id` 作为管理归属，但 MCP 工具仍不能绕过当前用户
知识授权。

## 9. 级联与软删除

- `ON DELETE CASCADE` 表示删除父记录时数据库自动删除明确声明的子记录。
- `ON DELETE SET NULL` 表示父记录删除后保留子记录，但清空引用。
- `knowledge_resource_grants` 对知识域和知识库使用数据库级级联；目录、文档和多态
  主体没有固定外键，由 Service/Repository 显式清理。
- 带 `deleted_at` 的主表通常使用软删除；直接 SQL 物理删除可能绕开服务层的
  Milvus、Neo4j 和对象存储清理。
- 删除知识库/文档必须调用 API 或 Service，不应只删 PostgreSQL 行。
