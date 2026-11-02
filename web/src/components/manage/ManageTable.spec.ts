import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { h, nextTick } from 'vue'
import ManageTable, { type Column } from './ManageTable.vue'
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
    expect(w.find('.jc-card-list').exists()).toBe(false)
  })

  it('mobile + 默认 mobileVariant=card → .jc-card-list, 无 <table>', () => {
    forceMode('mobile')
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('.jc-card-list').exists()).toBe(true)
    expect(w.find('table').exists()).toBe(false)
    expect(w.findAll('article')).toHaveLength(2)
  })

  it('mobile + mobileVariant=scroll → 保留 <table>', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', mobileVariant: 'scroll' }
    })
    expect(w.find('table').exists()).toBe(true)
    expect(w.find('.jc-card-list').exists()).toBe(false)
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
    expect(w.find('.jc-card-list').exists()).toBe(false)
    expect(w.find('table').exists()).toBe(false)
  })

  it('rows=[] → 不进 card 也不出 table (走 Empty)', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows: [], rowKey: 'id' }
    })
    expect(w.find('.jc-card-list').exists()).toBe(false)
    expect(w.find('table').exists()).toBe(false)
  })
})

describe('ManageTable 内置分页 (T4a)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('total=100,current=2,pageSize=20 → 渲染 .jc-table__pagination 且含页码 2', () => {
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', total: 100, current: 2, pageSize: 20 }
    })
    const pager = w.find('.jc-table__pagination')
    expect(pager.exists()).toBe(true)
    expect(pager.findAll('button').some(b => b.text() === '2')).toBe(true)
  })

  it('点击分页第 3 页 → 同时 emit page-change 与 update:current 各一次 [[3]]', async () => {
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', total: 100, current: 2, pageSize: 20 }
    })
    const page3 = w.find('.jc-table__pagination').findAll('button').find(b => b.text() === '3')
    expect(page3).toBeTruthy()
    await page3!.trigger('click')
    expect(w.emitted('page-change')).toEqual([[3]])
    expect(w.emitted('update:current')).toEqual([[3]])
  })

  it('不传 total → 不渲染分页条; total=0 同', () => {
    const w1 = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w1.find('.jc-table__pagination').exists()).toBe(false)
    const w2 = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', total: 0, current: 1, pageSize: 20 }
    })
    expect(w2.find('.jc-table__pagination').exists()).toBe(false)
  })

  it('total=5,current=9 越界 → 正常渲染不抛错, 钳制经 update:current 回传', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', total: 5, current: 9, pageSize: 20 }
    })
    expect(w.find('table').exists()).toBe(true)
    expect(w.emitted('update:current')).toEqual([[1]])
    expect(spy).not.toHaveBeenCalled()
    spy.mockRestore()
  })
})

describe('ManageTable fit 高度 + 紧凑密度 (T4b)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('desktop 默认 fit → 根节点含 jc-table--fit, 滚动容器内联 min-height:320px', () => {
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('section').classes()).toContain('jc-table--fit')
    const scroll = w.find('.jc-table__scroll')
    expect(scroll.exists()).toBe(true)
    expect(scroll.attributes('style')).toContain('min-height')
    expect(scroll.attributes('style')).toContain('320px')
  })

  it('minHeight="480px" → 滚动容器内联 min-height:480px', () => {
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', minHeight: '480px' }
    })
    expect(w.find('.jc-table__scroll').attributes('style')).toContain('480px')
  })

  it('fit=false → 不挂 jc-table--fit 也不渲染 min-height', () => {
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id', fit: false } })
    expect(w.find('section').classes()).not.toContain('jc-table--fit')
    expect(w.find('.jc-table__scroll').exists()).toBe(false)
  })

  it('mobile card 模式 → 忽略 fit: 不挂 jc-table--fit 不渲染 min-height', () => {
    forceMode('mobile')
    const w = mount(ManageTable, {
      props: { columns: cols, rows, rowKey: 'id', minHeight: '480px' }
    })
    expect(w.find('section').classes()).not.toContain('jc-table--fit')
    expect(w.find('.jc-table__scroll').exists()).toBe(false)
    expect(w.find('.jc-card-list').exists()).toBe(true)
  })

  it('桌面 th/td 挂紧凑密度类 jc-table__th/jc-table__td (py 走 --jc-table-cell-py 令牌)', () => {
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('thead th').classes()).toContain('jc-table__th')
    expect(w.find('tbody td').classes()).toContain('jc-table__td')
    // 移动卡片模式不引入该类 (卡片渲染不变)
  })

  it('mobile card 模式不挂 jc-table__th/__td (卡片密度不变)', () => {
    forceMode('mobile')
    const w = mount(ManageTable, { props: { columns: cols, rows, rowKey: 'id' } })
    expect(w.find('thead th').exists()).toBe(false)
    expect(w.find('.jc-table__td').exists()).toBe(false)
  })
})

