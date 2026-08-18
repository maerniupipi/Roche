# `frontend-app` 组合部署设计

## 背景

远程 `main` 已新增独立前端工程 `frontend-app`。该工程当前仍以根路径 `/` 构建，尚未进入现有组合前端镜像。生产环境已经通过一个 `frontend` 镜像同时提供：

- `/`：`frontend`
- `/default/`：`frontend-default`
- `/admin/`：`frontend-admin`

本次将 `frontend-app` 按与 `frontend-admin` 相同的模式接入，线上入口为 `/app/`。

## 目标与非目标

目标：

1. `frontend-app` 独立安装依赖并构建，生产静态资源 base 为 `/app/`。
2. 四套前端产物进入同一个 `frontend` 镜像，由同一个 Nginx 容器提供服务。
3. 新增 `/app -> /app/`、`/app/assets/` 和 `/app/` SPA fallback。
4. 新增默认开启的 `FRONTEND_APP_ENABLED` 开关；关闭时 `/app` 相关入口返回 404。
5. 保持 `/`、`/default/`、`/admin/`、`/api/` 和 `/files` 的现有行为不变。

非目标：

- 不修改 `frontend-app` 的业务功能。
- 不新增独立 `frontend-app` 容器、Compose 服务或 `FRONTEND_APP_IMAGE`。
- 不修改后端接口、鉴权、跨域或远程服务配置。

## 构建与镜像布局

`frontend/Dockerfile` 增加 `frontend-app-builder`，继续使用仓库根目录作为 Docker build context。最终镜像布局为：

```text
/usr/share/nginx/html/          frontend/dist
/usr/share/nginx/html/default/  frontend-default/dist
/usr/share/nginx/html/admin/    frontend-admin/dist
/usr/share/nginx/html/app/      frontend-app/dist
```

根 `.dockerignore` 和 `frontend/Dockerfile.dockerignore` 必须允许 `frontend-app` 源码进入构建上下文，同时排除其 `node_modules` 与 `dist`。任意一套前端构建失败时，整个组合镜像构建失败。

## 路由与运行时开关

`frontend-app/vite.config.ts` 的生产 `base` 调整为 `/app/`，Vue Router 继续通过 `import.meta.env.BASE_URL` 获取路由基路径。业务 API 仍请求根路径 `/api/...`，不能变成 `/app/api/...`。

Nginx 增加：

- `/app`：301 跳转到 `/app/`。
- `/app/assets/`：提供带 hash 的静态资源，并沿用现有缓存和安全响应头。
- `/app/`：深层路由 fallback 到 `/app/index.html`。

`frontend/docker-entrypoint.sh` 注入 `FRONTEND_APP_ENABLED`，默认值为 `true`。该值为 `false` 时，`/app`、`/app/` 和 `/app/assets/...` 均返回 404，不影响其他三个前端入口。

## Compose、本地开发与 CI

- Compose 继续只有一个 `frontend` 服务和一个 `FRONTEND_IMAGE`，仅为该服务增加 `FRONTEND_APP_ENABLED=${FRONTEND_APP_ENABLED:-true}`。
- 服务器源码开发模式为 `frontend-app` 增加独立源码挂载和 `node_modules` 命名卷，由现有单个前端容器构建四套产物。
- 本地开发增加 `make dev-frontend-app` 与对应脚本入口，默认端口使用 `5176`，仍允许 `VITE_DEV_PORT` 覆盖。
- 通用静态构建脚本接受 `frontend-app`，并增加对称的 app wrapper。
- GitLab CI 继续使用 `docker build -f frontend/Dockerfile ... .` 构建和发布单一镜像，不增加新镜像变量或独立任务。

## 测试与验收

1. 配置测试先证明当前缺少 `frontend-app` builder、产物复制、Nginx 路由、entrypoint 变量和 Compose 开关，再通过最小配置修改使其转绿。
2. `frontend-app` 的生产构建成功，生成的 HTML/资源地址以 `/app/` 开头。
3. Nginx 配置验证 `/app` 301、`/app/` 及深层 SPA 路由可访问，并验证关闭开关后的 404。
4. Compose 配置校验通过，服务列表仍只有一个前端服务，不出现 `FRONTEND_APP_IMAGE`。
5. 原有 `/`、`/default/`、`/admin/` 路由和相关测试保持通过。

## 回滚

本次仍以单一 `frontend` 镜像整体发布。发生异常时回滚一个镜像标签即可恢复四套前端的一致版本；也可先将 `FRONTEND_APP_ENABLED=false` 临时关闭 `/app/`，而不影响其他入口。
