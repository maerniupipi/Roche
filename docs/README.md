# RD China Enterprise AI Knowledge Hub 技术文档

本文档集以当前代码、全部版本化迁移和 `internal/router/router.go` 为事实来源。
旧版个人空间、共享空间、tenant 成员、双授权表、智能体授权和 PostgreSQL 向量表
不属于当前架构。

## 阅读路线

新同事建议按以下顺序阅读：

1. [系统架构](architecture/system-overview.md)
2. [源码地图](architecture/codebase-map.md)
3. [数据与运行时架构](architecture/data-runtime.md)
4. [Auth Service 与 API Gateway](architecture/auth-service-api-gateway.md)
5. [认证与权限模型](architecture/security-permissions.md)
6. [资源层级、授权与删除](knowledge/resource-access-and-deletion.md)
7. [知识库构建流程](knowledge/ingestion-pipeline.md)
8. [切片、索引与多模态](knowledge/chunking-indexing.md)
9. [检索、重排与引用](knowledge/retrieval-rerank.md)
10. [智能体运行时与工具](agents/runtime-tools.md)
11. [数据库目录](database/README.md)
12. [数据库字段全景目录](database-catalog/README.md)
13. [API 使用说明](api/README.md)
14. [企业认证与组织同步](integrations/oidc-workday.md)
15. [开发环境企业集成 Mock](development/enterprise-mocks.md)
16. [Google Drive、MinerU 与 Langfuse](integrations/google-drive-mineru-langfuse.md)
17. [本地开发、服务器开发与正式部署](development/deployment-modes.md)

## 文档目录

| 目录 | 内容 |
|---|---|
| `architecture/` | 系统边界、模块关系、代码入口、权限模型 |
| `development/` | 本地开发、服务器部署、配置、测试与调试 |
| `knowledge/` | 上传、解析、切片、向量化、检索、图谱 |
| `agents/` | 会话、智能体循环、内置工具、数据分析 |
| `integrations/` | PingIdentity SAML/OIDC Mock、Workday、Google Drive、MinerU、Langfuse |
| `database/` | PostgreSQL 表、Milvus、Redis、对象存储和 Neo4j |
| `database-catalog/` | 38 张应用表及迁移框架表的分组字段字典、来源和 Mock 数据 |
| `api/` | 面向调用方的 REST/SSE 接口说明 |

## 当前系统边界

- 后端：Go、Gin、GORM、Asynq。
- 前端：Vue 3、TypeScript、Vite。
- 关系数据：PostgreSQL。
- 向量检索：Milvus。当前标准部署不使用 PostgreSQL 保存向量。
- 任务队列和流式状态：Redis。
- 原文件、解析图片和会话附件：本地存储、MinIO 或兼容 S3 的对象存储。
- 知识图谱：Neo4j，可选。
- 文档解析：DocReader gRPC；按知识库规则可转发 MinerU。
- 表格分析：DuckDB，仅在需要读取 Excel/CSV 和执行分析 SQL 时临时使用。
- 可观测性：Langfuse，可选。
- 身份认证：独立 Auth Service；开发使用邮箱密码和 Dex OIDC Mock，生产使用 PingIdentity SAML；业务 API 使用平台 JWT。
- 企业组织同步：Workday 适配层，支持 mock 与 HTTP provider。

## 核心概念

### 知识域

`knowledge_domains` 是知识管理分组。知识库必须属于一个知识域。知识域用于
知识库归类、知识域管理员分配和存储配额统计，不代表企业真实部门。

### 企业组织

`org_units` 和 `user_org_memberships` 表示企业组织树和员工归属，可由
Workday 同步。组织成员关系本身不产生知识权限，只是批量授权主体。

### 知识权限

- `knowledge_resource_grants` 用一张表表达知识库、目录和文档的
  `allow/deny + read/manage` 规则。
- 知识库规则继承到全部子资源；目录规则可选择继承；文档规则只作用于自身。
- 黑名单是 `effect=deny`，匹配的 deny 优先于 allow。
- 系统管理员拥有全局管理能力。
- 知识域管理员管理指定知识域及其知识库和授权。
- 普通用户只能读取直接用户授权或其有效企业组织授权得到的资源。
- 智能体没有独立的知识权限；运行时始终使用当前用户的有效授权范围。

### 知识资源层级

一个物理知识库内部使用以下逻辑树：

```text
knowledge_base
├─ folder
│  ├─ folder
│  └─ knowledge
└─ knowledge
```

目录不是新的 Milvus Collection，也不是新的物理知识库。权限服务在查询前把目录
规则展开为允许的 `knowledge_id`，再与知识库 ID 和请求筛选条件共同生成
`SearchTargets`。因此可以获得目录/文件级控制，同时复用同一个物理知识库的解析、
索引和存储配置。

## Swagger

开发模式启动后访问：

```text
http://localhost:8080/swagger/index.html
```

静态产物：

- `docs/swagger.json`
- `docs/swagger.yaml`
- `docs/docs.go`

重新生成：

```bash
make install-swagger
make docs
```

Swagger UI 只在 `GIN_MODE != release` 时挂载。生产环境应由受控的内部文档
站点发布静态 OpenAPI 文件，不应直接暴露调试 UI。

## 文档维护规则

1. 路由以 `internal/router/router.go` 为准。
2. 表结构以 `migrations/versioned/` 中按版本执行后的最终结果为准。
3. API DTO 以 `internal/handler/dto` 和 handler 请求结构为准。
4. 核心流程以 `internal/application/service` 和 `internal/agent` 为准。
5. 修改接口时同时更新 Swag 注释、Swagger 产物和 `docs/api/`。
6. 修改数据结构时同时更新迁移、`docs/database/` 和相关流程文档。
7. 修改目录、文档权限或删除逻辑时同步更新
   `docs/knowledge/resource-access-and-deletion.md`。
