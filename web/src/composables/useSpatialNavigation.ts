/**
 * 空间导航 composable（TV 模式专用）
 *
 * 行为：
 *  - 仅在 <html data-mode="tv"> 下生效（非 tv 模式时 disable）
 *  - 监听全局 keydown，把方向键转化为 [data-focusable="true"] 元素之间的几何最近邻焦点切换
 *  - Enter / Space 在非 button/a/input 元素聚焦时主动派发 click（让原生处理）
 *  - Escape / Backspace（KeyCode 4 已被 dpad.ts 映射为 Escape）触发 router.back()
 *  - 路由切换时：旧路由焦点元素的稳定 ID 写 sessionStorage；新路由进入后尝试恢复，失败则聚焦第一个 focusable
 *  - D-pad keyCode 兼容已由 dpad.ts 在 main.ts 装载的 installDpadBridge 处理（派发标准 KeyboardEvent）
 *
 * 暴露：focusFirst / focusElement / enable / disable
 *
 * 使用：在 App.vue setup 内调用一次 useSpatialNavigation()
 *
 * 参考：04-tv-addendum.md 第 3 节
 */

import { onScopeDispose, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useViewMode } from '@/composables/useViewMode'
import { normalizeDpadKey } from '@/utils/dpad'

const FOCUSABLE_SELECTOR = '[data-focusable="true"]:not([disabled]):not([aria-hidden="true"])'
const FOCUS_MEMORY_PREFIX = 'gf-tv-focus:'
const FOCUS_RESTORE_DELAY = 50

interface Rect {
  el: HTMLElement
  cx: number
  cy: number
  top: number
  left: number
  right: number
  bottom: number
}

interface Api {
  focusFirst: (container?: HTMLElement | null) => boolean
  focusElement: (el: HTMLElement | null) => boolean
  enable: () => void
  disable: () => void
}

/** 空间导航对一次按键的决策结果 */
export type SpatialAction =
  | { type: 'move'; dir: 'up' | 'down' | 'left' | 'right' }
  | { type: 'activate'; el: HTMLElement }
  | { type: 'back' }
  | { type: 'none' }

/**
 * 纯函数：根据按键 + 事件可信度 + 当前焦点上下文，决定空间导航动作。
 * 抽出来便于单测，并集中两处 TV 体验修复的判定逻辑：
 *
 *  1. 「选不中」：D-pad 经 dpad.ts 合成派发的 Enter 是 untrusted 事件，
 *     Chromium 不会用它激活 <button>/<a>。故 untrusted Enter 落在原生可点击元素上时
 *     返回 activate（调用方 el.click() 程序化激活）；trusted Enter 交给浏览器原生激活（none）。
 *  2. 「焦点乱跳」：方向键一律返回 move（调用方据此无条件 preventDefault 消费），
 *     避免 WebView 内置焦点引擎与本空间导航双重移动焦点。
 *
 * @param key   已 normalizeDpadKey 归一化后的 key
 * @param trusted  KeyboardEvent.isTrusted（物理键盘=true，dpad 合成派发=false）
 */
export function decideSpatialAction(
  key: string,
  trusted: boolean,
  ctx: { currentFocusable: HTMLElement | null; modalOpen: boolean }
): SpatialAction {
  switch (key) {
    case 'ArrowUp':
      return { type: 'move', dir: 'up' }
    case 'ArrowDown':
      return { type: 'move', dir: 'down' }
    case 'ArrowLeft':
      return { type: 'move', dir: 'left' }
    case 'ArrowRight':
      return { type: 'move', dir: 'right' }
    case 'Enter':
    case ' ':
    case 'Spacebar': {
      const el = ctx.currentFocusable
      if (!el) return { type: 'none' }
      const tag = el.tagName?.toLowerCase()
      // 编辑控件：交给输入框 / IME，不代为激活
      if (tag === 'input' || tag === 'textarea' || tag === 'select') return { type: 'none' }
      // 原生可点击元素 + 可信事件：浏览器会自然激活，避免重复 click
      const nativelyActivates = tag === 'button' || tag === 'a'
      if (nativelyActivates && trusted) return { type: 'none' }
      // 其余情况（含 D-pad 合成的 untrusted Enter、普通 focusable div）：程序化激活
      return { type: 'activate', el }
    }
    case 'Escape':
    case 'Backspace':
      // 弹窗打开时交给弹窗自己关，不跳页
      if (ctx.modalOpen) return { type: 'none' }
      return { type: 'back' }
    default:
      return { type: 'none' }
  }
}

