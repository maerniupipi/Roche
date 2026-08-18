# `frontend-admin` 独立前端与组合部署设计

## 背景

仓库当前包含两套彼此独立的 Vue 前端源码：

- `frontend`：生产入口 `/`。
- `frontend-default`：生产入口 `/default/`。

两套源码在构建阶段独立安装依赖、独立生成静态资源，但交付时合并进一个 `frontend` 镜像，并由一个 Nginx 容器提供页面和后端代理。此次需要新增第三套独立源码 `frontend-admin`，其初始内容来自最新远程 `main` 的 `frontend`，部署方式与 `frontend-default` 一致。

本次复制基线为远程提交 `baa19182f2cb733aa6e3cf2116a80bceb98463f0`。因此 `frontend-admin` 初始版本自然包含该提交之前乐园园对 MCP、WebSearch 和新手指引的三次删除调整。复制完成后，`frontend` 与 `frontend-admin` 是两个独立项目，后续修改不自动同步。

## 目标

1. 将 `frontend` 的最新受 Git 管理源码完整复制为 `frontend-admin`，不复制 `node_modules`、`dist` 等本地生成物。
2. 保留三个独立的前端工程及依赖边界。
3. 构建一个同时包含三套静态产物的 `frontend` 镜像。
4. 生产和服务器源码部署仍只运行一个 Compose 服务 `frontend`。
5. 保持现有 `/`、`/default/`、`/api/` 和 `/files` 行为不变，新增 `/admin/`。
6. 支持独立启动 `frontend-admin` 的 Vite 开发服务器，默认端口为 `5175`。

## 非目标

- 不合并或抽取三套 Vue 源码中的公共组件。
- 不让 `frontend-admin` 与 `frontend` 在复制后自动保持同步。
- 不增加 `frontend-admin` 独立容器、Compose 服务或生产镜像变量。
- 不修改后端接口、鉴权、跨域或远程服务配置。
- 不改变 `/` 与 `/default/` 当前展示的前端。

## 源码复制与项目身份

`frontend-admin` 从基线提交中的 `frontend` 物理复制，包含源码、静态资源、测试、锁文件、工作区文件及项目内构建配置。复制后进行以下最小身份调整：

- `package.json` 的包名调整为 admin 专用名称，避免三套工程在日志和工具中难以区分。
- Vite 的生产 `base` 从 `/` 调整为 `/admin/`。
- Vite 默认开发端口调整为 `5175`，仍允许 `VITE_DEV_PORT` 覆盖。
- 开发代理继续读取 `VITE_DEV_PROXY_TARGET` 或 `FRONTEND_BACKEND_URL`，默认回退到本地后端。

admin 页面发起的后端请求必须继续使用根路径 `/api/v1/...` 和 `/files`，不能生成 `/admin/api/...`。`/admin/` 只用于静态资源地址和 SPA 前端路由。

## 构建与镜像布局

`frontend/Dockerfile` 增加第三个独立 builder：

1. `frontend-builder` 构建 `frontend`。
2. `frontend-default-builder` 构建 `frontend-default`。
3. `frontend-admin-builder` 构建 `frontend-admin`。
4. 最终 Nginx 阶段复制三份产物。

最终镜像布局如下：

```text
/usr/share/nginx/html/          frontend/dist
/usr/share/nginx/html/default/  frontend-default/dist
/usr/share/nginx/html/admin/    frontend-admin/dist
```

任何一套前端构建失败，整个组合镜像构建失败，不能交付缺少某个入口的不完整镜像。CI 继续只构建并部署：

```text
rochekap/frontend:${CI_COMMIT_SHORT_SHA}
```

不得新增 `FRONTEND_ADMIN_IMAGE`，也不得新增独立 admin 镜像构建任务。

## Docker 构建上下文兼容

CI 使用仓库根目录作为 Docker build context。根 `.dockerignore` 必须允许 `frontend`、`frontend-default`、`frontend-admin` 三套源码进入上下文，同时排除各目录的 `node_modules/` 与 `dist/`。

`frontend/Dockerfile.dockerignore` 同样加入 `frontend-admin/**`，作为 BuildKit 构建上下文优化。部署主机可能使用 classic Docker builder，因此构建正确性不能依赖 Dockerfile 专属 ignore 文件；根 `.dockerignore` 才是兼容性基线。

## Nginx 路由

单个 Nginx 容器处理以下边界：

