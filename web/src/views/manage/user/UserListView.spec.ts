import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserListView from './UserListView.vue'
import UserFormDialog from './UserFormDialog.vue'
import UserResetPasswordDialog from './UserResetPasswordDialog.vue'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import ManageTable from '@/components/manage/ManageTable.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'
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

/** 页面 + 弹窗 Host 一起挂(共享 useManageDialog 模块单例); 弹窗内容断言走 Host wrapper */
async function mountAll(): Promise<{ w: VueWrapper; wh: VueWrapper }> {
  setActivePinia(createPinia())
  const w = mount(UserListView, {
    global: { stubs: { Teleport: true } }
  })
  const wh = mount(ManageDialogHost, {
    attachTo: document.body,
    global: { stubs: { Teleport: true } }
  })
  await flushPromises()
  return { w, wh }
}

const SILENT = { silent: true }

describe('UserListView 用户管理(列表骨架/分页/状态列)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [], page: { total: 0, size: 20, current: 1 } })
    h.create.mockResolvedValue({ id: 3 })
  })

  afterEach(async () => {
    document.body.innerHTML = ''
    // useManageDialog 模块单例复位(测试间隔离, 同 useManageDialog.spec)
    manageDialogHostMounted.value = false
    if (manageDialogState.resolve) {
      answerManageDialog({ type: 'close' })
    }
    manageDialogState.visible = false
    manageDialogState.title = ''
    manageDialogState.component = null
    manageDialogState.props = {}
    manageDialogState.mobileMode = 'sheet'
    manageDialogState.width = undefined
    await flushPromises()
  })

  it('挂载即拉列表(第 1 页, 20 条), 关键字随闭包', async () => {
    const { w } = await mountAll()
    expect(h.list).toHaveBeenCalledWith({ keyword: '', page: 1, size: 20 })
    expect(w.findComponent(ManageTable).exists()).toBe(true)
  })

  it('内置分页: total>0 渲染 ManageTable 分页 footer, update:current 驱动翻页', async () => {
    h.list.mockResolvedValue({
      list: Array.from({ length: 21 }, (_, i) => ({
        ID: i + 1,
        userName: `u${i + 1}`,
        role: 0,
        disabled: 0,
        CreatedAt: '',
        UpdatedAt: ''
      })),
      page: { total: 21, size: 20, current: 1 }
    })
    const { w } = await mountAll()
    expect(w.find('.jc-table__pagination').exists()).toBe(true)
    // 页面顶层不再有独立 BasePagination 拼接
    w.findComponent(ManageTable).vm.$emit('update:current', 2)
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', page: 2, size: 20 })
  })

  it('搜索: 关键字随请求提交且回到第 1 页', async () => {
    const { w } = await mountAll()
    const toolbar = w.find('header')
    await toolbar.find('input').setValue('ali')
    await toolbar.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: 'ali', page: 1, size: 20 })
  })

  it('搜索框有独立宽度容器, 桌面端不换行', async () => {
    const { w } = await mountAll()
    const toolbar = w.find('header')
    const keyword = toolbar.find('input')
    expect(keyword.element.parentElement?.className).toContain('sm:w-[220px]')
    const row = keyword.element.parentElement?.parentElement
    expect(row?.className).toContain('sm:flex-nowrap')
  })

  it('加载失败: 呈现错误态与重试按钮, 不冒充"未找到用户"; 重试成功恢复列表', async () => {
    h.list.mockRejectedValueOnce(new Error('网络断了'))
    const { w } = await mountAll()
    await flushPromises()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('未找到用户')

    h.list.mockResolvedValue({
      list: [{ ID: 1, userName: 'alice', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
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
      total = 20
      page2Rows = []
    })
    const { w } = await mountAll()
    w.findComponent(ManageTable).vm.$emit('update:current', 2)
    await flushPromises()
    expect(h.list).toHaveBeenLastCalledWith({ keyword: '', page: 2, size: 20 })

    await w.findAll('tbody button').find((b) => b.text() === '删除')!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(h.remove).toHaveBeenCalledWith(9, SILENT)
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
    const { w } = await mountAll()
    // 初始 load 仍挂起, 点搜索发起第二次 load 并先返回
    w.find('header').find('input').setValue('x')
    await w.find('header').findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('fresh')

    resolveStale({
      list: [{ ID: 1, userName: 'stale', role: 0, disabled: 0, CreatedAt: '', UpdatedAt: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    await flushPromises()
    expect(w.text()).toContain('fresh')
    expect(w.text()).not.toContain('stale')
  })

  it('角色/状态列走 StatusTag 映射', async () => {
    h.list.mockResolvedValue({
      list: [{ ID: 1, userName: 'alice', role: 1, disabled: 0, CreatedAt: '', UpdatedAt: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    const { w } = await mountAll()
    const tags = w.findAllComponents(StatusTag)
    expect(tags.length).toBeGreaterThanOrEqual(2)
    // 管理员 role=1 + 正常 disabled=0 都有映射命中
    expect(w.text()).toContain('管理员')
    expect(w.text()).toContain('正常')
  })
})

describe('UserListView 新增/编辑弹窗(函数化)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(h).forEach((fn) => fn.mockReset())
    h.list.mockResolvedValue({ list: [], page: { total: 0, size: 20, current: 1 } })
    h.create.mockResolvedValue({ id: 3 })
  })

  afterEach(async () => {
    document.body.innerHTML = ''
    manageDialogHostMounted.value = false
    if (manageDialogState.resolve) answerManageDialog({ type: 'close' })
    manageDialogState.visible = false
    manageDialogState.component = null
    manageDialogState.resolve = null
    await flushPromises()
  })

  async function openCreate() {
    const { w, wh } = await mountAll()
    const btn = w.findAll('button').find((b) => b.text().includes('新增用户'))
    if (!btn) throw new Error('未找到「新增用户」按钮')
    await btn.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(UserFormDialog)
    expect(dlg.exists(), '新增用户应经 openManageDialog 发起').toBe(true)
    return { w, wh, dlg }
  }

  it('新增: 弹窗经 ManageDialogHost 渲染, 邮箱填了随请求提交(带 silent)', async () => {
    const { w, dlg } = await openCreate()
    expect(dlg.props('initial')).toBeNull()
    expect(w.text()).toContain('新增用户')
    const form = dlg.find('form')
    const inputs = form.findAll('input')
    // 用户名 / 密码 / 邮箱
    expect(inputs.length).toBe(3)
    expect(dlg.text()).toContain('邮箱')
    await inputs[0].setValue('newbie')
    await inputs[1].setValue('secret1')
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    expect(emailInput, '邮箱输入框应存在').toBeTruthy()
    await emailInput!.setValue('newbie@example.com')
    await form.trigger('submit')
    await flushPromises()
    expect(h.create).toHaveBeenCalledWith('newbie', 'secret1', 0, 'newbie@example.com', SILENT)
    // 提交成功 → emit confirm → 弹窗关闭 → 页面重拉列表
    expect(h.list.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('邮箱留空 = 不绑定(仍能建号)', async () => {
    const { dlg } = await openCreate()
    const inputs = dlg.find('form').findAll('input')
    await inputs[0].setValue('noemail')
    await inputs[1].setValue('secret1')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).toHaveBeenCalledWith('noemail', 'secret1', 0, '', SILENT)
  })

  it('邮箱格式非法时前端拦下, 不发请求', async () => {
    const { dlg } = await openCreate()
    const inputs = dlg.find('form').findAll('input')
    await inputs[0].setValue('badmail')
    await inputs[1].setValue('secret1')
    const emailInput = inputs.find((i) => (i.element as HTMLInputElement).type === 'email')
    await emailInput!.setValue('not-an-email')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', expect.stringContaining('邮箱格式'))
  })

  it('建号用户名长度按字符数与后端 runeLen 同口径: 33 个拦截, 32 个放行', async () => {
    const { dlg } = await openCreate()
    const inputs = dlg.find('form').findAll('input')
    await inputs[0].setValue('汉'.repeat(33))
    await inputs[1].setValue('secret1')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '用户名需为 1-32 个字符')

    await inputs[0].setValue('汉'.repeat(32))
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).toHaveBeenCalledWith('汉'.repeat(32), 'secret1', 0, '', SILENT)
  })

  it('建号密码 <6 位前端拦截, 不发请求', async () => {
    const { dlg } = await openCreate()
    const inputs = dlg.find('form').findAll('input')
    await inputs[0].setValue('shortpw')
    await inputs[1].setValue('12345')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '密码长度需在 6-64 位之间')
  })

  it('角色下拉换 ManageSelect(数字 value 原类型回传)', async () => {
    const { dlg } = await openCreate()
    const sel = dlg.findComponent(ManageSelect)
    expect(sel.exists(), '角色应使用 ManageSelect').toBe(true)
    expect(sel.props('options')).toEqual([
      { label: '普通用户', value: 0 },
      { label: '管理员', value: 1 }
    ])
    const inputs = dlg.find('form').findAll('input')
    await inputs[0].setValue('roleuser')
    await inputs[1].setValue('secret1')
    await sel.vm.$emit('update:modelValue', 1)
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.create).toHaveBeenCalledWith('roleuser', 'secret1', 1, '', SILENT)
  })
})

describe('UserListView 编辑用户邮箱(函数化弹窗回填)', () => {
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

  afterEach(async () => {
    document.body.innerHTML = ''
    manageDialogHostMounted.value = false
    if (manageDialogState.resolve) answerManageDialog({ type: 'close' })
    manageDialogState.visible = false
    manageDialogState.component = null
    manageDialogState.resolve = null
    await flushPromises()
  })

  async function openEdit() {
    const { w, wh } = await mountAll()
    const btn = w.findAll('button').find((b) => b.text() === '编辑')
    if (!btn) throw new Error('未找到「编辑」按钮')
    await btn.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(UserFormDialog)
    expect(dlg.exists()).toBe(true)
    return { w, dlg }
  }

  it('表格显示邮箱列(有邮箱展示原文, 无邮箱显示 —)', async () => {
    const { w } = await mountAll()
    expect(w.text()).toContain('邮箱')
    expect(w.text()).toContain('bob@example.com')

    h.list.mockResolvedValue({
      list: [{ ...row, ID: 10, userName: 'noemail', email: '' }],
      page: { total: 1, size: 20, current: 1 }
    })
    const w2 = (await mountAll()).w
    const cells = w2.findAll('td, span').filter((c) => c.text() === '—')
    expect(cells.length, '无邮箱行应有 — 占位').toBeGreaterThan(0)
  })

  it('编辑弹窗回填 initial, 改动后随更新请求提交(带 silent)', async () => {
    const { dlg } = await openEdit()
    expect(dlg.props('initial')).toMatchObject({ ID: 9, userName: 'bob' })
    const emailInput = dlg
      .find('form')
      .findAll('input')
      .find((i) => (i.element as HTMLInputElement).type === 'email')
    expect(emailInput, '编辑弹窗应有邮箱输入').toBeTruthy()
    expect((emailInput!.element as HTMLInputElement).value).toBe('bob@example.com')

    await emailInput!.setValue('bob2@example.com')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.update).toHaveBeenCalledWith(9, 'bob', 0, 'bob2@example.com', SILENT)
  })

  it('清空邮箱 = 解绑(仍提交空串, 由服务端写 NULL)', async () => {
    const { dlg } = await openEdit()
    const emailInput = dlg
      .find('form')
      .findAll('input')
      .find((i) => (i.element as HTMLInputElement).type === 'email')
    await emailInput!.setValue('')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.update).toHaveBeenCalledWith(9, 'bob', 0, '', SILENT)
  })

  it('改自己的角色被前端拦截(ManageSelect 锁定 + 提交检查)', async () => {
    const { w, wh } = await mountAll()
    // mountAll 里已建 pinia, 登录态须在挂载后设置; 且要先于「编辑」点击(点击时页面把 selfId 传给弹窗)
    useUserStore().info = { id: 9 }
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '编辑')!.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(UserFormDialog)
    expect(dlg.exists()).toBe(true)
    const sel = dlg.findComponent(ManageSelect)
    expect(sel.exists(), '角色应为 ManageSelect').toBe(true)
    // 角色下拉对自己的行锁定
    expect(sel.props('disabled')).toBe(true)
    // 程序化 emit 改值, 覆盖提交时的第二道拦截(正常 UI 改不动, 防御纵深)
    sel.vm.$emit('update:modelValue', 1)
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.update).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '不能修改自己的角色')
  })

  it('注册时间列用统一 formatDateTime 渲染(YYYY-MM-DD HH:mm, 时区无关断言)', async () => {
    const { w } = await mountAll()
    expect(w.text()).toMatch(/2026-10-01 \d{2}:\d{2}/)
  })
})

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

  afterEach(async () => {
    document.body.innerHTML = ''
    manageDialogHostMounted.value = false
    if (manageDialogState.resolve) answerManageDialog({ type: 'close' })
    manageDialogState.visible = false
    manageDialogState.component = null
    manageDialogState.resolve = null
    await flushPromises()
  })

  it('禁用: confirm 弹文案与危险样式, 确认后 setDisabled(id, true, silent) 并刷新', async () => {
    const { w } = await mountAll()
    const btn = w.findAll('tbody button').find((b) => b.text() === '禁用')!
    await btn.trigger('click')
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '禁用「bob」？',
        danger: true,
        okText: '禁用'
      })
    )
    await flushPromises()
    expect(h.setDisabled).toHaveBeenCalledWith(9, true, SILENT)
    expect(h.toast).toHaveBeenCalledWith('success', '已禁用')
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('启用: 同一按钮按当前状态翻转文案与参数', async () => {
    h.list.mockResolvedValue({
      list: [{ ...row, disabled: 1 }],
      page: { total: 1, size: 20, current: 1 }
    })
    const { w } = await mountAll()
    const btn = w.findAll('tbody button').find((b) => b.text() === '启用')!
    await btn.trigger('click')
    await flushPromises()
    expect(h.setDisabled).toHaveBeenCalledWith(9, false, SILENT)
    expect(h.toast).toHaveBeenCalledWith('success', '已启用')
  })

  it('禁用失败: toast error, 不误报成功', async () => {
    h.setDisabled.mockRejectedValue(new Error('服务器繁忙'))
    const { w } = await mountAll()
    await w.findAll('tbody button').find((b) => b.text() === '禁用')!.trigger('click')
    await flushPromises()
    expect(h.toast).toHaveBeenCalledWith('error', '服务器繁忙')
  })

  it('删除: confirm 描述不可恢复, 确认后 remove(id, silent) 并刷新', async () => {
    const { w } = await mountAll()
    await w.findAll('tbody button').find((b) => b.text() === '删除')!.trigger('click')
    expect(confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '删除「bob」？',
        danger: true,
        okText: '删除'
      })
    )
    await flushPromises()
    expect(h.remove).toHaveBeenCalledWith(9, SILENT)
    expect(h.list).toHaveBeenCalledTimes(2)
  })

  it('自己的行: 禁用/重置密码/删除按钮 disabled, 编辑仍可用', async () => {
    const { w } = await mountAll()
    // mountAll 里已建 pinia, 登录态须在挂载后设置(否则被新 pinia 覆盖)
    useUserStore().info = { id: 9 }
    await flushPromises()
    const buttons = Object.fromEntries(w.findAll('tbody button').map((b) => [b.text(), b]))
    expect(buttons['禁用']!.attributes('disabled')).toBeDefined()
    expect(buttons['重置密码']!.attributes('disabled')).toBeDefined()
    expect(buttons['删除']!.attributes('disabled')).toBeDefined()
    expect(buttons['编辑']!.attributes('disabled')).toBeUndefined()
  })

  it('重置密码: 函数化弹窗提交, <6 位前端拦截, 合法后 resetPassword(id, pw, silent)', async () => {
    const { w, wh } = await mountAll()
    await w.findAll('tbody button').find((b) => b.text() === '重置密码')!.trigger('click')
    await flushPromises()
    const dlg = wh.findComponent(UserResetPasswordDialog)
    expect(dlg.exists(), '重置密码应经 openManageDialog 发起').toBe(true)
    expect(dlg.props('target')).toMatchObject({ ID: 9, userName: 'bob' })
    const pw = dlg.find('input[type="password"]')
    expect(pw.exists()).toBe(true)

    await pw.setValue('123')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.resetPassword).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '密码长度需在 6-64 位之间')

    await pw.setValue('newpass1')
    await dlg.find('form').trigger('submit')
    await flushPromises()
    expect(h.resetPassword).toHaveBeenCalledWith(9, 'newpass1', SILENT)
  })
})
