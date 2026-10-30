import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import Artplayer from 'artplayer'
import { usePlayer, resolutionLabel, type UsePlayerOptions, type UsePlayerReturn } from './usePlayer'

/**
 * usePlayer composable 测试（ArtPlayer + hls.js 桩替，2026-10-30 迁移配套）。
 *
 * jsdom 无真实媒体播放 —— ArtPlayer / hls.js 均以可记录调用的类桩替：
 *  - ArtPlayer 桩: 构造 option 记到 globalThis.__lastArtOption；实例 url/type/poster/
 *    currentTime/duration/playbackRate/volume/fullscreen 用 get/set 存内部值；url setter
 *    命中 customType 时回调(模拟 ArtPlayer 装载分发)；on/off/emit 事件总线供契约断言。
 *  - hls.js 桩: isSupported()=true(可按用例覆写)，on 存 handler + emit 手动触发，
 *    用于 hls 致命错误自愈链路(netRetries/mediaRetries 阶梯)。
 */

/* ---------------- 桩实例访问 helper ---------------- */

interface ArtMockInstance {
  option: Record<string, unknown>
  video: {
    paused: boolean
    error: { message: string } | null
    videoWidth: number
    videoHeight: number
    canPlayType: (t: string) => string
    src: string
  }
  notice: { show: string }
  hls: HlsMockInstance | null | undefined
  type: string
  url: string
  poster: string
  currentTime: number
  duration: number
  playbackRate: number
  volume: number
  fullscreen: boolean
  play: ReturnType<typeof vi.fn>
  pause: ReturnType<typeof vi.fn>
  on: ReturnType<typeof vi.fn>
  off: ReturnType<typeof vi.fn>
  emit: (e: string, ...args: unknown[]) => void
  destroy: ReturnType<typeof vi.fn>
  onCalls: Array<[string, (...args: unknown[]) => void]>
  offCalls: Array<[string, (...args: unknown[]) => void]>
}

interface HlsMockInstance {
  on: ReturnType<typeof vi.fn>
  loadSource: ReturnType<typeof vi.fn>
  attachMedia: ReturnType<typeof vi.fn>
  startLoad: ReturnType<typeof vi.fn>
  recoverMediaError: ReturnType<typeof vi.fn>
  swapAudioCodec: ReturnType<typeof vi.fn>
  destroy: ReturnType<typeof vi.fn>
  bandwidthEstimate: number
  emit: (evt: string, ...args: unknown[]) => void
}

/* eslint-disable @typescript-eslint/no-explicit-any */
const g = (key: string): any => (globalThis as any)[key]

function lastArt(): ArtMockInstance {
  const list = g('__artInstances') as ArtMockInstance[]
  expect(list.length).toBeGreaterThan(0)
  return list[list.length - 1]!
}
function artList(): ArtMockInstance[] {
  return g('__artInstances') as ArtMockInstance[]
}
function lastArtOption(): Record<string, any> {
  return g('__lastArtOption')
}
function hlsList(): HlsMockInstance[] {
  return g('__hlsInstances') as HlsMockInstance[]
}
function lastHlsConfig(): Record<string, any> {
  return g('__lastHlsConfig')
}

/* ---------------- vi.mock: artplayer ---------------- */

