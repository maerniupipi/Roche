# 08 用户体验状态与审计

本组共 3 张表：

```text
user_resource_favorites
user_kb_pins
audit_logs
```

## 1. `user_resource_favorites`

**用途：** 保存用户收藏的知识库或智能体。它只影响前端列表和快捷入口，不产生知识访问权限。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `user_id` | varchar(36), 联合 PK | 收藏操作用户 ID。 |
| `knowledge_domain_id` | integer, 联合 PK | 当前知识域。 |
| `resource_type` | varchar, 联合 PK | 资源类型，当前主要为 `kb` 或 `agent`。 |
| `resource_id` | varchar(36), 联合 PK | 被收藏的知识库或智能体 ID。 |
| `created_at` | timestamptz | 收藏时间。 |

**Mock 数据：**

```json
{
  "user_id": "usr-viewer-001",
  "knowledge_domain_id": 1,
  "resource_type": "kb",
  "resource_id": "kb-fin-policy-001",
  "created_at": "2026-07-29T10:20:00+08:00"
}
```

## 2. `user_kb_pins`

**用途：** 保存用户在知识库列表中置顶的知识库。与收藏相同，它是个人界面状态，不等于授权。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `knowledge_domain_id` | integer, 联合 PK | 当前知识域。 |
| `user_id` | varchar(36), 联合 PK | 执行置顶的用户 ID。 |
| `kb_id` | varchar(36), 联合 PK | 被置顶的知识库 ID。 |
| `pinned_at` | timestamptz | 置顶时间，用于排序。 |

**Mock 数据：**

```json
{
  "knowledge_domain_id": 1,
  "user_id": "usr-viewer-001",
  "kb_id": "kb-fin-policy-001",
  "pinned_at": "2026-07-29T10:21:00+08:00"
}
```

## 3. `audit_logs`

**用途：** 追加式记录关键管理操作、授权结果和拒绝事件，回答“谁在什么时候对什么对象做了什么”。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK | 审计记录 ID。 |
| `knowledge_domain_id` | integer | 操作发生的知识域。 |
| `actor_user_id` | varchar(36) | 操作者用户 ID。 |
| `actor_role` | varchar | 操作当时的角色快照。 |
| `action` | varchar | 稳定操作名，如 `knowledge_base.grant.create`。 |
| `target_type` | varchar | 被操作对象类型。 |
| `target_id` | varchar | 被操作对象 ID。 |
| `target_user_id` | varchar(36), nullable | 若操作针对某用户，记录目标用户。 |
| `request_path` | text | 触发操作的 API 路径。 |
| `request_method` | varchar | HTTP 方法。 |
| `outcome` | varchar | 常见为 `success` 或 `denied`。 |
| `details` | jsonb | 授权主体、变更前后值、原因等脱敏详情。 |
| `created_at` | timestamptz | 事件发生时间。 |

**Mock 数据：**

```json
{
  "id": 90031,
  "knowledge_domain_id": 1,
  "actor_user_id": "usr-admin-001",
  "actor_role": "system_admin",
  "action": "knowledge_base.grant.create",
  "target_type": "knowledge_base",
  "target_id": "kb-fin-policy-001",
  "target_user_id": "usr-viewer-001",
  "request_path": "/api/v1/knowledge-bases/kb-fin-policy-001/grants",
  "request_method": "POST",
  "outcome": "success",
  "details": {
    "subject_type": "user",
    "subject_id": "usr-viewer-001",
    "permission": "read"
  },
  "created_at": "2026-07-29T10:25:00+08:00"
}
```

## 三者的区别

| 表 | 会影响读取权限吗 | 主要消费者 |
|---|---|---|
| `user_resource_favorites` | 否 | 前端收藏列表 |
| `user_kb_pins` | 否 | 前端知识库排序 |
| `audit_logs` | 否，但会记录授权与拒绝结果 | 审计、合规、排障 |
