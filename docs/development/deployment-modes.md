# 三种运行与部署场景

项目固定支持本地开发、服务器开发和服务器正式部署三种场景。三者使用不同环境文件与 `COMPOSE_PROJECT_NAME`，数据库和具名 Volume 默认互不共享。

| 场景 | 应用运行方式 | 登录方式 | 环境文件 | 启动入口 |
|---|---|---|---|---|
| 本地开发 | Go/Auth/Vite 在宿主机，基础设施和 Gateway 在 Docker | 邮箱密码 + Mock SAML | `.env.local` | `scripts/dev-all.sh` |
| 服务器开发 | 当前 Git 源码挂载到容器并在启动时编译 | 邮箱密码 + Mock SAML | `.env.server-dev` | `scripts/server_dev.sh` |
| 正式部署 | 只运行 CI 构建的固定版本镜像 | PingIdentity SAML-only | `.env.production` | `scripts/production_server.sh` |

认证架构在三种模式下保持一致：浏览器调用 Gateway；Gateway 把 `/api/v1/auth/*` 交给 Auth Service，把受保护业务 API 在预验证后交给 Application Backend。管理/RAG 后端不再提供登录路由。

## 1. 本地开发

首次准备：

```bash
cp .env.local.example .env.local
bash ./scripts/dev-all.sh
```

Windows PowerShell：

```powershell
Copy-Item .env.local.example .env.local
& "C:\Program Files\Git\bin\bash.exe" "F:/RocheKAP/scripts/dev-all.sh"
```

默认进程：

- 宿主机：Application Backend `8080`、Auth Service `8081`、Vite `5173`。
- Docker：API Gateway `8088`、PostgreSQL、Redis、DocReader、Milvus、Neo4j、Langfuse、Mock SAML IdP。
- Workday Mock 由后端读取 `config/mock/workday.json`，不是独立容器。

浏览器仍打开：

```text
http://localhost:5173
```

Vite 把 `/api` 代理到 `http://127.0.0.1:8088`。不要把代理改回 App 的 `8080`，否则会绕过 Gateway 且登录路由不存在。

| 服务 | 地址 | 用途 |
|---|---|---|
| Vite UI | `http://localhost:5173` | 日常开发入口 |
| API Gateway | `http://localhost:8088` | 浏览器 API 边界 |
| App health | `http://localhost:8080/health` | 后端调试 |
| Auth health | `http://localhost:8081/health` | 认证服务调试 |
| Mock SAML IdP | `http://127.0.0.1:8091` | 开发环境 SAML IdP |
| Langfuse | `http://localhost:3000` | LLM/Agent 追踪 |
| Milvus WebUI | `http://localhost:9091/webui/` | 向量库调试 |
| Neo4j | `http://localhost:7474` | 图谱调试 |

停止并保留数据：

```bash
bash ./scripts/dev-all.sh stop
```

需要分别启动时：

```bash
make dev-start
make dev-app
make dev-auth
make dev-frontend
```

Windows 没有 `make` 时直接执行：

```powershell
& "C:\Program Files\Git\bin\bash.exe" scripts/dev.sh start
& "C:\Program Files\Git\bin\bash.exe" scripts/dev.sh app
& "C:\Program Files\Git\bin\bash.exe" scripts/dev.sh auth
& "C:\Program Files\Git\bin\bash.exe" scripts/dev.sh frontend
```

## 2. 服务器开发

服务器开发用于多人连接同一台 Linux 服务器验证。App、Auth Service、Frontend 和 DocReader 都由当前工作区源码构建；API Gateway 使用同一个镜像分别启动内网和外网实例。

首次配置：

```bash
cp .env.server-dev.example .env.server-dev
vi .env.server-dev
```

至少修改：

```dotenv
SERVER_DEV_PUBLIC_URL=http://<服务器可访问IP或域名>
```

`server_dev.sh` 自动派生：

- Mock SAML IdP：`http://<server>:8091`
- 内网 Gateway：`http://<server>:8088`
- 外网 Gateway：`http://<server>:8089`
- 两个 SAML SP Entity ID、ACS、Origin 和 redirect allowlist

默认 `GATEWAY_INTERNAL_BIND=127.0.0.1`，用于服务器本机或反向代理；需要让企业内网用户直接访问时，将它改为服务器内网网卡地址。`GATEWAY_EXTERNAL_BIND` 必须由防火墙、负载均衡和网络区策略保护。