vi.mock('artplayer', () => {
  const instances: unknown[] = []
  // notice 用带原型访问器的类: 让 usePlayer 的 suppressProgrammaticNotices 能拿到
  // prototype 上的 show 描述符并包装过滤(真实 ArtPlayer notice.show 即原型访问器)
  class NoticeMock {
    _show = ''
    get show(): string {
      return this._show
    }
    set show(v: string) {
      this._show = v
    }
  }
  class ArtMock {
    static PLAYBACK_RATE = [0.5, 0.75, 1, 1.25, 1.5, 2]
    static LOG_VERSION = true
    static CONTEXTMENU = true
    static RECONNECT_TIME_MAX = 5

    option: Record<string, any>
    video: ArtMockInstance['video']
    template: Record<string, unknown>
    notice: { show: string }
    controls: { show: boolean }
    hls: unknown
    onCalls: Array<[string, (...args: unknown[]) => void]> = []
    offCalls: Array<[string, (...args: unknown[]) => void]> = []
    private handlers = new Map<string, Array<(...args: unknown[]) => void>>()
    private _type = ''
    private _url = ''
    private _poster = ''
    private _currentTime = 0
    private _duration = 0
    private _playbackRate = 1
    private _volume = 0.6
    private _fullscreen = false

    play = vi.fn(() => Promise.resolve())
    pause = vi.fn()
    destroy = vi.fn(() => {
      // 真 ArtPlayer destroy 会广播 'destroy' 事件(hls 会话清理挂在这里)
      this.emit('destroy')
    })
    on = vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      this.onCalls.push([event, cb])
      const arr = this.handlers.get(event) ?? []
      arr.push(cb)
      this.handlers.set(event, arr)
    })
    off = vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      this.offCalls.push([event, cb])
      const arr = (this.handlers.get(event) ?? []).filter((f) => f !== cb)
      this.handlers.set(event, arr)
    })
    emit = (event: string, ...args: unknown[]): void => {
      for (const cb of [...(this.handlers.get(event) ?? [])]) cb(...args)
    }

    get type(): string {
      return this._type
    }
    set type(v: string) {
      this._type = v
    }
    get url(): string {
      return this._url
    }
    set url(v: string) {
      this._url = v
      // 模拟 ArtPlayer: url 装载时按 type 分发 customType
      const fn = this.option?.customType?.[this._type]
      if (typeof fn === 'function') fn.call(this, this.video, this._url)
    }
    get poster(): string {
      return this._poster
    }
    set poster(v: string) {
      this._poster = v
    }
    get currentTime(): number {
      return this._currentTime
    }
    set currentTime(v: number) {
      this._currentTime = v
    }
    get duration(): number {
      return this._duration
    }
    set duration(v: number) {
      this._duration = v
    }
    get playbackRate(): number {
      return this._playbackRate
    }
    set playbackRate(v: number) {
      this._playbackRate = v
    }
    get volume(): number {
      return this._volume
    }
    set volume(v: number) {
      this._volume = v
    }
    get fullscreen(): boolean {
      return this._fullscreen
    }
    set fullscreen(v: boolean) {
      this._fullscreen = v
    }

    constructor(option: Record<string, any>) {
      this.option = option
      this.video = {
        paused: true,
        error: null,
        videoWidth: 0,
        videoHeight: 0,
        canPlayType: () => (globalThis as any).__mockCanPlayType ?? 'maybe',
        src: ''
      }
      this.template = { $player: { querySelector: () => null } }
      this._poster = option.poster ?? ''
      this.notice = new NoticeMock()
      this.controls = { show: false }
      this.hls = null
      this._type = option.type ?? ''
      ;(globalThis as any).__lastArtOption = option
      instances.push(this)
      ;(globalThis as any).__artInstances = instances
      if (option.url) this.url = option.url
    }
  }
  ;(globalThis as any).__ArtMock = ArtMock
  return { default: ArtMock }
})

/* ---------------- vi.mock: hls.js ---------------- */

