import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import DashboardView from './DashboardView.vue'

const h = vi.hoisted(() => ({
  dashboard: vi.fn(),
  onlineOverview: vi.fn(),
  onlineDailySessions: vi.fn(),
  hotFilms: vi.fn(),
  mostFavorited: vi.fn(),
  overview: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    system: {
      dashboard: h.dashboard,
      onlineOverview: h.onlineOverview,
      onlineDailySessions: h.onlineDailySessions
    }
  }
}))

vi.mock('@/api/manage/telemetry', () => ({
  hotFilms: h.hotFilms,
  mostFavorited: h.mostFavorited,
  overview: h.overview
}))

// 记录导航目标的 RouterLink 替身: data-to = JSON.stringify(to), 供断言跳转链接
const RouterLinkStub = defineComponent({
  name: 'RouterLink',
  props: { to: { type: [String, Object], required: true } },
  template: '<a class="router-link-stub" :data-to="JSON.stringify(to)"><slot /></a>'
})

// 实时明细形态的会话: 无 online 字段(实时视图一律视为在线)
const session = {
  sid: 's1',
  ip: '58.48.225.70',
  watching: false,
  ua: 'Mozilla/5.0 Chrome/150.0.0.0',
  path: '/filmDetail',
  paths: ['/index', '/classify', '/filmDetail'],
  visits: 3,
  firstSeen: 1792483000,
  lastSeen: 1792483300
}

/** 造 daily-sessions 会话行: online 传 undefined 表示缺省该字段(旧服务端 omitempty 丢弃 false 的形态) */
function dailyRow(i: number, online?: boolean, watching = false) {
  return {
    sid: `s${i + 1}`,
    ip: `201.1.1.${i + 1}`,
    watching,
    ua: 'Mozilla/5.0 Chrome/150.0.0.0',
    path: '/index',
    paths: ['/index'],
    visits: 1,
    firstSeen: 1792483000 + i,
    lastSeen: 1792483300 + i,
    ...(online === undefined ? {} : { online })
  }
}

async function mountView() {
  const w = mount(DashboardView, {
    global: { stubs: { RouterLink: RouterLinkStub } },
    attachTo: document.body
  })
  await flushPromises()
  return w
}

type Wrapper = Awaited<ReturnType<typeof mountView>>

async function clickBtn(w: Wrapper, text: string): Promise<void> {
  const b = w.findAll('button').find((x) => x.text() === text)
  if (!b) throw new Error(`找不到按钮「${text}」`)
  await b.trigger('click')
  await flushPromises()
}

/** 所有 RouterLink 替身的目标(data-to 原始 JSON 串) */
function linkTargets(w: Wrapper): string[] {
  return w.findAll('a.router-link-stub').map((a) => a.attributes('data-to') ?? '')
}

describe('DashboardView 在线明细', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.dashboard.mockResolvedValue({ filmCount: 1 })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.overview.mockResolvedValue({ errorCount: 0, avgApiMs: 0 })
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [session] })
    h.onlineOverview.mockResolvedValue({
      uv: 1,
      pv: 1,
      watching: 0,
      bots: 2,
      sessions: [session],
      today: { uv: 1, pv: 1, peak: 1, botPV: 12, botReasons: { datacenter: 10, ua: 2 } }
    })
  })

  it('口径文案改为「近 30 分钟」', async () => {
    const w = await mountView()
    const text = w.text()
    expect(text).toContain('近 30 分钟')
    expect(text).toContain('活跃人数')
    // PV 口径 = 窗口内页面访问次数(不去重), 不再是"活跃会话数"
    expect(text).toContain('页面访问')
    expect(text).not.toContain('活跃会话')
    expect(text).not.toContain('90s')
    w.unmount()
  })

  it('新增「访问次数」列, 页面多值按「、」连接, 悬停分行显示', async () => {
    const w = await mountView()
    expect(w.text()).toContain('访问次数')
    // 访问次数单元格
    const cells = w.findAll('td').map((c) => c.text())
    expect(cells).toContain('3')
    // 页面列: 去重后的多个页面用顿号连接
    expect(w.text()).toContain('/index、/classify、/filmDetail')
    // 悬停: 一行一个路径
    const pathCell = w.findAll('span').find((s) => s.text().includes('/index、/classify'))
    expect(pathCell?.attributes('title')).toBe('/index\n/classify\n/filmDetail')
    w.unmount()
  })

  it('机器人请求 PV 悬停展示按原因的分布', async () => {
    const w = await mountView()
    const label = w.findAll('span').find((s) => s.text().includes('机器人请求 PV'))
    const title = label?.attributes('title') ?? ''
    expect(title).toContain('机房 IP：10')
    expect(title).toContain('UA 关键词：2')
    w.unmount()
  })
})

