import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import {
  usePwaUpdate,
  isPageStale,
  parseBuildTag,
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
  /**
   * 触发 registration.updatefound.
   * @param withInstalling 是否顺便挂上 `reg.installing`(默认 true —— 真实的 updatefound
   *   就是"新 worker 开始装", 不挂的话页面侧会把它当成"控制器已最新")
   * @returns 那个 installing worker(可用于驱动 statechange)
   */
  fireUpdateFound: (withInstalling?: boolean) => FakeWorker | null
  /** registration.update 的探针(每次核对都应让 SW 去比对 sw.js) */
  update: ReturnType<typeof vi.fn>
  /** 让 `navigator.serviceWorker.ready` resolve —— 模拟"首访注册完成、已激活" */
  resolveReady: () => void
  /** 直接设置 reg.installing(模拟"刷新后仍在装") */
  setInstalling: (w: FakeWorker | null) => void
}

/** 假的 installing worker —— 只有 statechange 与 state 有用 */
interface FakeWorker {
  state: ServiceWorkerState
  setState: (s: ServiceWorkerState) => void
}

function makeFakeWorker(initial: ServiceWorkerState = 'installing'): FakeWorker {
  const listeners = new Set<() => void>()
  const w = {
    state: initial,
    addEventListener: (t: string, cb: () => void) => {
      if (t === 'statechange') listeners.add(cb)
    },
    removeEventListener: () => {},
    setState: (s: ServiceWorkerState) => {
      w.state = s
      for (const cb of listeners) cb()
    }
  }
  return w
}

/**
 * 装一个假 navigator.serviceWorker.
 * @param controller 进入页面时是否已被 SW 控制(null = 首访)
 * @param opts.withRegistration 是否让 getRegistration() 返回一个注册对象(默认返回);
 *   传 false 模拟"首访: 此刻还没有任何注册, 要等注册脚本跑完"
 * @param opts.installing 初始是否已有 worker 在装(模拟"刷新时下载还没完")
 */
