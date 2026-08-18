import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { getCurrentUser, userInfoFromApi } from '@/api/auth'
import { hasPendingSSOCallback } from '@/utils/sso'

function defaultAuthenticatedPath(authStore: ReturnType<typeof useAuthStore>): string {
  if (authStore.canManageKnowledge) {
    // 优先取菜单 store 第一项的 path（管理端可能按角色下发不同首项）。
    // 这里 fire-and-forget 触发 loadMenu：首次访问 / 时 menu.vue 还没 mount
    // （侧栏只在 /platform/* 渲染），menuStore 是空的；后续跳转前菜单
    // 已就绪，firstNavigableNode 就有效了。
    // loadMenu 内部有 loaded 防重，多次触发幂等。
    try {
      const { useMenuStore } = require('@/stores/menu') as typeof import('@/stores/menu')
      const menuStore = useMenuStore()
      void menuStore.loadMenu()
      const first = menuStore.firstNavigableNode
      if (first?.path) return first.path
    } catch {
      // 兼容 SSR / 单元测试等没有 Pinia 上下文的环境
    }
    return '/platform/dashboard'
  }
  return '/platform/creatChat'
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: "/",
      redirect: "/platform/dashboard",
    },
    {
      path: "/login",
      name: "login",
      component: () => import("../views/auth/Login.vue"),
      meta: { requiresAuth: false, requiresInit: false }
    },
    {
      path: "/knowledgeBase",
      name: "home",
      component: () => import("../views/knowledge/KnowledgeBase.vue"),
      meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
    },
    {
      path: "/platform",
      name: "Platform",
      redirect: () => defaultAuthenticatedPath(useAuthStore()),
      component: () => import("../views/platform/index.vue"),
      meta: { requiresInit: true, requiresAuth: true },
      children: [
        {
          path: "knowledgeDomain",
          redirect: "/platform/settings"
        },
        {
          path: "settings",
          name: "settings",
          component: () => import("../views/settings/Settings.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "dashboard",
          name: "dashboard",
          component: () => import("../views/dashboard/Dashboard.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "recommend-questions",
          name: "recommendQuestions",
          component: () => import("../views/recommendQuestions/RecommendQuestions.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "answer-records",
          name: "answerRecords",
          component: () => import("../views/answerRecords/AnswerRecords.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "roles",
          name: "roles",
          component: () => import("../views/roles/Roles.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "exchange-rate",
          name: "exchangeRate",
          component: () => import("../views/exchangeRate/ExchangeRate.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          // 历史兼容：/platform/agents 路由已下线，保留重定向避免旧书签 / chat 模块链接 404。
          path: "agents",
          redirect: "/platform/dashboard",
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          // 父路由：访问 `/platform/knowledge-bases`（不带 domainId）时，
          // 通过 `beforeEnter` 异步重定向到第一个知识域子路由。
          // 这里用 sibling 路由而不是 children 父子结构，是为了避免父子
          // 共用同一组件（`KnowledgeBaseList.vue`）时的双实例渲染问题：
          // 子路由必须出现在父组件的 <router-view> 内，而我们的列表组件
          // 自身没有 router-view 槽位。
          path: "knowledge-bases",
          name: "knowledgeBaseList",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          // 默认重定向：等 useKnowledgeDomains 单例缓存 ready 后，
          // 用 `replace` 跳到第一个知识域，避免历史栈污染。
          // 如果没有任何知识域（极端空态），放行原 URL，组件内渲染空态。
          beforeEnter: async (_to, _from, next) => {
            const { useKnowledgeDomains } = await import("@/composables/useKnowledgeDomains")
            const { load, firstDomainId } = useKnowledgeDomains()
            await load()
            const firstId = firstDomainId.value
            if (firstId != null) {
              next({
                name: "knowledgeBaseListByDomain",
                params: { domainId: String(firstId) },
                replace: true,
              })
              return
            }
            next()
          },
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true },
        },
        {
          // 子路由：每个知识域对应一个 URL。props: true 让 domainId 作为
          // prop 注入到组件，避免组件自己读 route.params。
          path: "knowledge-bases/domain/:domainId(\\d+)",
          name: "knowledgeBaseListByDomain",
          component: () => import("../views/knowledge/KnowledgeBaseList.vue"),
          props: true,
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true },
        },
        {
          path: "knowledge-bases/:kbId",
          name: "knowledgeBaseDetail",
          component: () => import("../views/knowledge/KnowledgeBase.vue"),
          meta: { requiresInit: true, requiresAuth: true, requiresKnowledgeManagement: true }
        },
        {
          path: "knowledge-search",
          // 旧路径保留为重定向，打开全局命令面板（⌘K），带上可选的 q 参数
          redirect: (to) => {
            const q = to.query.q
            return {
              path: '/platform/knowledge-bases',
              query: typeof q === 'string' ? { cmdk: q } : { cmdk: '' },
            }
          },
        },
        {
          path: "creatChat",
          name: "globalCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "knowledge-bases/:kbId/creatChat",
          name: "kbCreatChat",
          component: () => import("../views/creatChat/creatChat.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        {
          path: "chat/:chatid",
          name: "chat",
          component: () => import("../views/chat/index.vue"),
          meta: { requiresInit: true, requiresAuth: true }
        },
        // Compatibility redirects for legacy /platform/system/* URLs.
        // The whole system administration surface — global settings
        // and the system-admin roster — now lives as a single section
        // inside the standard Settings modal. We keep the routes
        // around so old bookmarks / external links don't 404.
        {
          path: "system",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/settings",
          name: "systemSettings",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
        {
          path: "system/admins",
          name: "systemAdmins",
          redirect: { path: "/platform/settings", query: { section: "system-global" } },
          meta: { requiresInit: true, requiresAuth: true, requiresSystemAdmin: true },
        },
      ],
    },
    // Dev-only markdown rendering test page
    ...(import.meta.env.DEV ? [{
      path: '/platform/dev/markdown',
      name: 'markdownTest',
      component: () => import('../views/dev/MarkdownTestPage.vue'),
      meta: { requiresAuth: false, requiresInit: false }
    }] : []),
  ],
});

// 持久化 login 返回的认证信息到 store
function persistLoginResponse(authStore: ReturnType<typeof useAuthStore>, response: any) {
  if (response.user && response.token) {
    authStore.setUser(userInfoFromApi(response.user))
    authStore.setToken(response.token)
  }
}

async function hydrateSessionFromToken(authStore: ReturnType<typeof useAuthStore>) {
  const token = localStorage.getItem('roche_kap_token')
  if (!token) return false

  if (!authStore.token) {
    authStore.setToken(token)
  }

  try {
    const response = await getCurrentUser()
    const user = response.data?.user
    if (!response.success || !user) {
      return false
    }

    authStore.setUser(userInfoFromApi(user))
    return true
  } catch {
    return false
  }
}

// 路由守卫：检查认证状态和系统初始化状态
router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()

  // SAML 回跳登录结果依赖 App.vue 在挂载后消费 URL hash。
  // 如果这里先按"未登录"拦截到 /login，会导致回调结果没有机会落盘。
  if (hasPendingSSOCallback()) {
    next()
    return
  }

  // 如果访问的是登录页面或初始化页面，直接放行
  if (to.meta.requiresAuth === false || to.meta.requiresInit === false) {
    // 如果已登录用户访问登录页面，重定向到默认首页
    if (to.path === '/login' && authStore.isLoggedIn) {
      next(defaultAuthenticatedPath(authStore))
      return
    }
    next()
    return
  }

  // 检查用户认证状态
  if (to.meta.requiresAuth !== false) {
    if (!authStore.isLoggedIn) {
      const restored = await hydrateSessionFromToken(authStore)
      if (restored) {
        next(to.fullPath)
        return
      }
      next('/login')
      return
    }
  }

  // SystemAdmin gate — checked AFTER auth so a non-admin who's logged
  // out gets redirected to /login first (consistent with how the rest
  // of the auth flow works), and only an authenticated non-admin sees
  // the bounce. This is UI-only; the server enforces the real check.
  if (to.meta.requiresSystemAdmin === true) {
    if (!authStore.isSystemAdmin) {
      next(defaultAuthenticatedPath(authStore))
      return
    }
  }

  if (to.meta.requiresKnowledgeManagement === true && !authStore.canManageKnowledge) {
    next(defaultAuthenticatedPath(authStore))
    return
  }

  next()
})

export default router
