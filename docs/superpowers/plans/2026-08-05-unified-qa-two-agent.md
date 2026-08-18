# Unified QA 双子智能体实施计划

> 设计规格：`docs/superpowers/specs/2026-08-05-unified-qa-two-agent-design.md`  
> 分支：`feature/jyf-agent-chain`

## 约束

- 所有 `SessionService.KnowledgeQA` 请求直接进入 `UnifiedQAService`。
- 固定且只允许 `finance`、`compliance` 两个子智能体。
- 不增加 `execution_engine`，不设置 `generic`。
- 两个子智能体共享当前用户全部授权知识库和同一份 `SearchTargets`。
- 统一检索复用现有 `KnowledgeBaseService.HybridSearch`、加权 RRF、FAQ 后处理和 Reranker。
- 每个子智能体最多补查一次、复核两次、研究工具调用五次。
- 不修改现有迁移；新增迁移版本为 `000003`。
- 每个任务按红—绿—重构顺序推进，完成后运行相关包回归。

## Task 1：持久化模型与 Repository

文件：

- `migrations/versioned/000003_unified_qa.up.sql`
- `migrations/versioned/000003_unified_qa.down.sql`
- `internal/types/unified_qa.go`
- `internal/types/interfaces/unified_qa.go`
- `internal/application/repository/unified_qa.go`
- `internal/application/repository/unified_qa_test.go`

步骤：

1. 先写 SQLite Repository 测试，覆盖 Run 创建/结束、Langfuse Trace 关联和 JSON 数组读取。
2. 新增 `qa_execution_runs`，本地只保存业务执行结果；节点级技术明细统一写入 Langfuse。
3. JSON 字段复用项目 `types.JSON`/`types.JSONMap`/`types.StringArray` 模式。
4. Repository 所有查询使用 `WithContext`，Run 完成更新不得覆盖初始配置快照。
5. 验证：`go test ./internal/application/repository -run TestUnifiedQA -count=1`。

## Task 2：固定财务/合规 Catalog

文件：

- `config/unified_qa_agents.yaml`
- `config/prompt_templates/unified_qa.yaml`
- `internal/config/unified_qa_agents.go`
- `internal/config/unified_qa_agents_test.go`
- `internal/application/service/unifiedqa/agent_catalog.go`
- `internal/application/service/unifiedqa/agent_catalog_test.go`

步骤：

1. 先写配置测试，证明缺少任意一个固定 Agent、出现第三个 Agent、出现 KB 字段或非法工具时加载失败。
2. 定义 `finance` 和 `compliance` 的 Prompt 版本、研究规则、证据要求和工具白名单。
3. Catalog 只暴露固定查找和有序列表，不提供 `Generic()`。
4. 验证：`go test ./internal/config ./internal/application/service/unifiedqa -run "Test.*AgentCatalog|TestUnifiedQAAgents" -count=1`。

## Task 3：RunContext、配置快照与 NodeRunner

文件：

- `internal/application/service/unifiedqa/types.go`
- `internal/application/service/unifiedqa/errors.go`
- `internal/application/service/unifiedqa/node_runner.go`
- `internal/application/service/unifiedqa/node_runner_test.go`

步骤：

1. 定义 `AuthorizedScope`、`AgentTask`、`MasterRoutePlan`、证据类型、Observation 和不可变配置快照。
2. 先写 NodeRunner 测试，覆盖业务错误透传、Langfuse 观测失败不影响业务，以及节点状态、模型调用 ID、错误码和耗时上报。
3. Slice、map 和配置写入 RunContext 前深拷贝。
4. 验证：`go test ./internal/application/service/unifiedqa -run TestNodeRunner -count=1`。

## Task 4：全部授权知识库解析

文件：

- `internal/application/service/unifiedqa/authorized_kb_resolver.go`
- `internal/application/service/unifiedqa/authorized_kb_resolver_test.go`

步骤：

1. 先写测试，断言返回 `ListKnowledgeBases` 中全部可见且有效的 KB，并忽略入口 Agent 的 KB 配置。
2. 稳定排序后生成 KB ID 和 `SearchTargets`。
3. 指定文件和 Tag 只能形成提示或 KB 内过滤，不能缩小全局授权 KB 集合。
4. 空集合返回 `ErrNoAccessibleKnowledgeBase`，Router 不得被调用。
5. 验证：`go test ./internal/application/service/unifiedqa -run TestAuthorizedKBResolver -count=1`。

## Task 5：统一入口和依赖装配

文件：

- `internal/application/service/unifiedqa/service.go`
- `internal/application/service/session.go`
- `internal/application/service/session_knowledge_qa.go`
- `internal/container/container.go`
- `internal/application/service/session_unified_qa_dispatch_test.go`

步骤：

1. 先写分派测试，断言任意 KnowledgeQA 请求只调用 `UnifiedQAService.Execute`，旧 KB 解析和旧 Pipeline 不被调用。
2. 给 `sessionService` 注入最小 `UnifiedQAService` 接口。
3. `KnowledgeQA` 直接委托新服务；保留旧内部函数供其他现存调用编译使用。
4. 在容器中装配 Repository、Catalog、Resolver、NodeRunner 和 Service。
5. 验证：`go test ./internal/application/service -run "TestKnowledgeQA|TestUnifiedQA" -count=1`。

## Task 6：主路由与严格校验

文件：

