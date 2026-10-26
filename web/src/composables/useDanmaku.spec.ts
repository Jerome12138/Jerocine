import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref, shallowRef, nextTick } from 'vue'
import {
  useDanmaku,
  readDanmakuSettings,
  DANMAKU_DEFAULT_SETTINGS,
  DANMAKU_FETCH_TIMEOUTS,
  type DanmuState
} from './useDanmaku'

/**
 * useDanmaku composable 测试 —— 覆盖状态机六态 + 2026-10-25 线上死锁回归。
 *
 * 死锁背景: 模板弹幕层曾 v-if=providers 非空, 而 useDanmaku 又要求 stage 挂载后才建
 * 引擎 ⇒ providers 永远为空、按钮全不显示。修复=模板 stage 常驻(见 PlayView) +
 * composable 先落 providers 再建引擎。回归用例模拟"stage 常驻"的 host, 断言
 * 有源时 providers 必非空、链路必走到 ready。
 *
 * 渲染层已迁移到 danmaku-lite: 引擎经 adapter 自驱进度、dataSource 自驱腾讯分段,
 * 故无 manual time/start/stop/seek 联动, 也无 --jc-danmu-size / window.CommentManager。
 * 这里用假引擎（vi.mock('danmaku-lite')）记录 load/clear/setX 调用做断言。
 */

vi.mock('@/api', () => ({
  filmApi: {
    getDanmuSource: vi.fn(),
    getDanmuBilibili: vi.fn(),
    getDanmuTencentEpisodes: vi.fn(),
    getDanmuTencentSegments: vi.fn(),
    getDanmuTencentComments: vi.fn()
  }
}))

import { filmApi } from '@/api'

const m = filmApi as unknown as {
  getDanmuSource: ReturnType<typeof vi.fn>
  getDanmuBilibili: ReturnType<typeof vi.fn>
  getDanmuTencentEpisodes: ReturnType<typeof vi.fn>
  getDanmuTencentSegments: ReturnType<typeof vi.fn>
  getDanmuTencentComments: ReturnType<typeof vi.fn>
}

/** 假 danmaku-lite 引擎：记录装载/设置动作，供断言。引擎自驱，这里只 record。 */
const fakeEngine = {
  load: vi.fn(),
  clear: vi.fn(),
  destroy: vi.fn(),
  resize: vi.fn(),
  setArea: vi.fn(),
  setOpacity: vi.fn(),
  setSpeed: vi.fn(),
  setMaxVisible: vi.fn(),
  setFontSize: vi.fn(),
  setStrokeWidth: vi.fn(),
  setUseTextShadow: vi.fn(),
  setEnabled: vi.fn(),
  isDestroyed: false
}

vi.mock('danmaku-lite', () => ({
  createEngine: vi.fn(() => fakeEngine)
}))

const flush = async (): Promise<void> => {
  // loadDanmaku 内部串行 await 多个接口; 一个宏任务 tick 足以排空全部微任务
  await new Promise((r) => setTimeout(r, 0))
  await nextTick()
}

const biliSrc = { providers: [{ provider: 'bilibili', resourceId: '44847' }] }
const tencentSrc = { providers: [{ provider: 'tencent', resourceId: 'mzc00200miw8m3y' }] }
const comments = [
  // 非整秒时间: 整秒会被 scatterTimes 随机打散(±0.5s), 精确断言需避开
  { text: '第一条', time: 1250, color: 16777215, mode: 1 },
  { text: '第二条', time: 2750, color: 16777215, mode: 1 }
]
const segments = [
  { provider: 'tencent', startTime: 0, segmentId: 'seg-0' },
  { provider: 'tencent', startTime: 300, segmentId: 'seg-300' }
]

/** 腾讯全链路 mock: episodes → segments → comments(每段 same comments) */
function mockTencentChain(opts: { matched?: boolean; commentsOk?: boolean } = {}): void {
  m.getDanmuTencentEpisodes.mockResolvedValue(
    opts.matched === false
      ? { ok: true, data: { matched: null } }
      : { ok: true, data: { matched: { episodeId: 'ep-1' } } }
  )
  m.getDanmuTencentSegments.mockResolvedValue({ ok: true, data: segments })
  m.getDanmuTencentComments.mockImplementation(() =>
    opts.commentsOk === false ? Promise.resolve(null) : Promise.resolve({ ok: true, data: comments })
  )
}

