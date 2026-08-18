# Skills 示例

本目录包含 RocheKAP Agent Skills 的示例。Skill 用于提供可复用说明、参考资料，并按需激活已经注册且已授权的 Go/MCP Tool。

## 目录结构

```text
skills/
├── README.md
└── pdf-processing/
    ├── SKILL.md
    ├── FORMS.md
    └── scripts/          # 只读参考源码，不会执行
        ├── analyze_form.py
        └── extract_text.py
```

## 创建新 Skill

在 Skill 根目录创建文件夹和 `SKILL.md`：

```markdown
---
name: my-new-skill
description: Description of what this skill does and when to use it.
tools:
  - an_existing_go_or_mcp_tool
---

# My New Skill

Instructions for the agent...
```

`tools` 可以省略；省略后 Skill 只提供说明。声明的 Tool 必须已经为当前 Agent 和租户注册并授权。

## 执行链路

```text
用户请求
  → 模型匹配 Skill 摘要
  → 调用 read_skill
  → RocheKAP 读取完整说明并激活 tools 中的 Tool
  → 下一轮模型调用获得这些 Tool
  → 模型调用具体 Tool
```

Skill 目录中的 `.py`、`.sh` 等文件只能通过 `read_skill(file_path=...)` 作为参考内容读取。RocheKAP 不会执行这些文件，也不提供脚本执行 Tool。

新增只组合已有 Tool 的 Skill 可在重新扫描或下一次请求时生效。新增全新的 Go Tool 实现仍需编译和部署 RocheKAP；外部能力也可以通过授权的 MCP 服务提供。

完整说明参阅：[Agent Skills 文档](../../docs/agent-skills.md)。
