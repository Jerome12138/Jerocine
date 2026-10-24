import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
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

async function mountView() {
  const w = mount(DashboardView, {
    global: { stubs: { RouterLink: true }, plugins: [] },
    attachTo: document.body
  })
  await new Promise((r) => setTimeout(r, 20))
  return w
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

describe('DashboardView 历史明细状态判定', () => {
  it('30 分钟窗口外的会话先判「已离线」, 不被最后一次心跳的 watching 盖过', async () => {
    h.onlineDailySessions.mockResolvedValue({
      days: 1,
      sessions: [{ ...session, online: false, watching: true, path: '/index', paths: ['/index'] }]
    })
    const w = await mountView()
    await w.findAll('button').find((b) => b.text() === '今日')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('已离线')
    // "在播放"筛选不应包含已离线的行
    await w.findAll('button').find((b) => b.text() === '在播放')!.trigger('click')
    expect(w.text()).not.toContain('已离线')
    w.unmount()
  })
})
