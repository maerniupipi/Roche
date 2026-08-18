# Frontend App Combined Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 `frontend-app` 作为 `/app/` 入口加入单一 `frontend` 镜像，并保持 API、登录跳转、现有三个前端入口及部署变量兼容。

**Architecture:** `frontend-app` 继续是独立 Vue/Vite 工程，使用 `/app/` 作为静态资源和 Router base；API 始终走同源根路径 `/api/`。现有多阶段 Dockerfile 增加第四个 builder，最终 Nginx 镜像直接服务 `/app/`，并由 `FRONTEND_APP_ENABLED` 控制入口是否启用。

**Tech Stack:** Vue 3、Vite 7、Node test runner、Docker multi-stage build、Nginx、Docker Compose、Bash、Make。

## Global Constraints

- 线上入口固定为 `/app/`，裸路径 `/app` 必须 301 到 `/app/`。
- 不修改 `frontend-app` 业务功能；仅修改部署身份、API/登录基路径及构建部署配置。
- 继续只有一个 `frontend` 服务、一个 `FRONTEND_IMAGE` 和一个发布任务；禁止新增 `FRONTEND_APP_IMAGE`。
- `FRONTEND_APP_ENABLED` 默认 `true`；为 `false` 时 `/app`、`/app/` 和 `/app/assets/...` 返回 404。
- `/`、`/default/`、`/admin/`、`/api/` 和 `/files` 的行为保持不变。
- 本地 Vite 默认端口为 `5176`，允许 `VITE_DEV_PORT` 覆盖。

---

### Task 1: `frontend-app` 子路径与共享 API 行为

**Files:**
- Create: `frontend-app/src/utils/api-base.test.mjs`
- Create: `frontend-app/src/utils/auth-redirect.ts`
- Create: `frontend-app/src/utils/auth-redirect.test.mjs`
- Modify: `frontend-app/src/utils/api-base.ts`
- Modify: `frontend-app/src/utils/request.ts:1-90`
- Modify: `frontend-app/vite.config.ts:53-92`

**Interfaces:**
- Consumes: Vite 的 `import.meta.env.BASE_URL`，生产值为 `/app/`。
- Produces: `getApiBaseUrl(): string` 固定返回 `/`；`getLoginPath(baseUrl: string): string` 将 `/app/` 转成 `/app/login`。

- [ ] **Step 1: 写 API 根路径失败测试**

```js
import assert from 'node:assert/strict'
import test from 'node:test'
import { getApiBaseUrl } from './api-base.ts'

test('frontend-app keeps API requests at the origin root', () => {
  assert.equal(getApiBaseUrl(), '/')
})
```

- [ ] **Step 2: 运行测试并确认因当前函数依赖 Vite base 而失败**

Run: `npm test -- src/utils/api-base.test.mjs`

Expected: FAIL；当前实现读取 `import.meta.env.BASE_URL`，不能证明 `/app/` 部署时 API 仍位于根路径。

- [ ] **Step 3: 写登录跳转失败测试**

```js
import assert from 'node:assert/strict'
import test from 'node:test'
import { getLoginPath } from './auth-redirect.ts'

test('frontend-app keeps login redirects under its deployment base', () => {
  assert.equal(getLoginPath('/app/'), '/app/login')
})

test('root deployments still redirect to the root login path', () => {
  assert.equal(getLoginPath('/'), '/login')
})
```

- [ ] **Step 4: 运行测试并确认因模块不存在而失败**

Run: `npm test -- src/utils/auth-redirect.test.mjs`

Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `auth-redirect.ts`。

- [ ] **Step 5: 实现最小 API 与登录路径逻辑**

`frontend-app/src/utils/api-base.ts`：

```ts
export function getApiBaseUrl(): string {
  return '/'
}
```

`frontend-app/src/utils/auth-redirect.ts`：

```ts
export function getLoginPath(baseUrl: string): string {
  const segment = String(baseUrl || '/')
    .trim()
    .replace(/^\/+|\/+$/g, '')

  return segment ? `/${segment}/login` : '/login'
}
```

`frontend-app/src/utils/request.ts` 的 `redirectToLogin()` 使用 `getLoginPath(import.meta.env.BASE_URL)`，防止 401 后跳到根 `/login`。

- [ ] **Step 6: 设置 App 构建身份**

在 `frontend-app/vite.config.ts` 设置：

```ts
base: '/app/',
```

并将默认开发端口设置为：

```ts
port: Number(process.env.VITE_DEV_PORT) || 5176,
```

- [ ] **Step 7: 运行单元测试并确认通过**

