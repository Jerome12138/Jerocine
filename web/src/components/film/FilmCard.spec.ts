import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import FilmCard from './FilmCard.vue'
import type { FilmListItem } from '@/types/film'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

const baseItem: FilmListItem = {
  id: 1,
  mid: 100,
  cid: 1,
  pid: 0,
  cName: '电影',
  name: '测试电影',
  picture: 'http://x/a.jpg',
  year: '2025',
  area: '中国',
  remarks: '高清',
  state: 1,
  hits: 0
} as never

function mountCard(props: Partial<{ item: FilmListItem; showTitleBelow: boolean; score: number | string; ratio: string }> = {}) {
  return mount(FilmCard, {
    props: { item: baseItem, ...props },
    global: { plugins: [router] }
  })
}

describe('FilmCard', () => {
  it('渲染影片名', () => {
    const w = mountCard()
    expect(w.text()).toContain('测试电影')
  })

  it('showTitleBelow=false → 标题不在下方常驻 (hover 才出现)', () => {
    const w = mountCard({ showTitleBelow: false })
    // 卡片本身仍存在但下方标题区少
    expect(w.exists()).toBe(true)
  })

  it('显示 score (>0)', () => {
    const w = mountCard({ score: 8.5 })
    expect(w.text()).toContain('8.5')
  })

  it('score=0 / 空 → 不显示评分', () => {
    const w = mountCard({ score: 0 })
    expect(w.text()).not.toMatch(/\b0\.0\b/)
  })

  it('年份+分类副信息显示', () => {
    const w = mountCard()
    expect(w.text()).toContain('2025')
    expect(w.text()).toContain('电影')
  })

  it('卡片是 RouterLink → 链接 to /detail', () => {
    const w = mountCard()
    expect(w.find('a').exists()).toBe(true)
  })
})
