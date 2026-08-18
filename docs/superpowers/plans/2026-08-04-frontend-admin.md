# Frontend Admin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从最新 `frontend` 创建独立的 `frontend-admin`，并以 `/admin/` 接入现有单镜像、单前端服务部署。

**Architecture:** 三套 Vue 工程独立构建，静态产物分别放在 Nginx 根目录、`default/` 与 `admin/`。生产和服务器源码部署继续只有一个 Compose `frontend` 服务以及一个 `FRONTEND_IMAGE`。

**Tech Stack:** Vue 3、Vite、npm、Docker multi-stage build、Nginx、Docker Compose、Bash、Make。

## Global Constraints

- 复制基线固定为 `origin/main` 的 `baa19182f2cb733aa6e3cf2116a80bceb98463f0`。
- 新入口固定为 `/admin/`，本地默认端口固定为 `5175`。
- 不新增 `frontend-admin` Compose 服务或 `FRONTEND_ADMIN_IMAGE`。
- `/api/` 与 `/files` 保持根路径并继续代理到 `app`。
- 验收只覆盖 admin 构建、CI 同款镜像构建、单服务 Compose 和 admin 路由冒烟。

---

### Task 1: 创建独立 `frontend-admin` 工程

**Files:**
- Create: `frontend-admin/**`（从 `frontend/**` 复制）
- Modify: `frontend-admin/package.json`
- Modify: `frontend-admin/vite.config.ts`

**Interfaces:**
- Consumes: `frontend` 在 `baa19182` 的完整受 Git 管理文件集合。
- Produces: 包名带 `-admin`、Vite base 为 `/admin/`、默认端口为 `5175` 的独立工程。

- [ ] **Step 1: 运行缺失工程契约并确认失败**

```powershell
if (-not (Test-Path frontend-admin/package.json)) { throw 'frontend-admin is missing' }
```

Expected: FAIL with `frontend-admin is missing`。

- [ ] **Step 2: 复制受 Git 管理的 frontend 文件**

使用 `git ls-files frontend` 得到精确文件列表，将每个文件复制到相同的 `frontend-admin` 相对路径；不复制 `node_modules` 或 `dist`。

- [ ] **Step 3: 调整独立项目身份和 Vite 配置**

```text
package name: roche-knowledge-agent-platform-ui-admin
base: /admin/
port: Number(process.env.VITE_DEV_PORT) || 5175
DEV_PROXY_TARGET: VITE_DEV_PROXY_TARGET -> FRONTEND_BACKEND_URL -> http://localhost:8080
```

- [ ] **Step 4: 运行工程契约并确认通过**

校验目录存在、包名、`base`、端口和可配置代理；再执行 `npm test` 与 `npm run build`。

### Task 2: 接入组合镜像、Nginx 与 Compose

**Files:**
- Modify: `.dockerignore`
- Modify: `frontend/Dockerfile`
- Modify: `frontend/Dockerfile.dockerignore`
- Modify: `frontend/nginx.conf`
- Modify: `frontend/docker-entrypoint.sh`
- Modify: `docker/Dockerfile.frontend.source`
- Modify: `docker-compose.server-dev.yml`

**Interfaces:**
- Consumes: Task 1 的 `frontend-admin` 工程。
- Produces: `/usr/share/nginx/html/admin` 静态产物、`/admin/` 路由和默认开启的 `FRONTEND_ADMIN_ENABLED`。

- [ ] **Step 1: 运行部署契约并确认失败**

检查 Dockerfile 中不存在 `frontend-admin-builder`、Nginx 中不存在 `/admin/`、Compose 中不存在 admin 挂载；预期因功能缺失失败。

- [ ] **Step 2: 扩展构建上下文与生产 Dockerfile**

加入第三个 builder，将 `frontend-admin/dist` 复制到 `/usr/share/nginx/html/admin`；根 ignore 和 Dockerfile 专属 ignore 均允许 admin 源码、排除 admin 生成物。

- [ ] **Step 3: 扩展 Nginx 和 entrypoint**

加入 `/admin -> /admin/`、`/admin/assets/`、`/admin/` SPA fallback；通过 `FRONTEND_ADMIN_ENABLED` 控制 404，默认 `true`。

- [ ] **Step 4: 扩展服务器源码镜像与 Compose**

源码镜像构建第三套挂载源码；Compose 增加 admin 源码挂载、独立 node_modules 卷和开关，但不增加服务。

- [ ] **Step 5: 运行部署契约并确认通过**

校验 Docker、Nginx、entrypoint、Compose 所有 admin 接入点，并确认没有 `FRONTEND_ADMIN_IMAGE`。

### Task 3: 补齐本地开发和静态构建命令

**Files:**
- Modify: `Makefile`
- Modify: `scripts/dev.sh`
- Modify: `scripts/build_frontend_dist.sh`
- Create: `scripts/build_frontend_admin_dist.sh`

**Interfaces:**
- Consumes: Task 1 的独立 admin 工程。
- Produces: `make dev-frontend-admin`、`scripts/dev.sh frontend-admin` 和 admin 静态构建 wrapper。

- [ ] **Step 1: 运行命令契约并确认失败**

检查 `make -n dev-frontend-admin` 和 `scripts/build_frontend_dist.sh` 对 admin 的支持；预期因目标缺失失败。

- [ ] **Step 2: 按 default 现有模式增加 admin 命令**

新增 Make 目标、帮助文本、dev.sh 启动函数与 case；扩展通用构建脚本的允许值，并增加薄 wrapper。

- [ ] **Step 3: 运行命令契约并确认通过**

执行 `make -n dev-frontend-admin`、Bash 语法检查和静态引用检查。

### Task 4: 最小发布验收

**Files:**
- Modify: `docs/superpowers/specs/2026-08-04-frontend-admin-design.md`（将验收清单收敛为本任务四项）

**Interfaces:**
- Consumes: Tasks 1-3 的完整变更。
- Produces: 可提交的 admin 单服务部署改动。

- [ ] **Step 1: Admin 工程验收**

```bash
cd frontend-admin && npm test && npm run build
```

- [ ] **Step 2: Compose 单服务验收**

渲染 server-dev/production 组合配置，确认配置有效且服务列表只有 `frontend`，没有 `frontend-default` 或 `frontend-admin`。

- [ ] **Step 3: CI 同款镜像构建与路由冒烟**

使用 `.gitlab-ci.yml` 同款根上下文构建命令生成临时镜像。启动后只检查 `/`、`/default/`、`/admin/` 与 admin 深层 SPA 路由，并验证关闭 admin 开关时返回 404。

- [ ] **Step 4: 最终差异和工作区检查**

运行 `git diff --check`、确认无意外文件、确认不存在 `FRONTEND_ADMIN_IMAGE`，然后提交实现。
