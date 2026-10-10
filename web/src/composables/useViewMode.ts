import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'

/**
 * 四档视图模式：mobile / tablet / desktop / tv
 *
 * 触发 TV 模式优先级（高 → 低）：
 *  1. localStorage['jc-mode'] = 'tv'
 *  2. URL ?mode=tv
 *  3. UA 命中 SmartTV / Tizen / WebOS / HbbTV / Hisense / MiTV / Android TV / AFT[A-Z]+
 *  4. 视口 ≥ 1920 且 (hover: none)
 *
 * 否则按视口宽度：< 768 → mobile，768–1023 → tablet，>= 1024 → desktop
 *
 * 写入 <html data-mode="...">，监听 resize 自动切换。
 * 用户手动 setMode 后写 localStorage 持久化（仅 mobile / desktop / tv，不含 tablet）。
 *
 * **原生壳（APK）例外**：壳内 detectMode() 默认强制 tv 且**忽略 jc-mode**（原因见
 * NATIVE_MODE_KEY 注释），只在用户从原生抽屉显式选过模式时尊重 jc-native-mode；
 * 抽屉的切换通过 window.__jcSetMode 钩子下发（installNativeModeHook），先例 window.gfTvBack。
 */

export type ViewMode = 'mobile' | 'tablet' | 'desktop' | 'tv'
export type PersistedMode = 'mobile' | 'desktop' | 'tv'

const STORAGE_KEY = 'jc-mode'
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
 * 原生壳（APK）专用的显示模式键 —— **只有原生抽屉会写**（经 window.__jcSetMode）；
 * 没写过 ⇒ 壳内强制 TV（老行为不变），写过 ⇒ 尊重用户选择（同上，含 reload 后）。
 *
 * 为什么不复用 jc-mode：壳内 detectMode() 刻意忽略它 —— 历史上有两个渠道往这个键写过
 * 垃圾值（浏览器调试、以及旧版抽屉 persistViewMode 直接写它），留在 WebView 的
 * localStorage 里会让 TV 页面永久按桌面布局渲染（[data-mode='desktop'] 下没有
 * overflow-x:hidden，整页横向溢出超出电视屏幕，且用户无法自救）。那个键的来源
 * 不可信，所以另开一个来源唯一的新键。
 */
const NATIVE_MODE_KEY = 'jc-native-mode'

function readNativeOverride(): PersistedMode | null {
  try {
    const v = localStorage.getItem(NATIVE_MODE_KEY)
    if (v === 'tv' || v === 'desktop') {
      return v
    }
  } catch {
    // ignore
  }
  return null
}

/** 用户显式指定过的模式：原生壳看 jc-native-mode（抽屉写），浏览器看 jc-mode（网页写） */
function readExplicitMode(): PersistedMode | null {
  return detectCapacitorAndroid() ? readNativeOverride() : readPersistedMode()
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
  // Native APK 壳 (JerocineNative 注入) 默认强制 TV — 忽略 jc-mode 持久化(原因见 NATIVE_MODE_KEY).
  // 只有用户在原生抽屉里显式选过模式时才尊重 jc-native-mode(抽屉"显示模式"行);
  // URL 参数 ?mode=xxx 仍可临时覆盖, 用于本机调试.
  if (detectCapacitorAndroid()) {
    const urlMode = readUrlMode()
    if (urlMode) return urlMode
    return readNativeOverride() ?? 'tv'
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
 * 归一化原生壳 setter 的入参：'tv' / 'desktop' 透传；其余（'auto' / 空 / 非法 / undefined）
 * 一律当"清除覆盖" ⇒ 回到壳内默认的强制 TV。
 * Java 侧三态循环里的"自动"下发 'auto'（见 SettingsDrawerLogic.jsModeArg）。
 */
export function normalizeNativeMode(value: unknown): PersistedMode | null {
  return value === 'tv' || value === 'desktop' ? value : null
}

/**
 * 原生壳显示模式 setter（由 Java 经 window.__jcSetMode 调用，见 installNativeModeHook）。
 *
 * 与 setMode 的两点差别：
 *  1. 写的是壳专用键 jc-native-mode（理由见 NATIVE_MODE_KEY 注释）；
 *  2. **不 reload** —— mode 是响应式 ref，改完 applyMode 页面即时重排，切换没有白闪。
 */
export function setNativeMode(value: unknown): void {
  const next = normalizeNativeMode(value)
  try {
    if (next === null) {
      localStorage.removeItem(NATIVE_MODE_KEY)
    } else {
      localStorage.setItem(NATIVE_MODE_KEY, next)
    }
  } catch {
    // ignore
  }
  mode.value = next ?? detectMode()
  applyMode(mode.value)
}

/**
 * 挂 window.__jcSetMode 钩子（先例：App.vue 暴露的 window.gfTvBack）——
 * 原生抽屉的"显示模式"行靠它下发 tv / desktop / auto。
 *
 * 只在原生壳里安装：浏览器里没有 Java 调用方，挂上只是多一个无人用的全局。
 * 返回值固定 true，便于 Java 端 evaluateJavascript 回调判断"钩子确实存在"。
 */
export function installNativeModeHook(): void {
  if (typeof window === 'undefined' || !detectCapacitorAndroid()) {
    return
  }
  const w = window as unknown as { __jcSetMode?: (value: unknown) => boolean }
  w.__jcSetMode = (value: unknown): boolean => {
    setNativeMode(value)
    return true
  }
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
  // 原生壳：暴露 window.__jcSetMode 供 Java 抽屉切显示模式（见 installNativeModeHook）
  installNativeModeHook()

  if (typeof window === 'undefined') {
    return
  }

  const onResize = (): void => {
    // 用户显式指定过模式（网页写 jc-mode / 原生抽屉写 jc-native-mode）⇒ 不让 resize 自动改回去
    if (readExplicitMode()) {
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
    if (e.key === STORAGE_KEY || e.key === NATIVE_MODE_KEY) {
      mode.value = detectMode()
      applyMode(mode.value)
    }
  }
  window.addEventListener('storage', onStorage)
  // 不再注册 onScopeDispose；监听器随 window 生命周期存在
}

function setMode(value: PersistedMode | null): void {
  // 原生壳里 jc-mode 被 detectMode 忽略（见 NATIVE_MODE_KEY）—— 直接写会"看起来没生效"，
  // 故在壳内统一走壳专用通道。
  if (detectCapacitorAndroid()) {
    setNativeMode(value)
    return
  }
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