let installed = false

/** 元素是否在视口内（中心点判定） */
function isVisible(el: HTMLElement): boolean {
  const r = el.getBoundingClientRect()
  if (r.width === 0 || r.height === 0) return false
  const style = window.getComputedStyle(el)
  if (style.visibility === 'hidden' || style.display === 'none') return false
  return true
}

/** 当前有活跃 modal (aria-modal=true) 时返回它, 把 spatial nav 限制在 modal 内, 否则
 *  弹层后面的按钮也会被 D-pad 选到 (用户报 "X 不能选" "隔一个被选" 实际就是 nav
 *  飘出了 sidebar 范围). */
function getModalContainer(): HTMLElement | null {
  const modals = document.querySelectorAll<HTMLElement>('[aria-modal="true"]')
  if (modals.length === 0) return null
  for (let i = modals.length - 1; i >= 0; i--) {
    const m = modals[i]
    if (m && isVisible(m)) return m
  }
  return null
}

/** 拿当前 DOM 中所有可见 focusable. 无显式 container 时若有 modal 则自动 scope 到 modal */
function getCandidates(container?: HTMLElement | null): HTMLElement[] {
  const root = container ?? getModalContainer() ?? document.body
  const list = Array.from(
    root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
  )
  return list.filter(isVisible)
}

function rectOf(el: HTMLElement): Rect {
  const r = el.getBoundingClientRect()
  return {
    el,
    cx: r.left + r.width / 2,
    cy: r.top + r.height / 2,
    top: r.top,
    left: r.left,
    right: r.right,
    bottom: r.bottom
  }
}

/** 最近的 [data-focus-zone] 祖先(zone 软边界用); 无则 null。 */
export function getZoneContainer(el: HTMLElement | null): HTMLElement | null {
  if (!el) return null
  return el.closest<HTMLElement>('[data-focus-zone]')
}

/**
 * 在指定方向上寻找最近邻 —— zone 软边界 + 全局兜底(P1-c)。
 *
 * zone 软边界: 若当前元素在某 [data-focus-zone] 内(且无 modal), 先只在该 zone 内找最近邻 ——
 * 命中则返回(横向移动留在本行/区, 不乱跳到别的 zone/Tab)。zone 内该方向无候选(到达边界,
 * 如横向 rail 的上/下) 再全局兜底跨 zone。全局兜底 = 原纯几何最近邻(现状行为), 保证永不困住焦点。
 */
export function findNearest(
  current: HTMLElement,
  dir: 'up' | 'down' | 'left' | 'right'
): HTMLElement | null {
  // 弹层优先: modal 打开时只在 modal 内(getCandidates 自动 scope), 不应用 zone 软边界
  if (!getModalContainer()) {
    const zone = getZoneContainer(current)
    if (zone) {
      const within = findNearestIn(current, dir, getCandidates(zone))
      if (within) return within
    }
  }
  return findNearestIn(current, dir, getCandidates())
}

/** 两矩形水平方向的"边缘距离"(投影重叠=0, 否则取最近边间距) */
function edgeDistH(cur: Rect, r: Rect): number {
  if (r.right > cur.left && r.left < cur.right) return 0
  return r.right <= cur.left ? cur.left - r.right : r.left - cur.right
}
/** 两矩形垂直方向的"边缘距离"(投影重叠=0) */
function edgeDistV(cur: Rect, r: Rect): number {
  if (r.bottom > cur.top && r.top < cur.bottom) return 0
  return r.bottom <= cur.top ? cur.top - r.bottom : r.top - cur.bottom
}

