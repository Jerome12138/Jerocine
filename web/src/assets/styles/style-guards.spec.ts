import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

/**
 * 样式防呆（2026-11-06）—— 把当天踩过的「同特异性覆盖」三类坑固化成断言。
 *
 * 背景（本仓 CSS 两段式层叠）：
 *   virtual:uno.css  →  reset  →  theme.css  →  tv-cards  →  auth-tv  →  compat
 *   组件 <style>（随组件 chunk 注入）**晚于 theme.css**
 * 于是同一特异性下「后者胜」，产生过三次可见事故：
 *   1. theme.css 的 `.jc-hotarea{position:relative}` 压掉 UnoCSS `.absolute`
 *      → 卡角删除钮漂位/被裁；
 *   2. theme.css 的全局 `:focus-visible{outline:青}` 压掉控件的 `.outline-none`
 *      → 表单聚焦冒青色粗框（另有 focus:shadow-focus 的青色 3px 环同类）；
 *   3. 组件里 `.jc-source-tabs::-webkit-scrollbar{display:none}` 压掉 theme.css 的
 *      `.jc-scrollauto` → 滚动条永远看不见。
 * 本文件只做「静态文本断言」，不渲染组件——成本极低，但能拦住同类回归。
 */
const here = dirname(fileURLToPath(import.meta.url))
/** 读文件并剥掉注释 —— 断言只在「真实代码」上匹配, 否则会被"注释里提到该写法"误伤 */
const stripComments = (s: string): string =>
  s.replace(/\/\*[\s\S]*?\*\//g, '').replace(/<!--[\s\S]*?-->/g, '')
const read = (p: string): string => stripComments(readFileSync(resolve(here, p), 'utf8'))

describe('样式防呆：CSS 层叠陷阱', () => {
  const theme = read('./theme.css')

  it('.jc-hotarea 的 position 必须用 :where() 包住（否则压掉 .absolute/.fixed）', () => {
    expect(theme).toMatch(/:where\(\.jc-hotarea\)\s*\{[^}]*position:\s*relative/)
    // 反向：不允许裸 .jc-hotarea 直接声明 position
    expect(theme).not.toMatch(/^\.jc-hotarea\s*\{[^}]*position:/m)
  })

  it('.jc-field 聚焦必须同时清掉 outline 与 box-shadow（青框有两个来源）', () => {
    const block = theme.match(/\.jc-field:focus[\s\S]{0,500}?\n\}/)
    expect(block, '.jc-field 聚焦规则缺失').not.toBeNull()
    expect(block![0]).toMatch(/outline:\s*none/)
    expect(block![0]).toMatch(/box-shadow:\s*none/)
  })

  it('聚焦描边令牌 = 品牌紫（不得回退成白色/青色）', () => {
    const m = theme.match(/--jc-border-ctrl-focus:\s*([^;]+);/)
    expect(m, '--jc-border-ctrl-focus 缺失').not.toBeNull()
    expect(m![1]).toMatch(/155,\s*73,\s*231/)
  })

  it('.jc-scrollauto 常占位 + 滚动链只在「可滚」且「只锁滚动轴」时阻断', () => {
    const idle = theme.match(/\.jc-scrollauto\s*\{[^}]*\}/)?.[0] ?? ''
    expect(idle).toMatch(/scrollbar-width:\s*thin/)
    expect(idle).toMatch(/scrollbar-color:\s*transparent/)
    expect(theme).not.toMatch(/\.jc-scrollauto[^{]*\{[^}]*scrollbar-width:\s*none/)

    // 滚动链: 必须挂在 .is-scrollable + 轴向修饰上
    // 反例(2026-11-06 实测): 裸 .jc-scrollauto{overscroll-behavior:contain} 会在"无可滚内容"时
    // 把滚轮一并吃掉 → 内层不动、外层也不动。
    expect(theme).not.toMatch(/\.jc-scrollauto\s*\{[^}]*overscroll-behavior/)
    expect(theme).toMatch(/\.jc-scrollauto\.is-scrollable\.jc-scroll-x\s*\{[^}]*overscroll-behavior-x:\s*contain/)
    expect(theme).toMatch(/\.jc-scrollauto\.is-scrollable\.jc-scroll-y\s*\{[^}]*overscroll-behavior-y:\s*contain/)
    // 另一轴必须留 auto, 否则竖滚会被吃
    expect(theme).toMatch(/\.jc-scrollauto\.is-scrollable\.jc-scroll-x\s*\{[^}]*overscroll-behavior-y:\s*auto/)
    expect(theme).toMatch(/\.jc-scrollauto\.is-scrollable\.jc-scroll-y\s*\{[^}]*overscroll-behavior-x:\s*auto/)
  })

  it('用了 .jc-scrollauto 的容器都要声明滚动轴（jc-scroll-x / jc-scroll-y）', () => {
    expect(read('../../components/film/EpisodeTabs.vue')).toMatch(/jc-scrollauto jc-scroll-[xy]/)
    expect(read('../../components/layout/ManageLayout.vue')).toMatch(/jc-scrollauto jc-scroll-[xy]/)
  })

  it('选集浮窗「点外关闭」必须精确排除选集按钮（标记属性两端一致）', () => {
    // 反例(2026-11-06): 一开始把整条控制栏 .art-bottom 都排除 → 点控制条不关、点视频才关，行为不一致。
    expect(read('../../composables/usePlayer.ts')).toMatch(/setAttribute\('data-jc-episode-picker'/)
    expect(read('../../views/public/PlayView.vue')).toMatch(/closest\('\[data-jc-episode-picker\]'\)/)
    expect(read('../../views/public/PlayView.vue')).not.toMatch(/closest\('\.art-bottom'\)/)
  })

  it('组件里不得给集格写死 overscroll-behavior（会盖掉 .jc-scroll-y 的"可滚才锁"门控）', () => {
    // 反例(2026-11-06 实测): PlayView 里 `.jc-play-grid__aside :deep(.jc-episode-grid){overscroll-behavior:contain}`
    // 特异性高于 theme.css 的门控规则 → 网格无可滚内容时把滚轮整个吃掉。
    expect(read('../../views/public/PlayView.vue')).not.toMatch(/jc-episode-grid[^{]*\{[^}]*overscroll-behavior/)
  })

  it('选源条不得自行隐藏滚动条（组件样式晚于 theme.css，会盖掉 .jc-scrollauto）', () => {
    const tabs = read('../../components/film/EpisodeTabs.vue')
    expect(tabs).not.toMatch(/\.jc-source-tabs\s*\{[^}]*scrollbar-width:\s*none/)
    expect(tabs).not.toMatch(/\.jc-source-tabs::-webkit-scrollbar\s*\{[^}]*display:\s*none/)
  })
})
