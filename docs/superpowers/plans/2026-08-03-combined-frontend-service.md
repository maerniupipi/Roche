# Combined Frontend Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and deploy `frontend` and `frontend-default` as two static applications inside one `frontend` image and one Compose service.

**Architecture:** A multi-stage Dockerfile builds each Vue application independently and copies their distributions into one Nginx image at `/usr/share/nginx/html` and `/usr/share/nginx/html/default`. Nginx serves `/` and `/default/` locally, proxies root `/api/` to `app`, and preserves `FRONTEND_DEFAULT_ENABLED` as an entry switch. Compose and CI manage only the single `FRONTEND_IMAGE`.

**Tech Stack:** Vue 3, Vite 7, Node.js 22/24, Nginx 1.28, Docker Compose, GitLab CI, Bash, Node test runner.

## Global Constraints

- Keep `frontend` and `frontend-default` as separate source projects.
- Production runs one `frontend` image and one `frontend` container.
- `/` serves `frontend`; `/default/` serves `frontend-default`; `/api/` proxies to `app`.
- `frontend-default` keeps Vite `base: '/default/'` but sends API requests to root `/api/v1/...`.
- Preserve `FRONTEND_DEFAULT_ENABLED`; `false` makes `/default/` return 404.
- Local standalone Vite commands remain available for both projects.
- Do not modify backend API or CORS behavior.

## File Map

- `frontend-default/src/utils/api-base.ts`: decouple API URL from the static deployment base.
- `frontend-default/src/utils/api-base.test.mjs`: regression test for root API routing.
- `frontend/Dockerfile`: build both frontend projects from repository-root context and package one Nginx image.
- `frontend/nginx.conf`: serve the second distribution locally under `/default/`.
- `frontend/docker-entrypoint.sh`: render only backend variables plus `FRONTEND_DEFAULT_ENABLED`.
- `docker/Dockerfile.frontend.source`: build both mounted projects into one source deployment container.
- `docker-compose.server-dev.yml`: mount both source projects into one `frontend` service and remove the second service.
- `docker-compose.production.yml`: remove the required `FRONTEND_DEFAULT_IMAGE` service override.
- `.gitlab-ci.yml`, `Makefile`, `scripts/build_images.sh`: build one combined image from repository-root context.
- `frontend-default/Dockerfile`, `frontend-default/nginx.conf`, `frontend-default/docker-entrypoint.sh`, `docker/Dockerfile.frontend-default.source`: remove obsolete standalone-container files.

---

### Task 1: Fix frontend-default API routing

**Files:**
- Create: `frontend-default/src/utils/api-base.test.mjs`
- Modify: `frontend-default/src/utils/api-base.ts`

**Interfaces:**
- Produces: `getApiBaseUrl(): string` returning `/` for root same-origin API requests.
- Consumed by: `frontend-default/src/utils/request.ts` as Axios `baseURL`.

- [ ] **Step 1: Write the failing regression test**

```js
import assert from 'node:assert/strict'
import test from 'node:test'
import { getApiBaseUrl } from './api-base.ts'

test('frontend-default keeps API requests at the origin root', () => {
  assert.equal(getApiBaseUrl(), '/')
})
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
cd frontend-default
node --test src/utils/api-base.test.mjs
```

Expected: FAIL because the existing implementation reads `import.meta.env.BASE_URL` and resolves the application deployment prefix instead of a stable root API base.

- [ ] **Step 3: Implement the minimal API base**

Replace the implementation with:

```ts
export function getApiBaseUrl(): string {
  return '/'
}
```

- [ ] **Step 4: Run the focused and full frontend-default tests**

```bash
cd frontend-default
node --test src/utils/api-base.test.mjs
npm test
```

Expected: PASS; requests such as `/api/v1/auth/login` no longer become `/default/api/v1/auth/login`.

- [ ] **Step 5: Commit**

```bash
git add frontend-default/src/utils/api-base.ts frontend-default/src/utils/api-base.test.mjs
git commit -m "fix: keep default frontend APIs at root"
```

### Task 2: Package both frontend distributions in one image

**Files:**
- Modify: `frontend/Dockerfile`
- Delete: `frontend-default/Dockerfile`

**Interfaces:**
- Consumes: repository-root Docker build context containing `frontend/` and `frontend-default/`.
- Produces: one image with `/usr/share/nginx/html/index.html` and `/usr/share/nginx/html/default/index.html`.

