# 知识图谱

## 1. 开启条件

图谱是知识库级配置：

- `knowledge_bases.indexing_strategy.graph_enabled=true`
- `knowledge_bases.extract_config.enabled=true`
- `extract_config.nodes` 或 `extract_config.relations` 至少存在有效定义
- 已配置可用的抽取 Chat 模型
- Neo4j 已启用并可连接

实体配置表示“实体类型/抽取约束”，例如 `Person`、`Policy`、`Department`，不是
要求用户预先录入具体的“张三”。关系配置表示允许或期望抽取的关系类型，例如
`REPORTS_TO`、`APPROVES`。示例文本用于校准抽取，不是最终图数据。

## 2. 构建过程

1. 文档先完成普通解析和 `text` Chunk。
2. 后处理任务逐 Chunk 调用 LLM 抽取实体。
3. 同名实体在当前构建上下文中合并，并累计来源 `chunk_id`。
4. 多个 Chunk 成批交给 LLM 抽取实体间关系。
5. 关系保留来源 Chunk ID 和强度。
6. 数据写入 Neo4j，namespace 同时包含知识库 ID 和知识文档 ID。

因此概念上一个知识库有一套可统一查询的图，同时每个节点/关系带文档 namespace
和来源 Chunk，可在用户只有部分文档权限时限制到授权文档。一个实体可以关联多个
Chunk，也可以跨同一知识库的多份文档出现。

当前合并主要基于抽取名称和模型结果，不是完整的企业主数据消歧系统。“张三”和
“张顺”不会仅因上下文相近自动判定为同一个人；要实现这种实体消融，需要额外的
别名、稳定业务 ID 或人工审核规则。

## 3. 查询过程

`query_knowledge_graph` 输入：

```json
{
  "knowledge_base_ids": ["kb-uuid"],
  "query": "张三负责哪些审批"
}
```

工具把问题整体及分词结果作为图查询词，调用 Neo4j `SearchNode`：

- 整库授权：以知识库 namespace 查询。
- 只有单文档授权：分别以知识库 + 文档 namespace 查询，再合并去重。

返回实体、属性、关系和来源 `chunk_ids`。Agent 可继续调用
`list_knowledge_chunks(chunk_id=...)` 读取完整证据，然后生成带引用回答。

普通 RAG 和图谱工具可以在 Agent 循环中先后或并行使用。图谱提供关系线索，
Milvus/Chunk 提供可引用原文；最终答案不应只依赖没有正文证据的实体名称。

## 4. 存储边界

| 存储 | 图谱场景中的职责 |
|---|---|
| PostgreSQL | 图谱配置、文档、Chunk、处理状态 |
| Milvus | 普通文本和增强文本的向量召回 |
| Neo4j | 实体、关系、属性、来源 Chunk ID |
| 对象存储 | 原文件和图片 |

开启图谱不会改变 `knowledge_bases`、`knowledges`、`chunks` 的主从关系，也不会
用 Neo4j 替代普通 RAG。删除/重解析文档时必须按 namespace 清理旧图数据。
