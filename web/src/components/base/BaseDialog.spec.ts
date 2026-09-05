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
})
