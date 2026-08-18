# 02 企业组织与外部人事集成

这里的组织树与知识域完全独立：

```text
knowledge_domains：知识库管理分组
org_units：企业真实组织架构
```

属于某企业部门不会天然产生知识权限；只有管理员为该组织节点创建有效的
`knowledge_resource_grants` 后，组织成员才获得对应知识库、目录或文档权限。

## 1. `org_units`

**用途：** 系统内部规范化的企业组织树，可由管理员手工建立，也可以由 Workday 同步生成。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 组织节点 UUID。 |
| `parent_id` | varchar(36), self FK, nullable | 上级组织节点；根节点为空。父节点被引用时禁止直接删除。 |
| `code` | varchar(128), NOT NULL | 稳定业务编码；有效记录中唯一。 |
| `name` | varchar(255), NOT NULL | 部门显示名称。 |
| `status` | varchar(20), CHECK | `active` 或 `inactive`。 |
| `source` | varchar(32), CHECK | `manual`、`workday`、`bootstrap`。 |
| `external_id` | varchar(255), nullable | 外部系统组织 ID，通常来自 Workday。 |
| `sort_order` | integer, default 0 | 同级节点展示顺序。 |
| `attributes` | jsonb, default `{}` | 成本中心、地点、负责人等可扩展属性。 |
| `created_by` | varchar(36), FK, nullable | 手工创建人；用户删除时置空。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 名称、状态、层级等更新时间。 |
| `deleted_at` | timestamptz, nullable | 软删除时间。 |

**Mock 数据：**

```json
{
  "id": "org-finance-cn",
  "parent_id": "org-china",
  "code": "CN-FIN",
  "name": "China Finance",
  "status": "active",
  "source": "workday",
  "external_id": "WD-ORG-100028",
  "sort_order": 20,
  "attributes": {
    "cost_center": "CC-8801",
    "location": "Shanghai"
  },
  "created_by": null,
  "created_at": "2026-07-28T01:00:00+08:00",
  "updated_at": "2026-07-29T01:00:00+08:00",
  "deleted_at": null
}
```

## 2. `user_org_memberships`

**用途：** 表示一个本地用户属于哪些企业组织节点。该关系可用于批量授权，但关系本身不赋予知识访问权。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 成员关系 ID。 |
| `user_id` | varchar(36), FK, NOT NULL | 本地用户。 |
| `org_unit_id` | varchar(36), FK, NOT NULL | 企业组织节点。 |
| `is_primary` | boolean, default false | 是否为该用户主部门；每个用户最多一个有效主部门。 |
| `status` | varchar(20), CHECK | `active` 或 `inactive`。 |
| `source` | varchar(32), CHECK | `manual`、`workday`、`bootstrap`。 |
| `created_at` | timestamptz | 关系建立时间。 |
| `updated_at` | timestamptz | 调岗或状态更新时间。 |

**Mock 数据：**

```json
{
  "id": 20001,
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "org_unit_id": "org-finance-cn",
  "is_primary": true,
  "status": "active",
  "source": "workday",
  "created_at": "2026-07-28T01:10:00+08:00",
  "updated_at": "2026-07-29T01:10:00+08:00"
}
```

## 3. `external_org_units`

**用途：** 保存外部系统组织记录的投影，并映射到系统规范组织 `org_units`。它保留外部 ID 和同步状态，避免把 Workday 字段直接污染核心组织表。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 投影记录 UUID。 |
| `provider` | varchar(32), NOT NULL | 来源系统，如 `workday`。 |
| `external_org_id` | varchar(255), NOT NULL | 来源系统组织稳定 ID。 |
| `parent_external_org_id` | varchar(255), nullable | 来源系统中的上级组织 ID。 |
| `org_unit_id` | varchar(36), FK, nullable | 映射后的 `org_units.id`；组织删除时置空。 |
| `name` | varchar(255), NOT NULL | 外部系统组织名称。 |
| `org_type` | varchar(64), nullable | 如 `Company`、`CostCenter`、`Department`。 |
| `status` | varchar(20), CHECK | `active` 或 `inactive`。 |
| `attributes` | jsonb, default `{}` | 外部附加字段的受控投影。 |
| `checksum` | varchar(64), NOT NULL | 规范化记录哈希，用于判断本轮是否变化。 |
| `effective_from` | timestamptz, nullable | 生效时间。 |
| `effective_to` | timestamptz, nullable | 失效时间。 |
| `last_seen_at` | timestamptz, NOT NULL | 最近一次同步仍观察到该对象的时间。 |
| `created_at` | timestamptz | 首次同步时间。 |
| `updated_at` | timestamptz | 最近一次内容变化时间。 |

唯一性：`provider + external_org_id`。

**Mock 数据：**

```json
{
  "id": "ext-org-0001",
  "provider": "workday",
  "external_org_id": "WD-ORG-100028",
  "parent_external_org_id": "WD-ORG-100000",
  "org_unit_id": "org-finance-cn",
  "name": "China Finance",
  "org_type": "Department",
  "status": "active",
  "attributes": {
    "cost_center": "CC-8801"
  },
  "checksum": "sha256:mock-org-checksum",
  "effective_from": "2025-01-01T00:00:00Z",
  "effective_to": null,
  "last_seen_at": "2026-07-29T01:00:00Z",
  "created_at": "2026-07-01T01:00:00Z",
  "updated_at": "2026-07-29T01:00:00Z"
}
```

## 4. `external_workers`