- [ ] **Step 1: Record the current failing build boundary**

Run:

```bash
docker build --platform linux/amd64 -f frontend/Dockerfile -t rochekap/frontend:combined-test .
```

Expected: FAIL because the existing Dockerfile assumes `frontend/` is the build context and cannot build both source directories.

- [ ] **Step 2: Add two independent builder stages**

Use `frontend-builder` with `WORKDIR /app/frontend` and `frontend-default-builder` with `WORKDIR /app/frontend-default`. In each stage copy that project's `package.json`, `packages/`, and remaining source from the repository-root context, set `VITE_IS_DOCKER=true`, pass through `COMMIT_ID_ARG`, and run `npm install --no-package-lock && npm run build`.

The runtime stage must contain:

```dockerfile
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html
COPY --from=frontend-default-builder /app/frontend-default/dist /usr/share/nginx/html/default
COPY frontend/nginx.conf /etc/nginx/templates/default.conf.template
COPY frontend/docker-entrypoint.sh /docker-entrypoint.sh
```

- [ ] **Step 3: Build the combined image**

```bash
docker build \
  --platform linux/amd64 \
  --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
  --build-arg COMMIT_ID_ARG=combined-test \
  -f frontend/Dockerfile \
  -t rochekap/frontend:combined-test \
  .
```

Expected: PASS.

- [ ] **Step 4: Verify both distributions exist**

```bash
docker run --rm --entrypoint sh rochekap/frontend:combined-test -ec \
  'test -f /usr/share/nginx/html/index.html && test -f /usr/share/nginx/html/default/index.html'
```

Expected: exit 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/Dockerfile frontend-default/Dockerfile
git commit -m "build: package both frontends in one image"
```

### Task 3: Serve `/default/` locally from the combined Nginx image

**Files:**
- Modify: `frontend/nginx.conf`
- Modify: `frontend/docker-entrypoint.sh`
- Delete: `frontend-default/nginx.conf`
- Delete: `frontend-default/docker-entrypoint.sh`

**Interfaces:**
- Consumes: static files in `/usr/share/nginx/html/default` and environment variable `FRONTEND_DEFAULT_ENABLED`.
- Produces: `/default/` SPA routing without an upstream frontend container.

- [ ] **Step 1: Replace the `/default/` reverse proxy**

Set `FRONTEND_DEFAULT_ENABLED` to `true` by default in `docker-entrypoint.sh` and include it in `envsubst`. Remove `FRONTEND_DEFAULT_HOST`, `FRONTEND_DEFAULT_PORT`, and `FRONTEND_DEFAULT_SCHEME`.

In `nginx.conf`, make `/default/` use `root /usr/share/nginx/html` and:

```nginx
set $default_enabled "${FRONTEND_DEFAULT_ENABLED}";
if ($default_enabled = "false") { return 404; }
try_files $uri $uri/ /default/index.html;
```

Add a `/default/assets/` location using the same enable check and immutable cache headers. Keep `/api/` before the SPA fallback behavior and proxy it directly to `app`.

- [ ] **Step 2: Validate rendered Nginx configuration**

```bash
docker run --rm --entrypoint /bin/sh --add-host app:127.0.0.1 \
  -e FRONTEND_DEFAULT_ENABLED=true \
  -e APP_HOST=app -e APP_PORT=8080 -e APP_SCHEME=http \
  -e MAX_FILE_SIZE=50M \
  rochekap/frontend:combined-test -ec \
  'envsubst '\''${MAX_FILE_SIZE} ${APP_HOST} ${APP_PORT} ${APP_SCHEME} ${FRONTEND_DEFAULT_ENABLED}'\'' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -t'
