import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserListView from './UserListView.vue'

const h = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  setDisabled: vi.fn(),
  resetPassword: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    user: {
      list: h.list,
      create: h.create,
      update: h.update,
      remove: h.remove,
      setDisabled: h.setDisabled,
      resetPassword: h.resetPassword
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: vi.fn().mockResolvedValue(true) }))

async function mountView() {
  // ManageSheet 用 Teleport 挂到 body: stub 掉才能从 wrapper 里查到弹窗内的输入框
  const w = mount(UserListView, { global: { stubs: { teleport: true } } })
  await new Promise((r) => setTimeout(r, 10))
  return w
}

/** 打开「新增用户」弹窗: 返回弹窗表单与其中的输入框(用户名/密码/邮箱, 不含工具栏搜索框) */
async function openCreate(w: Awaited<ReturnType<typeof mountView>>) {
  const btn = w.findAll('button').find((b) => b.text().includes('新增用户'))
  if (!btn) throw new Error('未找到「新增用户」按钮')
  await btn.trigger('click')
  await w.vm.$nextTick()
  const form = w.findAll('form').at(-1)
  if (!form) throw new Error('弹窗未渲染')
  return { form, inputs: form.findAll('input') }
}

describe('UserListView 用户管理', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [], page: { total: 0, size: 20, current: 1 } })
    h.create.mockResolvedValue({ id: 3 })
  })

  it('新增弹窗有邮箱输入(选填), 填了就随请求提交', async () => {
    const w = await mountView()
    const { form, inputs } = await openCreate(w)
    expect(w.text()).toContain('邮箱(选填)')
    // 用户名 / 密码 / 邮箱
    expect(inputs.length).toBe(3)
    await inputs[0].setValue('newbie')
    await inputs[1].setValue('secret1')
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    expect(emailInput, '邮箱输入框应存在').toBeTruthy()
    await emailInput!.setValue('newbie@example.com')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).toHaveBeenCalledWith('newbie', 'secret1', 0, 'newbie@example.com')
  })

  it('邮箱留空 = 不绑定(仍能建号)', async () => {
    const w = await mountView()
    const { form, inputs } = await openCreate(w)
    await inputs[0].setValue('noemail')
    await inputs[1].setValue('secret1')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).toHaveBeenCalledWith('noemail', 'secret1', 0, '')
  })

  it('邮箱格式非法时前端拦下, 不发请求', async () => {
    const w = await mountView()
    const { form, inputs } = await openCreate(w)
    await inputs[0].setValue('badmail')
    await inputs[1].setValue('secret1')
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    await emailInput!.setValue('not-an-email')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('邮箱格式'))
  })

  it('工具栏: 搜索框有独立宽度容器, 桌面端不换行', async () => {
    const w = await mountView()
    const toolbar = w.find('header')
    const keyword = toolbar.find('input')
    // 输入框外层必须有自己的宽度限制, 否则 w-full 会把按钮挤到下一行
    expect(keyword.element.parentElement?.className).toContain('sm:w-[220px]')
    const row = keyword.element.parentElement?.parentElement
    expect(row?.className).toContain('sm:flex-nowrap')
  })
})
