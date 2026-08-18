# 06 外部数据源与同步

本组共 2 张表：

```text
data_sources
└─ sync_logs

data_sources -> knowledge_bases -> knowledges -> chunks -> Milvus
```

## 1. `data_sources`

**用途：** 保存 Google Drive 等外部内容源的连接、选择范围和同步策略。一条数据源绑定一个知识库，同步得到的每个文件仍会建立独立的 `knowledges` 记录。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 数据源 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `knowledge_base_id` | varchar(36) | 同步目标知识库 ID。 |
| `name` | varchar | 用户输入的数据源显示名称。 |
| `type` | varchar | 连接器类型，如 `google_drive`、`notion`、`rss`。 |
| `config` | jsonb | 加密凭据引用、远端目录 ID、选择范围等连接器配置。 |
| `sync_schedule` | varchar/json 兼容字段 | 同步频率或计划表达式。 |
| `sync_mode` | varchar | `incremental` 或 `full`。 |
| `status` | varchar | 常见为 `active`、`paused`、`error`、`deleted`。 |
| `conflict_strategy` | varchar | 远端同一文件发生变化时的处理策略，如 `overwrite`、`skip`。 |
| `sync_deletions` | boolean | 是否要求将远端删除同步到本地；最终行为仍取决于连接器实现。 |
| `last_sync_at` | timestamptz | 最近一次同步结束时间。 |
| `last_sync_cursor` | jsonb/text 兼容字段 | 增量同步游标或 change token。 |
| `last_sync_result` | jsonb | 最近一次同步的计数和结果摘要。 |
| `error_message` | text | 数据源当前错误摘要。 |
| `sync_log_retention_days` | integer | `sync_logs` 保留天数。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 配置或同步状态最后更新时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |

**Mock 数据：**

```json
{
  "id": "ds-gdrive-finance-001",
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-fin-policy-001",
  "name": "Finance Shared Drive",
  "type": "google_drive",
  "config": {
    "credential_ref": "secret://gdrive/finance-reader",
    "drive_id": "0AExampleSharedDrive",
    "folder_ids": ["1FinancePolicies"]
  },
  "sync_schedule": "0 */6 * * *",
  "sync_mode": "incremental",
  "status": "active",
  "conflict_strategy": "overwrite",
  "sync_deletions": true,
  "last_sync_at": "2026-07-29T06:00:42+08:00",
  "last_sync_cursor": {"page_token": "chg-00001872"},
  "last_sync_result": {
    "created": 2,
    "updated": 1,
    "deleted": 0,
    "failed": 0
  },
  "error_message": null,
  "sync_log_retention_days": 30,
  "created_at": "2026-07-28T12:00:00+08:00",
  "updated_at": "2026-07-29T06:00:42+08:00",
  "deleted_at": null
}
```

## 2. `sync_logs`

**用途：** 保存每次数据源同步运行的终态和统计。它用于审计、前端展示和失败排查，不替代 `knowledge_processing_spans` 的文档内部解析轨迹。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 同步运行 UUID。 |
| `data_source_id` | varchar(36), FK | 对应 `data_sources.id`，数据源删除时级联删除。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `status` | varchar | 本次同步状态，如 `running`、`completed`、`failed`。 |
| `started_at` | timestamptz | 同步开始时间。 |
| `finished_at` | timestamptz | 同步结束时间。 |
| `items_total` | integer | 扫描到的远端对象总数。 |
| `items_created` | integer | 新建本地知识文档数量。 |
| `items_updated` | integer | 检测变化并重建的文档数量。 |
| `items_deleted` | integer | 识别或实际处理的删除数量；需结合连接器实现判断。 |
| `items_skipped` | integer | 未变化、冲突策略跳过或不支持文件数量。 |
| `items_failed` | integer | 同步或知识构建失败数量。 |
| `error_message` | text | 本次同步总体错误。 |
| `result` | jsonb | 文件级结果、游标、耗时等扩展摘要。 |
| `created_at` | timestamptz | 日志创建时间。 |
| `updated_at` | timestamptz | 日志最后更新时间。 |

**Mock 数据：**

```json
{
  "id": "sync-run-gdrive-20260729-0600",
  "data_source_id": "ds-gdrive-finance-001",
  "knowledge_domain_id": 1,
  "status": "completed",
  "started_at": "2026-07-29T06:00:00+08:00",
  "finished_at": "2026-07-29T06:00:42+08:00",
  "items_total": 128,
  "items_created": 2,
  "items_updated": 1,
  "items_deleted": 0,
  "items_skipped": 125,
  "items_failed": 0,
  "error_message": null,
  "result": {
    "cursor_after": "chg-00001872",
    "changed_file_ids": ["gdoc-101", "gdoc-108", "gdoc-125"]
  },
  "created_at": "2026-07-29T06:00:00+08:00",
  "updated_at": "2026-07-29T06:00:42+08:00"
}
```

## 同步与知识构建的关系

```text
1. data_sources 决定去哪里读取、同步什么范围、多久同步一次
2. sync_logs 记录一次扫描与同步的总体结果
3. 新增或变化文件写入/重建 knowledges
4. 原文件进入 S3/MinIO 或本地存储
5. 文档进入解析、切片、VLM、问题生成等流程
6. chunks 写 PostgreSQL
7. Embedding 写 Milvus
8. 每份文档的阶段详情写 knowledge_processing_spans
```

因此，“Google Drive 同步成功”只表示远端文件已进入知识构建任务；应继续查看 `knowledges.parse_status` 和 `knowledge_processing_spans` 才能判断是否完成向量化。
