import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import type { FilmClass } from '@/types/manage'
import ManageCascader from './ManageCascader.vue'

/** 三层树: 1 → 2 → (3, 4), 复核 #14 的最小复现结构 */
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

function boxes(w: ReturnType<typeof mount>): HTMLInputElement[] {
  return w.findAll('input[type="checkbox"]').map((c) => c.element as HTMLInputElement)
}

describe('ManageCascader', () => {
  it('仅第 3 层叶子选中 → 顶层 indeterminate=true (复核 #14: 半选需递归子孙)', () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set([3]) } })
    const [top, mid] = boxes(w)
    expect(top.indeterminate).toBe(true)
    expect(top.checked).toBe(false)
    // 中间层同样有子孙被选中 → indeterminate
    expect(mid.indeterminate).toBe(true)
    w.unmount()
  })

  it('子级全选 → 顶层 checked=true 且非 indeterminate', () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set([1, 2, 3, 4]) } })
    const [top] = boxes(w)
    expect(top.checked).toBe(true)
    expect(top.indeterminate).toBe(false)
    w.unmount()
  })

  it('无选中 → 顶层既非 checked 也非 indeterminate', () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    const [top] = boxes(w)
    expect(top.checked).toBe(false)
    expect(top.indeterminate).toBe(false)
    w.unmount()
  })

  it('toggle 顶层 → 子树全选(change 集合含全部 id); 再 toggle → 全取消', async () => {
    const w = mount(ManageCascader, { props: { nodes: tree(), selected: new Set<number>() } })
    const first = w.findAll('input[type="checkbox"]')[0]
    await first.trigger('change')
    let emitted = w.emitted('change')!.at(-1)![0] as Set<number>
    expect([...emitted].sort()).toEqual([1, 2, 3, 4])

    await w.setProps({ selected: emitted })
    await first.trigger('change')
    emitted = w.emitted('change')!.at(-1)![0] as Set<number>
    expect(emitted.size).toBe(0)
    w.unmount()
  })

  it('MAX_DEPTH=4: 5 层链只渲染到第 5 个节点, 更深不渲染', () => {
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
    expect(w.findAll('input[type="checkbox"]').length).toBe(5)
    w.unmount()
  })
})
