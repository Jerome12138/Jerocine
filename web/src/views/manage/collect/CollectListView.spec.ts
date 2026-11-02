import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CollectListView from './CollectListView.vue'
import type { CollectSource } from '@/types/manage'
import type { SourceHealthRow } from '@/api/manage/collect'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  find: vi.fn(),
  add: vi.fn(),
  update: vi.fn(),
  change: vi.fn(),
  remove: vi.fn(),
  test: vi.fn(),
  testAll: vi.fn(),
  health: vi.fn(),
  sampleM3u8: vi.fn(),
  recordPlayLatency: vi.fn(),
  startSpider: vi.fn(),
  resetSpider: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn(),
  measureLine: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    collect: {
      list: h.list,
      find: h.find,
      add: h.add,
      update: h.update,
      change: h.change,
      remove: h.remove,
      test: h.test,
      testAll: h.testAll,
      health: h.health,
      sampleM3u8: h.sampleM3u8,
      recordPlayLatency: h.recordPlayLatency,
      startSpider: h.startSpider,
      resetSpider: h.resetSpider
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))
vi.mock('@/composables/usePlaySpeedTest', () => ({ measureLine: h.measureLine }))

function row(over: Partial<CollectSource> = {}): CollectSource {
  return {
    id: 'src_a',
    name: '源A',
    uri: 'https://a.example.com/api',
    siteUrl: '',
    resultModel: 0,
    grade: 1,
    syncPictures: false,
    collectType: 0,
    state: true,
    interval: 3000,
    ...over
  }
}

function healthRow(over: Partial<SourceHealthRow> = {}): SourceHealthRow {
  return {
    id: 'src_a',
    name: '源A',
    state: true,
    grade: 1,
    isMaster: false,
    status: 'healthy',
    suppressed: false,
    latencyMs: 120,
    films: 10,
    pageCount: 5,
    collected: 8,
    total: 100,
    playLatencyMs: 0,
    okCount: 3,
    probes: 3,
    consecutiveFails: 0,
    message: '',
    checkedAt: Date.now(),
    apiCheckedAt: Date.now(),
    playLatencyWeb: 0,
    playCheckedAt: 0,
    sampleM3u8: '',
    ...over
  }
}

async function mountView() {
  const w = mount(CollectListView, {
    // ManageSheet 内容 Teleport 到 body, stub 掉让弹窗内联渲染可查
    global: { stubs: { teleport: true } }
  })
  await flushPromises()
  await flushPromises()
  return w
}

function btn(w: ReturnType<typeof mount>, text: string | RegExp) {
  return w.findAll('button').find((b) =>
    typeof text === 'string' ? b.text().trim() === text : text.test(b.text())
  )
}

