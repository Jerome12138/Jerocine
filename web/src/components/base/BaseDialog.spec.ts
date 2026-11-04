import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseDialog from './BaseDialog.vue'

function clearBody(): void {
  // 清测试间 Teleport 残留 (不操作生产用户输入, 仅测试隔离)
  while (document.body.firstChild) {
    document.body.removeChild(document.body.firstChild)
  }
}

describe('BaseDialog', () => {
  beforeEach(() => clearBody())

  it('visible=true 渲染 dialog', async () => {
    const w = mount(BaseDialog, { props: { visible: true, title: 'T' }, attachTo: document.body })
    await w.vm.$nextTick()
    expect(document.body.textContent).toContain('T')
    w.unmount()
  })

  it('visible=false 不渲染', async () => {
    const w = mount(BaseDialog, { props: { visible: false, title: 'X' }, attachTo: document.body })
    await w.vm.$nextTick()
    expect(document.body.textContent ?? '').not.toContain('X')
    w.unmount()
  })

  it('title 显示', async () => {
    const w = mount(BaseDialog, {
      props: { visible: true, title: '自定义标题' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    expect(document.body.textContent).toContain('自定义标题')
    w.unmount()
  })

  it('面板 maxWidth=94vw 窄口兜底(D⑦), 调用点 width 仍原样生效', async () => {
    const w = mount(BaseDialog, {
      props: { visible: true, title: 'T', width: '880px' },
      attachTo: document.body
    })
    await w.vm.$nextTick()
    const dialog = document.querySelector('.jc-dialog') as HTMLElement
    // 窄口兜底(规格 §4): maxWidth 统一 94vw, 窄窗(<720px)打开弹窗不溢出
    expect(dialog.style.maxWidth).toBe('94vw')
    // 调用点 width 不动(D⑦): 保持固定 px
    expect(dialog.style.width).toBe('880px')
    w.unmount()
  })

  it('双弹层: ESC 只关最顶层; 顶层关闭释放后 ESC 再关底层', async () => {
    const wA = mount(BaseDialog, { props: { visible: true, title: 'A' }, attachTo: document.body })
    await wA.vm.$nextTick()
    const wB = mount(BaseDialog, { props: { visible: true, title: 'B' }, attachTo: document.body })
    await wB.vm.$nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wB.vm.$nextTick()
    expect(wB.emitted('update:visible')?.length).toBe(1)
    expect(wA.emitted('update:visible')).toBeUndefined()

    // 父组件响应 emit → B 真正关闭并释放层序
    await wB.setProps({ visible: false })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wA.vm.$nextTick()
    expect(wA.emitted('update:visible')?.length).toBe(1)

    await wA.setProps({ visible: false })
    wA.unmount()
    wB.unmount()
  })
})
