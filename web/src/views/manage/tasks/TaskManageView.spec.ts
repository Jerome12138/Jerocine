import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TaskManageView from './TaskManageView.vue'

// 页签与 ?tab= 同步：这里给一个可改的 query + replace 探针
vi.mock('vue-router', () => ({
  useRoute: () => ({
    get query() {
      return h.routeQuery.tab ? { tab: h.routeQuery.tab } : {}
    }
  }),
  useRouter: () => ({ replace: h.routerReplace })
}))
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
  confirm: vi.fn(),
  // 路由：页签同步到 ?tab=
  routeQuery: { tab: '' },
  routerReplace: vi.fn(),
  // 统一任务中心
  taskCenterTasks: vi.fn(),
  taskCenterSchedule: vi.fn(),
  taskCenterOverview: vi.fn(),
  taskCenterRun: vi.fn(),
  taskCenterUpdateSetting: vi.fn()
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
  rerun: h.rerun,
  // 统一任务中心（2026-11-25）
  taskCenterTasks: h.taskCenterTasks,
  taskCenterSchedule: h.taskCenterSchedule,
  taskCenterOverview: h.taskCenterOverview,
  taskCenterRun: h.taskCenterRun,
  taskCenterUpdateSetting: h.taskCenterUpdateSetting
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
    h.taskCenterSchedule.mockResolvedValue({ items: [], gateOpen: false, tickMs: 30000, now: Date.now() })
    // 近期任务的"正在运行"同时读任务中心列表；默认给空，避免 .catch on undefined
    h.taskCenterTasks.mockResolvedValue({ list: [] })
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
    // 2026-11-25：卡片文案改为「已完成 x / 共 y 页 · z%」（并新增 目标/启动/耗时）
    expect(text).toContain('已完成 5 / 共 10 页')
    expect(text).toContain('50%')
    expect(h.overview).toHaveBeenCalledTimes(1)
    expect(h.spiderJobs).toHaveBeenCalledTimes(1)
    // running 页签要同时读「任务中心列表」(带实时进度) 和「调度器快照」(QuietOnSuccess 的 busy)：
    // 只读其中一个就会出现"概览说运行中 1、列表空"（用户实测）
    expect(h.taskCenterTasks).toHaveBeenCalled()
    expect(h.taskCenterSchedule).toHaveBeenCalled()
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

  it('失败页补采页签: 20/页分页 + 待补采页数; recover 提交 ids 且 silent 自管提示', async () => {
    const w = await mountView()
    await gotoTab(w, '失败页补采')
    expect(h.failures).toHaveBeenCalledWith({ status: 0, page: 1, size: 20 }, { silent: true })
    expect(w.text()).toContain('待补采')

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
    // 2026-11-26 起为 5 个页签：近期任务 / 计划任务 / 历史记录 / 任务异常 / 失败页补采
    // （"任务异常"是任务级台账，"失败页补采"是采集页级，两个维度必须分开）
    expect(tabs).toHaveLength(5)
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

  it('近期任务: 只有在跑/暂停才显示进度；已完成只显示页数，不再挂百分比', async () => {
    h.spiderJobs.mockResolvedValue([
      job({ sourceId: 'src_run', state: 'running', totalPages: 40, donePages: 12 }),
      job({ sourceId: 'src_done', sourceName: '完成的源', state: 'done', totalPages: 40, donePages: 40 })
    ])
    const w = await mountView()
    await flushPromises()
    const text = w.text()
    // 在跑：有百分比进度（12/40 = 30%）
    expect(text).toContain('30%')
    // 完成：**文字进度保留**（"已完成 40 / 共 40 页 · 100%"），只是不再画进度条
    expect(text).toContain('共 40 页')
    expect(text).toContain('100%')
    // 进度条只在"在跑/暂停"时渲染：这里 1 个 running + 1 个 done ⇒ 只应有 1 条
    expect(w.findAll('[data-testid="job-bar"]')).toHaveLength(1)
    w.unmount()
  })

  it('计划任务行不再显示"已完成 x/y 页 · z%"（状态标签+耗时已足够，用户要求去掉）', async () => {
    const w = await mountView()
    await gotoTab(w, '计划任务')
    await flushPromises()
    const text = w.text()
    expect(text).not.toContain('已完成 24/24')
    expect(text).not.toContain('%')
    // 长期周期任务不再挂"已完成"这类终态标签（只有 运行中/失败 才显示）
    expect(text, '计划任务行不应出现「已完成」标签').not.toContain('已完成')
    w.unmount()
  })

  it('近期任务: 正在运行的内置任务（任务中心 state=running）也要显示，且带进度', async () => {
    h.spiderJobs.mockResolvedValue([])
    h.taskCenterSchedule.mockResolvedValue({ items: [], gateOpen: false, tickMs: 30000, now: Date.now() })
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:douban_backfill', kind: 'builtin', type: 'douban_backfill',
          name: '手动执行 · 豆瓣评分欠账补查', schedule: '', scheduleText: '仅手动', enabled: false,
          editable: [], caps: [], state: 'running',
          progress: { total: 2000, done: 300, failed: 2, unit: '部' },
          startedAt: Date.now() - 60000, elapsedMs: 60000, lastRunAt: 0, message: '', error: '', nextAt: 0
        },
        {
          key: 'builtin:health', kind: 'builtin', type: 'health', name: '采集源健康巡检',
          schedule: 'every:1h', scheduleText: '每 1 小时', enabled: true, editable: [], caps: [],
          state: 'idle', progress: null, startedAt: 0, elapsedMs: 0, lastRunAt: 0, message: '', error: '', nextAt: 0
        }
      ]
    })
    const w = await mountView()
    await flushPromises()
    const text = w.text()
    expect(text).toContain('正在运行的内置任务')
    expect(text).toContain('豆瓣评分欠账补查')
    // 有进度就显示 x/y + %
    expect(text).toContain('300/2000')
    expect(text).toContain('15%')
    // 空闲任务不列
    expect(text).not.toContain('采集源健康巡检')
    w.unmount()
  })

  it('页签与 URL ?tab= 同步：带参数进入停在对应页签；切页签写回 URL', async () => {
    h.routeQuery.tab = 'history'
    const w = await mountView()
    await flushPromises()
    const selected = w.findAll('[role="tab"]').find((t) => t.attributes('aria-selected') === 'true')
    expect(selected?.text(), '带 ?tab=history 进入应停在历史记录').toContain('历史记录')

    await gotoTab(w, '计划任务')
    expect(h.routerReplace).toHaveBeenCalledWith(
      expect.objectContaining({ query: expect.objectContaining({ tab: 'schedule' }) })
    )
    h.routeQuery.tab = '' // 复位，避免影响其它用例
    w.unmount()
  })

  it('非法 ?tab= 回落到「近期任务」', async () => {
    h.routeQuery.tab = 'nonsense'
    const w = await mountView()
    await flushPromises()
    const selected = w.findAll('[role="tab"]').find((t) => t.attributes('aria-selected') === 'true')
    expect(selected?.text()).toContain('近期任务')
    h.routeQuery.tab = ''
    w.unmount()
  })

  it('计划任务行: 失败任务要显示错误原因（后端失败写 Error 不是 Message）', async () => {
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:probe', kind: 'builtin', type: 'probe', name: '探针',
          schedule: 'daily:04:00', scheduleText: '每天 04:00', enabled: true,
          editable: ['enabled'], caps: ['run'], state: 'failed',
          error: 'net_timeout 连接超时', message: ''
        }
      ]
    })
    const w = await mountView()
    await gotoTab(w, '计划任务')
    await flushPromises()
    expect(w.text()).toContain('连接超时')
    w.unmount()
  })

  it('切到默认页签「近期任务」不把 tab 写进 URL', async () => {
    const w = await mountView()
    await gotoTab(w, '计划任务')
    h.routerReplace.mockClear()
    await gotoTab(w, '近期任务')
    const arg = h.routerReplace.mock.calls.at(-1)?.[0] as { query?: Record<string, string> } | undefined
    expect(arg?.query?.tab, '默认页签不该出现在 URL').toBeUndefined()
    w.unmount()
  })

  it('任务异常徽标：未加载过不显示（避免假 0 条）', async () => {
    const w = await mountView()
    const errTab = w.findAll('[role="tab"]').find((t) => t.text().includes('任务异常'))
    expect(errTab?.text(), '未加载时徽标不该出现 0').not.toMatch(/任务异常\s*0/)
    w.unmount()
  })

  it('计划任务行显示**真实**间隔（来自配置），不是静态说明文案', async () => {
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:douban_score', kind: 'builtin', type: 'douban_score', name: '豆瓣评分同步',
          schedule: 'weekly:1 03:00', scheduleText: '每周一 03:00', enabled: true,
          editable: ['enabled', 'schedule', 'remark', 'interval'], caps: ['run'], state: 'idle',
          intervalMs: 5000, minIntervalMs: 3000, intervalLabel: '±20% 抖动，按「接口间隔」执行',
          progress: null, startedAt: 0, elapsedMs: 0, lastRunAt: 0, message: '', error: '', nextAt: 0
        }
      ]
    })
    const w = await mountView()
    await gotoTab(w, '计划任务')
    await flushPromises()
    const text = w.text()
    expect(text).toContain('间隔 5s')          // 真实值
    expect(text).not.toContain('默认 10s')     // 静态文案不再冒充当前值
    w.unmount()
  })

  it('近期任务: 手动触发的异步任务也要显示（含进度与停止按钮）', async () => {
    h.spiderJobs.mockResolvedValue([])
    h.taskCenterSchedule.mockResolvedValue({ items: [], gateOpen: false, tickMs: 30000, now: Date.now() })
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:douban_score', kind: 'builtin', type: 'douban_score',
          name: '手动执行 · 豆瓣评分同步', schedule: 'weekly:1 03:00', scheduleText: '每周一 03:00',
          enabled: true, editable: [], caps: [], state: 'running',
          progress: { total: 1941, done: 120, failed: 0, unit: '部' },
          startedAt: Date.now() - 60000, elapsedMs: 60000, lastRunAt: 0, message: '', error: '', nextAt: 0
        }
      ]
    })
    const w = await mountView()
    await flushPromises()
    const text = w.text()
    expect(text, '概览说运行中 1 时，近期任务必须能看到它').toContain('正在运行的内置任务')
    expect(text).toContain('手动执行 · 豆瓣评分同步')
    expect(text).toContain('120/1941')
    expect(text).toContain('停止')
    w.unmount()
  })

  it('多接口任务：运行中有进度条+x/y+%，已完成也在列表显示 x/y（不画条）', async () => {
    // ① 运行中：近期任务里有细进度条
    h.spiderJobs.mockResolvedValue([])
    h.taskCenterSchedule.mockResolvedValue({ items: [], gateOpen: false, tickMs: 30000, now: Date.now() })
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:douban_backfill', kind: 'builtin', type: 'douban_backfill',
          name: '手动执行 · 豆瓣评分欠账补查', schedule: '', scheduleText: '仅手动', enabled: false,
          editable: [], caps: [], state: 'running',
          progress: { total: 2000, done: 500, failed: 0, unit: '部' },
          startedAt: Date.now() - 60000, elapsedMs: 60000, lastRunAt: 0, message: '', error: '', nextAt: 0
        }
      ]
    })
    const w = await mountView()
    await flushPromises()
    expect(w.findAll('[data-testid="builtin-bar"]')).toHaveLength(1)

    // ② 已完成：计划任务行显示 x/y，但不画条
    h.taskCenterTasks.mockResolvedValue({
      list: [
        {
          key: 'builtin:douban_backfill', kind: 'builtin', type: 'douban_backfill',
          name: '豆瓣评分欠账补查', schedule: '', scheduleText: '仅手动', enabled: false,
          editable: [], caps: [], state: 'succeeded',
          progress: { total: 2000, done: 2000, failed: 0, unit: '部' },
          startedAt: 0, elapsedMs: 0, lastRunAt: Date.now(), message: '完成', error: '', nextAt: 0
        }
      ]
    })
    await gotoTab(w, '计划任务')
    await flushPromises()
    const text = w.text()
    expect(text).toContain('已完成 2000/2000 部')
    expect(text).toContain('100%')
    w.unmount()
  })
