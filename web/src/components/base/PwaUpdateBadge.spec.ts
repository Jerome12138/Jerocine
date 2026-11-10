import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

/**
 * 只测渲染与可点性: 状态判定(构建号比对/去重/SW 状态)在 usePwaUpdate 的用例里覆盖,
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

/** 箭头图形(取填充层的 path) —— 断言是向上直箭头而不是循环箭头 */
const arrowPathOf = (w: ReturnType<typeof mount>): string =>
  w.find('.jc-pwa-badge__liquid svg path').attributes('d') ?? ''

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

  it('下载中 → 渲染 amber 圆形图标(向上箭头, 不显示文字), 带遥控器可聚焦属性', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toBe('') // 文字退到 aria-label, 视觉上只有箭头
    expect(w.findAll('.jc-pwa-badge__glyph .jc-icon').length).toBe(2) // 底 + 填充层
    // 向上直箭头(Material arrow_upward), 不是循环箭头(refresh)
    expect(arrowPathOf(w)).toMatch(/^M4 12l1\.41 1\.41L11 7\.83V20h2V7\.83/)
    expect(btn.classes()).toContain('is-found')
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.attributes('tabindex')).toBe('0')
    expect(btn.attributes('type')).toBe('button')
  })

  it('下载中 → 不可点: aria-disabled 且点击不触发刷新(还没装完, 刷了也是旧页面)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0.4 })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.attributes('aria-disabled')).toBe('true')
    await btn.trigger('click')
    expect(stub.applyUpdate).not.toHaveBeenCalled()
  })

  it('下载中 → 悬停提示带百分比(让用户判断"到底好没好")', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0.42 })

    const title = w.find('.jc-pwa-badge').attributes('title') ?? ''
    expect(title).toContain('正在下载新版本')
    expect(title).toContain('42%')
  })

  it('填充比例跟随 progress(按落地进度从下往上涨), 下载中封顶 99%', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0.42 })
    expect(fillOf(w)).toBe('42%')

    await setState({ updateFound: true, progress: 1 })
    // 满格要留给"可以点", 还在装就不能显示 100%
    expect(fillOf(w)).toBe('99%')
  })

  it('刚发现时也有一小格可见(不能看起来毫无反应)', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, progress: 0 })
    expect(fillOf(w)).toBe('6%')
  })

  it('已就绪 → 填满 100%, 转 ready 态且可点', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, updateReady: true, progress: 0.3 })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.attributes('aria-label')).toContain('新版本已就绪')
    expect(btn.attributes('aria-disabled')).toBeUndefined()
    expect(btn.classes()).not.toContain('is-found')
    expect(btn.classes()).toContain('is-ready')
    expect(fillOf(w)).toBe('100%')

    await btn.trigger('click')
    expect(stub.applyUpdate).toHaveBeenCalledTimes(1)
  })

  it('切换中 → 标注 aria-busy 且不可再点', async () => {
    const w = mount(PwaUpdateBadge)
    await setState({ updateFound: true, updateReady: true, applying: true, progress: 1 })

    const btn = w.find('.jc-pwa-badge')
    expect(btn.attributes('aria-label')).toContain('正在切换到新版本')
    expect(btn.attributes('aria-busy')).toBe('true')
    expect(btn.attributes('aria-disabled')).toBe('true')
    expect(fillOf(w)).toBe('100%')

    await btn.trigger('click')
    expect(stub.applyUpdate).not.toHaveBeenCalled()
  })
})
