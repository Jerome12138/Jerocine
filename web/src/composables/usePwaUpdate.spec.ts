import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { usePwaUpdate, isPageStale, parseBuildTag, type PwaUpdateDeps } from './usePwaUpdate'
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

/** 带依赖注入的宿主(构建号/探测函数可注入, 生产路径走默认实现) */
function mountWithDeps(deps: PwaUpdateDeps) {
  const HostWithDeps = defineComponent({
    setup() {
      return { api: usePwaUpdate(deps) }
    },
    render() {
      return h('div')
    }
  })
  return mount(HostWithDeps)
}

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

  // ---- 2026-10-31 修: 判据由"控制器换过"改为"构建号比对" ----
  // 报障场景: 旧页面点橙色「发现更新」⇒ 清缓存+reload(页面已是新版, 但控制器还是旧 SW)
  //          ⇒ 新 SW 随后激活 claim, controllerchange 二次触发 ⇒ 误报蓝色「新版本」。

  it('构建号相同 → 页面已是最新, 不再提示(报障回归)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: '20261029-120000',
      probeLatestBuild: async () => '20261029-120000'
    })
    await nextTick()

    await fire(sw)
    await flushPromises()

    expect(apiOf(w).updateReady.value).toBe(false)
    expect(apiOf(w).updateFound.value).toBe(false)
  })

  it('构建号不同 → 页面确实落后, 正常提示"新版本"', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: '20261029-120000',
      probeLatestBuild: async () => '20261031-101010'
    })
    await nextTick()

    await fire(sw)
    await flushPromises()

    expect(apiOf(w).updateReady.value).toBe(true)
    expect(apiOf(w).updateFound.value).toBe(true)
  })

  it('探测失败(空串/抛错) → 保守提示, 不让 TV 常驻停在旧版', async () => {
    for (const probe of [async () => '', async () => Promise.reject(new Error('offline'))]) {
      removeFakeSW()
      const sw = installFakeSW({ state: 'activated' })
      const w = mountWithDeps({ ownBuild: '20261029-120000', probeLatestBuild: probe })
      await nextTick()

      await fire(sw)
      await flushPromises()

      expect(apiOf(w).updateReady.value).toBe(true)
      w.unmount()
    }
  })

  it('首访(无 controller)不发起构建号探测', async () => {
    const probe = vi.fn(async () => '20261031-101010')
    const sw = installFakeSW(null)
    const w = mountWithDeps({ ownBuild: '20261029-120000', probeLatestBuild: probe })
    await nextTick()

    await fire(sw)
    await flushPromises()

    expect(probe).not.toHaveBeenCalled()
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('未注入构建号(dev/旧构建) → 沿用旧语义: 有接管即提示', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: '', probeLatestBuild: async () => '20261031-101010' })
    await nextTick()

    await fire(sw)
    await flushPromises()

    expect(apiOf(w).updateReady.value).toBe(true)
  })

  describe('isPageStale / parseBuildTag', () => {
    const base = { hadController: true, ownBuild: 'a', latestBuild: 'a' }

    it('首访(无 controller) 一律不算更新', () => {
      expect(isPageStale({ ...base, hadController: false, latestBuild: 'b' })).toBe(false)
    })

    it('构建号相同 = 页面已最新', () => {
      expect(isPageStale(base)).toBe(false)
    })

    it('构建号不同 = 页面落后', () => {
      expect(isPageStale({ ...base, latestBuild: 'b' })).toBe(true)
    })

    it('任一构建号未知 = 保守视为落后', () => {
      expect(isPageStale({ ...base, ownBuild: '' })).toBe(true)
      expect(isPageStale({ ...base, latestBuild: '' })).toBe(true)
    })

    it('parseBuildTag 从 index.html 取 assets/<TS>/', () => {
      expect(parseBuildTag('<script src="/assets/20261031-101010/index-abc.js"></script>')).toBe(
        '20261031-101010'
      )
      expect(parseBuildTag('<script src="/assets/index-abc.js"></script>')).toBe('')
      expect(parseBuildTag('')).toBe('')
    })
  })
})
