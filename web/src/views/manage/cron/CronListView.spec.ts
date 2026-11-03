import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CronListView from './CronListView.vue'
import type { CronTask, CollectSource } from '@/types/manage'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  add: vi.fn(),
  update: vi.fn(),
  change: vi.fn(),
  remove: vi.fn(),
  runNow: vi.fn(),
  sources: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    cron: {
      list: h.list,
      add: h.add,
      update: h.update,
      change: h.change,
      remove: h.remove,
      runNow: h.runNow
    },
    collect: {
      list: h.sources
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

function row(over: Partial<CronTask> = {}): CronTask {
  return {
    id: '7',
    ids: [],
    time: 24,
    spec: '0 0 3 * * *',
    model: 0,
    state: true,
    remark: '',
    lastRunAt: 0,
    ...over
  }
}

function src(over: Partial<CollectSource> = {}): CollectSource {
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

async function mountView() {
  const w = mount(CronListView, {
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

function inputByPlaceholder(w: ReturnType<typeof mount>, ph: string) {
  return w.findAll('input').find((i) => i.attributes('placeholder') === ph)
}

describe('CronListView 定时任务列表', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.list.mockResolvedValue([row()])
    h.sources.mockResolvedValue([src()])
    h.confirm.mockResolvedValue(true)
  })

  it('挂载即拉列表, 渲染行与 state/model/lastStatus 映射', async () => {
    h.list.mockResolvedValue([
      row({
        id: '1',
        model: 0,
        state: true,
        lastStatus: 'success',
        lastRunAt: new Date(2026, 0, 2, 3, 4).getTime()
      }),
      row({ id: '2', model: 1, state: false, lastStatus: 'failed' }),
      row({ id: '3', model: 2, state: true, lastStatus: 'running' }),
      row({ id: '4', model: 0, state: true, lastStatus: 'canceled' }),
      row({ id: '5', model: 0, state: true, lastStatus: undefined, lastRunAt: 0 })
    ])
    const w = await mountView()
    const texts = w.findAll('tbody tr').map((tr) => tr.text())
    expect(texts[0]).toContain('0 0 3 * * *')
    expect(texts[0]).toContain('自动')
    expect(texts[0]).toContain('24 小时')
    expect(texts[0]).toContain('运行中') // state=true 标签
    expect(texts[0]).toContain('成功') // lastStatus=success
    expect(texts[0]).toMatch(/01-02 03:04/)
    expect(texts[1]).toContain('指定')
    expect(texts[1]).toContain('停止')
    expect(texts[1]).toContain('失败')
    expect(texts[2]).toContain('补采')
    expect(texts[2]).toContain('运行中')
    expect(texts[3]).toContain('已取消')
    // lastStatus 缺省 → 无记录标签, lastRunAt=0 → '—'
    expect(texts[4]).toContain('无记录')
    w.unmount()
  })

  it('加载失败展示错误态与重试, 不落"暂无定时任务"空态', async () => {
    h.list.mockRejectedValue(new Error('网络错误'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败: 网络错误')
    expect(w.text()).toContain('点击重试')
    expect(w.text()).not.toContain('暂无定时任务')
    // 点重试成功后错误条消失
    h.list.mockResolvedValue([row()])
    await btn(w, '点击重试')!.trigger('click')
    await flushPromises()
    expect(w.text()).not.toContain('加载失败')
    w.unmount()
  })

  it('关键字过滤: 命中备注/ID/表达式, 清空恢复全量', async () => {
    h.list.mockResolvedValue([
      row({ id: '1', spec: '0 0 3 * * *', remark: '夜间全量' }),
      row({ id: '2', spec: '0 */6 * * * *', remark: '补采兜底' })
    ])
    const w = await mountView()
    expect(w.findAll('tbody tr')).toHaveLength(2)
    await inputByPlaceholder(w, '搜索 ID / 表达式 / 备注')!.setValue('夜间')
    expect(w.findAll('tbody tr')).toHaveLength(1)
    expect(w.findAll('tbody tr')[0].text()).toContain('夜间全量')
    await inputByPlaceholder(w, '搜索 ID / 表达式 / 备注')!.setValue('*/6')
    expect(w.findAll('tbody tr')).toHaveLength(1)
    expect(w.findAll('tbody tr')[0].text()).toContain('补采兜底')
    await inputByPlaceholder(w, '搜索 ID / 表达式 / 备注')!.setValue('')
    expect(w.findAll('tbody tr')).toHaveLength(2)
    w.unmount()
  })

  it('启停: 翻转 state 提交并重拉列表; 请求期间行内防重', async () => {
    let resolveChange: (v: void) => void = () => {}
    h.change.mockReturnValue(new Promise<void>((r) => (resolveChange = r)))
    const w = await mountView()
    await btn(w, '停止')!.trigger('click')
    expect(h.change).toHaveBeenCalledWith(expect.objectContaining({ id: '7', state: false }))
    // 请求未结束: 同行再点不重复提交(防连点), 其余行内按钮禁用
    await btn(w, '停止')!.trigger('click')
    expect(h.change).toHaveBeenCalledTimes(1)
    expect(btn(w, '立即执行')!.attributes('disabled')).toBeDefined()
    expect(btn(w, '编辑')!.attributes('disabled')).toBeDefined()
    expect(btn(w, '删除')!.attributes('disabled')).toBeDefined()
    resolveChange()
    await flushPromises()
    expect(h.list).toHaveBeenCalledTimes(2)
    expect(btn(w, '编辑')!.attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it('启停失败: 清除 busy 并恢复按钮, 错误交全局拦截器', async () => {
    h.change.mockRejectedValue(new Error('boom'))
    const w = await mountView()
    await btn(w, '停止')!.trigger('click')
    await flushPromises()
    expect(btn(w, '编辑')!.attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it('立即执行: 确认后调 runNow 并重拉; 取消不调', async () => {
    const w = await mountView()
    await btn(w, '立即执行')!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ title: '立即执行', okText: '执行' })
    )
    expect(h.runNow).toHaveBeenCalledWith('7')
    expect(h.list).toHaveBeenCalledTimes(2)

    h.runNow.mockClear()
    h.confirm.mockResolvedValue(false)
    await btn(w, '立即执行')!.trigger('click')
    await flushPromises()
    expect(h.runNow).not.toHaveBeenCalled()
    w.unmount()
  })

  it('立即执行失败: 清除 busy 恢复按钮', async () => {
    h.runNow.mockRejectedValue(new Error('boom'))
    const w = await mountView()
    await btn(w, '立即执行')!.trigger('click')
    await flushPromises()
    expect(btn(w, '编辑')!.attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it('已停用任务的"立即执行"禁点, 启停按钮文案为"启动"', async () => {
    h.list.mockResolvedValue([row({ id: '8', state: false })])
    const w = await mountView()
    const runBtn = btn(w, '立即执行')!
    expect(runBtn.attributes('disabled')).toBeDefined()
    expect(runBtn.attributes('title')).toContain('任务已停用')
    await runBtn.trigger('click')
    await flushPromises()
    expect(h.runNow).not.toHaveBeenCalled()
    expect(btn(w, '启动')).toBeDefined()
    w.unmount()
  })

  it('状态/类型/上次运行列经 StatusTag 渲染, 时间走 formatShortDateTime, 表格 fit', async () => {
    h.list.mockResolvedValue([
      row({ id: '1', state: true, lastStatus: 'success', lastRunAt: new Date(2026, 0, 2, 3, 4).getTime() })
    ])
    const w = await mountView()
    expect(w.find('section').classes()).toContain('jc-table--fit')
    // 状态列 StatusTag(布尔键 true → 运行中/success)
    const tags = w.findAll('tbody .jc-tag')
    expect(tags.find((t) => t.text() === '运行中')!.classes()).toContain('bg-[var(--jc-success-soft)]')
    // lastStatus 列 StatusTag(成功/success)
    expect(tags.find((t) => t.text() === '成功')!.classes()).toContain('bg-[var(--jc-success-soft)]')
    // 短时间格式 01-02 03:04 由 formatShortDateTime 输出
    expect(w.text()).toMatch(/01-02 03:04/)
    w.unmount()
  })

  it('删除: 确认(danger)后调 remove 并重拉; 取消不调', async () => {
    const w = await mountView()
    await btn(w, '删除')!.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledWith(
      expect.objectContaining({ danger: true, okText: '删除' })
    )
    expect(h.remove).toHaveBeenCalledWith('7')

    h.remove.mockClear()
    h.confirm.mockResolvedValue(false)
    await btn(w, '删除')!.trigger('click')
    await flushPromises()
    expect(h.remove).not.toHaveBeenCalled()
    w.unmount()
  })
})

describe('CronListView 表单校验', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    h.list.mockResolvedValue([row()])
    h.sources.mockResolvedValue([src(), src({ id: 'src_b', name: '源B' })])
    h.confirm.mockResolvedValue(true)
  })

  async function openAdd(w: ReturnType<typeof mount>): Promise<void> {
    await btn(w, '新增')!.trigger('click')
    await flushPromises()
  }

  async function fill(
    w: ReturnType<typeof mount>,
    over: { spec?: string; time?: string } = {}
  ): Promise<void> {
    const spec = inputByPlaceholder(w, '0 0 3 * * *')!
    await spec.setValue(over.spec ?? '0 0 3 * * *')
    const time = inputByPlaceholder(w, '24')!
    await time.setValue(over.time ?? '24')
  }

  it('空 spec / 非 6 字段表达式不提交', async () => {
    const w = await mountView()
    await openAdd(w)
    await inputByPlaceholder(w, '0 0 3 * * *')!.setValue('')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('请填写 Cron 表达式')
    expect(h.add).not.toHaveBeenCalled()

    await inputByPlaceholder(w, '0 0 3 * * *')!.setValue('0 0 3 * *')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Cron 表达式需为 6 个字段')
    expect(h.add).not.toHaveBeenCalled()
    w.unmount()
  })

  it('任务类型下拉 ManageSelect 化: aria-label + 同族 class + data-focusable', async () => {
    const w = await mountView()
    await openAdd(w)
    const modelSelect = w.findAll('select').find((s) => s.isVisible())!
    expect(modelSelect.attributes('aria-label')).toBe('任务类型')
    expect(modelSelect.classes()).toContain('min-h-[var(--jc-ctrl-h)]')
    expect(modelSelect.attributes('data-focusable')).toBe('true')
    // 数字 option 原类型 emit → model=1 联动显示资源站列表
    await modelSelect.setValue('1')
    await flushPromises()
    expect(w.text()).toContain('资源站列表')
    w.unmount()
  })

  it('time 清空/为 0/非整数不提交 —— 防 0 值被服务端反转为全量采集', async () => {
    const w = await mountView()
    await openAdd(w)
    for (const bad of ['', '0', '-1', '1.5']) {
      await fill(w, { time: bad })
      await btn(w, '保存')!.trigger('click')
      await flushPromises()
      expect(w.text()).toContain('采集时长需为不小于 1 的整数')
      expect(h.add).not.toHaveBeenCalled()
    }
    w.unmount()
  })

  it('model=1 未勾选资源站不提交, 勾选后放行', async () => {
    const w = await mountView()
    await openAdd(w)
    await fill(w)
    const modelSelect = w.findAll('select').find((s) => s.isVisible())!
    await modelSelect.setValue('1')
    await flushPromises()
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('请至少勾选一个资源站')
    expect(h.add).not.toHaveBeenCalled()

    // 勾选"源A"标签后提交成功
    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(
      expect.objectContaining({ model: 1, ids: ['src_a'], time: 24 })
    )
    w.unmount()
  })

  it('合法新增: 提交裁剪后的 spec 与数字 time, 成功后关弹窗重拉', async () => {
    const w = await mountView()
    await openAdd(w)
    await inputByPlaceholder(w, '0 0 3 * * *')!.setValue('  0 0 3 * * *  ')
    await inputByPlaceholder(w, '24')!.setValue('12')
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.add).toHaveBeenCalledWith(
      expect.objectContaining({ spec: '0 0 3 * * *', time: 12, model: 0 })
    )
    expect(h.list).toHaveBeenCalledTimes(2)
    w.unmount()
  })

  it('编辑: 回填行数据, 提交走 update; ids 深拷贝不污染列表行', async () => {
    const source = row({ id: '9', model: 1, ids: ['src_a'] })
    h.list.mockResolvedValue([source])
    const w = await mountView()
    await w.findAll('tbody tr')[0].findAll('button').find((b) => b.text().trim() === '编辑')!
      .trigger('click')
    await flushPromises()
    const specInput = inputByPlaceholder(w, '0 0 3 * * *')!
    expect((specInput.element as HTMLInputElement).value).toBe('0 0 3 * * *')
    // 弹窗内勾掉 src_a 再取消勾选 → 原行 ids 不受影响
    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    await w.findAll('[role="button"]').find((t) => t.text() === '源A')!.trigger('click')
    expect(source.ids).toEqual(['src_a'])
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.update).toHaveBeenCalledWith(expect.objectContaining({ id: '9', ids: ['src_a'] }))
    expect(h.add).not.toHaveBeenCalled()
    w.unmount()
  })

  it('保存失败: 弹窗保持打开并内联展示错误', async () => {
    h.add.mockRejectedValue(new Error('服务器开小差'))
    const w = await mountView()
    await openAdd(w)
    await fill(w)
    await btn(w, '保存')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('服务器开小差')
    expect(btn(w, '保存')).toBeDefined()
    w.unmount()
  })
})
