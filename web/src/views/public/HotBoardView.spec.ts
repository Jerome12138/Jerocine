import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import HotBoardView from './HotBoardView.vue'
import FilmFilterBar from '@/components/film/FilmFilterBar.vue'
import * as filmApi from '@/api/film'

vi.mock('@/api/film', () => ({
  getHotBoard: vi.fn(),
  getHotBoardFilters: vi.fn(),
  // nav store.ensureLoaded 动态 import 用到, 空分类即可
  getCategories: vi.fn().mockResolvedValue([])
}))

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/hotBoard', name: 'hot-board', component: HotBoardView }]
})

function pageResp(list: unknown[], total = list.length) {
  return {
    list,
    page: { size: 49, current: 1, pageCount: Math.max(1, Math.ceil(total / 49)), total }
  }
}

async function mountView() {
  await router.push('/hotBoard')
  await router.isReady()
  const w = mount(HotBoardView, { global: { plugins: [router] } })
  await flushPromises()
  return w
}

describe('HotBoardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(filmApi.getHotBoard).mockResolvedValue(
      pageResp([
        { mid: 1, name: '甲', picture: 'http://x/a.jpg', year: '2026', area: '中国大陆', hotRank: 1, hotBoard: '热门电影' },
        { mid: 2, name: '乙', picture: 'http://x/b.jpg', year: '2025', area: '美国', hotRank: 2, hotBoard: '热门电影' }
      ], 2) as never
    )
    vi.mocked(filmApi.getHotBoardFilters).mockResolvedValue({
      boards: [
        { value: 'movie_hot_gaia', label: '热门电影' },
        { value: 'tv_hot', label: '热门剧集' }
      ],
      years: [2026, 2025],
      areas: ['中国大陆', '美国'],
      languages: ['国语']
    } as never)
  })

  it('初始(全部榜单) → 按热度 hot 请求, board 空 / page1 / size49', async () => {
    await mountView()
    const firstArg = vi.mocked(filmApi.getHotBoard).mock.calls[0]?.[0]
    expect(firstArg).toMatchObject({
      board: '',
      sort: 'hot',
      page: 1,
      size: 49
    })
  })

  it('选具体榜单 → 联动 sort=rank, board 为该榜, 页码重置 1', async () => {
    const w = await mountView()
    vi.mocked(filmApi.getHotBoard).mockClear()
    w.findComponent(FilmFilterBar).vm.$emit('change', {
      key: 'Board',
      value: 'movie_hot_gaia'
    })
    await flushPromises()
    const arg = vi.mocked(filmApi.getHotBoard).mock.calls.at(-1)?.[0]
    expect(arg).toMatchObject({
      board: 'movie_hot_gaia',
      sort: 'rank',
      page: 1
    })
  })

  it('回到全部榜单 → sort 回到 hot', async () => {
    const w = await mountView()
    const bar = w.findComponent(FilmFilterBar)
    bar.vm.$emit('change', { key: 'Board', value: 'movie_hot_gaia' })
    await flushPromises()
    bar.vm.$emit('change', { key: 'Board', value: '' })
    await flushPromises()
    const arg = vi.mocked(filmApi.getHotBoard).mock.calls.at(-1)?.[0]
    expect(arg).toMatchObject({ board: '', sort: 'hot', page: 1 })
  })

  it('切换任意筛选 → 页码重置 1', async () => {
    const w = await mountView()
    w.findComponent(FilmFilterBar).vm.$emit('change', {
      key: 'Area',
      value: '中国大陆'
    })
    await flushPromises()
    const arg = vi.mocked(filmApi.getHotBoard).mock.calls.at(-1)?.[0]
    expect(arg).toMatchObject({ area: '中国大陆', page: 1 })
  })

  it('空列表 → 渲染空态', async () => {
    vi.mocked(filmApi.getHotBoard).mockResolvedValue(pageResp([], 0) as never)
    const w = await mountView()
    expect(w.findComponent({ name: 'BaseEmpty' }).exists()).toBe(true)
  })
})
