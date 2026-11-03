import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import LiveSourceListView from './LiveSourceListView.vue'
import LiveSourceFormDialog from './LiveSourceFormDialog.vue'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'
import type { LiveChannelManage, LiveSourceManage } from '@/types/live'

const h = vi.hoisted(() => ({
  getLiveNav: vi.fn(),
  updateLiveNav: vi.fn(),
  listLiveSources: vi.fn(),
  previewChannels: vi.fn(),
  createLiveSource: vi.fn(),
  updateLiveSource: vi.fn(),
  deleteLiveSource: vi.fn(),
  listLiveChannels: vi.fn(),
  upsertLiveChannel: vi.fn(),
  deleteLiveChannel: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    live: {
      getLiveNav: h.getLiveNav,
      updateLiveNav: h.updateLiveNav,
      listLiveSources: h.listLiveSources,
      previewChannels: h.previewChannels,
      createLiveSource: h.createLiveSource,
      updateLiveSource: h.updateLiveSource,
      deleteLiveSource: h.deleteLiveSource,
      listLiveChannels: h.listLiveChannels,
      upsertLiveChannel: h.upsertLiveChannel,
      deleteLiveChannel: h.deleteLiveChannel
    }
  }
}))
vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function source(over: Partial<LiveSourceManage> = {}): LiveSourceManage {
  return {
    id: 1,
    name: '主源',
    url: 'https://example.com/a.m3u',
    remark: '',
    sort: 0,
    state: 0,
    createdAt: 0,
    updatedAt: 0,
    ...over
  }
}

function channel(over: Partial<LiveChannelManage> = {}): LiveChannelManage {
  return {
    name: 'CCTV-1',
    group: '央视',
    url: 'http://stream.example.com/1.m3u8',
    hidden: false,
    isCustom: false,
    displayName: '',
    ...over
  }
}

let host: VueWrapper | null = null

async function mountView() {
  const w = mount(LiveSourceListView, {
    // ManageSheet/Teleport 到 body, stub 掉让弹窗内容内联渲染可查
    global: { stubs: { teleport: true } }
  })
  // 新增/编辑源表单经 openManageDialog 发起: 需挂 Host(共享模块单例)
  host = mount(ManageDialogHost, {
    attachTo: document.body,
    global: { stubs: { teleport: true } }
  })
  await flushPromises()
  await flushPromises()
  return w
}

afterEach(async () => {
  host?.unmount()
  host = null
  document.body.innerHTML = ''
  manageDialogHostMounted.value = false
  if (manageDialogState.resolve) answerManageDialog({ type: 'close' })
  manageDialogState.visible = false
  manageDialogState.title = ''
  manageDialogState.component = null
  manageDialogState.props = {}
  manageDialogState.mobileMode = 'sheet'
  manageDialogState.width = undefined
  await flushPromises()
})

const SILENT = { silent: true }

function btn(w: VueWrapper | null, text: string) {
  return w!.findAll('button').find((b) => b.text().trim() === text)
}

function inputByPlaceholder(w: VueWrapper | null, ph: string) {
  return w!.findAll('input').find((i) => i.attributes('placeholder') === ph)
}

