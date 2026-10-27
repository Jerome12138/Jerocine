import { ref } from 'vue'
import { AUTO_PLAY_NEXT_KEY } from '@/utils/nativePlay'

export { AUTO_PLAY_NEXT_KEY }

/**
 * Web 端统一设置（2026-10-18）—— 本模块是这些开关的**单一数据源**：
 *
 *   - adFilter     广告过滤（jc-ad-filter, 默认开）
 *   - filterMode   广告过滤取源方式（jc-filter-mode, 默认 client）
 *   - relayCache   全量中转（jc-relay-cache, 默认关）
 *   - autoPlayNext 自动连播（jerocine:autoPlayNext, 默认开）
 *
 * 数据一律存 web localStorage（改的是同一份，壳内外行为一致）：
 *   - 纯浏览器：`/settings` 配置页读写；
 *   - 安卓 web 壳：设置抽屉经 `window.__jcConfig` 钩子读写（Java 侧契约见
 *     player-core 的 WebSettingsContract，快照格式 `k=v;k=v`）；
 *   - 播放页等消费方直接用本模块的响应式 ref —— 抽屉/配置页改动即刻生效，
 *     无需刷新（PlayView 对 adFilter/filterMode 的 watch 会自动重算播放源）。
 *
 * filterMode 语义（服务器在境外，取源慢）：
 *   - client（默认）：端侧抓 m3u8 → POST /v1/m3u8/filter 服务器只做文本过滤不联网；
 *   - server：整条链路走服务端 /v1/m3u8/proxy（服务器抓源+过滤），仅端侧被
 *     地域封等场景手动开启。
 */

export type FilterMode = 'client' | 'server'

export const AD_FILTER_KEY = 'jc-ad-filter'
export const FILTER_MODE_KEY = 'jc-filter-mode'
export const RELAY_CACHE_KEY = 'jc-relay-cache'
export const DANMAKU_KEY = 'jc-danmaku'

export const FILTER_MODES: readonly FilterMode[] = ['client', 'server']

function readBool(key: string, def: boolean): boolean {
  try {
    const v = localStorage.getItem(key)
    if (v === null) return def
    return v === '1'
  } catch {
    return def
  }
}

function readFilterMode(): FilterMode {
  try {
    const v = localStorage.getItem(FILTER_MODE_KEY)
    return v === 'server' ? 'server' : 'client'
  } catch {
    return 'client'
  }
}

/* 模块级单例 ref：全站共享同一份状态（含 window.__jcConfig 钩子写入） */
const adFilter = ref<boolean>(readBool(AD_FILTER_KEY, true))
const filterMode = ref<FilterMode>(readFilterMode())
const relayCache = ref<boolean>(readBool(RELAY_CACHE_KEY, false))
const autoPlayNext = ref<boolean>(
  readBool(AUTO_PLAY_NEXT_KEY, true) // 旧约定: 缺省视为开(nativePlay.readAutoPlayNext 同口径)
)
/* 弹幕开关(P0): 与播放页/设置页共用; 不进 webSettingsSnapshot(快照契约对应原生抽屉, 弹幕不在抽屉范围)。
   ⚠️ 2026-10-26 改为默认关闭（用户级/本地级）: 关 = 不调弹幕获取接口; 需要时在设置页打开, 播放器弹幕设置里可对本集再开。 */
const danmaku = ref<boolean>(readBool(DANMAKU_KEY, false))

function persist(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* 隐私模式忽略 */
    return
  }
  // 账号记忆: 设置持久化即通知云同步(useCloudSettings 防抖上报 /me/web-settings)
  try {
    window.dispatchEvent(new CustomEvent('jc-setting-changed', { detail: { key } }))
  } catch {
    /* ignore */
  }
}

export function setAdFilter(v: boolean): void {
  adFilter.value = v
  persist(AD_FILTER_KEY, v ? '1' : '0')
}

export function setFilterMode(m: FilterMode): void {
  filterMode.value = m
  persist(FILTER_MODE_KEY, m)
}

export function setRelayCache(v: boolean): void {
  relayCache.value = v
  persist(RELAY_CACHE_KEY, v ? '1' : '0')
}

export function setAutoPlayNext(v: boolean): void {
  autoPlayNext.value = v
  persist(AUTO_PLAY_NEXT_KEY, v ? '1' : '0')
}

export function setDanmaku(v: boolean): void {
  danmaku.value = v
  persist(DANMAKU_KEY, v ? '1' : '0')
}

export function useWebSettings() {
  return {
    adFilter,
    filterMode,
    relayCache,
    autoPlayNext,
    danmaku,
    setAdFilter,
    setFilterMode,
    setRelayCache,
    setAutoPlayNext,
    setDanmaku
  }
}

/* ============ 安卓壳桥接（k=v; 快照契约, 与 player-core WebSettingsContract 对应） ============ */

/** 快照格式: "adFilter=1;filterMode=client;relayCache=0;autoPlayNext=1"（值均为白名单枚举） */
export function webSettingsSnapshot(): string {
  return [
    `adFilter=${adFilter.value ? '1' : '0'}`,
    `filterMode=${filterMode.value}`,
    `relayCache=${relayCache.value ? '1' : '0'}`,
    `autoPlayNext=${autoPlayNext.value ? '1' : '0'}`
  ].join(';')
}

/** 校验并应用一项设置（key/值白名单校验, 非法输入一律拒绝并返回 false）。 */
export function applyWebSetting(key: string, value: string): boolean {
  switch (key) {
    case 'adFilter':
      if (value === '1' || value === 'true') setAdFilter(true)
      else if (value === '0' || value === 'false') setAdFilter(false)
      else return false
      return true
    case 'relayCache':
      if (value === '1' || value === 'true') setRelayCache(true)
      else if (value === '0' || value === 'false') setRelayCache(false)
      else return false
      return true
    case 'autoPlayNext':
      if (value === '1' || value === 'true') setAutoPlayNext(true)
      else if (value === '0' || value === 'false') setAutoPlayNext(false)
      else return false
      return true
    case 'filterMode':
      if (value === 'client' || value === 'server') {
        setFilterMode(value)
        return true
      }
      return false
    default:
      return false
  }
}

/**
 * 安卓 web 壳设置抽屉的读写钩子（WebShellActivity 经 evaluateJavascript 调用）：
 *   getAll() -> "k=v;k=v" 快照;  set(key, value) -> 是否成功。
 * 纯浏览器环境也安装（无害, 且便于调试）。
 */
export function installWebSettingsHook(): void {
  ;(window as unknown as Record<string, unknown>).__jcConfig = {
    getAll: (): string => webSettingsSnapshot(),
    set: (key: unknown, value: unknown): boolean =>
      applyWebSetting(String(key ?? ''), String(value ?? ''))
  }
}
