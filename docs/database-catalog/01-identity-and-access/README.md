# 01 身份认证与知识访问授权

本目录覆盖 5 张表：

```text
users
├─ auth_tokens
├─ sso_identities
├─ knowledge_domain_admins
└─ knowledge_resource_grants
```

## 1. `users`

**用途：** 系统用户主档。无论用户通过开发密码、Dex OIDC 还是 PingIdentity SAML 登录，最终都需要映射到一条本地 `users` 记录。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 用户 UUID，由后端创建。 |
| `username` | varchar(100), UNIQUE, NOT NULL | 显示名；本地注册输入，SSO 用户由映射后的断言/claim 提供。 |
| `email` | varchar(255), UNIQUE, NOT NULL | 企业邮箱或注册邮箱；SAML/OIDC 场景来自配置映射。 |
| `password_hash` | varchar(255), NOT NULL | 密码哈希，不保存明文；纯 SSO 用户保存不可用于生产密码登录的随机占位哈希。 |
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

**用途：** 把外部 SAML/OIDC 身份稳定地映射到本地 `users`。识别键是 `provider + issuer + subject`；邮箱只用于首次辅助匹配或展示，不能替代稳定 subject。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 映射记录 ID。 |
| `user_id` | varchar(36), FK, NOT NULL | 对应本地用户；用户删除时级联删除。 |
| `provider` | varchar(64), default `oidc` | 协议/提供方逻辑名；生产 PingIdentity SAML 写 `saml`，开发 Dex 写 `oidc`。 |
| `issuer` | varchar(255), NOT NULL | SAML IdP EntityID 或 OIDC `iss`。 |
| `subject` | varchar(255), NOT NULL | SAML NameID/映射后的稳定 subject，或 OIDC `sub`。 |
| `created_at` | timestamptz | 首次绑定时间。 |
| `updated_at` | timestamptz | 映射更新时间。 |
| `last_login_at` | timestamptz, nullable | 最近一次使用该身份登录的时间。 |

**Mock 数据：**

```json
{
  "id": 101,
  "user_id": "usr-00000000-0000-0000-0000-000000000001",
  "provider": "saml",
  "issuer": "https://ping.example.com/idp/entity-id",
  "subject": "alice-stable-name-id",
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

## 5. `knowledge_resource_grants`

**用途：** 用一张表表达知识库、逻辑目录和文档的白名单、黑名单、读取、管理和继承
规则。迁移 2 已删除旧的整库授权表和文档授权表。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK, auto increment | 授权记录 ID。 |
| `knowledge_domain_id` | bigint, FK, NOT NULL | 资源所属知识域；知识域删除时级联删除。 |
| `knowledge_base_id` | varchar(36), FK, NOT NULL | 资源所属物理知识库；知识库删除时级联删除。 |
| `resource_type` | varchar(24), CHECK | `knowledge_base`、`folder` 或 `knowledge`。 |
| `resource_id` | varchar(36), NOT NULL | 对应知识库、目录或文档 ID；由 Service 校验。 |
| `subject_type` | varchar(16), CHECK | `user` 或 `org_unit`。 |
| `subject_id` | varchar(36), NOT NULL | `users.id` 或 `org_units.id`；由 Service 校验。 |
| `permission` | varchar(16), CHECK | `read` 或 `manage`；管理隐含读取。 |
| `effect` | varchar(8), CHECK | `allow` 或 `deny`。 |
| `inherit_to_children` | boolean | 知识库/目录规则是否应用到后代；文档规则固定 false。 |
| `granted_by` | varchar(36), FK, nullable | 操作人；删除该用户时置空。 |
| `created_at` | timestamptz | 首次创建规则的时间。 |
| `updated_at` | timestamptz | effect、继承或授权人最后更新时间。 |

唯一键：

```text
knowledge_base_id + resource_type + resource_id
+ subject_type + subject_id + permission
```

同一主体可以同时有 `read` 和 `manage` 两条记录，也可以通过再次 PUT 把同一权限规则
从 allow 更新为 deny。

**Mock 数据：**

```json
{
  "id": 9001,
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-finance-policy",
  "resource_type": "folder",
  "resource_id": "folder-confidential",
  "subject_type": "org_unit",
  "subject_id": "org-interns-cn",
  "permission": "read",
  "effect": "deny",
  "inherit_to_children": true,
  "granted_by": "usr-system-admin",
  "created_at": "2026-07-30T11:00:00+08:00",
  "updated_at": "2026-07-30T11:00:00+08:00"
}
```

## 6. 权限计算示例

用户 Alice：

```text
users.id = usr-alice
user_org_memberships = [org-finance-cn]
```

授权：

```text
knowledge_resource_grants:
  KB-A / knowledge_base / org-finance-cn / read / allow / inherit
  KB-A / Restricted folder / org-interns-cn / read / deny / inherit
  KB-B / Doc-B3 / usr-alice / read / allow / exact
```

最终结果：

```text
KB-A：整库可读，但若 Alice 同时属于实习生组，则 Restricted 子树不可读
KB-B：只能读取 Doc-B3
其他知识库：不可见、不可检索
```

完整继承和 deny 规则见
[资源层级、授权与删除](../../knowledge/resource-access-and-deletion.md)。
