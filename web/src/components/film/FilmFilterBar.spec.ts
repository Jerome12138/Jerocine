import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilmFilterBar from './FilmFilterBar.vue'

const groups = [
  {
    key: 'type',
    title: '类型',
    options: [
      { value: '', label: '全部' },
      { value: 'movie', label: '电影' },
      { value: 'tv', label: '电视剧' }
    ],
    current: ''
  },
  {
    key: 'year',
    title: '年份',
    options: [
      { value: '', label: '全部' },
      { value: 2024, label: '2024' },
      { value: 2023, label: '2023' }
    ],
    current: 2024
  }
]

describe('FilmFilterBar', () => {
  it('渲染所有 group title', () => {
    const w = mount(FilmFilterBar, { props: { groups } })
    expect(w.text()).toContain('类型')
    expect(w.text()).toContain('年份')
  })

  it('渲染每个 group 的所有 option', () => {
    const w = mount(FilmFilterBar, { props: { groups } })
    const chips = w.findAll('.gf-filter-chip, button')
    // 至少 3 + 3 = 6 个 chip
    expect(chips.length).toBeGreaterThanOrEqual(6)
  })

  it('点击 chip emit change({key, value})', async () => {
    const w = mount(FilmFilterBar, { props: { groups } })
    const buttons = w.findAll('button')
    // 找电影 chip
    const movieChip = buttons.find(b => b.text() === '电影')
    await movieChip?.trigger('click')
    expect(w.emitted('change')).toBeTruthy()
    expect(w.emitted('change')?.[0]).toEqual([{ key: 'type', value: 'movie' }])
  })

  it('数字 value 也能 emit (年份)', async () => {
    const w = mount(FilmFilterBar, { props: { groups } })
    const btn = w.findAll('button').find(b => b.text() === '2023')
    await btn?.trigger('click')
    expect(w.emitted('change')?.[0]).toEqual([{ key: 'year', value: 2023 }])
  })

  it('空 groups → 不报错', () => {
    expect(() => mount(FilmFilterBar, { props: { groups: [] } })).not.toThrow()
  })
})
