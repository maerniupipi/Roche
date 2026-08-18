# Google Drive、MinerU 与 Langfuse

## 1. Google Drive

Google Drive 是数据源 Connector，与本地文件上传并列。配置保存到
`data_sources`，凭据通过专用凭据接口加密处理。

同步步骤：

1. 校验 Service Account/OAuth 凭据。
2. 列出用户选择的 Drive/Folder 范围。
3. 递归枚举目录和文件。
4. 将远端目录映射为 `knowledge_folders`。
5. 新文件进入知识创建流水线。
6. 变化文件按远端 ID 和版本重新解析、切片、向量化。
7. 保存游标和 `sync_logs`。

本地上传与 Drive 同步可以共存于同一知识库。系统内删除 Drive 来源知识不会删除
云端文件；反向写 Google Drive 不是当前 Connector 的职责。

同步配置中的“增量”表示只处理新建或发生变化的远端对象，“全量”表示重新枚举
全部范围。冲突策略和同步删除的实际能力应以 Connector Service 实现与测试为准，
不能仅根据前端开关推断。

## 2. MinerU

MinerU 是解析器，不是对象存储和知识库数据库。原文件先写系统对象存储，再由
Worker 读取并请求 MinerU。适配器把响应统一成：

```text
ReadResult
  MarkdownContent
  ImageRefs[]
  Metadata
  Error
```

自建 MinerU 可直接返回 JSON Markdown + base64 图片；MinerU Cloud 可返回任务
结果/ZIP。两者差异被 Reader 隔离，主流水线只处理 `ReadResult`。

出站地址受 SSRF 校验。内网自建地址需要显式白名单，API Key 必须通过受控配置
注入且日志脱敏。

## 3. Langfuse

Langfuse 用于记录：

- 会话/请求 Trace。
- LLM Generation 输入输出与 Token。
- Agent 工具 Span。
- Rerank、解析和同步等可观测步骤。
- 错误和耗时。

Langfuse 不承担业务会话存储，产品历史仍来自 PostgreSQL `sessions/messages`。
本地默认地址通常为 `http://localhost:3000`；服务器 Compose 可按配置一并启动
Langfuse Web、Worker、ClickHouse 和内部对象存储。

通过 `request_id`、session ID 或 Langfuse trace ID 搜索一轮对话。生产环境需要
限制 Langfuse 访问权限，并对文档正文、Prompt、用户问题和模型回答制定脱敏及
保留策略。
