import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RegisterApplyListView from './RegisterApplyListView.vue'
import RegisterRejectDialog from './RegisterRejectDialog.vue'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import ManageTable from '@/components/manage/ManageTable.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'
import { confirm } from '@/composables/useConfirm'
import type { RegisterApplyRow } from '@/types/manage'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  approve: vi.fn(),
  reject: vi.fn(),
  resendActivation: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    register: {
      list: h.list,
      approve: h.approve,
      reject: h.reject,
      resendActivation: h.resendActivation
    }
  }
}))

vi.mock('@/api/http', () => ({
  toast: h.toast
}))

vi.mock('@/composables/useConfirm', () => ({
  confirm: vi.fn().mockResolvedValue(true)
}))

function row(over: Partial<RegisterApplyRow> = {}): RegisterApplyRow {
  return {
    id: 1,
    userName: 'jerry',
    email: 'jerry@x.com',
    reason: '朋友邀请',
    sourceIp: '1.2.3.4',
    status: 0,
    rejectReason: '',
    reviewedBy: 0,
    reviewedAt: null,
    activatedAt: null,
    createdAt: '2026-10-20T10:00:00Z',
    ...over
  }
}

/** 页面 + 弹窗 Host 一起挂(共享 useManageDialog 模块单例); 弹窗内容断言走 Host wrapper */
async function mountAll(): Promise<{ w: VueWrapper; wh: VueWrapper }> {
  setActivePinia(createPinia())
  const w = mount(RegisterApplyListView, {
    global: { stubs: { Teleport: true } }
  })
  const wh = mount(ManageDialogHost, {
    attachTo: document.body,
    global: { stubs: { Teleport: true } }
  })
  await flushPromises()
  return { w, wh }
}

/** 单例复位(测试间隔离) */
async function resetDialogSingleton(): Promise<void> {
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
}

describe('RegisterApplyListView 注册审核', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    h.list.mockReset()
    h.approve.mockReset()
    h.reject.mockReset()
    h.resendActivation.mockReset()
    h.toast.mockReset()
    vi.mocked(confirm).mockResolvedValue(true)
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
  })

  afterEach(resetDialogSingleton)

  it('挂载即拉列表(默认 status=-1 不限)', async () => {
    const { w } = await mountAll()
    expect(h.list).toHaveBeenCalledWith({ keyword: '', status: -1, page: 1, size: 20 })
    expect(w.text()).toContain('jerry')
    expect(w.text()).toContain('待审')
  })

  it('状态列走 StatusTag 映射', async () => {
    const { w } = await mountAll()
    const tag = w.findComponent(StatusTag)
    expect(tag.exists(), '状态列应使用 StatusTag').toBe(true)
    expect(tag.props('value')).toBe(0)
    expect(w.text()).toContain('待审')
  })

  it('pending 行: 通过/驳回; approved 未激活: 补发; 已激活/驳回: 无操作', async () => {
    h.list.mockResolvedValue({
      list: [
        row({ id: 1, userName: 'pending1', status: 0 }),
        row({ id: 2, userName: 'approved1', status: 1 }),
        row({
          id: 3,
          userName: 'activated1',
          status: 1,
          activatedAt: '2026-10-21T10:00:00Z'
        }),
        row({ id: 4, userName: 'rejected1', status: 2, rejectReason: '信息不实' })
      ],
      page: { current: 1, size: 20, total: 4 }
    })
    const { w } = await mountAll()
    // 桌面 table 模式(默认); 每行末单元格为操作区
    const cells = w.findAll('tbody tr td:last-child')
    expect(cells).toHaveLength(4)
    expect(cells[0].text()).toContain('通过')
    expect(cells[0].text()).toContain('驳回')
    expect(cells[1].text()).toContain('补发激活邮件')
    expect(cells[2].text()).toContain('—')
    expect(cells[3].text()).toContain('—')
  })

  it('通过: 确认后调 approve, warning 时 toast 提示可补发', async () => {
    h.approve.mockResolvedValue({ id: 1, approved: true, warning: '535 auth failed' })
    const { w } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text() === '通过')!.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalled()
    expect(h.approve).toHaveBeenCalledWith(1)
    expect(h.toast).toHaveBeenCalledWith('warning', expect.stringContaining('535 auth failed'))
    // 操作后刷新列表
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('驳回: 弹窗经 openManageDialog 发起; 空理由拦截, 填理由后调 reject', async () => {
    h.reject.mockResolvedValue({ id: 1, rejected: true, warning: '' })
    const { w, wh } = await mountAll()

    await w.findAll('tbody tr button').find((b) => b.text() === '驳回')!.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(RegisterRejectDialog)
    expect(dlg.exists(), '驳回应经 openManageDialog 发起').toBe(true)
    expect(dlg.props('target')).toMatchObject({ id: 1 })
    // 手写 textarea 已被 ManageTextarea 替代(maxlength=255)
    expect(dlg.findComponent(ManageTextarea).exists()).toBe(true)
    const textarea = dlg.find('textarea')
    expect(textarea.attributes('maxlength')).toBe('255')

    // 理由为空直接提交 → 拦截
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.reject).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请填写驳回理由（用户可见）')

    // 超过 255 字 → 拦截(驳回理由列是 VARCHAR(255), 与申请人自述 reason 的 500 不同)
    await textarea.setValue('理'.repeat(256))
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.reject).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '驳回理由不能超过 255 字')

    // 填理由后再提交
    await textarea.setValue('邀请人无法核实')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.reject).toHaveBeenCalledWith(1, '邀请人无法核实')
  })

  it('补发: approved 未激活行 → resendActivation', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 7, status: 1 })],
      page: { current: 1, size: 20, total: 1 }
    })
    h.resendActivation.mockResolvedValue({ id: 7, resent: true, warning: '' })
    const { w } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await flushPromises()
    expect(h.resendActivation).toHaveBeenCalledWith(7)
    expect(h.toast).toHaveBeenCalledWith('success', expect.stringContaining('补发'))
  })
})