vi.mock('hls.js', () => {
  class HlsMock {
    static isSupported = (): boolean => true
    static Events = { ERROR: 'error', FRAG_BUFFERED: 'fragBuffered', LEVEL_SWITCHED: 'levelSwitched' }
    static ErrorTypes = { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError' }

    on = vi.fn((evt: string, cb: (...args: unknown[]) => void) => {
      const arr = this.handlers.get(evt) ?? []
      arr.push(cb)
      this.handlers.set(evt, arr)
    })
    loadSource = vi.fn()
    attachMedia = vi.fn()
    startLoad = vi.fn()
    recoverMediaError = vi.fn()
    swapAudioCodec = vi.fn()
    destroy = vi.fn()
    destroy_ = false
    bandwidthEstimate = 2_500_000
    private handlers = new Map<string, Array<(...args: unknown[]) => void>>()

    constructor(public config: Record<string, unknown>) {
      ;(globalThis as any).__lastHlsConfig = config
      const list: unknown[] = (globalThis as any).__hlsInstances ?? []
      list.push(this)
      ;(globalThis as any).__hlsInstances = list
    }

    emit = (evt: string, ...args: unknown[]): void => {
      for (const cb of [...(this.handlers.get(evt) ?? [])]) cb(...args)
    }
  }
  ;(globalThis as any).__HlsMock = HlsMock
  return { default: HlsMock, Events: HlsMock.Events, ErrorTypes: HlsMock.ErrorTypes }
})

/* ---------------- host 组件挂载 ---------------- */

function mountHost(opts: UsePlayerOptions): { api: UsePlayerReturn; w: ReturnType<typeof mount> } {
  let api!: UsePlayerReturn
  const Host = defineComponent({
    setup() {
      api = usePlayer(opts)
      return () => h('div')
    }
  })
  const w = mount(Host)
  return { api, w }
}

const MP4 = 'http://x.mp4'

beforeEach(() => {
  localStorage.clear()
  ;(globalThis as any).__artInstances = []
  ;(globalThis as any).__lastArtOption = undefined
  ;(globalThis as any).__hlsInstances = []
  ;(globalThis as any).__lastHlsConfig = undefined
  ;(globalThis as any).__mockCanPlayType = 'maybe'
  const HlsMock = g('__HlsMock') as { isSupported: () => boolean }
  if (HlsMock) HlsMock.isSupported = () => true
})

describe('usePlayer — ArtPlayer 构造 option', () => {
  it('点播模式: setting/playbackRate/aspectRatio 开, hotkey/gesture/fastForward 关, customType.m3u8 为函数', () => {
    const { api } = mountHost({ src: MP4, poster: 'p.jpg', volume: 0.8 })
    api.init(document.createElement('div'))
    const opt = lastArtOption()
    expect(opt.url).toBe(MP4)
    expect(opt.poster).toBe('p.jpg')
    expect(opt.volume).toBe(0.8)
    expect(opt.isLive).toBe(false)
    expect(opt.setting).toBe(true)
    expect(opt.playbackRate).toBe(true)
    expect(opt.aspectRatio).toBe(true)
    expect(opt.hotkey).toBe(false)
    expect(opt.gesture).toBe(false)
    expect(opt.fastForward).toBe(false)
    expect(opt.autoplay).toBe(false)
    expect(opt.playsInline).toBe(true)
    expect(opt.mutex).toBe(true)
    expect(opt.type).toBe('') // mp4 非 HLS
    expect(typeof opt.customType.m3u8).toBe('function')
    // init 幂等: 已有实例再 init 不重建
    api.init(document.createElement('div'))
    expect(artList()).toHaveLength(1)
  })

  it('直播模式: setting/playbackRate/aspectRatio 关 + isLive:true + HLS mime 声明 type', () => {
    const { api } = mountHost({
      src: 'http://live/x.m3u8',
      type: 'application/x-mpegURL',
      isLive: true
    })
    api.init(document.createElement('div'))
    const opt = lastArtOption()
    expect(opt.isLive).toBe(true)
    expect(opt.setting).toBe(false)
    expect(opt.playbackRate).toBe(false)
    expect(opt.aspectRatio).toBe(false)
    expect(opt.type).toBe('m3u8') // mime 已知, 构造期声明 type 命中 customType
    // MSE 可用 → hls.js 接管
    expect(lastArt().hls).toBeTruthy()
    expect(lastArt().hls.loadSource).toHaveBeenCalledWith('http://live/x.m3u8')
    // ArtPlayer 全局行为收敛(tuneGlobals 一次性)
    expect(Artplayer.LOG_VERSION).toBe(false)
    expect(Artplayer.CONTEXTMENU).toBe(false)
    expect(Artplayer.RECONNECT_TIME_MAX).toBe(0)
  })
})

describe('usePlayer — load() 装载', () => {
  it('load(blob, m3u8 mime) → type=m3u8 + hls.js loadSource; load(mp4) → type 清空 + 旧 hls 销毁', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(art.url).toBe(MP4)

    api.load('blob:bbb', 'application/x-mpegURL')
    expect(art.type).toBe('m3u8')
    expect(art.url).toBe('blob:bbb')
    const hls1 = art.hls as HlsMockInstance
    expect(hls1).toBeTruthy()
    expect(hls1.loadSource).toHaveBeenCalledWith('blob:bbb')
    expect(hls1.attachMedia).toHaveBeenCalledWith(art.video)

    // 再装一条 m3u8: 上一条 hls 会话销毁 + 带宽估算落盘
    api.load('blob:ccc', 'application/x-mpegURL')
    const hls2 = art.hls as HlsMockInstance
    expect(hls2).not.toBe(hls1)
    expect(hls1.destroy).toHaveBeenCalled()
    expect(localStorage.getItem('jc-hls-bandwidth-estimate')).toBe('2500000')

    // 直链: type 清空 + 残留 hls 销毁 + art.hls 置空
    api.load('http://y.mp4')
    expect(art.type).toBe('')
    expect(art.url).toBe('http://y.mp4')
    expect(hls2.destroy).toHaveBeenCalled()
    expect(art.hls).toBeUndefined()
  })

  it('响应式 src: ref 变化 → loadInternal 同路径装载', async () => {
    const srcRef = ref(MP4)
    const typeRef = ref<string | undefined>(undefined)
    const { api } = mountHost({ src: srcRef, type: typeRef })
    api.init(document.createElement('div'))
    const art = lastArt()

    srcRef.value = 'blob:ccc'
    typeRef.value = 'application/x-mpegURL'
    await nextTick()
    expect(art.type).toBe('m3u8')
    expect(art.url).toBe('blob:ccc')
    // autoplay 未开: 切源不自动 play
    expect(art.play).not.toHaveBeenCalled()
  })

  it('响应式 poster: ref 变化 → art.poster 更新', async () => {
    const posterRef = ref<string | undefined>('a.jpg')
    const { api } = mountHost({ src: MP4, poster: posterRef })
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(art.poster).toBe('a.jpg')
    posterRef.value = 'b.jpg'
    await nextTick()
    expect(art.poster).toBe('b.jpg')
  })
})

