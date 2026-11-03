import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import TelemetryView from './TelemetryView.vue'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'
import type { ListEventsResp, OverviewResp, TelemetryEventRow, FilmStat } from '@/api/manage/telemetry'

const h = vi.hoisted(() => ({
  overview: vi.fn(),
  listEvents: vi.fn(),
  hotFilms: vi.fn(),
  mostFavorited: vi.fn(),
  apiPerf: vi.fn(),
  resolveIssue: vi.fn(),
  unresolveIssue: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api/manage/telemetry', () => ({
  overview: h.overview,
  listEvents: h.listEvents,
  hotFilms: h.hotFilms,
  mostFavorited: h.mostFavorited,
  apiPerf: h.apiPerf,
  resolveIssue: h.resolveIssue,
  unresolveIssue: h.unresolveIssue
}))
vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function ov(over: Partial<OverviewResp> = {}): OverviewResp {
  return { pv: 100, uv: 20, uniqueUsers: 10, errorCount: 2, apiCount: 50, avgApiMs: 88, windowDays: 7, ...over }
}

/** 服务端契约行: value=按 issue 聚合后的累计次数 */
function row(over: Partial<TelemetryEventRow> = {}): TelemetryEventRow {
  return {
    id: 1, ts: 0, serverTs: '2026-11-01T10:00:00Z', type: 'error', category: 'js-error',
    action: '', label: 'boom', value: 5, path: '/play', userId: 0, sessionId: '',
    platform: '', appVersion: '', extra: '', issueStatus: 'unresolved', ...over
  }
}

function film(over: Partial<FilmStat> = {}): FilmStat {
  return { mid: 1, name: '好片', cover: 'c.jpg', count: 3, ...over }
}

/** 页面 + 弹窗 Host 一起挂(共享 useManageDialog 模块单例); 弹窗内容断言走 Host wrapper。
 *  两个 mount 必须共享同一份 stubs: VTU 多 app 共享全局 stub 注册表, 后挂的 Host 若不带
 *  RouterLink stub 会把页面的 stub 顶掉(Vue warn: Component is missing template or render
 *  function: { name: 'RouterLink' }), 热点/收藏影片行渲染为空。 */
const STUBS = { RouterLink: { template: '<a><slot /></a>' }, teleport: true }

let w: VueWrapper | null = null
let wh: VueWrapper | null = null

async function mountView(): Promise<VueWrapper> {
  const both = await mountAll()
  w = both.w
  wh = both.wh
  return w
}

async function mountAll(): Promise<{ w: VueWrapper; wh: VueWrapper }> {
  const page = mount(TelemetryView, {
    global: {
      // RouterLink: 测试环境无路由; teleport: ManageSheet 内容内联渲染可查
      stubs: STUBS
    }
  })
  const host = mount(ManageDialogHost, {
    attachTo: document.body,
    global: { stubs: STUBS }
  })
  await flushPromises()
  return { w: page, wh: host }
}

// useManageDialog 模块单例复位(测试间隔离, 同 FilmClassView.spec)
beforeEach(() => {
  document.body.innerHTML = ''
  manageDialogHostMounted.value = false
  if (manageDialogState.resolve) {
    answerManageDialog({ type: 'close' })
  }
  manageDialogState.visible = false
  manageDialogState.title = ''
  manageDialogState.component = null
  manageDialogState.props = {}
  manageDialogState.mobileMode = 'sheet'
  manageDialogState.width = undefined
})

afterEach(async () => {
  // 先卸 Host(待决弹窗 resolve(null)), 再卸页面
  wh?.unmount()
  w?.unmount()
  wh = null
  w = null
  await flushPromises()
})

function btn(w: VueWrapper, text: string) {
  const found = w.findAll('button').find((b) => b.text().includes(text))
  expect(found, `应渲染「${text}」按钮`).toBeTruthy()
  return found!
}

/** 页面上两个 select: [0]=时间窗(days), [1]=状态筛选(statusFilter) */
function daysSelect(w: VueWrapper) { return w.findAll('select')[0] }
function statusSelect(w: VueWrapper) { return w.findAll('select')[1] }

