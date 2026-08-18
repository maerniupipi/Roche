# 配置参考

系统有三份环境模板，禁止跨场景复用：

- 本地：`.env.local.example` -> `.env.local`
- 服务器开发：`.env.server-dev.example` -> `.env.server-dev`
- 服务器正式部署：`.env.production.example` -> `.env.production`

## 0. 三种模式的地址规则

| 配置 | 本地源码开发（Go 在宿主机） | 服务器开发/正式部署（Go 在容器） |
|---|---|---|
| PostgreSQL | `localhost:5432` | `postgres:5432` |
| Redis | `localhost:6379` | `redis:6379` |
| DocReader | `localhost:50051` | `docreader:50051` |
| Milvus | `localhost:19530` | `milvus:19530` |
| Neo4j | `bolt://localhost:7687` | `bolt://neo4j:7687` |
| Langfuse | `http://localhost:3000` | `http://langfuse-web:3000` |
| 业务 MinIO | 可选 `localhost:9000` | `minio:9000` |

`.env.local.example` 中部分值使用 Compose 服务名，是为了让 Docker
基础设施配置可直接复用；`scripts/dev.sh app` 启动宿主机 Go 进程前会自动把这些
地址改成 `localhost`。不要把本地网页里填写的 `localhost` 地址直接复制到服务器
容器配置。

## 1. 应用和数据库

| 变量 | 作用 |
|---|---|
| `GIN_MODE` | Gin 模式；`release` 时不挂载 Swagger UI |
| `APP_PORT` | 服务器后端宿主机端口 |
| `FRONTEND_PORT` | 服务器前端端口 |
| `DB_DRIVER` | 标准部署使用 `postgres` |
| `DB_HOST/PORT/USER/PASSWORD/NAME` | PostgreSQL 连接 |
| `AUTO_RECOVER_DIRTY` | 是否自动尝试恢复 dirty migration |
| `JWT_SECRET` | 平台 JWT 签名密钥 |
| `SYSTEM_AES_KEY` | 平台敏感配置加密密钥，要求 32 字节 |

## 2. Redis 和任务

| 变量 | 作用 |
|---|---|
| `STREAM_MANAGER_TYPE` | 流管理器，标准为 `redis` |
| `REDIS_ADDR/PASSWORD/DB/PREFIX` | Redis 连接和 key 前缀 |
| `ROCHE_KAP_ASYNQ_CONCURRENCY` | Worker 并发 |
| `CONCURRENCY_POOL_SIZE` | 部分业务并发池大小 |

## 3. 存储

| 变量 | 作用 |
|---|---|
| `STORAGE_TYPE` | `local`、`minio`、`s3` 等 |
| `LOCAL_STORAGE_BASE_DIR` | 本地存储根目录 |
| `MINIO_ENDPOINT` | MinIO API 地址 |
| `MINIO_ACCESS_KEY_ID` | MinIO 用户 |
| `MINIO_SECRET_ACCESS_KEY` | MinIO 密码 |
| `MINIO_BUCKET_NAME` | Bucket |
| `MAX_FILE_SIZE_MB` | 上传大小上限 |

知识库保存存储 provider 选择，凭据来自平台存储配置或环境变量，不应把明文凭据写入
知识库记录。

## 4. 检索

| 变量 | 作用 |
|---|---|
| `RETRIEVE_DRIVER` | 标准为 `milvus` |
| `MILVUS_ADDRESS` | Milvus gRPC 地址 |
| `MILVUS_COLLECTION` | Collection 基础名 |
| `MILVUS_METRIC_TYPE` | `IP` 等距离度量 |

模型维度变化可能创建不同实际 Collection。不要在不清楚索引命名策略时直接删除
Milvus Collection。

## 5. 文档解析

| 变量 | 作用 |
|---|---|
| `DOCREADER_ADDR` | DocReader gRPC 地址 |
| `DOCREADER_TRANSPORT` | `grpc` |
| `GRPC_TLS_*` | gRPC TLS 和双向认证 |
| `GRPC_AUTH_TOKEN` | DocReader 服务认证 Token |

MinerU endpoint、API key、OCR、表格和公式选项保存在平台
`platform_runtime_configs.parser_engine_config`，由系统管理员管理。

## 6. 图谱

| 变量 | 作用 |
|---|---|
| `NEO4J_ENABLE` | 是否初始化 Neo4j |
| `NEO4J_URI` | Bolt URI |
| `NEO4J_USERNAME/PASSWORD` | 认证 |

仅连接 Neo4j 不会自动给所有知识库启用图谱；知识库还需要启用图谱索引策略。

## 7. 认证

