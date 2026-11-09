import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

/**
 * 只测渲染: 状态判定(构建号比对/去重)在 usePwaUpdate 的用例里覆盖,
 * 这里把 composable 整体替换成可控的 ref, 避免依赖 SW 事件时序。
 */
const stub = vi.hoisted(() => ({
  applyUpdate: vi.fn(),
  api: null as unknown as {
    updateFound: { value: boolean }
    updateReady: { value: boolean }
    applying: { value: boolean }
    applyUpdate: () => void
    supported: boolean
  }
}))

vi.mock('@/composables/usePwaUpdate', async () => {
  const { ref } = await import('vue')
  const api = {
    updateFound: ref(false),
    updateReady: ref(false),
    applying: ref(false),
    applyUpdate: stub.applyUpdate,
    supported: true
  }
  stub.api = api
  return { usePwaUpdate: () => api }
})

// vi.mock 会被提升到 import 之前, 组件拿到的就是下面的桩实现
import PwaUpdateBadge from './PwaUpdateBadge.vue'

const setState = async (s: {
  updateFound?: boolean
  updateReady?: boolean
  applying?: boolean
}): Promise<void> => {
  stub.api.updateFound.value = s.updateFound ?? false
  stub.api.updateReady.value = s.updateReady ?? false
  stub.api.applying.value = s.applying ?? false
  await nextTick()
}

describe('PwaUpdateBadge', () => {
  beforeEach(() => {
    stub.applyUpdate.mockClear()
    stub.api.updateFound.value = false
    stub.api.updateReady.value = false
    stub.api.applying.value = false
  })

  it('无新版本时不渲染', async () => {
    const w = mount(PwaUpdateBadge)
    await nextTick()
    expect(w.find('.jc-pwa-badge').exists()).toBe(false)
  })

  it('检测到更新 → 渲染 amber 态, 带遥控器可聚焦属性', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('发现更新')
    expect(btn.classes()).toContain('is-found')
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.attributes('tabindex')).toBe('0')
    expect(btn.attributes('type')).toBe('button')
  })

  it('新版本已接管 → 渲染"新版本"且不再是 amber', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, updateReady: true })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.text()).toContain('新版本')
    expect(btn.classes()).not.toContain('is-found')
  })

  it('更新中 → 显示"更新中"并禁用(防重复点击)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, applying: true })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.text()).toContain('更新中')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('aria-busy')).toBe('true')
  })

  it('点击 → 触发 applyUpdate', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateReady: true })

    await w.find('.jc-pwa-badge').trigger('click')
    expect(stub.applyUpdate).toHaveBeenCalledTimes(1)
  })
})
