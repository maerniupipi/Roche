# 04 知识库核心数据

本组共 7 张 PostgreSQL 表：

```text
knowledge_bases
├─ knowledge_folders
├─ knowledges
│  ├─ chunks
│  ├─ knowledge_tag_relations ── knowledge_tags
│  └─ knowledge_processing_spans
```

> 边界说明：原始文件和解析产物保存在本地存储或 S3/MinIO；Embedding 向量保存在 Milvus；图谱实体和关系保存在 Neo4j。PostgreSQL 保存业务主档、文本 Chunk、处理状态和这些外部数据的关联 ID。

## 1. `knowledge_bases`

**用途：** 一套知识构建和检索策略的顶层容器。它定义使用哪个解析、切片、Embedding、VLM、图谱和向量存储配置。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 知识库 UUID，由后端创建。 |
| `name` | varchar | 用户输入的知识库名称。 |
| `description` | text | 用户输入的知识库描述。 |
| `knowledge_domain_id` | integer | 所属 `knowledge_domains.id`。 |
| `chunking_config` | jsonb | 分块大小、重叠、切片模式、父子切片等配置。 |
| `image_processing_config` | jsonb | 图片提取、OCR、Caption 等配置。 |
| `embedding_model_id` | varchar(36) | 构建向量时使用的 `models.id`。 |
| `summary_model_id` | varchar(36) | 摘要、问题生成等任务使用的模型 ID。 |
| `cos_config` | jsonb | 历史对象存储配置兼容字段；新配置优先使用平台存储配置。 |
| `vlm_config` | jsonb | VLM 模型、提示词及图片处理参数。 |
| `extract_config` | jsonb | 实体关系抽取、元数据抽取等配置。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 配置最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `is_temporary` | boolean | 是否为临时知识库，例如会话临时上传场景。 |
| `type` | varchar | 知识库类型，当前主要为 `document` 或 `faq`。 |
| `faq_config` | jsonb | FAQ 导入、匹配等专用配置。 |
| `question_generation_config` | jsonb | 为 Chunk 自动生成问题的数量、模型等配置。 |
| `storage_provider_config` | jsonb | 知识库级存储提供商覆盖配置。 |
| `asr_config` | jsonb | 音频转写模型及参数。 |
| `vector_store_id` | varchar(36) | 使用的 `vector_stores.id`。 |
| `indexing_strategy` | jsonb | RAG、图谱等索引管线开关和策略。 |
| `creator_id` | varchar(36) | 创建该知识库的 `users.id`。 |

**Mock 数据：**

```json
{
  "id": "kb-fin-policy-001",
  "name": "Finance Policy",
  "description": "财务制度与差旅政策",
  "knowledge_domain_id": 1,
  "chunking_config": {
    "strategy": "parent_child",
    "chunk_size": 512,
    "chunk_overlap": 80,
    "parent_chunk_size": 1600
  },
  "image_processing_config": {"enabled": true, "ocr": true, "caption": true},
  "embedding_model_id": "model-embed-001",
  "summary_model_id": "model-qa-001",
  "cos_config": {},
  "vlm_config": {"enabled": true, "model_id": "model-vlm-001"},
  "extract_config": {"graph_enabled": false},
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:10:00+08:00",
  "deleted_at": null,
  "is_temporary": false,
  "type": "document",
  "faq_config": {},
  "question_generation_config": {"enabled": true, "count": 3},
  "storage_provider_config": {"provider": "minio"},
  "asr_config": {"enabled": false},
  "vector_store_id": "vs-milvus-001",
  "indexing_strategy": {"rag": true, "graph": false},
  "creator_id": "usr-admin-001"
}
```

## 2. `knowledge_folders`

**用途：** 在一个知识库内部保存目录树。上传目录时，目录节点进入本表，文档通过 `knowledges.folder_id` 挂到目录下。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 目录 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `knowledge_base_id` | varchar(36), FK | 所属知识库。 |
| `parent_id` | varchar(36), self FK, nullable | 父目录 ID；为空表示知识库根目录下的一级目录。 |
| `name` | varchar | 当前一级目录名称，不包含完整路径。 |
| `relative_path` | text | 相对知识库根目录的规范化完整路径，用于唯一定位。 |
| `created_at` | timestamptz | 目录记录创建时间。 |
| `updated_at` | timestamptz | 目录名称或层级最后更新时间。 |

