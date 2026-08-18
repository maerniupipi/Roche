# 数据与运行时架构

## 1. 数据主从关系

| 数据 | 主数据位置 | 派生或缓存位置 |
|---|---|---|
| 用户、组织、资源 ACL | PostgreSQL | 每个请求计算出的 `KnowledgeBaseAccessScope` / `SearchTargets` |
| 知识库、文档、Chunk | PostgreSQL | Milvus 检索副本 |
| 原文件和解析图片 | 对象存储 | 临时本地文件 |
| 向量 | Milvus | 无 PostgreSQL 向量表 |
| 图实体和关系 | Neo4j | PostgreSQL Chunk 保存来源 ID |
| 会话和消息 | PostgreSQL | Redis 流式状态 |
| 异步任务 | Redis/Asynq | PostgreSQL dead-letter 和 processing spans |
| LLM 追踪 | Langfuse | 应用日志 |

## 2. PostgreSQL

PostgreSQL 保存所有需要事务一致性、审计和权限判断的数据。新鲜部署先执行
`000000_init.up.sql`，再按序执行后续迁移；当前最终版本包含
`000002_knowledge_resource_grants`。

重要事实：

- 不存在 `tenants` 和 `tenant_members`。
- 不存在 `agent_permissions`。
- 不存在 PostgreSQL `embeddings` 业务表。
- `knowledge_domain_id` 是知识管理边界，不是企业部门 ID。
- 企业组织使用 `org_units`。
- 知识库、目录、文档授权统一保存在 `knowledge_resource_grants`。

完整表目录见 [数据库总览](../database/README.md)。

## 3. Milvus

Milvus Collection 保存用于检索的向量和过滤字段。核心字段来自 `IndexInfo`：

- `id`
- `source_id`
- `source_type`
- `chunk_id`
- `knowledge_id`
- `knowledge_base_id`
- `tag_id`
- `content`
- `dimension`
- `embedding`
- `is_enabled`

PostgreSQL `chunks.id` 与 Milvus `chunk_id` 建立逻辑关联。删除或重建文档时，
业务服务必须同时更新 PostgreSQL Chunk 和 Milvus 索引。

Milvus 不存目录 ID，也不执行 ACL 规则。权限服务先从 PostgreSQL 计算可读文档，
再生成以下两类过滤：

```text
整库可读：knowledge_base_id == "KB-A"
部分可读：knowledge_base_id == "KB-A" AND knowledge_id IN ["D1", "D7"]
```

黑名单目录会从允许的文档集合中扣除其后代文档；用户提交的筛选条件只能继续缩小
这个集合。

## 4. Redis

Redis 的主要用途：

1. Asynq 文档解析、索引、摘要、问题生成、图谱等任务队列。
2. 流式问答期间的停止信号和续传状态。
3. 有限生命周期的任务进度和协调数据。

Redis 不应成为用户、授权、知识元数据或会话历史的唯一存储。

## 5. 对象存储

根据 `STORAGE_TYPE` 和平台存储配置，可使用本地目录、MinIO、S3、COS、OSS 等。
对象内容包括：

- 用户上传的原文件。
- DocReader/MinerU 产生并持久化的图片。
- 会话临时附件和图片。
- 需要下载或预览的文件对象。

PostgreSQL 的 `knowledges.file_path` 保存对象路径或 provider URI，不保存文件二进制。
删除知识或知识库时由业务删除流程清理对象；必须通过任务幂等和补偿机制处理部分失败。

## 6. Neo4j

启用图谱索引后，实体与关系写入 Neo4j。图谱以 `knowledge_base_id` 和来源
`knowledge_id/chunk_id` 作为隔离、过滤和引用依据。一个知识库内多个文档可共同形成
一张可查询图，但每条事实仍需要保留来源文档和 Chunk。

## 7. DuckDB

DuckDB 是按请求创建的分析引擎：

- `data_schema` 读取 Excel/CSV 元信息。
- `data_analysis` 将文件物化为临时表并执行受限只读 SQL。
- 数据表摘要任务读取完整表、样本行和列信息。

DuckDB 文件和临时表不是业务主数据，生命周期结束后可删除。

## 8. 一致性原则

知识删除涉及多种存储，无法使用一个跨库事务。文档删除和目录递归删除复用完整
知识删除服务，按资源逐项清理：

1. 标记/校验文档状态并阻止与新上传冲突。
2. 清理原文件、解析图片等对象。
3. 删除 Chunk 和 Milvus 索引。
4. 删除该文档来源的 Neo4j 事实。
5. 清理标签关系和文档 ACL。
6. 删除文档记录。
7. 目录场景在所有后代文档清理完成后，删除目录子树及目录 ACL。

所有跨系统操作都应携带 `request_id` 或 `trace_id`，便于日志和 Langfuse 关联。
如果目录删除期间发现新的活动文档，服务返回冲突而不是留下悬挂数据。
