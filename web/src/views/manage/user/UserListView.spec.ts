import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserListView from './UserListView.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import { confirm } from '@/composables/useConfirm'
import { useUserStore } from '@/stores/user'

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

describe('UserListView 编辑用户邮箱', () => {
  const row = {
    ID: 9,
    userName: 'bob',
    role: 0,
    disabled: 0,
    email: 'bob@example.com',
    CreatedAt: '2026-10-01T10:00:00Z',
    UpdatedAt: '2026-10-01T10:00:00Z'
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [row], page: { total: 1, size: 20, current: 1 } })
    h.update.mockResolvedValue({ id: 9 })
  })

  async function openEdit() {
    const w = await mountView()
    const btn = w.findAll('button').find((b) => b.text() === '编辑')
    if (!btn) throw new Error('未找到「编辑」按钮')
    await btn.trigger('click')
    await w.vm.$nextTick()
    const form = w.findAll('form').at(-1)!
    return { w, form, inputs: form.findAll('input') }
  }

  it('表格显示邮箱列(有邮箱展示原文, 无邮箱显示 —)', async () => {
    const w = await mountView()
    expect(w.text()).toContain('邮箱')
    expect(w.text()).toContain('bob@example.com')

    h.list.mockResolvedValue({
      list: [{ ...row, ID: 10, userName: 'noemail', email: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    const w2 = await mountView()
    const cells = w2.findAll('td, span').filter((c) => c.text() === '—')
    expect(cells.length, '无邮箱行应有 — 占位').toBeGreaterThan(0)
  })

  it('编辑弹窗回填现有邮箱, 改动后随更新请求提交', async () => {
    const { form, inputs } = await openEdit()
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    expect(emailInput, '编辑弹窗应有邮箱输入').toBeTruthy()
    expect((emailInput!.element as HTMLInputElement).value).toBe('bob@example.com')

    await emailInput!.setValue('bob2@example.com')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.update).toHaveBeenCalledWith(9, 'bob', 0, 'bob2@example.com')
  })

  it('清空邮箱 = 解绑(仍提交空串, 由服务端写 NULL)', async () => {
    const { form, inputs } = await openEdit()
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    await emailInput!.setValue('')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.update).toHaveBeenCalledWith(9, 'bob', 0, '')
  })
})

// ---- 危险操作(禁用/删除/重置密码)与自保护 ----

describe('UserListView 危险操作与自保护', () => {
  const row = {
    ID: 9,
    userName: 'bob',
    role: 0,
    disabled: 0,
    email: '',
    CreatedAt: '2026-10-01T10:00:00Z',
    UpdatedAt: '2026-10-01T10:00:00Z'
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [row], page: { total: 1, size: 20, current: 1 } })
    vi.mocked(confirm).mockResolvedValue(true)
  })

  it('禁用: confirm 弹文案与危险样式, 确认后 setDisabled(id, true) 并刷新', async () => {
    const w = await mountView()
    const btn = w.findAll('tbody button').find((b) => b.text() === '禁用')!
    await btn.trigger('click')
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '禁用「bob」？',
        danger: true,
        okText: '禁用'
      })
    )
    await new Promise((r) => setTimeout(r, 10))
    expect(h.setDisabled).toHaveBeenCalledWith(9, true)
    expect(h.toast).toHaveBeenCalledWith('success', '已禁用')
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('启用: 同一按钮按当前状态翻转文案与参数', async () => {
    h.list.mockResolvedValue({
      list: [{ ...row, disabled: 1 }],
      page: { total: 1, size: 20, current: 1 }
    })
    const w = await mountView()
    const btn = w.findAll('tbody button').find((b) => b.text() === '启用')!
    await btn.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.setDisabled).toHaveBeenCalledWith(9, false)
    expect(h.toast).toHaveBeenCalledWith('success', '已启用')
  })

  it('禁用失败: toast error, 不误报成功', async () => {
    h.setDisabled.mockRejectedValue(new Error('服务器繁忙'))
    const w = await mountView()
    await w.findAll('tbody button').find((b) => b.text() === '禁用')!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.toast).toHaveBeenCalledWith('error', '服务器繁忙')
  })

  it('删除: confirm 描述不可恢复, 确认后 remove(id) 并刷新', async () => {
    const w = await mountView()
    await w.findAll('tbody button').find((b) => b.text() === '删除')!.trigger('click')
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '删除「bob」？',
        danger: true,
        okText: '删除'
      })
    )
    await new Promise((r) => setTimeout(r, 10))
    expect(h.remove).toHaveBeenCalledWith(9)
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('自己的行: 禁用/重置密码/删除按钮 disabled, 编辑仍可用', async () => {
    setActivePinia(createPinia())
    useUserStore().info = { id: 9 }
    const w = await mountView()
    const buttons = Object.fromEntries(
      w.findAll('tbody button').map((b) => [b.text(), b])
    )
    expect(buttons['禁用']!.attributes('disabled')).toBeDefined()
    expect(buttons['重置密码']!.attributes('disabled')).toBeDefined()
    expect(buttons['删除']!.attributes('disabled')).toBeDefined()
    expect(buttons['编辑']!.attributes('disabled')).toBeUndefined()
  })

  it('重置密码: 弹窗提交, <6 位前端拦截, 合法后 resetPassword(id, pw)', async () => {
    const w = await mountView()
    await w.findAll('tbody button').find((b) => b.text() === '重置密码')!.trigger('click')
    await w.vm.$nextTick()
    const form = w.findAll('form').at(-1)!
    const pw = form.find('input[type="password"]')
    expect(pw.exists()).toBe(true)

    await pw.setValue('123')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.resetPassword).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '密码长度需在 6-64 位之间')

    await pw.setValue('newpass1')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.resetPassword).toHaveBeenCalledWith(9, 'newpass1')
  })

  it('改自己的角色被前端拦截(select 锁定 + 提交检查)', async () => {
    setActivePinia(createPinia())
    useUserStore().info = { id: 9 }
    const w = await mountView()
    await w.findAll('tbody button').find((b) => b.text() === '编辑')!.trigger('click')
    await w.vm.$nextTick()
    const form = w.findAll('form').at(-1)!
    const select = form.find('select')
    // 角色下拉对自己的行锁定
    expect((select.element as HTMLSelectElement).disabled).toBe(true)
    // 程序化解锁改值, 覆盖提交时的第二道拦截(正常 UI 改不动, 防御纵深)
    ;(select.element as HTMLSelectElement).disabled = false
    await select.setValue('1')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.update).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '不能修改自己的角色')
  })

  it('建号用户名长度按字符数与后端 runeLen 同口径: 32 个汉字放行, 33 个拦截', async () => {
    const w = await mountView()
    const { form, inputs } = await openCreate(w)
    await inputs[0].setValue('汉'.repeat(33))
    await inputs[1].setValue('secret1')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '用户名需为 1-32 个字符')

    await inputs[0].setValue('汉'.repeat(32))
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).toHaveBeenCalledWith('汉'.repeat(32), 'secret1', 0, '')
  })

  it('建号密码 <6 位前端拦截, 不发请求', async () => {
    const w = await mountView()
    const { form, inputs } = await openCreate(w)
    await inputs[0].setValue('shortpw')
    await inputs[1].setValue('12345')
    await form.trigger('submit')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '密码长度需在 6-64 位之间')
  })
})

