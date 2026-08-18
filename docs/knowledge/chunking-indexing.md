# 切片、索引与多模态

## 1. 切片策略

`knowledge_bases.chunking_config` 控制 Go Chunker。可选策略：

| `strategy` | 行为 |
|---|---|
| `legacy` 或空 | 历史递归切分，按 separator、长度和 overlap 工作 |
| `auto` | 分析文档结构后选择 heading、heuristic 或 recursive |
| `heading` | 优先保持 Markdown 标题章节边界 |
| `heuristic` | 识别编号章节、段落和语言特征 |
| `recursive` | 按配置分隔符递归切到目标长度 |

通用参数：

- `chunk_size`：目标字符长度。
- `chunk_overlap`：相邻 Chunk 重叠字符数。
- `token_limit`：大于 0 时额外限制近似 Token 数。
- `languages`：启发式规则的语言提示。
- `parser_engine_rules`：按文件类型选择解析器。

`start_at` 和 `end_at` 是解析后 Markdown 的字符/rune 边界，不是页码、Token
下标或原始二进制偏移。解析器重排文本后，它们不能反向定位 PDF 的精确坐标。

## 2. 标题上下文

heading/auto 选中 heading 时，Chunker 可生成完整标题面包屑，例如：

```text
# 员工手册
## 差旅政策
### 境外住宿
```

正文 Chunk 的 `content` 可能只有“单晚限额为 1800 元”，而向量化内容为：

```text
# 员工手册 > ## 差旅政策 > ### 境外住宿

单晚限额为 1800 元
```

标题上下文保存在运行时 `ContextHeader`，通过 `Chunk.EmbeddingContent()` 拼接，
不写入 `chunks.content`。recursive 和 heuristic 是否产生标题上下文，取决于
Splitter 是否识别出有效 Markdown 标题；不保证每个 Chunk 都有。

## 3. 父子切片

开启 `enable_parent_child` 后：

1. 用 `parent_chunk_size` 生成较大父 Chunk。
2. 在每个父 Chunk 内用 `child_chunk_size` 生成较小子 Chunk。
3. 父块写入 PostgreSQL，`chunk_type=parent_text`。
4. 子块写入 PostgreSQL，`chunk_type=text`，`parent_chunk_id` 指向父块。
5. 只有子块进入向量/关键词索引，父块不直接建立向量。
6. 检索命中子块后，上下文扩展阶段读取父块，以更完整的段落交给 LLM。

示例：

```text
父块 P1: 差旅申请条件、审批链、票据要求，共 1800 字
子块 C1: 申请条件，350 字 -> parent_chunk_id=P1
子块 C2: 审批链，380 字 -> parent_chunk_id=P1
子块 C3: 票据要求，330 字 -> parent_chunk_id=P1
```

用户查询“谁批准境外差旅”命中 C2。系统保留 C2 的命中分数和引用 ID，但组装
LLM 上下文时可使用 P1，避免只给模型一个断裂的审批句子。

## 4. Chunk 类型

当前类型定义在 `internal/types/chunk.go`：

| 类型 | 产生条件 | 是否通常索引 |
|---|---|---|
| `text` | 普通正文或父子切片的子块 | 是 |
| `parent_text` | 开启父子切片 | 否，仅扩展上下文 |
| `image_ocr` | VLM OCR 有结果 | 是 |
| `image_caption` | VLM Caption 有结果 | 是 |
| `summary` | 文档摘要任务成功 | 是 |
| `entity` | 兼容的实体型 Chunk | 取决于流水线 |
| `relationship` | 兼容的关系型 Chunk | 取决于流水线 |
| `faq` | FAQ 条目 | 是 |
| `web_search` | Web 搜索临时结果 | 取决于调用路径 |
| `table_summary` | Excel/CSV 表摘要 | 是 |
| `table_column` | Excel/CSV 列说明 | 是 |

文档 `summary` 是该文档全部普通文本 Chunk 的摘要，不是单个 Chunk 摘要。

## 5. 表格知识

Excel/CSV 首先仍可产生普通文本 Chunk。表格摘要任务另外使用 DuckDB 读取完整
数据、表结构、行数和样例，再由 LLM 产生：

- `table_summary`：表用途、行数、业务含义、关键统计线索。
- `table_column`：列名、推断类型和列语义。

这两个 Chunk 是额外的检索入口，不会替换或修改已有 `text` Chunk。真正计算
“各区域销售总额”时，Agent 使用 `data_analysis` 把原文件加载到会话级 DuckDB，
执行只读 SQL，而不是依赖摘要中的样例值。

## 6. 图片与 VLM

每张已存储图片可同时执行：

- OCR：识别图中可复制文字，生成 `image_ocr`。
- Caption：描述图的结构、对象和语义，生成 `image_caption`。

两者互补。OCR 适合截图、表格和流程节点文字；Caption 能表达“箭头从经理审批
指向财务复核”等纯 OCR 无法还原的视觉关系。结果通过 `parent_chunk_id` 与正文
Chunk 关联，并在 `image_info` 中保留 URL、OCR 和 Caption。

VLM 不重写整份 Markdown，也不会重写原有 `text` Chunk。它只处理解析出来并
成功保存的图片，产生子 Chunk 和索引数据。

## 7. Milvus 索引记录

写入检索引擎前，每份索引文本转换为 `types.IndexInfo`：

| 字段 | 来源与作用 |
|---|---|
| `ID` | 索引记录 ID |
| `Content` | `ContextHeader + Chunk.Content`，或生成问题等实际索引文本 |
| `SourceID` | 本次索引源 ID，用于批量 Embedding 结果回配 |
| `SourceType` | `chunk`、`passage` 或 `summary` 的枚举 |
| `ChunkID` | PostgreSQL `chunks.id`，引用和回表主键 |
| `KnowledgeID` | `knowledges.id`，用于单文档过滤 |
| `KnowledgeBaseID` | `knowledge_bases.id`，用于知识库过滤 |
| `KnowledgeType` | document、FAQ、manual 等业务类型 |
| `TagID` | FAQ 或标签过滤维度 |
| `IsEnabled` | 是否参与召回 |
| `IsRecommended` | 推荐标志 |

Milvus 的 `content` 是“被索引的文本”。普通 Chunk 时可能包含未落库的标题上下文；
生成问题索引时它可能是问题文本。因此它不保证逐字等于 PostgreSQL
`chunks.content`。检索结果依靠 `chunk_id` 回到 PostgreSQL 取得权威正文、
父子关系、图片信息和文档标题。

## 8. 删除与重建

重新解析时，系统清理旧 Chunk 和对应索引，再按当前配置重建。删除知识文档或
知识库时，应同时清理：

- PostgreSQL 的文档、目录关系、Chunk、标签关系、授权和处理记录。
- Milvus 中匹配的 Chunk 向量。
- Neo4j 中对应 namespace 的图数据。
- 对象存储中的原文件和解析图片。

删除动作由后台服务协调，不应直接在数据库中手工删一张表，否则会留下孤立数据。

删除逻辑目录时也不会只删除目录行。系统先枚举整个子树，对每份后代文档执行上述
完整清理，再删除文档/目录上的 `knowledge_resource_grants` 和目录节点。详见
[资源层级、授权与删除](resource-access-and-deletion.md)。
