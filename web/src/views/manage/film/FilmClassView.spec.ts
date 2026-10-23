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

/** 顶级 + 单子级的固定树（计数齐全） */
function tree(): FilmClass[] {
  return [
    {
      id: 1,
      pid: 0,
      name: '电影',
      show: true,
      filmVisible: true,
      filmCount: 3,
      children: [
        { id: 2, pid: 1, name: '科幻', show: true, filmVisible: true, filmCount: 0 }
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

const MENU_TITLE = '只控制导航菜单'
const FILM_TITLE = '关闭后该分类影片'

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
    // 双开关：影片可见 + 菜单显示 各有文案标签
    expect(w.text()).toContain('影片可见')
    expect(w.text()).toContain('菜单显示')
  })

  it('计数失败降级（filmCount 缺省）显示 —', async () => {
    const t = tree()
    delete (t[0] as Partial<FilmClass>).filmCount
    delete (t[0].children![0] as Partial<FilmClass>).filmCount
    h.classTree.mockResolvedValue(t)
    const w = await mountView()
    expect(w.text()).toContain('—')
  })

  it('关顶级「菜单显示」→ 级联关闭已展示子级（classUpdate 调 2 次）', async () => {
    const w = await mountView()
    await switchByTitle(w, MENU_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(2)
    const first = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(first.id).toBe(1)
    expect(first.show).toBe(false)
    // 子级级联关闭（仅已展示的）
    const second = h.classUpdate.mock.calls[1][0] as FilmClass
    expect(second.id).toBe(2)
    expect(second.show).toBe(false)
  })

  it('关顶级「影片可见」→ 不级联子级（pid 过滤天然覆盖, classUpdate 只调 1 次）', async () => {
    const w = await mountView()
    await switchByTitle(w, FILM_TITLE).trigger('click')
    await flushPromises()

    expect(h.classUpdate).toHaveBeenCalledTimes(1)
    const payload = h.classUpdate.mock.calls[0][0] as FilmClass
    expect(payload.id).toBe(1)
    expect(payload.filmVisible).toBe(false)
    // 菜单开关不被误动
    expect(payload.show).toBe(true)
  })
})
