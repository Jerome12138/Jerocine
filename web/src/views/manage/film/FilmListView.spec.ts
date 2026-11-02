import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import FilmListView from './FilmListView.vue'
import type { ManageFilmRow, ManageFilmSearchResp } from '@/types/manage'
import { useViewMode } from '@/composables/useViewMode'

const h = vi.hoisted(() => ({
  searchList: vi.fn(),
  softDelete: vi.fn(),
  restore: vi.fn(),
  classTree: vi.fn(),
  confirm: vi.fn(),
  toast: vi.fn(),
  routerPush: vi.fn()
}))

vi.mock('@/api', () => ({
  manageApi: {
    film: {
      searchList: h.searchList,
      softDelete: h.softDelete,
      restore: h.restore,
      classTree: h.classTree
    }
  }
}))
vi.mock('@/api/http', () => ({ toast: h.toast }))
vi.mock('@/composables/useConfirm', () => ({ confirm: h.confirm }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: h.routerPush }) }))

function row(over: Partial<ManageFilmRow> = {}): ManageFilmRow {
  return {
    mid: 1,
    name: '流浪地球',
    cover: 'c.jpg',
    cid: 7,
    pid: 1,
    cName: '科幻',
    subTitle: '',
    area: '中国大陆',
    year: 2019,
    state: '',
    remarks: '热映',
    dbScore: 0,
    ...over
  }
}

function resp(list: ManageFilmRow[], total = list.length, size = 20): { body: ManageFilmSearchResp } {
  return {
    body: {
      list,
      page: { current: 1, size, total, pageCount: Math.max(1, Math.ceil(total / size)) }
    }
  }
}

/** 顶级 + 子级的固定分类树(供分类筛选弹层) */
function tree() {
  return [
    {
      id: 1, pid: 0, name: '电影', show: true, sidebarShow: true, filmVisible: true,
      children: [{ id: 2, pid: 1, name: '科幻', show: true, sidebarShow: true, filmVisible: true }]
    }
  ]
}

async function mountView() {
  const w = mount(FilmListView, { global: { plugins: [createPinia()] } })
  await flushPromises()
  return w
}

function deleteButton(w: ReturnType<typeof mount>) {
  const btn = w.findAll('button').find((b) => b.text().trim() === '删除')
  expect(btn, '删除按钮应存在').toBeTruthy()
  return btn!
}