/** 永不 settle 的请求桩：直到 signal 中止才 reject（用于超时测试）。 */
function neverUntilAbort(signal?: AbortSignal): Promise<never> {
  return new Promise((_, reject) => {
    if (!signal) return
    signal.addEventListener('abort', () =>
      reject(Object.assign(new Error('aborted'), { name: 'CanceledError', code: 'ERR_CANCELED' }))
    )
  })
}

function mountHost(mid: number | undefined, enabled = true, attachStage = true) {
  const midRef = ref(mid)
  const enabledRef = ref(enabled)
  const playingRef = ref(true)
  const Host = defineComponent({
    setup() {
      const api = useDanmaku({
        mid: midRef,
        episode: ref(1),
        player: shallowRef(null),
        playing: playingRef,
        enabled: enabledRef,
        // 渲染自驱，onPlayerEvent 仅保留兼容签名，不再订阅
        onPlayerEvent: () => () => undefined
      })
      return { api }
    },
    render() {
      // 与 PlayView 模板同款: stage 常驻(修复后姿势), ref 直接绑 api.stage。
      // attachStage=false 模拟回退到 v-if=providers 非空 的旧姿势: stage 不在 DOM。
      return attachStage ? h('div', { ref: this.api.stage }) : h('div')
    }
  })
  const w = mount(Host)
  const api = (w.vm as unknown as { api: ReturnType<typeof useDanmaku> }).api
  return { w, api, midRef, enabledRef, playingRef }
}

