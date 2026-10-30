import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref, shallowRef, nextTick } from 'vue'
import {
  useDanmaku,
  readDanmakuSettings,
  DANMAKU_DEFAULT_SETTINGS,
  DANMAKU_FETCH_TIMEOUTS,
  type DanmuState
} from './useDanmaku'
import { SETTINGS_APPLIED_EVENT } from '@/utils/settingEvents'
import { ApiError } from '@/types/api'

/**
 * useDanmaku composable 测试 —— 聚合单源数据流（标题+集号 → /danmu/aggregate）
 * 状态机覆盖 + 关→开恢复 + 过滤重映射 + 超时。
 *
 * 渲染层为 artplayer-plugin-danmuku（2026-10-30 换引擎）：插件实例挂在
 * art.plugins.artplayerPluginDanmuku 上, 这里用假 player + 假插件实例
 * （记录 load/config/show/hide 调用做断言）。
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

/** 假弹幕插件实例：记录装载/设置/显隐动作，供断言。 */
const fakePlugin = {
  load: vi.fn(async () => undefined),
  config: vi.fn(),
  show: vi.fn(),
  hide: vi.fn(),
  reset: vi.fn()
}

/** 带 plugins 挂点的假 Artplayer 实例（useDanmaku 只取 plugins[name]）。 */
function makeFakeArt(withPlugin = true) {
  return {
    plugins: withPlugin ? { artplayerPluginDanmuku: fakePlugin } : {}
  } as unknown as import('artplayer').default
}

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

function mountHost(mid: number | undefined, enabled = true, withPlayer = true) {
  const midRef = ref(mid)
  const enabledRef = ref(enabled)
  const playingRef = ref(true)
  const titleRef = ref('庆余年 第二季')
  const yearRef = ref<number | undefined>(2024)
  const episodeRef = ref(1)
  const playerRef = shallowRef<import('artplayer').default | null>(withPlayer ? makeFakeArt() : null)
  const Host = defineComponent({
    setup() {
      const api = useDanmaku({
        mid: midRef,
        title: titleRef,
        year: yearRef,
        episode: episodeRef,
        player: playerRef,
        playing: playingRef,
        enabled: enabledRef
      })
      return { api }
    },
    render() {
      return h('div')
    }
  })
  const w = mount(Host)
  const api = (w.vm as unknown as { api: ReturnType<typeof useDanmaku> }).api
  return { w, api, midRef, enabledRef, titleRef, playingRef, episodeRef, playerRef }
}

