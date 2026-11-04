import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ListPageHeader from './ListPageHeader.vue'

/**
 * W2c 列表页头(FavoritesView / HistoryView 桌面页头收编):
 * 标题 + 来源 Tag(云端/本地, purple/default) + 计数 + 同步中 + 登录链接(.jc-link-btn 收敛于此)。
 * 差异由 #actions 插槽承载: 无插槽 = 右侧裸登录链接(收藏页形态);
 * 有插槽 = flex 包装层(链接 + 插槽内容, 历史页形态 —— 包装层恒渲染, 与原模板一致)。
 */
const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})
router.push('/favorites')
void router.isReady()

function mountHeader(over: Record<string, unknown> = {}, slots: Record<string, unknown> = {}) {
  return mount(ListPageHeader, {
    props: {
      title: '我的收藏',
      sourceLabel: '云端收藏 · 跨设备同步',
      remoteMode: true,
      count: 7,
      unit: '部',
      remoteLoading: false,
      loggedIn: false,
      ...over
    },
    slots,
    global: { plugins: [router] }
  })
}

describe('ListPageHeader', () => {
  it('页头骨架: header/h1/meta p 类名与来源 Tag(云端 purple)逐字保留', () => {
    const w = mountHeader()
    expect(w.find('header').classes()).toEqual([
      'flex',
      'items-center',
      'justify-between',
      'mb-[var(--jc-space-5)]',
      'flex-wrap',
      'gap-[var(--jc-space-3)]'
    ])
    expect(w.find('h1').text()).toBe('我的收藏')
    expect(w.find('h1').classes()).toEqual([
      'text-[length:var(--jc-fs-2xl)]',
      'font-[var(--jc-fw-bold)]'
    ])
    const meta = w.find('p')
    expect(meta.classes()).toEqual([
      'text-sm',
      'text-muted',
      'mt-[var(--jc-space-1)]',
      'flex',
      'items-center',
      'gap-[var(--jc-space-2)]',
      'flex-wrap'
    ])
    const tag = meta.find('.jc-tag')
    expect(tag.text()).toBe('云端')
    expect(tag.classes().join(' ')).toContain('purple')
    expect(meta.text()).toContain('云端收藏 · 跨设备同步')
    expect(meta.text()).toContain('·')
    expect(meta.text()).toContain('共 7 部')
  })

  it('remoteMode=false → 本地(default); unit=条 → 共 N 条', () => {
    const w = mountHeader({ remoteMode: false, unit: '条', count: 3 })
    expect(w.find('p .jc-tag').text()).toBe('本地')
    expect(w.find('p').text()).toContain('共 3 条')
  })

  it('同步中: remoteLoading=false 无; =true → text-link"同步中…"', () => {
    expect(mountHeader().find('p .text-link').exists()).toBe(false)
    const syncing = mountHeader({ remoteLoading: true }).find('p .text-link')
    expect(syncing.exists()).toBe(true)
    expect(syncing.text()).toBe('同步中…')
  })

  it('未登录且无 #actions 插槽: 右侧裸 jc-link-btn 登录链接(收藏页形态)', () => {
    const w = mountHeader()
    const link = w.find('header > a.jc-link-btn')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('/login')
    expect(link.text()).toBe('登录以云端同步')
    // header 直接子级只有标题块 div 与裸链接(无右侧包装层)
    expect(w.findAll('header > div').length).toBe(1)
    expect(w.findAll('header > a').length).toBe(1)
  })

  it('已登录: 不渲染登录链接', () => {
    const w = mountHeader({ loggedIn: true })
    expect(w.find('.jc-link-btn').exists()).toBe(false)
  })

  it('#actions 插槽: flex 包装层恒渲染, 登录链接在层内、插槽内容随后(历史页形态)', () => {
    const w = mountHeader(
      { loggedIn: false },
      {
        actions: {
          template: '<button class="clear-btn" type="button">清空</button>'
        }
      }
    )
    const divs = w.findAll('header > div')
    const wrap = divs[divs.length - 1]
    expect(wrap.classes()).toEqual(['flex', 'items-center', 'gap-[var(--jc-space-2)]'])
    expect(wrap.find('a.jc-link-btn').exists()).toBe(true)
    expect(wrap.find('.clear-btn').exists()).toBe(true)
    // 登录态: 链接消失, 包装层与插槽内容仍在
    const w2 = mountHeader(
      { loggedIn: true },
      { actions: { template: '<button class="clear-btn" type="button">清空</button>' } }
    )
    expect(w2.find('a.jc-link-btn').exists()).toBe(false)
    expect(w2.find('header > div .clear-btn').exists()).toBe(true)
  })

  it('.jc-link-btn 视觉随本件收敛(原两页逐字副本)', () => {
    const w = mountHeader()
    const link = w.find('a.jc-link-btn')
    expect(link.exists()).toBe(true)
    // 样式经 scoped 类断言组件自身携带 data-v(收敛进组件而非调用方)
    const attr = Object.keys(link.attributes()).find((k) => k.startsWith('data-v-'))
    expect(attr).toBeDefined()
  })
})
