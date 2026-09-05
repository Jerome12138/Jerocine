import { describe, it, expect, beforeEach } from 'vitest'
import { confirm, answerConfirm, confirmState } from './useConfirm'

describe('useConfirm', () => {
  beforeEach(() => {
    confirmState.visible = false
    confirmState.resolve = null
  })

  it('confirm() 设 visible=true + 保存 resolve', () => {
    const p = confirm({ title: '删除？' })
    expect(confirmState.visible).toBe(true)
    expect(confirmState.title).toBe('删除？')
    expect(confirmState.resolve).toBeTypeOf('function')
    answerConfirm(false)
    return p
  })

  it('options 默认值: okText=确认 / cancelText=取消 / danger=false', () => {
    confirm({ title: 'x' })
    expect(confirmState.okText).toBe('确认')
    expect(confirmState.cancelText).toBe('取消')
    expect(confirmState.danger).toBe(false)
    answerConfirm(false)
  })

  it('options 自定义文案 + danger 生效', () => {
    confirm({ title: 'x', desc: 'y', okText: '删', cancelText: '取消', danger: true })
    expect(confirmState.desc).toBe('y')
    expect(confirmState.okText).toBe('删')
    expect(confirmState.danger).toBe(true)
    answerConfirm(false)
  })

  it('answerConfirm(true) → Promise resolve(true) + visible=false', async () => {
    const p = confirm({ title: 'x' })
    answerConfirm(true)
    await expect(p).resolves.toBe(true)
    expect(confirmState.visible).toBe(false)
    expect(confirmState.resolve).toBeNull()
  })

  it('answerConfirm(false) → Promise resolve(false)', async () => {
    const p = confirm({ title: 'x' })
    answerConfirm(false)
    await expect(p).resolves.toBe(false)
  })

  it('已有待应答 confirm 再开新的, 旧 Promise resolve(false)', async () => {
    const p1 = confirm({ title: 'A' })
    confirm({ title: 'B' })
    await expect(p1).resolves.toBe(false)
    expect(confirmState.title).toBe('B')
    answerConfirm(false)
  })
})