describe('CollectListView 采集源列表', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.list.mockResolvedValue([row()])
    h.health.mockResolvedValue([healthRow()])
    h.confirm.mockResolvedValue(true)
  })

  it('挂载即拉列表与健康面板, 渲染行与健康汇总', async () => {
    const w = await mountView()
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.health).toHaveBeenCalledTimes(1)
    expect(w.text()).toContain('源A')
    expect(w.text()).toContain('https://a.example.com/api')
    // 汇总: 1 行 1 健康 → 健康 1, 无降级/停采/未测
    expect(w.text()).toContain('健康 1')
    expect(w.text()).not.toContain('未测')
    w.unmount()
  })

  it('加载失败展示错误态与重试, 不落"暂无采集源"空态', async () => {
    h.list.mockRejectedValue(new Error('网络错误'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败: 网络错误')
    expect(w.text()).toContain('点击重试')
    expect(w.text()).not.toContain('暂无采集源')
    // 点重试成功后错误条消失
    h.list.mockResolvedValue([row()])
    await btn(w, '点击重试')!.trigger('click')
    await flushPromises()
    expect(w.text()).not.toContain('加载失败')
    w.unmount()
  })

  it('健康汇总只算现存行: 已删源的幽灵条目不把未测挤成负数', async () => {
    // 1 行 + 2 条健康(其中 src_ghost 已删) → 未测 = 1 - 1 = 0, 不出现负数
    h.health.mockResolvedValue([
      healthRow(),
      healthRow({ id: 'src_ghost', name: '幽灵', collected: 0 })
    ])
    const w = await mountView()
    expect(w.text()).not.toContain('未测')
    expect(w.text()).not.toContain('幽灵')
    w.unmount()
  })

  it('排序循环: 按采集延时把快源排前, 未测的排末尾', async () => {
    h.list.mockResolvedValue([
      row({ id: 'src_slow', name: '慢源' }),
      row({ id: 'src_a', name: '源A' }),
      row({ id: 'src_new', name: '新源' })
    ])
    h.health.mockResolvedValue([
      healthRow({ id: 'src_slow', latencyMs: 900 }),
      healthRow({ id: 'src_a', latencyMs: 120 })
      // src_new 无健康数据 → 未测, 排末尾
    ])
    const w = await mountView()
    await btn(w, '默认排序')!.trigger('click')
    const order = w.findAll('tbody tr').map((tr) => tr.text())
    expect(order[0]).toContain('源A')
    expect(order[1]).toContain('慢源')
    expect(order[2]).toContain('新源')
    w.unmount()
  })

  it('测采集成功: 静默调接口(silent 不叠通用 toast), 展示具体测速文案', async () => {
    h.test.mockResolvedValue({ ok: true, probes: 3, okCount: 3, latencyMs: 88, bestMs: 80, films: 5, message: '可用' })
    const w = await mountView()
    await btn(w, '测采集')!.trigger('click')
    await flushPromises()
    expect(h.test).toHaveBeenCalledWith('src_a', { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('采集 88ms'))
    w.unmount()
  })

  it('启停: 拉全量后翻转 state 提交, 完成后重拉列表与健康', async () => {
    h.find.mockResolvedValue(row())
    const w = await mountView()
    await btn(w, '停用')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.find).toHaveBeenCalledWith('src_a')
    expect(h.change).toHaveBeenCalledWith(expect.objectContaining({ id: 'src_a', state: false }))
    expect(h.list).toHaveBeenCalledTimes(2)
    w.unmount()
  })

  it('采集按钮: 停用源置灰禁点, 启用源点击带 silent 触发任务', async () => {
    h.list.mockResolvedValue([row({ id: 'src_off', name: '停用源', state: false }), row()])
    const w = await mountView()
    const rows = w.findAll('tbody tr')
    const offBtn = rows[0].findAll('button').find((b) => b.text().trim() === '采集')!
    expect(offBtn.attributes('disabled')).toBeDefined()
    await rows[1].findAll('button').find((b) => b.text().trim() === '采集')!.trigger('click')
    await flushPromises()
    expect(h.startSpider).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'src_a', time: 24 }),
      expect.objectContaining({ silent: true })
    )
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已触发「源A」'))
    w.unmount()
  })

  it('删除: 确认(danger)后调 remove 并清掉该源健康条目', async () => {
    const w = await mountView()
    await btn(w, '删除')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ danger: true, okText: '删除' }))
    expect(h.remove).toHaveBeenCalledWith('src_a')
    // health 重建不含已删源
    expect(w.text()).toContain('健康 1')
    w.unmount()
  })
})

