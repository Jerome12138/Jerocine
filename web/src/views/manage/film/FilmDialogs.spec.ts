import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { defineComponent } from 'vue'
import FilmCollectDialog from './FilmCollectDialog.vue'
import FilmManualAddDialog from './FilmManualAddDialog.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import type {
  CollectSource,
  FilmAddPayload,
  SourceFilmResult,
  SourceSearchHit
} from '@/types/manage'

/**
 * 「采集新片 / 手动新增」两个弹窗（原 FilmAddView 独立页收编, 见 FilmListView 工具栏两个入口）。
 * Teleport stub 掉, 便于用 w.find 直接查 ManageSheet 内的内容。
 */
const h = vi.hoisted(() => ({
  classTree: vi.fn(),
  searchSources: vi.fn(),
  collectFilm: vi.fn(),
  add: vi.fn(),
  list: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    film: {
      classTree: h.classTree,
      searchSources: h.searchSources,
      collectFilm: h.collectFilm,
      add: h.add
    },
    collect: { list: h.list },
    file: { upload: vi.fn() }
  }
}))
vi.mock('@/api/manage/film', () => ({
  // 与真实实现同构: 单个/数组归一为数组
  normalizeCollectResult: (res: SourceFilmResult | SourceFilmResult[]) =>
    Array.isArray(res) ? res : [res]
}))
vi.mock('@/api/http', () => ({ toast: h.toast }))

/** 候选弹框 stub: 断言 visible/candidates, 可触发 pick */
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

function src(over: Partial<CollectSource> = {}): CollectSource {
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

const hit: SourceSearchHit = {
  sourceVodId: 7,
  name: '流浪地球',
  year: 2019,
  typeName: '科幻',
  remarks: '热映',
  cover: 'c.jpg',
  episodes: 3
}

const stubs = { SourceCandidateDialog: CandidateStub, Teleport: true }

/** 采集新片弹窗（打开态） */
async function mountCollect() {
  const w = mount(FilmCollectDialog, {
    props: { modelValue: true },
    global: { plugins: [createPinia()], stubs }
  })
  await flushPromises()
  return w
}

/** 手动新增弹窗（打开态） */
async function mountManual() {
  const w = mount(FilmManualAddDialog, {
    props: { modelValue: true },
    global: { plugins: [createPinia()], stubs }
  })
  await flushPromises()
  return w
}

describe('FilmCollectDialog 采集新片', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.classTree.mockResolvedValue([{ id: 1, pid: 0, name: '电影', show: true, sidebarShow: true, filmVisible: true }])
    h.list.mockResolvedValue([src(), src({ id: 'b', name: '源B', grade: 1 })])
    h.searchSources.mockResolvedValue([])
    h.collectFilm.mockResolvedValue([])
  })

  it('搜索渲染: 源名/命中数/命中条目与单条采集按钮', async () => {
    h.searchSources.mockResolvedValue([
      { sourceId: 'a', sourceName: '源A', master: true, collected: 0, hits: [hit] }
    ])
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    await w.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(h.searchSources).toHaveBeenCalledWith('流浪地球')
    expect(w.text()).toContain('源A')
    expect(w.text()).toContain('命中 1 部')
    expect(w.text()).toContain('流浪地球')
    expect(w.text()).toContain('3 集')
  })

  it('空关键字搜索被拦截, 不发请求', async () => {
    const w = await mountCollect()
    await w.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(h.searchSources).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请输入片名关键字')
  })

  it('多源采集结果汇报 + 成功即 emit saved(silent 关闭「操作成功」双响)', async () => {
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    h.collectFilm.mockResolvedValue([
      { sourceId: 'a', sourceName: '源A', master: true, collected: 2 },
      { sourceId: 'b', sourceName: '源B', master: false, collected: 0, error: '上游超时' }
    ])
    await w.findAll('button').find((b) => b.text().includes('采集全部源'))!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledWith(
      { keyword: '流浪地球', sources: [] },
      { silent: true }
    )
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('采集完成, 共入库 2 集'))
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('1 个源采集失败'))
    expect(w.text()).toContain('采集失败：上游超时')
    expect(w.emitted('saved')).toBeTruthy()
  })

  it('命中条目精确采集 → collectFilm({sourceId, vodId}, silent) + 成功文案 + saved', async () => {
    h.searchSources.mockResolvedValue([
      { sourceId: 'a', sourceName: '源A', master: true, collected: 0, hits: [hit] }
    ])
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    await w.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    h.collectFilm.mockResolvedValue({ sourceId: 'a', sourceName: '源A', master: true, collected: 3 })
    await w.findAll('button').find((b) => b.text().trim() === '采集')!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledWith({ sourceId: 'a', vodId: 7 }, { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('入库 3 集'))
    expect(w.emitted('saved')).toBeTruthy()
  })

  it('候选弹框: 多版本提示 → 选择版本 → pick 精确采集', async () => {
    h.searchSources.mockResolvedValue([
      { sourceId: 'a', sourceName: '源A', master: true, collected: 0, candidates: [hit] }
    ])
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    await w.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('多版本 1 部')
    await w.findAll('button').find((b) => b.text().includes('选择版本'))!.trigger('click')
    const dlg = w.findComponent(CandidateStub)
    expect(dlg.props('visible')).toBe(true)
    h.collectFilm.mockResolvedValue({ sourceId: 'a', sourceName: '源A', master: true, collected: 5 })
    dlg.vm.$emit('pick', hit)
    await flushPromises()
    expect(h.collectFilm).toHaveBeenLastCalledWith({ sourceId: 'a', vodId: 7 }, { silent: true })
    expect(dlg.props('visible')).toBe(false)
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('入库 5 集'))
    expect(w.emitted('saved')).toBeTruthy()
  })

  it('停用源: 显示「已停用」且复选框禁用, 不会被勾进采集名单', async () => {
    h.list.mockResolvedValue([src(), src({ id: 'c', name: '源C', state: false })])
    const w = await mountCollect()
    expect(w.text()).toContain('已停用')
    const cLabel = w.findAll('label').find((l) => l.text().includes('源C'))!
    const cBox = cLabel.find('input[type="checkbox"]')
    expect((cBox.element as HTMLInputElement).disabled).toBe(true)
    await w.findAll('button').find((b) => b.text() === '全选')!.trigger('click')
    expect((cBox.element as HTMLInputElement).checked).toBe(false)
  })

  it('未勾选源点「采集选中源」→ 提示先勾选', async () => {
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    await w.findAll('button').find((b) => b.text().includes('采集选中源'))!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请先勾选采集源')
  })

  it('采集并发拦截: 采集进行中再点采集按钮不重复发请求(useAsyncAction busy)', async () => {
    let resolveCollect!: (v: SourceFilmResult[]) => void
    h.collectFilm.mockImplementationOnce(
      () => new Promise<SourceFilmResult[]>((r) => (resolveCollect = r))
    )
    const w = await mountCollect()
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪地球')
    await w.findAll('button').find((b) => b.text().includes('采集全部源'))!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledTimes(1)
    await w.findAll('button').find((b) => b.text().includes('采集全部源'))!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledTimes(1)
    resolveCollect([{ sourceId: 'a', sourceName: '源A', master: true, collected: 1 }])
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('采集全部源'))!.trigger('click')
    await flushPromises()
    expect(h.collectFilm).toHaveBeenCalledTimes(2)
  })

  it('关闭按钮 emit update:modelValue=false(不再自己跳路由)', async () => {
    const w = await mountCollect()
    await w.findAll('button').find((b) => b.text() === '关闭')!.trigger('click')
    await flushPromises()
    expect(w.emitted('update:modelValue')).toContainEqual([false])
  })
})