describe('usePlayer — seek / seekBy / volume', () => {
  it('duration=0 时 seek 忽略; duration>0 时钳制到 [0, duration]', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.duration = 0
    api.seek(100)
    expect(art.currentTime).toBe(0) // 元数据未就绪: 不重置进度
    art.duration = 100
    api.seek(150)
    expect(art.currentTime).toBe(100)
    api.seek(-5)
    expect(art.currentTime).toBe(0)
    api.seek(50)
    expect(art.currentTime).toBe(50)
  })

  it('seekBy 相对跳转基于实例 currentTime', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.duration = 100
    art.currentTime = 30
    api.seekBy(10)
    expect(art.currentTime).toBe(40)
    api.seekBy(-50)
    expect(art.currentTime).toBe(0) // 钳到 0
  })

  it('setVolume 钳制 0-1 并清掉 notice', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    api.setVolume(2)
    expect(art.volume).toBe(1)
    api.setVolume(-1)
    expect(art.volume).toBe(0)
    api.setVolume(0.5)
    expect(art.volume).toBe(0.5)
    expect(art.notice.show).toBe('')
  })
})

describe('usePlayer — 事件契约', () => {
  it("on('ended') → art 'video:ended'; on('fullscreenchange') → art 'fullscreen'", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    const ended = vi.fn()
    const fsc = vi.fn()
    api.on('ended', ended)
    api.on('fullscreenchange', fsc)
    expect(art.onCalls.some(([e, cb]) => e === 'video:ended' && cb === ended)).toBe(true)
    expect(art.onCalls.some(([e, cb]) => e === 'fullscreen' && cb === fsc)).toBe(true)
    art.emit('video:ended')
    art.emit('fullscreen')
    expect(ended).toHaveBeenCalledTimes(1)
    expect(fsc).toHaveBeenCalledTimes(1)
  })

  it("on('useractive') → art 'control' 包装(state=true 才透传); off 对称解绑包装闭包", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    const ua = vi.fn()
    api.on('useractive', ua)
    const [, wrapped] = art.onCalls.find(([e]) => e === 'control')!
    expect(wrapped).not.toBe(ua) // 包装闭包
    art.emit('control', false)
    expect(ua).not.toHaveBeenCalled()
    art.emit('control', true)
    expect(ua).toHaveBeenCalledTimes(1)
    // off: 实例侧解绑的是包装闭包(非原 fn)
    api.off('useractive', ua)
    const offCall = art.offCalls.find(([e]) => e === 'control')
    expect(offCall).toBeTruthy()
    expect(offCall![1]).toBe(wrapped)
    art.emit('control', true)
    expect(ua).toHaveBeenCalledTimes(1) // 已解绑, 不再透传
  })

  it("error 不直挂实例: on('error') 不产生 art.on 调用, 由 video:error 手动分发(1s 去抖)", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    const errFn = vi.fn()
    api.on('error', errFn)
    expect(art.onCalls.every(([e]) => e !== 'error')).toBe(true)
    art.video.error = { message: 'boom' }
    art.emit('video:error')
    art.emit('video:error')
    expect(errFn).toHaveBeenCalledTimes(1) // 1s 窗口内去抖
    expect(api.errored.value).toBe(true)
    // 越过 1s 窗口后可再次分发
    const base = Date.now()
    vi.useFakeTimers()
    try {
      vi.setSystemTime(base + 1001)
      art.emit('video:error')
      expect(errFn).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('init 前注册的订阅在 init 后统一重挂(attachRegistryToArt)', () => {
    const { api } = mountHost({ src: MP4 })
    const ended = vi.fn()
    api.on('ended', ended) // player 还没建
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(art.onCalls.some(([e, cb]) => e === 'video:ended' && cb === ended)).toBe(true)
  })
})