Run: `npm test -- src/utils/api-base.test.mjs src/utils/auth-redirect.test.mjs`

Expected: 3 tests, 3 passed, 0 failed。

- [ ] **Step 8: 构建并验证产物 base**

Run: `npm run build`

Expected: exit 0；`dist/index.html` 中入口脚本、CSS 和 `config.js` 均以 `/app/` 开头。

- [ ] **Step 9: 提交**

```bash
git add frontend-app/src/utils frontend-app/vite.config.ts
git commit -m "feat: configure frontend-app deployment base"
```

### Task 2: 生产组合镜像与 Nginx `/app/` 路由

**Files:**
- Modify: `frontend/Dockerfile:43-77`
- Modify: `.dockerignore:23-30`
- Modify: `frontend/Dockerfile.dockerignore:1-14`
- Modify: `frontend/nginx.conf:129-175`
- Modify: `frontend/docker-entrypoint.sh:10-16`

**Interfaces:**
- Consumes: `frontend-app/dist`、`FRONTEND_APP_ENABLED`。
- Produces: 镜像目录 `/usr/share/nginx/html/app`；Nginx 路由 `/app`、`/app/assets/`、`/app/`。

- [ ] **Step 1: 记录当前失败的容器行为**

使用当前配置构建镜像并启动容器后请求：

```bash
curl -I http://127.0.0.1:18080/app
curl -s http://127.0.0.1:18080/app/ | grep '/app/assets/'
```

Expected: `/app` 不会按设计跳转，`/app/` 回退到根前端且不包含 `/app/assets/`。

- [ ] **Step 2: 增加第四个 Docker builder**

在 `frontend/Dockerfile` 增加 `frontend-app-builder`，使用 `/app/frontend-app` 工作目录、复制 `frontend-app/package.json`、`frontend-app/packages/` 和源码并运行 `npm run build`。最终阶段增加：

```dockerfile
COPY --from=frontend-app-builder /app/frontend-app/dist /usr/share/nginx/html/app
```

- [ ] **Step 3: 扩展构建上下文**

`.dockerignore` 增加 `frontend-app/dist/`；`frontend/Dockerfile.dockerignore` 放行 `frontend-app/**`，并排除：

```text
frontend-app/node_modules/
frontend-app/dist/
```

- [ ] **Step 4: 增加 Nginx 路由**

在通用根路由语义覆盖 `/app/` 之前增加三个 location：

```nginx
location = /app {
    set $app_enabled "${FRONTEND_APP_ENABLED}";
    if ($app_enabled = "false") { return 404; }
    return 301 /app/;
}

location ^~ /app/assets/ {
    set $app_enabled "${FRONTEND_APP_ENABLED}";
    if ($app_enabled = "false") { return 404; }
    root /usr/share/nginx/html;
    add_header Cache-Control "public, max-age=31536000, immutable" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
}

location /app/ {
    set $app_enabled "${FRONTEND_APP_ENABLED}";
    if ($app_enabled = "false") { return 404; }
    root /usr/share/nginx/html;
    index index.html;
    try_files $uri $uri/ /app/index.html;
    add_header Cache-Control "no-cache, must-revalidate" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
}
```

- [ ] **Step 5: 注入运行时开关**

`frontend/docker-entrypoint.sh` 增加：

```sh
export FRONTEND_APP_ENABLED=${FRONTEND_APP_ENABLED:-true}
```

并在 `envsubst` 变量列表中加入 `${FRONTEND_APP_ENABLED}`。

- [ ] **Step 6: 构建并执行容器行为验证**

Run:

```bash
docker build --build-arg COMMIT_ID_ARG=test -f frontend/Dockerfile -t rochekap/frontend:frontend-app-test .
docker run --rm -d --name rochekap-frontend-app-test -p 18080:80 -e APP_HOST=host.docker.internal -e FRONTEND_APP_ENABLED=true rochekap/frontend:frontend-app-test
curl -I http://127.0.0.1:18080/app
curl -s http://127.0.0.1:18080/app/ | grep '/app/assets/'
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/app/platform/creatChat
docker stop rochekap-frontend-app-test
```

Expected: 301 到 `/app/`；HTML 引用 `/app/assets/`；深层 SPA 路由返回 200。

随后以 `FRONTEND_APP_ENABLED=false` 启动同一镜像，验证 `/app`、`/app/` 和一条 `/app/assets/...` 均为 404。

- [ ] **Step 7: 提交**

```bash
git add .dockerignore frontend/Dockerfile frontend/Dockerfile.dockerignore frontend/nginx.conf frontend/docker-entrypoint.sh
git commit -m "feat: package frontend-app in combined image"
```

