import router from '@/router'
import { isNative } from '@/utils/jerocineNative'
import { useUserStore } from '@/stores/user'

/**
 * 原生设置抽屉的「账号」行数据源（2026-10-10 抽屉改版）。
 *
 * 背景：原生抽屉要直接显示 web 登录态的用户信息，右侧按钮直接走 web 的登录/退出，
 * 不再跳 SPA `/settings`（该页已删）。Java 摸不到 pinia，所以沿用 `__jcSetMode` 的先例 ——
 * 原生壳里挂 `window.__jcAuth` 钩子，Java 经 evaluateJavascript 调用。
 *
 * 三个入口：
 *   - `__jcAuth.user()`  → JSON 字符串 `{loggedIn, name}`（Java 解析后刷行内文案）
 *   - `__jcAuth.login()` → 跳 web 自己的 /login（带 redirect 回当前页；登录页是 SPA 路由，
 *     在 WebView 内导航，不会被 ServerNavigationWebViewClient 丢给系统浏览器）
 *   - `__jcAuth.logout()`→ 调 userStore.logout()（后端注销 + 清 token；异步，Java 端
 *     调完应延迟重查 user() 刷行）
 *
 * 钩子只在原生壳里安装（浏览器没有 Java 调用方，挂上只是多一个无人用的全局）。
 */

/** 给 Java 的账号快照（行内文案的唯一数据源） */
export interface NativeAuthSnapshot {
  loggedIn: boolean
  name: string
}

/** 从 user store 取当前快照（displayName 已含 nickName > userName 兜底链） */
export function authSnapshot(): NativeAuthSnapshot {
  const store = useUserStore()
  return {
    loggedIn: store.isLoggedIn,
    name: store.isLoggedIn ? store.displayName : ''
  }
}

/** 兜底解析：Java 侧拿到的 JSON 串 → 快照；任何异常都归一成"未登录" */
export function normalizeAuthSnapshot(raw: string | null | undefined): NativeAuthSnapshot {
  const fallback: NativeAuthSnapshot = { loggedIn: false, name: '' }
  if (!raw) return fallback
  try {
    const data = JSON.parse(raw) as Partial<NativeAuthSnapshot> | null
    if (!data || typeof data !== 'object') return fallback
    return {
      loggedIn: data.loggedIn === true,
      name: typeof data.name === 'string' ? data.name : ''
    }
  } catch {
    return fallback
  }
}

/** 登录按钮：跳 web 自己的登录页（redirect 回当前页，登录完自动回来） */
export function openWebLogin(): void {
  void router.push({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
}

/** 退出按钮：走 web 的登出流程（后端注销 + 清 token；store 响应式，页面各处即时生效） */
export function webLogout(): void {
  const store = useUserStore()
  void store.logout().catch(() => {})
}

/** 挂 window.__jcAuth 钩子（仅原生壳）；返回值约定同 __jcSetMode —— true 表示钩子在 */
export function installNativeAuthHook(): void {
  if (typeof window === 'undefined' || !isNative()) {
    return
  }
  const w = window as unknown as {
    __jcAuth?: {
      user: () => string
      login: () => boolean
      logout: () => boolean
    }
  }
  w.__jcAuth = {
    user: () => JSON.stringify(authSnapshot()),
    login: () => {
      openWebLogin()
      return true
    },
    logout: () => {
      webLogout()
      return true
    }
  }
}
