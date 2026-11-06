import { onMounted, onUnmounted } from 'vue'

/**
 * 滚动容器通用行为（2026-11-06）：
 *   1. **滚动条自动显隐** —— 滚动中给容器加 `.is-scrolling`（显形），停止 `delay` 毫秒后移除（渐隐）。
 *   2. **可滚状态标记** —— 按实际溢出给容器加/去 `.is-scrollable`，供 CSS 决定是否阻断滚动链。
 *
 * 为什么必须区分「可滚」：`overscroll-behavior: contain` 有个坑 —— 元素**自身没有可滚内容**时
 * 它会把滚轮一并吃掉，表现为「鼠标放在该区域滚，内层不动、外层也不动」（2026-11-06 用户实测）。
 * 所以约定：`.jc-scrollable` 时才上 contain，且**只锁真正滚动的那一轴**
 * （横向条锁 x、纵向容器锁 y，另一轴留 auto，否则竖滚也会被吃）。
 *
 * 用法：
 *   const { onScroll, sync } = useAutoHideScrollbar(() => elRef.value)
 *   - 容器加 `class="jc-scrollauto"`（+ 轴向修饰 `jc-scroll-x` / `jc-scroll-y`）
 *   - 容器上 `@scroll="onScroll"`
 *   - 内容变化（列表变长/变短）后手动调 `sync()`
 */
export function useAutoHideScrollbar(
  getEl?: () => HTMLElement | null | undefined,
  delay = 700
): { onScroll: (e: Event) => void; sync: () => void } {
  let timer: number | null = null
  let ro: ResizeObserver | null = null

  function overflowed(el: HTMLElement): boolean {
    return el.scrollHeight - el.clientHeight > 1 || el.scrollWidth - el.clientWidth > 1
  }

  function apply(el: HTMLElement): void {
    el.classList.toggle('is-scrollable', overflowed(el))
  }

  /** 重新判断可滚状态（挂载 / 尺寸变化 / 内容变化时调用） */
  function sync(): void {
    const el = getEl?.()
    if (el) apply(el)
  }

  function onScroll(e: Event): void {
    const el = e.currentTarget as HTMLElement | null
    if (!el) return
    el.classList.add('is-scrolling')
    // 滚动过程中内容/视口可能变化, 顺带刷新可滚状态
    apply(el)
    if (timer !== null) window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      el.classList.remove('is-scrolling')
      timer = null
    }, delay)
  }

  onMounted(() => {
    sync()
    const el = getEl?.()
    if (el && typeof ResizeObserver !== 'undefined') {
      ro = new ResizeObserver(() => sync())
      ro.observe(el)
    }
  })

  onUnmounted(() => {
    ro?.disconnect()
    ro = null
    if (timer !== null) {
      window.clearTimeout(timer)
      timer = null
    }
  })

  return { onScroll, sync }
}