function installFakeSW(
  controller: unknown,
  opts: { withRegistration?: boolean; installing?: FakeWorker | null } = {}
): FakeSW {
  const listeners = new Set<() => void>()
  const updateFoundListeners = new Set<() => void>()
  const update = vi.fn(async () => undefined)
  const reg = {
    update,
    installing: (opts.installing ?? null) as FakeWorker | null,
    waiting: null as { postMessage: (m: unknown) => void } | null,
    addEventListener: (t: string, cb: () => void) => {
      if (t === 'updatefound') updateFoundListeners.add(cb)
    },
    removeEventListener: (t: string, cb: () => void) => {
      if (t === 'updatefound') updateFoundListeners.delete(cb)
    }
  }
  // ready 默认永不 resolve(绝大多数用例不关心首访注册链路); 需要的用例显式 resolveReady()
  let releaseReady: ((r: unknown) => void) | null = null
  const ready = new Promise((res) => {
    releaseReady = res as (r: unknown) => void
  })
  const fake = {
    controller,
    addEventListener: (_t: string, cb: () => void) => {
      listeners.add(cb)
    },
    removeEventListener: (_t: string, cb: () => void) => {
      listeners.delete(cb)
    },
    getRegistration: async () => (opts.withRegistration === false ? undefined : reg),
    ready
  }
  Object.defineProperty(navigator, 'serviceWorker', { value: fake, configurable: true })
  return {
    listeners,
    updateFoundListeners,
    fireUpdateFound: (withInstalling = true) => {
      let w: FakeWorker | null = null
      if (withInstalling) {
        w = makeFakeWorker()
        reg.installing = w
      }
      for (const cb of updateFoundListeners) cb()
      return w
    },
    update,
    resolveReady: () => releaseReady?.(reg),
    setInstalling: (w) => {
      reg.installing = w
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
    localStorage.clear()
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

  // ---- 2026-11-10 改: 去掉"同一构建号只提示一次"的去重 ----
  // 旧实现用 localStorage 记住"这个构建号提示过", 代价是"刷新后仍落后就再也不给入口"
  // (用户报障: 点了更新、页面没变、入口也没了)。现在"要不要显示"完全由真实状态决定 ——
  // 页面确实旧就亮灯, 亮灯时要么"能点"(ready)、要么"在装"(不可点), 都不是骚扰;
  // 升级成功(构建号相同)自然就不亮了 —— 闭环靠构建号, 不靠记忆。

  it('刷新后页面已是新版(构建号相同) → 不提示, 闭环自洽', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: NEWER, probeLatestBuild: async () => NEWER })
    await settle()
    await fire(sw)
    await settle()

    expect(apiOf(w).updateFound.value).toBe(false)
    expect(apiOf(w).updateReady.value).toBe(false)
  })

  it('刷新后仍落后 → 照常给入口(刷新一定拿得到新版, 不该因为"提示过"而静默)', async () => {
    const deps = { ownBuild: OWN, probeLatestBuild: async () => NEWER }
    const sw1 = installFakeSW({ state: 'activated' })
    const w1 = mountWithDeps(deps)
    await settle()
    await fire(sw1)
    await settle()
    expect(apiOf(w1).updateReady.value).toBe(true)
    w1.unmount()

    // 模拟刷新: 新页面实例, 页面仍是旧构建 ⇒ 入口必须还在
    removeFakeSW()
    installFakeSW({ state: 'activated' })
    const w2 = mountWithDeps(deps)
    await settle()

    expect(apiOf(w2).updateFound.value).toBe(true)
    expect(apiOf(w2).updateReady.value).toBe(true)
    w2.unmount()
  })

  it('出现更新的构建号 → 照样提示(构建号变了就是真更新)', async () => {
    const NEWEST = '20261101-090000'
    const sw1 = installFakeSW({ state: 'activated' })
    const w1 = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()
    await fire(sw1)
    await settle()
    expect(apiOf(w1).updateFound.value).toBe(true)
    w1.unmount()

    removeFakeSW()
    const sw2 = installFakeSW({ state: 'activated' })
    const w2 = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWEST })
    await settle()
    await fire(sw2)
    await settle()

    expect(apiOf(w2).updateFound.value).toBe(true)
    w2.unmount()
    void sw2
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

  it('首访: 初次接管只翻"已被控制", 之后再来的 controllerchange 按构建号正常判定', async () => {
    const probe = vi.fn(async () => NEWER)
    const sw = installFakeSW(null)
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: probe })
    await settle()

    // 第一次 controllerchange = clientsClaim 的初次接管: 页面就是刚下载的, 不该提示
    await fire(sw)
    await settle()
    expect(probe).not.toHaveBeenCalled()
    expect(apiOf(w).updateFound.value).toBe(false)

    // 这是"真的换了新 SW" —— 首访会话也必须具备判定资格(原来钉死常量会整条会话静默)
    await fire(sw)
    await settle()
    expect(probe).toHaveBeenCalledTimes(1)
    expect(apiOf(w).updateReady.value).toBe(true)
  })

  it('首访: 注册在 load 之后才完成 ⇒ ready 解决时补挂 updatefound, 之后能正常收到信号', async () => {
    // withRegistration: false = mount 时还没有任何注册(首访 / 被硬重置后)
    const sw = installFakeSW(null, { withRegistration: false })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()

    expect(sw.updateFoundListeners.size).toBe(0) // 还没注册, 挂不上

    await fire(sw) // 初次接管
    await settle()
    expect(sw.updateFoundListeners.size).toBe(0) // 注册仍未完成

    sw.resolveReady() // 注册脚本(load 后)跑完 → install → activate
    await settle()
    expect(sw.updateFoundListeners.size).toBe(1)

    sw.fireUpdateFound()
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)
  })

  it('已在控制中时不再等 ready(不重复挂同一个 updatefound)', async () => {
    const sw = installFakeSW({ state: 'activated' })
    mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()
    expect(sw.updateFoundListeners.size).toBe(1)

    sw.resolveReady() // 即使 ready 也 resolve, 也不该挂第二个
    await settle()
    expect(sw.updateFoundListeners.size).toBe(1)
  })

  // ---- applyUpdate: 只有"刷新一定拿到新版"才生效(2026-11-10 铁律) ----

  it('applyUpdate: 就绪(新 worker 已接管)时点一下就刷新', async () => {
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

  it('applyUpdate: 下载中不可点 —— 连 reload 都不该发生(刷了也是旧页面)', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER, reload })
    await settle()
    sw.fireUpdateFound() // 新 worker 开始装
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)
    expect(apiOf(w).updateReady.value).toBe(false)

    apiOf(w).applyUpdate()
    await settle()
    expect(reload).not.toHaveBeenCalled()
    expect(apiOf(w).applying.value).toBe(false) // 压根没进入"切换中"
  })

  it('applyUpdate: 重复点击只刷新一次(applying 拦住)', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER, reload })
    await settle()
    await fire(sw) // 接管 ⇒ ready
    await settle()

    apiOf(w).applyUpdate()
    apiOf(w).applyUpdate()
    apiOf(w).applyUpdate()
    await settle()

    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('控制器已是最新(update() 后没有新 worker) → 直接判定可就绪, 点了就刷', async () => {
    const reload = vi.fn()
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER, reload })
    await settle()
    // 没有任何 controllerchange / updatefound: 只是"页面旧了, SW 已是最新"
    expect(apiOf(w).updateReady.value).toBe(true)
    expect(sw.update).toHaveBeenCalled()

    apiOf(w).applyUpdate()
    await settle()
    expect(reload).toHaveBeenCalledTimes(1)
  })

  // ---- 徽标亮起 ⇒ 让 SW 真的开始装 + 进度水位 ----

  it('页面落后但还没开始装 → 核对时让 SW update() 一次, 装起来就转"下载中"', async () => {
    const sw = installFakeSW({ state: 'activated' })
    // 真实浏览器: update() 发现 sw.js 变了才会创建 installing
    sw.update.mockImplementation(async () => {
      sw.setInstalling(makeFakeWorker())
    })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()

    expect(sw.update).toHaveBeenCalled()
    expect(apiOf(w).updateFound.value).toBe(true)
    expect(apiOf(w).updateReady.value).toBe(false) // 在装 ⇒ 不可点
    w.unmount()
  })

  it('进度按 assets/<新构建号>/ 的落地条目数推进, 装完(installed)到 0.99, 接管才满', async () => {
    // 旧版 10 条(在 OWN 目录下), 新版也是 10 条但目录换成 NEWER
    const keys = Array.from({ length: 10 }, (_, i) => `https://x/assets/${OWN}/old${i}.js?__WB_REVISION__=a`)
    const list = vi.fn(async () => keys.slice())
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => NEWER,
      listPrecacheKeys: list,
      progressPollMs: 10
    })
    await settle()
    const worker = sw.fireUpdateFound()!
    await settle()
    // 亮起就有一格可见进度, 不是 0
    expect(apiOf(w).progress.value).toBeGreaterThan(0)

    // 新版本落地一半: 命中 NEWER 目录的 5 条
    for (let i = 0; i < 5; i++) keys.push(`https://x/assets/${NEWER}/new${i}.js?__WB_REVISION__=b`)
    await new Promise((r) => setTimeout(r, 60))
    await settle()
    expect(apiOf(w).progress.value).toBeGreaterThanOrEqual(0.5)
    expect(apiOf(w).progress.value).toBeLessThan(1)

    // 浏览器说装完了 ⇒ 水位顶到 0.99(满格仍然只留给"可点")
    worker.setState('installed')
    await settle()
    expect(apiOf(w).progress.value).toBe(0.99)
    expect(apiOf(w).updateReady.value).toBe(false)

    await fire(sw) // 新 SW 接管 = 可刷新
    await settle()
    expect(apiOf(w).progress.value).toBe(1)
    w.unmount()
  })

  it('刷新后在途安装仍在 → 立刻接着显示进度(不用等构建号比对)', async () => {
    const keys = Array.from({ length: 10 }, (_, i) => `https://x/assets/${OWN}/old${i}.js?__WB_REVISION__=a`)
    for (let i = 0; i < 4; i++) keys.push(`https://x/assets/${NEWER}/part${i}.js?__WB_REVISION__=b`)
    const list = vi.fn(async () => keys.slice())
    const sw = installFakeSW({ state: 'activated' }, { installing: makeFakeWorker() })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => NEWER,
      listPrecacheKeys: list,
      progressPollMs: 10
    })

    // mount 即亮灯, 且进度直接落在"已下 4 条"的位置, 不是从 0 重来
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)
    expect(apiOf(w).updateReady.value).toBe(false)
    expect(apiOf(w).progress.value).toBeGreaterThanOrEqual(0.3)

    // 构建号比对也认可(页面确实旧) ⇒ 灯留着
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)
    w.unmount()
    void sw
  })

  it('有在途安装但页面已是最新 → 比对后收灯, 不误报', async () => {
    const sw = installFakeSW({ state: 'activated' }, { installing: makeFakeWorker() })
    const w = mountWithDeps({
      ownBuild: OWN,
      probeLatestBuild: async () => OWN, // 页面就是最新
      listPrecacheKeys: async () => []
    })
    await settle()
    expect(apiOf(w).updateFound.value).toBe(false)
    expect(apiOf(w).updateReady.value).toBe(false)
    w.unmount()
    void sw
  })

  it('安装失败(redundant) → 收灯, 不留下永远不动的图标', async () => {
    const sw = installFakeSW({ state: 'activated' })
    const w = mountWithDeps({ ownBuild: OWN, probeLatestBuild: async () => NEWER })
    await settle()
    const worker = sw.fireUpdateFound()!
    await settle()
    expect(apiOf(w).updateFound.value).toBe(true)

    worker.setState('redundant')
    await settle()
    expect(apiOf(w).updateFound.value).toBe(false)
    expect(apiOf(w).updateReady.value).toBe(false)
    w.unmount()
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
