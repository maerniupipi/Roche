# 开发环境企业集成 Mock

本地开发和服务器开发同时支持三种登录和组织数据测试方式：

1. 邮箱注册与密码登录。
2. Mock SAML SSO 登录，走真实的 SAML 2.0 SP-initiated 流程。
3. 文件型 Workday Mock，同步企业组织树和员工归属。

这些 Mock 只属于开发环境。服务器正式部署使用 PingIdentity SAML 与 HTTP Workday
Provider，不启动 Mock SAML IdP，也不读取 Mock 文件。

## 1. 本地开发启动

首次使用：

```bash
cp .env.local.example .env.local
```

启动完整源码开发环境：

```bash
bash ./scripts/dev-all.sh
```

该命令默认启动 Mock SAML IdP、PostgreSQL、Redis、DocReader、Milvus、Neo4j、Langfuse
和 API Gateway，并在宿主机运行 Go 后端、Auth Service 和 Vite 前端。

服务器开发使用：

```bash
cp .env.server-dev.example .env.server-dev
# 修改 SERVER_DEV_PUBLIC_URL
bash ./scripts/server_dev.sh up
```

服务器脚本根据 `SERVER_DEV_PUBLIC_URL` 派生 Mock IdP 公网地址、内外网两个
SP Entity ID、ACS URL 和重定向白名单。开发 IdP 会同时注册两个 SP。

## 2. 注册入口

本地浏览器打开 `http://localhost:5173`；服务器开发通常打开
`http://<server>:8089`。登录表单下方应显示“立即注册”。开发注册页面允许选择：

- `viewer`：普通用户；
- `system_admin`：系统管理员。

入口由以下变量控制：

```dotenv
AUTH_REGISTRATION_ENABLE=true
AUTH_REGISTRATION_DEFAULT_ROLE=viewer
AUTH_REGISTRATION_DEV_ROLE_SELECTION=true
AUTH_PASSWORD_LOGIN_ENABLE=true
```

如果修改认证环境变量，必须重启 Auth Service。前端会通过 Gateway 调用
`GET /api/v1/auth/registration/config` 决定是否显示入口。

## 3. Mock SSO

本地 Mock SAML IdP 地址：`http://127.0.0.1:8091`。服务器开发地址为
`http://<服务器地址>:8091`，metadata 位于 `/metadata`。

| 用户名 | 密码 | Assertion 邮箱 | 用途 |
|---|---|---|---|
| `admin` | `Admin123!` | `admin@rochekap.local` | SAML 身份映射测试 |
| `developer001` ... `developer100` | `Dev12345!` | 对应的 `developerNNN@rochekap.local` | 多人开发和不同角色测试 |

点击登录页的企业 SAML 登录按钮后，浏览器经 Gateway 和 Auth Service 跳转到
Mock IdP。SAML 首次登录会自动创建或关联本地 `users`，并写入
`sso_identities`。Mock IdP 只识别身份，不直接授予知识权限。

首次通过 SAML 自动创建时，`developer001` 至 `developer010` 会由开发环境白名单写成真正的系统管理员，
`developer011` 至 `developer100` 为普通用户。系统管理员可在用户管理界面继续调整平台角色；后续 SAML
登录只更新身份映射和最后登录时间，不会覆盖平台中的角色。
开发账号数量和统一密码可通过 `MOCK_SAML_DEVELOPER_COUNT`、
`MOCK_SAML_DEVELOPER_PASSWORD` 调整；首次建档管理员名单由
`SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS` 控制。

配置文件：

- 本地：`.env.local`、`docker-compose.local.yml`
- 服务器开发：`.env.server-dev`、`docker-compose.server-dev.yml`
- Mock IdP 实现：`cmd/mock-saml-idp/main.go`

仅启动或关闭 Mock SAML IdP：

```bash
docker compose --env-file .env.local -f docker-compose.local.yml --profile mock-saml up -d mock-saml-idp
docker compose --env-file .env.local -f docker-compose.local.yml --profile mock-saml stop mock-saml-idp
```

## 4. Workday Mock

测试数据位于 `config/mock/workday.json`，包含 RD China、Finance、Informatics
三级组织样例，以及 `admin`、`viewer` 和 `developer001` 至 `developer100` 对应的员工投影。本地默认配置：

```dotenv
WORKDAY_ENABLE=true
WORKDAY_PROVIDER=mock
WORKDAY_CONNECTION_KEY=local-mock
WORKDAY_MOCK_FILE=config/mock/workday.json
```

使用系统管理员的平台 JWT 触发同步：

```http
POST /api/v1/system/admin/integrations/workday/sync
Authorization: Bearer <platform-jwt>
Content-Type: application/json

{"mode":"full"}
```

同步是异步任务。使用返回的 `run_id` 查询结果：

```http
GET /api/v1/system/admin/integrations/workday/runs/{run_id}
Authorization: Bearer <platform-jwt>
```

成功后数据写入：

- `external_org_units`、`external_workers`：Workday 原始投影；
- `org_units`：系统规范组织树；
- `user_org_memberships`：已通过企业邮箱匹配到本地用户的员工归属。

Workday 组织关系本身不自动授予知识权限。管理员仍需通过知识库列表外层的
“访问权限”弹窗，为知识库、目录或文档创建 `knowledge_resource_grants`。

## 5. 自动检查

前后端和 Mock SAML IdP 启动后运行：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify-local-mocks.ps1
```

脚本验证注册开关、密码登录开关、开发角色选择、Mock IdP 健康状态、Auth Service
SAML 配置和 Gateway SAML 授权跳转。

## 6. 常见问题

注册入口或 SSO 按钮消失时，先检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8088/api/v1/auth/registration/config
Invoke-RestMethod http://127.0.0.1:8088/api/v1/auth/saml/config
Invoke-RestMethod http://127.0.0.1:8088/api/v1/auth/saml/url?redirect_uri=http://localhost:5173/
Invoke-RestMethod http://127.0.0.1:8091/healthz
```

如果前两个接口连接失败，检查 Gateway 和 Auth Service；如果返回 `enabled=false`，
说明 Auth Service 没有加载最新 `.env.local`。只刷新浏览器不够，必须重启认证进程。