describe('ManageTable Column 放宽 + 渲染优先级 (T4c)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('render 优先级最高: 返回 vnode 直接渲染, 压过 #cell 与默认文本', () => {
    const renderCols: Column<Row>[] = [
      { key: 'name', label: '用户名', render: (row: Row) => h('b', { class: 'render-mark' }, `x-${row.name}`) },
      { key: 'email', label: '邮箱' }
    ]
    const w = mount(ManageTable, {
      props: { columns: renderCols, rows, rowKey: 'id' },
      slots: { cell: ({ col }) => h('i', { class: 'slot-mark' }, col.label) }
    })
    const tds = w.findAll('tbody td')
    expect(tds[0].find('b.render-mark').exists()).toBe(true)
    expect(tds[0].text()).toBe('x-admin')
    // 无 render 的列仍走 #cell
    expect(tds[1].find('i.slot-mark').exists()).toBe(true)
  })

  it('派生列 (key 非 row 字段) + field 显式取值: #cell 正常渲染不抛错', () => {
    const derivedCols: Column<Row>[] = [
      { key: 'status', label: '状态' },
      { key: 'uname', field: 'name', label: '用户名' }
    ]
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const w = mount(ManageTable, {
      props: { columns: derivedCols, rows, rowKey: 'id' },
      slots: {
        cell: ({ row, col }) =>
          col.key === 'status'
            ? h('span', { class: 'st' }, `st-${row.id}`)
            : h('span', { class: 'nm' }, row.name)
      }
    })
    expect(w.find('.st').text()).toBe('st-1')
    expect(w.find('.nm').text()).toBe('admin')
    expect(spy).not.toHaveBeenCalled()
    spy.mockRestore()
  })

  it('#cell 的 value 参数 = row[col.field ?? col.key]; 派生列为 undefined', () => {
    const derivedCols: Column<Row>[] = [
      { key: 'status', label: '状态' },
      { key: 'uname', field: 'name', label: '用户名' }
    ]
    const w = mount(ManageTable, {
      props: { columns: derivedCols, rows, rowKey: 'id' },
      slots: { cell: ({ col, value }) => h('span', { class: `v-${col.key}` }, String(value)) }
    })
    expect(w.find('.v-status').text()).toBe('undefined')
    expect(w.find('.v-uname').text()).toBe('admin')
  })

  it('派生列无 render/#cell → td 文本为占位符 —', () => {
    const derivedCols: Column<Row>[] = [{ key: 'status', label: '状态' }]
    const w = mount(ManageTable, { props: { columns: derivedCols, rows, rowKey: 'id' } })
    expect(w.find('tbody td').text()).toBe('—')
  })

  it('mobile card 默认渲染同样走 field 取值与占位符 (共享列取值 helper)', () => {
    forceMode('mobile')
    const derivedCols: Column<Row>[] = [
      { key: 'uname', field: 'name', label: '用户名' },
      { key: 'status', label: '状态' }
    ]
    const w = mount(ManageTable, { props: { columns: derivedCols, rows, rowKey: 'id' } })
    const cards = w.findAll('article')
    expect(cards[0].find('header').text()).toBe('admin')
    expect(cards[0].find('dl').text()).toContain('—')
  })
})