**Mock 数据：**

```json
{
  "id": "folder-treasury-001",
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-fin-policy-001",
  "parent_id": null,
  "name": "Treasury",
  "relative_path": "Finance/Treasury",
  "created_at": "2026-07-29T09:15:00+08:00",
  "updated_at": "2026-07-29T09:15:00+08:00"
}
```

## 3. `knowledges`

**用途：** 一份知识文档或 FAQ 导入对象的业务主档。它记录文件名、对象存储路径、解析状态、元数据和所属目录，但不保存完整二进制文件。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 知识文档 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `knowledge_base_id` | varchar(36), FK/逻辑 FK | 所属知识库 ID。 |
| `type` | varchar | 知识类型，例如文件、文本或 FAQ 导入对象。 |
| `title` | varchar | 展示标题；目录功能启用后，新上传文件通常保存文件基名。 |
| `description` | text | 用户填写或处理流程生成的文档描述。 |
| `source` | varchar | 来源标识，如本地上传、Google Drive、API。 |
| `parse_status` | varchar | 解析状态：`pending`、`processing`、`finalizing`、`completed`、`failed`、`deleting`、`cancelled`。 |
| `enable_status` | boolean | 是否允许进入检索。 |
| `embedding_model_id` | varchar(36) | 本文档向量化实际使用的模型 ID。 |
| `file_name` | varchar | 原始文件名。 |
| `file_type` | varchar | 扩展名或 MIME 对应类型，如 `pdf`、`docx`。 |
| `file_size` | bigint | 上传原文件字节数。 |
| `file_path` | text | 原文件在本地/S3/MinIO 中的对象键或路径。 |
| `file_hash` | varchar | 文件内容哈希，用于去重和变化检测。 |
| `storage_size` | bigint | 原文件及相关产物占用的统计空间。 |
| `metadata` | jsonb | 解析器、外部数据源、页数、对象键等扩展元数据。 |
| `created_at` | timestamptz | 文档主档创建时间，通常接近上传开始时间。 |
| `updated_at` | timestamptz | 状态、元数据等最后更新时间。 |
| `processed_at` | timestamptz | 文档成功完成知识构建的时间。 |
| `error_message` | text | 最近一次处理失败信息。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `summary_status` | varchar | 摘要任务状态：`none`、`pending`、`processing`、`completed`、`failed`。 |
| `last_faq_import_result` | jsonb | 最近一次 FAQ 导入统计及错误。 |
| `channel` | varchar | 上传或同步渠道标识。 |
| `pending_subtasks_count` | integer | 尚未完成的异步子任务数量。 |
| `folder_id` | varchar(36), FK, nullable | 所属 `knowledge_folders.id`；根目录文档为空。 |

**Mock 数据：**

```json
{
  "id": "knowledge-doa-001",
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-fin-policy-001",
  "type": "file",
  "title": "RDSL_DOA_16.0.pdf",
  "description": "Delegation of Authority policy",
  "source": "local_upload",
  "parse_status": "completed",
  "enable_status": true,
  "embedding_model_id": "model-embed-001",
  "file_name": "RDSL_DOA_16.0.pdf",
  "file_type": "pdf",
  "file_size": 1850342,
  "file_path": "documents/kb-fin-policy-001/knowledge-doa-001/RDSL_DOA_16.0.pdf",
  "file_hash": "sha256:4f6d...a821",
  "storage_size": 2448100,
  "metadata": {
    "parser": "docreader",
    "page_count": 42,
    "relative_path": "Finance/Treasury/RDSL_DOA_16.0.pdf"
  },
  "created_at": "2026-07-29T09:20:00+08:00",
  "updated_at": "2026-07-29T09:24:30+08:00",
  "processed_at": "2026-07-29T09:24:30+08:00",
  "error_message": null,
  "deleted_at": null,
  "summary_status": "completed",
  "last_faq_import_result": null,
  "channel": "web",
  "pending_subtasks_count": 0,
  "folder_id": "folder-treasury-001"
}
```

## 4. `chunks`

**用途：** 保存解析后可检索的文本单元、父子关系、图片信息和处理元数据。Milvus 中的向量记录通过 `chunk_id` 回指本表。

常见 `chunk_type`：

