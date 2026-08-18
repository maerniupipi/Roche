# 单一 Frontend 服务承载两套前端设计

## 背景

仓库包含 `frontend` 与 `frontend-default` 两套前端源码。它们是两个独立构建项目，但生产部署边界应为一个 `frontend` 镜像和一个 `frontend` 容器：

- `/` 提供 `frontend` 构建产物。
- `/default/` 提供 `frontend-default` 构建产物。
- `/api/` 与 `/files` 继续由同一 Nginx 代理到后端 `app`。

提交 `64f56628` 将 `frontend-default` 建模成独立 Compose 服务，并在生产 Compose 中强制要求 `FRONTEND_DEFAULT_IMAGE`。现有 CI 只构建一个前端镜像，因此部署在 Compose 插值阶段失败。

## 目标

1. 保留两个前端源码目录和各自的构建配置。
2. 构建一个同时包含两套静态产物的 `frontend` 镜像。
3. 生产与 Compose 部署只运行一个 `frontend` 服务。
4. 保持原入口 `/`、新版入口 `/default/` 和后端入口 `/api/` 稳定。
5. 本地仍允许分别运行两个 Vite 开发服务器。

## 非目标

- 不合并两套 Vue 源码或路由。
- 不修改后端接口和跨域配置。
- 不改变 `app`、`docreader` 等其他服务的部署模型。

## 构建与镜像布局

前端镜像使用两个独立构建阶段：

1. 在 `frontend` 目录安装依赖并构建旧前端。
2. 在 `frontend-default` 目录安装依赖并构建新版前端。
3. 在最终 Nginx 镜像中按以下结构复制构建结果：

```text
/usr/share/nginx/html/          frontend/dist
/usr/share/nginx/html/default/  frontend-default/dist
```

CI 继续只生成一个固定标签镜像：

```text
rochekap/frontend:${CI_COMMIT_SHORT_SHA}
```

Docker 构建上下文必须覆盖仓库根目录，使同一个 Dockerfile 能读取两个前端目录。

部署主机可能仍使用 classic Docker builder。该构建器只读取仓库根目录的
`.dockerignore`，不会使用 `frontend/Dockerfile.dockerignore`。因此根
`.dockerignore` 必须放行 `frontend/` 与 `frontend-default/` 的源码，同时继续
排除两个目录下的 `node_modules/` 和 `dist/`。Dockerfile 专属 ignore 文件只作为
BuildKit 的上下文优化，不能作为构建正确性的前提。

## Nginx 路由

Nginx 在单一容器中处理三个边界：

- `location /` 从 `/usr/share/nginx/html` 提供旧前端，并回退到 `/index.html`。
- `location /default/` 从 `/usr/share/nginx/html/default` 提供新版前端，并回退到 `/default/index.html`。
- `location /api/` 与 `/files` 保持现有后端代理行为。

`/default/` 不再反向代理到 `frontend-default` 容器。删除仅用于跨容器代理的 `FRONTEND_DEFAULT_HOST`、`FRONTEND_DEFAULT_PORT` 和 `FRONTEND_DEFAULT_SCHEME`。保留 `FRONTEND_DEFAULT_ENABLED`，由同一 Nginx 容器直接控制 `/default/` 静态入口；设为 `false` 时该入口返回 404。

## API 路径

`frontend-default` 的 Vite `base` 继续为 `/default/`，仅用于静态资源和 SPA 路由。API 基础地址不得继承该路径，浏览器请求必须保持为根路径 `/api/v1/...`。

本地开发时：

```text
/api/v1/... -> Vite proxy -> VITE_DEV_PROXY_TARGET
```

生产部署时：

```text
/api/v1/... -> frontend Nginx -> app
```

## Compose 与脚本

- 从 `docker-compose.server-dev.yml` 和 `docker-compose.production.yml` 删除独立的 `frontend-default` 服务。
- `frontend` 源码镜像同时构建两个挂载的前端目录，并将产物复制到上述两个目标目录。
- 删除 `frontend` 对 `frontend-default` 的 `depends_on`。
- 删除 `frontend-default-node-modules` 等仅服务于独立容器的 Compose 资源。
- `frontend` 服务继续接收 `FRONTEND_DEFAULT_ENABLED`，保持新版入口的启停能力。
- 生产部署继续只使用 `FRONTEND_IMAGE`；不新增 `FRONTEND_DEFAULT_IMAGE`。
- `ci-update` 继续更新 `app`、`docreader`、`frontend` 三个核心镜像服务。
- 清理 Makefile 和镜像脚本中“独立 frontend-default 镜像”的目标；本地 `dev-frontend-default` Vite 命令保留。

## 错误处理与回滚

- 任意一个前端构建失败时，组合镜像构建整体失败，不产生不完整镜像。
- Nginx 配置检查失败时阻止镜像交付。
- 回滚只需要回退一个 `frontend` 镜像标签，两套前端保持同版本发布。

## 验证

实施后必须验证：

1. Docker Compose 在仅设置 `FRONTEND_IMAGE` 时可以完成配置插值。
2. 组合镜像中同时存在 `/index.html` 与 `/default/index.html`。
3. `/` 返回旧前端 HTML。
4. `/default/` 及其深层 SPA 路由返回新版前端 HTML。
5. `/default/assets/...` 返回新版静态资源，而不是旧前端资源。
6. 新版前端发出的登录请求为 `/api/v1/auth/login`，不包含 `/default/api`。
7. `/api/` 请求仍能由 Nginx 转发到 `app`。
8. CI 只构建并部署一个 `frontend:${CI_COMMIT_SHORT_SHA}` 镜像。
9. `FRONTEND_DEFAULT_ENABLED=false` 时 `/default/` 返回 404，设为 `true` 时正常提供新版前端。
10. 关闭 BuildKit 后执行同一条前端镜像构建命令时，classic builder 能读取两套前端的 `package.json` 和源码，不再在 `COPY frontend/package.json` 阶段失败。