```

Expected: configuration syntax is valid and contains no unresolved `FRONTEND_DEFAULT_HOST`, `FRONTEND_DEFAULT_PORT`, or `FRONTEND_DEFAULT_SCHEME` variables.

- [ ] **Step 3: Smoke-test both routes and the feature switch**

Start the image with `--add-host app:127.0.0.1` on a free local port. Verify:

```bash
curl -f http://127.0.0.1:18080/
curl -f http://127.0.0.1:18080/default/
curl -f http://127.0.0.1:18080/default/chat/example
```

Start a second container with `FRONTEND_DEFAULT_ENABLED=false` and verify:

```bash
test "$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/default/)" = "404"
```

- [ ] **Step 4: Commit**

```bash
git add frontend/nginx.conf frontend/docker-entrypoint.sh frontend-default/nginx.conf frontend-default/docker-entrypoint.sh
git commit -m "feat: serve default frontend from combined nginx"
```

### Task 4: Collapse Compose to one frontend service

**Files:**
- Modify: `docker/Dockerfile.frontend.source`
- Delete: `docker/Dockerfile.frontend-default.source`
- Modify: `docker-compose.server-dev.yml`
- Modify: `docker-compose.production.yml`

**Interfaces:**
- Produces: a single Compose service named `frontend` with both source trees mounted and only `FRONTEND_IMAGE` required in production.

- [ ] **Step 1: Reproduce the current production interpolation failure**

```bash
APP_IMAGE=rochekap/app:test-sha \
FRONTEND_IMAGE=rochekap/frontend:test-sha \
DOCREADER_IMAGE=rochekap/docreader:test-sha \
docker compose -f docker-compose.server-dev.yml -f docker-compose.production.yml config --quiet
```

Expected: FAIL with `FRONTEND_DEFAULT_IMAGE is missing a value`.

- [ ] **Step 2: Update the source image and service mounts**

Make `docker/Dockerfile.frontend.source` build mounted `/src/frontend` into `/tmp/frontend-dist` and `/src/frontend-default` into `/tmp/frontend-default-dist`, then copy them to `/usr/share/nginx/html` and `/usr/share/nginx/html/default` before starting the shared entrypoint.

In `docker-compose.server-dev.yml`:

- Remove the `frontend-default` service.
- Remove `frontend`'s `depends_on.frontend-default`.
- Remove `FRONTEND_DEFAULT_HOST`, `FRONTEND_DEFAULT_PORT`, and `FRONTEND_DEFAULT_SCHEME`.
- Keep `FRONTEND_DEFAULT_ENABLED`.
- Mount both source directories and separate node_modules volumes into the one `frontend` container.

In `docker-compose.production.yml`, delete the entire `frontend-default` service block.

- [ ] **Step 3: Verify Compose configuration is complete with three image variables**

Run the command from Step 1 again.

Expected: PASS without `FRONTEND_DEFAULT_IMAGE`.

- [ ] **Step 4: Verify there is one frontend service**

```bash
docker compose -f docker-compose.server-dev.yml -f docker-compose.production.yml config --services
```

Expected: contains `frontend` and does not contain `frontend-default`.

- [ ] **Step 5: Commit**

```bash
git add docker/Dockerfile.frontend.source docker/Dockerfile.frontend-default.source docker-compose.server-dev.yml docker-compose.production.yml
git commit -m "fix: deploy both frontends as one service"
```

### Task 5: Align CI and local image commands with the combined image

**Files:**
- Modify: `.gitlab-ci.yml`
- Modify: `Makefile`
- Modify: `scripts/build_images.sh`
- Keep: `scripts/build_frontend_dist.sh`
- Keep: `scripts/build_frontend_default_dist.sh`
- Keep: `scripts/dev.sh`

**Interfaces:**
- Produces: one immutable `rochekap/frontend:${CI_COMMIT_SHORT_SHA}` image containing both distributions.

- [ ] **Step 1: Update GitLab CI build context**

Keep the existing image name and change the frontend build to use repository-root context:

```bash
docker build \
  --platform linux/amd64 \
  --build-arg NPM_REGISTRY="https://registry.npmmirror.com" \
  --build-arg COMMIT_ID_ARG="${CI_COMMIT_SHORT_SHA}" \
  -f frontend/Dockerfile \
  -t rochekap/frontend:${CI_COMMIT_SHORT_SHA} \
  .
```

Do not introduce `FRONTEND_DEFAULT_IMAGE` or a second CI image.

- [ ] **Step 2: Remove standalone frontend-default image targets**

In `Makefile`, make `docker-build-frontend` build the root-context combined image and remove `docker-build-frontend-default` and `build-images-frontend-default`. Keep `dev-frontend-default` because it starts the standalone Vite development server.

In `scripts/build_images.sh`, remove the `--frontend-default` flag and `build_frontend_default_image`. Make `build_frontend_image` pass `COMMIT_ID_ARG` and root context to the combined Dockerfile. Keep `build_frontend_dist.sh frontend-default` for explicit local static builds.

- [ ] **Step 3: Run syntax and reference checks**

```bash
bash -n scripts/build_images.sh scripts/build_frontend_dist.sh scripts/build_frontend_default_dist.sh scripts/dev.sh
grep -R "FRONTEND_DEFAULT_IMAGE\|Dockerfile.frontend-default.source" \
  .gitlab-ci.yml Makefile scripts docker-compose.server-dev.yml docker-compose.production.yml docker
