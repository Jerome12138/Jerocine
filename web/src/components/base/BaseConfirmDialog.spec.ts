import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseConfirmDialog from './BaseConfirmDialog.vue'
import { confirm, answerConfirm, confirmState } from '@/composables/useConfirm'

function clearBody(): void {
  while (document.body.firstChild) document.body.removeChild(document.body.firstChild)
}

describe('BaseConfirmDialog', () => {
  beforeEach(() => {
    clearBody()
    confirmState.visible = false
    confirmState.resolve = null
  })

  it('confirmState.visible=false → 不渲染', async () => {
    const w = mount(BaseConfirmDialog, { attachTo: document.body })
    await w.vm.$nextTick()
    expect(document.body.textContent ?? '').not.toContain('确认')
    w.unmount()
  })

  it('confirm() 调用 → dialog 显示 title', async () => {
    const w = mount(BaseConfirmDialog, { attachTo: document.body })
    confirm({ title: '删除这条记录?' })
    await w.vm.$nextTick()
    expect(document.body.textContent).toContain('删除这条记录?')
    answerConfirm(false)
    w.unmount()
  })

  it('danger=true → 确认按钮变 danger variant', async () => {
    const w = mount(BaseConfirmDialog, { attachTo: document.body })
    confirm({ title: 'X', danger: true })
    await w.vm.$nextTick()
    // BaseButton danger variant 应被使用
    expect(document.body.textContent).toContain('X')
    answerConfirm(false)
    w.unmount()
  })

  it('点击 cancel → resolve(false)', async () => {
    const w = mount(BaseConfirmDialog, { attachTo: document.body })
    const p = confirm({ title: 'X' })
    await w.vm.$nextTick()
    // 找 cancel 按钮 (含 cancelText)
    const buttons = document.body.querySelectorAll('button')
    const cancelBtn = Array.from(buttons).find(b => b.textContent?.includes('取消'))
    cancelBtn?.click()
    const result = await p
    expect(result).toBe(false)
    w.unmount()
  })

  it('点击 ok → resolve(true)', async () => {
    const w = mount(BaseConfirmDialog, { attachTo: document.body })
    const p = confirm({ title: 'X' })
    await w.vm.$nextTick()
    const buttons = document.body.querySelectorAll('button')
    const okBtn = Array.from(buttons).find(b => b.textContent?.includes('确认'))
    okBtn?.click()
    const result = await p
    expect(result).toBe(true)
    w.unmount()
  })
})
