import { onUnmounted } from 'vue'

/**
 * 滚动条自动显隐：滚动中显形，停止 `delay` 毫秒后收起。
 *
 * 与侧栏 `PublicHeader` 的 `.jc-mnav__body` 内联实现同款语义（那边是 2026-10-11 用户要求），
 * 这里抽成可复用 composable，避免在选集网格 / 选源横向条 / 管理后台主区各抄一遍。
 *
 * 用法：
 *   - 滚动容器加 `class="jc-scrollauto"`（样式见 theme.css，负责 0↔6px / none↔thin 的切换）
 *   - 容器上 `@scroll="onScroll"`（事件对象里取 currentTarget，无需传 ref）
 *
 * 说明：本模块只负责加/删 `.is-scrolling`，具体显隐完全交给 CSS；
 * 定时器在组件卸载时清理，避免离场后仍操作已卸载的节点。
 */
export function useAutoHideScrollbar(delay = 700): { onScroll: (e: Event) => void } {
  let timer: number | null = null

  function onScroll(e: Event): void {
    const el = e.currentTarget as HTMLElement | null
    if (!el) return
    el.classList.add('is-scrolling')
    if (timer !== null) window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      el.classList.remove('is-scrolling')
      timer = null
    }, delay)
  }

  onUnmounted(() => {
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
  })

  return { onScroll }
}
