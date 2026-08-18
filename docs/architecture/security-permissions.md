# 认证与权限模型

## 1. 身份认证

身份认证由独立 Auth Service 负责，不再由管理/RAG 后端提供登录路由。开发环境支持
邮箱密码和 Dex OIDC Authorization Code Flow；生产环境强制使用 PingIdentity
SAML 2.0。所有入口最终映射到 `users` 和 `sso_identities`。企业 IdP 的断言或 Token
不直接作为业务 API 凭据；Auth Service 完成身份映射后签发平台 Access JWT 和可轮换
Refresh Token。API Gateway 在转发业务请求前调用 Auth Service 预验证，业务后端再做
一次 JWT 与资源权限校验。

## 2. 平台身份与资源权限

平台身份：

| 身份 | 来源 | 能力 |
|---|---|---|
| 系统管理员 | `users.is_system_admin` | 平台配置、组织、所有知识域和资源 |
| 知识域管理员 | `knowledge_domain_admins` | 管理指定知识域及其知识库 |
| 普通用户 | 有效 `users` 记录 | 读取或管理被显式授权的资源 |

`read/manage` 不是新的用户角色，而是知识资源 ACL 权限。普通用户也可被授予某个
知识库的 `manage`，但不会因此获得模型、Workday 或其他知识域的管理能力。

## 3. 三套独立结构

```text
知识管理：knowledge_domains -> knowledge_bases -> folders / knowledges
企业组织：org_units -> user_org_memberships -> users
资源授权：knowledge_resource_grants
```

知识域和企业组织可以同名，但没有自动绑定。员工属于“财务部”本身不产生知识权限；
管理员还必须向该组织节点创建资源 ACL。

## 4. 统一资源 ACL

迁移 `000002_knowledge_resource_grants` 删除旧的 `knowledge_base_grants` 和
`knowledge_grants`，统一使用：

```text
knowledge_resource_grants
  resource_type = knowledge_base | folder | knowledge
  subject_type  = user | org_unit
  permission    = read | manage
  effect        = allow | deny
```

知识库规则始终继承，目录规则可配置是否继承，文档规则只作用于自身。`manage`
隐含 `read`；`read deny` 阻断读取和管理；`manage deny` 只阻断管理。任意适用的
deny 覆盖同权限 allow。

完整算法、示例和删除规则见
[资源层级、授权与删除](../knowledge/resource-access-and-deletion.md)。

## 5. 有效访问范围

普通用户的有效范围由直接用户规则、有效组织及其祖先规则、目录继承和 deny 共同
计算。结果可能是整库访问，也可能只包含若干目录或文档。

系统管理员和所属知识域管理员拥有管理旁路，不要求为每个资源创建 ACL。资源级
`manage` 接收者可以管理内容，但只有知识库级管理者可以修改 ACL 或在统一弹窗执行
资源删除。

## 6. 智能体权限

智能体没有授权表。普通 RAG 和 Agent 工具都使用当前请求用户的动态授权范围，
但范围入口不同：

```text
普通 RAG = 用户有效资源范围 ∩ 本轮知识库/文档/标签过滤
Agent 初始范围 = 用户全部有效资源范围
Agent 工具调用 = Agent 初始范围 ∩ 工具参数过滤
```

智能体配置中的知识库绑定和本轮 `@知识库` 当前不参与 Agent 初始范围。工具参数
只能缩小范围。以下能力必须复用同一权限计算：

- 知识库、目录和文档列表。
- `knowledge_search`、`grep_chunks`。
- `list_knowledge_chunks`、`get_document_info`。
- `query_knowledge_graph`。
- `data_schema`、`data_analysis`、`database_query`。
- Chunk、下载、预览和引用详情。

## 7. 路由与 Service 守卫

`internal/router/rbac.go` 负责路由级守卫装配，`internal/middleware/rbac.go` 和
`internal/middleware/kb_access.go` 提供登录、系统管理员、知识库读写以及按
文档/Chunk 反查父知识库的守卫。资源 ACL 的最终计算位于
`internal/application/service/enterprise_access.go`。

前端隐藏按钮只是体验控制，不能作为安全边界。Handler、Service 和 Repository
必须始终使用服务端身份与知识库归属校验。

## 8. Token 与权限变更

JWT 只携带稳定身份和平台级声明，不携带知识库或文档 ID 列表。授权、黑名单和组织
成员关系每次请求从 PostgreSQL 重新计算，所以撤销或调岗在下一次请求立即生效。

## 9. 安全要求

- Refresh Token 支持撤销和轮换，开发密码只保存强哈希；生产关闭密码登录和注册。
- SAML 校验签名、证书、有效期、audience、recipient、RelayState 和 `InResponseTo`。
- OIDC Mock 校验 issuer、audience、state、nonce 和 redirect URI。
- 登录后 redirect URI 使用精确白名单，拒绝开放重定向。
- 外部 URL 和代理配置经过 SSRF 校验。
- 模型、数据源、MCP 和对象存储凭据加密且不回显。
- 文件下载使用鉴权路由或短时签名 URL。
- 系统管理、授权和删除操作写入审计日志。
