import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TaskManageView from './TaskManageView.vue'
import type { SpiderJob } from '@/api/manage/collect'
import type { CollectFailure, TaskRun } from '@/types/manage'

const h = vi.hoisted(() => ({
  spiderJobs: vi.fn(),
  list: vi.fn(),
  failures: vi.fn(),
  recoverFailures: vi.fn(),
  startSpider: vi.fn(),
  spiderJobPause: vi.fn(),
  spiderJobResume: vi.fn(),
  spiderJobCancel: vi.fn(),
  overview: vi.fn(),
  runs: vi.fn(),
  rerun: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api/manage/collect', () => ({
  spiderJobs: h.spiderJobs,
  list: h.list,
  failures: h.failures,
  recoverFailures: h.recoverFailures,
  startSpider: h.startSpider,
  spiderJobPause: h.spiderJobPause,
  spiderJobResume: h.spiderJobResume,
  spiderJobCancel: h.spiderJobCancel
}))

vi.mock('@/api/manage/tasks', () => ({
  overview: h.overview,
  runs: h.runs,
  rerun: h.rerun
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function job(over: Partial<SpiderJob> = {}): SpiderJob {
  return {
    sourceId: 'src_a',
    sourceName: '源A',
    state: 'running',
    totalPages: 10,
    donePages: 5,
    failedPages: 0,
    hour: 24,
    startedAt: 1_760_000_000,
    endedAt: 0,
    elapsedMs: 60_000,
    ...over
  }
}

function run(over: Partial<TaskRun> = {}): TaskRun {
  return {
    id: 11,
    kind: 'manual',
    type: 'collect',
    cronId: 0,
    sourceId: 'src_a',
    name: '手动采集：源A',
    hours: 24,
    status: 'failed',
    total: 10,
    done: 5,
    failed: 5,
    message: '',
    error: '获取分页总数失败: 连接超时',
    startedAt: 1_760_000_000_000,
    endedAt: 1_760_000_060_000,
    ...over
  }
}

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

const page = (total: number) => ({ current: 1, size: 20, total, pageCount: Math.ceil(total / 20) || 1 })

async function mountView() {
  const w = mount(TaskManageView, {
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
  })
  await flushPromises()
  await flushPromises()
  return w
}

function btn(w: ReturnType<typeof mount>, text: string) {
  return w.findAll('button').find((b) => b.text().includes(text))
}

async function gotoTab(w: ReturnType<typeof mount>, label: string) {
  await w.findAll('button').find((b) => b.text().includes(label))!.trigger('click')
  await flushPromises()
  await flushPromises()
}

/** ManageSelect v2(自绘浮层)交互辅助: 按 aria-label 打开浮层并点选指定文案的选项(行为断言口径) */
async function pickSelect(w: ReturnType<typeof mount>, ariaLabel: string, optionLabel: string) {
  const trig = w.findAll('button[aria-haspopup="listbox"]').find((b) => b.attributes('aria-label') === ariaLabel)
  if (!trig) throw new Error(`未找到下拉触发钮: ${ariaLabel}`)
  await trig.trigger('click')
  const opt = w.findAll('[role="option"]').find((o) => o.text().includes(optionLabel))
  if (!opt) throw new Error(`未找到选项: ${optionLabel}`)
  await opt.trigger('click')
  await flushPromises()
}

describe('TaskManageView 任务管理', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.overview.mockResolvedValue({ running: 1, todaySuccess: 2, todayFailed: 1, pendingFailures: 3 })
    h.spiderJobs.mockResolvedValue([job()])
    h.list.mockResolvedValue([
      { id: 'src_a', name: '源A', uri: '', resultModel: 0, grade: 0, syncPictures: false, collectType: 0, state: true, interval: 0 }
    ])
    h.runs.mockResolvedValue({ list: [run()], page: page(1) })
    h.failures.mockResolvedValue({ list: [failureRow()], page: page(25) })
    h.recoverFailures.mockResolvedValue({ accepted: true, pending: 3 })
    h.rerun.mockResolvedValue({ accepted: true })
    h.confirm.mockResolvedValue(true)
  })

  it('挂载: 统计卡 + 运行中列表渲染, 非当前页签不预取', async () => {
    const w = await mountView()
    const text = w.text()
    expect(text).toContain('运行中任务')
    expect(text).toContain('源A')
    expect(text).toContain('采集中')
    expect(text).toContain('进度 50%')
    expect(h.overview).toHaveBeenCalledTimes(1)
    expect(h.spiderJobs).toHaveBeenCalledTimes(1)
    expect(h.runs).not.toHaveBeenCalled()
    expect(h.failures).not.toHaveBeenCalled()
    w.unmount()
  })

  it('运行中加载失败: 页内错误态+重试, 不伪装成空态; 重试成功恢复列表', async () => {
    h.spiderJobs.mockRejectedValueOnce(new Error('网络断了'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败: 网络断了')
    expect(w.text()).toContain('点击重试')
    expect(w.text()).not.toContain('暂无正在运行')

    h.spiderJobs.mockResolvedValue([job()])
    await btn(w, '点击重试')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(w.text()).toContain('源A')
    w.unmount()
  })

  it('历史页签: 筛选变更重置页码, 翻页带上当前筛选', async () => {
    // 总数 40 → 2 页, 翻页按钮可用
    h.runs.mockResolvedValue({ list: [run()], page: page(40) })
    const w = await mountView()
    await gotoTab(w, '历史记录')
    expect(h.runs).toHaveBeenCalledWith({ status: undefined, type: undefined, page: 1, size: 20 })

    // 筛选变更 → 页码重置为 1(ManageSelect v2: 打开浮层点选「失败」)
    await pickSelect(w, '状态', '失败')
    expect(h.runs).toHaveBeenLastCalledWith({ status: 'failed', type: undefined, page: 1, size: 20 })

    // 翻到第 2 页 → 保留筛选
    await w.find('button[aria-label="next page"]').trigger('click')
    await flushPromises()
    expect(h.runs).toHaveBeenLastCalledWith({ status: 'failed', type: undefined, page: 2, size: 20 })

    // 再改类型筛选 → 页码再次重置
    await pickSelect(w, '类型', '采集')
    expect(h.runs).toHaveBeenLastCalledWith({ status: 'failed', type: 'collect', page: 1, size: 20 })
    w.unmount()
  })

  it('失败页签: 20/页分页 + 共 N 条展示; recover 提交 ids 且 silent 自管提示', async () => {
    const w = await mountView()
    await gotoTab(w, '失败记录')
    expect(h.failures).toHaveBeenCalledWith({ status: 0, page: 1, size: 20 }, { silent: true })
    expect(w.text()).toContain('共 25 条')

    // 单条补采: 传该行 id, silent(页面自管提示, 不与拦截器叠两条)
    await w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().trim() === '补采')!.trigger('click')
    await flushPromises()
    expect(h.recoverFailures).toHaveBeenLastCalledWith([7], { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已提交补采'))

    // 补采全部: ids 为空
    await btn(w, '补采全部')!.trigger('click')
    await flushPromises()
    expect(h.recoverFailures).toHaveBeenLastCalledWith(undefined, { silent: true })

    // 翻页 → 第 2 页
    await w.find('button[aria-label="next page"]').trigger('click')
    await flushPromises()
    expect(h.failures).toHaveBeenLastCalledWith({ status: 0, page: 2, size: 20 }, { silent: true })
    w.unmount()
  })

  it('历史重跑: 确认后提交并自管提示「已提交, 结果见历史记录」, 等待期间行按钮禁用', async () => {
    let release!: (v: { accepted: boolean }) => void
    h.rerun.mockReturnValueOnce(new Promise((res) => (release = res)))
    const w = await mountView()
    await gotoTab(w, '历史记录')

    const rerunBtn = w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().trim() === '重跑')!
    const pending = rerunBtn.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '重跑任务' }))
    expect(h.rerun).toHaveBeenCalledWith(11, { silent: true })
    // 后台受理等待期间: 按钮禁用 + 文案提示, 防连点登记重复台账
    const busyBtn = w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().includes('提交中'))!
    expect(busyBtn.attributes('disabled')).toBeDefined()

    release({ accepted: true })
    await pending
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('success', '已提交, 结果见历史记录')
    // 完成: 回到可点状态, 且重载台账/统计
    expect(w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().trim() === '重跑')!
      .attributes('disabled')).toBeUndefined()
    expect(h.runs.mock.calls.length).toBeGreaterThan(1)
    expect(h.overview.mock.calls.length).toBeGreaterThan(1)
    w.unmount()
  })

  it('运行中卡片操作防连点: 请求在途禁用全部操作按钮, silent 调用 + 差异化文案', async () => {
    let release!: (v: void) => void
    h.spiderJobPause.mockReturnValueOnce(new Promise<void>((res) => (release = res)))
    const w = await mountView()
    const clickPause = w.findAll('button').find((b) => b.text().trim() === '暂停')!.trigger('click')
    await flushPromises()
    expect(h.spiderJobPause).toHaveBeenCalledWith('src_a', { silent: true })
    const actions = w.findAll('button').filter((b) => ['暂停', '取消'].includes(b.text().trim()))
    expect(actions.length).toBeGreaterThan(0)
    for (const b of actions) expect(b.attributes('disabled')).toBeDefined()

    release()
    await clickPause
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('success', '已暂停')
    for (const b of w.findAll('button').filter((b) => ['暂停', '取消'].includes(b.text().trim()))) {
      expect(b.attributes('disabled')).toBeUndefined()
    }
    w.unmount()
  })

  it('取消采集: 走确认弹窗, 取消时不发请求', async () => {
    h.confirm.mockResolvedValueOnce(false)
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().trim() === '取消')!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '取消采集任务' }))
    expect(h.spiderJobCancel).not.toHaveBeenCalled()
    w.unmount()
  })

  it('页签 ManageTabs(underline): role=tablist/tab + jc-tabs--underline; 筛选下拉 ManageSelect 化(补 TV 聚焦)', async () => {
    h.runs.mockResolvedValue({ list: [run()], page: page(40) })
    const w = await mountView()
    const tablist = w.find('[role="tablist"]')
    expect(tablist.exists()).toBe(true)
    expect(tablist.classes()).toContain('jc-tabs--underline')
    const tabs = w.findAll('[role="tab"]')
    expect(tabs).toHaveLength(3)
    expect(tabs[0]!.attributes('aria-selected')).toBe('true')
    for (const t of tabs) expect(t.attributes('data-focusable')).toBe('true')

    await gotoTab(w, '历史记录')
    // 两处筛选下拉(ManageSelect v2 触发钮): data-focusable(TV 聚焦缺口修复) + aria-label
    const triggers = w.findAll('button[aria-haspopup="listbox"]')
    expect(triggers[0]!.attributes('data-focusable')).toBe('true')
    expect(triggers[0]!.attributes('aria-label')).toBe('状态')
    expect(triggers[1]!.attributes('aria-label')).toBe('类型')
    // 内置分页: 表格 fit + footer 分页条
    expect(w.find('section').classes()).toContain('jc-table--fit')
    expect(w.find('footer').exists()).toBe(true)
    w.unmount()
  })
})

describe('TaskManageView 轮询', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.overview.mockResolvedValue({ running: 1, todaySuccess: 0, todayFailed: 0, pendingFailures: 0 })
    h.spiderJobs.mockResolvedValue([job()])
    h.list.mockResolvedValue([])
    h.confirm.mockResolvedValue(true)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('卸载停轮询: unmount 后计时器到点不再拉数据', async () => {
    vi.useFakeTimers()
    const w = mount(TaskManageView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    expect(h.overview).toHaveBeenCalledTimes(1)

    // 首轮排的 30s 空闲轮询
    vi.advanceTimersByTime(30_000)
    await flushPromises()
    expect(h.overview).toHaveBeenCalledTimes(2)

    // 列表已有 running job → 转 5s 活跃节奏
    vi.advanceTimersByTime(5_000)
    await flushPromises()
    expect(h.overview).toHaveBeenCalledTimes(3)

    w.unmount()
    vi.advanceTimersByTime(60_000)
    await flushPromises()
    expect(h.overview).toHaveBeenCalledTimes(3)
  })
})