describe('useDanmaku', () => {
  beforeEach(() => {
    localStorage.clear()
    // 复位请求超时（超时测试会改小），避免串扰
    DANMAKU_FETCH_TIMEOUTS.source = 15_000
    DANMAKU_FETCH_TIMEOUTS.segment = 15_000
    DANMAKU_FETCH_TIMEOUTS.bilibili = 30_000
    // jsdom 无 ResizeObserver，提供桩避免 ensureEngine 抛错（被 try/catch 捕获会误判 failed）
    ;(globalThis as unknown as { ResizeObserver?: unknown }).ResizeObserver = class {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    }
    for (const fn of Object.values(fakeEngine)) {
      if (typeof fn === 'function' && 'mockClear' in (fn as unknown as Record<string, unknown>)) {
        ;(fn as unknown as { mockClear: () => void }).mockClear()
      }
    }
    fakeEngine.isDestroyed = false
    m.getDanmuSource.mockReset()
    m.getDanmuBilibili.mockReset()
    m.getDanmuTencentEpisodes.mockReset()
    m.getDanmuTencentSegments.mockReset()
    m.getDanmuTencentComments.mockReset()
  })
  afterEach(() => {
    delete (globalThis as unknown as { ResizeObserver?: unknown }).ResizeObserver
  })

  it('回归·死锁: stage 常驻 + 有源 ⇒ providers 非空且走到 ready（旧实现此处 providers 恒空）', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain()
    const { api } = mountHost(4)
    await flush()
    expect(api.providers.value).toHaveLength(1)
    expect(api.providers.value[0].provider).toBe('tencent')
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.hasDanmaku.value).toBe(true)
    // 腾讯走 dataSource 自驱流式，不直接 engine.load（content 由引擎按进度 fetch）
    expect(fakeEngine.load).not.toHaveBeenCalled()
  })

  it('回归·死锁顺序: stage 未挂载(模板回退 v-if 的旧姿势) ⇒ providers 仍先落库, UI 有按钮, 引擎未建 → failed', async () => {
    // 旧实现先 ensureManager 后取源: stage null ⇒ 引擎建不起 ⇒ 提前 return ⇒
    // providers 恒空、按钮全不显示(2026-10-25 线上事故)。修复后顺序相反, 本用例对顺序敏感。
    // mid 从无到有模拟首播(detail 加载后赋值), 触发 watch → loadDanmaku。
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, midRef } = mountHost(undefined, true, false)
    midRef.value = 3
    await flush()
    expect(api.providers.value).toHaveLength(1)
    expect(api.danmuState.value).toBe<DanmuState>('failed') // 容器缺失/引擎未建
    // 内容仍会被后台预拉（enabled+playing），只是 stage 缺失引擎未建 → 不渲染；开启后命中缓存瞬时显示
    expect(m.getDanmuBilibili).toHaveBeenCalled()
  })

  it('无候选源（平台无此片）⇒ unsupported, providers 空, 不建引擎', async () => {
    m.getDanmuSource.mockResolvedValue({ providers: [] })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(api.providers.value).toEqual([])
  })

  it('source 接口失败 ⇒ unsupported（用户要求查不到也要有显示, 不许静默）', async () => {
    m.getDanmuSource.mockRejectedValue(new Error('500'))
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(api.providers.value).toEqual([])
  })

  it('B站路径: 拉到弹幕 ⇒ ready 且 engine.load 收到 DanmakuItem[]', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.hasDanmaku.value).toBe(true)
    expect(fakeEngine.load).toHaveBeenCalledTimes(1)
    const items = fakeEngine.load.mock.calls[0]![0] as Array<{ id: unknown; time: number; mode: number; color: number }>
    expect(items).toHaveLength(2)
    expect(items[0]!.time).toBe(1.25) // 毫秒 1250 → 秒（非整秒, 不被打散）
    expect(items[0]!.mode).toBe(1)
    expect(items[0]!.color).toBe(16777215)
    expect(typeof items[0]!.id).toBe('string') // 稳定唯一 id
  })

  it('回归·关闭弹幕后再打开应重新装载并显示（关→开不失效）', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, false) // 初始关闭
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(fakeEngine.load).not.toHaveBeenCalled() // 关闭态不装载
    enabledRef.value = true // 首次打开
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 装载一次
    // 同集关→开: 即时恢复显示, 不重新拉取装载(免 clear+重拉的"等好久")
    enabledRef.value = false
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(fakeEngine.setEnabled).toHaveBeenLastCalledWith(false) // 仅停绘, 内容保留
    expect(fakeEngine.clear).not.toHaveBeenCalled() // 关闭不清屏
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 仍只有初始一次装载
    expect(fakeEngine.setEnabled).toHaveBeenLastCalledWith(true) // 恢复绘制
  })

  it('回归·播放/暂停切换不触发重载（暂停弹幕不消失）', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, playingRef } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    const clearsAfterLoad = fakeEngine.clear.mock.calls.length
    playingRef.value = false // 暂停
    await flush()
    playingRef.value = true // 恢复
    await flush()
    expect(fakeEngine.clear.mock.calls.length).toBe(clearsAfterLoad) // 未清屏
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 未重载
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('回归·底部/顶部/彩色切换: B站内存重映射, 零请求零重载即时生效', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    const loadsBefore = fakeEngine.load.mock.calls.length
    const clearsBefore = fakeEngine.clear.mock.calls.length
    api.updateSettings({ bottomMode: false })
    await flush()
    expect(fakeEngine.load.mock.calls.length).toBe(loadsBefore + 1) // 仅内存重映射一次
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1) // 零网络请求
    expect(fakeEngine.clear.mock.calls.length).toBe(clearsBefore) // 不走 loadDanmaku(无 clear)
    expect(api.danmuState.value).toBe<DanmuState>('ready') // 状态机未被重置
  })

  it('B站路径: comments 空 ⇒ empty（有源无内容）', async () => {  
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments: [] })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('empty')
    expect(api.hasDanmaku.value).toBe(false)
    expect(api.providers.value).toHaveLength(1) // providers 仍可见(UI 有按钮)
  })

  it('B站路径: 请求失败 ⇒ failed（可重试）', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockRejectedValue(new Error('timeout'))
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
  })

  it('B站整集请求超时(整集30s/分片15s；此处注入极小超时) ⇒ failed 可重试', async () => {
    DANMAKU_FETCH_TIMEOUTS.bilibili = 30 // ms，加速测试
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockImplementation((_p, opts) => neverUntilAbort(opts?.signal))
    const { api } = mountHost(3) // enabled+playing → 触发预拉与装载
    await new Promise((r) => setTimeout(r, 80)) // 越过 30ms 超时
    await nextTick()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    DANMAKU_FETCH_TIMEOUTS.bilibili = 30_000
  })

  it('源列表请求超时 ⇒ unsupported（暂无候选, 不阻塞播放）', async () => {
    DANMAKU_FETCH_TIMEOUTS.source = 30
    m.getDanmuSource.mockImplementation((_mid, opts) => neverUntilAbort(opts?.signal))
    const { api } = mountHost(3)
    await new Promise((r) => setTimeout(r, 80))
    await nextTick()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    DANMAKU_FETCH_TIMEOUTS.source = 15_000
  })

  it('prefetchContent: 播放中后台预热内容, 切集后新集也预热(并发去重不重复发)', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { midRef } = mountHost(3) // enabled+playing → 挂载即预拉
    await flush()
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1) // 预拉与装载并发去重
    midRef.value = 4
    await flush()
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(2) // 新集预热
  })

  it('腾讯路径: 平台无匹配集 ⇒ empty', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain({ matched: false })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('empty')
  })

  it('腾讯路径: comments 请求失败 ⇒ 仍 ready（流式自驱, 单段失败为瞬时, 引擎重试而非整体 failed）', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain({ commentsOk: false })
    const { api } = mountHost(4)
    await flush()
    // meta + segments 解析成功即标 ready（引擎按进度自驱流式预拉）；单段 comments 拉取失败
    // 视为瞬时，由引擎后续重试，不把整条链路判 failed（区别于 matched=null 的 empty）。
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.providers.value).toHaveLength(1)
  })

  it('开关关闭但播放中 ⇒ idle（不显示），但后台已预拉源与内容（首开即命中缓存）', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, false)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle') // 关着，不显示
    expect(m.getDanmuSource).toHaveBeenCalled() // 源列表后台预热
    expect(m.getDanmuBilibili).toHaveBeenCalled() // 内容后台预热（播放中）
    // 开启即命中缓存，零额外请求
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1) // 仅预热那一次
  })

  it('开关从关到开 ⇒ 触发装载（UI 打开即有弹幕或三态提示）', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, false)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('引擎未创建(stage 缺失) ⇒ providers 仍落库(按钮可显示) + failed 态', async () => {
    // watch 默认非 immediate，需先以 undefined 挂载、再赋 mid 触发 loadDanmaku；
    // 此时 stage 未挂载（attachStage=false）⇒ ensureEngine 跳过 ⇒ 引擎未建。
    m.getDanmuSource.mockResolvedValue(biliSrc)
    const { api, midRef } = mountHost(undefined, true, false)
    midRef.value = 3
    await flush()
    expect(api.providers.value).toHaveLength(1)
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    expect(m.getDanmuBilibili).toHaveBeenCalled() // 后台预热（即使 stage 缺失也先暖缓存，引擎未建则仅不渲染）
  })

  it('切集(mid 变化) ⇒ 重新查询, 旧 token 结果不落地', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, midRef } = mountHost(3)
    await flush()
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    midRef.value = 4
    await flush()
    expect(m.getDanmuSource).toHaveBeenCalledTimes(2)
    expect(m.getDanmuSource).toHaveBeenLastCalledWith(4, expect.anything())
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('弹幕设置: 默认值运行时应用到引擎 + 持久化', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    // 默认(2026-10-26): 自适应字号、透明度 0.8、区域半屏(0.5)、速度 2(→引擎 1/2)、
    // 同屏上限 30、描边开(1.25 + useTextShadow)。
    expect(fakeEngine.setOpacity).toHaveBeenCalledWith(0.8)
    expect(fakeEngine.setArea).toHaveBeenCalledWith(0.5)
    expect(fakeEngine.setSpeed).toHaveBeenCalledWith(1.4) // danmakuSpeedOf(1) = 1.4/1（系数整体提一档）
    expect(fakeEngine.setMaxVisible).toHaveBeenCalledWith(30)
    expect(fakeEngine.setStrokeWidth).toHaveBeenCalledWith(1.25)
    expect(fakeEngine.setUseTextShadow).toHaveBeenCalledWith(true)
    expect(fakeEngine.setFontSize).toHaveBeenCalledWith(expect.any(Number))
  })

  it('弹幕设置: 速度变更 ⇒ setSpeed 运行时生效, 不重载(load 次数不变)', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(fakeEngine.load).toHaveBeenCalledTimes(1)
    api.updateSettings({ speed: 3 }) // 运行时 setSpeed(1.4/3), 不触发 loadDanmaku 重载
    await flush()
    expect(fakeEngine.setSpeed).toHaveBeenCalledWith(1.4 / 3)
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 未重载
  })

  it('弹幕设置: 底部/顶部/彩色变更 ⇒ B站内存重映射(零请求, 不重置状态机)', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(fakeEngine.load).toHaveBeenCalledTimes(1)
    api.updateSettings({ bottomMode: false, colored: false, outline: false })
    await flush()
    // 零 HTTP；engine.load 再次调用（内存重映射装载过滤后的弹幕）
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    expect(fakeEngine.load).toHaveBeenCalledTimes(2)
    expect(fakeEngine.setUseTextShadow).toHaveBeenCalledWith(false) // outline 关(混合 patch 仍应用运行时设置)
    // 持久化
    expect(localStorage.getItem('jc-danmaku-settings')).toContain('"bottomMode":false')
  })

  it('请求去重: 同一集开关关→开命中缓存, 不重复发请求', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, true)
    await flush()
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    // 关 → 不发任何请求
    enabledRef.value = false
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    // 再开 → 同集已装载, 即时恢复显示(零请求、零重载)
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 恢复路径, 不重新装载
    expect(fakeEngine.setEnabled).toHaveBeenLastCalledWith(true)
  })

  it('请求去重: 切集(mid 变化)缓存 key 自然失效, 照常重新请求新集', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, midRef } = mountHost(3)
    await flush()
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    midRef.value = 4
    await flush()
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(2) // 不同 mid, miss 重新拉
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    // 切回已看过的 3 ⇒ 命中缓存（biliCache.mid===3 仍保留）零请求
    midRef.value = 3
    await flush()
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(2)
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('readDanmakuSettings: localStorage 损坏/越界回退默认并钳制', async () => {
    localStorage.setItem('jc-danmaku-settings', '{broken json')
    expect(readDanmakuSettings()).toEqual({ ...DANMAKU_DEFAULT_SETTINGS })
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ fontSize: 999, opacity: 9, speed: 0.3, limit: -5 }))
    const s = readDanmakuSettings()
    expect(s.fontSize).toBe(64) // 钳制上限
    expect(s.opacity).toBe(1) // 钳制上限
    expect(s.speed).toBe(1) // 非法档位回新默认
    expect(s.limit).toBe(30) // 负数回默认(同屏上限 30)
  })

  it('readDanmakuSettings: 旧速度档(0.5..1.5) ×2 迁移到新档(整体慢一倍)', () => {
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ speed: 1, opacity: 0.8, area: 0.5 }))
    expect(readDanmakuSettings().speed).toBe(2)
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ speed: 0.5, opacity: 0.8, area: 0.5 }))
    expect(readDanmakuSettings().speed).toBe(1)
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ speed: 1.5, opacity: 0.8, area: 0.5 }))
    expect(readDanmakuSettings().speed).toBe(3)
    // 新档值原样保留
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ speed: 2.5, opacity: 0.8, area: 0.5 }))
    expect(readDanmakuSettings().speed).toBe(2.5)
  })

  it('readDanmakuSettings: 无记录返回新默认(opacity 0.8 / area 0.5 / speed 1)', () => {
    const s = readDanmakuSettings()
    expect(s.opacity).toBe(0.8)
    expect(s.area).toBe(0.5)
    expect(s.speed).toBe(1)
  })
})
