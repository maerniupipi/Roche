# 01 身份认证与知识访问授权

本目录覆盖 6 张表：

```text
users
├─ auth_tokens
├─ sso_identities
├─ knowledge_domain_admins
├─ knowledge_base_grants
└─ knowledge_grants
```

## 1. `users`

**用途：** 系统用户主档。无论用户通过邮箱密码还是 PingIdentity/OIDC 登录，最终都需要映射到一条本地 `users` 记录。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 用户 UUID，由后端创建。 |
| `username` | varchar(100), UNIQUE, NOT NULL | 显示名；本地注册输入，OIDC 用户可由声明映射。 |
| `email` | varchar(255), UNIQUE, NOT NULL | 企业邮箱或注册邮箱；OIDC 场景通常来自 `email` claim。 |
| `password_hash` | varchar(255), NOT NULL | 密码哈希，不保存明文；纯 OIDC 用户也可能保存不可登录的占位哈希。 |
| `avatar` | varchar(500), nullable | 头像 URL。 |
| `is_active` | boolean, default true | 是否允许登录和使用系统。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 用户资料或状态最后更新时间。 |
| `deleted_at` | timestamptz, nullable | 软删除时间；为空表示有效。 |
| `preferences` | jsonb, default `{}` | 跨浏览器保存的个人偏好，当前支持 `enable_memory`。 |
| `is_system_admin` | boolean, default false | 是否为系统管理员；该权限独立于知识域角色。 |

**主要关联：**

- `auth_tokens.user_id -> users.id`
- `sso_identities.user_id -> users.id`
- `knowledge_domain_admins.user_id -> users.id`
- `user_org_memberships.user_id -> users.id`

**Mock 数据：**

```json
{
  "id": "usr-00000000-0000-0000-0000-000000000001",
  "username": "Alice Zhang",
  "email": "alice.zhang@example.com",
  "password_hash": "$2a$10$mock-bcrypt-hash",
  "avatar": "https://assets.example.com/avatar/alice.png",
  "is_active": true,
  "preferences": {
    "enable_memory": false
  },
  "is_system_admin": false,
  "created_at": "2026-07-29T10:00:00+08:00",
  "updated_at": "2026-07-29T10:00:00+08:00",
  "deleted_at": null
}
```

## 2. `auth_tokens`

**用途：** 保存本平台发出的认证 Token 状态，用于过期、撤销和刷新控制。它不是 PingIdentity 自己的 Token 表。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | Token 记录 UUID。 |
| `user_id` | varchar(36), FK, NOT NULL | Token 所属本地用户；用户删除时级联删除。 |
| `token` | text, NOT NULL | Token 值或服务使用的 Token 表示。生产文档和日志不应打印。 |
| `token_type` | varchar(50), NOT NULL | 常见为 `access_token`、`refresh_token`。 |
| `expires_at` | timestamptz, NOT NULL | 失效时间。 |
| `is_revoked` | boolean, default false | 是否已被注销、轮换或管理员撤销。 |
| `created_at` | timestamptz | 签发记录创建时间。 |
| `updated_at` | timestamptz | 撤销等状态最后更新时间。 |

**Mock 数据：**

```json
{
  "id": "tok-00000000-0000-0000-0000-000000000001",
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "token": "<redacted-refresh-token>",
  "token_type": "refresh_token",
  "expires_at": "2026-08-28T10:00:00+08:00",
  "is_revoked": false,
  "created_at": "2026-07-29T10:00:00+08:00",
  "updated_at": "2026-07-29T10:00:00+08:00"
}
```

## 3. `sso_identities`

**用途：** 把外部 OIDC/PingIdentity 身份稳定地映射到本地 `users`。识别键是 `issuer + subject`，邮箱只用于首次匹配或展示，不能代替稳定 subject。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 映射记录 ID。 |
| `user_id` | varchar(36), FK, NOT NULL | 对应本地用户；用户删除时级联删除。 |
| `provider` | varchar(64), default `oidc` | 身份提供方逻辑名称，如 `pingidentity`。 |
| `issuer` | varchar(255), NOT NULL | OIDC `iss`，例如 PingIdentity Issuer URL。 |
| `subject` | varchar(255), NOT NULL | OIDC `sub`，外部用户稳定标识。 |
| `created_at` | timestamptz | 首次绑定时间。 |
| `updated_at` | timestamptz | 映射更新时间。 |
| `last_login_at` | timestamptz, nullable | 最近一次使用该身份登录的时间。 |

**Mock 数据：**

```json
{
  "id": 101,
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "provider": "pingidentity",
  "issuer": "https://sso.example.com/as/authorization.oauth2",
  "subject": "00u1-alice-stable-subject",
  "created_at": "2026-07-20T09:00:00+08:00",
  "updated_at": "2026-07-29T10:00:00+08:00",
  "last_login_at": "2026-07-29T10:00:00+08:00"
}
```