/**
 * 在给定候选集内按方向找最近邻(纯几何, 参数化候选集, 便于 zone/全局两种 scope 复用)。
 *
 * 策略「真·按行/列: 最近一排整排纳入, 排内取边缘最近」:
 *  1. 收集该方向上所有候选, 算主轴间隙 primary。
 *  2. 取 primary 最小的候选 anchor 作为"目标行/列"的锚; 凡与 anchor 在副轴上跨度重叠者
 *     都算同一行/列(整排纳入, 哪怕不在光标正前方)。
 *  3. 行/列内取副轴"边缘距离"最近者(光标正前方=边缘 0 最优); 平手取中心更对齐, 再取主轴更近。
 *
 * 关键改动(用户反馈): 用"与最近候选的副轴跨度重叠"界定一整行, 修复「光标正上方没有元素时整行被跳过」——
 * 正上方无、右边有时, 该右侧元素属最近行且边缘距离最小, 会被选中, 而不是跳过该行去更上一行的对齐元素。
 *
 * 两轮: 严格(候选完全在方向外) → 宽松(候选中心点在方向外, 允许投影重叠);
 * 宽松轮解决"目标与当前在副轴上投影重叠(如详情页 立即播放 vs 海报)时严格轮找不到"。
 */
export function findNearestIn(
  current: HTMLElement,
  dir: 'up' | 'down' | 'left' | 'right',
  all: HTMLElement[]
): HTMLElement | null {
  if (all.length === 0) return null
  const cur = rectOf(current)
  const vertical = dir === 'up' || dir === 'down'

  for (const strict of [true, false]) {
    const valid: { el: HTMLElement; r: Rect; primary: number }[] = []
    for (const cand of all) {
      if (cand === current) continue
      const r = rectOf(cand)
      let primary = 0
      let ok = false
      switch (dir) {
        case 'up':
          if (strict ? r.bottom <= cur.top + 1 : r.cy < cur.cy - 1) {
            primary = Math.max(0, cur.top - r.bottom)
            ok = true
          }
          break
        case 'down':
          if (strict ? r.top >= cur.bottom - 1 : r.cy > cur.cy + 1) {
            primary = Math.max(0, r.top - cur.bottom)
            ok = true
          }
          break
        case 'left':
          if (strict ? r.right <= cur.left + 1 : r.cx < cur.cx - 1) {
            primary = Math.max(0, cur.left - r.right)
            ok = true
          }
          break
        case 'right':
          if (strict ? r.left >= cur.right - 1 : r.cx > cur.cx + 1) {
            primary = Math.max(0, r.left - cur.right)
            ok = true
          }
          break
      }
      if (ok) valid.push({ el: cand, r, primary })
    }
    if (valid.length === 0) continue

    // 最近候选(主轴间隙最小)锚定"目标行/列"
    let anchor = valid[0]!
    for (const v of valid) if (v.primary < anchor.primary) anchor = v

    // 同一行/列: 与锚在副轴跨度重叠的候选(整排纳入, 不要求在光标正前方)
    const line = valid.filter((v) =>
      vertical
        ? v.r.bottom > anchor.r.top && v.r.top < anchor.r.bottom
        : v.r.right > anchor.r.left && v.r.left < anchor.r.right
    )

    // 行/列内: 边缘距离最近(正前方=0)优先; 平手取中心对齐, 再取主轴更近
    let best: { el: HTMLElement; r: Rect; primary: number } | null = null
    let bestEdge = Infinity
    let bestCenter = Infinity
    for (const v of line) {
      const edge = vertical ? edgeDistH(cur, v.r) : edgeDistV(cur, v.r)
      const center = vertical ? Math.abs(v.r.cx - cur.cx) : Math.abs(v.r.cy - cur.cy)
      if (
        edge < bestEdge ||
        (edge === bestEdge && center < bestCenter) ||
        (edge === bestEdge && center === bestCenter && best !== null && v.primary < best.primary)
      ) {
        best = v
        bestEdge = edge
        bestCenter = center
      }
    }
    if (best) return best.el
  }
  return null
}

