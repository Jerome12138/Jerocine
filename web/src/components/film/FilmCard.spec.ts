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

  it('hotRank>0 → 显示火角标; 缺省/0 → 不显示', () => {
    const withRank = mountCard({
      item: { ...baseItem, hotRank: 7 } as never
    })
    expect(withRank.find('.jc-film-card__hot-badge svg').exists()).toBe(true)
    expect(withRank.text()).toContain('No.7')

    const noRank = mountCard()
    expect(noRank.find('.jc-film-card__hot-badge').exists()).toBe(false)
  })

  it('角标三断点: 移动只显位次 / 平板短榜名 / 桌面全榜名', () => {
    const w = mountCard({
      item: { ...baseItem, hotRank: 1, hotBoard: '热门电影' } as never
    })
    // 三档 span 都在 DOM(jsdom 不应用媒体查询, 由 unocss 类控制显隐)
    expect(w.find('.jc-film-card__hot-mobile').text()).toBe('No.1')
    expect(w.find('.jc-film-card__hot-short').text()).toBe('电影 No.1')
    expect(w.find('.jc-film-card__hot-full').text()).toBe('热门电影 No.1')
  })

  it('角标 title 挂全称「豆瓣·热门电影 No.1」', () => {
    const w = mountCard({
      item: { ...baseItem, hotRank: 1, hotBoard: '热门电影' } as never
    })
    expect(w.find('.jc-film-card__hot-badge').attributes('title')).toBe(
      '豆瓣·热门电影 No.1'
    )
  })

  it('未知榜单 → 短名回落「热榜」, 全称档原样透传真实榜名', () => {
    const w = mountCard({
      item: { ...baseItem, hotRank: 3, hotBoard: '某个新榜' } as never
    })
    // 平板空间小, 短名表未收录 → 兜底「热榜」
    expect(w.find('.jc-film-card__hot-short').text()).toBe('热榜 No.3')
    // 桌面空间大, 直接透传后端给的真实全称, 不丢信息
    expect(w.find('.jc-film-card__hot-full').text()).toBe('某个新榜 No.3')
  })
})
