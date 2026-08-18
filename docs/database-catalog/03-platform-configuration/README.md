# 03 平台与基础能力配置

本组共 7 张表：

```text
knowledge_domains
├─ knowledge_domain_storage
├─ models
├─ vector_stores
├─ web_search_providers
├─ platform_runtime_configs
└─ system_settings
```

`knowledge_domain_id` 始终表示知识管理域，不表示 Workday 企业组织节点。企业组织由 `org_units` 建模。

## 1. `knowledge_domains`

**用途：** 知识库的管理域。它用于归属知识库、模型等资源，不等同于 `org_units` 企业组织架构。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | integer, PK | 知识域内部数字 ID，由数据库生成。 |
| `name` | varchar, NOT NULL | 知识域名称，由系统管理员创建或初始化。 |
| `description` | text | 知识域说明。 |
| `status` | varchar | 启用状态，通常为 `active` 等应用状态值。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最近更新时间。 |
| `deleted_at` | timestamptz, nullable | 软删除时间；为空表示有效。 |
| `code` | varchar | 稳定业务编码，用于展示、配置或外部映射。 |

**Mock 数据：**

```json
{
  "id": 1,
  "name": "Finance Knowledge Domain",
  "description": "财务知识库管理域",
  "status": "active",
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null,
  "code": "FIN-KD"
}
```

## 2. `knowledge_domain_storage`

**用途：** 汇总知识域的文件存储配额和已使用空间。它记录容量，不保存文件本体。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `knowledge_domain_id` | integer, PK/FK | 对应 `knowledge_domains.id`，知识域删除时级联删除。 |
| `storage_quota` | bigint | 可用存储上限，单位为字节。 |
| `storage_used` | bigint | 已用空间，上传、删除文档时由服务更新。 |
| `created_at` | timestamptz | 配额记录创建时间。 |
| `updated_at` | timestamptz | 配额或已用空间最后更新时间。 |

**Mock 数据：**

```json
{
  "knowledge_domain_id": 1,
  "storage_quota": 107374182400,
  "storage_used": 2147483648,
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T10:30:00+08:00"
}
```

## 3. `platform_runtime_configs`

**用途：** 全平台共享的运行配置。当前设计为单例表，只允许 `id = 1`，适合保存所有用户共用的检索、解析和存储策略。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | smallint, PK, 固定为 1 | 单例主键。 |
| `retriever_engines` | jsonb | 可用检索引擎及默认引擎配置。 |
| `context_config` | jsonb | RAG 上下文拼接、窗口等全局配置。 |
| `web_search_config` | jsonb | Web 搜索全局开关和默认提供商。 |
| `parser_engine_config` | jsonb | DocReader、MinerU 等解析引擎配置。 |
| `storage_engine_config` | jsonb | 本地、S3/MinIO 等存储引擎配置。 |
| `retrieval_config` | jsonb | Top-K、阈值、混合检索等默认参数。 |
| `created_at` | timestamptz | 首次初始化时间。 |
| `updated_at` | timestamptz | 管理员最后修改时间。 |

**Mock 数据：**

```json
{
  "id": 1,
  "retriever_engines": {"default": "milvus"},
  "context_config": {"max_chunks": 12},
  "web_search_config": {"enabled": false},
  "parser_engine_config": {"default": "docreader"},
  "storage_engine_config": {"default": "minio"},
  "retrieval_config": {"embedding_top_k": 20, "rerank_top_k": 8},
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:30:00+08:00"
}
```

## 4. `models`

**用途：** 注册平台可使用的 Embedding、Rerank、对话大模型、VLM 和 ASR 模型。模型密钥位于 `parameters` 中时由应用加密后落库。

常见 `type`：`Embedding`、`Rerank`、`KnowledgeQA`、`VLLM`、`ASR`。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 模型配置 UUID。 |
| `name` | varchar | 应用内部模型名称。 |
| `display_name` | varchar | 前端显示名称。 |
| `type` | varchar | 模型能力类型。 |
| `source` | varchar | 模型供应商或兼容协议，如 `openai`、`aliyun`、`deepseek`。 |
| `description` | text | 模型说明。 |
| `parameters` | jsonb | Base URL、模型名、加密 API Key、超时和维度等参数。 |
| `is_default` | boolean | 是否为该类型默认模型。 |
| `status` | varchar | 配置可用状态。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `is_builtin` | boolean | 是否为系统内置记录。 |
| `managed_by` | varchar | 配置管理范围，例如平台统一管理。 |