**用途：** 保存 Workday 等系统的员工投影，并可映射到本地 `users`。`external_worker_id` 是就业身份稳定键，邮箱只作为辅助匹配。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 投影记录 UUID。 |
| `provider` | varchar(32), NOT NULL | 来源系统。 |
| `external_worker_id` | varchar(255), NOT NULL | 外部员工稳定 ID。 |
| `user_id` | varchar(36), FK, nullable | 匹配到的本地用户；用户删除时置空。 |
| `primary_org_external_id` | varchar(255), nullable | 外部主部门 ID。 |
| `manager_external_worker_id` | varchar(255), nullable | 外部直属经理员工 ID。 |
| `corporate_email` | varchar(255), nullable | 企业邮箱，只用于辅助匹配和展示。 |
| `worker_status` | varchar(20), CHECK | `active`、`inactive`、`leave`。 |
| `attributes` | jsonb, default `{}` | 职位、地点、员工类型等受控扩展字段。 |
| `checksum` | varchar(64), NOT NULL | 规范化员工记录哈希。 |
| `effective_from` | timestamptz, nullable | 就业记录生效时间。 |
| `effective_to` | timestamptz, nullable | 就业记录失效时间。 |
| `last_seen_at` | timestamptz, NOT NULL | 最近同步发现时间。 |
| `created_at` | timestamptz | 首次同步时间。 |
| `updated_at` | timestamptz | 最近内容变化时间。 |

唯一性：`provider + external_worker_id`。

**Mock 数据：**

```json
{
  "id": "ext-worker-0001",
  "provider": "workday",
  "external_worker_id": "WD-WORKER-900188",
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "primary_org_external_id": "WD-ORG-100028",
  "manager_external_worker_id": "WD-WORKER-900001",
  "corporate_email": "alice.zhang@example.com",
  "worker_status": "active",
  "attributes": {
    "job_title": "Finance Analyst",
    "location": "Shanghai"
  },
  "checksum": "sha256:mock-worker-checksum",
  "effective_from": "2024-03-01T00:00:00Z",
  "effective_to": null,
  "last_seen_at": "2026-07-29T01:00:00Z",
  "created_at": "2026-07-01T01:00:00Z",
  "updated_at": "2026-07-29T01:00:00Z"
}
```

## 5. `integration_sync_runs`

**用途：** 记录一次企业集成全量或增量同步的游标、计数和最终状态。每一批完成后推进游标，便于断点续传和审计。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 同步运行 UUID。 |
| `provider` | varchar(32), NOT NULL | 如 `workday`。 |
| `connection_key` | varchar(128), NOT NULL | 配置中的连接逻辑名称，不保存凭据。 |
| `mode` | varchar(20), CHECK | `full` 或 `incremental`。 |
| `cursor_before` | jsonb, default `{}` | 本轮开始前的游标。 |
| `cursor_after` | jsonb, default `{}` | 成功处理后的新游标。 |
| `status` | varchar(20), CHECK | `pending`、`running`、`succeeded`、`failed`。 |
| `counters` | jsonb, default `{}` | 组织、员工、匹配、更新等数量。 |
| `trace_id` | varchar(128), nullable | 跨日志、Langfuse 或集成平台追踪 ID。 |
| `error_code` | varchar(64), nullable | 稳定错误码。 |
| `error_summary` | text, nullable | 脱敏后的失败摘要。 |
| `started_at` | timestamptz, nullable | 真正开始执行时间。 |
| `finished_at` | timestamptz, nullable | 终态时间。 |
| `created_at` | timestamptz | 任务记录创建时间。 |

**Mock 数据：**

```json
{
  "id": "sync-run-workday-20260729-01",
  "provider": "workday",
  "connection_key": "workday-prod",
  "mode": "incremental",
  "cursor_before": {
    "updated_after": "2026-07-28T00:00:00Z"
  },
  "cursor_after": {
    "updated_after": "2026-07-29T00:00:00Z"
  },
  "status": "succeeded",
  "counters": {
    "org_units_seen": 50,
    "org_units_changed": 2,
    "workers_seen": 1200,
    "workers_changed": 18,
    "workers_linked": 18,
    "memberships_changed": 4,
    "unmatched_workers": 0
  },
  "trace_id": "trace-workday-20260729-01",
  "error_code": null,
  "error_summary": null,
  "started_at": "2026-07-29T01:00:00Z",
  "finished_at": "2026-07-29T01:03:12Z",
  "created_at": "2026-07-29T00:59:58Z"
}
```

## 6. `integration_events`

**用途：** 保存外部 Webhook 或批次事件的幂等信封。不会保存完整敏感 payload，只保存事件标识和 payload 哈希。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 事件记录 ID。 |
| `provider` | varchar(32), NOT NULL | 事件来源。 |
| `external_event_id` | varchar(255), NOT NULL | 来源事件 ID。 |
| `event_type` | varchar(128), NOT NULL | 如 `worker.updated`。 |
| `payload_hash` | varchar(64), NOT NULL | 原始 payload 的哈希，用于校验和审计。 |
| `status` | varchar(20), CHECK | `received`、`processing`、`processed`、`failed`。 |
| `attempt_count` | integer, default 0 | 已尝试处理次数。 |
| `trace_id` | varchar(128), nullable | 跨系统追踪 ID。 |
| `received_at` | timestamptz | 接收时间。 |
| `processed_at` | timestamptz, nullable | 成功或最终失败时间。 |
| `error_summary` | text, nullable | 脱敏失败摘要。 |

唯一性：`provider + external_event_id`，用于防止同一事件重复执行。

**Mock 数据：**

```json
{
  "id": 30001,
  "provider": "workday",
  "external_event_id": "evt-WD-778899",
  "event_type": "worker.updated",
  "payload_hash": "sha256:mock-event-payload",
  "status": "processed",
  "attempt_count": 1,
  "trace_id": "trace-workday-event-778899",
  "received_at": "2026-07-29T02:00:00Z",
  "processed_at": "2026-07-29T02:00:02Z",
  "error_summary": null
}
```