// ---- 筛选/搜索/翻页/错误分支 ----

describe('RegisterApplyListView 筛选与错误分支', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    h.list.mockReset()
    h.approve.mockReset()
    h.reject.mockReset()
    h.resendActivation.mockReset()
    h.toast.mockReset()
    vi.mocked(confirm).mockResolvedValue(true)
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
  })

  afterEach(resetDialogSingleton)

  it('状态筛选换 ManageSelect: 切换重置页码并按新 status 拉取', async () => {
    const { w } = await mountAll()
    // 先翻到第 2 页(ManageTable 内置分页)
    w.findComponent(ManageTable).vm.$emit('update:current', 2)
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: -1, page: 2, size: 20 })
    // 切状态 → 回第 1 页 + 新 status(数字 value 原类型; ManageSelect v2: 打开浮层点选「待审」)
    await w.find('button[aria-haspopup="listbox"]').trigger('click')
    await w.findAll('[role="option"]').find((o) => o.text().includes('待审'))!.trigger('click')
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: 0, page: 1, size: 20 })
  })

  it('翻页: 页码随请求提交', async () => {
    const { w } = await mountAll()
    w.findComponent(ManageTable).vm.$emit('update:current', 3)
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: -1, page: 3, size: 20 })
  })

  it('关键字搜索: 用户名/邮箱关键字随请求提交且回到第 1 页', async () => {
    const { w } = await mountAll()
    w.findComponent(ManageTable).vm.$emit('update:current', 2)
    await flushPromises()
    await w.find('header input').setValue('u1@qq')
    await w.findAll('header button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: 'u1@qq', status: -1, page: 1, size: 20 })
  })

  it('补发返回 warning: toast"补发失败"原文, 且不刷新列表(现有行为锁定)', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 7, status: 1 })],
      page: { current: 1, size: 20, total: 1 }
    })
    h.resendActivation.mockResolvedValue({ id: 7, resent: true, warning: '535 auth failed' })
    const { w } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('warning', expect.stringContaining('补发失败'))
    expect(h.toast).toHaveBeenCalledWith('warning', expect.stringContaining('535 auth failed'))
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
    // 补发不改变申请状态, 不刷新列表
    expect(h.list).toHaveBeenCalledTimes(1)
  })

  it('补发请求异常(catch): toast error 原文', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 7, status: 1 })],
      page: { current: 1, size: 20, total: 1 }
    })
    h.resendActivation.mockRejectedValue(new Error('SMTP 挂了'))
    const { w } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('error', 'SMTP 挂了')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
  })

  it('通过请求异常(catch): toast error, 不刷新列表', async () => {
    h.approve.mockRejectedValue(new Error('已被别人审了'))
    const { w } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text() === '通过')!.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenCalled()
    expect(h.approve).toHaveBeenCalledWith(1)
    expect(h.toast).toHaveBeenCalledWith('error', '已被别人审了')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
    expect(h.list).toHaveBeenCalledTimes(1)
  })

  it('驳回请求异常(catch): toast error, 弹窗不关闭', async () => {
    h.reject.mockRejectedValue(new Error('状态已变化'))
    const { w, wh } = await mountAll()
    await w.findAll('tbody tr button').find((b) => b.text() === '驳回')!.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(RegisterRejectDialog)
    await dlg.find('textarea').setValue('理由')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.reject).toHaveBeenCalledWith(1, '理由')
    expect(h.toast).toHaveBeenCalledWith('error', '状态已变化')
    // 失败弹窗停留: confirm 未 emit, 单例仍持有该弹窗
    expect(manageDialogState.visible).toBe(true)
  })

  it('已驳回行: 状态旁展示驳回理由, 申请时间下附审核时间', async () => {
    h.list.mockResolvedValue({
      list: [
        row({ id: 4, userName: 'rejected1', status: 2, rejectReason: '信息不实', reviewedAt: '2026-10-21T10:00:00Z' })
      ],
      page: { current: 1, size: 20, total: 1 }
    })
    const { w } = await mountAll()
    expect(w.text()).toContain('理由：信息不实')
    expect(w.text()).toContain('审核：')
    // 待审行不出现驳回理由与审核时间
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
    const w2 = (await mountAll()).w
    expect(w2.text()).not.toContain('理由：')
    expect(w2.text()).not.toContain('审核：')
  })

  it('加载失败: 错误态+重试按钮, 不冒充"暂无注册申请"', async () => {
    h.list.mockRejectedValueOnce(new Error('网络断了'))
    const { w } = await mountAll()
    await flushPromises()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('暂无注册申请')

    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('jerry')
  })

  it('竞态守卫: 慢的旧响应后到不覆盖新列表', async () => {
    let resolveStale!: (v: unknown) => void
    h.list.mockImplementationOnce(() => new Promise((res) => (resolveStale = res)))
    h.list.mockImplementationOnce(async () => ({
      list: [row({ id: 2, userName: 'fresh' })],
      page: { current: 1, size: 20, total: 1 }
    }))
    const { w } = await mountAll()
    await w.find('button[aria-haspopup="listbox"]').trigger('click')
    await w.findAll('[role="option"]').find((o) => o.text().includes('待审'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('fresh')

    resolveStale({ list: [row({ id: 1, userName: 'stale' })], page: { current: 1, size: 20, total: 1 } })
    await flushPromises()
    expect(w.text()).toContain('fresh')
    expect(w.text()).not.toContain('stale')
  })
})
