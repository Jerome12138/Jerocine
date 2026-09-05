/**
 * Capacitor 兜底垫片。
 *
 * 为什么需要：
 *  本 APK 以「远程加载」方式运行（MainActivity loadUrl https://jerocine.art），
 *  Capacitor 的 native-bridge.js / window.Capacitor 仅在本地 capacitor:// 资源下注入，
 *  远程页面里不存在。但 Capacitor/Cordova 壳在 App 生命周期（pause / resume）会执行
 *    webView.evaluateJavascript("window.Capacitor.triggerEvent('pause','document')")
 *  远程页无 window.Capacitor → Uncaught TypeError: Cannot read properties of undefined
 *  (reading 'triggerEvent') → 被 MainActivity.onConsoleMessage 抓出来弹 Toast 打扰用户。
 *
 * 垫片职责：保证 window.Capacitor.triggerEvent 存在且为 no-op。
 *  - 绝不覆盖真实 Capacitor（本地 capacitor:// 模式下 native-bridge 先注入，
 *    我们用「缺失才补」策略，真实对象与其方法保持原样）。
 *  - 幂等：重复调用不替换已装的 no-op。
 *  - SSR / win 缺失安全。
 *
 * 仅消除「无害但烦人」的生命周期事件报错；不改变任何业务行为。
 */

type CapacitorLike = {
  triggerEvent?: (...args: unknown[]) => unknown
  [key: string]: unknown
}

export function installCapacitorShim(
  win: (Window & typeof globalThis) | undefined = typeof window !== 'undefined' ? window : undefined
): void {
  if (!win) {
    return
  }
  const w = win as unknown as { Capacitor?: CapacitorLike }
  if (!w.Capacitor) {
    // 远程加载场景：完全没有 Capacitor，造一个最小垫片
    w.Capacitor = { triggerEvent: noop }
    return
  }
  // 已有 Capacitor（真实注入或上次垫片）：仅当 triggerEvent 缺失时补 no-op，绝不覆盖真实实现
  if (typeof w.Capacitor.triggerEvent !== 'function') {
    w.Capacitor.triggerEvent = noop
  }
}

function noop(): undefined {
  return undefined
}
