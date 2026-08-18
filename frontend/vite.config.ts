import { fileURLToPath, URL } from 'node:url'
import { resolve, dirname } from 'node:path'
import { existsSync } from 'node:fs'
import { execSync } from 'node:child_process'
import { createRequire } from 'node:module'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'

const __dirname = dirname(fileURLToPath(import.meta.url))
const require = createRequire(import.meta.url)

const pkg = require('./package.json') as { version?: string }
const FRONTEND_VERSION = pkg.version ?? 'unknown'

function resolveFrontendCommit(): string {
  const fromEnv = process.env.VITE_FRONTEND_COMMIT || process.env.BUILD_COMMIT_SHA || process.env.CI_COMMIT_SHA
  if (fromEnv) {
    return fromEnv.slice(0, 7)
  }
  try {
    return execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] })
      .toString()
      .trim()
  } catch {
    return 'unknown'
  }
}

const FRONTEND_COMMIT = resolveFrontendCommit()

const DEV_PROXY_TARGET =
  process.env.VITE_DEV_PROXY_TARGET ||
  process.env.FRONTEND_BACKEND_URL ||
  'http://10.3.97.217:8089'

function resolveDevProxyOrigin(target: string): string | undefined {
  const configuredOrigin = process.env.VITE_DEV_PROXY_ORIGIN?.trim()
  if (configuredOrigin) return new URL(configuredOrigin).origin
  if (process.env.VITE_DEV_PROXY_REWRITE_ORIGIN?.trim().toLowerCase() === 'false') return undefined

  const targetURL = new URL(target)
  const loopbackHosts = new Set(['localhost', '127.0.0.1', '0.0.0.0', '[::1]'])
  return loopbackHosts.has(targetURL.hostname.toLowerCase()) ? undefined : targetURL.origin
}

// Remote gateways allow their own browser-facing origin. Browsers still send
// the local Vite origin on mutating requests, so rewrite it only for remote
// targets; fully local development keeps its localhost origin allowlist.
const DEV_PROXY_ORIGIN = resolveDevProxyOrigin(DEV_PROXY_TARGET)

function resolveVueOfficePptxEntry(): string {
  try {
    const pkgDir = dirname(require.resolve('@vue-office/pptx/package.json'))
    const candidates = [
      resolve(pkgDir, 'lib/v3/index.js'),
      resolve(pkgDir, 'lib/index.js'),
      resolve(pkgDir, 'lib/v3/vue-office-pptx.mjs'),
    ]
    const matched = candidates.find((candidate) => existsSync(candidate))
    return matched ?? '@vue-office/pptx'
  } catch {
    return '@vue-office/pptx'
  }
}

export default defineConfig({
  define: {
    __FRONTEND_VERSION__: JSON.stringify(FRONTEND_VERSION),
    __FRONTEND_COMMIT__: JSON.stringify(FRONTEND_COMMIT),
  },
  // frontend 作为当前默认入口前端，发布后挂在 / 下；
  base: '/',
  build: {
    rollupOptions: {
      input: resolve(__dirname, 'index.html'),
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          if (id.includes('mermaid') || id.includes('/dagre') || id.includes('cytoscape')) {
            return 'vendor-mermaid'
          }
          if (id.includes('marked') || id.includes('katex')) {
            return 'vendor-markdown'
          }
          if (id.includes('highlight.js')) {
            return 'vendor-highlight'
          }
        },
      },
    },
  },
  plugins: [
    vue(),
    vueJsx(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@vue-office/pptx': resolveVueOfficePptxEntry(),
    },
  },
  server: {
    // 默认 5173；可通过 VITE_DEV_PORT 覆盖。
    port: Number(process.env.VITE_DEV_PORT) || 5173,
    host: true,
    // 代理配置，用于开发环境
    proxy: {
      '/api': {
        target: DEV_PROXY_TARGET,
        changeOrigin: true,
        secure: false,
        headers: DEV_PROXY_ORIGIN ? { Origin: DEV_PROXY_ORIGIN } : undefined,
      },
      '/files': {
        target: DEV_PROXY_TARGET,
        changeOrigin: true,
        secure: false,
      }
    }
  }
})
