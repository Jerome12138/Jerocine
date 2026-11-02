import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FilmClassView from './FilmClassView.vue'
import type { FilmClass } from '@/types/manage'

const h = vi.hoisted(() => ({
  classTree: vi.fn(),
  classUpdate: vi.fn(),
  classDel: vi.fn(),
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
vi.mock('@/composables/useConfirm', () => ({ confirm: vi.fn().mockResolvedValue(true) }))

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
  const w = mount(FilmClassView, { global: { plugins: [createPinia()] } })
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
})
