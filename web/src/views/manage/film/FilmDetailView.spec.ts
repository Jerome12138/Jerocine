import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { defineComponent } from 'vue'
import FilmDetailView from './FilmDetailView.vue'
import type {
  CollectSource,
  ManageFilmDetailResp,
  SourceFilmResult,
  SourceSearchHit
} from '@/types/manage'

const h = vi.hoisted(() => ({
  detail: vi.fn(),
  collectFilm: vi.fn(),
  list: vi.fn(),
  toast: vi.fn(),
  push: vi.fn(),
  back: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    film: {
      detail: h.detail,
      collectFilm: h.collectFilm
    },
    collect: {
      list: h.list
    }
  }
}))
vi.mock('@/api/manage/film', () => ({
  // 与真实实现同构: 单个/数组归一为数组
  normalizeCollectResult: (res: SourceFilmResult | SourceFilmResult[]) =>
    Array.isArray(res) ? res : [res]
}))
vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: { id: '123' } }),
  useRouter: () => ({ push: h.push, back: h.back })
}))

/** 候选弹框 stub: 断言 visible/candidates props, 并可触发 pick */
const CandidateStub = defineComponent({
  props: {
    visible: { type: Boolean, default: false },
    candidates: { type: Array, default: () => [] },
    sourceName: { type: String, default: '' },
    picking: { type: Boolean, default: false }
  },
  emits: ['pick'],
  template: '<div class="stub-candidate" :data-visible="visible ? \'1\' : \'0\'" />'
})

function source(over: Partial<CollectSource> = {}): CollectSource {
  return {
    id: 'a',
    name: '源A',
    uri: '',
    resultModel: 0,
    grade: 0,
    syncPictures: true,
    collectType: 0,
    state: true,
    interval: 0,
    ...over
  }
}

function detailResp(over: Partial<ManageFilmDetailResp> = {}): ManageFilmDetailResp {
  return {
    movie: {
      mid: 123,
      name: '流浪地球',
      cover: 'c.jpg',
      cName: '科幻',
      year: 2019,
      area: '中国大陆'
    },
    sources: [
      {
        siteId: 'a',
        siteName: '源A别名',
        playFrom: 'p1',
        master: true,
        episodes: [{ episode: '1', link: 'l1' }]
      },
      {
        siteId: 'a',
        siteName: '源A别名',
        playFrom: 'p2',
        master: true,
        episodes: [
          { episode: '1', link: 'l1' },
          { episode: '2', link: 'l2' }
        ]
      },
      {
        siteId: 'z',
        siteName: '已删源',
        playFrom: 'p',
        master: false,
        episodes: [{ episode: '1', link: 'l' }]
      }
    ],
    ...over
  }
}

async function mountView() {
  const w = mount(FilmDetailView, {
    global: {
      plugins: [createPinia()],
      stubs: { SourceCandidateDialog: CandidateStub, Teleport: true }
    }
  })
  await flushPromises()
  return w
}

function rowByName(w: ReturnType<typeof mount>, name: string) {
  const row = w.findAll('div').find((d) => d.classes().includes('border') && d.text().includes(name))
  expect(row, `应存在名为 ${name} 的源行`).toBeTruthy()
  return row!
}