启动：

```bash
bash ./scripts/server_dev.sh up
```

浏览器测试入口通常是：

```text
http://<server>:8089
```

App、Auth Service 和 Frontend 的直接调试端口默认只绑定 `127.0.0.1`，远程浏览器不应绕过 Gateway。

常用命令：

```bash
bash ./scripts/server_dev.sh config
bash ./scripts/server_dev.sh status
bash ./scripts/server_dev.sh logs api-gateway-external auth-service-external app
bash ./scripts/server_dev.sh update
bash ./scripts/server_dev.sh down
```

`update` 使用 `git pull --ff-only` 后重新构建容器，不删除 Volume。代码变化后可以重建单个服务：

```bash
docker compose --env-file .env.server-dev -f docker-compose.server-dev.yml up -d --build app
docker compose --env-file .env.server-dev -f docker-compose.server-dev.yml up -d --build auth-service-internal auth-service-external
docker compose --env-file .env.server-dev -f docker-compose.server-dev.yml up -d --build frontend
```

## 3. 服务器正式部署

正式环境不从服务器 Git 工作区编译，CI 构建五种固定版本镜像：

1. `APP_IMAGE`
2. `FRONTEND_IMAGE`
3. `DOCREADER_IMAGE`
4. `AUTH_SERVICE_IMAGE`
5. `API_GATEWAY_IMAGE`

Auth Service 镜像部署两次，Gateway 镜像部署两次，因此认证边界是“两种镜像、四个实例”。

准备：

```bash
cp .env.production.example .env.production
vi .env.production
```

必须完成：

1. 替换全部 `CHANGE_ME`。
2. 五种镜像使用固定版本标签或 digest，禁止 `latest`。
3. 配置 PingIdentity SAML IdP metadata URL。
4. 在 PingIdentity 注册内网/外网两个 SP entity ID 和 ACS URL。
5. 为内外网 Auth Service 分别提供稳定证书和私钥挂载。
6. 配置两个 UI Origin 和登录后 redirect URI 精确白名单。
7. 配置真实 Workday/MuleSoft、数据库、Redis、S3/MinIO、Milvus、Neo4j 和 Langfuse 参数。
8. 仅通过企业 HTTPS Ingress/LB 暴露 Gateway。

生产强制值：

```dotenv
AUTH_PASSWORD_LOGIN_ENABLE=false
AUTH_REGISTRATION_ENABLE=false
AUTH_REGISTRATION_DEV_ROLE_SELECTION=false
OIDC_AUTH_ENABLE=false
SAML_AUTH_ENABLE=true
```

启动和更新：

```bash
bash ./scripts/production_server.sh config
bash ./scripts/production_server.sh up
bash ./scripts/production_server.sh update
```

状态与日志：

```bash
bash ./scripts/production_server.sh status
bash ./scripts/production_server.sh logs api-gateway-external auth-service-external app
bash ./scripts/production_server.sh down
```

生产 Compose 会清除源码 build、源码挂载和 App/Frontend/Auth 的直接端口，只暴露两个 Gateway 端口。普通 `update` 和 `down` 不删除 PostgreSQL、对象存储、Milvus、Neo4j 或 Langfuse 数据。

## 4. 开发账号

本地和服务器开发 Mock SAML 账号：

| 用户名 | 密码 | Assertion 邮箱 |
|---|---|---|
| `admin` | `Admin123!` | `admin@rochekap.local` |
| `developer001` ... `developer100` | `Dev12345!` | `developer001@rochekap.local` ... `developer100@rochekap.local` |

Mock SAML IdP 只证明身份。开发环境在首次自动建档时将 `developer001` 至 `developer010` 引导为真正的系统管理员，其他开发账号为普通用户。此后系统角色以平台数据库为准，在界面调整角色后不会被后续 SAML 登录覆盖。

## 5. 数据安全

以下命令保留具名 Volume：

```bash
docker compose down
```

以下命令会永久删除数据，不要用于更新或普通停止：

```bash
docker compose down -v
docker volume prune
```

正式更新前至少备份 PostgreSQL、S3/MinIO、Milvus、Neo4j，以及 Langfuse 使用的 PostgreSQL 和 ClickHouse。
