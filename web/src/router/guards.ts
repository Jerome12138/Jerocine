import type { Router } from 'vue-router'
import { getToken } from '@/utils/token'

const APP_TITLE = (import.meta.env.VITE_APP_TITLE as string | undefined) ?? 'Jerocine'

/**
 * 全局前后置守卫
 *
 * 鉴权策略（与 user-api-frontend-guide.md 对齐）：
 *  - requiresAuth：缺 token → /login?redirect=...
 *  - requiresAdmin：role != 1 → /index 并 toast"权限不足"（懒加载 user store 避免循环依赖）
 *  - TV 模式：屏蔽 /manage/*
 */
export function registerGuards(router: Router): void {
  router.beforeEach(async (to) => {
    const needAuth = !!to.meta.requiresAuth || !!to.meta.requiresAdmin

    if (needAuth) {
      if (!getToken()) {
        return {
          path: '/login',
          query: { redirect: to.fullPath }
        }
      }
    }

    if (to.meta.requiresAdmin) {
      // 懒加载 store + http 防循环依赖；store 内部从 token 派生 isLoggedIn
      try {
        const [{ useUserStore }, httpMod] = await Promise.all([
          import('@/stores/user'),
          import('@/api/http')
        ])
        const store = useUserStore()
        if (!store.info) {
          // 未拉过 user info：先尝试一次（拦截器若 401 会清 token + replace /login）
          await store.fetchInfo().catch(() => undefined)
        }
        if (!store.isLoggedIn) {
          return { path: '/login', query: { redirect: to.fullPath } }
        }
        if (!store.isAdmin) {
          // 非管理员误进 /manage：tip 一下并送回首页
          httpMod.toast?.('error', '权限不足，仅管理员可访问后台')
          return { path: '/index' }
        }
      } catch {
        // store / http 加载失败：保守起见送回登录
        return { path: '/login', query: { redirect: to.fullPath } }
      }
    }

    // TV 模式禁止访问 /manage（在 useViewMode install 后生效）
    if (typeof document !== 'undefined') {
      const mode = document.documentElement.getAttribute('data-mode')
      if (mode === 'tv' && to.path.startsWith('/manage')) {
        return { path: '/index' }
      }
    }

    return true
  })

  router.afterEach((to) => {
    const t = (to.meta.title as string | undefined) ?? ''
    document.title = t ? `${t} - ${APP_TITLE}` : APP_TITLE
  })
}
