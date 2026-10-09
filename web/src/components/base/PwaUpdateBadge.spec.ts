import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PwaUpdateBadge from './PwaUpdateBadge.vue'

vi.mock('@/utils/telemetry', () => ({
  telemetry: { track: vi.fn(), trackError: vi.fn(), trackPv: vi.fn(), trackApi: vi.fn() }
}))

const reloadSpy = vi.fn()

interface FakeSW {
  listeners: Set<() => void>
}

function installFakeSW(controller: unknown): FakeSW {
  const listeners = new Set<() => void>()
  Object.defineProperty(navigator, 'serviceWorker', {
    value: {
      controller,
      addEventListener: (_t: string, cb: () => void) => listeners.add(cb),
      removeEventListener: (_t: string, cb: () => void) => listeners.delete(cb),
      getRegistration: async () => undefined
    },
    configurable: true
  })
  return { listeners }
}

const fire = async (sw: FakeSW): Promise<void> => {
  for (const cb of sw.listeners) cb()
  await nextTick()
}

describe('PwaUpdateBadge', () => {
  beforeEach(() => {
    sessionStorage.clear()
    reloadSpy.mockClear()
    Object.defineProperty(window, 'location', {
      value: { ...window.location, reload: reloadSpy },
      configurable: true
    })
  })
  afterEach(() => {
    delete (navigator as unknown as { serviceWorker?: unknown }).serviceWorker
  })

  it('无新版本时不渲染', async () => {
    installFakeSW({ state: 'activated' })
    const w = mount(PwaUpdateBadge)
    await nextTick()
    expect(w.find('.jc-pwa-badge').exists()).toBe(false)
  })

  it('首访初次接管(无 controller)不渲染', async () => {
    const sw = installFakeSW(null)
    const w = mount(PwaUpdateBadge)
    await fire(sw)
    expect(w.find('.jc-pwa-badge').exists()).toBe(false)
  })

  it('新版本接管后渲染, 带遥控器可聚焦属性', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mount(PwaUpdateBadge)
    await fire(sw)

    const btn = w.find('.jc-pwa-badge')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('新版本')
    expect(btn.attributes('data-focusable')).toBe('true')
    expect(btn.attributes('tabindex')).toBe('0')
    expect(btn.attributes('type')).toBe('button')
  })

  it('点击 → 刷新页面', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mount(PwaUpdateBadge)
    await fire(sw)

    await w.find('.jc-pwa-badge').trigger('click')
    expect(reloadSpy).toHaveBeenCalledTimes(1)
  })
})
