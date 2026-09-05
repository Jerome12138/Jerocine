import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  decideSpatialAction,
  getZoneContainer,
  findNearestIn,
  findNearest
} from './useSpatialNavigation'

/**
 * decideSpatialAction 纯函数单测 — TV 空间导航按键决策核心。
 *
 * 覆盖 Phase 1 三个 TV 体验修复中的两个（第三个滚动 behavior 在 focusAndScroll，
 * 非纯逻辑，不在此测）：
 *   - 修复「选不中」：D-pad 合成(untrusted) Enter 必须程序化 click 激活 button/<a>
 *   - 修复「焦点乱跳」：方向键一律返回 move（调用方据此无条件 preventDefault 消费）
 */

function elem(tag: string): HTMLElement {
  return document.createElement(tag)
}

describe('decideSpatialAction — 方向键', () => {
  it.each([
    ['ArrowUp', 'up'],
    ['ArrowDown', 'down'],
    ['ArrowLeft', 'left'],
    ['ArrowRight', 'right']
  ])('%s → move(%s)，与是否有候选无关（调用方据此无条件消费，防双重移焦）', (key, dir) => {
    const a = decideSpatialAction(key, true, { currentFocusable: null, modalOpen: false })
    expect(a.type).toBe('move')
    expect(a.type === 'move' && a.dir).toBe(dir)
  })
})

describe('decideSpatialAction — 激活（修复"选不中"）', () => {
  it('合成(untrusted) Enter 落在 button 上 → activate（程序化 click）', () => {
    const b = elem('button')
    const a = decideSpatialAction('Enter', false, { currentFocusable: b, modalOpen: false })
    expect(a.type).toBe('activate')
    expect(a.type === 'activate' && a.el).toBe(b)
  })

  it('合成(untrusted) Enter 落在 <a> 上 → activate', () => {
    const link = elem('a')
    const a = decideSpatialAction('Enter', false, { currentFocusable: link, modalOpen: false })
    expect(a.type).toBe('activate')
    expect(a.type === 'activate' && a.el).toBe(link)
  })

  it('真实(trusted) Enter 落在 button 上 → none（原生已激活，避免重复 click）', () => {
    const b = elem('button')
    const a = decideSpatialAction('Enter', true, { currentFocusable: b, modalOpen: false })
    expect(a.type).toBe('none')
  })

  it('Enter 落在普通 focusable(div) 上 → activate，无论 trusted', () => {
    const d = elem('div')
    expect(decideSpatialAction('Enter', true, { currentFocusable: d, modalOpen: false }).type).toBe('activate')
    expect(decideSpatialAction('Enter', false, { currentFocusable: d, modalOpen: false }).type).toBe('activate')
  })

  it('Enter 落在 input 上 → none（交给输入框 / IME）', () => {
    const i = elem('input')
    expect(decideSpatialAction('Enter', false, { currentFocusable: i, modalOpen: false }).type).toBe('none')
  })

  it('Space / Spacebar 与 Enter 同义', () => {
    const b = elem('button')
    expect(decideSpatialAction(' ', false, { currentFocusable: b, modalOpen: false }).type).toBe('activate')
    expect(decideSpatialAction('Spacebar', false, { currentFocusable: b, modalOpen: false }).type).toBe('activate')
  })

  it('无聚焦元素时 Enter → none', () => {
    expect(decideSpatialAction('Enter', false, { currentFocusable: null, modalOpen: false }).type).toBe('none')
  })
})

describe('decideSpatialAction — 返回 / 其他', () => {
  it('Escape 非 modal → back', () => {
    expect(decideSpatialAction('Escape', true, { currentFocusable: null, modalOpen: false }).type).toBe('back')
  })

  it('Backspace 非 modal → back', () => {
    expect(decideSpatialAction('Backspace', true, { currentFocusable: null, modalOpen: false }).type).toBe('back')
  })

  it('Escape 在 modal 打开时 → none（交给弹窗自行关闭，不跳页）', () => {
    expect(decideSpatialAction('Escape', true, { currentFocusable: null, modalOpen: true }).type).toBe('none')
  })

  it('未知键 → none', () => {
    expect(decideSpatialAction('a', true, { currentFocusable: null, modalOpen: false }).type).toBe('none')
  })
})

/* ===== P1-c zone 软边界 + 几何最近邻 ===== */

type Rect = { x: number; y: number; w: number; h: number }

function mockRect(el: HTMLElement, r: Rect): void {
  el.getBoundingClientRect = (() => ({
    x: r.x,
    y: r.y,
    left: r.x,
    top: r.y,
    right: r.x + r.w,
    bottom: r.y + r.h,
    width: r.w,
    height: r.h,
    toJSON() {
      return ''
    }
  })) as () => DOMRect
}

function mkFocusable(r: Rect, parent: HTMLElement): HTMLElement {
  const el = document.createElement('div')
  el.setAttribute('data-focusable', 'true')
  el.tabIndex = 0
  parent.appendChild(el)
  mockRect(el, r)
  return el
}

function mkZone(name: string): HTMLElement {
  const z = document.createElement('div')
  z.setAttribute('data-focus-zone', name)
  document.body.appendChild(z)
  return z
}

