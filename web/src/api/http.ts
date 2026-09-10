import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosResponse,
  type InternalAxiosRequestConfig
} from 'axios'
import { ApiError, type Problem } from '@/types/api'
import { logger } from '@/utils/logger'
import { useUserStore } from '@/stores/user'
import { useUIStore } from '@/stores/ui'

declare module 'axios' {
  export interface AxiosRequestConfig {
    silent?: boolean
  }
  export interface InternalAxiosRequestConfig {
    silent?: boolean
    __pushed?: boolean
  }
}

type ToastType = 'success' | 'error' | 'info' | 'warning'
interface ToastApi {
  push: (type: ToastType, msg: string) => void
}

let toastApi: ToastApi | null = null

export function registerToast(api: ToastApi): void {
  toastApi = api
}

function toastError(msg: string): void {
  if (toastApi) {
    toastApi.push('error', msg)
  } else if (import.meta.env.DEV) {
    logger.error('[toast]', msg)
  }
}

export function toast(type: ToastType, msg: string): void {
  if (toastApi) {
    toastApi.push(type, msg)
  }
}

/** axios 实例: baseURL = (VITE_API_BASE || '/api') + '/v1' */
export const http: AxiosInstance = axios.create({
  baseURL: (import.meta.env.VITE_API_BASE || '/api') + '/v1',
  timeout: 80_000,
  headers: { 'Content-Type': 'application/json' }
})

function safePop(config: InternalAxiosRequestConfig | undefined): void {
  if (!config?.__pushed) return
  try {
    useUIStore().popLoading()
  } catch {
    /* pinia 未就绪 */
  }
  config.__pushed = false
}

/** ===== 请求拦截器 ===== */
http.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    try {
      const userStore = useUserStore()
      if (userStore.token) {
        config.headers.set('Authorization', `Bearer ${userStore.token}`)
      }
      if (!config.silent) {
        useUIStore().pushLoading()
        config.__pushed = true
      }
      ;(config as InternalAxiosRequestConfig & { __startedAt?: number }).__startedAt = Date.now()
    } catch (e) {
      logger.warn('http request interceptor pre-init', e)
    }
    // 清理空查询参数
    if (config.params && typeof config.params === 'object') {
      const cleaned: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(config.params)) {
        if (v !== undefined && v !== null && v !== '') {
          cleaned[k] = v
        }
      }
      config.params = cleaned
    }
    return config
  },
  (error) => Promise.reject(error)
)

/** 主动取消(AbortController / AbortSignal)判定。
 *  用 boolean 包装而非直接内联 axios.isCancel: 后者是类型谓词(value is CanceledError),
 *  而 CanceledError 是 AxiosError 的子类型, 直接判会把形参收窄成 never, 后续取 error.config
 *  等属性全部报 TS2339。 */
function isCanceled(err: unknown): boolean {
  return axios.isCancel(err) || (err as AxiosError | undefined)?.code === 'ERR_CANCELED'
}

/** ===== 响应拦截器 =====
 * 成功 (2xx): 直接返回 resp.data(新契约无信封)。204 → undefined。
 * 失败: 由 error 分支抛 ApiError。
 * 业务侧 `http.get<unknown, T>(...)` → 解析为 T。
 */
http.interceptors.response.use(
  ((resp: AxiosResponse) => {
    safePop(resp.config as InternalAxiosRequestConfig)
    trackApi(resp.config, resp.status)
    // 写操作成功轻提示(silent 关闭)
    try {
      const method = (resp.config.method ?? 'get').toLowerCase()
      const isWrite = ['post', 'put', 'patch', 'delete'].includes(method)
      if (isWrite && !resp.config.silent && toastApi) {
        toastApi.push('success', '操作成功')
      }
    } catch { /* ignore */ }
    return resp.status === 204 ? undefined : resp.data
  }) as unknown as (resp: AxiosResponse) => Promise<AxiosResponse>,
  async (error: AxiosError<Problem>) => {
    // 主动取消(AbortController / useAbortable.refresh)不是"加载失败": 原样抛出, 不上报、不弹提示。
    // 必须先于下面的包装 —— 否则会被包成 ApiError(name='ApiError'),
    // 各页面 `e.name === 'CanceledError'` 的守卫失效, 把"被新请求取代的旧请求"
    // 误判成失败(表现为: 点筛选/翻页后偶发"加载失败", 刷新又正常)。
    if (isCanceled(error)) {
      safePop(error.config as InternalAxiosRequestConfig | undefined)
      return Promise.reject(error)
    }
    safePop(error.config as InternalAxiosRequestConfig | undefined)
    const status = error.response?.status ?? 0
    trackApi(error.config, status || 599)
    const problem = error.response?.data
    const apiErr = new ApiError(status, problem)
    await handleHttpError(error.config?.silent, status, apiErr.message)
    return Promise.reject(apiErr)
  }
)

function trackApi(config: InternalAxiosRequestConfig | undefined, status: number): void {
  try {
    const url = config?.url ?? ''
    if (url.includes('/telemetry/')) return
    const started = (config as (InternalAxiosRequestConfig & { __startedAt?: number }) | undefined)?.__startedAt
    const dur = started ? Date.now() - started : 0
    void import('@/utils/telemetry').then(({ telemetry }) => telemetry.trackApi(url, dur, status))
  } catch { /* ignore */ }
}

/** 全局 HTTP 错误处理(按真实 HTTP status 单一判定) */
async function handleHttpError(silent: boolean | undefined, status: number, msg: string): Promise<void> {
  if (status === 401) {
    try {
      useUserStore().clearAuth()
      const { default: router } = await import('@/router')
      const cur = router.currentRoute.value
      if (cur.path !== '/login' && !silent) {
        await router.replace({ path: '/login', query: { redirect: cur.fullPath } })
      }
    } catch (e) {
      logger.warn('401 redirect failed', e)
    }
    if (!silent) toastError(msg || '登录已过期，请重新登录')
    return
  }
  if (status === 403) {
    if (!silent) toastError(msg || '权限不足，仅管理员可操作')
    return
  }
  if (status === 429) {
    if (!silent) toastError('请求过于频繁，请稍后再试')
    return
  }
  if (!silent) toastError(msg || '服务器繁忙，请稍后再试')
}