/** 把焦点移动到 el，并 scrollIntoView 居中 */
function focusAndScroll(el: HTMLElement | null): boolean {
  if (!el) return false
  try {
    el.focus({ preventScroll: true })
  } catch {
    el.focus()
  }
  // 修复「滚动卡顿」：D-pad 连续移焦时, 'smooth' 在弱 WebView 会排队动画掉帧;
  // 'auto'(瞬时) 跟手且不掉帧, 焦点环+scale 过渡已提供视觉连续性。
  el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'auto' })
  return true
}

/** 给元素生成稳定 ID（用于 sessionStorage 记忆） */
function stableIdOf(el: HTMLElement): string {
  if (el.id) return `#${el.id}`
  const name = el.getAttribute('name')
  if (name) return `name:${name}`
  // 优先 data-* 属性
  const tagAttrs: string[] = []
  for (const attr of ['data-key', 'data-id', 'data-href', 'href', 'aria-label']) {
    const v = el.getAttribute(attr)
    if (v) {
      tagAttrs.push(`${attr}=${v}`)
    }
  }
  if (tagAttrs.length) {
    return `${el.tagName.toLowerCase()}|${tagAttrs.join('|')}`
  }
  // 兜底用 DOM 路径 + index
  const candidates = getCandidates()
  const idx = candidates.indexOf(el)
  return `idx:${idx}`
}

function findElementByStableId(id: string): HTMLElement | null {
  if (id.startsWith('#')) {
    return document.querySelector<HTMLElement>(id)
  }
  if (id.startsWith('idx:')) {
    const idx = Number(id.slice(4))
    const list = getCandidates()
    return list[idx] ?? null
  }
  if (id.startsWith('name:')) {
    return document.querySelector<HTMLElement>(`[name="${id.slice(5)}"]`)
  }
  // tag|attr=value|... 形式
  const [tag, ...attrs] = id.split('|')
  if (!tag || attrs.length === 0) return null
  const sel = `${tag}${attrs
    .map((a) => {
      const eq = a.indexOf('=')
      if (eq < 0) return ''
      const k = a.slice(0, eq)
      const v = a.slice(eq + 1).replace(/"/g, '\\"')
      return `[${k}="${v}"]`
    })
    .join('')}`
  return document.querySelector<HTMLElement>(sel)
}