describe('LiveSourceListView 直播源管理', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.getLiveNav.mockResolvedValue({ liveNavEnabled: true })
    h.listLiveSources.mockResolvedValue({ sources: [source()] })
    h.previewChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel(), channel({ name: 'CCTV-2', url: 'http://stream.example.com/2.m3u8' })]
    })
    h.confirm.mockResolvedValue(true)
    // clearAllMocks 只清调用记录不清实现, F9 用例的 pending mockImplementation 会泄漏到
    // 后续用例 —— 这里统一给写操作默认成功实现, 需要挂起/失败的场景在用例内自行覆盖
    h.upsertLiveChannel.mockResolvedValue(undefined)
    h.deleteLiveChannel.mockResolvedValue(undefined)
    h.updateLiveSource.mockResolvedValue(undefined)
    h.deleteLiveSource.mockResolvedValue(undefined)
  })

  it('挂载即拉源列表 + 顶栏开关, 渲染名称/地址/启停徽标/频道数', async () => {
    h.listLiveSources.mockResolvedValue({
      sources: [source(), source({ id: 2, name: '备用源', state: 1 })]
    })
    const w = await mountView()
    expect(h.listLiveSources).toHaveBeenCalledTimes(1)
    expect(h.getLiveNav).toHaveBeenCalledTimes(1)
    expect(w.text()).toContain('主源')
    expect(w.text()).toContain('https://example.com/a.m3u')
    expect(w.text()).toContain('启用中')
    expect(w.text()).toContain('备用源')
    expect(w.text()).toContain('已停用')
    // 频道数预览: 启用源显示数量, 停用源显示 — (公开端点对停用源 404, 不显示数字)
    expect(w.text()).toContain('2 个频道')
    expect(w.text()).toMatch(/—/)
    const switchBtn = w.find('[role="switch"]')
    expect(switchBtn.exists()).toBe(true)
    expect(switchBtn.attributes('aria-checked')).toBe('true')
    expect(w.text()).toContain('顶栏显示')
  })

  it('停用源不请求频道数预览(F8: 公开端点 404 且占用限流配额)', async () => {
    h.listLiveSources.mockResolvedValue({
      sources: [source({ id: 1, state: 0 }), source({ id: 2, state: 1 })]
    })
    await mountView()
    expect(h.previewChannels).toHaveBeenCalledTimes(1)
    expect(h.previewChannels).toHaveBeenCalledWith(1)
  })

  it('点击顶栏开关调 updateLiveNav 并翻转状态', async () => {
    const w = await mountView()
    await w.find('[role="switch"]').trigger('click')
    expect(h.updateLiveNav).toHaveBeenCalledWith(false, SILENT)
    await flushPromises()
    expect(w.find('[role="switch"]').attributes('aria-checked')).toBe('false')
    expect(w.text()).toContain('顶栏隐藏')
  })

  it('getLiveNav 失败时开关兜底为开(与迁移前行为一致)', async () => {
    h.getLiveNav.mockRejectedValue(new Error('boom'))
    const w = await mountView()
    expect(w.find('[role="switch"]').attributes('aria-checked')).toBe('true')
  })

  it('加载失败显示错误态 + 重试, 不显示空态文案(F5)', async () => {
    // Once: 首载失败, 重试走 beforeEach 里的成功 mock
    h.listLiveSources.mockRejectedValueOnce(new Error('网络炸了'))
    const w = await mountView()
    expect(w.text()).toContain('直播源加载失败')
    expect(w.text()).toContain('网络炸了')
    expect(w.text()).not.toContain('还没有直播源')
    // 重试成功后恢复正常渲染
    const retry = btn(w, '重试')
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.listLiveSources).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('主源')
  })

  it('列表为空且请求成功时才显示空态', async () => {
    h.listLiveSources.mockResolvedValue({ sources: [] })
    const w = await mountView()
    expect(w.text()).toContain('还没有直播源')
  })

  it('删除源前弹站内确认; 取消不动手, 确认后删除并刷新(F3)', async () => {
    const w = await mountView()
    h.confirm.mockResolvedValue(false)
    await btn(w, '删除')!.trigger('click')
    expect(h.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认删除直播源？', danger: true })
    )
    expect(h.deleteLiveSource).not.toHaveBeenCalled()

    h.confirm.mockResolvedValue(true)
    await btn(w, '删除')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.deleteLiveSource).toHaveBeenCalledWith(1, SILENT)
    expect(h.listLiveSources).toHaveBeenCalledTimes(2) // 初载 + 删除后 load
  })

  it('停用源: 整体覆盖 payload(state 翻转, 其余字段保留)', async () => {
    const w = await mountView()
    await btn(w, '停用')!.trigger('click')
    await flushPromises()
    expect(h.updateLiveSource).toHaveBeenCalledWith(
      1,
      {
        name: '主源',
        url: 'https://example.com/a.m3u',
        remark: '',
        sort: 0,
        state: 1
      },
      SILENT
    )
  })

  it('新增源(函数化弹窗): 校验通过后 createLiveSource, 清空的排序按 0 兜底送出(F10)', async () => {
    const w = await mountView()
    await btn(w, '新增直播源')!.trigger('click')
    await flushPromises()
    // 表单经 ManageDialogHost 渲染, 断言与操作走 Host wrapper
    const dlg = host!.findComponent(LiveSourceFormDialog)
    expect(dlg.exists(), '新增直播源应经 openManageDialog 发起').toBe(true)
    await inputByPlaceholder(host, '如： IPTV 主源')!.setValue('新源')
    await inputByPlaceholder(host, 'http(s):// 开头的 .m3u / .m3u8 地址')!.setValue('https://x.example.com/b.m3u')
    // 排序输入框(type=number)清空 → ManageInput 发出 undefined → save 时应按 0 送出而非 ''
    const sortInput = host!.findAll('input').find((i) => i.attributes('type') === 'number')
    expect(sortInput).toBeTruthy()
    await sortInput!.setValue('')
    await btn(host, '保存')!.trigger('click')
    await flushPromises()
    expect(h.createLiveSource).toHaveBeenCalledTimes(1)
    const payload = h.createLiveSource.mock.calls[0][0]
    expect(payload.sort).toBe(0)
    expect(payload.name).toBe('新源')
    expect(payload.url).toBe('https://x.example.com/b.m3u')
    expect(h.createLiveSource).toHaveBeenLastCalledWith(expect.anything(), SILENT)
    expect(h.toast).toHaveBeenCalledWith('warning', '排序需为整数，已按 0 处理')
  })

  it('编辑源(函数化弹窗回填): 正常整数排序原样送出', async () => {
    h.listLiveSources.mockResolvedValue({ sources: [source({ sort: 7 })] })
    const w = await mountView()
    await btn(w, '编辑')!.trigger('click')
    await flushPromises()
    const dlg = host!.findComponent(LiveSourceFormDialog)
    expect(dlg.exists()).toBe(true)
    expect(dlg.props('initial')).toMatchObject({ id: 1, name: '主源', sort: 7 })
    await btn(host, '保存')!.trigger('click')
    await flushPromises()
    expect(h.updateLiveSource).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ sort: 7, name: '主源' }),
      SILENT
    )
    expect(h.toast).not.toHaveBeenCalledWith('warning', expect.stringContaining('排序'))
  })

  it('频道面板: 渲染分页(每页 100), 翻页/搜索先过滤再分页(F6)', async () => {
    const many = Array.from({ length: 120 }, (_, i) =>
      channel({ name: `CH-${i + 1}`, url: `http://stream.example.com/${i + 1}.m3u8` })
    )
    h.listLiveChannels.mockResolvedValue({ id: 1, name: '主源', channels: many })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    expect(h.listLiveChannels).toHaveBeenCalledWith(1)
    // 第一页只渲染 100 条, 共数 hint 显示全量
    expect(w.findAll('.jc-chmgr__item')).toHaveLength(100)
    expect(w.text()).toContain('共 120 个频道')
    expect(w.text()).not.toContain('CH-101')
    // 翻到第 2 页看到剩余 20 条
    const page2 = w.findAll('button').find((b) => b.text().trim() === '2')
    expect(page2, '应渲染分页器第 2 页按钮').toBeTruthy()
    await page2!.trigger('click')
    await flushPromises()
    expect(w.findAll('.jc-chmgr__item')).toHaveLength(20)
    expect(w.text()).toContain('CH-101')
    // 搜索命中 1 条且自动回第 1 页(否则停留在第 2 页会渲染 0 条)
    await inputByPlaceholder(w, '搜索频道名/分组/地址')!.setValue('CH-105')
    await flushPromises()
    expect(w.findAll('.jc-chmgr__item')).toHaveLength(1)
    expect(w.text()).toContain('CH-105')
    // 搜索无结果 → 空态文案
    await inputByPlaceholder(w, '搜索频道名/分组/地址')!.setValue('不存在的频道')
    await flushPromises()
    expect(w.text()).toContain('无匹配频道')
  })

  it('隐藏解析频道: upsert payload 带 hidden 翻转, name/groupName 不送', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel({ displayName: '旧显示名' })]
    })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await btn(w, '隐藏')!.trigger('click')
    await flushPromises()
    expect(h.upsertLiveChannel).toHaveBeenCalledTimes(1)
    const p = h.upsertLiveChannel.mock.calls[0][0]
    expect(p.sourceId).toBe(1)
    expect(p.url).toBe('http://stream.example.com/1.m3u8')
    expect(p.hidden).toBe(true)
    expect(p.displayName).toBe('旧显示名')
    expect(p.name).toBeUndefined()
    expect(p.groupName).toBeUndefined()
  })

  it('隐藏自定义频道: payload 保留 name/groupName, displayName 不送', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel({ isCustom: true, name: '我的频道', group: '我的分组' })]
    })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await btn(w, '隐藏')!.trigger('click')
    await flushPromises()
    const p = h.upsertLiveChannel.mock.calls[0][0]
    expect(p.name).toBe('我的频道')
    expect(p.groupName).toBe('我的分组')
    expect(p.hidden).toBe(true)
    expect(p.displayName).toBeUndefined()
  })

  it('upsert 在途期间行按钮禁用, 防重复提交(F9)', async () => {
    let resolveUpsert!: (v: void) => void
    h.upsertLiveChannel.mockImplementation(() => new Promise<void>((r) => (resolveUpsert = r)))
    h.listLiveChannels.mockResolvedValue({ id: 1, name: '主源', channels: [channel()] })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    const hideBtn = btn(w, '隐藏')!
    await hideBtn.trigger('click')
    await flushPromises()
    expect(h.upsertLiveChannel).toHaveBeenCalledTimes(1)
    expect(btn(w, '隐藏')!.attributes('disabled')).toBeDefined()
    expect(btn(w, '改名')!.attributes('disabled')).toBeDefined()
    expect(h.toast).not.toHaveBeenCalled() // 未完成, 不提示
    resolveUpsert()
    await flushPromises()
    // 完成后 hidden 翻转, 按钮文案变「显示」且恢复可点
    const showBtn = btn(w, '显示')
    expect(showBtn).toBeTruthy()
    expect(showBtn!.attributes('disabled')).toBeUndefined()
    expect(h.toast).toHaveBeenCalledWith('success', '已隐藏')
  })

  it('解析频道改名: payload 带 displayName + 当前 hidden(改名不得重置隐藏状态, F2)', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel({ hidden: true })]
    })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await btn(w, '改名')!.trigger('click')
    await flushPromises()
    // 弹窗打开后才渲染「显示名 · 频道名」标题
    expect(w.text()).toContain('显示名 · CCTV-1')
    // 改名弹窗输入框无 placeholder(面板搜索框有), 以此定位
    const renameInput = w.findAll('input').find((i) => !i.attributes('placeholder'))
    expect(renameInput).toBeTruthy()
    await renameInput!.setValue('新显示名')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    const p = h.upsertLiveChannel.mock.calls[0][0]
    expect(p.displayName).toBe('新显示名')
    expect(p.hidden).toBe(true) // 已隐藏频道改名后必须仍隐藏
    expect(p.name).toBeUndefined()
  })

  it('自定义频道改名走 name 字段并带回 groupName/hidden(F4)', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel({ isCustom: true, name: '我的频道', group: '我的分组' })]
    })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await btn(w, '改名')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('改名 · 我的频道')
    const renameInput = w.findAll('input').find((i) => !i.attributes('placeholder'))
    await renameInput!.setValue('新频道名')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    const p = h.upsertLiveChannel.mock.calls[0][0]
    expect(p.name).toBe('新频道名')
    expect(p.groupName).toBe('我的分组')
    expect(p.hidden).toBe(false)
    expect(p.displayName).toBeUndefined()
    expect(h.toast).toHaveBeenCalledWith('success', '频道名已保存')
  })

  it('自定义频道改名为空名时拦截, 不发请求(F4)', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [channel({ isCustom: true, name: '我的频道' })]
    })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await btn(w, '改名')!.trigger('click')
    const renameInput = w.findAll('input').find((i) => !i.attributes('placeholder'))
    await renameInput!.setValue('')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.upsertLiveChannel).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('warning', '自定义频道名不能为空')
  })

  it('移除频道前弹站内确认; 确认后调 deleteLiveChannel 并重拉(F3)', async () => {
    h.listLiveChannels.mockResolvedValue({ id: 1, name: '主源', channels: [channel()] })
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    h.confirm.mockResolvedValue(false)
    await btn(w, '隐藏并移除')!.trigger('click')
    expect(h.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ danger: true, okText: '隐藏并移除' })
    )
    expect(h.deleteLiveChannel).not.toHaveBeenCalled()

    h.confirm.mockResolvedValue(true)
    await btn(w, '隐藏并移除')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.deleteLiveChannel).toHaveBeenCalledWith(1, 'http://stream.example.com/1.m3u8', SILENT)
    expect(h.listLiveChannels).toHaveBeenCalledTimes(2) // 打开面板 + 删除后重拉
  })

  it('探测可访问性: 端侧逐个探测(限流), 角标按结果点亮「可用」', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [
        channel({ name: 'A', url: 'http://s/a.m3u8' }),
        channel({ name: 'B', url: 'http://s/b.m3u8' })
      ]
    })
    const fetchMock = vi.fn(() => Promise.resolve({ ok: true } as Response))
    vi.stubGlobal('fetch', fetchMock)
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    const probe = w.findAll('button').find((b) => b.text().includes('探测可访问性'))
    expect(probe, '应渲染「探测可访问性」按钮').toBeTruthy()
    await probe!.trigger('click')
    // 探测走限流(每个请求后 sleep 150ms), 等真实定时器跑完
    await new Promise((r) => setTimeout(r, 280))
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(2) // 两个频道各探一次(cors 模式即成功)
    expect(w.text()).toContain('可用')
    vi.unstubAllGlobals()
  })

  it('一键隐藏不可用: 探测出的离线频道 → 确认后逐个 upsert(hidden) 隐藏', async () => {
    h.listLiveChannels.mockResolvedValue({
      id: 1,
      name: '主源',
      channels: [
        channel({ name: 'OK', url: 'http://s/ok.m3u8' }),
        channel({ name: 'DOWN', url: 'http://s/down.m3u8' })
      ]
    })
    // 探测: ok 可达, down 两段式都失败 → 离线
    const fetchMock = vi.fn((u: RequestInfo | URL) => {
      const s = String(u)
      return s.includes('down')
        ? Promise.reject(new TypeError('down'))
        : Promise.resolve({ ok: true } as Response)
    })
    vi.stubGlobal('fetch', fetchMock)
    const w = await mountView()
    await btn(w, '频道管理')!.trigger('click')
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('探测可访问性'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 280))
    await flushPromises()
    expect(w.text()).toContain('离线')

    const hideBtn = w.findAll('button').find((b) => b.text().includes('隐藏不可用'))
    expect(hideBtn, '应渲染「隐藏不可用」按钮').toBeTruthy()
    expect(hideBtn!.text()).toContain('1')

    h.confirm.mockResolvedValue(true)
    await hideBtn!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ title: expect.stringContaining('隐藏 1 个不可用频道') })
    )
    await new Promise((r) => setTimeout(r, 160))
    await flushPromises()
    expect(h.upsertLiveChannel).toHaveBeenCalledTimes(1)
    const p = h.upsertLiveChannel.mock.calls[0][0]
    expect(p.url).toBe('http://s/down.m3u8')
    expect(p.hidden).toBe(true)
    vi.unstubAllGlobals()
  })
})