describe('FilmDetailView 影片详情', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.detail.mockResolvedValue(detailResp())
    h.list.mockResolvedValue([
      source({ id: 'a', name: '源A', grade: 0 }),
      source({ id: 'b', name: '源B', grade: 1 }),
      source({ id: 'c', name: '源C', grade: 1, state: false })
    ])
    h.collectFilm.mockResolvedValue({ sourceId: 'b', sourceName: '源B', master: false, collected: 0 })
  })

  it('mergedSources 按 siteId 聚合集数 + 未采集源 + 已删源回退', async () => {
    const w = await mountView()
    // 源A: 详情里两条 playFrom(1集+2集) 聚合成 3 集, 主站
    const rowA = rowByName(w, '源A')
    expect(rowA.text()).toContain('3 集')
    expect(rowA.text()).toContain('主站')
    // 源B: 在源列表但未采集
    const rowB = rowByName(w, '源B')
    expect(rowB.text()).toContain('未采集')
    // 源C: 停用源标识
    const rowC = rowByName(w, '源C')
    expect(rowC.text()).toContain('已停用')
    // 源Z: 详情有、源列表已无 → 回退保留
    expect(rowByName(w, '已删源').text()).toContain('1 集')
  })

  it('采集源列表加载失败 → 提示 + 重试(不静默吞掉"按源补采"入口)', async () => {
    h.list.mockRejectedValue(new Error('源列表炸了'))
    const w = await mountView()
    expect(w.text()).toContain('采集源列表加载失败')
    expect(w.text()).toContain('源列表炸了')
    // 详情里已有的源仍在
    expect(rowByName(w, '已删源').text()).toContain('1 集')
    // 重试成功
    h.list.mockResolvedValue([source({ id: 'b', name: '源B', grade: 1 })])
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(rowByName(w, '源B').exists()).toBe(true)
  })

  it('单源采集四态 ①失败(error) → toast 采集失败, 无「操作成功」双响', async () => {
    const w = await mountView()
    h.collectFilm.mockResolvedValue({ sourceId: 'b', sourceName: '源B', master: false, collected: 0, error: '上游 403' })
    await rowByName(w, '源B').find('button').trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledWith({ sourceId: 'b', mid: 123 }, { silent: true })
    expect(h.toast).toHaveBeenCalledWith('error', '源B 采集失败: 上游 403')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.stringContaining('完成'))
  })

  it('单源采集四态 ②collected>0 → toast 完成 + 重载详情', async () => {
    const w = await mountView()
    h.collectFilm.mockResolvedValue({ sourceId: 'b', sourceName: '源B', master: false, collected: 12 })
    await rowByName(w, '源B').find('button').trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('success', '源B 采集完成, 入库 12 集')
    expect(h.detail).toHaveBeenCalledTimes(2)
  })

  it('单源采集四态 ③candidates → 弹出候选框; ④空结果 → 未搜到此片', async () => {
    const hit = { sourceVodId: 7, name: '流浪地球', year: 2019, typeName: '', remarks: '', cover: '', episodes: 1 }
    const w = await mountView()
    h.collectFilm.mockResolvedValue({
      sourceId: 'b', sourceName: '源B', master: false, collected: 0, candidates: [hit]
    })
    await rowByName(w, '源B').find('button').trigger('click')
    await flushPromises()
    const dlg = w.findComponent(CandidateStub)
    expect(dlg.props('visible')).toBe(true)
    expect(dlg.props('candidates')).toEqual([hit])
    expect(dlg.props('sourceName')).toBe('源B')

    // ④ 空结果
    h.collectFilm.mockResolvedValue({ sourceId: 'b', sourceName: '源B', master: false, collected: 0 })
    await rowByName(w, '源B').find('button').trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('error', '源B 未搜到此片')
  })

  it('候选弹框选定版本 → 精确采集成功 → 关弹框 + 重载', async () => {
    const hit: SourceSearchHit = { sourceVodId: 7, name: '流浪地球', year: 2019, typeName: '', remarks: '', cover: '', episodes: 1 }
    const w = await mountView()
    h.collectFilm.mockResolvedValue({
      sourceId: 'b', sourceName: '源B', master: false, collected: 0, candidates: [hit]
    })
    await rowByName(w, '源B').find('button').trigger('click')
    await flushPromises()
    // 用户选定候选 → 精确采集成功
    h.collectFilm.mockResolvedValue({ sourceId: 'b', sourceName: '源B', master: false, collected: 3 })
    w.findComponent(CandidateStub).vm.$emit('pick', hit)
    await flushPromises()
    expect(h.collectFilm).toHaveBeenLastCalledWith({ sourceId: 'b', vodId: 7 }, { silent: true })
    expect(w.findComponent(CandidateStub).props('visible')).toBe(false)
    expect(h.toast).toHaveBeenCalledWith('success', '源B 采集完成, 入库 3 集')
    expect(h.detail).toHaveBeenCalledTimes(2)
  })

  it('软删影片: 显示回收站标签, 公开跳转按钮禁用', async () => {
    h.detail.mockResolvedValue(detailResp({ movie: { ...(detailResp().movie), deletedAt: 1767000000000 } }))
    const w = await mountView()
    expect(w.text()).toContain('回收站')
    const gotoDetail = w.findAll('button').find((b) => b.text().includes('去详情页'))!
    expect((gotoDetail.element as HTMLButtonElement).disabled).toBe(true)
    const gotoPlay = w.findAll('button').find((b) => b.text().includes('去播放页'))!
    expect((gotoPlay.element as HTMLButtonElement).disabled).toBe(true)
    expect(gotoDetail.attributes('title')).toContain('回收站')
  })

  it('在架影片: 跳转按钮可用', async () => {
    const w = await mountView()
    const gotoDetail = w.findAll('button').find((b) => b.text().includes('去详情页'))!
    expect((gotoDetail.element as HTMLButtonElement).disabled).toBe(false)
    expect(w.text()).not.toContain('回收站')
  })
})
