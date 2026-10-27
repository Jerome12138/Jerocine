import { watch } from 'vue'
import { getWebSettings, putWebSettings } from '@/api/webSettings'
import {
  AD_FILTER_KEY,
  AUTO_PLAY_NEXT_KEY,
  DANMAKU_KEY,
  FILTER_MODE_KEY,
  RELAY_CACHE_KEY,
  setAdFilter,
  setAutoPlayNext,
  setDanmaku,
  setFilterMode,
  setRelayCache
} from '@/composables/useWebSettings'
import { DANMAKU_SETTINGS_KEY } from '@/composables/useDanmaku'
import type { FilterMode } from '@/composables/useWebSettings'
import { SETTING_CHANGED_EVENT, SETTINGS_APPLIED_EVENT } from '@/utils/settingEvents'
import { useUserStore } from '@/stores/user'

/**
 * Web 设置账号记忆 (2026-10-27):
 *  - 未登录: 仅本地 localStorage(现状不变)。
 *  - 登录后: 启动/登录时从 /me/web-settings 拉取云端快照合并到本地(云端为准),
 *    之后任何设置变更(各 setter 持久化时派发 `jc-setting-changed`)防抖上报云端。
 *  - 云端是 localStorage 指定 key 的镜像(字符串原样); 服务端只存不解释,
 *    新设置项把 key 加进 SYNC_KEYS 即随账号记忆。
 *  - 防回环: apply(拉取合并)期间忽略 changed 事件; 幂等, 多一次上报也无碍。
 */

/** 参与账号记忆的 localStorage key(值按字符串原样镜像) */
const SYNC_KEYS: readonly string[] = [
  AD_FILTER_KEY,
  FILTER_MODE_KEY,
  RELAY_CACHE_KEY,
  AUTO_PLAY_NEXT_KEY,
  DANMAKU_KEY,
  DANMAKU_SETTINGS_KEY
]

/** 变更通知事件名与 applied 事件名见 @/utils/settingEvents */

const PUT_DEBOUNCE_MS = 2_000

let started = false
let applying = false
let putTimer: number | undefined

function readKey(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeKey(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* 隐私模式忽略 */
  }
}

/** 本地 → 云端快照(字符串原样) */
function snapshot(): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const key of SYNC_KEYS) {
    const v = readKey(key)
    if (v !== null) out[key] = v
  }
  return out
}

/** 云端 → 本地: 覆盖差异 key, 同步响应式状态, 有变化则广播 applied */
function apply(data: Record<string, unknown>): void {
  let changed = false
  for (const key of SYNC_KEYS) {
    const cloud = data[key]
    if (typeof cloud !== 'string') continue // 云端没有的 key 不动(本地未登录期的状态保留)
    if (cloud === readKey(key)) continue
    writeKey(key, cloud)
    changed = true
    // 同步模块级单例 ref(setter 会再 persist 一次同值, 幂等无害)
    if (key === DANMAKU_KEY) setDanmaku(cloud === '1')
    else if (key === AD_FILTER_KEY) setAdFilter(cloud === '1')
    else if (key === RELAY_CACHE_KEY) setRelayCache(cloud === '1')
    else if (key === AUTO_PLAY_NEXT_KEY) setAutoPlayNext(cloud === '1')
    else if (key === FILTER_MODE_KEY) setFilterMode(cloud as FilterMode)
    // jc-danmaku-settings 由 applied 事件 → useDanmaku 重载(响应式 ref 无法感知第三方写入 localStorage)
  }
  if (changed) {
    window.dispatchEvent(new CustomEvent(SETTINGS_APPLIED_EVENT))
  }
}

function schedulePut(): void {
  if (putTimer !== undefined) window.clearTimeout(putTimer)
  putTimer = window.setTimeout(() => {
    putTimer = undefined
    const { isLoggedIn } = useUserStore()
    if (!isLoggedIn) return
    void putWebSettings(snapshot()).catch(() => {
      /* 云端失败不影响本地, 下次变更再试 */
    })
  }, PUT_DEBOUNCE_MS)
}

/** 登录态挂接: 在 App.vue 挂载一次(模块级 started 防重入)。 */
export function startCloudSettingsSync(): void {
  if (started || typeof window === 'undefined') return
  started = true
  const user = useUserStore()
  // 登录态可用(含刷新时已带 token)即拉取合并
  const pull = (): void => {
    if (!user.isLoggedIn) return
    void getWebSettings()
      .then((resp) => {
        applying = true
        try {
          apply((resp?.data ?? {}) as Record<string, unknown>)
        } finally {
          applying = false
        }
      })
      .catch(() => {}) // 云端不可用 → 纯本地, 不打扰
  }
  pull()
  watch(
    () => user.token,
    () => pull()
  )
  // 本地设置变更 → 防抖上报(apply 期间忽略, 防回环)
  window.addEventListener(SETTING_CHANGED_EVENT, () => {
    if (applying) return
    schedulePut()
  })
  // 跨标签页同步(同浏览器其他标签改了设置)
  window.addEventListener('storage', (e) => {
    if (e.key && SYNC_KEYS.includes(e.key)) schedulePut()
  })
}
