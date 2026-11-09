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
    progress: { value: number }
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
    progress: ref(0),
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
  progress?: number
}): Promise<void> => {
  stub.api.updateFound.value = s.updateFound ?? false
  stub.api.updateReady.value = s.updateReady ?? false
  stub.api.applying.value = s.applying ?? false
  stub.api.progress.value = s.progress ?? 0
  await nextTick()
}

/** 填充比例(写在 --jc-pwa-fill 上, 由 clip-path 消费) */
const fillOf = (w: ReturnType<typeof mount>): string =>
  (w.find('.jc-pwa-badge__glyph').attributes('style') ?? '').match(/--jc-pwa-fill:\s*([^;"]+)/)?.[1] ??
  ''

describe('PwaUpdateBadge', () => {
  beforeEach(() => {
    stub.applyUpdate.mockClear()
    stub.api.updateFound.value = false
    stub.api.updateReady.value = false
    stub.api.applying.value = false
    stub.api.progress.value = 0
  })

  it('无新版本时不渲染', async () => {
    const w = mount(PwaUpdateBadge)
    await nextTick()
    expect(w.find('.jc-pwa-badge').exists()).toBe(false)
  })

  it('检测到更新 → 渲染 amber 圆形图标按钮(不显示文字), 带遥控器可聚焦属性', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toBe('') // 文字退到 aria-label, 视觉上只有箭头
    expect(btn.attributes('aria-label')).toContain('发现新版本')
    expect(w.findAll('.jc-pwa-badge__glyph .jc-icon').length).toBe(2) // 底 + 填充层
    expect(btn.classes()).toContain('is-found')
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.attributes('tabindex')).toBe('0')
    expect(btn.attributes('type')).toBe('button')
  })

  it('填充比例跟随 progress(按落地进度从下往上涨)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0.42 })
    expect(fillOf(w)).toBe('42%')

    await setState({ updateFound: true, progress: 1 })
    // 未接管时封顶 95% —— 还在装就不能显示"满"
    expect(fillOf(w)).toBe('95%')
  })

  it('刚发现时也有一小格可见(不能看起来毫无反应)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0 })
    expect(fillOf(w)).toBe('6%')
  })

  it('新版本已接管 → 填满 100% 且转 ready 态(不再是 amber)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, updateReady: true, progress: 0.3 })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.attributes('aria-label')).toContain('新版本已就绪')
    expect(btn.classes()).not.toContain('is-found')
    expect(btn.classes()).toContain('is-ready')
    expect(fillOf(w)).toBe('100%')
  })

  it('更新中 → 禁用(防重复点击)并标注 aria-busy', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, applying: true, progress: 0.6 })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.attributes('aria-label')).toContain('正在切换到新版本')
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
