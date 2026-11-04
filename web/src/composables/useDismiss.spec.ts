import { describe, it, expect, vi } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { useDismiss } from './useDismiss'

/**
 * useDismiss(规格 §3.1)用例:
 *   外点关闭 / ESC 关闭 / isOpen=false 不监听 / 关闭后卸载监听 / 组件卸载清理 / 初始即打开。
 * jsdom 无 PointerEvent 构造器, 用 MouseEvent 冒充 pointerdown(监听方只看事件类型)。
 */
function firePointerDown(el: Element): void {
  el.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
}
function fireKeydown(key: string): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

/** 宿主组件: anchor 是「触发钮+浮层」的容器, 另造一个 body 下的外部节点模拟 anchor 外点击 */
function mountHost() {
  const onClose = vi.fn()
  const isOpen = ref(false)
  const anchor = ref<Element | null>(null)
  const outside = document.createElement('div')
  document.body.appendChild(outside)
  const Host = defineComponent({
    setup() {
      useDismiss(anchor, isOpen, onClose)
      return () => h('div', { ref: anchor }, [h('button', { type: 'button' }, '触发钮')])
    }
  })
  const w = mount(Host, { attachTo: document.body })
  const innerButton = w.find('button').element
  return {
    w,
    onClose,
    isOpen,
    anchor,
    outside,
    innerButton,
    cleanup() {
      w.unmount()
      outside.remove()
    }
  }
}

describe('useDismiss', () => {
  it('isOpen=true: 点击 anchor 外部 → onClose; 点击 anchor 内(触发钮) → 不触发', async () => {
    const t = mountHost()
    t.isOpen.value = true
    await nextTick()

    firePointerDown(t.outside)
    expect(t.onClose).toHaveBeenCalledTimes(1)

    firePointerDown(t.innerButton)
    expect(t.onClose).toHaveBeenCalledTimes(1) // 容器内点击交给触发钮自己的 click toggle
    t.cleanup()
  })

  it('isOpen=true: ESC → onClose; 其他键 → 不触发', async () => {
    const t = mountHost()
    t.isOpen.value = true
    await nextTick()

    fireKeydown('Enter')
    expect(t.onClose).not.toHaveBeenCalled()
    fireKeydown('Escape')
    expect(t.onClose).toHaveBeenCalledTimes(1)
    t.cleanup()
  })

  it('isOpen=false: 外点与 ESC 均不触发(不监听)', async () => {
    const t = mountHost()
    await nextTick()

    firePointerDown(t.outside)
    fireKeydown('Escape')
    expect(t.onClose).not.toHaveBeenCalled()
    t.cleanup()
  })

  it('isOpen true→false 后停止响应(false 时卸载监听)', async () => {
    const t = mountHost()
    t.isOpen.value = true
    await nextTick()
    t.isOpen.value = false
    await nextTick()

    firePointerDown(t.outside)
    fireKeydown('Escape')
    expect(t.onClose).not.toHaveBeenCalled()

    // 再打开恢复监听
    t.isOpen.value = true
    await nextTick()
    firePointerDown(t.outside)
    expect(t.onClose).toHaveBeenCalledTimes(1)
    t.cleanup()
  })

  it('组件卸载后监听一并清理', async () => {
    const t = mountHost()
    t.isOpen.value = true
    await nextTick()
    t.w.unmount()

    firePointerDown(t.outside)
    fireKeydown('Escape')
    expect(t.onClose).not.toHaveBeenCalled()
    t.outside.remove()
  })

  it('挂载时 isOpen 已为 true → 立即生效(immediate attach)', async () => {
    const t = mountHost()
    t.isOpen.value = true // 在 mount 前置真, 模拟"setup 时即打开"
    const w = t.w
    w.unmount()
    // 重新挂一个初始即打开的宿主
    const onClose = vi.fn()
    const isOpen = ref(true)
    const anchor = ref<Element | null>(null)
    const Host = defineComponent({
      setup() {
        useDismiss(anchor, isOpen, onClose)
        return () => h('div', { ref: anchor })
      }
    })
    const w2 = mount(Host, { attachTo: document.body })
    firePointerDown(t.outside)
    expect(onClose).toHaveBeenCalledTimes(1)
    w2.unmount()
    t.outside.remove()
    void w
  })
})
