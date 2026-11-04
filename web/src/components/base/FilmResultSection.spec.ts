import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilmResultSection from './FilmResultSection.vue'
import BasePagination from './BasePagination.vue'
import type { PageMeta } from '@/types/api'
import type { FilterGroup } from '@/components/film/FilmFilterBar.vue'

/**
 * W2b 桌面/移动结果流水线(ClassifySearchView / HotBoardView 桌面分支收编):
 * 筛选栏(吸边 wrap) → 结果统计 → 骨架 → FilmGrid → 分页 → 空态。
 * 契约 = 纯模板收编(fragment, 不加 wrapper 层), 类名逐字保留
 * (jc-classify-search__filter-wrap / jc-classify-search__count);
 * BasePagination / FilmFilterBar 原样内嵌, 事件原样上抛。
 */
const page = (over: Partial<PageMeta> = {}): PageMeta => ({
  size: 49,
  current: 2,
  pageCount: 4,
  total: 98,
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
  }
]

function mountSection(over: Record<string, unknown> = {}) {
  return mount(FilmResultSection, {
    props: {
      loading: false,
      loaded: true,
      errorMsg: '',
      films: [],
      page: page(),
      filterGroups: groups,
      emptyTitle: '无符合条件的影片',
      ...over
    },
    global: { stubs: { FilmCard: { template: '<div class="film-card-stub" />' } } }
  })
}

describe('FilmResultSection', () => {
  it('fragment 收编: 各块互为兄弟节点, 不新增 wrapper 层', () => {
    const w = mountSection()
    const fw = w.find('.jc-classify-search__filter-wrap').element
    expect(fw.nextElementSibling?.classList.contains('jc-classify-search__count')).toBe(true)
  })

  it('筛选栏: jc-classify-search__filter-wrap 包 FilmFilterBar, 点击 chip 上抛 filter-change', async () => {
    const w = mountSection()
    expect(w.find('.jc-classify-search__filter-wrap .jc-filter-bar').exists()).toBe(true)
    const chip = w.find('.jc-filter-chip')
    await chip.trigger('click')
    expect(w.emitted('filter-change')).toEqual([[{ key: 'Year', value: '' }]])

    const bare = mountSection({ filterGroups: [] })
    expect(bare.find('.jc-classify-search__filter-wrap').exists()).toBe(false)
    expect(bare.find('.jc-classify-search__count').exists()).toBe(true)
  })

  it('结果统计: 共 N 部影片 + 第 X / Y 页(pageCount 0 → 兜底 1)', () => {
    const w = mountSection()
    expect(w.find('.jc-classify-search__count').text()).toContain('共')
    expect(w.find('.jc-classify-search__count').text()).toContain('98')
    expect(w.find('.jc-classify-search__count').text()).toContain('第 2 / 4 页')
    expect(mountSection({ page: page({ pageCount: 0 }) }).find('.jc-classify-search__count').text())
      .toContain('第 2 / 1 页')
  })

  it('骨架/网格: loading 且无 films → poster 骨架; 有 films → jc-film-grid N 格', () => {
    const loading = mountSection({ loading: true, loaded: false })
    expect(loading.find('.jc-card-grid').exists()).toBe(true)
    expect(loading.find('.jc-film-grid').exists()).toBe(false)

    const grid = mountSection({ films: [{ mid: 1 }, { mid: 2 }, { mid: 3 }] } as never)
    expect(grid.findAll('.jc-film-grid__cell .film-card-stub').length).toBe(3)
  })

  it('分页: BasePagination 原样内嵌(current/pageSize/total), change 上抛 paginate', async () => {
    const w = mountSection({ films: [{ mid: 1 }] } as never)
    const pager = w.findComponent(BasePagination)
    expect(pager.exists()).toBe(true)
    expect(pager.props('current')).toBe(2)
    expect(pager.props('pageSize')).toBe(49)
    expect(pager.props('total')).toBe(98)
    pager.vm.$emit('change', 3)
    expect(w.emitted('paginate')).toEqual([[3]])
    // loading 时不渲染分页
    expect(mountSection({ loading: true }).findComponent(BasePagination).exists()).toBe(false)
  })

  it('空态: !loading && loaded && 无 films && !errorMsg → BaseEmpty(emptyTitle); errorMsg 时不渲染', () => {
    const w = mountSection()
    expect(w.find('.jc-empty').exists()).toBe(true)
    expect(w.text()).toContain('无符合条件的影片')
    expect(w.text()).toContain('试试切换其他筛选条件')
    expect(mountSection({ errorMsg: '加载失败，请稍后重试' }).find('.jc-empty').exists()).toBe(false)
  })
})
