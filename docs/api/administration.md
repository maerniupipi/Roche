# Administration API

## 1. 平台配置

模型、解析、检索和存储配置是平台级共享配置，只允许系统管理员修改。

```text
GET /system/runtime-config/{key}
PUT /system/runtime-config/{key}
```

常见 `key` 对应检索、上下文、Web Search、解析和存储运行配置。客户端应先读取配置，
保留未知字段后再提交，避免覆盖后端新增配置。

## 2. 模型

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| `GET` | `/models/providers` | 已登录 | 模型提供商能力目录 |
| `GET` | `/models` | 已登录 | 模型列表，敏感凭据被脱敏 |
| `GET` | `/models/{id}` | 已登录 | 模型详情 |
| `POST` | `/models` | 系统管理员 | 创建模型 |
| `PUT` | `/models/{id}` | 系统管理员 | 更新非敏感配置 |
| `DELETE` | `/models/{id}` | 系统管理员 | 删除模型 |
| `POST` | `/models/{id}/debug` | 系统管理员 | 连接诊断 |
| `PUT` | `/models/{id}/credentials` | 系统管理员 | 写入敏感凭据 |
| `DELETE` | `/models/{id}/credentials/{field}` | 系统管理员 | 删除单个凭据 |

凭据接口是 write-only 语义；读取模型配置不会返回 API Key 明文。

## 3. 企业组织

企业组织与知识域完全独立。

### 组织节点

```text
GET    /enterprise/org-units
POST   /enterprise/org-units
PUT    /enterprise/org-units/{org_unit_id}
DELETE /enterprise/org-units/{org_unit_id}
GET    /enterprise/org-units/{org_unit_id}/members
```

创建或更新请求：

```json
{
  "parent_id": null,
  "code": "FIN-CN",
  "name": "China Finance",
  "status": "active",
  "sort_order": 10,
  "attributes": {
    "cost_center": "CN100"
  }
}
```

`status` 允许 `active`、`inactive`。`source=workday` 的组织由同步过程管理，不应在
人工页面覆盖关键映射字段。

### 用户成员关系

```text
GET /enterprise/users?search=alice&limit=50
GET /enterprise/users/{user_id}/org-memberships
PUT /enterprise/users/{user_id}/org-memberships
```

替换请求：

```json
{
  "memberships": [
    {
      "org_unit_id": "org-uuid",
      "is_primary": true
    }
  ]
}
```

PUT 是全量替换人工成员关系，不是追加。Workday 管理的成员关系受到服务端保护。

## 4. 知识授权

知识库、目录和文档统一使用资源 ACL：

```text
GET /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grants
GET /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grant-subjects
PUT /knowledge-bases/{kb_id}/resources/{resource_type}/{resource_id}/grants
DELETE /knowledge-bases/{kb_id}/resource-grants/{grant_id}
```

`resource_type` 为 `knowledge_base`、`folder` 或 `knowledge`。知识库资源的
`resource_id` 必须等于 `{kb_id}`。

PUT 请求：

```json
{
  "subject_type": "org_unit",
  "subject_id": "org-finance-id",
  "permission": "read",
  "effect": "allow",
  "inherit_to_children": true
}
```

| 字段 | 可选值 | 说明 |
|---|---|---|
| `subject_type` | `user`、`org_unit` | 单个用户或企业组织 |
| `permission` | `read`、`manage` | 管理隐含读取 |
| `effect` | `allow`、`deny` | 白名单或黑名单 |
| `inherit_to_children` | boolean | 知识库强制 true，文档强制 false |

只有知识库级管理者可以读取或修改 ACL。目录/文档 `manage` 不授予 ACL 管理权。
前端只在知识库列表外层的“访问权限”弹窗提供管理入口。

删除目录：

```text
DELETE /knowledge-bases/{kb_id}/folders/{folder_id}
```

该操作允许删除非空目录，并递归清理后代文档、对象、Chunk、Milvus、Neo4j、标签
关系和 ACL。完整语义见
[资源层级、授权与删除](../knowledge/resource-access-and-deletion.md)。

## 5. 知识域管理员

```text
GET    /knowledge-domains/{id}/administrators
POST   /knowledge-domains/{id}/administrators
DELETE /knowledge-domains/{id}/administrators/{user_id}
```

只有系统管理员可以任命或撤销知识域管理员。知识域管理员可管理该域下全部知识库，
但不能修改平台模型和 Workday 配置。

## 6. 系统状态与诊断

| 路径 | 作用 |
|---|---|
| `GET /system/info` | 版本与运行信息 |
| `GET /system/parser-engines` | 解析引擎列表 |
| `POST /system/parser-engines/check` | 检查解析器连接 |
| `POST /system/docreader/reconnect` | 重连 DocReader |
| `GET /system/storage-engine-status` | 对象存储状态 |
| `POST /system/storage-engine-check` | 存储连通性测试 |
| `/initialization/*` | 模型、Embedding、Rerank、ASR、VLM 和抽取诊断 |

诊断接口可能调用外部服务并产生费用，除只读状态接口外均限制系统管理员。

## 7. 审计

```text
GET /system/admin/audit-log
GET /knowledge-domains/{id}/audit-log
```

审计记录应使用请求 ID、操作人、动作、目标资源和结果检索。客户端不应将审计日志
作为权限判断依据，权限仍由业务中间件实时计算。