describe('FilmListView 影片管理', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    h.searchList.mockResolvedValue(resp([row()]).body)
    h.classTree.mockResolvedValue(tree())
    h.confirm.mockResolvedValue(true)
    h.softDelete.mockResolvedValue(undefined)
    h.restore.mockResolvedValue(undefined)
  })
  afterEach(() => {
    useViewMode().setMode('desktop')
  })

  it('渲染行与状态列: 软删行显示回收站, 在架行显示在架', async () => {
    const w = await mountView()
    expect(w.text()).toContain('流浪地球')
    expect(w.text()).toContain('在架')
  })

  it('deletedAt>0 的行显示回收站标签(带删除时间 title)', async () => {
    h.searchList.mockResolvedValue(resp([row({ mid: 9, deletedAt: 1767000000000 })]).body)
    const w = await mountView()
    expect(w.text()).toContain('回收站')
    const tag = w.findAll('span').find((s) => s.text() === '回收站')
    expect(tag?.attributes('title')).toContain('删除于')
  })

  it('兜底单元格: year=0 与空串分类/地区显示 —(不兜成 0/空白)', async () => {
    h.searchList.mockResolvedValue(resp([row({ mid: 3, name: '缺数据', year: 0, cName: '', area: '' })]).body)
    const w = await mountView()
    const html = w.find('tbody').text()
    expect(html).not.toContain('0')
    // 名称行仍在, 缺数据影片的空字段归 —
    expect(html).toContain('缺数据')
    expect(w.find('tbody').findAll('td').some((td) => td.text().trim() === '—')).toBe(true)
  })

  it('switchStatus 切到回收站 → status=deleted 且页码重置为 1', async () => {
    const w = await mountView()
    const tab = w.findAll('button').find((b) => b.text() === '回收站')
    await tab!.trigger('click')
    await flushPromises()
    const last = h.searchList.mock.calls.at(-1)[0]
    expect(last.status).toBe('deleted')
    expect(last.page).toBe(1)
  })

  it('排序下拉 → 请求带 sort=score 且页码重置', async () => {
    const w = await mountView()
    const sel = w.find('select[aria-label="排序方式"]')
    expect(sel.exists()).toBe(true)
    await sel.setValue('score')
    await flushPromises()
    const last = h.searchList.mock.calls.at(-1)[0]
    expect(last.sort).toBe('score')
    expect(last.page).toBe(1)
  })

  it('分类筛选 pending→applied: 勾子级 → 确定 → catIds 生效且弹层关闭', async () => {
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().includes('分类筛选'))!.trigger('click')
    await flushPromises()
    const panel = w.find('div.top-full')
    expect(panel.exists()).toBe(true)
    // 勾选子分类"科幻"(第二个 checkbox)
    const boxes = panel.findAll('input[type="checkbox"]')
    expect(boxes.length).toBe(2)
    await boxes[1].setValue()
    await flushPromises()
    // 确定前(searchList 只在 mount 时调过一次, 不带 catIds)
    const before = h.searchList.mock.calls.at(-1)[0]
    expect(before.catIds).toBeUndefined()
    await panel.findAll('button').find((b) => b.text() === '确定')!.trigger('click')
    await flushPromises()
    const last = h.searchList.mock.calls.at(-1)[0]
    expect(last.catIds).toBe('2')
    // 弹层关闭
    expect(w.find('div.top-full').exists()).toBe(false)
  })

  it('关键词与分类筛选互斥: 有关键词时搜索清掉分类筛选并提示', async () => {
    const w = await mountView()
    // 先应用分类筛选(顶级"电影"带整棵子树 → catIds=1,2)
    await w.findAll('button').find((b) => b.text().includes('分类筛选'))!.trigger('click')
    await flushPromises()
    const panel = w.find('div.top-full')
    await panel.findAll('input[type="checkbox"]')[0].setValue()
    await panel.findAll('button').find((b) => b.text() === '确定')!.trigger('click')
    await flushPromises()
    expect(h.searchList.mock.calls.at(-1)[0].catIds).toBe('1,2')
    // 再输入关键词搜索 → 分类筛选被清除
    await w.find('input[placeholder*="影片名关键字"]').setValue('流浪')
    await w.findAll('button').find((b) => b.text().includes('搜索'))!.trigger('click')
    await flushPromises()
    const last = h.searchList.mock.calls.at(-1)[0]
    expect(last.keyword).toBe('流浪')
    expect(last.catIds).toBeUndefined()
    expect(h.toast).toHaveBeenCalledWith('info', expect.stringContaining('已清除分类筛选'))
  })

  it('删除流程: confirm → softDelete(silent) → 定制文案 toast → 重载列表', async () => {
    const w = await mountView()
    await deleteButton(w).trigger('click')
    await flushPromises()
    expect(h.confirm).toHaveBeenCalledTimes(1)
    expect(h.softDelete).toHaveBeenCalledWith(1, { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', '已移入回收站')
    // 重载: mount 1 次 + 删除后 1 次
    expect(h.searchList).toHaveBeenCalledTimes(2)
  })

  it('busy 防重: softDelete 未返回期间删除按钮禁用', async () => {
    let resolveDel: () => void = () => {}
    h.softDelete.mockReturnValue(new Promise<void>((r) => (resolveDel = r)))
    const w = await mountView()
    const btn = deleteButton(w)
    await btn.trigger('click')
    await flushPromises()
    expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    resolveDel()
    await flushPromises()
    // 重载后重新取按钮(表格重渲染, 旧 wrapper 可能指向已替换的元素)
    const btnAfter = deleteButton(w)
    expect((btnAfter.element as HTMLButtonElement).disabled).toBe(false)
  })

  it('回收站行显示恢复按钮 → restore(silent) + 定制文案', async () => {
    h.searchList.mockResolvedValue(resp([row({ mid: 9, deletedAt: 1767000000000 })]).body)
    const w = await mountView()
    const btn = w.findAll('button').find((b) => b.text().trim() === '恢复')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()
    expect(h.restore).toHaveBeenCalledWith(9, { silent: true })
    expect(h.toast).toHaveBeenCalledWith('success', '已恢复')
  })

  it('移动端卡片视图有删除/恢复操作入口(此前插槽覆盖后完全不可达)', async () => {
    useViewMode().setMode('mobile')
    const w = await mountView()
    const card = w.find('.jc-card-list')
    expect(card.exists()).toBe(true)
    const del = card.findAll('button').find((b) => b.text().trim() === '删除')
    expect(del, '移动端卡片应有删除按钮').toBeTruthy()
    const view = card.findAll('button').find((b) => b.text().trim() === '查看')
    expect(view).toBeTruthy()
  })

  it('末页删空回夹: 第 4 页空且 total 缩水 → 自动回拉最后一页', async () => {
    // total=61(4 页); 第 4 页返回空列表且 total 已缩到 5(1 页) → 应回夹到第 1 页
    h.searchList.mockImplementation((p: { page: number }) => {
      if (p.page >= 4) {
        return Promise.resolve({
          list: [],
          page: { current: 4, size: 20, total: 5, pageCount: 1 }
        })
      }
      return Promise.resolve({
        list: [row({ mid: p.page })],
        page: { current: p.page, size: 20, total: 61, pageCount: 4 }
      })
    })
    const w = await mountView()
    const page4 = w.findAll('nav button').find((b) => b.text().trim() === '4')
    expect(page4, '第 4 页按钮应存在').toBeTruthy()
    await page4!.trigger('click')
    await flushPromises()
    const last = h.searchList.mock.calls.at(-1)[0]
    expect(last.page).toBe(1)
  })

  it('加载失败且无旧数据 → 错误态+重试(不冒充"未找到影片")', async () => {
    h.searchList.mockRejectedValue(new Error('网络错误'))
    const w = await mountView()
    await flushPromises()
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('网络错误')
    expect(w.text()).not.toContain('未找到影片')
    // 重试成功 → 恢复列表
    h.searchList.mockResolvedValue(resp([row({ mid: 5, name: '重试后' })]).body)
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('重试后')
  })

  it('分类树加载失败 → 弹层显示错误+重试(不冒充"暂无分类"); 重试成功可勾选', async () => {
    // onMounted 首次加载也失败; 每次打开弹层都会重试, 这里让它一直失败直到手动重试
    h.classTree.mockRejectedValue(new Error('树挂了'))
    const w = await mountView()
    await w.findAll('button').find((b) => b.text().includes('分类筛选'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('分类树加载失败')
    expect(w.text()).toContain('树挂了')
    // 重试 → 树渲染
    h.classTree.mockResolvedValue(tree())
    await w.findAll('button').find((b) => b.text().includes('重试'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('科幻')
  })

  it('竞态守卫: 慢返回的旧页签响应不覆盖新结果', async () => {
    let resolveOld: (v: ManageFilmSearchResp) => void = () => {}
    h.searchList.mockImplementationOnce(
      () => new Promise<ManageFilmSearchResp>((r) => (resolveOld = r))
    )
    const w = mount(FilmListView, { global: { plugins: [createPinia()] } })
    await flushPromises()
    // 切到回收站(新请求, 立即返回)
    await w.findAll('button').find((b) => b.text() === '回收站')!.trigger('click')
    await flushPromises()
    // 此时旧请求才慢返回(挂在 active 页签) → 应被丢弃
    resolveOld(resp([row({ mid: 99, name: '旧响应' })]).body)
    await flushPromises()
    expect(w.text()).not.toContain('旧响应')
    expect(h.searchList).toHaveBeenCalledTimes(2)
  })
})
