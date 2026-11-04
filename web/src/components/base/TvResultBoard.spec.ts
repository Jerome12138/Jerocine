import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TvResultBoard from './TvResultBoard.vue'
import type { PageMeta } from '@/types/api'
import type { FilterGroup } from '@/components/film/FilmFilterBar.vue'

/**
 * W2a TV 分屏结果板(ClassifySearchView / HotBoardView TV 分支收编):
 * 契约 = DOM 类名与 data-focusable 顺序逐字不变 —— 左竖栏筛选 chip(jc-tv-chip jc-cs-tv__chip)
 * → 右结果区(骨架/jc-tv-grid/空态) → jc-cs-tv__pager 翻页(上一页/下一页)。
 * 页头(标题切换/榜单名+计数)由调用方经 #head 插槽提供(两页只留数据与头部)。
 */
const page = (over: Partial<PageMeta> = {}): PageMeta => ({
  size: 49,
  current: 1,
  pageCount: 3,
  total: 100,
  ...over
})

const groups: FilterGroup[] = [
  {
    key: 'Year',
    title: '年份',
    options: [
      { value: '', label: '全部' },
      { value: '2024', label: '2024' }
    ],
    current: '2024'
  },
  {
    key: 'Area',
    title: '地区',
    options: [
      { value: '', label: '全部' },
      { value: '大陆', label: '大陆' }
    ],
    current: ''
  }
]

function mountBoard(over: Record<string, unknown> = {}) {
  return mount(TvResultBoard, {
    props: {
      loading: false,
      loaded: true,
      errorMsg: '',
      errorDescription: 'err-desc',
      films: [],
      page: page(),
      filterGroups: groups,
      emptyTitle: '无符合条件的影片',
      ...over
    },
    slots: { head: '<div class="jc-cs-tv__head">head</div>' },
    global: { stubs: { FilmCard: { template: '<div class="film-card-stub" />' } } }
  })
}

