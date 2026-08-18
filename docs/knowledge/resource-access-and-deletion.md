# 资源层级、授权与删除

本文描述当前版本知识库、目录和文档的统一访问控制模型。实现基线：

- 数据表：`knowledge_resource_grants`
- 类型：`internal/types/enterprise_access.go`
- 权限计算：`internal/application/service/enterprise_access.go`
- 数据访问：`internal/application/repository/enterprise_access.go`
- HTTP 接口：`internal/handler/enterprise_access.go`
- 路由：`internal/router/router.go`

## 1. 物理知识库与逻辑资源

一个物理知识库保存一套解析、切片、模型、向量库和图谱配置。其内部资源形成目录树：

```text
knowledge_base
├─ folder
│  ├─ child folder
│  │  └─ knowledge
│  └─ knowledge
└─ knowledge
```

对应 PostgreSQL：

```text
knowledge_bases.id
  ├─ knowledge_folders.knowledge_base_id
  │    └─ knowledge_folders.parent_id
  └─ knowledges.knowledge_base_id
       └─ knowledges.folder_id
```

目录和文档可以像逻辑知识库一样单独授权，但不会复制为新的物理知识库。它们继续
共享所属知识库的模型、切片、Milvus Collection、对象存储和图谱配置。

## 2. 统一授权表

旧的 `knowledge_base_grants` 和 `knowledge_grants` 已由迁移
`000002_knowledge_resource_grants` 删除。当前只使用：

```text
knowledge_resource_grants
```

一个规则由四个维度确定：

| 维度 | 可选值 | 含义 |
|---|---|---|
| `resource_type` | `knowledge_base`、`folder`、`knowledge` | 被保护的资源类型 |
| `subject_type` | `user`、`org_unit` | 被授权的单个用户或企业组织 |
| `permission` | `read`、`manage` | 读取或管理；`manage` 隐含 `read` |
| `effect` | `allow`、`deny` | 白名单或黑名单 |

`resource_id` 按 `resource_type` 分别保存知识库 ID、目录 ID 或文档 ID。
`subject_id` 按 `subject_type` 分别保存 `users.id` 或 `org_units.id`。这两个字段都是
多态引用，因此数据库不能为它们建立一条固定外键；Service 会在写入前验证目标。

## 3. 继承规则

| 资源规则 | 自身 | 子目录 | 后代文档 |
|---|---:|---:|---:|
| 知识库规则 | 是 | 是 | 是 |
| 目录规则，`inherit_to_children=true` | 是 | 是 | 是 |
| 目录规则，`inherit_to_children=false` | 是 | 否 | 否 |
| 文档规则 | 是 | 不适用 | 不适用 |

知识库规则始终继承，后端会强制 `inherit_to_children=true`。文档规则始终为精确规则，
后端会强制 `false`。

企业组织树采用向上闭包：用户属于一个组织时，同时被视为其所有有效上级组织的成员。
因此给“中国区”授权，可覆盖属于“中国区 > 财务部”的员工。组织归属本身不产生
知识权限，只有存在显式授权规则时才生效。

## 4. Allow、Deny 与优先级

系统收集对当前用户生效的所有直接用户规则和组织规则，然后计算目标资源上所有适用
的知识库、祖先目录和精确文档规则。

规则如下：

1. `manage allow` 同时产生读取和管理能力。
2. `read deny` 同时阻断读取和管理。
3. `manage deny` 只阻断管理，不自动阻断已经允许的读取。
4. 任意适用的 deny 都覆盖同权限的 allow，不按“距离资源更近”反向覆盖 deny。
5. 系统管理员和所属知识域管理员走平台管理旁路，不受普通资源 ACL 的 deny 限制。

示例：

```text
财务部：KB-A / read / allow / inherit
实习生组：KB-A/Restricted / read / deny / inherit
Alice：KB-A/Restricted/Guide.pdf / read / allow
```

如果 Alice 同时属于实习生组，她仍不能读取 `Restricted/Guide.pdf`，因为适用的目录
deny 覆盖精确文档 allow。要恢复访问，应撤销或调整 deny，而不是再叠加 allow。

## 5. 动态访问范围

`my_permission`、`full_access` 和允许的文档列表都不是静态列。每次请求根据以下数据
重新计算：

```text
当前 users.id
  + users.is_system_admin
  + knowledge_domain_admins
  + user_org_memberships 的有效组织及祖先
  + knowledge_resource_grants
  + knowledge_folders / knowledges 当前层级
```

普通用户最终得到：

- `FullAccess=true`：知识库级读取成立，且没有后代文档被 deny。
- `KnowledgeIDs=[...]`：只能读取列出的文档。
- `FolderIDs=[...]`：只返回可见目录以及到这些目录的祖先路径。
- `CanManage=true`：具有知识库级 `manage`，可以维护知识库内容和 ACL。