describe('CollectListView 表单校验', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.list.mockResolvedValue([row()])
    h.health.mockResolvedValue([])
    h.confirm.mockResolvedValue(true)
  })

  async function openAddAndFill(
    w: ReturnType<typeof mount>,
    over: Partial<Record<'id' | 'name' | 'uri' | 'siteUrl', string>> = {}
  ): Promise<void> {
    await btn(w, '新增')!.trigger('click')
    const inputs = w.findAll('input')
    // 顺序: 资源站标识 / 名称 / 采集 URI / 站点网址 / 重置密钥(另一个弹窗, 不可见)
    const visible = inputs.filter((i) => i.isVisible())
    await visible[0].setValue(over.id ?? '')
    await visible[1].setValue(over.name ?? '')
    await visible[2].setValue(over.uri ?? '')
    if (over.siteUrl !== undefined) await visible[3].setValue(over.siteUrl)
  }

  it('空 id / 非法 id 不提交', async () => {
    const w = await mountView()
    await openAddAndFill(w, { id: '', name: 'n', uri: 'https://x.com/a' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('资源站标识不合法')
    expect(h.add).not.toHaveBeenCalled()

    await openAddAndFill(w, { id: 'BadId', name: 'n', uri: 'https://x.com/a' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('资源站标识不合法')
    expect(h.add).not.toHaveBeenCalled()
    w.unmount()
  })

  it('空名称 / 空 URI / 非 http(s) URI 不提交', async () => {
    const w = await mountView()
    await openAddAndFill(w, { id: 'src_x', name: '', uri: 'https://x.com/a' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('请填写名称')
    expect(h.add).not.toHaveBeenCalled()

    await openAddAndFill(w, { id: 'src_x', name: 'n', uri: '' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('采集 URI 需为 http(s):// 开头的完整链接')

    await openAddAndFill(w, { id: 'src_x', name: 'n', uri: 'ftp://x.com/a' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('采集 URI 需为 http(s):// 开头的完整链接')
    expect(h.add).not.toHaveBeenCalled()
    w.unmount()
  })

  it('非法站点网址不提交, 合法则调 add 并关弹窗', async () => {
    const w = await mountView()
    await openAddAndFill(w, { id: 'src_x', name: 'n', uri: 'https://x.com/a', siteUrl: 'notaurl' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('站点网址需为 http(s):// 开头的完整链接')
    expect(h.add).not.toHaveBeenCalled()

    await openAddAndFill(w, { id: 'src_x', name: 'n', uri: 'https://x.com/a', siteUrl: 'https://x.com' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.add).toHaveBeenCalled()
    expect(h.toast).not.toHaveBeenCalledWith('error', expect.anything())
    w.unmount()
  })

  it('服务端冲突错误在弹窗内联展示(如 URI 重复)', async () => {
    h.add.mockRejectedValue(new Error('该采集 URI 已被其他源使用'))
    const w = await mountView()
    await openAddAndFill(w, { id: 'src_x', name: 'n', uri: 'https://x.com/a' })
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('该采集 URI 已被其他源使用')
    w.unmount()
  })

  it('已有主站时再设主站需确认; 取消则不提交', async () => {
    // 两行: src_m 是主站, 编辑附属源 src_a 并把等级切到主站 → 应弹确认
    h.list.mockResolvedValue([row({ id: 'src_m', name: '老主站', grade: 0 }), row()])
    const w = await mountView()
    const slaveRow = w.findAll('tbody tr').find((tr) => tr.text().includes('源A'))!
    await slaveRow.findAll('button').find((b) => b.text().trim() === '编辑')!.trigger('click')
    // select 顺序: 采集时长(工具栏) / 返回类型 / 站点等级 / 资源类型
    const selects = w.findAll('select').filter((s) => s.isVisible())
    const gradeSelect = selects[2]
    await gradeSelect.setValue('0')
    h.confirm.mockResolvedValue(false)
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(expect.objectContaining({ title: '确认再设一个主站？' }))
    expect(h.update).not.toHaveBeenCalled()
    // 确认后放行
    h.confirm.mockResolvedValue(true)
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.update).toHaveBeenCalled()
    w.unmount()
  })

  it('重置库存: 空密钥内联报错, 有密钥静默提交', async () => {
    const w = await mountView()
    await btn(w, '重置库存')!.trigger('click')
    await btn(w, '确认重置')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('请输入重置密钥')
    expect(h.resetSpider).not.toHaveBeenCalled()

    const pwd = w.findAll('input[type="password"]')[0]
    await pwd.setValue('key-1')
    await btn(w, '确认重置')!.trigger('click')
    await flushPromises()
    expect(h.resetSpider).toHaveBeenCalledWith('key-1', { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('已重置'))
    w.unmount()
  })
})
