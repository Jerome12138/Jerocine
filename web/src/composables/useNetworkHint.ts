import { ref, onMounted, onUnmounted, computed, type Ref, type ComputedRef } from 'vue'

/**
 * 弱网感知 composable.
 *
 * 读 navigator.connection (Network Information API) 给出"是否弱网"的判断,
 * 用于播放器/资源加载做差异化策略.
 *
 * 浏览器支持:
 *  - Chromium 系全支持
 *  - Firefox 不支持 (返回 undefined → isSlow=false, 走默认网速)
 *  - Safari 不支持
 * 不支持时默认按"非弱网"处理, 不影响主路径.
 *
 * 用户重写:
 *  - localStorage 里写 'gf-network-pref': 'slow' / 'fast' / 'auto' 可强制
 *    (用户在弱网设备但 connection API 误报时, 给一条手动通道)
 */

type EffectiveType = 'slow-2g' | '2g' | '3g' | '4g' | undefined

interface NetworkInfo {
  effectiveType?: EffectiveType
  downlink?: number // Mbps
  rtt?: number // ms
  saveData?: boolean
}

interface NavigatorConn extends NetworkInfo {
  addEventListener: (event: 'change', listener: () => void) => void
  removeEventListener: (event: 'change', listener: () => void) => void
}

function readConnection(): NavigatorConn | null {
  if (typeof navigator === 'undefined') return null
  // Chromium 用 connection, 老 Edge 用 mozConnection / webkitConnection
  const nav = navigator as unknown as {
    connection?: NavigatorConn
    mozConnection?: NavigatorConn
    webkitConnection?: NavigatorConn
  }
  return nav.connection || nav.mozConnection || nav.webkitConnection || null
}

function readManualPref(): 'slow' | 'fast' | 'auto' {
  try {
    const v = localStorage.getItem('gf-network-pref')
    if (v === 'slow' || v === 'fast') return v
  } catch {
    /* ignore */
  }
  return 'auto'
}

export interface NetworkHint {
  /** 浏览器给的 effectiveType, 不支持时 undefined */
  effectiveType: Ref<EffectiveType>
  /** 是否当前应当走弱网策略. 综合 effectiveType + saveData + 用户手动偏好. */
  isSlow: ComputedRef<boolean>
  /** 用户手动覆盖, 'slow' / 'fast' / 'auto'. 写 localStorage 持久化. */
  manualPref: Ref<'slow' | 'fast' | 'auto'>
  setManualPref: (v: 'slow' | 'fast' | 'auto') => void
}

export function useNetworkHint(): NetworkHint {
  const effectiveType = ref<EffectiveType>(undefined)
  const saveData = ref<boolean>(false)
  const manualPref = ref<'slow' | 'fast' | 'auto'>(readManualPref())

  let conn: NavigatorConn | null = null

  function sync(): void {
    if (!conn) return
    effectiveType.value = conn.effectiveType
    saveData.value = !!conn.saveData
  }

  onMounted(() => {
    conn = readConnection()
    sync()
    conn?.addEventListener('change', sync)
  })

  onUnmounted(() => {
    conn?.removeEventListener('change', sync)
  })

  const isSlow = computed<boolean>(() => {
    if (manualPref.value === 'slow') return true
    if (manualPref.value === 'fast') return false
    // auto: navigator 判定
    if (saveData.value) return true
    const t = effectiveType.value
    return t === 'slow-2g' || t === '2g' || t === '3g'
  })

  function setManualPref(v: 'slow' | 'fast' | 'auto'): void {
    manualPref.value = v
    try {
      if (v === 'auto') localStorage.removeItem('gf-network-pref')
      else localStorage.setItem('gf-network-pref', v)
    } catch {
      /* ignore */
    }
  }

  return { effectiveType, isSlow, manualPref, setManualPref }
}
