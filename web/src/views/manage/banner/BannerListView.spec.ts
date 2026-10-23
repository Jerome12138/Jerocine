import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BannerListView from './BannerListView.vue'
import type { BannerBoard, EffectiveSlide } from '@/types/manage'

const h = vi.hoisted(() => ({
  board: vi.fn(),
  move: vi.fn(),
  save: vi.fn(),
  remove: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    banner: { board: h.board, move: h.move, save: h.save, remove: h.remove }
  }
}))

function slide(over: Partial<EffectiveSlide> = {}): EffectiveSlide {
  return {
    source: 'fallback',
    mid: 1,
    name: '影片',
    image: 'x.jpg',
    ...over
  }
}

function board(over: Partial<BannerBoard> = {}): BannerBoard {
  return {
    active: Array.from({ length: 10 }, (_, i) =>
      slide({ mid: i + 1, name: `片${i + 1}`, source: i < 2 ? 'banner' : 'fallback' })
    ),
    liveCount: 5,
    inactive: [],
    ...over
  }
}

async function mountView(): Promise<ReturnType<typeof mount>> {
  const w = mount(BannerListView, {
    global: { plugins: [createPinia()], stubs: { RouterLink: { template: '<a><slot /></a>' } } }
  })
  await new Promise((r) => setTimeout(r, 10))
  return w
}

describe('BannerListView 轮播管理(生效位 + 候选位)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.board.mockResolvedValue(board())
  })

  it('展示全部 10 位, 并在 liveCount 处分区(生效位/候选位提示)', async () => {
    const w = await mountView()
    const rows = w.findAll('ul > li').filter((li) => li.find('img, span').exists())
    expect(w.text()).toContain('以下为候选位')
    expect(w.text()).toContain('前 5 位对外生效')
    // 10 个数据行(分区提示行不算): 每行有编号或名称
    expect(w.text()).toContain('片10')
  })

  it('无候选位(全部手动且不足 liveCount)时不渲染分区提示', async () => {
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', name: 'A' }), slide({ source: 'banner', name: 'B' })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).not.toContain('以下为候选位')
    expect(w.text()).toContain('A')
  })

  it('liveCount 缺省(旧后端)按 5 处理', async () => {
    const b = board()
    delete (b as Partial<BannerBoard>).liveCount
    h.board.mockResolvedValue(b)
    const w = await mountView()
    expect(w.text()).toContain('前 5 位对外生效')
  })

  it('行内展示轮播元信息(评分/标签/榜位)且关联影片可跳详情', async () => {
    h.board.mockResolvedValue({
      active: [
        slide({
          source: 'fallback',
          mid: 160975,
          name: '好片',
          dbScore: 8.7,
          classTag: '动作,冒险',
          hotRank: 3,
          hotBoard: '热门电影'
        })
      ],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).toContain('8.7')
    expect(w.text()).toContain('动作,冒险')
    expect(w.text()).toContain('热门电影 No.3')
    // 跳转列: 关联影片渲染为链接(点击进详情页), 文案保持「影片 #mid」
    const link = w.findAll('a').find((a) => a.text().includes('影片 #160975'))
    expect(link, '关联影片应渲染为可点击跳转').toBeTruthy()
  })

  it('自定义链接行跳转列仅展示文本(不渲染跳详情链接)', async () => {
    h.board.mockResolvedValue({
      active: [slide({ source: 'banner', mid: 0, name: '活动位', link: '/filmClassify?Pid=1' })],
      liveCount: 5,
      inactive: []
    })
    const w = await mountView()
    expect(w.text()).toContain('/filmClassify?Pid=1')
    expect(w.findAll('a').find((a) => a.text().includes('filmClassify'))).toBeUndefined()
  })
})
