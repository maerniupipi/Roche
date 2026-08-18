# 05 智能体、会话与消息

本组共 3 张表：

```text
custom_agents
└─ sessions
   └─ messages
```

> 当前权限设计中，智能体不拥有独立授权表。用户可检索范围由知识库/文档授权计算，不能因为选择了某个智能体而越权。

## 1. `custom_agents`

**用途：** 保存内置和自定义智能体的行为配置，包括提示词、工具开关、推理参数和默认检索行为。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 智能体 UUID。 |
| `name` | varchar | 智能体显示名称。 |
| `description` | text | 智能体用途说明。 |
| `avatar` | text | 图标或头像地址。 |
| `is_builtin` | boolean | 是否为系统内置智能体。 |
| `created_by` | varchar(36) | 创建者 `users.id`。 |
| `config` | jsonb | 系统提示词、工具列表、模型参数、最大轮次等核心配置。 |
| `created_at` | timestamptz | 创建时间。 |
| `updated_at` | timestamptz | 配置最后修改时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |

**Mock 数据：**

```json
{
  "id": "agent-reasoning-001",
  "name": "智能推理",
  "description": "使用 ReAct 循环检索知识并生成带引用回答",
  "avatar": "/assets/agents/reasoning.svg",
  "is_builtin": true,
  "created_by": "usr-admin-001",
  "config": {
    "system_prompt": "仅依据授权知识回答，并为事实附加引用。",
    "max_iterations": 8,
    "tools": [
      "knowledge_search",
      "grep_chunks",
      "list_knowledge_chunks",
      "get_document_info",
      "query_knowledge_graph"
    ]
  },
  "created_at": "2026-07-29T09:00:00+08:00",
  "updated_at": "2026-07-29T09:00:00+08:00",
  "deleted_at": null
}
```

## 2. `sessions`

**用途：** 一段对话会话的主档。会话归属于用户，并保存最近一次输入栏 UI 状态；检索授权不会从会话记录中读取。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 会话 UUID。 |
| `title` | varchar | 会话标题，可能由首条问题或模型生成。 |
| `description` | text | 会话说明。 |
| `last_request_state` | jsonb, nullable | 最近一次输入栏选择，用于重新打开会话时恢复 UI；不是授权或检索范围事实源。 |
| `created_at` | timestamptz | 会话创建时间。 |
| `updated_at` | timestamptz | 最后一次对话或设置更新时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `user_id` | varchar(512) | 会话所有者；支持本地用户 ID 和外部 API principal。 |
| `is_pinned` | boolean | 是否置顶会话。 |
| `pinned_at` | timestamptz | 置顶时间。 |

**Mock 数据：**

```json
{
  "id": "session-20260729-001",
  "title": "国内异地派遣审批权限",
  "description": null,
  "last_request_state": {
    "agent_id": "agent-reasoning-001",
    "agent_enabled": true,
    "model_id": "model-qa-001",
    "knowledge_base_ids": ["kb-fin-policy-001"],
    "knowledge_ids": [],
    "tag_ids": []
  },
  "created_at": "2026-07-29T10:00:00+08:00",
  "updated_at": "2026-07-29T10:02:15+08:00",
  "deleted_at": null,
  "user_id": "usr-viewer-001",
  "is_pinned": false,
  "pinned_at": null
}
```

## 3. `messages`

**用途：** 保存用户消息、助手回答、引用、智能体步骤、附件和渲染结果。一次 ReAct 工具调用过程可序列化到 `agent_steps`。

| 字段 | 类型/约束 | 含义与值来源 |
|---|---|---|
| `id` | varchar(36), PK | 消息 UUID。 |
| `request_id` | varchar | 一次前端请求或流式回答的关联 ID，可用于日志串联。 |
| `session_id` | varchar(36) | 所属 `sessions.id`。 |
| `role` | varchar | 消息角色，如 `user`、`assistant`、`system`。 |
| `content` | text | 用户输入或模型回答的原始内容。 |
| `knowledge_references` | jsonb | 回答引用的文档、Chunk ID、文件名和引用片段。 |
| `agent_steps` | jsonb | 智能体计划、工具名称、输入、观察结果和迭代步骤。 |
| `is_completed` | boolean | 流式回答是否完整结束。 |
| `created_at` | timestamptz | 消息创建时间。 |
| `updated_at` | timestamptz | 流式内容或状态最后更新时间。 |
| `deleted_at` | timestamptz | 软删除时间。 |
| `mentioned_items` | jsonb | 用户通过界面提及的知识库、文档等对象。 |
| `is_fallback` | boolean | 是否为回退回答。 |
| `agent_duration_ms` | bigint | 智能体完整执行耗时。 |
| `images` | jsonb | 用户输入或回答涉及的图片信息。 |
| `channel` | varchar | 消息来源渠道，如 `web`、`api`。 |
| `rendered_content` | text | 为前端展示预处理后的回答内容。 |
| `attachments` | jsonb | 临时上传文件等附件信息。 |

**Mock 数据：**

```json
{
  "id": "message-assistant-001",
  "request_id": "req-9f7a6c",
  "session_id": "session-20260729-001",
  "role": "assistant",
  "content": "直线经理是流程发起人，最终由指定负责人授权。",
  "knowledge_references": [
    {
      "knowledge_id": "knowledge-doa-001",
      "chunk_id": "chunk-doa-001-03",
      "document": "RDSL_DOA_16.0.pdf"
    }
  ],
  "agent_steps": [
    {
      "iteration": 1,
      "tool": "knowledge_search",
      "input": {"queries": ["国内异地派遣 直线经理 审批"]},
      "status": "success"
    }
  ],
  "is_completed": true,
  "created_at": "2026-07-29T10:02:00+08:00",
  "updated_at": "2026-07-29T10:02:15+08:00",
  "deleted_at": null,
  "mentioned_items": [],
  "is_fallback": false,
  "agent_duration_ms": 15230,
  "images": [],
  "channel": "web",
  "rendered_content": "<p>直线经理是流程发起人，最终由指定负责人授权。</p>",
  "attachments": []
}
```

## 本组关系摘要

```text
users.id
  └─ sessions.user_id
       └─ messages.session_id

custom_agents are platform definitions selected per request; sessions do not
persist an agent foreign key.

messages.knowledge_references[*].chunk_id
  └─ chunks.id
```

`agent_steps` 是运行记录，不是权限依据；所有知识工具在执行时仍需按当前用户授权范围过滤。