describe('DashboardView 历史明细状态判定(锁定 daily-sessions wire 契约)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.dashboard.mockResolvedValue({ filmCount: 1 })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.overview.mockResolvedValue({ errorCount: 0, avgApiMs: 0 })
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [session] })
    h.onlineOverview.mockResolvedValue({
      uv: 1,
      pv: 1,
      watching: 0,
      bots: 0,
      sessions: [session],
      today: { uv: 1, pv: 1, peak: 1, botPV: 0 }
    })
  })

  it('会话带 online:false(服务端修复后的真实 wire)判「已离线」, 在播放/在线筛选都排除', async () => {
    // watching=true + online=false: 不被最后一次心跳的 watching 盖过
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [dailyRow(0, false, true)] })
    const w = await mountView()
    await clickBtn(w, '今日')
    expect(w.text()).toContain('已离线')
    // 「在播放」筛选不应包含已离线的行
    await clickBtn(w, '在播放')
    expect(w.text()).not.toContain('已离线')
    expect(w.text()).not.toContain('201.1.1.1')
    // 「在线」筛选同样排除已离线的行
    await clickBtn(w, '在线')
    expect(w.text()).not.toContain('201.1.1.1')
    w.unmount()
  })

  it('会话缺 online 字段(旧服务端 omitempty 丢弃 false 的形态)也判「已离线」(防回归)', async () => {
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [dailyRow(0, undefined, true)] })
    const w = await mountView()
    await clickBtn(w, '今日')
    expect(w.text()).toContain('已离线')
    await clickBtn(w, '在播放')
    expect(w.text()).not.toContain('201.1.1.1')
    w.unmount()
  })

  it('online:true 会话: watching 判「在播放」, 非 watching 判「在线」, 两筛选互斥', async () => {
    h.onlineDailySessions.mockResolvedValue({
      days: 1,
      sessions: [dailyRow(0, true, true), dailyRow(1, true, false)]
    })
    const w = await mountView()
    await clickBtn(w, '今日')
    expect(w.text()).toContain('在播放')
    expect(w.text()).toContain('在线')
    // 「在线」筛选只留非在播的那行
    await clickBtn(w, '在线')
    expect(w.text()).toContain('201.1.1.2')
    expect(w.text()).not.toContain('201.1.1.1')
    // 「在播放」筛选只留在播的那行
    await clickBtn(w, '在播放')
    expect(w.text()).toContain('201.1.1.1')
    expect(w.text()).not.toContain('201.1.1.2')
    w.unmount()
  })

  it('实时视图不受影响: 无 online 字段的会话一律判「在线」而非「已离线」', async () => {
    const w = await mountView()
    expect(w.text()).toContain('在线')
    expect(w.text()).not.toContain('已离线')
    w.unmount()
  })
})

