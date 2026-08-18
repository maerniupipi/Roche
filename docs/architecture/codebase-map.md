# 源码地图

## 1. 顶层目录

| 路径 | 作用 |
|---|---|
| `cmd/server` | Go 服务主入口 |
| `cmd/auth-service` | 独立认证服务入口 |
| `internal/authserver` | Auth-only 路由、数据库连接和 Gateway 内部 Token 校验 |
| `internal/router` | HTTP 路由和中间件装配 |
| `internal/handler` | HTTP 参数解析、响应和 SSE |
| `internal/application/service` | 核心业务服务 |
| `internal/application/repository` | PostgreSQL、Milvus 等数据访问 |
| `internal/agent` | ReAct 循环、工具注册与执行 |
| `internal/types` | 核心实体、配置和跨层类型 |
| `internal/infrastructure/chunker` | 文档切片实现 |
| `internal/middleware` | 认证、审计、限流和权限守卫 |
| `internal/container` | 依赖注入与 Worker 注册 |
| `migrations/versioned` | PostgreSQL 基线及版本化迁移 |
| `frontend` | Vue 3 前端 |
| `docreader` | Python gRPC 文档解析服务 |
| `config` | 模板、内置智能体、mock 集成数据 |
| `scripts` | 本地和服务器启动脚本 |

## 2. 请求分层

```text
router
  -> middleware
  -> handler
  -> application/service
  -> application/repository
  -> PostgreSQL / Milvus / Redis / Neo4j / Object Storage / External API
```

Handler 不应直接拼 SQL，Repository 不应决定业务权限，前端也不能替代后端权限校验。

## 3. 知识库构建核心文件

| 阶段 | 核心文件 |
|---|---|
| 上传接口 | `internal/handler/knowledge.go` |
| 创建知识记录和保存文件 | `internal/application/service/knowledge_create.go` |
| 异步解析总流程 | `internal/application/service/knowledge_process.go` |
| DocReader 客户端 | `internal/infrastructure/docparser/grpc_parser.go`、`internal/grpc/` |
| MinerU 自建/Cloud 适配 | `internal/infrastructure/docparser/mineru_converter.go`、`mineru_cloud_converter.go` |
| 图片解析与持久化前处理 | `internal/infrastructure/docparser/image_resolver.go` |
| 切片器 | `internal/infrastructure/chunker/` |
| Chunk 持久化 | `internal/application/repository/chunk.go` |
| 索引组织 | `internal/application/service/retriever/` |
| Milvus Repository | `internal/application/repository/retriever/milvus/` |
| 图片 OCR/Caption | `internal/application/service/image_multimodal.go` |
| 表格摘要 | `internal/application/service/extract.go` |
| 图谱抽取 | `internal/application/service/extract.go`、`internal/application/service/knowledge_process.go` |
| 进度状态 | `internal/application/repository/knowledge_span.go` |

主异步入口是：

```go
func (s *knowledgeService) ProcessDocument(...)
```

阅读时应继续跟踪它调用的解析、切片、索引和 enrichment 子任务，而不是只看上传 handler。

## 4. 检索和问答核心文件

| 阶段 | 核心文件 |
|---|---|
| 普通 RAG HTTP | `internal/handler/session/qa.go` |
| 普通 RAG Service | `internal/application/service/session_knowledge_qa.go` |
| Agent HTTP | `internal/handler/session/qa.go` |
| Agent Service | `internal/application/service/session_agent_qa.go` |
| Agent 循环 | `internal/agent/` |
| 内置知识工具 | `internal/agent/tools/` |
| 检索组合器 | `internal/application/service/retriever/composite.go` |
| 混合检索 | `internal/application/service/retriever/keywords_vector_hybrid_indexer.go` |
| 重排与上下文 | `internal/application/service/chat_pipeline/` |
| 引用结构 | `internal/types/message.go` |
| 会话/消息存储 | `internal/application/repository/session.go`、`message.go` |

## 5. 权限核心文件

| 责任 | 路径 |
|---|---|
| 路由和资源 API | `internal/router/router.go`、`internal/handler/enterprise_access.go` |
| 知识库访问中间件 | `internal/middleware/kb_access.go` |
| 统一 ACL 计算 | `internal/application/service/enterprise_access.go` |
| ACL、组织和资源查询 | `internal/application/repository/enterprise_access.go` |
| 知识域管理员 | `internal/application/service/knowledge_domain_admin.go` |
| 认证中间件 | `internal/middleware/auth.go` |
| 组织、ACL 和计算结果类型 | `internal/types/enterprise_access.go` |
| 检索范围构建 | `internal/application/service/session_knowledge_qa.go`、`session_agent_qa.go` |
| Agent 工具范围复用 | `internal/agent/tools/scope_authorization.go` |
| 目录递归删除业务 | `internal/application/service/knowledge.go` |
| 目录/文档 ACL 清理 | `internal/application/repository/knowledge.go` |
| ACL 数据库迁移 | `migrations/versioned/000002_knowledge_resource_grants.up.sql` |
| 前端授权弹窗 | `frontend/src/views/knowledge/components/KnowledgeBaseAccessDialog.vue` |
| 前端授权 API | `frontend/src/api/enterprise-access.ts` |
| SAML/OIDC 身份映射 | `internal/handler/auth.go`、`internal/application/service/saml.go`、`internal/application/service/user.go` |
| Gateway 认证边界 | `docker/gateway/`、`docker/Dockerfile.gateway` |

权限阅读顺序建议：

```text
router
  -> EnterpriseAccessHandler
  -> ResolveKnowledgeBaseAccess
  -> ListEffectiveOrgUnitIDs / ListSubjectResourceGrants
  -> buildSearchTargets
  -> Retriever 或 Agent Tool
```

## 6. 外部集成核心文件

| 集成 | 路径 |
|---|---|
| Google Drive 等数据源 | `internal/application/service/datasource_service.go`、`internal/connectors/` |
| Workday | `internal/application/service/enterprise_integration.go` |
| PingIdentity SAML / Dex OIDC | `cmd/auth-service`、`internal/authserver/`、`internal/handler/auth.go`、`internal/application/service/saml.go` |
| Langfuse | `internal/observability/langfuse/` |
| Web Search | `internal/application/service/web_search*.go` |
| MCP | `internal/mcp/`、`internal/application/service/mcp*.go` |

## 7. 修改模块时的最小检查

| 修改类型 | 至少检查 |
|---|---|
| 新增 API | router、handler、DTO、service、Swagger、权限守卫 |
| 修改表 | migration、types、repository、测试、数据库文档 |
| 修改知识流程 | 状态机、任务幂等、清理补偿、Milvus、对象存储 |
| 修改检索 | 用户授权范围、过滤字段、重排、引用 |
| 修改 Agent 工具 | 参数 schema、授权、输出大小、超时、Agent 测试 |
| 修改外部集成 | 凭据边界、SSRF、幂等、游标、审计 |
