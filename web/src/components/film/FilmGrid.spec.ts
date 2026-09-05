import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { h } from 'vue'
import FilmGrid from './FilmGrid.vue'
import type { FilmListItem } from '@/types/film'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

const items: FilmListItem[] = [
  { id: 1, mid: 100, name: 'A', picture: 'http://x/a.jpg' } as never,
  { id: 2, mid: 101, name: 'B', picture: 'http://x/b.jpg' } as never,
  { id: 3, mid: 102, name: 'C', picture: 'http://x/c.jpg' } as never
]

describe('FilmGrid', () => {
  it('items.length 决定渲染数量', () => {
    const w = mount(FilmGrid, { props: { items }, global: { plugins: [router] } })
    expect(w.findAll('.gf-film-grid__cell').length).toBe(3)
  })

  it('空 items → 不渲染 cell', () => {
    const w = mount(FilmGrid, { props: { items: [] }, global: { plugins: [router] } })
    expect(w.findAll('.gf-film-grid__cell').length).toBe(0)
  })

  it('gap prop 应用 inline style', () => {
    const w = mount(FilmGrid, {
      props: { items, gap: '20px' },
      global: { plugins: [router] }
    })
    const style = w.find('.gf-film-grid').attributes('style') ?? ''
    expect(style).toContain('20px')
  })

  it('#item slot 完全覆盖默认 FilmCard 渲染', () => {
    const w = mount(FilmGrid, {
      props: { items },
      slots: {
        item: (ctx) => h('div', { class: 'custom-cell' }, (ctx as { item: FilmListItem }).item.name)
      },
      global: { plugins: [router] }
    })
    expect(w.findAll('.custom-cell').length).toBe(3)
    expect(w.text()).toContain('A')
    expect(w.text()).toContain('B')
  })

  it('itemKey 字段非 string/number → fallback 用 index', () => {
    const weird: FilmListItem[] = [{ id: undefined, name: 'X' } as never]
    expect(() => mount(FilmGrid, {
      props: { items: weird, itemKey: 'id' as never },
      global: { plugins: [router] }
    })).not.toThrow()
  })
})