**Mock 数据：**

```json
{
  "id": "model-embed-001",
  "name": "text-embedding-v3",
  "display_name": "Enterprise Embedding",
  "type": "Embedding",
  "source": "openai",
  "description": "经 Taichi 网关调用的统一向量模型",
  "parameters": {
    "base_url": "https://taichi.example.com/v1",
    "model": "text-embedding-v3",
    "dimension": 1024,
    "api_key": "<encrypted>"
  },
  "is_default": true,
  "status": "active",
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null,
  "is_builtin": false,
  "managed_by": "platform"
}
```

## 5. `vector_stores`

**用途：** 保存向量数据库连接与索引配置。使用 Milvus 时，向量本体位于 Milvus Collection，本表只保存“如何连接”。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 向量存储配置 UUID。 |
| `name` | varchar | 配置显示名称。 |
| `engine_type` | varchar | 引擎类型；本项目当前主要使用 `milvus`。 |
| `connection_config` | jsonb | 地址、认证、数据库等连接参数，敏感值由应用保护。 |
| `index_config` | jsonb | Collection、距离度量、索引类型及参数。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |

**Mock 数据：**

```json
{
  "id": "vs-milvus-001",
  "name": "Primary Milvus",
  "engine_type": "milvus",
  "connection_config": {
    "address": "milvus:19530",
    "database": "default"
  },
  "index_config": {
    "collection": "roche_kap_embeddings",
    "metric_type": "IP",
    "index_type": "HNSW"
  },
  "knowledge_domain_id": 1,
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null
}
```

## 6. `web_search_providers`

**用途：** 注册可供智能体调用的外部 Web 搜索提供商。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 搜索提供商配置 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `name` | varchar | 前端显示名称。 |
| `provider` | varchar | 实现类型，如 `bing`、`google`、`tavily`、`searxng`。 |
| `description` | text | 配置说明。 |
| `parameters` | jsonb | API 地址、密钥、结果数等参数。 |
| `is_default` | boolean | 是否为默认 Web 搜索提供商。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |

**Mock 数据：**

```json
{
  "id": "web-search-001",
  "knowledge_domain_id": 1,
  "name": "Corporate SearXNG",
  "provider": "searxng",
  "description": "企业内网搜索代理",
  "parameters": {
    "base_url": "https://search.example.com",
    "result_count": 5
  },
  "is_default": true,
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null
}
```

## 7. `system_settings`

**用途：** 保存不适合放入固定表结构的系统级键值设置，并支持标记敏感配置和是否需要重启。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK | 设置记录 ID。 |
| `key` | varchar, UNIQUE | 稳定配置键。 |
| `value` | jsonb | 配置值。 |
| `value_type` | varchar | 值类型，用于校验和前端编辑。 |
| `category` | varchar | 设置分类。 |
| `description` | text | 设置说明。 |
| `is_secret` | boolean | 是否为敏感值；前端读取时应脱敏。 |
| `requires_restart` | boolean | 修改后是否需要重启服务。 |
| `last_modified_by` | varchar(36) | 最后修改用户 ID。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |

**Mock 数据：**

```json
{
  "id": 12,
  "key": "security.max_login_attempts",
  "value": 5,
  "value_type": "integer",
  "category": "security",
  "description": "连续登录失败上限",
  "is_secret": false,
  "requires_restart": false,
  "last_modified_by": "usr-admin-001",
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T11:00:00+08:00"
}
```

## 本组关系摘要

```text
knowledge_domains.id
  ├─ knowledge_domain_storage.knowledge_domain_id
  ├─ vector_stores.knowledge_domain_id
  └─ web_search_providers.knowledge_domain_id

platform_runtime_configs(id=1) -> 全平台统一运行配置
system_settings                -> 通用系统键值设置
```
