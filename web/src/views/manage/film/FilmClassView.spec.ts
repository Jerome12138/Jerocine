import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FilmClassView from './FilmClassView.vue'
import FilmClassFormDialog from './FilmClassFormDialog.vue'
import ManageDialogHost from '@/components/manage/ManageDialogHost.vue'
import {
  answerManageDialog,
  manageDialogState,
  manageDialogHostMounted
} from '@/composables/useManageDialog'
import type { FilmClass } from '@/types/manage'

const h = vi.hoisted(() => ({
  classTree: vi.fn(),
  classUpdate: vi.fn(),
  classDel: vi.fn(),
  confirm: vi.fn(),
  toast: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    film: {
      classTree: h.classTree,
      classUpdate: h.classUpdate,
      classDel: h.classDel
    }
  }
}))

vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))

/** 顶级 + 单子级的固定树（计数齐全; sidebarShow=1 是后端 000037 列默认值） */
function tree(): FilmClass[] {
  return [
    {
      id: 1,
      pid: 0,
      name: '电影',
      show: true,
      sidebarShow: true,
      filmVisible: true,
      filmCount: 3,
      children: [
        { id: 2, pid: 1, name: '科幻', show: true, sidebarShow: true, filmVisible: true, filmCount: 0 }
      ]
    }
  ]
}

/** 页面 + 弹窗 Host 一起挂(共享 useManageDialog 模块单例); 弹窗内容断言走 Host wrapper */
async function mountAll(): Promise<{ w: VueWrapper; wh: VueWrapper }> {
  setActivePinia(createPinia())
  const w = mount(FilmClassView, {
    global: { plugins: [createPinia()], stubs: { Teleport: true } }
  })
  const wh = mount(ManageDialogHost, {
    attachTo: document.body,
    global: { stubs: { Teleport: true } }
  })
  await flushPromises()
  return { w, wh }
}

/** 按包裹 label 的 title 定位到对应 ManageSwitch 按钮（顶级行的两个开关） */
function switchByTitle(w: VueWrapper, titlePrefix: string) {
  const label = w.findAll('label').find((l) => (l.attributes('title') ?? '').startsWith(titlePrefix))
  expect(label, `title 前缀 ${titlePrefix} 的开关应存在`).toBeTruthy()
  const btn = label!.find('[role="switch"]')
  expect(btn.exists()).toBe(true)
  return btn
}

/**
 * 顶级行的两开关:
 *   - 顶栏: title="控制 web/TV 顶部一级导航..."
 *   - 侧栏: title="侧边栏显示（已关）" 或 "侧边栏显示（已开）"
 *   - 影片: title="关闭后该分类影片从公开列表与搜索中剔除；默认可见"
 * 用足够长的前缀让 find 唯一命中。
 */
const SHOW_TOP_TITLE = '只控制顶部导航'
const SIDEBAR_TITLE = '侧边栏显示'
// 影片可见标题长前缀: 避免匹配到子级 (子级也带同样前缀但 find 取第一个)
const FILM_TITLE = '关闭后该分类影片从公开列表'

