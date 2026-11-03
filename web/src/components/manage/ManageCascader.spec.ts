import { describe, it, expect } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import type { FilmClass } from '@/types/manage'
import ManageCascader from './ManageCascader.vue'

/** 三层树: 1 → 2 → (3, 4) */
function tree(): FilmClass[] {
  return [
    {
      id: 1,
      pid: 0,
      name: '电影',
      show: true,
      sidebarShow: true,
      filmVisible: true,
      children: [
        {
          id: 2,
          pid: 1,
          name: '动作片',
          show: true,
          sidebarShow: true,
          filmVisible: true,
          children: [
            { id: 3, pid: 2, name: '拳类', show: true, sidebarShow: true, filmVisible: true },
            { id: 4, pid: 2, name: '腿类', show: true, sidebarShow: true, filmVisible: true }
          ]
        }
      ]
    }
  ]
}

function boxes(w: VueWrapper): HTMLInputElement[] {
  return w.findAll('input[type="checkbox"]').map((c) => c.element as HTMLInputElement)
}

/** 按名称点开某一行(触发下一列展开) */
function rowByName(w: VueWrapper, name: string) {
  const row = w.findAll('[role="button"]').find((r) => r.text().includes(name))
  expect(row, `行 ${name} 应存在`).toBeTruthy()
  return row!
}

describe('ManageCascader (EP 风格多列)', () => {
  it('初始只渲染第一列(顶层节点)', () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    expect(w.findAll('.jc-cas__col').length).toBe(1)
    expect(boxes(w).length).toBe(1)
    w.unmount()
  })

  it('点击带子级的节点 → 右侧展开下一列', async () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    await rowByName(w, '电影').trigger('click')
    expect(w.findAll('.jc-cas__col').length).toBe(2)
    await rowByName(w, '动作片').trigger('click')
    expect(w.findAll('.jc-cas__col').length).toBe(3)
    // 末列渲染出叶子 拳类/腿类
    expect(w.text()).toContain('拳类')
    expect(w.text()).toContain('腿类')
    w.unmount()
  })

  it('叶子节点点击即勾选(无需先展开列)', async () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    await rowByName(w, '电影').trigger('click')
    await rowByName(w, '动作片').trigger('click')
    await rowByName(w, '拳类').trigger('click')
    const emitted = w.emitted('change')!.at(-1)![0] as Set<number>
    expect([...emitted]).toEqual([3])
    w.unmount()
  })

  it('勾选父级复选框 → 整棵子树全选; 再勾 → 全取消', async () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    const top = w.findAll('input[type="checkbox"]')[0]
    await top.trigger('change')
    let emitted = w.emitted('change')!.at(-1)![0] as Set<number>
    expect([...emitted].sort()).toEqual([1, 2, 3, 4])

    await w.setProps({ selected: emitted })
    await top.trigger('change')
    emitted = w.emitted('change')!.at(-1)![0] as Set<number>
    expect(emitted.size).toBe(0)
    w.unmount()
  })

  it('仅第 3 层叶子选中 → 顶层 indeterminate=true(递归子孙)', async () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set([3]) } })
    const top = boxes(w)[0]
    expect(top.indeterminate).toBe(true)
    expect(top.checked).toBe(false)
    // 展开到第二列, 中层同样 indeterminate
    await rowByName(w, '电影').trigger('click')
    const mid = boxes(w)[1]
    expect(mid.indeterminate).toBe(true)
    w.unmount()
  })

  it('子级全选 → 顶层 checked=true 且非 indeterminate', () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set([1, 2, 3, 4]) } })
    const top = boxes(w)[0]
    expect(top.checked).toBe(true)
    expect(top.indeterminate).toBe(false)
    w.unmount()
  })

  it('MAX_DEPTH=4: 逐级展开最多 5 列, 更深一层不再展开', async () => {
    const node = (id: number, children?: FilmClass[]): FilmClass => ({
      id,
      pid: id - 1,
      name: `n${id}`,
      show: true,
      sidebarShow: true,
      filmVisible: true,
      children
    })
    const deep: FilmClass = node(1, [node(2, [node(3, [node(4, [node(5, [node(6)])])])])])
    const w = mount(ManageCascader, { props: { nodes: [deep], selected: new Set<number>() } })
    for (const name of ['n1', 'n2', 'n3', 'n4']) {
      await rowByName(w, name).trigger('click')
    }
    // 已展开 5 列(层级 0..4)
    expect(w.findAll('.jc-cas__col').length).toBe(5)
    // 第 5 层的 n5 有子级但 level=4 不再展开 → n6 不渲染
    await rowByName(w, 'n5').trigger('click')
    expect(w.findAll('.jc-cas__col').length).toBe(5)
    expect(w.text()).not.toContain('n6')
    w.unmount()
  })
})