describe('getZoneContainer / findNearest(zone 软边界)', () => {
  let tabZone: HTMLElement
  let railZone: HTMLElement
  let tabs: HTMLElement[]
  let cards: HTMLElement[]
  let orphan: HTMLElement

  beforeEach(() => {
    // Tab 区(横排, 顶部): 3 个 80x40, x=0/100/200, y=0
    tabZone = mkZone('tab')
    tabs = [
      mkFocusable({ x: 0, y: 0, w: 80, h: 40 }, tabZone),
      mkFocusable({ x: 100, y: 0, w: 80, h: 40 }, tabZone),
      mkFocusable({ x: 200, y: 0, w: 80, h: 40 }, tabZone)
    ]
    // Rail 区(横排, 下方): 3 个 180x160, x=0/200/400, y=100
    railZone = mkZone('rail')
    cards = [
      mkFocusable({ x: 0, y: 100, w: 180, h: 160 }, railZone),
      mkFocusable({ x: 200, y: 100, w: 180, h: 160 }, railZone),
      mkFocusable({ x: 400, y: 100, w: 180, h: 160 }, railZone)
    ]
    // 无 zone 的游离 focusable(更下方)
    orphan = document.createElement('div')
    orphan.setAttribute('data-focusable', 'true')
    document.body.appendChild(orphan)
    mockRect(orphan, { x: 0, y: 400, w: 50, h: 50 })
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('getZoneContainer 返回最近 [data-focus-zone] 祖先', () => {
    expect(getZoneContainer(tabs[1]!)).toBe(tabZone)
    expect(getZoneContainer(cards[0]!)).toBe(railZone)
    expect(getZoneContainer(orphan)).toBe(null)
    expect(getZoneContainer(null)).toBe(null)
  })

  it('zone 内横向移动留在本 zone', () => {
    expect(findNearest(tabs[0]!, 'right')).toBe(tabs[1]!)
    expect(findNearest(cards[0]!, 'right')).toBe(cards[1]!)
    expect(findNearest(cards[1]!, 'left')).toBe(cards[0]!)
  })

  it('zone 边界(横排上下无候选)→ 全局兜底跨 zone', () => {
    expect(findNearest(tabs[0]!, 'down')).toBe(cards[0]!)
    expect(findNearest(cards[0]!, 'up')).toBe(tabs[0]!)
  })

  it('zone 内该方向到头且全局也无 → null(不困住)', () => {
    expect(findNearest(cards[2]!, 'right')).toBe(null)
  })

  it('无 zone 的游离元素走纯全局几何', () => {
    const up = findNearest(orphan, 'up')
    expect(up && cards.includes(up)).toBe(true)
  })
})

describe('findNearestIn(纯几何, 给定候选集)', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('在候选集内取该方向最近', () => {
    const wrap = document.createElement('div')
    document.body.appendChild(wrap)
    const a = mkFocusable({ x: 0, y: 0, w: 100, h: 40 }, wrap)
    const b = mkFocusable({ x: 120, y: 0, w: 100, h: 40 }, wrap)
    const c = mkFocusable({ x: 260, y: 0, w: 100, h: 40 }, wrap)
    expect(findNearestIn(a, 'right', [a, b, c])).toBe(b)
    expect(findNearestIn(c, 'left', [a, b, c])).toBe(b)
    expect(findNearestIn(a, 'left', [a, b, c])).toBe(null)
  })

  it('正上方无、右侧有时, 选最近一行的右侧元素, 不跳过该行(按行导航)', () => {
    const wrap = document.createElement('div')
    document.body.appendChild(wrap)
    // 当前: 较高的海报卡(左下)
    const poster = mkFocusable({ x: 0, y: 400, w: 180, h: 240 }, wrap)
    // 最近一行: 只有右侧一个按钮(光标正上方为空)
    const nearRight = mkFocusable({ x: 400, y: 330, w: 120, h: 48 }, wrap)
    // 更上一行: 左侧与光标对齐, 但更远
    const farLeft = mkFocusable({ x: 0, y: 250, w: 120, h: 48 }, wrap)
    // UP 应落到最近一行的右侧按钮, 而非跳过该行去更上一行的对齐按钮
    expect(findNearestIn(poster, 'up', [poster, nearRight, farLeft])).toBe(nearRight)
  })

  it('最近一行内, 正上方(投影重叠)的元素优先于同行侧边元素', () => {
    const wrap = document.createElement('div')
    document.body.appendChild(wrap)
    const cur = mkFocusable({ x: 100, y: 200, w: 100, h: 40 }, wrap)
    const aboveAligned = mkFocusable({ x: 120, y: 100, w: 100, h: 40 }, wrap)
    const aboveSide = mkFocusable({ x: 400, y: 100, w: 100, h: 40 }, wrap)
    expect(findNearestIn(cur, 'up', [cur, aboveAligned, aboveSide])).toBe(aboveAligned)
  })

  it('空候选集 → null', () => {
    const el = document.createElement('div')
    expect(findNearestIn(el, 'up', [])).toBe(null)
  })
})