- `internal/application/service/unifiedqa/master_router.go`
- `internal/application/service/unifiedqa/route_validator.go`
- `internal/application/service/unifiedqa/master_router_test.go`
- `internal/application/service/unifiedqa/route_validator_test.go`

步骤：

1. 严格 JSON Schema 只允许完整问题、意图、实体和 Agent Tasks。
2. Prompt 同时包含财务、合规研究配置。
3. 校验结果只能是 `[finance]`、`[compliance]` 或 `[finance, compliance]`。
4. 所有任务共享同一个 `standalone_query`；检索词去重且每个 Agent 最多五个。
5. 路由超时、JSON 错误、非法或空结果统一降级为两个任务。
6. 验证：`go test ./internal/application/service/unifiedqa -run "TestMasterRouter|TestRouteValidator" -count=1`。

## Task 7：并行统一检索和研究计划

文件：

- `internal/application/service/unifiedqa/research_plan_validator.go`
- `internal/application/service/unifiedqa/retrieval_adapter.go`
- `internal/application/service/unifiedqa/domain_agent_executor.go`
- 对应 `*_test.go`

步骤：

1. 先写测试，断言两个 Agent 获得完全相同的授权 KB 和完整问题。
2. 每个检索词调用现有 `HybridSearch`，通过 `SearchParams.KnowledgeBaseIDs` 传入全部 KB。
3. 跨查询与 Regex 按资源主键去重，不执行第二次 RRF。
4. 合并候选只调用一次 Reranker；失败时使用原始排序。
5. 两个任务通过 `errgroup` 并行，一个失败转为失败 Observation，不取消另一个。
6. 工具意图取 Agent 和系统白名单交集，总调用预算最多五次。
7. 验证：`go test ./internal/application/service/unifiedqa -run "TestResearchPlan|TestRetrieval|TestDomainAgent" -count=1`。

## Task 8：证据复核、一次补查与聚合

文件：

- `internal/application/service/unifiedqa/evidence.go`
- `internal/application/service/unifiedqa/domain_evidence_reviewer.go`
- `internal/application/service/unifiedqa/evidence_recovery.go`
- `internal/application/service/unifiedqa/observation_aggregator.go`
- 对应 `*_test.go`

步骤：

1. 复核输出使用严格 JSON；每条事实必须引用输入候选的 opaque ID。
2. `attempt=0` 且 `insufficient` 时才允许补查；`attempt=1` 丢弃所有新补查请求。
3. 补查动作只允许新查询、扩大一次 TopK、候选引用文档、相邻 Chunk、Regex 和配置允许的图查询。
4. 拒绝越权资源、Web、MCP 和写操作。
5. 聚合保留贡献 Agent、冲突和缺失项，Coverage 输出 `complete/partial/insufficient`。
6. 验证：`go test ./internal/application/service/unifiedqa -run "TestDomainEvidence|TestEvidenceRecovery|TestObservation|TestCoverage" -count=1`。

## Task 9：总编排、最终 Prompt 与 SSE

文件：

- `internal/application/service/unifiedqa/service.go`
- `internal/application/service/unifiedqa/prompt_builder.go`
- `internal/application/service/unifiedqa/service_test.go`
- `internal/application/service/session_unified_qa_stream_test.go`

步骤：

1. 严格按 RequestInit、History、Scope、Catalog、Route、Execute、PrepareAnswerContext、GenerateAnswer、Finalize 顺序编排。
2. PG 只记录 `qa_execution_runs` 业务主记录并保存 `langfuse_trace_id`；Langfuse 保留各执行节点、聚合、Coverage 和 Prompt 子 Span。
3. 复用现有流式模型和 SSE 事件类型。
4. 测试调用预算 `2 + A + R`，最少三次、最多六次。
5. 验证：`go test ./internal/application/service/unifiedqa ./internal/application/service -run "TestUnifiedQA|TestPrepareAnswer|Test.*GenerativeCalls|TestSessionUnifiedQAStream" -count=1`。

## Task 10：API、前端兼容和回归

文件：

- `internal/application/service/unified_qa_observation.go`
- `internal/handler/unified_qa_observation.go`
- `internal/router/router.go`
- `frontend/**`、`frontend-admin/**`、`frontend-app/**` 中必要配置说明
- `tests/unified_qa_badcases.json`
- `README.md`

步骤：

1. 提供 Run 查询 API，返回业务执行结果和 `langfuse_trace_id`，并校验所有者或管理员权限。
2. 三套前端不新增引擎或子智能体选择；移除或禁用会误导用户的知识库范围配置并给出说明。
3. 保留静态 bad case 回归数据集；用户点赞点踩作为问题收集入口，暂不建设 bad case 标注表和管理 API。
4. 运行 Go 全量测试、三套前端测试/类型检查/构建和 Docker 数据库迁移验证。

## 最终验证

```powershell
go test ./internal/config -count=1
go test ./internal/application/repository -count=1
go test ./internal/application/service/unifiedqa -count=1
go test ./internal/application/service -count=1
go test ./internal/handler -count=1
go test ./...
```

分别在 `frontend`、`frontend-admin`、`frontend-app` 执行：

```powershell
npm test
npm run type-check
npm run build-only
```

最后在本地 PostgreSQL 版本 2 数据库上执行迁移，断言版本为 3、三张 QA 表存在且 `dirty=false`。
