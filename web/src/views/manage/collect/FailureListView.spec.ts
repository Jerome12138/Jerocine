import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FailureListView from './FailureListView.vue'
import type { CollectFailure } from '@/types/manage'

const h = vi.hoisted(() => ({
  failures: vi.fn(),
  list: vi.fn(),
  recoverFailures: vi.fn(),
  clearHandledFailures: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api/manage/collect', () => ({
  failures: h.failures,
  list: h.list,
  recoverFailures: h.recoverFailures,
  clearHandledFailures: h.clearHandledFailures
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function failureRow(over: Partial<CollectFailure> = {}): CollectFailure {
  return {
    id: 7,
    sourceId: 'src_a',
    pageNo: 3,
    hours: 24,
    cause: '连接超时',
    attempts: 2,
    status: 0,
    createdAt: 1_760_000_000_000,
    ...over
  }
}

function pageResp(list: CollectFailure[], total = list.length) {
  return { list, page: { current: 1, size: 20, total, pageCount: 1 } }
}

async function mountView() {
  const w = mount(FailureListView)
  await flushPromises()
  await flushPromises()
  return w
}

function btn(w: ReturnType<typeof mount>, text: string) {
  return w.findAll('button').find((b) => b.text().includes(text))
}

describe('FailureListView 采集失败台账', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.failures.mockResolvedValue(pageResp([failureRow()]))
    h.list.mockResolvedValue([
      { id: 'src_a', name: '源A', uri: '', resultModel: 0, grade: 1, syncPictures: false, collectType: 0, state: true, interval: 0 }
    ])
    h.recoverFailures.mockResolvedValue({ accepted: true, pending: 0 })
    h.clearHandledFailures.mockResolvedValue({ deleted: 3 })
    h.confirm.mockResolvedValue(true)
  })

  it('挂载: 探测待补采数 + 拉列表, 渲染源名/页码/原因/状态', async () => {
    const w = await mountView()
    // 探测请求: status=0, size=1; 列表请求: status=0, page=1, size=20
    expect(h.failures).toHaveBeenCalledWith({ status: 0, page: 1, size: 1 })
    expect(h.failures).toHaveBeenCalledWith({ status: 0, page: 1, size: 20 })
    const text = w.text()
    expect(text).toContain('源A') // srcLabel 用源名替代 id
    expect(text).toContain('连接超时')
    expect(text).toContain('待补采')
    expect(text).toContain('近 24 小时')
    w.unmount()
  })

  it('tab 切换把对应 status 传给接口, 待补采徽标展示数量', async () => {
    h.failures.mockResolvedValue(pageResp([failureRow()], 5))
    const w = await mountView()
    expect(w.text()).toContain('（5）')
    await btn(w, '全部')!.trigger('click')
    await flushPromises()
    expect(h.failures).toHaveBeenCalledWith({ status: -1, page: 1, size: 20 })
    await btn(w, '已处理')!.trigger('click')
    await flushPromises()
    expect(h.failures).toHaveBeenCalledWith({ status: 1, page: 1, size: 20 })
    w.unmount()
  })

  it('页签 ManageTabs 化: role=tab/aria-selected/count 徽标; 表格内置分页(无独立分页组件)', async () => {
    h.failures.mockResolvedValue(pageResp([failureRow()], 45))
    const w = await mountView()
    const tabs = w.findAll('[role="tab"]')
    expect(tabs).toHaveLength(3)
    expect(tabs[0]!.attributes('aria-selected')).toBe('true')
    expect(tabs[2]!.attributes('aria-selected')).toBe('false')
    // count 徽标在 segment 页签内(D① 后默认 variant=segment)
    expect(tabs[0]!.text()).toContain('（45）')
    // 内置分页: footer 存在且表格根挂 jc-table--fit; 页面不再拼独立 BasePagination 区块
    expect(w.find('footer').exists()).toBe(true)
    expect(w.find('section').classes()).toContain('jc-table--fit')

    // 翻页保留当前 tab 筛选
    await w.find('button[aria-label="next page"]').trigger('click')
    await flushPromises()
    expect(h.failures).toHaveBeenLastCalledWith({ status: 0, page: 2, size: 20 })
    w.unmount()
  })

  it('快速切 tab: 只采纳最新一次响应(旧响应后到被丢弃)', async () => {
    let releaseSlow!: (v: unknown) => void
    const slow = new Promise((r) => (releaseSlow = r))
    h.failures
      .mockResolvedValueOnce(pageResp([], 0)) // 挂载时 loadPending 探测
      .mockResolvedValueOnce(pageResp([failureRow({ status: 0 })])) // 初载
      .mockImplementationOnce(() => slow) // 切"已处理"的请求挂起
      .mockResolvedValueOnce(pageResp([failureRow({ id: 99, cause: '最新数据', status: 1 })])) // 切"全部"先回
    const w = await mountView()
    expect(w.text()).toContain('连接超时')
    await btn(w, '已处理')!.trigger('click')
    await btn(w, '全部')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('最新数据')
    // 慢的旧响应(已处理)后到 → 不得覆盖
    releaseSlow(pageResp([failureRow({ id: 88, cause: '过期数据', status: 1 })]))
    await flushPromises()
    expect(w.text()).toContain('最新数据')
    expect(w.text()).not.toContain('过期数据')
    w.unmount()
  })

  it('补采单条/全部: 传对应 ids + silent, 成功后重查待补采徽标', async () => {
    h.failures.mockResolvedValue(pageResp([failureRow()], 4))
    const w = await mountView()
    await w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().trim() === '补采')!.trigger('click')
    await flushPromises()
    expect(h.recoverFailures).toHaveBeenCalledWith([7], { silent: true })
    // recover 成功分支 → loadPending 重查(初始探测 1 次 + recover 后 1 次)
    expect(h.failures).toHaveBeenCalledWith({ status: 0, page: 1, size: 1 })

    await btn(w, '补采全部待处理')!.trigger('click')
    await flushPromises()
    expect(h.recoverFailures).toHaveBeenLastCalledWith(undefined, { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已提交补采'))
    w.unmount()
  })

  it('清理已处理: danger 确认 + silent 调用 + 刷新', async () => {
    const w = await mountView()
    await btn(w, '清理已处理')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ danger: true, okText: '清理' }))
    expect(h.clearHandledFailures).toHaveBeenCalledWith({ silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已清理 3 条'))
    w.unmount()
  })
})

