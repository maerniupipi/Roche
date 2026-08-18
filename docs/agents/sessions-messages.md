# 会话、消息与流式输出

## 1. 数据模型

会话主要涉及两张表：

### `sessions`

保存 `id`、`user_id`、标题、描述、置顶状态和时间。`last_request_state` 保存前端
恢复输入框所需的上一次请求状态，不是永久知识授权，也不能替代服务端权限检查。

### `messages`

每条用户、助手或工具相关消息保存：

- `request_id`：一轮请求的稳定 ID。
- `session_id`：所属会话。
- `role`、`content`、`rendered_content`。
- `knowledge_references`：引用 Chunk/文档数据。
- `agent_steps`：工具调用和观察步骤。
- `mentioned_items`：本轮选择项。
- `images`、`attachments`。
- `is_completed`、`is_fallback`、`agent_duration_ms`。

会保存本轮工具使用情况，位于 `messages.agent_steps`；Langfuse 则保存更细的运行时
Trace。两者用途不同：PostgreSQL 用于产品会话回放，Langfuse 用于调试、性能和
模型调用分析。

## 2. API

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/v1/sessions` | 创建会话 |
| GET | `/api/v1/sessions` | 当前用户会话列表 |
| GET | `/api/v1/sessions/{id}` | 会话详情 |
| PUT | `/api/v1/sessions/{id}` | 修改标题/描述 |
| DELETE | `/api/v1/sessions/{id}` | 删除会话 |
| DELETE | `/api/v1/sessions/{id}/messages` | 清空消息 |
| POST | `/api/v1/sessions/{session_id}/stop` | 停止当前生成 |
| GET | `/api/v1/sessions/continue-stream/{session_id}` | 恢复流 |
| GET | `/api/v1/messages/{session_id}/load` | 分页加载消息 |
| POST | `/api/v1/knowledge-chat/{session_id}` | 普通 RAG |
| POST | `/api/v1/agent-chat/{session_id}` | Agentic RAG |

所有会话操作都应校验 `sessions.user_id` 与当前 JWT 用户一致。切换账号后不能仅凭
会话 UUID 读取其他用户消息。

## 3. SSE

问答接口采用 Server-Sent Events。典型事件包括：

- 请求建立和阶段进度。
- 模型正文 Token。
- Agent 工具开始、参数、结果和错误。
- 引用更新。
- 最终完成或取消。

前端断线后可通过 continue-stream 恢复仍保存在 Redis 中的流状态。Redis 数据有
TTL，不是长期消息存储；最终完整消息仍写 PostgreSQL。