describe('ManageTable 表头排序 (T4d)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('sortable 列点表头 → sort-change 循环 asc → desc → null; 按钮带 data-focusable', async () => {
    const sortCols: Column<Row>[] = [
      { key: 'name', label: '用户名', sortable: true },
      { key: 'email', label: '邮箱' }
    ]
    const w = mount(ManageTable, { props: { columns: sortCols, rows, rowKey: 'id' } })
    const btn = w.find('thead th button.jc-th-sort')
    expect(btn.exists()).toBe(true)
    expect(btn.attributes('data-focusable')).toBe('true')
    await btn.trigger('click')
    expect(w.emitted('sort-change')![0]).toEqual([{ key: 'name', order: 'asc' }])
    await btn.trigger('click')
    expect(w.emitted('sort-change')![1]).toEqual([{ key: 'name', order: 'desc' }])
    await btn.trigger('click')
    expect(w.emitted('sort-change')![2]).toEqual([{ key: 'name', order: null }])
    // 无 sortState → 不显示箭头 (受控箭头由父组件驱动)
    expect(btn.find('.jc-th-sort__arrow').exists()).toBe(false)
  })

  it('sortState 命中 → 受控列显示方向箭头', () => {
    const sortCols: Column<Row>[] = [{ key: 'name', label: '用户名', sortable: true }]
    const w = mount(ManageTable, {
      props: { columns: sortCols, rows, rowKey: 'id', sortState: { key: 'name', order: 'desc' } }
    })
    expect(w.find('thead th button.jc-th-sort .jc-th-sort__arrow').text()).toBe('▼')
  })

  it('sorter 列点击 → 客户端排序渲染 (asc/desc/清除回原序), sort-change 仍 emit', async () => {
    const sortRows: Row[] = [
      { id: 1, name: 'c', email: 'c@c.com' },
      { id: 2, name: 'a', email: 'a@a.com' },
      { id: 3, name: 'b', email: 'b@b.com' }
    ]
    const sorterCols: Column<Row>[] = [
      { key: 'name', label: '用户名', sorter: (a, b) => a.name.localeCompare(b.name) },
      { key: 'id', label: 'ID' }
    ]
    const w = mount(ManageTable, { props: { columns: sorterCols, rows: sortRows, rowKey: 'id' } })
    const btn = w.find('thead th button.jc-th-sort')
    const names = () => w.findAll('tbody tr').map(r => r.find('td').text())
    expect(names()).toEqual(['c', 'a', 'b']) // 原序
    await btn.trigger('click')
    expect(names()).toEqual(['a', 'b', 'c']) // asc
    expect(w.emitted('sort-change')![0]).toEqual([{ key: 'name', order: 'asc' }])
    expect(w.find('.jc-th-sort__arrow').text()).toBe('▲')
    await btn.trigger('click')
    expect(names()).toEqual(['c', 'b', 'a']) // desc
    expect(w.emitted('sort-change')![1]).toEqual([{ key: 'name', order: 'desc' }])
    await btn.trigger('click')
    expect(names()).toEqual(['c', 'a', 'b']) // 清除回原序
    expect(w.emitted('sort-change')![2]).toEqual([{ key: 'name', order: null }])
  })

  it('props.rows 不被原地修改 (computed 排序输出副本)', async () => {
    const sortRows: Row[] = [
      { id: 1, name: 'b', email: '' },
      { id: 2, name: 'a', email: '' }
    ]
    const sorterCols: Column<Row>[] = [
      { key: 'name', label: '用户名', sorter: (a, b) => a.name.localeCompare(b.name) }
    ]
    const w = mount(ManageTable, { props: { columns: sorterCols, rows: sortRows, rowKey: 'id' } })
    await w.find('thead th button.jc-th-sort').trigger('click')
    expect(sortRows.map(r => r.name)).toEqual(['b', 'a']) // 原数组不变
  })
})

