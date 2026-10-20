import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ApplyStatusPanel from './ApplyStatusPanel.vue'
import { applyStatus, resendActivation } from '@/api/register'

vi.mock('@/api/register', () => ({
  getRegisterConfig: vi.fn(),
  sendRegisterCode: vi.fn(),
  registerAccount: vi.fn(),
  applyRegister: vi.fn(),
  applyStatus: vi.fn(),
  resendActivation: vi.fn(),
  activateAccount: vi.fn()
}))

async function mountPanel(prefillEmail = '') {
  const w = mount(ApplyStatusPanel, { props: { prefillEmail } })
  await new Promise((r) => setTimeout(r, 10))
  return w
}

describe('ApplyStatusPanel 申请进度查询', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(applyStatus).mockReset()
    vi.mocked(resendActivation).mockReset()
  })

  it('空邮箱提交 → 拦截并提示', async () => {
    const w = await mountPanel()
    await w.find('input[type="email"]').setValue('')
    await w.findAll('button').find((b) => b.text().includes('查询进度'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('请输入邮箱')
    expect(applyStatus).not.toHaveBeenCalled()
  })

  it('pending → 展示「审核中」与预期说明', async () => {
    vi.mocked(applyStatus).mockResolvedValue({ status: 'pending', reason: '', canResend: false })
    const w = await mountPanel('jerry@x.com')
    expect(applyStatus).toHaveBeenCalledWith('jerry@x.com')
    expect(w.text()).toContain('审核中')
  })

  it('approved + canResend → 展示「已通过」与重发按钮; 重发作废旧 token 发新链接', async () => {
    vi.mocked(applyStatus).mockResolvedValue({ status: 'approved', reason: '', canResend: true })
    vi.mocked(resendActivation).mockResolvedValue({ resent: true })
    const w = await mountPanel('jerry@x.com')
    expect(w.text()).toContain('已通过')

    const resendBtn = w.findAll('button').find((b) => b.text().includes('重发激活邮件'))
    expect(resendBtn).toBeTruthy()
    await resendBtn!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(resendActivation).toHaveBeenCalledWith('jerry@x.com')
    // 重发后刷新状态(第二次 applyStatus 调用)
    expect(applyStatus).toHaveBeenCalledTimes(2)
  })

  it('rejected → 展示理由 + 重新申请按钮(emit reapply)', async () => {
    vi.mocked(applyStatus).mockResolvedValue({
      status: 'rejected',
      reason: '邀请人信息无法核实',
      canResend: false
    })
    const w = await mountPanel('jerry@x.com')
    expect(w.text()).toContain('邀请人信息无法核实')
    const reapply = w.findAll('button').find((b) => b.text().includes('重新申请'))
    expect(reapply).toBeTruthy()
    await reapply!.trigger('click')
    expect(w.emitted('reapply')).toHaveLength(1)
  })

  it('none → 未找到记录 + 提交申请入口', async () => {
    vi.mocked(applyStatus).mockResolvedValue({ status: 'none', reason: '', canResend: false })
    const w = await mountPanel('new@x.com')
    expect(w.text()).toContain('未找到该邮箱的申请记录')
    const applyBtn = w.findAll('button').find((b) => b.text().includes('提交申请'))
    expect(applyBtn).toBeTruthy()
  })
})
