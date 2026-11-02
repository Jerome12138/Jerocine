import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FilmClassView from './FilmClassView.vue'
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

async function mountView(): Promise<ReturnType<typeof mount>> {
  // ManageSheet 用 Teleport 到 body: stub 掉让弹窗内容留在 wrapper 树内便于断言
  const w = mount(FilmClassView, {
    global: { plugins: [createPinia()], stubs: { Teleport: true } }
  })
  await flushPromises()
  return w
}

/** 按包裹 label 的 title 定位到对应 ManageSwitch 按钮（顶级行的两个开关） */
function switchByTitle(w: ReturnType<typeof mount>, titlePrefix: string) {
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

/** 顶级 + 单子级的固定树（计数齐全; sidebarShow=1 是后端 000037 列默认值） */

describe('FilmClassView 分类管理', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.classTree.mockResolvedValue(tree())
    h.classUpdate.mockResolvedValue(undefined)
    h.confirm.mockResolvedValue(true)
  })

  it('渲染双开关与影片数（真 0 部也要显示，不能被降级成 —）', async () => {
    const w = await mountView()
    expect(w.text()).toContain('电影')
    expect(w.text()).toContain('科幻')
    // 顶级 3 部、子级真 0 部（后端以指针区分 0 与计数失败）
    expect(w.text()).toContain('3 部')
    expect(w.text()).toContain('0 部')
    // 三开关文案（顶栏/侧边栏/影片可见）
    expect(w.text()).toContain('顶栏显示')
    expect(w.text()).toContain('侧栏显示')
    expect(w.text()).toContain('影片可见')
  })

  it('计数失败降级（filmCount 缺省）显示 —', async () => {
    const t = tree()
    delete (t[0] as Partial<FilmClass>).filmCount
    delete (t[0].children![0] as Partial<FilmClass>).filmCount
    h.classTree.mockResolvedValue(t)
    const w = await mountView()
    expect(w.text()).toContain('—')
  })

  it('关顶级「顶栏显示」→ 不影响子级(show 开关独立, 000037 拆出后 toggleShow 不复级联)', async () => {
    const w = await mountView()
    await switchByTitle(w, SHOW_TOP_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const first = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(first.id).toBe(1)
    expect(first.show).toBe(false)
    // 子级 show 不被联动（toggleShow 只做自身, 不再级联）
    expect(h.classUpdate).not.toHaveBeenCalledWith(expect.objectContaining({ id: 2 }))
  })

  it('关顶级「侧边栏显示」→ 不影响子级(各自独立)', async () => {
    const w = await mountView()
    await switchByTitle(w, SIDEBAR_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const first = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(first.id).toBe(1)
    expect(first.sidebarShow).toBe(false)
  })

  it('关顶级「影片可见」→ 前端只发本节点 filmVisible=false(级联由服务端 cascadeHideDescendants 负责); classUpdate 调 1 次', async () => {
    const w = await mountView()
    await switchByTitle(w, FILM_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const top = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(top.id).toBe(1)
    expect(top.filmVisible).toBe(false)
    // 子孙 show/sidebarShow 的级联关闭由服务端 cascadeHideDescendants 落库,
    // 前端只负责向服务提交 filmVisible=false; 服务返回 InvalidateCategory 后下次 load() 拿到最新状态.
  })

  it('开顶级「影片可见」(false→true) → 只发本节点 filmVisible=true, 不级联子级', async () => {
    const t = tree()
    t[0].filmVisible = false
    h.classTree.mockResolvedValue(t)
    const w = await mountView()
    expect(w.text()).toContain('不可见')
    await switchByTitle(w, FILM_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const top = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(top.id).toBe(1)
    expect(top.filmVisible).toBe(true)
  })

  it('toggle 失败 → 乐观更新回滚(开关文案回到原状态), 不与服务端漂移', async () => {
    h.classUpdate.mockRejectedValueOnce(new Error('403'))
    const w = await mountView()
    expect(w.text()).toContain('顶栏显示')
    await switchByTitle(w, SHOW_TOP_TITLE).trigger('click')
    await flushPromises()
    // 失败后回滚: 仍是"顶栏显示"
    expect(w.text()).toContain('顶栏显示')
    expect(w.text()).not.toContain('顶栏隐藏')
    expect(h.classUpdate).toHaveBeenCalledTimes(1)
  })

  it('删除子分类: confirm → classDel(id) → 重拉树', async () => {
    const w = await mountView()
    const del = w.find('button[aria-label="删除 科幻"]')
    expect(del.exists()).toBe(true)
    await del.trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledTimes(1)
    expect(h.classDel).toHaveBeenCalledWith(2)
    // mount 1 次 + 删除后 load 1 次
    expect(h.classTree).toHaveBeenCalledTimes(2)
  })

  it('顶级分类也有删除入口(此前只有子级行可删)', async () => {
    const w = await mountView()
    const del = w.find('button[aria-label="删除 电影"]')
    expect(del.exists()).toBe(true)
    await del.trigger('click')
    await flushPromises()
    expect(h.classDel).toHaveBeenCalledWith(1)
  })

  it('加载失败 → 错误态+重试(不冒充"暂无分类数据"); 空树 → 空态', async () => {
    h.classTree.mockRejectedValue(new Error('后端炸了'))
    const w = await mountView()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).not.toContain('暂无分类数据')
    h.classTree.mockResolvedValue(tree())
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('电影')

    h.classTree.mockResolvedValue([])
    const w2 = mount(FilmClassView, {
      global: { plugins: [createPinia()], stubs: { Teleport: true } }
    })
    await flushPromises()
    expect(w2.text()).toContain('暂无分类数据')
  })

  it('编辑弹窗回填并提交: 载荷带节点 id 与数字 sort, 保存后关弹窗+重拉', async () => {
    const w = await mountView()
    await w.find('[aria-label="编辑分类 电影"]').trigger('click')
    expect(w.text()).toContain('编辑分类')
    // 回填
    const nameInput = w.find('.jc-sheet--modal input[type="text"]')
    expect((nameInput.element as HTMLInputElement).value).toBe('电影')
    await nameInput.setValue('电影改名')
    await w.find('.jc-sheet--modal input[type="number"]').setValue('9')
    await w.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.id).toBe(1)
    expect(payload.pid).toBe(0)
    expect(payload.name).toBe('电影改名')
    expect(payload.sort).toBe(9)
    // 弹窗关闭 + 重拉树
    expect(w.find('.jc-sheet--modal').exists()).toBe(false)
    expect(h.classTree).toHaveBeenCalledTimes(2)
  })

  it('新增顶级分类: pid=0/id=0, sort 清空归 0', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().includes('新增分类'))!.trigger('click')
    expect(w.text()).toContain('新增顶级分类')
    await w.find('.jc-sheet--modal input[type="text"]').setValue('动画')
    // 排序输入清空(ManageInput number 会 emit 空串)
    await w.find('.jc-sheet--modal input[type="number"]').setValue('')
    await w.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.id).toBe(0)
    expect(payload.pid).toBe(0)
    expect(payload.name).toBe('动画')
    expect(payload.sort).toBe(0) // 空串归 0, 不再 422 invalid body
  })

  it('新增子分类: 标题带父级名, 载荷 pid=父 id', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().includes('新增「电影」子分类'))!.trigger('click')
    expect(w.text()).toContain('在「电影」下新增子分类')
    await w.find('.jc-sheet--modal input[type="text"]').setValue('喜剧')
    await w.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()

    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.pid).toBe(1)
    expect(payload.name).toBe('喜剧')
  })

  it('空名保存被前端拦截(不再只是视觉红星)', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().includes('新增分类'))!.trigger('click')
    await w.findAll('button').find((b) => b.text().includes('保存'))!.trigger('click')
    await flushPromises()
    expect(h.classUpdate).not.toHaveBeenCalled()
    expect(h.toast).toHaveBeenCalledWith('error', '请填写分类名称')
    // 弹窗停留供修正
    expect(w.find('.jc-sheet--modal').exists()).toBe(true)
  })

  it('表单无「英文名」死字段(服务端 entity 无 ename, 输入会被静默丢弃)', async () => {
    const w = await mountView()
    await w.find('[aria-label="编辑分类 电影"]').trigger('click')
    expect(w.text()).not.toContain('英文名')
  })
})
