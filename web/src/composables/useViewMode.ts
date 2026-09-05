import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'

/**
 * 四档视图模式：mobile / tablet / desktop / tv
 *
 * 触发 TV 模式优先级（高 → 低）：
 *  1. localStorage['gf-mode'] = 'tv'
 *  2. URL ?mode=tv
 *  3. UA 命中 SmartTV / Tizen / WebOS / HbbTV / Hisense / MiTV / Android TV / AFT[A-Z]+
 *  4. 视口 ≥ 1920 且 (hover: none)
 *
 * 否则按视口宽度：< 768 → mobile，768–1023 → tablet，>= 1024 → desktop
 *
 * 写入 <html data-mode="...">，监听 resize 自动切换。
 * 用户手动 setMode 后写 localStorage 持久化（仅 mobile / desktop / tv，不含 tablet）。
 */

export type ViewMode = 'mobile' | 'tablet' | 'desktop' | 'tv'
export type PersistedMode = 'mobile' | 'desktop' | 'tv'

const STORAGE_KEY = 'gf-mode'
const TV_UA_REGEX =
  /SmartTV|Tizen|WebOS|HbbTV|Hisense|MiTV|Android TV|AFT[A-Z]+|GoogleTV|AppleTV|BRAVIA|VIDAA/i

let installed = false
const mode = ref<ViewMode>('desktop')

function readUrlMode(): ViewMode | null {
  if (typeof window === 'undefined') {
    return null
  }
  const sp = new URLSearchParams(window.location.search)
  const v = sp.get('mode')
  if (v === 'tv' || v === 'mobile' || v === 'desktop') {
    return v
  }
  return null
}

function readPersistedMode(): PersistedMode | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'tv' || v === 'mobile' || v === 'desktop') {
      return v
    }
  } catch {
    // ignore
  }
  return null
}

/**
 * 原生壳判定：优先级
 *   1. window.JerocineNative / JerocinePlayer (我们 addJavascriptInterface 注入,
 *      远程 origin 也存在 — 最可靠)
 *   2. window.Capacitor.getPlatform() (Capacitor 注入, 远程加载时通常不注入,
 *      仅本地 assets 走 capacitor:// 协议时有)
 */
function detectCapacitorAndroid(): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  const w = window as unknown as {
    JerocineNative?: unknown
    JerocinePlayer?: unknown
    Capacitor?: {
      platform?: string
      isNativePlatform?: () => boolean
      getPlatform?: () => string
    }
  }
  // 我们自己的 JS Bridge — 最可靠 (远程 SPA 也有)
  if (w.JerocineNative || w.JerocinePlayer) {
    return true
  }
  const cap = w.Capacitor
  if (!cap) {
    return false
  }
  if (typeof cap.getPlatform === 'function') {
    return cap.getPlatform() === 'android'
  }
  return cap.platform === 'android'
}

function detectTV(): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  // Capacitor Android 壳：默认按 TV 渲染（用户后续可手动 setMode 覆盖）
  if (detectCapacitorAndroid()) {
    return true
  }
  if (TV_UA_REGEX.test(navigator.userAgent)) {
    return true
  }
  const w = window.innerWidth
  if (w >= 1920) {
    const noHover = window.matchMedia?.('(hover: none)').matches ?? false
    if (noHover) {
      return true
    }
  }
  return false
}

function detectMode(): ViewMode {
  // Native APK 壳 (JerocineNative 注入) 优先强制 TV — 忽略 localStorage 持久化.
  // 原因: APK 在物理 TV 上跑, 用户可能调试时点过"切桌面模式"留下 gf-mode='desktop'
  // 这个值, 之后没法自动恢复 TV 模式. desktop 模式下 [data-mode='tv'] 的
  // overflow-x:hidden / max-width:100vw 不会生效 → 整页横向溢出超出电视屏幕.
  // URL 参数 ?mode=xxx 仍可临时覆盖, 用于本机调试.
  if (detectCapacitorAndroid()) {
    const urlMode = readUrlMode()
    if (urlMode) return urlMode
    return 'tv'
  }
  // 非 APK (浏览器访问): 仍按 persisted > URL > auto
  const persisted = readPersistedMode()
  if (persisted) {
    return persisted
  }
  const urlMode = readUrlMode()
  if (urlMode) {
    return urlMode
  }
  if (detectTV()) {
    return 'tv'
  }
  if (typeof window === 'undefined') {
    return 'desktop'
  }
  const w = window.innerWidth
  if (w < 768) return 'mobile'
  if (w < 1024) return 'tablet'
  return 'desktop'
}

function applyMode(value: ViewMode): void {
  if (typeof document === 'undefined') {
    return
  }
  document.documentElement.setAttribute('data-mode', value)
}

/**
 * 在 main.ts 全局调用一次，绑定 window 级 resize / storage 监听到 app 生命周期。
 * 避免在某个 component 的 setup 内 install 导致组件卸载后 resize 失联。
 */
export function installViewMode(): void {
  if (installed) {
    return
  }
  installed = true
  mode.value = detectMode()
  applyMode(mode.value)

  if (typeof window === 'undefined') {
    return
  }

  const onResize = (): void => {
    const persisted = readPersistedMode()
    if (persisted) {
      return
    }
    if (mode.value === 'tv' && detectTV()) {
      return
    }
    const w = window.innerWidth
    const next: ViewMode =
      detectTV() ? 'tv' : w < 768 ? 'mobile' : w < 1024 ? 'tablet' : 'desktop'
    if (mode.value !== next) {
      mode.value = next
      applyMode(next)
    }
  }
  window.addEventListener('resize', onResize)

  const onStorage = (e: StorageEvent): void => {
    if (e.key === STORAGE_KEY) {
      mode.value = detectMode()
      applyMode(mode.value)
    }
  }
  window.addEventListener('storage', onStorage)
  // 不再注册 onScopeDispose；监听器随 window 生命周期存在
}

function setMode(value: PersistedMode | null): void {
  try {
    if (value === null) {
      localStorage.removeItem(STORAGE_KEY)
    } else {
      localStorage.setItem(STORAGE_KEY, value)
    }
  } catch {
    // ignore
  }
  mode.value = value ?? detectMode()
  applyMode(mode.value)
}

export function useViewMode(): {
  mode: Ref<ViewMode>
  setMode: (value: PersistedMode | null) => void
  isMobile: ComputedRef<boolean>
  isTablet: ComputedRef<boolean>
  isDesktop: ComputedRef<boolean>
  isTV: ComputedRef<boolean>
  isNarrow: ComputedRef<boolean>
} {
  // 兜底：如果 main.ts 还没 installViewMode（例如单测/SSR），首次访问时按一次性安装
  if (!installed) {
    installViewMode()
  }

  // 保险起见，watch mode 时再次同步 DOM
  watch(mode, applyMode, { immediate: true })

  return {
    mode,
    setMode,
    isMobile: computed(() => mode.value === 'mobile'),
    isTablet: computed(() => mode.value === 'tablet'),
    isDesktop: computed(() => mode.value === 'desktop'),
    isTV: computed(() => mode.value === 'tv'),
    isNarrow: computed(() => mode.value === 'mobile' || mode.value === 'tablet')
  }
}