## 4. `knowledge_domain_admins`

**用途：** 将用户任命为指定知识域的管理员。该表只表达管理职责，不表达普通用户的知识读取权限。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 管理员分配记录 ID。 |
| `knowledge_domain_id` | integer, FK, NOT NULL | 被管理的 `knowledge_domains.id`。 |
| `user_id` | varchar(36), FK, NOT NULL | 被任命的 `users.id`。 |
| `granted_by` | varchar(36), FK, nullable | 执行任命的系统管理员；删除该用户时置空。 |
| `status` | varchar(16), default `active` | 当前基线只允许 `active`。 |
| `created_at` | timestamptz | 任命时间。 |
| `updated_at` | timestamptz | 分配记录最后更新时间。 |

`(knowledge_domain_id, user_id)` 唯一。系统管理员由 `users.is_system_admin` 表示，不需要为每个知识域写入本表。

**Mock 数据：**

```json
{
  "id": 501,
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "knowledge_domain_id": 10000,
  "status": "active",
  "granted_by": "usr-system-admin-001",
  "created_at": "2026-07-20T09:00:00+08:00",
  "updated_at": "2026-07-20T09:00:00+08:00"
}
```

## 5. `knowledge_base_grants`

**用途：** 给单个用户或整个企业组织节点授予“整库读取”权限。授权组织节点后，当前属于该节点的有效用户可获得整库访问。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 授权记录 ID。 |
| `knowledge_domain_id` | integer, FK, NOT NULL | 知识库所属知识域。 |
| `knowledge_base_id` | varchar(36), FK, NOT NULL | 被授权的知识库。 |
| `subject_type` | varchar(16), CHECK | `user` 或 `org_unit`。 |
| `subject_id` | varchar(36), NOT NULL | `user` 时为 `users.id`；`org_unit` 时为 `org_units.id`。这是多态引用，数据库没有单一 FK。 |
| `permission` | varchar(16), CHECK | 当前只能是 `read`。 |
| `granted_by` | varchar(36), FK, nullable | 执行授权的管理员；管理员被删时置空，授权记录保留。 |
| `created_at` | timestamptz | 首次授权时间。 |
| `updated_at` | timestamptz | 授权更新时间。 |

唯一性：同一知识库、同一主体类型、同一主体只能有一行。

**Mock 数据：**

```json
{
  "id": 9001,
  "knowledge_domain_id": 10000,
  "knowledge_base_id": "kb-finance-policy",
  "subject_type": "org_unit",
  "subject_id": "org-finance-cn",
  "permission": "read",
  "granted_by": "usr-system-admin",
  "created_at": "2026-07-29T11:00:00+08:00",
  "updated_at": "2026-07-29T11:00:00+08:00"
}
```

## 6. `knowledge_grants`

**用途：** 给用户或组织节点授予知识库中“单个文档”的读取权限。当用户没有整库授权，但有这里的记录时，只能检索列出的文档。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 授权记录 ID。 |
| `knowledge_domain_id` | integer, FK, NOT NULL | 文档所属知识域。 |
| `knowledge_base_id` | varchar(36), FK, NOT NULL | 文档所属知识库。 |
| `knowledge_id` | varchar(36), FK, NOT NULL | 被授权的 `knowledges.id`。 |
| `subject_type` | varchar(16), CHECK | `user` 或 `org_unit`。 |
| `subject_id` | varchar(36), NOT NULL | 用户 ID 或组织节点 ID。 |
| `permission` | varchar(16), CHECK | 当前只能是 `read`。 |
| `granted_by` | varchar(36), FK, nullable | 授权人。 |
| `created_at` | timestamptz | 首次授权时间。 |
| `updated_at` | timestamptz | 授权更新时间。 |

唯一性：同一文档、同一主体类型、同一主体只能有一行。

**Mock 数据：**

```json
{
  "id": 9101,
  "knowledge_domain_id": 10000,
  "knowledge_base_id": "kb-legal",
  "knowledge_id": "doc-contract-template-v3",
  "subject_type": "user",
  "subject_id": "usr-00000000-0000-0000-0000-000000000001",
  "permission": "read",
  "granted_by": "usr-system-admin",
  "created_at": "2026-07-29T11:10:00+08:00",
  "updated_at": "2026-07-29T11:10:00+08:00"
}
```

## 7. 权限计算示例

用户 Alice：

```text
users.id = usr-alice
user_org_memberships = [org-finance-cn]
```

授权：

```text
knowledge_base_grants:
  KB-A -> org-finance-cn

knowledge_grants:
  KB-B / Doc-B3 -> usr-alice
```

最终结果：

```text
KB-A：整库可读
KB-B：只能读取 Doc-B3
其他知识库：不可见、不可检索
```
