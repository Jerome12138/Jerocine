import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h } from 'vue'
import ManageTable from './ManageTable.vue'
import { useViewMode } from '@/composables/useViewMode'

interface Row {
  id: number
  name: string
  email: string
}

const cols = [
  { key: 'name' as const, label: '用户名' },
  { key: 'email' as const, label: '邮箱' }
]
const rows: Row[] = [
  { id: 1, name: 'admin', email: 'a@a.com' },
  { id: 2, name: 'jerry', email: 'j@j.com' }
]

function forceMode(m: 'mobile' | 'desktop'): void {
  useViewMode().setMode(m)
}

describe('ManageTable mobile card mode', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('desktop → 渲染 <table>', () => {
    forceMode('desktop')
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('table').exists()).toBe(true)
    expect(w.find('.gf-card-list').exists()).toBe(false)
  })

  it('mobile + 默认 mobileVariant=card → .gf-card-list, 无 <table>', () => {
    forceMode('mobile')
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('.gf-card-list').exists()).toBe(true)
    expect(w.find('table').exists()).toBe(false)
    expect(w.findAll('article')).toHaveLength(2)
  })

  it('mobile + mobileVariant=scroll → 保留 <table>', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', mobileVariant: 'scroll' }
    })
    expect(w.find('table').exists()).toBe(true)
    expect(w.find('.gf-card-list').exists()).toBe(false)
  })

  it('mobile card 默认 fallback: 首列标题 + 剩余列 meta', () => {
    forceMode('mobile')
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    const cards = w.findAll('article')
    expect(cards.length).toBe(2)
    expect(cards[0].find('header').text()).toContain('admin')
    expect(cards[0].find('dl').text()).toContain('邮箱')
    expect(cards[0].find('dl').text()).toContain('a@a.com')
  })

  it('mobile + #mobile-card slot 完全覆盖默认渲染', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id' },
      slots: {
        'mobile-card': () => h('div', { class: 'custom-card' }, 'override')
      }
    })
    expect(w.findAll('.custom-card')).toHaveLength(2)
    expect(w.find('header').exists()).toBe(false)
  })

  it('loading=true → 不进 card 也不出 table (走 Skeleton)', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', loading: true }
    })
    expect(w.find('.gf-card-list').exists()).toBe(false)
    expect(w.find('table').exists()).toBe(false)
  })

  it('rows=[] → 不进 card 也不出 table (走 Empty)', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows: [], rowKey: 'id' }
    })
    expect(w.find('.gf-card-list').exists()).toBe(false)
    expect(w.find('table').exists()).toBe(false)
  })
})