describe('usePlayer — 播放状态机', () => {
  it("ready/playing/pause/timeupdate/loadedmetadata/waiting/canplay 驱动响应式状态", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(api.ready.value).toBe(false)
    art.emit('ready')
    expect(api.ready.value).toBe(true)
    expect(api.paused.value).toBe(true) // ready 时同步 video.paused
    art.emit('video:play')
    expect(api.paused.value).toBe(false)
    art.emit('video:pause')
    expect(api.paused.value).toBe(true)
    art.currentTime = 42
    art.emit('video:timeupdate')
    expect(api.currentTime.value).toBe(42)
    art.duration = 120
    art.video.videoWidth = 1920
    art.video.videoHeight = 1080
    art.emit('video:loadedmetadata')
    expect(api.duration.value).toBe(120)
    expect(api.videoSize.value).toEqual({ width: 1920, height: 1080 })
    art.emit('video:waiting')
    expect(api.buffering.value).toBe(true)
    art.emit('video:error')
    expect(api.errored.value).toBe(true)
    art.emit('video:canplay')
    expect(api.buffering.value).toBe(false)
    expect(api.errored.value).toBe(false)
  })

  it('play()/pause() 透传实例; play() 吞掉自动播放策略 reject', async () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    await api.play()
    expect(art.play).toHaveBeenCalledTimes(1)
    api.pause()
    expect(art.pause).toHaveBeenCalledTimes(1)
    // 浏览器 autoplay 策略 reject → play() 静默
    art.play.mockImplementationOnce(() => Promise.reject(new Error('NotAllowedError')))
    await expect(api.play()).resolves.toBeUndefined()
  })
})

describe('usePlayer — 倍速', () => {
  it("ratechange: 用户选非 1 档位 → 写 localStorage; 1 倍速/临时倍速不落盘", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.playbackRate = 1.5
    art.emit('video:ratechange')
    expect(localStorage.getItem('jc-playback-rate')).toBe('1.5')
    // 1 倍速是默认值不记
    art.playbackRate = 1
    art.emit('video:ratechange')
    expect(localStorage.getItem('jc-playback-rate')).toBe('1.5')
    // 非 support 档位不记
    art.playbackRate = 3
    art.emit('video:ratechange')
    expect(localStorage.getItem('jc-playback-rate')).toBe('1.5')
    // 临时倍速(手势长按)不落盘(先把倍速回默认档, 让 tempRateBefore=1)
    art.playbackRate = 1
    api.enterTempRate(3)
    expect(art.playbackRate).toBe(3)
    art.emit('video:ratechange')
    expect(localStorage.getItem('jc-playback-rate')).toBe('1.5')
    api.exitTempRate()
    expect(art.playbackRate).toBe(1) // 恢复进入前档位
  })

  it('ready 时恢复上次倍速 + 档位注入 Artplayer.PLAYBACK_RATE', () => {
    localStorage.setItem('jc-playback-rate', '2')
    const { api } = mountHost({ src: MP4, playbackRates: [0.5, 1, 2] })
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(Artplayer.PLAYBACK_RATE).toEqual([0.5, 1, 2])
    art.emit('ready')
    expect(art.playbackRate).toBe(2) // 缓存恢复
    // 非法档位缓存不生效
    localStorage.setItem('jc-playback-rate', '7')
    const h2 = mountHost({ src: MP4, playbackRates: [0.5, 1, 2] })
    h2.api.init(document.createElement('div'))
    const art2 = lastArt()
    art2.emit('ready')
    expect(art2.playbackRate).toBe(1) // 默认值
  })
})