describe('TelemetryView 埋点监控看板(渲染)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.overview.mockResolvedValue(ov())
    h.listEvents.mockResolvedValue({ total: 2, rows: [row(), row({ id: 2, label: 'err2', value: 1 })] })
    h.hotFilms.mockResolvedValue([film()])
    h.mostFavorited.mockResolvedValue([film({ mid: 2, count: 9 })])
    h.apiPerf.mockResolvedValue([{ action: 'GET /x', count: 10, avg: 5, p50: 4, p95: 9, p99: 12 }])
  })

  it('渲染概览数字卡 / 错误行(累计次数列) / 热点视频', async () => {
    const w = await mountView()
    expect(w.text()).toContain('PV')
    expect(w.text()).toContain('未处理') // issueStatus 标签
    expect(w.find('input[placeholder="e.g. c68eff0"]').exists()).toBe(false) // 对话框默认关闭
    // 首次加载契约: type=error, 默认 7 天 / 100 条 / offset 0 / active
    expect(h.listEvents).toHaveBeenCalledWith({ type: 'error', days: 7, limit: 100, offset: 0, status: 'active' })
    expect(h.hotFilms).toHaveBeenCalledWith(7, 10)
    expect(h.mostFavorited).toHaveBeenCalledWith(10)
    expect(h.apiPerf).toHaveBeenCalledWith(7, 10)
    // 累计次数列: 服务端聚合值直接展示, 无 "x" 前缀
    expect(w.text()).toContain('累计次数')
    expect(w.text()).not.toContain('x5')
    expect(w.text()).toContain('好片')
  })

  it('初次加载中渲染骨架而非空态(与「确实没数据」区分)', async () => {
    let resolveList!: (v: ListEventsResp) => void
    h.listEvents.mockImplementation(() => new Promise<ListEventsResp>((res) => { resolveList = res }))
    const w = mount(TelemetryView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, teleport: true } }
    })
    await flushPromises()
    expect(w.find('.jc-skeleton').exists()).toBe(true)
    expect(w.text()).not.toContain('暂无错误')
    resolveList({ total: 0, rows: [] })
    await flushPromises()
    expect(w.find('.jc-skeleton').exists()).toBe(false)
    expect(w.text()).toContain('暂无错误')
  })

  it('加载失败渲染错误态(非空态文案), 重试成功后恢复', async () => {
    h.listEvents.mockRejectedValueOnce(new Error('网络断了'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('网络断了')
    expect(w.text()).not.toContain('暂无错误')
    await btn(w, '重试').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('boom')
    expect(w.text()).not.toContain('加载失败')
  })

  it('确实没数据时显示空态', async () => {
    h.listEvents.mockResolvedValue({ total: 0, rows: [] })
    const w = await mountView()
    expect(w.text()).toContain('暂无错误')
    expect(w.text()).not.toContain('加载失败')
  })

  it('未知影片兜底: name 为空显示「未知影片 (mid:xxx)」而非空白行', async () => {
    h.hotFilms.mockResolvedValue([film({ mid: 77, name: '', cover: '' })])
    h.mostFavorited.mockResolvedValue([film({ mid: 88, name: '', count: 2 })])
    const w = await mountView()
    expect(w.text()).toContain('未知影片 (mid:77)')
    expect(w.text()).toContain('未知影片 (mid:88)')
  })
})

describe('TelemetryView 筛选联动', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.overview.mockResolvedValue(ov())
    h.listEvents.mockResolvedValue({ total: 1, rows: [row()] })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.apiPerf.mockResolvedValue([])
  })

  it('切换时间窗: 5 个指标全部按新窗口重拉', async () => {
    const w = await mountView()
    await daysSelect(w).setValue('30')
    await flushPromises()
    expect(h.overview).toHaveBeenLastCalledWith(30)
    expect(h.listEvents).toHaveBeenLastCalledWith({ type: 'error', days: 30, limit: 100, offset: 0, status: 'active' })
    expect(h.hotFilms).toHaveBeenLastCalledWith(30, 10)
  })

  it('切换状态筛选: listEvents 带新 status 且 offset 重置', async () => {
    const w = await mountView()
    await statusSelect(w).setValue('resolved')
    await flushPromises()
    expect(h.listEvents).toHaveBeenLastCalledWith({ type: 'error', days: 7, limit: 100, offset: 0, status: 'resolved' })
  })

  it('快速切换时间窗: 旧慢响应后到被丢弃, 不覆盖新数据(请求序号守卫)', async () => {
    let resolveOld!: (v: ListEventsResp) => void
    h.listEvents.mockImplementationOnce(() => new Promise<ListEventsResp>((res) => { resolveOld = res }))
    h.listEvents.mockResolvedValue({ total: 1, rows: [row({ id: 2, label: '新数据' })] })
    const w = mount(TelemetryView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' }, teleport: true } }
    })
    await flushPromises()
    // 第一轮(listEvents 挂起)未完成时切时间窗 → 第二轮请求立即返回
    await daysSelect(w).setValue('30')
    await flushPromises()
    expect(w.text()).toContain('新数据')
    resolveOld({ total: 1, rows: [row({ id: 9, label: '旧数据' })] })
    await flushPromises()
    expect(w.text()).not.toContain('旧数据')
    expect(w.text()).toContain('新数据')
  })

  it('total 大于已加载行数时提供「加载更多」, 追加请求带 offset', async () => {
    h.listEvents
      .mockResolvedValueOnce({ total: 150, rows: Array.from({ length: 100 }, (_, i) => row({ id: i + 1, label: `e${i}` })) })
      .mockResolvedValueOnce({ total: 150, rows: Array.from({ length: 50 }, (_, i) => row({ id: i + 101, label: `e${i}` })) })
    const w = await mountView()
    // 计数徽标展示服务端过滤后 total(150), 不是当前页行数(100)
    expect(w.text()).toContain('错误 (150)')
    expect(w.text()).toContain('加载更多')
    expect(w.text()).toContain('还剩 50')
    await btn(w, '加载更多').trigger('click')
    await flushPromises()
    expect(h.listEvents).toHaveBeenLastCalledWith({ type: 'error', days: 7, limit: 100, offset: 100, status: 'active' })
    expect(w.text()).not.toContain('加载更多') // 全部加载完, 按钮消失
  })
})