describe('FailureListView 待补采徽标与「补采全部」可用性', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.failures.mockResolvedValue(pageResp([failureRow()]))
    h.list.mockResolvedValue([])
    h.recoverFailures.mockResolvedValue({ accepted: true, pending: 0 })
    h.confirm.mockResolvedValue(true)
  })

  it('探测失败(挂载时)不归 0: 「补采全部」仍可用, 不被永久误禁', async () => {
    h.failures.mockRejectedValue(new Error('探测失败'))
    const w = await mountView()
    const recoverAll = btn(w, '补采全部待处理')!
    expect(recoverAll.attributes('disabled')).toBeUndefined()
    // 待补采 tab 无徽标数字(null 不冒充 0); 列表失败有明确错误态
    const pendingTab = w.findAll('button').find((b) => b.text().includes('待补采'))!
    expect(pendingTab.text().trim()).toBe('待补采')
    expect(w.text()).toContain('加载失败: 探测失败')
    w.unmount()
  })

  it('探测到 0 条: 「补采全部」禁用; >0 条: 可用且带徽标', async () => {
    h.failures.mockResolvedValue(pageResp([], 0))
    let w = await mountView()
    expect(btn(w, '补采全部待处理')!.attributes('disabled')).toBeDefined()
    w.unmount()

    h.failures.mockResolvedValue(pageResp([failureRow()], 2))
    w = await mountView()
    expect(btn(w, '补采全部待处理')!.attributes('disabled')).toBeUndefined()
    expect(w.text()).toContain('（2）')
    w.unmount()
  })
})
