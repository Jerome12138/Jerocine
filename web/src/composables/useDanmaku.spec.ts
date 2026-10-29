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
import { ApiError } from '@/types/api'

/**
 * useDanmaku composable 测试 —— 聚合单源数据流（标题+集号 → /danmu/aggregate）
 * 状态机覆盖 + 关→开恢复 + 过滤重映射 + 超时。
 *
 * 渲染层为 danmaku-lite：引擎经 adapter 自驱进度，这里用假引擎
 * （vi.mock('danmaku-lite/canvas')）记录 load/clear/setX 调用做断言。
 */

vi.mock('@/api', () => ({
  filmApi: {
    getDanmuAggregate: vi.fn()
  }
}))

import { filmApi } from '@/api'

const m = filmApi as unknown as {
  getDanmuAggregate: ReturnType<typeof vi.fn>
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

vi.mock('danmaku-lite/canvas', () => ({
  createEngine: vi.fn(() => fakeEngine)
}))

const flush = async (): Promise<void> => {
  // loadDanmaku 内部 await 接口; 一个宏任务 tick 足以排空全部微任务
  await new Promise((r) => setTimeout(r, 0))
  await nextTick()
}

const comments = [
  // 非整秒时间: 整秒会被 scatterTimes 随机打散(±0.5s), 精确断言需避开
  { text: '第一条', time: 1250, color: 16777215, mode: 1 },
  { text: '第二条', time: 2750, color: 16777215, mode: 1 },
  { text: '底部条', time: 3400, color: 16711680, mode: 4 }
]

function mountHost(mid: number | undefined, enabled = true, attachStage = true) {
  const midRef = ref(mid)
  const enabledRef = ref(enabled)
  const playingRef = ref(true)
  const titleRef = ref('庆余年 第二季')
  const yearRef = ref<number | undefined>(2024)
  const Host = defineComponent({
    setup() {
      const api = useDanmaku({
        mid: midRef,
        title: titleRef,
        year: yearRef,
        episode: ref(1),
        player: shallowRef(null),
        playing: playingRef,
        enabled: enabledRef
      })
      return { api }
    },
    render() {
      // 与 PlayView 模板同款: stage 常驻, ref 直接绑 api.stage。
      // attachStage=false 模拟 stage 不在 DOM（引擎建不起 → failed）。
      return attachStage ? h('div', { ref: this.api.stage }) : h('div')
    }
  })
  const w = mount(Host)
  const api = (w.vm as unknown as { api: ReturnType<typeof useDanmaku> }).api
  return { w, api, midRef, enabledRef, titleRef }
}

describe('useDanmaku', () => {
  beforeEach(() => {
    localStorage.clear()
    // 复位请求超时（超时测试会改小），避免串扰
    DANMAKU_FETCH_TIMEOUTS.aggregate = 30_000
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
    m.getDanmuAggregate.mockReset()
  })
  afterEach(() => {
    delete (globalThis as unknown as { ResizeObserver?: unknown }).ResizeObserver
  })

  it('正常链路: 聚合接口返回弹幕 ⇒ ready 且 engine.load 收到 DanmakuItem[]', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.hasDanmaku.value).toBe(true)
    expect(fakeEngine.load).toHaveBeenCalledTimes(1)
    expect(m.getDanmuAggregate).toHaveBeenCalledWith(
      { title: '庆余年 第二季', episode: 1, year: 2024 },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    const items = fakeEngine.load.mock.calls[0]![0] as Array<{ id: unknown; time: number; mode: number; color: number }>
    expect(items).toHaveLength(3)
    expect(items[0]!.time).toBe(1.25) // 毫秒 1250 → 秒（非整秒, 不被打散）
    expect(items[0]!.mode).toBe(1)
    // 源 mode 4 → danmaku-lite 6（底部）
    expect(items[2]!.mode).toBe(6)
    expect(typeof items[0]!.id).toBe('string') // 稳定唯一 id
  })

  it('404（查无作品/该集）⇒ unsupported「该片暂无弹幕」', async () => {
    m.getDanmuAggregate.mockRejectedValue(new ApiError(404))
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(fakeEngine.load).not.toHaveBeenCalled()
  })

  it('接口空列表 ⇒ empty「未找到弹幕」', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments: [] })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('empty')
    expect(api.hasDanmaku.value).toBe(false)
  })

  it('接口失败（网络/超时）⇒ failed 可重试', async () => {
    m.getDanmuAggregate.mockRejectedValue(new ApiError(500))
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    // reload 重试后恢复
    m.getDanmuAggregate.mockResolvedValue({ comments })
    api.reload()
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('无影片名 ⇒ unsupported（聚合按标题匹配, 无标题无从匹配）', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, titleRef } = mountHost(3)
    titleRef.value = ''
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(m.getDanmuAggregate).not.toHaveBeenCalled()
  })

  it('回归·关闭弹幕后再打开应重新装载并显示（关→开不失效）', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
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
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 未重载
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(1) // 命中内存缓存
  })

  it('底部/顶部/彩色过滤: 内存重映射, 不再发请求', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    fakeEngine.load.mockClear()
    api.updateSettings({ bottomMode: false })
    await flush()
    expect(fakeEngine.load).toHaveBeenCalledTimes(1) // 内存重映射
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(1) // 零请求
    const items = fakeEngine.load.mock.calls[0]![0] as Array<{ mode: number }>
    expect(items.some((it) => it.mode === 6)).toBe(false) // 底部(6)被过滤
  })

  it('请求超时 ⇒ failed（withTimeout 中止）', async () => {
    DANMAKU_FETCH_TIMEOUTS.aggregate = 20
    m.getDanmuAggregate.mockImplementation(
      (_params: unknown, options?: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          options?.signal?.addEventListener('abort', () =>
            reject(Object.assign(new Error('aborted'), { name: 'CanceledError', code: 'ERR_CANCELED' }))
          )
        })
    )
    const { api } = mountHost(4)
    await flush()
    // 等 20ms 超时定时器真正触发后再断言
    await new Promise((r) => setTimeout(r, 60))
    await nextTick()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
  })

  it('设置读写: readDanmakuSettings 缺省回默认 + 越界钳制 + 旧速度档迁移', () => {
    expect(readDanmakuSettings()).toEqual(DANMAKU_DEFAULT_SETTINGS)
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ opacity: 5, fontSize: 999, speed: 0.75 }))
    const s = readDanmakuSettings()
    expect(s.opacity).toBe(1) // 钳制
    expect(s.fontSize).toBe(64) // 钳制
    expect(s.speed).toBe(1.5) // 旧档 0.75 ×2 迁移
  })
})