describe('useDanmaku', () => {
  beforeEach(() => {
    localStorage.clear()
    // 复位请求超时（超时测试会改小），避免串扰
    DANMAKU_FETCH_TIMEOUTS.aggregate = 30_000
    for (const fn of Object.values(fakePlugin)) {
      if (typeof fn === 'function' && 'mockClear' in (fn as unknown as Record<string, unknown>)) {
        ;(fn as unknown as { mockClear: () => void }).mockClear()
      }
    }
    m.getDanmuAggregate.mockReset()
  })

  it('正常链路: 聚合接口返回弹幕 ⇒ ready 且 plugin.load 收到插件 Danmu[]', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.hasDanmaku.value).toBe(true)
    expect(fakePlugin.load).toHaveBeenCalledTimes(1)
    expect(m.getDanmuAggregate).toHaveBeenCalledWith(
      { title: '庆余年 第二季', episode: 1, year: 2024 },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    const items = fakePlugin.load.mock.calls[0]![0] as Array<{ time: number; mode: number; color: string; text: string }>
    expect(items).toHaveLength(3)
    expect(items[0]!.time).toBe(1.25) // 毫秒 1250 → 秒（非整秒, 不被打散）
    expect(items[0]!.mode).toBe(0)
    expect(items[0]!.color).toBe('#ffffff')
    // 源 mode 4 → 插件 2（底部）
    expect(items[2]!.mode).toBe(2)
    expect(items[2]!.color).toBe('#ff0000')
  })

  it('404（查无作品/该集）⇒ unsupported「该片暂无弹幕」', async () => {
    m.getDanmuAggregate.mockRejectedValue(new ApiError(404))
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(fakePlugin.load).not.toHaveBeenCalled()
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
    // 初始关闭: 避免 immediate 装载后"同集已装载→恢复显示"分支抢先（title 置空不改变集 key）
    const { api, titleRef, enabledRef } = mountHost(3, false)
    await flush()
    titleRef.value = ''
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(m.getDanmuAggregate).not.toHaveBeenCalled() // 无标题无从匹配
  })

  it('回归·关闭弹幕后再打开应重新装载并显示（关→开不失效）', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, false) // 初始关闭
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(fakePlugin.load).not.toHaveBeenCalled() // 关闭态不装载
    expect(fakePlugin.hide).toHaveBeenCalled() // 关闭态隐藏弹幕层
    enabledRef.value = true // 首次打开
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(fakePlugin.load).toHaveBeenCalledTimes(1) // 装载一次
    // 同集关→开: 即时恢复显示, 不重新拉取装载(免 clear+重拉的"等好久")
    enabledRef.value = false
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(fakePlugin.hide).toHaveBeenCalled() // 仅隐藏, 内容保留
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(fakePlugin.load).toHaveBeenCalledTimes(1) // 未重载
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(1) // 命中内存缓存
  })

  it('底部/顶部过滤: 内存重映射, 不再发请求', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    fakePlugin.load.mockClear()
    api.updateSettings({ bottomMode: false })
    await flush()
    expect(fakePlugin.load).toHaveBeenCalledTimes(1) // 内存重映射
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(1) // 零请求
    const items = fakePlugin.load.mock.calls[0]![0] as Array<{ mode: number }>
    expect(items.some((it) => it.mode === 2)).toBe(false) // 底部(2)被过滤
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

  /* ---------------- 覆盖率补强(2026-10-30) ---------------- */

  it('损坏的设置 JSON 回默认值', () => {
    localStorage.setItem('jc-danmaku-settings', '{bad json')
    expect(readDanmakuSettings()).toEqual(DANMAKU_DEFAULT_SETTINGS)
  })

  it('player 缺失(插件取不到) ⇒ failed', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, episodeRef } = mountHost(3, true, false)
    await flush()
    // player 为 null: watch 仍触发 loadDanmaku, 但插件取不到 → failed 短路
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    episodeRef.value = 2
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    expect(m.getDanmuAggregate).not.toHaveBeenCalled() // 插件缺失即短路, 不发请求
  })

  it('player 就绪后补装载(watch player 触发)', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, playerRef } = mountHost(3, true, false)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    playerRef.value = makeFakeArt()
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(fakePlugin.load).toHaveBeenCalledTimes(1)
  })

  it('内存缓存 FIFO: 超过 8 集淘汰最旧集, 回看重新拉取', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { episodeRef } = mountHost(3)
    await flush() // ep1
    for (let ep = 2; ep <= 9; ep++) {
      episodeRef.value = ep
      await flush()
    }
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(9)
    episodeRef.value = 1 // ep1 已被 FIFO 淘汰 → 重拉
    await flush()
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(10)
  })

  it('updateSettings 非内容项: 插件 config 即时生效, 不重载', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    fakePlugin.load.mockClear()
    api.updateSettings({ opacity: 0.5, fontSize: 20 })
    await flush()
    expect(fakePlugin.config).toHaveBeenCalled()
    const cfg = fakePlugin.config.mock.calls.at(-1)![0] as { opacity: number; fontSize: number }
    expect(cfg.opacity).toBe(0.5)
    expect(cfg.fontSize).toBe(20) // 固定字号档直接生效
    expect(fakePlugin.load).not.toHaveBeenCalled()
  })

  it('updateSettings 内容项但无已装内容: 标记失效重载, 开关后按新映射装载', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, enabledRef } = mountHost(3, false) // 初始关闭
    api.updateSettings({ bottomMode: false })
    await flush()
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    const items = fakePlugin.load.mock.calls.at(-1)![0] as Array<{ mode: number }>
    expect(items.some((it) => it.mode === 2)).toBe(false)
  })

  it('播放/暂停切换不触发重载(渲染跟随由插件自驱)', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api, playingRef } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    playingRef.value = false
    await flush()
    playingRef.value = true
    await flush()
    expect(m.getDanmuAggregate).toHaveBeenCalledTimes(1)
    expect(fakePlugin.load).toHaveBeenCalledTimes(1)
  })

  it('云端设置合并事件: 重读设置, 内容映射变化(彩色)触发重载且全白', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(fakePlugin.load).toHaveBeenCalledTimes(1)
    localStorage.setItem('jc-danmaku-settings', JSON.stringify({ colored: false }))
    window.dispatchEvent(new CustomEvent(SETTINGS_APPLIED_EVENT))
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    const items = fakePlugin.load.mock.calls.at(-1)![0] as Array<{ color: string }>
    expect(items.length).toBeGreaterThan(0)
    expect(items.every((it) => it.color === '#ffffff')).toBe(true) // colored:false → 全白
  })

  it('卸载清理: 不抛错(插件随 player.destroy 销毁, 这里只清缓存/中止请求)', async () => {
    m.getDanmuAggregate.mockResolvedValue({ comments })
    const { w } = mountHost(3)
    await flush()
    expect(() => w.unmount()).not.toThrow()
  })
})