JWT 不携带知识库 ID 列表，所以撤销授权、组织调岗或新增黑名单会在下一次请求立即
生效，不必等待 Token 过期。

## 6. Agent 与检索

智能体没有独立授权表，也没有独立知识范围。普通 RAG 和 Agent 工具都只能在当前
用户动态授权范围内工作，但入口行为略有差异：

```text
普通 RAG = 当前用户有效资源范围 ∩ 本次请求的知识库/文档/标签过滤
Agent 初始范围 = 当前用户全部有效资源范围
Agent 单次工具调用 = Agent 初始范围 ∩ 工具参数中的合法过滤
```

智能体配置中的知识库选择和本轮 `@知识库` 不参与 Agent 初始授权计算；它们不能
扩大或替代用户权限。

Milvus 仍按 `knowledge_base_id` 和 `knowledge_id` 过滤：

- 整库可读时只需知识库过滤。
- 部分文档可读时同时传入允许的 `knowledge_ids`。
- 文件夹授权会先在 PostgreSQL 展开为后代文档 ID，再用于 Milvus 过滤。
- deny 先在 PostgreSQL 权限计算中排除文档，不能依赖前端隐藏结果。

## 7. 管理界面

资源 ACL 统一从知识库列表卡片的“访问权限”打开。弹窗包含：

1. 整个知识库。
2. 目录。
3. 指定文档。

目录和文档浏览页不再提供独立权限按钮或删除按钮，以免出现多个管理入口和行为不一致。
删除目录或文档也在该外层弹窗中完成。

只有知识库级管理者可以查看授权对象、创建规则、撤销规则和执行弹窗中的删除操作。
目录或文档上的 `manage` 只控制资源使用，不允许接收者反过来修改 ACL 策略。

## 8. 授权 API

所有路径都位于 `/api/v1`：

```text
GET /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grants
GET /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grant-subjects
PUT /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grants
DELETE /knowledge-bases/{kb_id}/resource-grants/{grant_id}
DELETE /knowledge-bases/{kb_id}/folders/{folder_id}
```

新增或更新规则：

```json
{
  "subject_type": "org_unit",
  "subject_id": "org-finance-id",
  "permission": "read",
  "effect": "allow",
  "inherit_to_children": true
}
```

唯一键包含 `permission`。对同一资源和主体再次 PUT 相同权限时，会更新
`effect`、继承标记、授权人和更新时间。

## 9. 删除语义

### 删除文档

必须通过业务 API 或 Service 删除，不能只执行 SQL。完整流程会处理：

- 停止或隔离仍在运行的异步任务。
- 删除原文件和解析图片等对象。
- 删除 PostgreSQL Chunk、标签关系和文档记录。
- 删除 Milvus 向量与关键词索引。
- 删除 Neo4j 中该文档来源的图谱事实。
- 删除该文档的 `knowledge_resource_grants`。

### 删除目录

系统管理员、知识域管理员或知识库级管理者可以删除任意目录，不要求目录为空。
后端会：

1. 读取目标目录及所有后代目录。
2. 找出子树中的所有有效文档。
3. 对这些文档执行完整文档删除流程。
4. 在事务内确认没有并发上传留下的有效文档。
5. 删除目录和后代文档的直接 ACL。
6. 删除整棵目录子树。

如果步骤 4 发现并发写入，返回 `409`，避免数据库外键把新文档静默移动到根目录。

### 删除知识库

知识库删除会清理其所有文档和外部派生数据。`knowledge_resource_grants` 对
`knowledge_bases` 使用 `ON DELETE CASCADE`，知识库行物理删除时 ACL 自动删除。
对象存储、Milvus 和 Neo4j 不参与 PostgreSQL 事务，仍必须由业务服务先清理。

## 10. 404 排查

授权弹窗出现纯文本 `404 page not found` 时，优先检查后端是否仍运行旧代码。
Vite 会热更新前端，但当前 Windows 开发脚本不会自动重启 Go：

```bash
bash ./scripts/dev-all.sh stop
bash ./scripts/dev-all.sh
```

或只重启后端：

```bash
# 停止旧 make dev-app / scripts/dev.sh app 进程后
make dev-app
```

判断方法：

1. `GET /health` 只证明进程存活，不证明版本正确。
2. 检查后端启动日志中的数据库迁移版本，应至少为 `2`。
3. 已登录调用 `.../resources/.../grants`：
   - `200`：路由和权限都通过。
   - `401`：未携带有效平台 JWT。
   - `403`：已登录但不是知识库级管理者。
   - `404 page not found`：通常是旧进程没有新路由。
   - JSON 404：资源 ID 不存在或不属于指定知识库。