describe('ManageTable 表头筛选 (T4d)', () => {
  const filterCols: Column<Row>[] = [
    { key: 'name', label: '用户名' },
    {
      key: 'status',
      label: '状态',
      filter: {
        options: [
          { label: '启用', value: 1 },
          { label: '禁用', value: 0 }
        ]
      }
    }
  ]

  beforeEach(() => {
    setActivePinia(createPinia())
    forceMode('desktop')
  })

  it('点漏斗 → 面板出现; 点 option value=1 → filter-change {key,value:1}; 受控回写后再点同值 → undefined', async () => {
    const w = mount(ManageTable, { props: { columns: filterCols, rows, rowKey: 'id' } })
    const funnel = w.find('button[aria-label="筛选 状态"]')
    expect(funnel.exists()).toBe(true)
    expect(funnel.attributes('data-focusable')).toBe('true')
    await funnel.trigger('click')
    expect(w.find('.jc-th-filter__panel').exists()).toBe(true)
    const opt = w.findAll('.jc-th-filter__panel button').find(b => b.text() === '启用')
    expect(opt).toBeTruthy()
    await opt!.trigger('click')
    expect(w.emitted('filter-change')![0]).toEqual([{ key: 'status', value: 1 }])
    // 受控: 父组件把 filter.value 回写为 1
    const hitCols: Column<Row>[] = [
      { key: 'name', label: '用户名' },
      {
        key: 'status',
        label: '状态',
        filter: {
          options: [
            { label: '启用', value: 1 },
            { label: '禁用', value: 0 }
          ],
          value: 1
        }
      }
    ]
    await w.setProps({ columns: hitCols })
    const optAgain = w.findAll('.jc-th-filter__panel button').find(b => b.text() === '启用')
    await optAgain!.trigger('click')
    expect(w.emitted('filter-change')![1]).toEqual([{ key: 'status', value: undefined }])
  })

  it('单选高亮: filter.value 命中的 option 挂 active 类', async () => {
    const hitCols: Column<Row>[] = [
      { key: 'name', label: '用户名' },
      {
        key: 'status',
        label: '状态',
        filter: {
          options: [
            { label: '启用', value: 1 },
            { label: '禁用', value: 0 }
          ],
          value: 0
        }
      }
    ]
    const w = mount(ManageTable, { props: { columns: hitCols, rows, rowKey: 'id' } })
    await w.find('button[aria-label="筛选 状态"]').trigger('click')
    const active = w.find('.jc-th-filter__panel button.jc-th-filter__option-btn--active')
    expect(active.exists()).toBe(true)
    expect(active.text()).toBe('禁用')
  })

  it('multiple: 勾两个 → 第二次 emit 数组长度 2; 全取消 → undefined', async () => {
    const multiFilter: Column<Row>['filter'] = {
      options: [
        { label: '启用', value: 1 },
        { label: '禁用', value: 0 }
      ],
      multiple: true
    }
    const multiCols: Column<Row>[] = [{ key: 'status', label: '状态', filter: { ...multiFilter } }]
    const w = mount(ManageTable, { props: { columns: multiCols, rows, rowKey: 'id' } })
    await w.find('button[aria-label="筛选 状态"]').trigger('click')
    const boxes = w.findAll('.jc-th-filter__panel input[type="checkbox"]')
    expect(boxes).toHaveLength(2)
    await boxes[0].setValue(true)
    expect(w.emitted('filter-change')![0]).toEqual([{ key: 'status', value: [1] }])
    // 受控回写 [1]
    const half: Column<Row>[] = [
      { key: 'status', label: '状态', filter: { ...multiFilter, value: [1] } }
    ]
    await w.setProps({ columns: half })
    const boxes2 = w.findAll('.jc-th-filter__panel input[type="checkbox"]')
    await boxes2[1].setValue(true)
    expect(w.emitted('filter-change')![1]).toEqual([{ key: 'status', value: [1, 0] }])
    // 受控回写 [1,0], 再逐个取消
    const full: Column<Row>[] = [
      { key: 'status', label: '状态', filter: { ...multiFilter, value: [1, 0] } }
    ]
    await w.setProps({ columns: full })
    const boxes3 = w.findAll('.jc-th-filter__panel input[type="checkbox"]')
    await boxes3[0].setValue(false)
    expect(w.emitted('filter-change')![2]).toEqual([{ key: 'status', value: [0] }])
    const half2: Column<Row>[] = [
      { key: 'status', label: '状态', filter: { ...multiFilter, value: [0] } }
    ]
    await w.setProps({ columns: half2 })
    const boxes4 = w.findAll('.jc-th-filter__panel input[type="checkbox"]')
    await boxes4[1].setValue(false)
    expect(w.emitted('filter-change')![3]).toEqual([{ key: 'status', value: undefined }])
  })

  it('ESC 关闭筛选面板', async () => {
    const w = mount(ManageTable, { props: { columns: filterCols, rows, rowKey: 'id' } })
    await w.find('button[aria-label="筛选 状态"]').trigger('click')
    expect(w.find('.jc-th-filter__panel').exists()).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(w.find('.jc-th-filter__panel').exists()).toBe(false)
  })

  it('点击面板外关闭面板', async () => {
    // attachTo: 组件须真实挂进 document, 冒泡到 document 的 mousedown 才能触发外点关闭
    const w = mount(ManageTable, {
      props: { columns: filterCols, rows, rowKey: 'id' },
      attachTo: document.body
    })
    try {
      await w.find('button[aria-label="筛选 状态"]').trigger('click')
      expect(w.find('.jc-th-filter__panel').exists()).toBe(true)
      await w.find('table').trigger('mousedown')
      await nextTick()
      expect(w.find('.jc-th-filter__panel').exists()).toBe(false)
    } finally {
      w.unmount()
    }
  })

  it('再点漏斗按钮 → 面板收起', async () => {
    const w = mount(ManageTable, { props: { columns: filterCols, rows, rowKey: 'id' } })
    const funnel = w.find('button[aria-label="筛选 状态"]')
    await funnel.trigger('click')
    expect(w.find('.jc-th-filter__panel').exists()).toBe(true)
    await funnel.trigger('click')
    expect(w.find('.jc-th-filter__panel').exists()).toBe(false)
  })
})
