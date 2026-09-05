import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { useNetworkHint } from './useNetworkHint'

// 容器组件，把 composable return 挂到 setup 暴露
const Host = defineComponent({
  setup() {
    const api = useNetworkHint()
    return { api }
  },
  render() {
    return h('div')
  }
})

function setConnection(conn: Partial<{ effectiveType: string; saveData: boolean }> | null): void {
  Object.defineProperty(navigator, 'connection', {
    value: conn ? {
      ...conn,
      addEventListener: () => undefined,
      removeEventListener: () => undefined
    } : undefined,
    configurable: true
  })
}

describe('useNetworkHint', () => {
  beforeEach(() => {
    localStorage.clear()
    setConnection(null)
  })

  it('无 navigator.connection → effectiveType undefined, isSlow=false (auto)', async () => {
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    expect(api.effectiveType.value).toBeUndefined()
    expect(api.isSlow.value).toBe(false)
  })

  it('effectiveType=4g → isSlow=false', async () => {
    setConnection({ effectiveType: '4g' })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    expect(api.isSlow.value).toBe(false)
  })

  it('effectiveType=3g → isSlow=true', async () => {
    setConnection({ effectiveType: '3g' })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    expect(api.isSlow.value).toBe(true)
  })

  it('effectiveType=2g → isSlow=true', async () => {
    setConnection({ effectiveType: '2g' })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    expect(api.isSlow.value).toBe(true)
  })

  it('saveData=true → isSlow=true (即使 4g)', async () => {
    setConnection({ effectiveType: '4g', saveData: true })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    expect(api.isSlow.value).toBe(true)
  })

  it('manualPref=slow 强制 isSlow=true (即使 4g)', async () => {
    setConnection({ effectiveType: '4g' })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    api.setManualPref('slow')
    expect(api.isSlow.value).toBe(true)
  })

  it('manualPref=fast 强制 isSlow=false (即使 2g)', async () => {
    setConnection({ effectiveType: '2g' })
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    api.setManualPref('fast')
    expect(api.isSlow.value).toBe(false)
  })

  it('setManualPref 持久化到 localStorage', async () => {
    const w = mount(Host)
    await w.vm.$nextTick()
    const api = (w.vm as never as { api: ReturnType<typeof useNetworkHint> }).api
    api.setManualPref('slow')
    expect(localStorage.getItem('gf-network-pref')).toBe('slow')
    api.setManualPref('auto')
    expect(localStorage.getItem('gf-network-pref')).toBeNull()
  })
})
