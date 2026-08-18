---
name: 数据处理器
description: 分析 CSV 或 Excel 知识文档。当用户需要查看表结构、执行统计计算、聚合或生成数据分析结论时使用此技能。
tools:
  - data_schema
  - data_analysis
---

# Data Processor

使用 RocheKAP 内置的结构化数据 Tool 分析 CSV 或 Excel 知识文档。

## 可用 Tool

- `data_schema`：读取指定知识文档的表名、字段和数据量。
- `data_analysis`：针对指定知识文档执行只读分析 SQL。

这些 Tool 只有在本 Skill 被 `read_skill` 加载后才会对模型可见。

## 工作流程

1. 确认用户要分析的 CSV 或 Excel 知识文档。
2. 当字段或表结构不明确时，先调用 `data_schema`。
3. 根据返回的真实字段名生成只读 SQL。
4. 调用 `data_analysis` 执行统计、筛选或聚合。
5. 检查结果是否回答了用户问题，再用清晰的表格或文字总结。

## 约束

- 不猜测字段名；优先使用 `data_schema` 返回的精确名称。
- 只执行查询和分析，不生成修改数据的 SQL。
- 大结果应先聚合或限制行数，避免把大量原始数据放入上下文。
- 如果 Tool 未注册或当前 Agent 无权使用，向用户说明能力不可用，不要尝试执行本地脚本或 shell 命令。
- `scripts/` 目录中的历史 Python 文件仅保留为只读参考，不会被 RocheKAP 执行。

## 示例

用户请求：统计某个销售表中各产品的销售额。

处理步骤：

1. 调用 `data_schema` 获取产品和销售额字段名。
2. 生成按产品分组、汇总销售额的只读 SQL。
3. 调用 `data_analysis`。
4. 返回汇总表并说明主要结论。
