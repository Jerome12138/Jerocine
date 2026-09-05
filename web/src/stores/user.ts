import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type {
  ChangePasswordPayload,
  LoginPayload,
  UserInfo
} from '@/types/user'
import { clearToken, getToken, setToken } from '@/utils/token'

/**
 * 用户态 store: token / 个人信息 / 登录 / 登出 / 改密.
 *
 * 关键约定:
 *  - 登录走 POST /user/login, 后端 body 直接返回 {userName, token, expires, role};
 *    store 用 body.token 写本地存储, 不再走 new-token 响应头
 *  - 登录后立即拉 GET /user/info 拿到完整 UserInfo (nickName / avatar 等)
 *  - isAdmin 决定后台入口与路由守卫
 *  - 401 由 http 拦截器自行清 token + redirect; 续期场景仍走响应头 new-token
 */
export const useUserStore = defineStore('user', () => {
  const token = ref<string>(getToken())
  const info = ref<UserInfo | null>(null)

  const isLoggedIn = computed(() => token.value.length > 0)

  // 开机已登录: 拉取账号级跳过设置灌入本地(setTokenValue 只覆盖登录/续期, 开机不触发)
  if (token.value) {
    void import('@/composables/useSkipSettings').then(({ hydrateSkipFromAccount }) =>
      hydrateSkipFromAccount()
    )
  }

  /** 是否管理员（role==1） */
  const isAdmin = computed(() => Number(info.value?.role ?? 0) === 1)

  /** 显示用昵称：nickName > userName > username > 默认 */
  const displayName = computed(() => {
    const i = info.value
    return i?.nickName || i?.userName || i?.username || '用户'
  })

  function setTokenValue(t: string, expires?: number): void {
    token.value = t
    if (t) {
      setToken(t, expires)
      // 登录/续期后拉取账号级片头片尾跳过设置灌入本地(跨设备同步)
      void import('@/composables/useSkipSettings').then(({ hydrateSkipFromAccount }) =>
        hydrateSkipFromAccount()
      )
    } else {
      clearToken()
    }
    // 同步给 Android 原生壳 (Jerocine APK), 让 UpdateChecker 带 Authorization 拉灰度版本
    notifyNativeAuth(t)
    // 同步 telemetry userId
    void import('@/utils/telemetry').then(({ telemetry }) => {
      const id = Number(info.value?.id ?? 0) || 0
      telemetry.setUserId(t ? id : 0)
    })
  }

  /** Android 壳 JS Bridge: 把 token 同步给 native (用于自升级灰度); web 端无 bridge 时静默 */
  function notifyNativeAuth(t: string): void {
    if (typeof window === 'undefined') return
    const bridge = (window as unknown as {
      JerocinePlayer?: {
        setAuthToken?: (token: string) => void
        clearAuthToken?: () => void
      }
    }).JerocinePlayer
    if (!bridge) return
    try {
      if (t) bridge.setAuthToken?.(t)
      else bridge.clearAuthToken?.()
    } catch {
      /* native bridge 调失败不影响 web 端流程 */
    }
  }

  /** 登录: 兼容传 username 或 userName; 登录成功后用 body.token 落本地存储, 再拉 /user/info 完善资料 */
  async function login(
    payload: LoginPayload | { username: string; password: string }
  ): Promise<UserInfo> {
    const { login: doLogin } = await import('@/api/auth')
    const userName =
      'userName' in payload ? payload.userName : (payload as { username: string }).username
    const result = await doLogin({ userName, password: payload.password })
    if (!result?.token) {
      throw new Error('登录响应缺少 token 字段')
    }
    setTokenValue(result.token, result.expires)
    try {
      return await fetchInfo()
    } catch {
      // /user/info 失败时, 用 LoginResult 里的字段构造一个最小 UserInfo
      const fallback: UserInfo = {
        userName: result.userName,
        role: result.role
      }
      info.value = fallback
      return fallback
    }
  }

  async function fetchInfo(): Promise<UserInfo> {
    const { getUserInfo } = await import('@/api/auth')
    const res = await getUserInfo()
    info.value = res
    return res
  }

  async function changePassword(payload: ChangePasswordPayload): Promise<void> {
    const { changePassword: doChange } = await import('@/api/auth')
    await doChange(payload)
  }

  async function logout(): Promise<void> {
    try {
      const { logout: doLogout } = await import('@/api/auth')
      await doLogout()
    } catch {
      // 无论后端是否成功，前端都清 token
    } finally {
      info.value = null
      setTokenValue('')
    }
  }

  /** http 拦截器在 401 时调用，避免循环依赖 */
  function clearAuth(): void {
    info.value = null
    setTokenValue('')
  }

  return {
    token,
    info,
    isLoggedIn,
    isAdmin,
    displayName,
    login,
    fetchInfo,
    changePassword,
    setToken: setTokenValue,
    logout,
    clearAuth
  }
})