describe('usePlayer — hls.js 自愈链路', () => {
  function initHls(): { api: UsePlayerReturn; art: ArtMockInstance; hls: HlsMockInstance } {
    const { api } = mountHost({ src: 'http://live/x.m3u8', type: 'application/x-mpegURL' })
    api.init(document.createElement('div'))
    const art = lastArt()
    return { api, art, hls: art.hls as HlsMockInstance }
  }

  it('hls 配置: 弱网缓冲/默认带宽估算/贴源 startLevel', () => {
    initHls()
    const cfg = lastHlsConfig()
    expect(cfg.enableWorker).toBe(true)
    expect(cfg.maxBufferLength).toBe(40)
    expect(cfg.maxMaxBufferLength).toBe(80)
    expect(cfg.abrEwmaDefaultEstimate).toBe(2_000_000)
    expect(cfg.startLevel).toBe(-1)
  })

  it('网络致命错误: 重试 3 次(FRAG_BUFFERED 复位) → 超限销毁并冒泡 error', () => {
    const { api } = initHls()
    const hls = hlsList()[hlsList().length - 1]!
    const errFn = vi.fn()
    api.on('error', errFn)
    const netErr = { fatal: true, type: 'networkError', details: 'manifestLoadError' }
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(hls.destroy).not.toHaveBeenCalled()
    // 第 4 次: 超限 → 销毁 + 订阅者收到一次(去抖)
    hls.emit('error', {}, { ...netErr })
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(hls.destroy).toHaveBeenCalledTimes(1)
    expect(errFn).toHaveBeenCalledTimes(1)
    expect(api.errored.value).toBe(true)
  })

  it('媒体致命错误: recoverMediaError ×2(第二次换音频编解码) → 超限销毁', () => {
    const { api } = initHls()
    const hls = hlsList()[hlsList().length - 1]!
    const errFn = vi.fn()
    api.on('error', errFn)
    const mediaErr = { fatal: true, type: 'mediaError', details: 'bufferStalledError' }
    hls.emit('error', {}, { ...mediaErr })
    hls.emit('error', {}, { ...mediaErr })
    expect(hls.recoverMediaError).toHaveBeenCalledTimes(2)
    expect(hls.swapAudioCodec).toHaveBeenCalledTimes(1) // 第二次轮换编解码
    hls.emit('error', {}, { ...mediaErr })
    expect(hls.destroy).toHaveBeenCalledTimes(1)
    expect(errFn).toHaveBeenCalledTimes(1)
  })

  it('非致命错误忽略; FRAG_BUFFERED 不清 mediaRetries 阶梯', () => {
    initHls()
    const hls = hlsList()[hlsList().length - 1]!
    hls.emit('error', {}, { fatal: false, type: 'networkError', details: 'fragLoadError' })
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(hls.destroy).not.toHaveBeenCalled()
  })

  it('MSE 不可用: 原生 HLS 退化(Safari) 或 notice 提示不支持', () => {
    const HlsMock = g('__HlsMock') as { isSupported: () => boolean }
    // iOS Safari: canPlayType 支持 → video.src 直链
    HlsMock.isSupported = () => false
    const { api } = mountHost({ src: 'http://live/x.m3u8', type: 'application/x-mpegURL' })
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(art.video.src).toBe('http://live/x.m3u8')
    expect(art.hls).toBeNull()
    // 完全不支持: notice 提示
    ;(globalThis as any).__mockCanPlayType = ''
    const h2 = mountHost({ src: 'http://live/y.m3u8', type: 'application/x-mpegURL' })
    h2.api.init(document.createElement('div'))
    expect(lastArt().notice.show).toContain('不支持 HLS')
  })

  it('dispose: destroy 调用 + hls 会话经 art destroy 事件清理 + 带宽估算落盘', () => {
    const { api } = initHls()
    const art = lastArt()
    const hls = art.hls as HlsMockInstance
    api.dispose()
    expect(art.destroy).toHaveBeenCalledWith(true)
    expect(hls.destroy).toHaveBeenCalledTimes(1) // hlsDestroyHandler 经 'destroy' 事件触发
    expect(localStorage.getItem('jc-hls-bandwidth-estimate')).toBe('2500000')
    expect(api.player.value).toBeNull()
    expect(api.ready.value).toBe(false)
    expect(api.duration.value).toBe(0)
    expect(api.paused.value).toBe(true)
  })
})

describe('usePlayer — dispose 与回调单例', () => {
  it('dispose 清空控制条回调单例: 旧控件点击失效, 重建后新回调生效', () => {
    const settings1 = vi.fn()
    const { api } = mountHost({ src: MP4, danmakuSettings: { onClick: settings1 } })
    api.init(document.createElement('div'))
    const art1 = lastArt()
    api.dispose()
    expect(art1.destroy).toHaveBeenCalledWith(true)
    // 单例已清空: 旧控件点击不触发(也不抛错)
    const oldCtrl = (art1.option.controls as Array<{ name: string; click: () => void }>).find(
      (c) => c.name === 'danmaku-settings'
    )!
    oldCtrl.click()
    expect(settings1).not.toHaveBeenCalled()
    // 重复 dispose 安全
    api.dispose()
    // 重新 init 注册新 settings → 新回调生效 → 再 dispose 不报错
    const settings2 = vi.fn()
    const h2 = mountHost({ src: MP4, danmakuSettings: { onClick: settings2 } })
    h2.api.init(document.createElement('div'))
    const art2 = lastArt()
    const newCtrl = (art2.option.controls as Array<{ name: string; click: () => void }>).find(
      (c) => c.name === 'danmaku-settings'
    )!
    newCtrl.click()
    expect(settings2).toHaveBeenCalledTimes(1)
    h2.api.dispose()
  })

  it('dispose 后再注册订阅 → 重建 init 重挂', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    api.dispose()
    const ended = vi.fn()
    api.on('ended', ended)
    api.init(document.createElement('div'))
    const art = lastArt()
    expect(art.onCalls.some(([e, cb]) => e === 'video:ended' && cb === ended)).toBe(true)
    art.emit('video:ended')
    expect(ended).toHaveBeenCalledTimes(1)
  })
})