| 值 | 含义 |
|---|---|
| `text` | 普通正文切片 |
| `parent_text` | 父子切片中的父 Chunk |
| `image_ocr` | 图片 OCR 文本 |
| `image_caption` | VLM 图片描述 |
| `summary` | 文档或处理管线生成的摘要切片 |
| `entity` | 图谱实体相关切片 |
| `relationship` | 图谱关系相关切片 |
| `faq` | FAQ 问答切片 |
| `web_search` | Web 搜索结果切片 |
| `table_summary` | 表格整体摘要 |
| `table_column` | 表格列说明 |

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | Chunk UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `knowledge_base_id` | varchar(36) | 所属知识库。 |
| `knowledge_id` | varchar(36) | 所属文档。 |
| `content` | text | 原始切片文本或增强后的文本内容。 |
| `chunk_index` | integer | 在文档中的切片顺序。 |
| `is_enabled` | boolean | 是否允许检索。 |
| `start_at` | integer | 在解析文本中的起始字符偏移。 |
| `end_at` | integer | 在解析文本中的结束字符偏移。 |
| `pre_chunk_id` | varchar(36) | 前一个相邻 Chunk ID。 |
| `next_chunk_id` | varchar(36) | 后一个相邻 Chunk ID。 |
| `chunk_type` | varchar | 上表所列切片类型。 |
| `parent_chunk_id` | varchar(36) | 父子切片中的父 Chunk ID。 |
| `image_info` | jsonb | 该 Chunk 关联图片的 URL、对象键、OCR/Caption 等信息。 |
| `relation_chunks` | jsonb | 显式关联 Chunk 列表。 |
| `indirect_relation_chunks` | jsonb | 间接关联 Chunk 列表。 |
| `created_at` | timestamptz | Chunk 首次生成时间。 |
| `updated_at` | timestamptz | 内容、状态或索引信息最后更新时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `metadata` | jsonb | 标题层级、页码、表格位置、解析器属性等扩展信息。 |
| `tag_id` | varchar/兼容字段 | 旧版单标签兼容字段；当前文档标签以关系表为主。 |
| `status` | integer | 索引状态：常见为 `0` 初始、`1` 已保存、`2` 已索引。 |
| `content_hash` | varchar | Chunk 内容哈希，用于增量识别。 |
| `flags` | integer | 位标志集合；例如推荐 Chunk 标志。 |
| `seq_id` | bigint | 可排序序号。 |
| `video_info` | jsonb | 视频片段、时间戳等扩展信息。 |

**Mock 数据：**

```json
{
  "id": "chunk-doa-001-03",
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-fin-policy-001",
  "knowledge_id": "knowledge-doa-001",
  "content": "## 2.7.3.2 国内异地派遣流程\n所有其他员工的流程发起人为直线经理。",
  "chunk_index": 3,
  "is_enabled": true,
  "start_at": 2048,
  "end_at": 2101,
  "pre_chunk_id": "chunk-doa-001-02",
  "next_chunk_id": "chunk-doa-001-04",
  "chunk_type": "text",
  "parent_chunk_id": "chunk-doa-parent-001",
  "image_info": [],
  "relation_chunks": [],
  "indirect_relation_chunks": [],
  "created_at": "2026-07-29T09:22:00+08:00",
  "updated_at": "2026-07-29T09:23:30+08:00",
  "deleted_at": null,
  "metadata": {"page": 16, "headers": ["2.7 HR", "2.7.3 Relocation"]},
  "tag_id": null,
  "status": 2,
  "content_hash": "sha256:71f2...5ac0",
  "flags": 0,
  "seq_id": 103,
  "video_info": null
}
```

## 5. `knowledge_tags`

**用途：** 定义知识库内部可复用的标签。标签由用户或管理流程建立，不是每次查询时由 LLM 自动创建。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 标签 UUID。 |
| `knowledge_domain_id` | integer | 所属知识域。 |
| `knowledge_base_id` | varchar(36) | 所属知识库。 |
| `name` | varchar | 标签名称。 |
| `color` | varchar | 前端显示颜色。 |
| `sort_order` | integer | 前端排序值。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `seq_id` | bigint | 数字顺序 ID，便于排序或索引。 |

**Mock 数据：**

