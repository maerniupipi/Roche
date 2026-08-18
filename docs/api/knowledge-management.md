# Knowledge Management API

## 1. 资源层级

```text
knowledge_domain
  -> knowledge_base
      -> knowledge_folder
      -> knowledge (document/file)
          -> chunk
```

`knowledge_domain` 是知识管理域，不是 Workday 企业部门。企业部门位于独立的
`org_units` 树中，只作为批量授权主体。

## 2. 知识域

| 方法 | 路径 | 权限 | 作用 |
|---|---|---|---|
| `POST` | `/knowledge-domains` | 系统管理员 | 创建知识域 |
| `GET` | `/knowledge-domains` | 已登录 | 列出当前可见知识域 |
| `GET` | `/knowledge-domains/all` | 系统管理员 | 列出全部知识域 |
| `GET` | `/knowledge-domains/search` | 系统管理员 | 搜索知识域 |
| `GET` | `/knowledge-domains/{id}` | 域访问校验 | 读取详情 |
| `PUT` | `/knowledge-domains/{id}` | 域管理员或系统管理员 | 更新名称、说明和状态 |
| `DELETE` | `/knowledge-domains/{id}` | 系统管理员 | 删除知识域 |
| `GET/POST` | `/knowledge-domains/{id}/administrators` | 管理员 | 查询或授予域管理员 |
| `DELETE` | `/knowledge-domains/{id}/administrators/{user_id}` | 系统管理员 | 撤销域管理员 |
| `GET` | `/knowledge-domains/{id}/audit-log` | 域管理员或系统管理员 | 查询审计日志 |

## 3. 知识库

### 创建

```http
POST /api/v1/knowledge-bases
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Finance Policies",
  "description": "Finance policy documents",
  "type": "document",
  "knowledge_domain_id": 1,
  "embedding_model_id": "model-uuid",
  "summary_model_id": "model-uuid",
  "chunking_config": {
    "chunk_size": 512,
    "chunk_overlap": 80,
    "strategy": "auto",
    "enable_parent_child": true,
    "parent_chunk_size": 4096,
    "child_chunk_size": 384
  },
  "indexing_strategy": {
    "vector_enabled": true,
    "keyword_enabled": true,
    "graph_enabled": false
  }
}
```

实际请求结构以 Swagger 的 `types.KnowledgeBase` 为准。创建人必须是系统管理员或目标
知识域管理员。`vector_store_id` 一旦创建后不可通过普通更新接口修改。

### 查询与管理

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/knowledge-bases` | 返回当前用户可读或可管理的知识库 |
| `GET` | `/knowledge-bases/{id}` | 返回详情和动态计算的 `my_permission` |
| `PUT` | `/knowledge-bases/{id}` | 更新名称、描述和可变配置 |
| `DELETE` | `/knowledge-bases/{id}` | 删除知识库及关联业务数据 |
| `PUT` | `/knowledge-bases/{id}/pin` | 设置当前用户的置顶状态 |
| `POST/GET` | `/knowledge-bases/{id}/hybrid-search` | 在单知识库中执行混合检索 |
| `POST` | `/knowledge-bases/copy` | 异步复制知识库 |
| `GET` | `/knowledge-bases/copy/progress/{task_id}` | 查询复制进度 |
| `GET` | `/knowledge-bases/{id}/move-targets` | 查询允许移动文档的目标知识库 |

`my_permission` 不落库。它由当前用户的系统管理员身份、知识域管理员身份以及
`knowledge_resource_grants` 实时计算。普通用户可能获得整库访问，也可能只看到
被授权目录、被授权文档以及用于导航到这些资源的祖先目录。

## 4. 上传文件

```http
POST /api/v1/knowledge-bases/{kb_id}/knowledge/file
Authorization: Bearer <token>
Content-Type: multipart/form-data
```

表单字段：

| 字段 | 必填 | 含义 |
|---|---|---|
| `file` | 是 | 原始文件 |
| `fileName` | 否 | 自定义显示文件名 |
| `folder_path` | 否 | 知识库内部相对目录，如 `Policies/Finance` |
| `metadata` | 否 | JSON 字符串，值为字符串的自定义元数据 |
| `enable_multimodel` | 否 | 是否覆盖知识库的多模态处理开关 |
| `tag_ids` | 否 | 逗号分隔标签 ID |
| `process_config` | 否 | `KnowledgeProcessOverrides` JSON |
| `channel` | 否 | 来源通道标记 |

示例：

```bash
curl -X POST "http://localhost:8080/api/v1/knowledge-bases/$KB_ID/knowledge/file" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@policy.pdf" \
  -F "folder_path=Compliance/China" \
  -F 'metadata={"classification":"internal"}' \
  -F "enable_multimodel=true"