```

Expected: Bash syntax passes; grep returns no deployment references to the removed image or Dockerfile.

- [ ] **Step 4: Build through the same command used by CI**

Run the combined `docker build` command from Step 1 with `CI_COMMIT_SHORT_SHA=local-test`.

Expected: PASS and both index files exist.

- [ ] **Step 5: Commit**

```bash
git add .gitlab-ci.yml Makefile scripts/build_images.sh
git commit -m "ci: build one combined frontend image"
```

### Task 6: Final verification

**Files:**
- Verify all files changed in Tasks 1-5.

- [ ] **Step 1: Run frontend tests**

```bash
cd frontend-default && npm test
cd ../frontend && npm test
```

Expected: PASS.

- [ ] **Step 2: Validate production Compose**

```bash
APP_IMAGE=rochekap/app:test-sha \
FRONTEND_IMAGE=rochekap/frontend:test-sha \
DOCREADER_IMAGE=rochekap/docreader:test-sha \
docker compose -f docker-compose.server-dev.yml -f docker-compose.production.yml config --quiet
```

Expected: PASS.

- [ ] **Step 3: Rebuild and smoke-test the final combined image**

Repeat the combined image build and route checks from Tasks 2 and 3. Confirm `/`, `/default/`, a `/default/` deep link, and the disabled 404 behavior.

- [ ] **Step 4: Verify scope and repository state**

```bash
git diff --check
git status --short
git diff --stat HEAD~5..HEAD
```

Expected: no whitespace errors; only the combined-frontend implementation and pre-existing user migration work are present.

- [ ] **Step 5: Request code review**

Use `superpowers:requesting-code-review` against the implementation commits, address findings, and rerun the verification commands before reporting completion.

### Task 7: Support the classic Docker builder in CI

**Files:**
- Modify: `.dockerignore`
- Modify: `docs/superpowers/specs/2026-08-03-combined-frontend-service-design.md`

**Interfaces:**
- Consumes: repository-root Docker context used by `frontend/Dockerfile`.
- Produces: a context containing both `frontend/` and `frontend-default/` source trees for BuildKit and the classic Docker builder, while excluding generated dependencies and distributions.

- [ ] **Step 1: Reproduce the missing-context failure with the classic builder**

Run a minimal context probe with BuildKit disabled:

```powershell
$env:DOCKER_BUILDKIT = '0'
@'
FROM scratch
COPY frontend/package.json /frontend/package.json
COPY frontend-default/package.json /frontend-default/package.json
'@ | docker build -f - .
```

Expected: FAIL with `COPY failed: file not found in build context or excluded by .dockerignore: stat frontend/package.json: file does not exist`.

- [ ] **Step 2: Make the root ignore rules builder-independent**

In the root `.dockerignore`, replace the legacy rule that excludes `frontend/*` except for Nginx runtime files:

```dockerignore
**/node_modules/
frontend/*
!frontend/nginx.conf
!frontend/docker-entrypoint.sh
```

with rules that include both source trees and exclude only their generated outputs:

```dockerignore
**/node_modules/
frontend/dist/
frontend-default/dist/
```

Keep `frontend/Dockerfile.dockerignore` as a BuildKit-only context-size optimization.

- [ ] **Step 3: Verify the classic builder context**

Repeat the context probe from Step 1.

Expected: PASS and produce a small scratch image after both `COPY` instructions succeed.

- [ ] **Step 4: Verify current BuildKit and repository checks**

```powershell
docker build --check -f frontend/Dockerfile .
git diff --check
git status --short
```

Expected: Dockerfile check passes; only `.dockerignore`, the compatibility documentation, and pre-existing migration work are present.

- [ ] **Step 5: Fold the compatibility fix into the existing single commit**

Preserve the existing title and detailed body for `fix：前端代码结构变更部署相关`, amend the compatibility changes into that commit, confirm the resulting tree contains the classic-builder fix, then update `origin/main` with an exact `--force-with-lease` bound to the previously verified remote commit.