describe('usePlayer — 导出工具函数', () => {
  it('isHlsSupported 跟随 hls.js isSupported', async () => {
    const { isHlsSupported } = await import('./usePlayer')
    expect(isHlsSupported()).toBe(true)
  })

  it('resolutionLabel 按分辨率打标', () => {
    expect(resolutionLabel(3840, 2160)).toBe('4K')
    expect(resolutionLabel(2560, 1440)).toBe('2K')
    expect(resolutionLabel(1920, 1080)).toBe('1080P')
    expect(resolutionLabel(1280, 720)).toBe('720P')
    expect(resolutionLabel(854, 480)).toBe('480P')
    expect(resolutionLabel(600, 360)).toBe('360P')
    expect(resolutionLabel(100, 0)).toBe('标清')
  })
})

describe('usePlayer — 补充盲区(覆盖率补强)', () => {
  it('控制条自定义按钮: 只注册 nextEpisode(左)+ danmakuSettings(右); 无 danmakuToggle/playSettings', () => {
    const settings = vi.fn()
    const next = vi.fn()
    const { api } = mountHost({
      src: MP4,
      danmakuSettings: { onClick: settings },
      nextEpisode: { onClick: next }
    })
    api.init(document.createElement('div'))
    const ctrls = lastArtOption().controls as Array<{
      name: string
      position: string
      click: () => void
    }>
    // next-episode 在左侧(index=15), danmaku-settings 在右侧(index=15)
    const nextCtrl = ctrls.find((c) => c.name === 'next-episode')
    const settingsCtrl = ctrls.find((c) => c.name === 'danmaku-settings')
    expect(nextCtrl).toBeTruthy()
    expect(nextCtrl!.position).toBe('left')
    expect(settingsCtrl).toBeTruthy()
    expect(settingsCtrl!.position).toBe('right')
    // 不应出现已被移除的 danmaku-toggle / playSettings
    expect(ctrls.every((c) => c.name !== 'danmaku-toggle')).toBe(true)
    expect(ctrls.every((c) => c.name !== 'play-settings')).toBe(true)
    settingsCtrl!.click()
    nextCtrl!.click()
    expect(settings).toHaveBeenCalledTimes(1)
    expect(next).toHaveBeenCalledTimes(1)
  })

  it('settings 选项透传: 三项自定义开关项(弹幕/过滤广告/自动连播)', () => {
    const { api } = mountHost({
      src: MP4,
      settings: [
        { name: 'jc-danmaku', html: '弹幕', switch: true, onSwitch: () => false },
        { name: 'jc-ad', html: '去广告', switch: false, onSwitch: () => true },
        { name: 'jc-autoplay', html: '连播', switch: true, onSwitch: () => true }
      ] as Artplayer['option']['settings']
    })
    api.init(document.createElement('div'))
    const settings = lastArtOption().settings as Array<{ name: string; html: string; switch: boolean }>
    expect(Array.isArray(settings)).toBe(true)
    expect(settings.length).toBe(3)
    expect(settings.map((s) => s.name)).toEqual(['jc-danmaku', 'jc-ad', 'jc-autoplay'])
    expect(settings[0].switch).toBe(true)
    expect(settings[1].switch).toBe(false)
    expect(settings[2].switch).toBe(true)
  })

  it("on('ready') → art 'ready'; on('userinactive') → 'control' 包装(state=false 才透传)", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    const ready = vi.fn()
    const ui = vi.fn()
    api.on('ready', ready)
    api.on('userinactive', ui)
    expect(art.onCalls.some(([e, cb]) => e === 'ready' && cb === ready)).toBe(true)
    art.emit('control', true)
    expect(ui).not.toHaveBeenCalled()
    art.emit('control', false)
    expect(ui).toHaveBeenCalledTimes(1)
  })

  it('FRAG_BUFFERED 复位网络重试计数(分片成功后重新计 3 次)', () => {
    const { api } = mountHost({ src: 'http://live/x.m3u8', type: 'application/x-mpegURL' })
    api.init(document.createElement('div'))
    const hls = hlsList()[hlsList().length - 1]!
    const netErr = { fatal: true, type: 'networkError', details: 'manifestLoadError' }
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    hls.emit('fragBuffered') // 分片成功 → 计数复位
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    hls.emit('error', {}, { ...netErr })
    expect(hls.startLoad).toHaveBeenCalledTimes(6)
    expect(hls.destroy).not.toHaveBeenCalled()
  })

  it('load 空串 no-op', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    api.load('')
    expect(art.url).toBe(MP4)
  })

  it('init 前调用命令式 API 均安全 no-op', () => {
    const { api } = mountHost({ src: MP4 })
    expect(() => {
      api.play()
      api.pause()
      api.seek(10)
      api.seekBy(5)
      api.setVolume(0.5)
      api.load('http://y.mp4')
      api.enterTempRate(2)
      api.exitTempRate()
      api.off('ended', vi.fn()) // 未知订阅 off 安全
    }).not.toThrow()
    expect(artList()).toHaveLength(0)
  })

  it("off('error') 不触碰实例(error 未直挂 art)", () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    const fn = vi.fn()
    api.on('error', fn)
    api.off('error', fn)
    expect(art.offCalls.every(([e]) => e !== 'error')).toBe(true)
  })

  it('autoplay 切源自动重播(reject 静默); 空 src 跳过装载', async () => {
    const srcRef = ref(MP4)
    const { api } = mountHost({ src: srcRef, autoplay: true })
    api.init(document.createElement('div'))
    const art = lastArt()
    srcRef.value = 'http://b.mp4'
    await nextTick()
    expect(art.play).toHaveBeenCalledTimes(1)
    // 浏览器打断旧 play() → reject 被吞
    srcRef.value = 'http://c.mp4'
    art.play.mockImplementationOnce(() => Promise.reject(new Error('AbortError')))
    await nextTick()
    expect(art.play).toHaveBeenCalledTimes(2)
    // 空 src: 不装载
    srcRef.value = ''
    await nextTick()
    expect(art.url).toBe('http://c.mp4')
  })

  it('volume NaN 钳为 0(构造 option)', () => {
    const { api } = mountHost({ src: MP4, volume: NaN })
    api.init(document.createElement('div'))
    expect(lastArtOption().volume).toBe(0)
  })

  it('durationchange 更新时长; video:playing 退出缓冲态', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.duration = 90
    art.emit('video:durationchange')
    expect(api.duration.value).toBe(90)
    art.emit('video:waiting')
    expect(api.buffering.value).toBe(true)
    art.emit('video:playing')
    expect(api.buffering.value).toBe(false)
  })

  it('loadedmetadata 后重新套用缓存倍速(切源被底层重置时自愈)', () => {
    localStorage.setItem('jc-playback-rate', '2')
    const { api } = mountHost({ src: MP4, playbackRates: [1, 2] })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.playbackRate = 1 // 模拟换流后底层重置
    art.emit('video:loadedmetadata')
    expect(art.playbackRate).toBe(2)
  })

  it('程序化 notice 过滤: Play/Pause/Rate 文案被吞, 其它保留', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.notice.show = 'Play'
    expect(art.notice.show).toBe('')
    art.notice.show = 'Pause'
    expect(art.notice.show).toBe('')
    art.notice.show = 'Rate: 2.0x'
    expect(art.notice.show).toBe('')
    art.notice.show = '音量 50%'
    expect(art.notice.show).toBe('音量 50%')
  })

  it('组件卸载(onScopeDispose)自动 dispose', () => {
    const { api, w } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    w.unmount()
    expect(art.destroy).toHaveBeenCalledWith(true)
    expect(api.player.value).toBeNull()
  })

  it('dispose 重置 videoSize', () => {
    const { api } = mountHost({ src: MP4 })
    api.init(document.createElement('div'))
    const art = lastArt()
    art.video.videoWidth = 640
    art.video.videoHeight = 360
    art.emit('video:loadedmetadata')
    expect(api.videoSize.value).toEqual({ width: 640, height: 360 })
    api.dispose()
    expect(api.videoSize.value).toBeNull()
  })
})