| 请求路径 | 来源或行为 |
| --- | --- |
| `/` | `frontend`，SPA fallback 到 `/index.html` |
| `/default/` | `frontend-default`，SPA fallback 到 `/default/index.html` |
| `/admin/` | `frontend-admin`，SPA fallback 到 `/admin/index.html` |
| `/api/`、`/files` | 按现有规则代理到后端 `app` |

admin 路由细节：

- `/admin` 返回 `301 /admin/`，使相对路径和 Vite base 行为一致。
- `/admin/assets/` 从 `/usr/share/nginx/html/admin/assets/` 提供带 hash 的静态资源，并保持现有长期缓存和安全响应头策略。
- `/admin/` 及其深层路由回退到 `/admin/index.html`，保证浏览器直接访问 SPA 路由时正常加载。
- admin 路由必须排列在通用 `location /` 的语义覆盖范围之前或使用足够明确的匹配规则，避免回退到根前端。

新增 `FRONTEND_ADMIN_ENABLED` 运行时开关，默认值为 `true`。设为 `false` 时 `/admin`、`/admin/` 和 `/admin/assets/...` 均返回 404；该开关由现有 `frontend/docker-entrypoint.sh` 注入 Nginx 模板。现有 `FRONTEND_DEFAULT_ENABLED` 行为保持不变。

## Compose 与服务器源码部署

生产 Compose 不新增服务，只继续覆盖 `frontend` 使用固定的 `FRONTEND_IMAGE`。服务器源码部署中的一个 `frontend` 服务增加：

- `./frontend-admin:/src/frontend-admin` 源码挂载。
- 独立的 `frontend-admin-node-modules:/src/frontend-admin/node_modules` 命名卷。
- `FRONTEND_ADMIN_ENABLED=${FRONTEND_ADMIN_ENABLED:-true}`。

`docker/Dockerfile.frontend.source` 依次构建三个挂载目录，将结果暂存到 `/tmp`，再分别复制到根目录、`default/` 和 `admin/`。三个项目使用独立 `node_modules`，构建结果不得写回 Git 工作树。

Compose 服务列表中仍只能出现一个前端服务名 `frontend`，不能出现 `frontend-default` 或 `frontend-admin` 服务。

## 本地开发与静态构建命令

本地开发命令扩展为：

```text
make dev-frontend          -> frontend，默认 5173
make dev-frontend-default  -> frontend-default，默认 5174
make dev-frontend-admin    -> frontend-admin，默认 5175
```

`scripts/dev.sh` 增加 `frontend-admin` 分支，沿用现有安装依赖、读取本地环境变量和输出代理目标的逻辑。Makefile 的 `.PHONY`、帮助信息和目标同步更新。

通用静态构建脚本 `scripts/build_frontend_dist.sh` 接受 `frontend-admin`。增加与 default wrapper 对称的 `scripts/build_frontend_admin_dist.sh`，方便显式构建 admin 静态产物。现有命令和默认目标保持兼容。

## CI 与发布行为

`.gitlab-ci.yml` 的前端构建命令继续使用：

```text
docker build -f frontend/Dockerfile -t rochekap/frontend:${CI_COMMIT_SHORT_SHA} .
```

Dockerfile 增加第三个构建阶段后，这条命令会自动包含 admin。发布时仍只传递 `PRODUCTION_FRONTEND_IMAGE`，`ci-update` 仍更新一个 `frontend` 容器。三套页面随同一镜像标签一起发布和回滚，避免版本错配。

## 错误处理与回滚

- 任一前端依赖安装、测试或构建失败时，阻止镜像交付。
- Nginx 配置中存在未替换变量或语法错误时，阻止容器启动。
- `/admin/` 可通过 `FRONTEND_ADMIN_ENABLED=false` 临时关闭，不影响 `/`、`/default/` 和后端代理。
- 版本回滚只需回滚一个 `frontend` 镜像标签，三套页面保持同一发布版本。

## 验收标准

1. `frontend-admin` 保持最新 `frontend` 的源码基线，并能以 `/admin/`、默认端口 `5175` 完成生产构建。
2. 使用 CI 同款根上下文命令构建组合镜像，并冒烟验证 `/admin/`、深层 SPA 路由及关闭开关后的 404。
3. Compose 配置通过校验，仍只有一个 `frontend` 服务、一个 `FRONTEND_IMAGE`，不引入 `FRONTEND_ADMIN_IMAGE`。
4. `/`、`/default/`、`/api/` 和 `/files` 的现有路径与代理行为保持不变。
