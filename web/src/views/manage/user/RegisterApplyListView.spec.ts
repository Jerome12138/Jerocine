import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, DOMWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import RegisterApplyListView from './RegisterApplyListView.vue'
import BasePagination from '@/components/base/BasePagination.vue'
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

import { confirm } from '@/composables/useConfirm'

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

async function mountView() {
  const w = mount(RegisterApplyListView)
  await new Promise((r) => setTimeout(r, 10))
  return w
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
  })

  it('挂载即拉列表(默认 status=-1 不限)', async () => {
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
    const w = await mountView()
    expect(h.list).toHaveBeenCalledWith({ keyword: '', status: -1, page: 1, size: 20 })
    expect(w.text()).toContain('jerry')
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
    const w = await mountView()
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
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
    h.approve.mockResolvedValue({ id: 1, approved: true, warning: '535 auth failed' })
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text() === '通过')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(confirm).toHaveBeenCalled()
    expect(h.approve).toHaveBeenCalledWith(1)
    expect(h.toast).toHaveBeenCalledWith('warning', expect.stringContaining('535 auth failed'))
    // 操作后刷新列表
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('驳回: 空理由拦截, 填理由后调 reject', async () => {
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
    h.reject.mockResolvedValue({ id: 1, rejected: true, warning: '' })
    const w = await mountView()

    await w.findAll('tbody tr button').find((b) => b.text() === '驳回')!.trigger('click')
    await w.vm.$nextTick()
    // ManageSheet Teleport 到 body: 表单不在 wrapper 树内, 从 document 取
    const form = document.body.querySelector('form')
    expect(form).toBeTruthy()

    // 理由为空直接提交 → 拦截
    form!.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((r) => setTimeout(r, 10))
    expect(h.reject).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请填写驳回理由（用户可见）')

    // 超过 255 字 → 拦截(驳回理由列是 VARCHAR(255), 与申请人自述 reason 的 500 不同)
    await new DOMWrapper(document.body.querySelector('textarea')!).setValue('理'.repeat(256))
    form!.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((r) => setTimeout(r, 10))
    expect(h.reject).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '驳回理由不能超过 255 字')

    // 填理由后再提交
    await new DOMWrapper(document.body.querySelector('textarea')!).setValue('邀请人无法核实')
    form!.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((r) => setTimeout(r, 10))
    expect(h.reject).toHaveBeenCalledWith(1, '邀请人无法核实')
  })

  it('补发: approved 未激活行 → resendActivation', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 7, status: 1 })],
      page: { current: 1, size: 20, total: 1 }
    })
    h.resendActivation.mockResolvedValue({ id: 7, resent: true, warning: '' })
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
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

  it('状态筛选: 切换重置页码并按新 status 拉取', async () => {
    const w = await mountView()
    // 先翻到第 2 页
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: -1, page: 2, size: 20 })
    // 切状态 → 回第 1 页 + 新 status
    await w.find('select').setValue('0')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: 0, page: 1, size: 20 })
  })

  it('翻页: 页码随请求提交', async () => {
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 3)
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', status: -1, page: 3, size: 20 })
  })

  it('关键字搜索: 用户名/邮箱关键字随请求提交且回到第 1 页', async () => {
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await new Promise((r) => setTimeout(r, 10))
    await w.find('header input').setValue('u1@qq')
    await w.findAll('header button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: 'u1@qq', status: -1, page: 1, size: 20 })
  })

  it('补发返回 warning: toast"补发失败"原文, 且不刷新列表(现有行为锁定)', async () => {
    h.list.mockResolvedValue({
      list: [row({ id: 7, status: 1 })],
      page: { current: 1, size: 20, total: 1 }
    })
    h.resendActivation.mockResolvedValue({ id: 7, resent: true, warning: '535 auth failed' })
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
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
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text().includes('补发激活邮件'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('error', 'SMTP 挂了')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
  })

  it('通过请求异常(catch): toast error, 不刷新列表', async () => {
    h.approve.mockRejectedValue(new Error('已被别人审了'))
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text() === '通过')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(confirm).toHaveBeenCalled()
    expect(h.approve).toHaveBeenCalledWith(1)
    expect(h.toast).toHaveBeenCalledWith('error', '已被别人审了')
    expect(h.toast).not.toHaveBeenCalledWith('success', expect.anything())
    expect(h.list).toHaveBeenCalledTimes(1)
  })

  it('驳回请求异常(catch): toast error, 弹窗不关闭', async () => {
    h.reject.mockRejectedValue(new Error('状态已变化'))
    const w = await mountView()
    await w.findAll('tbody tr button').find((b) => b.text() === '驳回')!.trigger('click')
    await w.vm.$nextTick()
    const form = document.body.querySelector('form')!
    await new DOMWrapper(document.body.querySelector('textarea')!).setValue('理由')
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((r) => setTimeout(r, 10))
    expect(h.reject).toHaveBeenCalledWith(1, '理由')
    expect(h.toast).toHaveBeenCalledWith('error', '状态已变化')
  })

  it('已驳回行: 状态旁展示驳回理由, 申请时间下附审核时间', async () => {
    h.list.mockResolvedValue({
      list: [
        row({ id: 4, userName: 'rejected1', status: 2, rejectReason: '信息不实', reviewedAt: '2026-10-21T10:00:00Z' })
      ],
      page: { current: 1, size: 20, total: 1 }
    })
    const w = await mountView()
    expect(w.text()).toContain('理由：信息不实')
    expect(w.text()).toContain('审核：')
    // 待审行不出现驳回理由与审核时间
    h.list.mockResolvedValue({ list: [row()], page: { current: 1, size: 20, total: 1 } })
    const w2 = await mountView()
    expect(w2.text()).not.toContain('理由：')
    expect(w2.text()).not.toContain('审核：')
  })

  it('加载失败: 错误态+重试按钮, 不冒充"暂无注册申请"', async () => {
    h.list.mockRejectedValueOnce(new Error('网络断了'))
    const w = await mountView()
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('暂无注册申请')

    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('jerry')
  })

  it('竞态守卫: 慢的旧响应后到不覆盖新列表', async () => {
    let resolveStale!: (v: unknown) => void
    h.list.mockImplementationOnce(() => new Promise((res) => (resolveStale = res)))
    h.list.mockImplementationOnce(async () => ({
      list: [row({ id: 2, userName: 'fresh' })],
      page: { current: 1, size: 20, total: 1 }
    }))
    const w = await mountView()
    await w.find('select').setValue('0')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('fresh')

    resolveStale({ list: [row({ id: 1, userName: 'stale' })], page: { current: 1, size: 20, total: 1 } })
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('fresh')
    expect(w.text()).not.toContain('stale')
  })
})