describe('FilmClassView 分类管理', () => {
  let w: VueWrapper | null = null
  let wh: VueWrapper | null = null

  beforeEach(() => {
    vi.clearAllMocks()
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

    h.classTree.mockResolvedValue(tree())
    h.classUpdate.mockResolvedValue(undefined)
    h.classDel.mockResolvedValue(undefined)
    h.confirm.mockResolvedValue(true)
  })

  afterEach(async () => {
    // 先卸 Host(待决弹窗 resolve(null)), 再卸页面
    wh?.unmount()
    w?.unmount()
    wh = null
    w = null
    await flushPromises()
  })

  it('渲染双开关与影片数（真 0 部也要显示，不能被降级成 —）', async () => {
    ;({ w } = await mountAll())
    expect(w!.text()).toContain('电影')
    expect(w!.text()).toContain('科幻')
    // 顶级 3 部、子级真 0 部（后端以指针区分 0 与计数失败）
    expect(w!.text()).toContain('3 部')
    expect(w!.text()).toContain('0 部')
    // 三开关文案（顶栏/侧边栏/影片可见）
    expect(w!.text()).toContain('顶栏显示')
    expect(w!.text()).toContain('侧栏显示')
    expect(w!.text()).toContain('影片可见')
  })

  it('计数失败降级（filmCount 缺省）显示 —', async () => {
    const t = tree()
    delete (t[0] as Partial<FilmClass>).filmCount
    delete (t[0].children![0] as Partial<FilmClass>).filmCount
    h.classTree.mockResolvedValue(t)
    ;({ w } = await mountAll())
    expect(w!.text()).toContain('—')
  })

  it('关顶级「顶栏显示」→ silent classUpdate 且不影响子级(000037 拆出后 toggleShow 不复级联)', async () => {
    ;({ w } = await mountAll())
    await switchByTitle(w!, SHOW_TOP_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    expect(h.classUpdate.mock.calls[0][1]).toEqual({ silent: true })
    const first = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(first.id).toBe(1)
    expect(first.show).toBe(false)
    // 子级 show 不被联动（toggleShow 只做自身, 不再级联）
    expect(h.classUpdate).not.toHaveBeenCalledWith(expect.objectContaining({ id: 2 }))
  })

  it('关顶级「侧边栏显示」→ silent classUpdate 且不影响子级(各自独立)', async () => {
    ;({ w } = await mountAll())
    await switchByTitle(w!, SIDEBAR_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    expect(h.classUpdate.mock.calls[0][1]).toEqual({ silent: true })
    const first = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(first.id).toBe(1)
    expect(first.sidebarShow).toBe(false)
  })

  it('关顶级「影片可见」→ 前端只发本节点 filmVisible=false(级联由服务端 cascadeHideDescendants 负责)', async () => {
    ;({ w } = await mountAll())
    await switchByTitle(w!, FILM_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    expect(h.classUpdate.mock.calls[0][1]).toEqual({ silent: true })
    const top = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(top.id).toBe(1)
    expect(top.filmVisible).toBe(false)
  })

  it('开顶级「影片可见」(false→true) → 只发本节点 filmVisible=true, 不级联子级', async () => {
    const t = tree()
    t[0].filmVisible = false
    h.classTree.mockResolvedValue(t)
    ;({ w } = await mountAll())
    expect(w!.text()).toContain('不可见')
    await switchByTitle(w!, FILM_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const top = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(top.id).toBe(1)
    expect(top.filmVisible).toBe(true)
  })

  it('toggle 失败 → 乐观更新回滚(开关文案回到原状态), 错误由 useAsyncAction 单点 toast', async () => {
    h.classUpdate.mockRejectedValueOnce(new Error('403'))
    ;({ w } = await mountAll())
    expect(w!.text()).toContain('顶栏显示')
    await switchByTitle(w!, SHOW_TOP_TITLE).trigger('click')
    await flushPromises()
    // 失败后回滚: 仍是"顶栏显示"
    expect(w!.text()).toContain('顶栏显示')
    expect(w!.text()).not.toContain('顶栏隐藏')
    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    // 失败 toast 恰好一次(无拦截器双响)
    expect(h.toast).toHaveBeenCalledTimes(1)
    expect(h.toast).toHaveBeenCalledWith('error', '403')
  })

  it('删除子分类: confirm → classDel(id, silent) → 重拉树', async () => {
    ;({ w } = await mountAll())
    const del = w!.find('button[aria-label="删除 科幻"]')
    expect(del.exists()).toBe(true)
    await del.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledTimes(1)
    expect(h.classDel).toHaveBeenCalledWith(2, { silent: true })
    // mount 1 次 + 删除后 load 1 次
    expect(h.classTree).toHaveBeenCalledTimes(2)
  })

  it('顶级分类也有删除入口(此前只有子级行可删)', async () => {
    ;({ w } = await mountAll())
    const del = w!.find('button[aria-label="删除 电影"]')
    expect(del.exists()).toBe(true)
    await del.trigger('click')
    await flushPromises()
    expect(h.classDel).toHaveBeenCalledWith(1, { silent: true })
  })

  it('加载失败 → 错误态+重试(不冒充"暂无分类数据"); 空树 → 空态', async () => {
    h.classTree.mockRejectedValue(new Error('后端炸了'))
    ;({ w } = await mountAll())
    expect(w!.text()).toContain('加载失败')
    expect(w!.text()).not.toContain('暂无分类数据')
    h.classTree.mockResolvedValue(tree())
    await w!.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(w!.text()).toContain('电影')

    h.classTree.mockResolvedValue([])
    const w2 = mount(FilmClassView, {
      global: { plugins: [createPinia()], stubs: { Teleport: true } }
    })
    await flushPromises()
    expect(w2.text()).toContain('暂无分类数据')
    w2.unmount()
  })

  it('编辑弹窗函数化: Host 渲染 FilmClassFormDialog 并回填 initial', async () => {
    ;({ w, wh } = await mountAll())
    await w!.find('[aria-label="编辑分类 电影"]').trigger('click')
    await flushPromises()
    expect(wh!.findComponent(FilmClassFormDialog).exists()).toBe(true)
    expect(manageDialogState.title).toBe('编辑分类')
    const dlg = wh!.findComponent(FilmClassFormDialog)
    expect((dlg.props('initial') as FilmClass).name).toBe('电影')
    // 回填
    const nameInput = wh!.find('.jc-sheet--modal input[type="text"]')
    expect((nameInput.element as HTMLInputElement).value).toBe('电影')
  })

  it('编辑提交: classUpdate(payload, silent) → emit confirm → 关弹窗+重拉', async () => {
    ;({ w, wh } = await mountAll())
    await w!.find('[aria-label="编辑分类 电影"]').trigger('click')
    await flushPromises()
    const nameInput = wh!.find('.jc-sheet--modal input[type="text"]')
    await nameInput.setValue('电影改名')
    await wh!.find('.jc-sheet--modal input[type="number"]').setValue('9')
    await wh!.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const [payload, config] = h.classUpdate.mock.calls[0]
    expect(config).toEqual({ silent: true })
    expect(payload.id).toBe(1)
    expect(payload.pid).toBe(0)
    expect(payload.name).toBe('电影改名')
    expect(payload.sort).toBe(9)
    // 弹窗关闭 + 重拉树
    expect(wh!.find('.jc-sheet--modal').exists()).toBe(false)
    expect(h.classTree).toHaveBeenCalledTimes(2)
  })

  it('新增顶级分类: pid=0/id=0, sort 清空归 0', async () => {
    ;({ w, wh } = await mountAll())
    await w!.findAll('button').find((b) => b.text().includes('新增分类'))!.trigger('click')
    await flushPromises()
    expect(manageDialogState.title).toBe('新增顶级分类')
    await wh!.find('.jc-sheet--modal input[type="text"]').setValue('动画')
    // 排序输入清空(ManageInput number 清空 emit undefined)
    await wh!.find('.jc-sheet--modal input[type="number"]').setValue('')
    await wh!.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.id).toBe(0)
    expect(payload.pid).toBe(0)
    expect(payload.name).toBe('动画')
    expect(payload.sort).toBe(0) // undefined 归 0, 不再 422 invalid body
  })

  it('新增子分类: 标题带父级名, 载荷 pid=父 id', async () => {
    ;({ w, wh } = await mountAll())
    await w!.findAll('button').find((b) => b.text().includes('新增「电影」子分类'))!.trigger('click')
    await flushPromises()
    expect(manageDialogState.title).toBe('在「电影」下新增子分类')
    await wh!.find('.jc-sheet--modal input[type="text"]').setValue('喜剧')
    await wh!.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.pid).toBe(1)
    expect(payload.name).toBe('喜剧')
  })

  it('空名保存被前端拦截: toast 单点, classUpdate 不发, 弹窗停留供修正', async () => {
    ;({ w, wh } = await mountAll())
    await w!.findAll('button').find((b) => b.text().includes('新增分类'))!.trigger('click')
    await flushPromises()
    await wh!.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()
    expect(h.classUpdate).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请填写分类名称')
    // 弹窗停留供修正
    expect(wh!.find('.jc-sheet--modal').exists()).toBe(true)
  })

  it('弹窗取消/ESC → openManageDialog resolve(null), 不重拉树', async () => {
    ;({ w, wh } = await mountAll())
    await w!.find('[aria-label="编辑分类 电影"]').trigger('click')
    await flushPromises()
    await wh!.findAll('button').find((b) => b.text().trim() === '取消')!.trigger('click')
    await flushPromises()
    expect(wh!.find('.jc-sheet--modal').exists()).toBe(false)
    // 未 confirm → 不重拉(仍只有 mount 的 1 次)
    expect(h.classTree).toHaveBeenCalledTimes(1)
  })

  it('表单无「英文名」死字段(服务端 entity 无 ename, 输入会被静默丢弃)', async () => {
    ;({ w, wh } = await mountAll())
    await w!.find('[aria-label="编辑分类 电影"]').trigger('click')
    await flushPromises()
    expect(wh!.find('.jc-sheet--modal').text()).not.toContain('英文名')
  })
})