describe('TelemetryView 问题闭环(resolve/unresolve)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.overview.mockResolvedValue(ov())
    h.listEvents.mockResolvedValue({ total: 1, rows: [row()] })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.apiPerf.mockResolvedValue([])
    h.resolveIssue.mockResolvedValue(undefined)
    h.unresolveIssue.mockResolvedValue(undefined)
  })

  it('标记解决: commitId 为空拦截提交(toast, 不发请求); 填写后提交 trim 过的 body 并重拉', async () => {
    await mountView()
    await btn(w!, '标记解决').trigger('click')
    await flushPromises()
    const commitInput = wh!.find('input[placeholder="e.g. c68eff0"]')
    const noteInput = wh!.find('input[placeholder="改了什么 / 为什么"]')
    expect(commitInput.exists()).toBe(true)

    await btn(wh!, '保存').trigger('click')
    expect(h.toast).toHaveBeenCalledWith('error', '请填写修复 commit ID')
    expect(h.resolveIssue).not.toHaveBeenCalled()

    await commitInput.setValue('  abc123  ')
    await noteInput.setValue('  改了重试逻辑  ')
    await btn(wh!, '保存').trigger('click')
    await flushPromises()
    // silent: 写操作提示收敛到 useAsyncAction 单点(弹窗内成功「已标记解决」/失败单条)
    expect(h.resolveIssue).toHaveBeenCalledWith(
      {
        category: 'js-error', path: '/play', label: 'boom',
        commitId: 'abc123', note: '改了重试逻辑'
      },
      { silent: true }
    )
    expect(h.listEvents).toHaveBeenCalledTimes(2) // 初载 + 保存后重拉
    expect(wh!.find('input[placeholder="e.g. c68eff0"]').exists()).toBe(false) // 对话框关闭
  })

  it('撤销解决: 需确认, 取消不调接口, 确认后调用并重拉', async () => {
    h.listEvents.mockResolvedValue({
      total: 1,
      rows: [row({ issueStatus: 'resolved', resolution: { id: 1, category: 'js-error', path: '/play', label: 'boom', resolvedAt: '', resolvedCommitId: 'abc123', resolvedBy: 0, note: '', reopenedCount: 0 } })]
    })
    const w = await mountView()

    h.confirm.mockResolvedValue(false)
    await btn(w, '撤销').trigger('click')
    await flushPromises()
    expect(h.unresolveIssue).not.toHaveBeenCalled()

    h.confirm.mockResolvedValue(true)
    await btn(w, '撤销').trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '撤销已解决' }))
    // silent: 写操作提示收敛到 useAsyncAction 单点
    expect(h.unresolveIssue).toHaveBeenCalledWith(
      { category: 'js-error', path: '/play', label: 'boom' },
      { silent: true }
    )
    expect(h.listEvents).toHaveBeenCalledTimes(2)
  })

  it('撤销 in-flight 连点被守卫(避免服务端 reopened_count 重复 +1); 失败不产生未处理 rejection', async () => {
    h.listEvents.mockResolvedValue({
      total: 1,
      rows: [row({ issueStatus: 'resolved', resolution: { id: 1, category: 'js-error', path: '/play', label: 'boom', resolvedAt: '', resolvedCommitId: 'abc', resolvedBy: 0, note: '', reopenedCount: 0 } })]
    })
    const w = await mountView()
    let resolveUndo!: () => void
    h.confirm.mockResolvedValue(true)
    h.unresolveIssue.mockImplementation(() => new Promise<void>((res) => { resolveUndo = res }))
    const undoBtn = btn(w, '撤销')
    await undoBtn.trigger('click')
    await undoBtn.trigger('click') // in-flight 中: 守卫直接 return
    expect(h.unresolveIssue).toHaveBeenCalledTimes(1)
    resolveUndo()
    await flushPromises()
    // 完成后可再次操作(第二次进入 pending, 同样需放行)
    await undoBtn.trigger('click')
    expect(h.unresolveIssue).toHaveBeenCalledTimes(2)
    resolveUndo()
    await flushPromises()

    // 失败路径: try/catch 吞掉(拦截器已 toast), 无未处理 rejection
    h.unresolveIssue.mockRejectedValueOnce(new Error('x'))
    await undoBtn.trigger('click')
    await flushPromises()
    expect(h.unresolveIssue).toHaveBeenCalledTimes(3)
  })
})