describe('FilmManualAddDialog 手动新增', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.classTree.mockResolvedValue([{ id: 1, pid: 0, name: '电影', show: true, sidebarShow: true, filmVisible: true }])
    h.add.mockResolvedValue(undefined)
  })

  it('建片提交 payload: cid 清空归 0(不再 422), add 带 silent, 成功后关弹窗 + saved', async () => {
    const w = await mountManual()
    await w.find('input').setValue('手动建的片')
    await w.find('input[type="number"]').setValue('5')
    await w.find('input[type="number"]').setValue('')
    await w.find('form').trigger('submit')
    await flushPromises()

    expect(h.add).toHaveBeenCalledTimes(1)
    expect(h.add.mock.calls[0][1]).toEqual({ silent: true })
    const payload = h.add.mock.calls[0][0] as FilmAddPayload
    expect(payload.name).toBe('手动建的片')
    expect(payload.cid).toBe(0)
    expect(payload.year).toBe(0)
    expect(w.emitted('saved')).toBeTruthy()
    expect(w.emitted('update:modelValue')).toContainEqual([false])
  })

  it('顶级分类下拉换 ManageSelect: 数字 value 按原类型回传并进 payload', async () => {
    const w = await mountManual()
    expect(w.findComponent(ManageSelect).exists()).toBe(true)
    // v2 自绘浮层: 未选中 → 触发钮显示 placeholder「请选择」; 选项含「电影」
    const trigger = w.find('button[aria-haspopup="listbox"]')
    expect(trigger.text()).toBe('请选择')
    await trigger.trigger('click')
    await w.findAll('[role="option"]').find((o) => o.text() === '电影')!.trigger('click')
    await w.find('input').setValue('手动建的片')
    await w.find('form').trigger('submit')
    await flushPromises()
    const payload = h.add.mock.calls[0][0] as FilmAddPayload
    expect(payload.pid).toBe(1)
  })

  it('建片提交: year 非 4 位数字被拦截并展示字段级错误', async () => {
    const w = await mountManual()
    await w.find('input').setValue('某片')
    const yearInput = w.findAll('input').find((i) => i.attributes('placeholder') === '2024')!
    await yearInput.setValue('20')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(h.add).not.toHaveBeenCalled()
    expect(w.text()).toContain('年份需为 4 位数字')
  })

  it('建片提交: 空名被拦截并展示字段级错误(不再裸奔 400)', async () => {
    const w = await mountManual()
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(h.add).not.toHaveBeenCalled()
    expect(w.text()).toContain('请填写影片名')
  })

  it('取消按钮只关弹窗, 不发请求', async () => {
    const w = await mountManual()
    await w.findAll('button').find((b) => b.text() === '取消')!.trigger('click')
    await flushPromises()
    expect(h.add).not.toHaveBeenCalled()
    expect(w.emitted('update:modelValue')).toContainEqual([false])
  })
})
