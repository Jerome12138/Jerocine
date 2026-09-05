import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import RelatedList from './RelatedList.vue'
import type { FilmListItem } from '@/types/film'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }]
})

const items: FilmListItem[] = [
  { id: 1, mid: 100, name: 'A', picture: '' } as never,
  { id: 2, mid: 101, name: 'B', picture: '' } as never
]

describe('RelatedList', () => {
  it('空 items → 不渲染 section', () => {
    const w = mount(RelatedList, { props: { items: [] }, global: { plugins: [router] } })
    expect(w.find('section').exists()).toBe(false)
  })

  it('有 items → 渲染 section', () => {
    const w = mount(RelatedList, { props: { items }, global: { plugins: [router] } })
    expect(w.find('section.gf-related-list').exists()).toBe(true)
  })

  it('默认 title=相关推荐', () => {
    const w = mount(RelatedList, { props: { items }, global: { plugins: [router] } })
    expect(w.find('h2').text()).toBe('相关推荐')
  })

  it('自定义 title', () => {
    const w = mount(RelatedList, {
      props: { items, title: '猜你喜欢' },
      global: { plugins: [router] }
    })
    expect(w.find('h2').text()).toBe('猜你喜欢')
  })

  it('items.length 决定 FilmCard 数量', () => {
    const w = mount(RelatedList, { props: { items }, global: { plugins: [router] } })
    // FilmCard 内部含 a (RouterLink), 通过 a 数量近似判断
    expect(w.findAll('a').length).toBeGreaterThanOrEqual(2)
  })
})