// ---- 加载/搜索/翻页 ----

describe('UserListView 加载与翻页', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [], page: { total: 0, size: 20, current: 1 } })
    vi.mocked(confirm).mockResolvedValue(true)
  })

  it('搜索: 关键字随请求提交且回到第 1 页', async () => {
    const w = await mountView()
    expect(h.list).toHaveBeenCalledWith({ keyword: '', page: 1, size: 20 })
    const toolbar = w.find('header')
    await toolbar.find('input').setValue('ali')
    await toolbar.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: 'ali', page: 1, size: 20 })
  })

  it('翻页: 页码随请求提交', async () => {
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', page: 2, size: 20 })
  })

  it('加载失败: 呈现错误态与重试按钮, 不冒充"未找到用户"; 重试成功恢复列表', async () => {
    h.list.mockRejectedValueOnce(new Error('网络断了'))
    const w = await mountView()
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('未找到用户')

    h.list.mockResolvedValue({
      list: [{ ID: 1, userName: 'alice', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('alice')
  })

  it('末页删空回夹: 删除后页码越界自动回夹到最后一页重拉', async () => {
    let total = 21 // 第 2 页只有 1 条
    let page2Rows = [{ ID: 9, userName: 'tail', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }]
    h.list.mockImplementation(async (p: { page?: number }) => ({
      list: p.page === 2 ? page2Rows : [],
      page: { total, size: 20, current: p.page ?? 1 }
    }))
    h.remove.mockImplementation(async () => {
      total = 20 // 删除后只剩 20 条 = 1 页, 原第 2 页的行消失
      page2Rows = []
    })
    const w = await mountView()
    w.findComponent(BasePagination).vm.$emit('change', 2)
    await new Promise((r) => setTimeout(r, 10))
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', page: 2, size: 20 })

    await w.findAll('tbody button').find((b) => b.text() === '删除')!.trigger('click')
    await new Promise((r) => setTimeout(r, 20))
    expect(h.remove).toHaveBeenCalledWith(9)
    // 越界的第 2 页被自动回夹回第 1 页
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', page: 1, size: 20 })
  })

  it('竞态守卫: 旧响应后到不覆盖新结果', async () => {
    let resolveStale!: (v: unknown) => void
    h.list.mockImplementationOnce(() => new Promise((res) => (resolveStale = res)))
    h.list.mockImplementationOnce(async () => ({
      list: [{ ID: 2, userName: 'fresh', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }],
      page: { total: 1, size: 20, current: 1 }
    }))
    const w = await mountView()
    // 初始 load(seq=1)仍挂起, 点搜索发起 load(seq=2)并先返回
    w.find('header').find('input').setValue('x')
    await w.find('header').findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('fresh')

    resolveStale({ list: [{ ID: 1, userName: 'stale', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }], page: { total: 1, size: 20, current: 1 } })
    await new Promise((r) => setTimeout(r, 10))
    expect(w.text()).toContain('fresh')
    expect(w.text()).not.toContain('stale')
  })
})