describe('DashboardView 时间×状态筛选与分页', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.dashboard.mockResolvedValue({ filmCount: 1 })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.overview.mockResolvedValue({ errorCount: 0, avgApiMs: 0 })
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [session] })
    h.onlineOverview.mockResolvedValue({
      uv: 1,
      pv: 1,
      watching: 0,
      bots: 0,
      sessions: [session],
      today: { uv: 1, pv: 1, peak: 1, botPV: 0 }
    })
  })

  it('时间筛选按对应天数拉取明细; 切回实时不再打 daily 接口', async () => {
    const w = await mountView()
    expect(h.onlineDailySessions).not.toHaveBeenCalled() // 默认实时视图
    await clickBtn(w, '今日')
    expect(h.onlineDailySessions).toHaveBeenLastCalledWith(1)
    await clickBtn(w, '近 7 天')
    expect(h.onlineDailySessions).toHaveBeenLastCalledWith(7)
    await clickBtn(w, '近 30 分钟')
    expect(h.onlineDailySessions).toHaveBeenCalledTimes(2) // 切回实时不新增请求
    expect(w.text()).toContain('58.48.225.70') // 展示回轮询的实时会话
    w.unmount()
  })

  it('刷新按钮: 实时态不拉明细; 今日态顺带重拉当前时间段', async () => {
    const w = await mountView()
    h.onlineDailySessions.mockClear()
    await clickBtn(w, '刷新')
    expect(h.onlineDailySessions).not.toHaveBeenCalled()
    await clickBtn(w, '今日')
    expect(h.onlineDailySessions).toHaveBeenCalledTimes(1)
    h.onlineDailySessions.mockClear()
    await clickBtn(w, '刷新')
    expect(h.onlineDailySessions).toHaveBeenCalledTimes(1)
    expect(h.onlineDailySessions).toHaveBeenLastCalledWith(1)
    w.unmount()
  })

  it('分页: 翻页显示后段行, 状态筛选收缩数据后页码钳制回第 1 页', async () => {
    h.onlineDailySessions.mockResolvedValue({
      days: 1,
      sessions: Array.from({ length: 12 }, (_, i) => dailyRow(i, true, i < 3))
    })
    const w = await mountView()
    await clickBtn(w, '今日')
    let trs = w.findAll('tbody tr')
    expect(trs.length).toBe(10) // 每页 10 条
    expect(trs[0].text()).toContain('201.1.1.1')
    // 翻到第 2 页: 剩 2 行
    await w.find('button[aria-label="next page"]').trigger('click')
    await flushPromises()
    trs = w.findAll('tbody tr')
    expect(trs.length).toBe(2)
    expect(trs[0].text()).toContain('201.1.1.11')
    // 切「在播放」只剩 3 行 → 页码钳制回第 1 页
    await clickBtn(w, '在播放')
    trs = w.findAll('tbody tr')
    expect(trs.length).toBe(3)
    expect(trs[0].text()).toContain('201.1.1.1')
    expect(w.text()).not.toContain('201.1.1.11')
    w.unmount()
  })

  it('切换时间/状态筛选后回到第 1 页', async () => {
    h.onlineDailySessions.mockResolvedValue({
      days: 1,
      sessions: Array.from({ length: 12 }, (_, i) => dailyRow(i, true))
    })
    const w = await mountView()
    await clickBtn(w, '今日')
    await w.find('button[aria-label="next page"]').trigger('click')
    await flushPromises()
    expect(w.findAll('tbody tr').length).toBe(2)
    // 切换状态筛选 → 回到第 1 页(10 条)
    await clickBtn(w, '在线')
    expect(w.findAll('tbody tr').length).toBe(10)
    w.unmount()
  })
})