### Task 3: 源码部署、本地开发与构建工具

**Files:**
- Modify: `docker-compose.server-dev.yml:5-45`
- Modify: `docker-compose.server-dev.yml` volumes section
- Modify: `docker/Dockerfile.frontend.source`
- Modify: `scripts/dev.sh`
- Modify: `Makefile`
- Modify: `scripts/build_frontend_dist.sh:1-18`
- Create: `scripts/build_frontend_app_dist.sh`

**Interfaces:**
- Consumes: `frontend-app` 源码目录及独立 `node_modules` 卷。
- Produces: server-dev 镜像中的 `/app/` 产物、`make dev-frontend-app`、通用与专用静态构建入口。

- [ ] **Step 1: 扩展 server-dev 单服务配置**

为唯一的 `frontend` 服务增加：

```yaml
- FRONTEND_APP_ENABLED=${FRONTEND_APP_ENABLED:-true}
- ./frontend-app:/src/frontend-app
- frontend-app-node-modules:/src/frontend-app/node_modules
```

并在顶层 volumes 定义 `frontend-app-node-modules`，不增加新 service。

- [ ] **Step 2: 扩展源码构建镜像**

在 `docker/Dockerfile.frontend.source` 中按 admin 的现有模式构建 `/src/frontend-app`，将结果暂存后复制到 `/usr/share/nginx/html/app`，并保持四套 `node_modules` 独立。

- [ ] **Step 3: 增加本地开发入口**

在 `scripts/dev.sh` 增加 `frontend-app` 命令分支，使用端口 `5176` 并输出地址 `http://localhost:5176/app/`。在 `Makefile` 增加：

```make
dev-frontend-app:
	./scripts/dev.sh frontend-app
```

同时更新帮助文本和四套前端描述。

- [ ] **Step 4: 扩展静态构建脚本**

`scripts/build_frontend_dist.sh` 的 case 接受 `frontend-app`，错误信息列出四个有效目标。新增可执行 wrapper：

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/build_frontend_dist.sh" frontend-app
```

- [ ] **Step 5: 校验 Compose 与脚本行为**

Run:

```bash
docker compose -f docker-compose.server-dev.yml config --services
docker compose -f docker-compose.server-dev.yml config
bash -n scripts/dev.sh scripts/build_frontend_dist.sh scripts/build_frontend_app_dist.sh
```

Expected: 服务列表中只有一个 `frontend`；渲染配置包含 `FRONTEND_APP_ENABLED`、源码挂载和命名卷；所有脚本语法通过。

- [ ] **Step 6: 验证 app wrapper 构建**

Run: `bash scripts/build_frontend_app_dist.sh`

Expected: exit 0，生成 `frontend-app/dist/index.html`，其中资源路径以 `/app/` 开头。

- [ ] **Step 7: 提交**

```bash
git add docker-compose.server-dev.yml docker/Dockerfile.frontend.source scripts/dev.sh Makefile scripts/build_frontend_dist.sh scripts/build_frontend_app_dist.sh
git commit -m "feat: add frontend-app development tooling"
```

### Task 4: 最终回归验证与交付整理

**Files:**
- Modify only if verification exposes a scoped defect in files from Tasks 1-3.

**Interfaces:**
- Consumes: Tasks 1-3 的完整改动。
- Produces: 可审查、可构建、可部署的最终分支。

- [ ] **Step 1: 运行 `frontend-app` 全量测试**

Run: `npm test`

Expected: 新增测试通过；若存在从上游复制而来的既有失败，分别在未修改的 `origin/main:frontend-app` 基线复现并记录。

- [ ] **Step 2: 重新执行生产构建**

Run: `npm run build`

Expected: exit 0，产物资源使用 `/app/` base。

- [ ] **Step 3: 运行配置与差异检查**

Run:

```bash
git diff --check origin/main...HEAD
git status --short
git diff --stat origin/main...HEAD
git grep -n 'FRONTEND_APP_IMAGE' -- . ':!docs/**'
```

Expected: 无空白错误；没有 `FRONTEND_APP_IMAGE`；改动仅覆盖设计范围。

- [ ] **Step 4: 重跑组合镜像和路由冒烟**

重新运行 Task 2 的 Docker build、启用/关闭开关路由检查，以及 Task 3 的 Compose config 检查。所有命令必须使用本次最新工作树状态并记录退出码。

- [ ] **Step 5: 将实现提交整理为用户指定的信息**

在最终验证全部完成后，根据用户要求保留或压缩实现提交；设计与实施计划可一并保留，也可在最终合并前按用户明确指示整理。
