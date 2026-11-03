import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CollectJobsView from './CollectJobsView.vue'
import type { SpiderJob } from '@/api/manage/collect'

const h = vi.hoisted(() => ({
  spiderJobs: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
  cancel: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api/manage/collect', () => ({
  spiderJobs: h.spiderJobs,
  spiderJobPause: h.pause,
  spiderJobResume: h.resume,
  spiderJobCancel: h.cancel
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function job(over: Partial<SpiderJob> = {}): SpiderJob {
  return {
    sourceId: 'src_a',
    sourceName: '源A',
    state: 'running',
    totalPages: 10,
    donePages: 4,
    failedPages: 0,
    elapsedMs: 65_000,
    hour: 24,
    startedAt: 1_700_000_000,
    endedAt: 0,
    ...over
  }
}

async function mountView() {
  const w = mount(CollectJobsView, {
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
  })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  h.spiderJobs.mockResolvedValue([job()])
  h.confirm.mockResolvedValue(true)
})

afterEach(() => {
  vi.useRealTimers()
})

describe('CollectJobsView 采集任务监控', () => {
  it('渲染任务卡: 状态标签/进度/失败页数', async () => {
    const w = await mountView()
    expect(w.text()).toContain('源A')
    expect(w.text()).toContain('采集中')
    expect(w.text()).toContain('已完成 4 页 / 共 10 页')
    expect(w.text()).toContain('近 24h')
    w.unmount()
  })

  it('hour=0 显示"时长未知"(不冒充全量), hour=-1 显示"全量"', async () => {
    h.spiderJobs.mockResolvedValue([job({ hour: 0 }), job({ sourceId: 'src_b', sourceName: '源B', hour: -1 })])
    const w = await mountView()
    expect(w.text()).toContain('时长未知')
    expect(w.text()).toContain('全量')
    w.unmount()
  })

  it('error 态卡片内展示服务端落库的失败原因', async () => {
    h.spiderJobs.mockResolvedValue([job({ state: 'error', error: '源站连接超时', donePages: 3 })])
    const w = await mountView()
    const box = w.find('.jc-job__error')
    expect(box.exists()).toBe(true)
    expect(box.text()).toContain('源站连接超时')
    w.unmount()
  })

  it('排序: 运行/暂停置顶, 同组按开始时间倒序', async () => {
    h.spiderJobs.mockResolvedValue([
      job({ sourceId: 'old_done', sourceName: '旧完成', state: 'done', startedAt: 1_700_000_500 }),
      job({ sourceId: 'b_run', sourceName: '运行B', state: 'running', startedAt: 1_700_000_100 }),
      job({ sourceId: 'a_run', sourceName: '运行A', state: 'running', startedAt: 1_700_000_900 }),
      job({ sourceId: 'p', sourceName: '暂停中', state: 'paused', startedAt: 1_700_000_200 })
    ])
    const w = await mountView()
    const names = w.findAll('.jc-job__name').map((n) => n.text())
    // 活跃组: startedAt 降序 → 运行A(900) 运行B(100) 暂停中(200); 组内降序后 暂停中 在 B 后? 200>100
    expect(names).toEqual(['运行A', '暂停中', '运行B', '旧完成'])
    w.unmount()
  })

  it('暂停/继续/取消: 调对应接口(silent) + toast + 立即重拉', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().trim() === '暂停')!.trigger('click')
    await flushPromises()
    expect(h.pause).toHaveBeenCalledWith('src_a', { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', '已暂停')
    expect(h.spiderJobs).toHaveBeenCalledTimes(2) // 初载 + 操作后重拉

    // 服务端状态翻转为 paused → 手动刷新后出「继续/取消」
    h.spiderJobs.mockResolvedValue([job({ state: 'paused' })])
    await w.findAll('button').find((b) => b.text().trim() === '手动刷新')!.trigger('click')
    await flushPromises()
    await w.findAll('button').find((b) => b.text().trim() === '继续')!.trigger('click')
    await flushPromises()
    expect(h.resume).toHaveBeenCalledWith('src_a', { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', '已继续')

    h.confirm.mockResolvedValue(false) // 取消弹窗点"取消"
    await w.findAll('button').find((b) => b.text().trim() === '取消')!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '取消采集任务', danger: true }))
    expect(h.cancel).not.toHaveBeenCalled()

    h.confirm.mockResolvedValue(true)
    await w.findAll('button').find((b) => b.text().trim() === '取消')!.trigger('click')
    await flushPromises()
    expect(h.cancel).toHaveBeenCalledWith('src_a', { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', '已取消')
    w.unmount()
  })

  it('状态标签经 StatusTag 渲染: running→brand 底色, error→danger 底色', async () => {
    const w = await mountView()
    const runningTag = w.findAll('.jc-tag').find((t) => t.text() === '采集中')!
    expect(runningTag.classes()).toContain('bg-[var(--jc-brand-primary)]')
    w.unmount()

    h.spiderJobs.mockResolvedValue([job({ state: 'error' })])
    const w2 = await mountView()
    const errorTag = w2.findAll('.jc-tag').find((t) => t.text() === '出错')!
    expect(errorTag.classes()).toContain('bg-[var(--jc-danger-soft)]')
    w2.unmount()
  })

  it('操作进行中: 卡片按钮禁用防连点', async () => {
    let release!: (v: void) => void
    h.pause.mockReturnValue(new Promise<void>((res) => (release = res)))
    const w = await mountView()
    const clickPause = w.findAll('button').find((b) => b.text().trim() === '暂停')!.trigger('click')
    await flushPromises()
    const buttons = w.findAll('button').filter((b) => ['暂停', '取消'].includes(b.text().trim()))
    expect(buttons.length).toBeGreaterThan(0)
    for (const b of buttons) expect(b.attributes('disabled')).toBeDefined()
    release()
    await clickPause
    await flushPromises()
    w.unmount()
  })
})

describe('CollectJobsView 轮询与错误态', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.confirm.mockResolvedValue(true)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('轮询失败不断链: 置错误态可重试, 计时器到点仍续拉', async () => {
    vi.useFakeTimers()
    h.spiderJobs.mockRejectedValueOnce(new Error('boom'))
    const w = mount(CollectJobsView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })
    await flushPromises()
    expect(h.spiderJobs).toHaveBeenCalledTimes(1)
    // 失败: 错误态 + 重试按钮, 不落"暂无任务"空态误导
    expect(w.find('.jc-tm-empty').text()).toContain('加载失败')
    expect(w.text()).toContain('点击重试')

    // 计时器到点 → 无条件续拉(修复点: 此前失败路径不续排, 轮询链断裂)
    h.spiderJobs.mockResolvedValue([job()])
    vi.advanceTimersByTime(30_000)
    await flushPromises()
    expect(h.spiderJobs).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('源A')
    w.unmount()
  })

  it('失败但已有旧数据: 顶部提示旧数据, 列表仍展示', async () => {
    // fakeTimers 必须在 mount 前: mount 时挂进 window 的才是受控计时器
    vi.useFakeTimers()
    h.spiderJobs.mockResolvedValue([job()])
    const w = await mountView()
    expect(w.text()).toContain('源A')
    h.spiderJobs.mockRejectedValueOnce(new Error('boom'))
    vi.advanceTimersByTime(30_000)
    await flushPromises()
    expect(w.text()).toContain('上次刷新失败')
    expect(w.text()).toContain('源A') // 旧数据保留
    w.unmount()
  })

  it('无任务空态引导去采集源列表', async () => {
    h.spiderJobs.mockResolvedValue([])
    const w = await mountView()
    expect(w.find('.jc-tm-empty').text()).toContain('采集源列表')
    w.unmount()
  })
})
