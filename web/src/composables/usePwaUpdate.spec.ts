import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import {
  usePwaUpdate,
  isPageStale,
  parseBuildTag,
  resetPromptedBuildForTest,
  type PwaUpdateDeps
} from './usePwaUpdate'
import { CHUNK_RELOAD_GUARD_KEY } from '@/utils/chunkReload'

vi.mock('@/utils/telemetry', () => ({
  telemetry: { track: vi.fn(), trackError: vi.fn(), trackPv: vi.fn(), trackApi: vi.fn() }
}))

interface FakeSW {
  /** controllerchange 监听 */
  listeners: Set<() => void>
  /** updatefound 监听(挂在 registration 上) */
  updateFoundListeners: Set<() => void>
  /** 触发 registration.updatefound */
  fireUpdateFound: () => void
}

/**
 * 装一个假 navigator.serviceWorker.
 * @param controller 进入页面时是否已被 SW 控制(null = 首访)
 * @param opts.withRegistration 是否让 getRegistration() 返回一个注册对象(默认返回)
 */
function installFakeSW(
  controller: unknown,
  opts: { withRegistration?: boolean } = {}
): FakeSW {
  const listeners = new Set<() => void>()
  const updateFoundListeners = new Set<() => void>()
  const reg = {
    update: async () => undefined,
    waiting: null as { postMessage: (m: unknown) => void } | null,
    addEventListener: (t: string, cb: () => void) => {
      if (t === 'updatefound') updateFoundListeners.add(cb)
    },
    removeEventListener: (t: string, cb: () => void) => {
      if (t === 'updatefound') updateFoundListeners.delete(cb)
    }
  }
  const fake = {
    controller,
    addEventListener: (_t: string, cb: () => void) => {
      listeners.add(cb)
    },
    removeEventListener: (_t: string, cb: () => void) => {
      listeners.delete(cb)
    },
    getRegistration: async () => (opts.withRegistration === false ? undefined : reg)
  }
  Object.defineProperty(navigator, 'serviceWorker', { value: fake, configurable: true })
  return {
    listeners,
    updateFoundListeners,
    fireUpdateFound: () => {
      for (const cb of updateFoundListeners) cb()
    }
  }
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

/** 触发 controllerchange(所有监听者) */
const fire = async (sw: FakeSW): Promise<void> => {
  for (const cb of sw.listeners) cb()
  await nextTick()
}

/** 等异步链路(含 0ms 兜底定时器)跑完 */
const settle = async (): Promise<void> => {
  await flushPromises()
  await new Promise((r) => setTimeout(r, 5))
  await flushPromises()
}

const OWN = '20261029-120000'
const NEWER = '20261031-101010'

describe('usePwaUpdate', () => {
  beforeEach(() => {
    sessionStorage.clear()
    resetPromptedBuildForTest()
    removeFakeSW()
    // 默认探测失败(返回不含构建号的 HTML): 绝大多数用例不关心初始核对, 让它静默
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, text: async () => '<html></html>' }))
    )
  })
  afterEach(() => {
    removeFakeSW()
    vi.unstubAllGlobals()
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
    await settle()
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('回访(进入时已有 controller) + 构建号不同 → 提示"新版本就绪"', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()

    await fire(sw)
    await settle()

    expect(apiOf(w).updateFound.value).toBe(true)
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

  it('卸载后移除 controllerchange 与 updatefound 监听', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mount(Host)
    await settle()
    expect(sw.listeners.size).toBe(1)
    expect(sw.updateFoundListeners.size).toBe(1)

    w.unmount()
    expect(sw.listeners.size).toBe(0)
    expect(sw.updateFoundListeners.size).toBe(0)
  })

  // ---- 2026-10-31 修: 判据由"控制器换过"改为"构建号比对" ----
  // 报障场景: 旧页面点橙色「发现更新」⇒ 清缓存+reload(页面已是新版, 但控制器还是旧 SW)
  //          ⇒ 新 SW 随后激活 claim, controllerchange 二次触发 ⇒ 误报蓝色「新版本」。

  it('构建号相同 → 页面已是最新, 不再提示(报障回归)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => OWN })
    await settle()

    await fire(sw)
    await settle()

    expect(apiOf(w).updateReady.value).toBe(false)
    expect(apiOf(w).updateFound.value).toBe(false)
  })

  // ---- 2026-11-09 修: 报障"一个版本提示好几遍, 即使刷新了也一样" ----

  it('同一构建号刷新后不再重复提示(跨页面实例去重)', async () => {
    const deps = { ownBuild: OWN, probeLatestBuild: async () => NEWER }

    const sw1 = installFakeSW({ state: 'activated' })
    const w1 = mountWithDeps(deps)
    await settle()
    await fire(sw1)
    await settle()
    expect(apiOf(w1).updateFound.value).toBe(true)
    w1.unmount()

    // 模拟刷新: 新页面实例, same session
    removeFakeSW()
    const sw2 = installFakeSW({ state: 'activated' })
    const w2 = mountWithDeps(deps)
    await settle()
    await fire(sw2)
    await settle()

    expect(apiOf(w2).updateFound.value).toBe(false)
    expect(apiOf(w2).updateReady.value).toBe(false)
  })

  it('去重必须跨 session: 新标签/TV 冷启动(sessionStorage 为空)也不再提示同一版本', async () => {
    const deps = { ownBuild: OWN, probeLatestBuild: async () => NEWER }

    const sw1 = installFakeSW({ state: 'activated' })
    const w1 = mountWithDeps(deps)
    await settle()
    await fire(sw1)
    await settle()
    expect(apiOf(w1).updateFound.value).toBe(true)
    w1.unmount()

    // 模拟 TV 壳冷启动 / 新开标签: sessionStorage 是全新的, localStorage 仍在
    sessionStorage.clear()
    removeFakeSW()
    const sw2 = installFakeSW({ state: 'activated' })
    const w2 = mountWithDeps(deps)
    await settle()
    await fire(sw2)
    await settle()

    expect(apiOf(w2).updateFound.value).toBe(false)
    expect(localStorage.getItem('jc-pwa-prompted-build')).toBe(NEWER)
  })

  it('去重按构建号而非永久静默: 出现更新的构建号仍会提示一次', async () => {
    const NEWEST = '20261101-090000'
    const sw1 = installFakeSW({ state: 'activated' })
    const w1 = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()
    await fire(sw1)
    await settle()
    expect(apiOf(w1).updateFound.value).toBe(true)
    w1.unmount()

    // 又有更新版本: 页面(OWN)依然落后, 但可用版本变成了 NEWEST ⇒ 必须再提示
    removeFakeSW()
    const sw2 = installFakeSW({ state: 'activated' })
    const w2 = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWEST })
    await settle()
    await fire(sw2)
    await settle()

    expect(apiOf(w2).updateFound.value).toBe(true)
    expect(localStorage.getItem('jc-pwa-prompted-build')).toBe(NEWEST)
  })

  it('updatefound 但构建号相同 → 不亮灯(安装重试不再误报)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => OWN })
    await settle()

    sw.fireUpdateFound()
    await settle()

    expect(apiOf(w).updateFound.value).toBe(false)
  })

  it('updatefound 触发瞬间不亮灯(必须等构建号核对结果, 否则安装重试会闪现误报)', async () => {
    let resolveProbe: (v: string) => void = () => {}
    const probe = vi.fn(
      () =>
        new Promise<string>((r) => {
          resolveProbe = r
        })
    )
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: probe })
    await nextTick()

    // 挂载时的例行核对先放行(同版本), 页面处于"最新"状态
    resolveProbe(OWN)
    await settle()
    expect(apiOf(w).updateFound.value).toBe(false)

    // 新一次 install 开始: 核对还没回来之前, 徽标不能亮
    sw.fireUpdateFound()
    await nextTick()
    expect(apiOf(w).updateFound.value).toBe(false)

    // 核对结果: 仍是同一版本 ⇒ 依然不亮
    resolveProbe(OWN)
    await settle()
    expect(apiOf(w).updateFound.value).toBe(false)
  })

  it('探测失败且只有 controllerchange → 不提示(丢包链路不再随机误报)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => {
        throw new Error('offline')
      }
    })
    await settle()

    await fire(sw)
    await settle()

    expect(apiOf(w).updateReady.value).toBe(false)
    expect(apiOf(w).updateFound.value).toBe(false)
  })

  it('探测失败但有 updatefound 硬证据 → 提示"发现更新"(保守兜底)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => '' })
    await settle()

    sw.fireUpdateFound()
    await settle()

    expect(apiOf(w).updateFound.value).toBe(true)
    // 尚未接管 ⇒ 只给 amber, 不能声称"新版本已就绪"
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('同一实例内 amber 可升级为 ready(同一次提示的进展, 不算重复)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()
    sw.fireUpdateFound()
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)
    expect(apiOf(w).updateReady.value).toBe(false)

    await fire(sw)
    await settle()
    expect(apiOf(w).updateReady.value).toBe(true)
  })

  it('核对进行中又发生接管 → 补跑一次, 最终升级为 ready', async () => {
    let resolveFirstProbe: (v: string) => void = () => {}
    let firstCall = true
    const probe = vi.fn(() => {
      if (firstCall) {
        firstCall = false
        return new Promise<string>((r) => {
          resolveFirstProbe = r
        })
      }
      return Promise.resolve(NEWER)
    })
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: probe })
    await nextTick() // 挂载触发的核对卡在第一次探测上

    await fire(sw) // 接管信号在核对期间到达, 会被记账而不是丢弃
    resolveFirstProbe(NEWER)
    await settle()

    expect(apiOf(w).updateFound.value).toBe(true)
    expect(apiOf(w).updateReady.value).toBe(true)
  })

  it('未注入构建号(dev/旧构建) → 沿用旧语义: 有接管即提示', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: '', probeLatestBuild: async () => NEWER })
    await settle()

    await fire(sw)
    await settle()

    expect(apiOf(w).updateReady.value).toBe(true)
  })

  it('首访(无 controller)不发起构建号探测', async () => {
    const probe = vi.fn(async () => NEWER)
    const sw = installFakeSW(null)
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: probe })
    await settle()

    await fire(sw)
    await settle()

    expect(probe).not.toHaveBeenCalled()
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  // ---- applyUpdate: 确定性切换(2026-11-09) ----

  it('applyUpdate: 已接管时直接刷新', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER, reload })
    await settle()
    await fire(sw)
    await settle()
    expect(apiOf(w).updateReady.value).toBe(true)

    apiOf(w).applyUpdate()
    await settle()
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('applyUpdate: 未接管时等待新 SW 接管后才刷新(不再"清缓存立刻 reload")', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => NEWER,
      applyWaitMs: 30,
      reload
    })
    await settle()
    sw.fireUpdateFound()
    await settle()
    expect(apiOf(w).updateReady.value).toBe(false)

    apiOf(w).applyUpdate()
    expect(apiOf(w).applying.value).toBe(true)
    await settle()
    // 还没接管 ⇒ 不能刷新
    expect(reload).not.toHaveBeenCalled()

    // 新 SW 接管
    await fire(sw)
    await settle()
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('applyUpdate: 等待超时也要给出口(刷新一次, 不重复)', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => NEWER,
      applyWaitMs: 10,
      reload
    })
    await settle()
    sw.fireUpdateFound()
    await settle()

    apiOf(w).applyUpdate()
    apiOf(w).applyUpdate() // 重复点击被 applying 拦住
    await new Promise((r) => setTimeout(r, 60))
    await settle()

    expect(reload).toHaveBeenCalledTimes(1)
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