```

接口成功表示原始文件和知识记录已经创建、异步任务已经进入队列。文档变为可检索状态
还需要完成解析、切片和索引。

## 5. URL 与手工内容

### URL

```http
POST /api/v1/knowledge-bases/{kb_id}/knowledge/url
Content-Type: application/json

{
  "url": "https://example.com/policy.pdf",
  "file_name": "policy.pdf",
  "file_type": "pdf",
  "title": "Policy",
  "enable_multimodel": false,
  "tag_ids": [],
  "process_config": {}
}
```

URL 会经过 SSRF 校验。内网地址必须通过部署侧白名单明确允许。

### 手工 Markdown

```http
POST /api/v1/knowledge-bases/{kb_id}/knowledge/manual
Content-Type: application/json
```

请求体是 `ManualKnowledgePayload`，用于录入标题、Markdown 内容、元数据和标签。

## 6. 文档查询与生命周期

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/knowledge-bases/{id}/knowledge` | 分页列出有权读取的文档 |
| `GET` | `/knowledge-bases/{id}/knowledge/folders` | 列出知识库内部目录 |
| `DELETE` | `/knowledge-bases/{id}/folders/{folder_id}` | 递归删除目录、后代文档及派生数据 |
| `GET` | `/knowledge/{id}` | 文档详情和处理状态 |
| `PUT` | `/knowledge/{id}` | 更新文档元信息 |
| `DELETE` | `/knowledge/{id}` | 删除文档及 Chunk、索引和关联资源 |
| `GET` | `/knowledge/{id}/download` | 下载原文件 |
| `GET` | `/knowledge/{id}/preview` | 预览原文件 |
| `POST` | `/knowledge/{id}/reparse` | 重新解析并重建索引 |
| `POST` | `/knowledge/{id}/cancel-parse` | 取消仍在执行的解析 |
| `GET` | `/knowledge/{id}/stages` | 查询阶段状态 |
| `GET` | `/knowledge/{id}/spans` | 查询处理链路 spans |
| `POST` | `/knowledge/move` | 异步移动文档 |
| `POST` | `/knowledge/batch-reparse` | 批量重解析 |
| `POST` | `/knowledge/batch-delete` | 批量删除 |

状态接口中的完成状态表示数据库 Chunk 和目标检索索引已经写入，不只是解析器返回成功。

资源权限和删除操作统一位于知识库列表外层的“访问权限”弹窗。内部目录和文档浏览页
不再提供权限或删除按钮。目录删除不要求为空；服务端会先执行完整文档删除流程，再
删除目录树和直接 ACL。详见
[资源层级、授权与删除](../knowledge/resource-access-and-deletion.md)。

## 7. Chunk

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/chunks/{knowledge_id}` | 分页读取指定文档 Chunk |
| `GET` | `/chunks/by-id/{id}` | 按 Chunk ID 读取 |
| `PUT` | `/chunks/{knowledge_id}/{id}` | 人工修改 Chunk |
| `DELETE` | `/chunks/{knowledge_id}/{id}` | 删除单个 Chunk |
| `DELETE` | `/chunks/{knowledge_id}` | 删除文档全部 Chunk |
| `DELETE` | `/chunks/by-id/{id}/questions` | 删除生成问题 |
| `POST` | `/chunker/preview` | 不落库预览切片结果 |

Chunk 修改后必须同步更新向量索引；该一致性由服务层维护，客户端不应直接写 PostgreSQL。

## 8. 标签与 FAQ

标签端点：

```text
GET    /knowledge-bases/{id}/tags
POST   /knowledge-bases/{id}/tags
PUT    /knowledge-bases/{id}/tags/{tag_id}
DELETE /knowledge-bases/{id}/tags/{tag_id}
PUT    /knowledge/tags
```

标签是人工配置的过滤维度，不会在每次用户提问时自动从问题中生成。

FAQ 端点集中在：

```text
/knowledge-bases/{id}/faq/*
```

支持 FAQ 条目增删改查、相似问、批量导入导出和 FAQ 检索。FAQ 与文档知识库使用不同
的数据结构和索引内容，调用方应先检查知识库 `type`。