```json
{
  "id": "tag-treasury-001",
  "knowledge_domain_id": 1,
  "knowledge_base_id": "kb-fin-policy-001",
  "name": "Treasury",
  "color": "#00B86B",
  "sort_order": 10,
  "created_at": "2026-07-29T09:30:00+08:00",
  "updated_at": "2026-07-29T09:30:00+08:00",
  "deleted_at": null,
  "seq_id": 8
}
```

## 6. `knowledge_tag_relations`

**用途：** 建立文档与标签的多对多关系。一份文档可有多个标签，一个标签也可关联多份文档。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `knowledge_id` | varchar(36), 联合主键/FK | 被标记的 `knowledges.id`。 |
| `tag_id` | varchar(36), 联合主键/FK | 使用的 `knowledge_tags.id`。 |
| `created_at` | timestamptz | 关联建立时间。 |

**Mock 数据：**

```json
{
  "knowledge_id": "knowledge-doa-001",
  "tag_id": "tag-treasury-001",
  "created_at": "2026-07-29T09:31:00+08:00"
}
```

## 7. `knowledge_processing_spans`

**用途：** 记录一次文档构建任务的阶段化执行轨迹，用于排查 DocReader、切片、Embedding、VLM 等环节在哪里失败。

常见 `kind`：`root`、`stage`、`subspan`、`generation`。

常见 `status`：`pending`、`running`、`done`、`failed`、`skipped`、`cancelled`。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | bigint, PK | Span 数据库 ID。 |
| `knowledge_id` | varchar(36) | 对应文档 ID。 |
| `attempt` | integer | 第几次处理/重试。 |
| `span_id` | varchar | 本次 Span 的稳定标识。 |
| `parent_span_id` | varchar, nullable | 父 Span 标识，用于组成调用树。 |
| `name` | varchar | 阶段名称，如 `docreader`、`chunking`、`embedding`。 |
| `kind` | varchar | Span 层级类型。 |
| `status` | varchar | 执行状态。 |
| `input` | jsonb | 脱敏后的输入摘要。 |
| `output` | jsonb | 输出摘要和计数。 |
| `metadata` | jsonb | 模型、引擎、任务 ID 等诊断数据。 |
| `error_code` | varchar | 稳定错误码。 |
| `error_message` | text | 面向排障的错误摘要。 |
| `error_detail` | text | 更详细的错误信息。 |
| `started_at` | timestamptz | 开始时间。 |
| `finished_at` | timestamptz | 结束时间。 |
| `duration_ms` | bigint | 执行耗时毫秒数。 |
| `created_at` | timestamptz | 记录创建时间。 |
| `updated_at` | timestamptz | 记录最后更新时间。 |

**Mock 数据：**

```json
{
  "id": 5012,
  "knowledge_id": "knowledge-doa-001",
  "attempt": 1,
  "span_id": "span-embedding-001",
  "parent_span_id": "span-root-001",
  "name": "embedding",
  "kind": "stage",
  "status": "done",
  "input": {"chunk_count": 86},
  "output": {"indexed_count": 86, "collection": "roche_kap_embeddings"},
  "metadata": {"model_id": "model-embed-001", "vector_store_id": "vs-milvus-001"},
  "error_code": null,
  "error_message": null,
  "error_detail": null,
  "started_at": "2026-07-29T09:23:00+08:00",
  "finished_at": "2026-07-29T09:23:18+08:00",
  "duration_ms": 18000,
  "created_at": "2026-07-29T09:23:00+08:00",
  "updated_at": "2026-07-29T09:23:18+08:00"
}
```

## 一份文件如何串起这些表

```text
knowledge_bases.id = kb-fin-policy-001
  ├─ knowledge_folders.id = folder-treasury-001
  │    └─ knowledges.folder_id = folder-treasury-001
  ├─ knowledges.id = knowledge-doa-001
  │    ├─ chunks.knowledge_id = knowledge-doa-001
  │    ├─ knowledge_tag_relations.knowledge_id = knowledge-doa-001
  │    └─ knowledge_processing_spans.knowledge_id = knowledge-doa-001
  └─ knowledge_tags.knowledge_base_id = kb-fin-policy-001
```

Milvus 中对应向量至少带有：

```text
chunk_id = chunks.id
knowledge_id = knowledges.id
knowledge_base_id = knowledge_bases.id
```

检索先在 Milvus 找到 `chunk_id`，再回到 PostgreSQL 读取 Chunk、文档、权限和引用信息。