describe('TvResultBoard', () => {
  it('根节点 = section(类名由调用方透传), #head 插槽内容渲染在 split 之前', () => {
    const w = mountBoard()
    expect(w.element.tagName).toBe('SECTION')
    const children = w.find('section').element.children
    expect(children[0].classList.contains('jc-cs-tv__head')).toBe(true)
    expect(children[1].classList.contains('jc-cs-tv__split')).toBe(true)
  })

  it('loading 骨架: jc-cs-tv__split 左右分屏 + poster-tv 骨架(jc-tv-grid), 无翻页', () => {
    const w = mountBoard({ loading: true, loaded: false })
    expect(w.find('.jc-cs-tv__split').exists()).toBe(true)
    expect(w.find('.jc-cs-tv__filter').exists()).toBe(true)
    expect(w.find('.jc-cs-tv__main .jc-tv-grid').exists()).toBe(true)
    expect(w.find('.jc-cs-tv__pager').exists()).toBe(false)
  })

  it('筛选竖栏: chip 类名/选中态/aria 逐字保留(jc-tv-chip jc-cs-tv__chip + sel)', () => {
    const w = mountBoard()
    const chips = w.findAll('.jc-cs-tv__grp-chips button')
    expect(chips.length).toBe(4)
    expect(chips[0].classes()).toEqual(['jc-tv-chip', 'jc-cs-tv__chip'])
    expect(chips[0].text()).toBe('全部')
    expect(chips[0].classes().includes('sel')).toBe(false)
    // Year 组 current='2024' → 第 2 枚选中
    expect(chips[1].classes().includes('sel')).toBe(true)
    expect(chips[1].attributes('aria-pressed')).toBe('true')
    expect(chips[0].attributes('aria-pressed')).toBe('false')
    expect(chips[0].attributes('data-focusable')).toBe('true')
    expect(chips[0].attributes('tabindex')).toBe('0')
    expect(w.find('.jc-cs-tv__grp-title').text()).toBe('年份')
  })

  it('filterGroups 为空: 不渲染竖栏, 主区照常(对应 ClassifySearch v-if 语义)', () => {
    const w = mountBoard({ filterGroups: [] })
    expect(w.find('.jc-cs-tv__filter').exists()).toBe(false)
    expect(w.find('.jc-cs-tv__main').exists()).toBe(true)
  })

  it('结果网格: films → jc-tv-grid 包 N 张卡; 空态 → BaseEmpty(emptyTitle + 固定提示)', () => {
    const w = mountBoard({ films: [{ mid: 1 }, { mid: 2 }] } as never)
    expect(w.findAll('.jc-cs-tv__main .jc-tv-grid .film-card-stub').length).toBe(2)

    const empty = mountBoard()
    expect(empty.find('.jc-cs-tv__main .jc-empty').exists()).toBe(true)
    expect(empty.text()).toContain('无符合条件的影片')
    expect(empty.text()).toContain('试试切换其他筛选条件')
  })

  it('错误态: !loading && errorMsg → BaseEmpty(title=errorMsg, desc=errorDescription), 分屏隐藏', () => {
    const w = mountBoard({ errorMsg: '加载失败，请稍后重试', errorDescription: '请检查链接中的 Pid 参数' })
    expect(w.find('.jc-cs-tv__split').exists()).toBe(false)
    expect(w.find('.jc-empty').exists()).toBe(true)
    expect(w.text()).toContain('加载失败，请稍后重试')
    expect(w.text()).toContain('请检查链接中的 Pid 参数')
  })

  it('翻页器: jc-cs-tv__pager 结构与禁用态逐字保留, 点击上抛 paginate', async () => {
    const w = mountBoard({ films: [{ mid: 1 }] } as never)
    const pager = w.find('.jc-cs-tv__pager')
    expect(pager.exists()).toBe(true)
    const btns = pager.findAll('button')
    expect(btns[0].text()).toBe('上一页')
    expect(btns[0].classes()).toEqual(['jc-tv-btn'])
    expect(btns[1].text()).toBe('下一页')
    expect(btns[1].classes()).toEqual(['jc-tv-btn', 'cyan'])
    expect(pager.find('.jc-cs-tv__pager-info').text()).toBe('1 / 3')
    // 边界: 第 1 页 → 上一页 disabled; 点下一页 → paginate(2)
    expect(btns[0].attributes('disabled')).toBeDefined()
    expect(btns[1].attributes('disabled')).toBeUndefined()
    await btns[1].trigger('click')
    expect(w.emitted('paginate')).toEqual([[2]])

    const last = mountBoard({ films: [{ mid: 1 }], page: page({ current: 3 }) } as never)
    const lastBtns = last.findAll('.jc-cs-tv__pager button')
    expect(lastBtns[1].attributes('disabled')).toBeDefined()
    await lastBtns[0].trigger('click')
    expect(last.emitted('paginate')).toEqual([[2]])
  })

  it('翻页器隐藏条件: pageCount<=1 / total=0 时不渲染(loading 时亦不渲染)', () => {
    expect(mountBoard({ page: page({ pageCount: 1 }) }).find('.jc-cs-tv__pager').exists()).toBe(false)
    expect(mountBoard({ page: page({ total: 0, pageCount: 0 }) }).find('.jc-cs-tv__pager').exists()).toBe(false)
    expect(mountBoard({ loading: true, page: page() }).find('.jc-cs-tv__pager').exists()).toBe(false)
  })

  it('data-focusable 顺序不变: 竖栏 chips(组序×选项序) → 翻页上一页/下一页', () => {
    const w = mountBoard({ films: [{ mid: 1 }] } as never)
    const order = w.findAll('[data-focusable="true"]')
    expect(order.length).toBe(6)
    expect(order[0].text()).toBe('全部')
    expect(order[1].text()).toBe('2024')
    expect(order[2].text()).toBe('全部')
    expect(order[3].text()).toBe('大陆')
    expect(order[4].text()).toBe('上一页')
    expect(order[5].text()).toBe('下一页')
  })

  it('点击 chip 上抛 filter-change({key, value})', async () => {
    const w = mountBoard()
    await w.findAll('.jc-cs-tv__grp-chips button')[1].trigger('click')
    expect(w.emitted('filter-change')).toEqual([[{ key: 'Year', value: '2024' }]])
  })
})
