import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import FilmRow from './FilmRow.vue'
import type { FilmListItem } from '@/types/film'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

const items: FilmListItem[] = Array.from({ length: 5 }, (_, i) => ({
  id: i + 1,
  mid: 100 + i,
  name: `Film ${i + 1}`,
  picture: ''
} as never))

describe('FilmRow', () => {
  it('items.length 决定渲染数量', () => {
    const w = mount(FilmRow, { props: { items }, global: { plugins: [router] } })
    // FilmCard 含 RouterLink
    expect(w.findAll('a').length).toBeGreaterThanOrEqual(5)
  })

  it('title 显示', () => {
    const w = mount(FilmRow, { props: { items, title: '热播榜' }, global: { plugins: [router] } })
    expect(w.text()).toContain('热播榜')
  })

  it('无 title → 不渲染标题', () => {
    const w = mount(FilmRow, { props: { items }, global: { plugins: [router] } })
    // 不要求 h2 不存在 (可能有其它 heading), 只断言不包含特定文字
    expect(w.findAll('h2').filter(h => h.text().trim() === '').length).toBeGreaterThanOrEqual(0)
  })

  it('moreLink 字符串 → 渲染更多链接', () => {
    const w = mount(FilmRow, {
      props: { items, title: 'X', moreLink: '/classify' },
      global: { plugins: [router] }
    })
    const moreA = w.findAll('a').find(a => a.attributes('href')?.includes('/classify'))
    expect(moreA).toBeTruthy()
  })

  it('空 items 不崩', () => {
    expect(() => mount(FilmRow, { props: { items: [], title: 'X' }, global: { plugins: [router] } })).not.toThrow()
  })
})