| 变量 | 作用 |
|---|---|
| `AUTH_PASSWORD_LOGIN_ENABLE` | 邮箱密码登录；生产 SAML-only 必须为 `false` |
| `AUTH_REGISTRATION_ENABLE` | 是否开放邮箱注册 |
| `AUTH_REGISTRATION_DEFAULT_ROLE` | 默认角色，生产应为 `viewer` |
| `AUTH_REGISTRATION_DEV_ROLE_SELECTION` | 开发注册页角色选择，生产必须关闭 |
| `AUTH_SERVICE_INTERNAL_SECRET` | 当前 Auth Service 实例接收的 Gateway 共享密钥；本地单实例开发使用 |
| `AUTH_INTERNAL_SERVICE_SECRET` | 内网 Gateway/Auth Service 独立共享密钥；生产至少 48 字符 |
| `AUTH_EXTERNAL_SERVICE_SECRET` | 外网 Gateway/Auth Service 独立共享密钥；生产至少 48 字符且不得与内网相同 |
| `AUTH_REFRESH_COOKIE_SECURE` | 强制 Refresh Token Cookie 使用 `Secure`；生产必须为 `true` |
| `AUTH_ALLOWED_ORIGINS` | 当前 Auth Service 实例允许的 UI Origin 精确列表 |
| `AUTH_ALLOWED_REDIRECT_URIS` | 当前 Auth Service 实例允许的登录完成重定向 URI 精确列表 |
| `OIDC_AUTH_ENABLE` | 当前三种部署模式均固定为 `false`；不作为开发 Mock 入口 |
| `SAML_AUTH_ENABLE` | 是否启用 SAML 2.0；本地/服务器开发和生产均为 `true` |
| `SAML_AUTH_IDP_METADATA_URL` | 开发指向 Mock SAML metadata；生产指向 PingIdentity HTTPS metadata |
| `SAML_AUTH_AUTO_PROVISION` | SAML 首次登录时自动创建本地用户 |
| `SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS` | 仅开发环境使用的首次建档系统管理员邮箱白名单；生产必须为空 |
| `SAML_AUTH_SP_ENTITY_ID` | 当前区域 SP entity ID |
| `SAML_AUTH_ACS_URL` | 当前区域 ACS URL |
| `SAML_AUTH_SP_CERT_FILE/KEY_FILE` | 当前区域稳定证书和私钥容器路径 |
| `SAML_AUTH_ALLOW_EPHEMERAL_CERT` | 是否允许启动时生成临时 SP 证书；仅限开发，生产必须为 `false` |
| `SAML_AUTH_ALLOW_IDP_INITIATED` | 是否允许 IdP-initiated，默认和生产建议为 `false` |
| `MOCK_SAML_DEVELOPER_COUNT` | Mock SAML 批量开发账号数量，默认 `100` |

内网和外网 Auth Service 使用同一镜像，但必须分别配置 SP entity ID、ACS URL、证书、私钥和 redirect allowlist。

## 8. Workday

| 变量 | 作用 |
|---|---|
| `WORKDAY_ENABLE` | 启用同步 |
| `WORKDAY_PROVIDER` | `mock` 或 `http` |
| `WORKDAY_CONNECTION_KEY` | 连接配置稳定键 |
| `WORKDAY_MOCK_FILE` | 本地 fixture |
| `WORKDAY_BASE_URL` | HTTP adapter 基础地址 |
| `WORKDAY_ORG_UNITS_PATH` | 组织接口 |
| `WORKDAY_WORKERS_PATH` | 员工接口 |
| `WORKDAY_TOKEN_URL` | OAuth2 token endpoint |
| `WORKDAY_CLIENT_ID/SECRET/SCOPE` | 机器凭据 |

Workday 凭据只存在环境/Secret Manager，不写入投影表。

## 9. Langfuse

| 变量 | 作用 |
|---|---|
| `LANGFUSE_ENABLED` | 应用是否发送追踪 |
| `LANGFUSE_HOST` | Langfuse 地址 |
| `LANGFUSE_PUBLIC_KEY/SECRET_KEY` | 项目 API key |
| `LANGFUSE_RELEASE` | 发布版本 |
| `LANGFUSE_ENVIRONMENT` | 环境名 |
| `LANGFUSE_SAMPLE_RATE` | 采样率 |

启动 Langfuse 容器与启用应用追踪是两件事。容器运行但没有有效项目 key 时，应用不会
产生完整 trace。

## 10. 安全配置

- 所有示例密码都必须替换。
- `.env.local`、`.env.server-dev` 和 `.env.production` 不进入 Git。
- 本地开发和服务器开发启用 Mock SAML、Workday Mock 与注册角色选择；不依赖客户 PingIdentity 配置。
- 正式部署必须关闭公开注册和角色选择，禁止 `WORKDAY_PROVIDER=mock`，
  并且只使用带固定版本号的应用镜像。
- 生产凭据使用 Secret Manager 注入。
- `SSRF_WHITELIST` 只添加必要的内部主机。
- 不在日志打印 DSN、Authorization、private key 和完整事件 payload。