describe('DashboardView 主看板与容错', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.dashboard.mockResolvedValue({ filmCount: 1 })
    h.hotFilms.mockResolvedValue([])
    h.mostFavorited.mockResolvedValue([])
    h.overview.mockResolvedValue({ errorCount: 0, avgApiMs: 0 })
    h.onlineDailySessions.mockResolvedValue({ days: 1, sessions: [session] })
    h.onlineOverview.mockResolvedValue({
      uv: 1,
      pv: 1,
      watching: 0,
      bots: 0,
      sessions: [session],
      today: { uv: 1, pv: 1, peak: 1, botPV: 0 }
    })
  })

  it('dashboard 加载失败显示错误态(不渲染统计卡); 点刷新恢复', async () => {
    h.dashboard.mockRejectedValue(new Error('db down'))
    const w = await mountView()
    expect(w.text()).toContain('db down')
    expect(w.text()).toContain('点击右上角刷新重试')
    expect(w.text()).not.toContain('影片总数')
    // 刷新成功后恢复
    h.dashboard.mockResolvedValue({ filmCount: 5 })
    await clickBtn(w, '刷新')
    expect(w.text()).toContain('影片总数')
    expect(w.text()).not.toContain('db down')
    w.unmount()
  })

  it('loadDaily 失败: 清空上一时间段明细并提示, 不沿用旧数据', async () => {
    h.onlineDailySessions.mockResolvedValue({ days: 7, sessions: [dailyRow(0, true)] })
    const w = await mountView()
    await clickBtn(w, '近 7 天')
    expect(w.text()).toContain('201.1.1.1')
    // 切「今日」失败: 不得继续展示近 7 天的行
    h.onlineDailySessions.mockRejectedValue(new Error('redis down'))
    await clickBtn(w, '今日')
    expect(w.text()).not.toContain('201.1.1.1')
    expect(w.text()).toContain('明细加载失败：redis down')
    expect(w.text()).toContain('点击右上角「刷新」重试')
    w.unmount()
  })

  it('热点/收藏 Top5: 渲染、万位缩写与 /filmDetail?link= 跳转', async () => {
    h.hotFilms.mockResolvedValue([{ mid: 101, name: '热门片A', cover: 'a.jpg', count: 12345 }])
    h.mostFavorited.mockResolvedValue([{ mid: 202, name: '收藏片B', cover: 'b.jpg', count: 6789 }])
    const w = await mountView()
    expect(w.text()).toContain('热门片A')
    expect(w.text()).toContain('收藏片B')
    // fmtCount 万位缩写: 12345 → 1.2万; 万以下原样
    expect(w.text()).toContain('1.2万 次')
    expect(w.text()).toContain('6789 收藏')
    const filmLinks = linkTargets(w)
      .map((t) => JSON.parse(t) as { path: string; query?: Record<string, string> })
      .filter((t) => t.path === '/filmDetail')
    expect(filmLinks).toContainEqual({ path: '/filmDetail', query: { link: '101' } })
    expect(filmLinks).toContainEqual({ path: '/filmDetail', query: { link: '202' } })
    w.unmount()
  })

  it('统计卡: 数值渲染与跳转链接(待补采页 / 遥测)', async () => {
    h.dashboard.mockResolvedValue({
      filmCount: 123,
      todayNew: 4,
      weekNew: 5,
      collectCount: 6,
      cronCount: 7,
      pendingFails: 8
    })
    h.overview.mockResolvedValue({
      pv: 0,
      uv: 0,
      uniqueUsers: 0,
      errorCount: 9,
      apiCount: 0,
      avgApiMs: 123.6,
      windowDays: 7
    })
    const w = await mountView()
    const text = w.text()
    expect(text).toContain('影片总数')
    expect(text).toContain('待补采页')
    expect(text).toContain('8') // pendingFails
    expect(text).toContain('124ms') // Math.round(123.6)
    // 待补采页 → /manage/collect/failures; 异常事件卡 + API 耗时卡 + 两个「查看全部」→ /manage/telemetry
    const raw = linkTargets(w)
    expect(raw).toContain(JSON.stringify('/manage/collect/failures'))
    expect(raw.filter((t) => t === JSON.stringify('/manage/telemetry'))).toHaveLength(4)
    w.unmount()
  })
})
