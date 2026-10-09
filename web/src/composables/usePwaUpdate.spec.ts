import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { usePwaUpdate } from './usePwaUpdate'
import { CHUNK_RELOAD_GUARD_KEY } from '@/utils/chunkReload'

vi.mock('@/utils/telemetry', () => ({
  telemetry: { track: vi.fn(), trackError: vi.fn(), trackPv: vi.fn(), trackApi: vi.fn() }
}))

interface FakeSW {
  listeners: Set<() => void>
}

/** 装一个假 navigator.serviceWorker; controller 决定"进入页面时是否已被 SW 控制" */
function installFakeSW(controller: unknown): FakeSW {
  const listeners = new Set<() => void>()
  const fake = {
    controller,
    addEventListener: (_t: string, cb: () => void) => {
      listeners.add(cb)
    },
    removeEventListener: (_t: string, cb: () => void) => {
      listeners.delete(cb)
    },
    getRegistration: async () => undefined
  }
  Object.defineProperty(navigator, 'serviceWorker', { value: fake, configurable: true })
  return { listeners }
}

function removeFakeSW(): void {
  delete (navigator as unknown as { serviceWorker?: unknown }).serviceWorker
}

const Host = defineComponent({
  setup() {
    return { api: usePwaUpdate() }
  },
  render() {
    return h('div')
  }
})

type Api = ReturnType<typeof usePwaUpdate>
const apiOf = (w: ReturnType<typeof mount>): Api =>
  (w.vm as never as { api: Api }).api

const fire = async (sw: FakeSW): Promise<void> => {
  for (const cb of sw.listeners) cb()
  await nextTick()
}

describe('usePwaUpdate', () => {
  beforeEach(() => {
    sessionStorage.clear()
    removeFakeSW()
  })
  afterEach(() => {
    removeFakeSW()
  })

  it('环境不支持 Service Worker → supported=false, 不监听', async () => {
    removeFakeSW()
    const w = mount(Host)
    await nextTick()
    expect(apiOf(w).supported).toBe(false)
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('首访(进入时无 controller) → 初次接管不算更新, 不提示', async () => {
    const sw = installFakeSW(null)
    const w = mount(Host)
    await nextTick()
    expect(apiOf(w).supported).toBe(true)

    await fire(sw) // 初次 clientsClaim 触发的 controllerchange
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('回访(进入时已有 controller) → controllerchange 即"新版本就绪"', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mount(Host)
    await nextTick()
    expect(apiOf(w).updateReady.value).toBe(false)

    await fire(sw)
    expect(apiOf(w).updateReady.value).toBe(true)
  })

  it('新版本接管后清掉 chunk 自愈闸门(恢复自愈能力)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    sessionStorage.setItem(CHUNK_RELOAD_GUARD_KEY, '1')
    const w = mount(Host)
    await nextTick()

    await fire(sw)
    expect(sessionStorage.getItem(CHUNK_RELOAD_GUARD_KEY)).toBeNull()
  })

  it('卸载后移除 controllerchange 监听', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mount(Host)
    await nextTick()
    expect(sw.listeners.size).toBe(1)

    w.unmount()
    expect(sw.listeners.size).toBe(0)
  })
})
