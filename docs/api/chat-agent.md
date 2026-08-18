# Chat and Agent API

## 1. 会话模型

会话是用户私有的对话容器，不绑定固定知识库。普通 RAG 的知识范围由请求传入并与
当前用户授权求交集；Agent 的初始范围直接取当前用户全部有效知识授权。

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/sessions` | 创建会话 |
| `GET` | `/sessions` | 列出当前用户会话 |
| `GET` | `/sessions/{id}` | 会话详情 |
| `PUT` | `/sessions/{id}` | 更新标题或描述 |
| `DELETE` | `/sessions/{id}` | 删除会话 |
| `DELETE` | `/sessions/batch` | 批量删除 |
| `DELETE` | `/sessions/{id}/messages` | 清空消息 |
| `POST` | `/sessions/{session_id}/generate_title` | 根据消息生成标题 |
| `POST` | `/sessions/{session_id}/stop` | 停止流式生成 |
| `POST/DELETE` | `/sessions/{id}/pin` | 置顶或取消置顶 |
| `GET` | `/sessions/continue-stream/{session_id}` | 恢复未结束流 |

创建请求：

```json
{
  "title": "Expense policy",
  "description": "Optional"
}
```

消息读取：

```text
GET    /messages/{session_id}/load
DELETE /messages/{session_id}/{message_id}
```

## 2. 普通知识问答

```http
POST /api/v1/knowledge-chat/{session_id}
Authorization: Bearer <token>
Content-Type: application/json
Accept: text/event-stream
```

请求示例：

```json
{
  "query": "国内差旅报销需要哪些审批？",
  "knowledge_base_ids": ["kb-a"],
  "knowledge_ids": [],
  "agent_enabled": false,
  "web_search_enabled": false,
  "mentioned_items": [],
  "tag_ids": [],
  "enable_memory": false,
  "images": [],
  "attachment_uploads": [],
  "channel": "web"
}
```

## 3. Agent 问答

```http
POST /api/v1/agent-chat/{session_id}
Authorization: Bearer <token>
Content-Type: application/json
Accept: text/event-stream
```

请求体与知识问答共用 `CreateKnowledgeQARequest`，Agent 模式通常增加：

```json
{
  "query": "比较这两个政策并给出差异",
  "agent_enabled": true,
  "agent_id": "agent-uuid",
  "knowledge_base_ids": ["kb-a", "kb-b"],
  "knowledge_ids": [],
  "web_search_enabled": false,
  "mcp_service_ids": [],
  "skill_names": [],
  "disable_title": false,
  "channel": "web"
}
```

关键字段：

| 字段 | 作用 |
|---|---|
| `knowledge_base_ids` | 普通 RAG 的本轮知识库筛选；Agent 当前忽略该字段作为初始范围输入 |
| `knowledge_ids` | 普通 RAG 的本轮文档筛选；Agent 当前忽略该字段作为初始范围输入 |
| `agent_id` | 选择 Agent 行为、提示词、模型和允许工具 |
| `mentioned_items` | 前端 `@` 选择项的结构化表示 |
| `tag_ids` | 标签显示和过滤信息 |
| `enable_memory` | 三态覆盖；省略时使用用户偏好 |
| `images` | 图片 base64 或已保存 URL |
| `attachment_uploads` | 临时附件 |

Agent 没有独立知识权限。即使 Agent 配置或模型计划要求访问某知识库，服务端也只
把当前用户全部有效授权构造成 `SearchTargets`。模型随后可在工具调用中指定
`knowledge_base_ids` 来缩小范围，但不能通过请求字段、`@知识库` 或换用其他工具
访问范围外资源。

## 4. SSE 处理

聊天接口返回 `text/event-stream`。客户端应：

1. 持续读取事件，不按普通 JSON 一次性解析。
2. 按事件类型累积正文、思考、工具步骤和引用。
3. 使用服务端返回的稳定 `chunk_id` 展示引用。
4. 收到结束事件后再将本轮标记为完成。
5. 网络中断时调用 `/sessions/continue-stream/{session_id}`。

代理、网关和 Nginx 必须关闭响应缓冲，否则前端会在生成结束后一次性看到所有内容。

## 5. 仅检索

不需要 LLM 总结时：

```http
POST /api/v1/knowledge-search
Authorization: Bearer <token>
Content-Type: application/json

{
  "query": "差旅审批",
  "knowledge_base_ids": ["kb-a"],
  "knowledge_ids": [],
  "tag_ids": [],
  "mentioned_items": []
}
```

服务端执行授权过滤、召回、可选重排和父块扩展，并返回结构化 Chunk。

## 6. Agent 管理

| 方法 | 路径 | 权限 |
|---|---|---|
| `GET` | `/agents` | 已登录 |
| `GET` | `/agents/{id}` | 已登录 |
| `GET` | `/agents/placeholders` | 已登录 |
| `GET` | `/agents/type-presets` | 已登录 |
| `GET` | `/agents/{id}/suggested-questions` | 已登录 |
| `POST` | `/agents` | 系统管理员 |
| `PUT` | `/agents/{id}` | 系统管理员 |
| `DELETE` | `/agents/{id}` | 系统管理员 |
| `POST` | `/agents/{id}/copy` | 系统管理员 |

Agent 配置控制模型、提示词、工具和迭代参数，不保存一套独立的用户或知识授权。

## 7. 内置知识工具

| UI 名称 | 工具名 | 主要输入 | 作用 |
|---|---|---|---|
| 语义搜索 | `knowledge_search` | `queries`, 可选知识范围 | 向量、关键词或混合检索 |
| 关键词搜索 | `grep_chunks` | `query` | 对已授权 Chunk 做正则/文本匹配 |
| 查看文档分块 | `list_knowledge_chunks` | 文档或 Chunk 标识、分页 | 顺序读取文档完整分块 |
| 获取文档信息 | `get_document_info` | 文档或 FAQ IDs | 返回名称、状态、元数据和 Chunk 数 |
| 查询知识图谱 | `query_knowledge_graph` | 知识库 IDs、query | 查询实体和一跳关系 |
| 查看数据元信息 | `data_schema` | `knowledge_id` | 读取 Excel/CSV 表结构 |
| 数据分析 | `data_analysis` | `knowledge_id`, SQL | 将表格加载到 DuckDB 执行只读 SQL |
| 查询数据库 | `database_query` | SQL | 查询允许列表内的平台只读业务表 |

工具共享本轮由服务端预计算的 `SearchTargets`，并在精确文档/Chunk 读取时继续
验证目标是否处于范围内。禁止依赖前端隐藏选项来实现安全控制。

## 8. 引用

模型答案中的引用使用稳定 Chunk ID：

```xml
<kb doc="Policies/Travel.pdf"
    chunk_id="93514f58-ee1d-6874-ac0b-f3ee5edbe574" />
```

前端通过 Chunk ID 读取引用详情。引用“加载失败”通常表示：

- Chunk 已被重解析或删除；
- 当前用户已失去对应文档权限；
- 文档级授权只允许部分文件；
- 引用生成后索引与 PostgreSQL 状态短暂不一致。

引用详情接口同样执行权限检查，不能因为 ID 已出现在历史消息中就绕过授权。
