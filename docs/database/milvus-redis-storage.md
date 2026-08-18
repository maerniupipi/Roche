# Milvus、Redis、对象存储与 Neo4j

## 1. Milvus

标准部署使用 Milvus 保存向量，PostgreSQL 不保存 embedding。Collection 名来自
`MILVUS_COLLECTION`，默认部署使用统一 Collection，并用标量字段隔离知识范围。

核心字段：

| 字段 | 类型 | 用途 |
|---|---|---|
| `id` | VarChar | 索引记录主键 |
| `content` | VarChar | 实际参与索引/匹配并随结果返回的文本 |
| `vector` | FloatVector | Embedding 模型输出 |
| `source_id` | VarChar | 批量索引源 ID |
| `source_type` | Int | Chunk、Passage、Summary |
| `chunk_id` | VarChar | 回查 PostgreSQL Chunk |
| `knowledge_id` | VarChar | 单文档过滤 |
| `knowledge_base_id` | VarChar | 知识库过滤 |
| `knowledge_type` | VarChar | document/faq/manual 等 |
| `tag_id` | VarChar | 标签/FAQ 过滤 |
| `is_enabled` | Bool | 是否召回 |
| `is_recommended` | Bool | 推荐过滤 |

`content` 启用 Analyzer/Match 时，Milvus 可以对这个字段建立文本分析和关键词匹配
能力。它不是到 S3 中搜索原文件，而是在已经写入 Collection 的文本中匹配。

普通正文向量的 `content` 可能是“标题面包屑 + `chunks.content`”；问题生成任务
的索引文本可能是生成问题。因此引用展示必须根据 `chunk_id` 回 PostgreSQL，
不能把 Milvus content 当唯一权威正文。

Milvus Collection 不保存 `folder_id` 和授权规则。一次查询的过滤条件由后端先从
PostgreSQL 计算：

```text
整库授权：
  knowledge_base_id IN ["KB-A"]

目录/文档级授权：
  knowledge_base_id == "KB-B"
  AND knowledge_id IN ["DOC-B1", "DOC-B4"]
```

文件夹规则先通过 `knowledge_folders` 和 `knowledges.folder_id` 展开，deny 子树
从文档 ID 集合中排除。Milvus 只执行结果过滤，不判断用户身份和继承优先级。

## 2. Redis

Redis 用途：

- Asynq 队列、重试和调度。
- 文档 VLM 子任务剩余计数。
- SSE 流恢复和停止信号。
- 分布式锁、短期缓存和幂等状态。
- 数据源同步的运行协调。

Redis 不是用户、权限、知识或会话的长期存储。清空 Redis 会丢失正在排队/运行的
任务和临时流状态，但不应删除 PostgreSQL 已完成知识和对象存储文件。

## 3. 对象存储

支持 `local`、`minio`、`s3`、`cos`、`tos`、`oss`、`ks3`、`obs`。统一由
`interfaces.FileService` 操作。

保存内容：

- 用户上传的原文件。
- URL 导入后下载的文件。
- DocReader/MinerU 解析出的图片。
- 聊天附件和用户图片。

PostgreSQL 只保存 provider URL，例如：

```text
minio://rochekap/123/document.pdf
s3://enterprise-bucket/knowledge/123/figure-1.png
local://42/uuid/document.docx
```

服务文件接口在鉴权后将 provider URL 转换成下载或预签名响应。不要把 Bucket
永久公开，也不要把对象存储凭据返回前端。

解析后的完整 Markdown 当前不单独保存为对象；它在 Worker 内存中完成图片 URL
替换后切成 `chunks`。若未来需要可审计的解析中间产物，可新增受控对象
`parsed/{knowledge_id}/{attempt}/document.md`，但这不是当前实现。

## 4. Neo4j

Neo4j 保存：

- 实体名称和属性。
- 实体间关系。
- 实体/关系的来源 Chunk ID。
- 知识库和知识文档 namespace。

图谱查询可按整库 namespace 或授权文档 namespace 执行。Neo4j 不是关系数据库
“一知识库一张表”，所有图数据共享图存储，通过属性/标签和 namespace 隔离。

## 5. DuckDB

DuckDB 嵌入 Go 进程，不是长期服务容器。`data_analysis` 调用时：

1. 从对象存储下载授权 Excel/CSV。
2. 创建会话级内存表。
3. 执行经过校验的只读 SQL。
4. 返回行数据。
5. 会话清理时 DROP 临时表。

它不参与普通 PDF/Word RAG，也不替代 PostgreSQL 或 Milvus。

## 6. 一致性

PostgreSQL 是业务状态权威源。Milvus、Neo4j 和对象存储属于关联资源，无法与
PostgreSQL 做一个跨系统 ACID 事务，因此服务层使用状态机、幂等任务和补偿删除：

- 先写业务状态，再异步写索引。
- 每个任务携带知识/尝试 ID，重复执行可识别。
- 最终失败写 `task_dead_letters` 和 processing span。
- 重解析先清理旧索引，再建立新索引。
- 删除通过 Service 执行多存储清理；删除非空目录会递归调用完整文档删除，再清理
  目录/文档 ACL 和目录树。