export function useSpatialNavigation(): Api {
  const { isTV } = useViewMode()
  const route = useRoute()
  const router = useRouter()

  let active = false
  let lastFocusPath = route.fullPath

  function isEditingTarget(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null
    if (!el) return false
    const tag = el.tagName?.toLowerCase()
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
    if (el.isContentEditable) return true
    return false
  }

  /** 当前 focus 落在哪个 focusable，未命中则返回首个 */
  function currentFocusable(): HTMLElement | null {
    const ae = document.activeElement as HTMLElement | null
    if (ae && ae.matches(FOCUSABLE_SELECTOR)) return ae
    if (ae && ae.closest) {
      const inside = ae.closest(FOCUSABLE_SELECTOR) as HTMLElement | null
      if (inside) return inside
    }
    return getCandidates()[0] ?? null
  }

  function move(dir: 'up' | 'down' | 'left' | 'right'): boolean {
    const cur = currentFocusable()
    if (!cur) {
      const first = getCandidates()[0]
      return focusAndScroll(first ?? null)
    }
    const next = findNearest(cur, dir)
    return focusAndScroll(next)
  }

  function handleKeydown(e: KeyboardEvent): void {
    if (!active) return
    if (isEditingTarget(e.target)) {
      // 输入框聚焦：仅 Escape 拦截做返回
      const k = normalizeDpadKey(e)
      if (k === 'Escape') {
        ;(e.target as HTMLElement).blur()
      }
      return
    }
    const key = normalizeDpadKey(e)
    const action = decideSpatialAction(key, e.isTrusted, {
      currentFocusable: currentFocusable(),
      modalOpen: document.body.hasAttribute('data-gf-modal-open')
    })

    switch (action.type) {
      case 'move':
        // 修复「焦点乱跳」：TV 下方向键一律消费(无论是否成功移动)，
        // 阻止 WebView 内置焦点引擎在边界处二次移动焦点造成双重移焦。
        e.preventDefault()
        move(action.dir)
        break
      case 'activate':
        // 修复「选不中」：D-pad 合成(untrusted)Enter 不能激活 button/<a>，
        // 程序化 click 之。trusted Enter 已被判为 none，交浏览器原生激活。
        e.preventDefault()
        action.el.click()
        break
      case 'back':
        e.preventDefault()
        // history.length 兜底：回不去就回首页
        if (window.history.length > 1) {
          router.back()
        } else {
          void router.push('/index')
        }
        break
      case 'none':
      default:
        break
    }
  }

  /** 记忆当前路由焦点 */
  function rememberFocus(): void {
    try {
      const ae = document.activeElement as HTMLElement | null
      if (!ae || !ae.matches(FOCUSABLE_SELECTOR)) return
      const id = stableIdOf(ae)
      sessionStorage.setItem(FOCUS_MEMORY_PREFIX + lastFocusPath, id)
    } catch {
      /* ignore */
    }
  }

  /** 恢复焦点：先按记忆，否则聚焦首个 */
  function restoreFocus(path: string): void {
    let target: HTMLElement | null = null
    let fromMemory = false
    try {
      const id = sessionStorage.getItem(FOCUS_MEMORY_PREFIX + path)
      if (id) {
        target = findElementByStableId(id)
        fromMemory = !!target
      }
    } catch {
      /* ignore */
    }
    if (!target) {
      target = getCandidates()[0] ?? null
    }
    if (target) {
      const ae = document.activeElement
      const noActive = !ae || ae === document.body || (ae as HTMLElement).tagName === 'BODY'
      if (noActive) {
        // 首次进入新路由 (无记忆): 只 focus 不 scroll, 让 router.scrollBehavior 的
        // {top:0} 留住; 用户实际滚到下面, focus 会跟着用户 D-pad 自动 scrollIntoView.
        // 有记忆 (back/forward) 时: 焦点元素可能在页中, scroll 居中合理.
        if (fromMemory) {
          focusAndScroll(target)
        } else {
          try {
            target.focus({ preventScroll: true })
          } catch {
            target.focus()
          }
        }
      }
    }
  }

  function enable(): void {
    if (active) return
    active = true
    window.addEventListener('keydown', handleKeydown, true)
  }

  function disable(): void {
    if (!active) return
    active = false
    window.removeEventListener('keydown', handleKeydown, true)
  }

  function focusFirst(container?: HTMLElement | null): boolean {
    const list = getCandidates(container)
    return focusAndScroll(list[0] ?? null)
  }

  function focusElement(el: HTMLElement | null): boolean {
    return focusAndScroll(el)
  }

  // 路由切换：先记忆旧路径焦点，再延迟恢复新路径
  watch(
    () => route.fullPath,
    (next, prev) => {
      lastFocusPath = prev ?? next
      rememberFocus()
      lastFocusPath = next
      if (!active) return
      // 等组件渲染完成（页面切换 transition + 异步数据）
      window.setTimeout(() => {
        if (!active) return
        restoreFocus(next)
      }, FOCUS_RESTORE_DELAY)
    }
  )

  // beforeunload 也尝试记一次
  const onBeforeUnload = (): void => {
    rememberFocus()
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', onBeforeUnload)
  }

  // 跟 viewMode 联动：进入 TV → enable，离开 → disable
  watch(
    isTV,
    (v) => {
      if (v) {
        enable()
        // 首次进入也尝试聚焦（路由可能已就位）
        window.setTimeout(() => {
          if (active) restoreFocus(route.fullPath)
        }, FOCUS_RESTORE_DELAY)
      } else {
        disable()
      }
    },
    { immediate: true }
  )

  onScopeDispose(() => {
    disable()
    if (typeof window !== 'undefined') {
      window.removeEventListener('beforeunload', onBeforeUnload)
    }
  })

  return { focusFirst, focusElement, enable, disable }
}

/** 防止重复 install 的简便包装（App.vue 内调用） */
export function installSpatialNavigationOnce(): Api | null {
  if (installed) return null
  installed = true
  return useSpatialNavigation()
}
