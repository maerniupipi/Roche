# Demo Registration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在当前 Demo 部署中开放邮箱注册，并允许注册用户选择 `viewer` 或 `system_admin`。

**Architecture:** 保持现有前后端认证接口不变，只调整生产部署层传入应用容器的两个注册环境变量。部署脚本、Compose 覆盖、环境示例和脚本测试保持同一有效值；OIDC 临时兼容逻辑完全不变。

**Tech Stack:** Bash、Docker Compose、GitLab CI、现有 Go/Vue 认证实现

## Global Constraints

- `AUTH_REGISTRATION_ENABLE=true`。
- `AUTH_REGISTRATION_DEV_ROLE_SELECTION=true`。
- `AUTH_REGISTRATION_DEFAULT_ROLE=viewer` 保持不变。
- OIDC/SSO 配置、CI 临时兼容开关、数据库和前后端认证代码不变。
- 不暂存或提交当前工作区中的数据库迁移草稿。

---

### Task 1: 开放 Demo 注册和角色选择

**Files:**
- Modify: `scripts/production_server_test.sh`
- Modify: `scripts/production_server.sh`
- Modify: `docker-compose.production.yml`
- Modify: `.env.production.example`
- Modify: `docs/superpowers/specs/2026-08-01-enable-demo-registration-design.md`
- Create: `docs/superpowers/plans/2026-08-01-enable-demo-registration.md`

**Interfaces:**
- Consumes: `PRODUCTION_ENV_FILE` 和现有 `PRODUCTION_ALLOW_OIDC_DISABLED` 调用方兼容开关。
- Produces: 应用容器中的 `AUTH_REGISTRATION_ENABLE=true` 与 `AUTH_REGISTRATION_DEV_ROLE_SELECTION=true`。

- [x] **Step 1: 修改脚本测试，先表达新的有效配置**

  将测试环境和断言中的注册有效值改为：

  ```bash
  AUTH_REGISTRATION_ENABLE=true
  AUTH_REGISTRATION_DEV_ROLE_SELECTION=true
  ```

  保留 OIDC 临时禁用、Workday 和不可变镜像校验用例。

- [x] **Step 2: 运行测试并确认旧实现失败**

  Run:

  ```bash
  bash scripts/production_server_test.sh
  ```

  Expected: FAIL，输出表明生产脚本仍把注册有效值强制为 `false`。

- [x] **Step 3: 实现最小部署配置改动**

  在 `scripts/production_server.sh` 中将两个强制有效值设为 `true`，并将校验改为要求 `true`；在 `docker-compose.production.yml` 和 `.env.production.example` 中同步设置：

  ```text
  AUTH_REGISTRATION_ENABLE=true
  AUTH_REGISTRATION_DEV_ROLE_SELECTION=true
  ```

- [x] **Step 4: 运行部署脚本测试并确认通过**

  Run:

  ```bash
  bash scripts/production_server_test.sh
  ```

  Expected: exit 0，注册、OIDC 临时兼容、Workday 和部署安全校验全部通过。

- [x] **Step 5: 检查最终差异和提交范围**

  Run:

  ```bash
  git diff --check
  git diff -- scripts/production_server.sh scripts/production_server_test.sh docker-compose.production.yml .env.production.example
  git status --short
  ```

  Expected: 只有注册配置、测试、设计和计划文件属于本次提交；迁移草稿保持未暂存。

- [x] **Step 6: 合并到最近提交**

  仅暂存本计划列出的文件，然后执行：

  ```bash
  git commit --amend -m "fix: 开放 Demo 环境注册和角色选择"
  ```

  Expected: 最近的设计提交被替换为一个包含设计、实现和测试的提交。
