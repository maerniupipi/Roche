# 09 运行维护与数据库迁移

本组共 2 张表：

```text
task_dead_letters
schema_migrations
```

## 1. `task_dead_letters`

**用途：** 保存多次执行仍失败的异步任务。它让任务不再无限重试，同时保留后续人工排查和重新投递所需的信息。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK | 死信记录 ID。 |
| `knowledge_domain_id` | integer | 任务所属知识域。 |
| `task_type` | varchar | 任务类型，如文档解析、Embedding、数据源同步。 |
| `scope` | varchar | 业务作用域类型，如 `knowledge`、`data_source`。 |
| `scope_id` | varchar | 作用域对象 ID。 |
| `related_id` | varchar, nullable | 关联任务、文档或批次 ID。 |
| `payload` | jsonb | 重新执行所需的脱敏任务参数。 |
| `last_error` | text | 最后一次失败原因。 |
| `fail_count` | integer | 累计失败次数。 |
| `failed_at` | timestamptz | 最后失败并进入死信的时间。 |

**Mock 数据：**

```json
{
  "id": 3107,
  "knowledge_domain_id": 1,
  "task_type": "knowledge_embedding",
  "scope": "knowledge",
  "scope_id": "knowledge-doa-001",
  "related_id": "span-embedding-001",
  "payload": {
    "knowledge_id": "knowledge-doa-001",
    "attempt": 4
  },
  "last_error": "embedding gateway timeout after 30s",
  "fail_count": 4,
  "failed_at": "2026-07-29T09:40:00+08:00"
}
```

## 2. `schema_migrations`

**用途：** 数据库迁移工具维护的版本状态。应用启动时通过它判断当前数据库是否需要升级，以及上次迁移是否中途失败。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `version` | bigint, PK | 已应用的最高迁移版本，对应 `migrations/versioned` 文件号。 |
| `dirty` | boolean | `true` 表示迁移执行到一半失败，必须修复后才能安全继续启动。 |

**Mock 数据：**

```json
{
  "version": 79,
  "dirty": false
}
```

## 运维判断示例

```text
schema_migrations = {version: 79, dirty: false}
```

表示数据库已经执行到 `000079_*.up.sql`，且没有未完成迁移。

```text
schema_migrations = {version: 79, dirty: true}
```

表示第 79 版迁移过程中出现异常。此时不能把它理解为“已经完成 79”；应检查迁移日志、数据库实际结构和对应 down/up 脚本，修复后再继续。

`task_dead_letters` 与 `schema_migrations` 的区别：

- 前者记录应用业务异步任务失败。
- 后者记录数据库结构迁移状态。
- 两者都不存放知识正文或向量。
