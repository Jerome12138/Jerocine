import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref, shallowRef, nextTick } from 'vue'
import {
  useDanmaku,
  readDanmakuSettings,
  DANMAKU_DEFAULT_SETTINGS,
  type DanmuState
} from './useDanmaku'

/**
 * useDanmaku composable 测试 —— 覆盖状态机六态 + 2026-10-25 线上死锁回归。
 *
 * 死锁背景: 模板弹幕层曾 v-if=providers 非空, 而 useDanmaku 又要求 stage 挂载后才建
 * manager ⇒ providers 永远为空、按钮全不显示。修复=模板 stage 常驻(见 PlayView) +
 * composable 先落 providers 再建 manager。回归用例模拟"stage 常驻"的 host, 断言
 * 有源时 providers 必非空、链路必走到 ready。
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

/** 假 CommentCoreLibrary: 记录装载动作, 供 ready 路径断言 */
class FakeManager {
  init = vi.fn()
  load = vi.fn()
  time = vi.fn()
  start = vi.fn()
  stop = vi.fn()
  clear = vi.fn()
  seek = vi.fn()
  // applySettings 会读写 options/filter（真实库同款结构）
  options: { global?: { opacity?: number; scale?: number }; scroll?: { opacity?: number; scale?: number }; limit?: number } = {
    scroll: {},
    limit: 0
  }
  filter: {
    allowTypes: Record<string, boolean>
    addModifier: ReturnType<typeof vi.fn>
    removeModifier: ReturnType<typeof vi.fn>
  } = {
    allowTypes: {},
    addModifier: vi.fn(),
    removeModifier: vi.fn()
  }
}
let managerInstances: FakeManager[] = []

function installLib(): void {
  managerInstances = []
  // 与官方 dist(0.11.0)一致: 顶层 var CommentManager 挂 window(无聚合对象)
  ;(window as unknown as { CommentManager?: unknown }).CommentManager = class extends FakeManager {
    constructor() {
      super()
      managerInstances.push(this)
    }
  }
}
function uninstallLib(): void {
  delete (window as unknown as { CommentManager?: unknown }).CommentManager
  managerInstances = []
}

const flush = async (): Promise<void> => {
  // loadDanmaku 内部串行 await 多个接口; 一个宏任务 tick 足以排空全部微任务
  await new Promise((r) => setTimeout(r, 0))
  await nextTick()
}

const biliSrc = { providers: [{ provider: 'bilibili', resourceId: '44847' }] }
const tencentSrc = { providers: [{ provider: 'tencent', resourceId: 'mzc00200miw8m3y' }] }
const comments = [
  { text: '第一条', time: 1000, color: 16777215, mode: 1 },
  { text: '第二条', time: 2000, color: 16777215, mode: 1 }
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

function mountHost(mid: number | undefined, enabled = true, attachStage = true) {
  const midRef = ref(mid)
  const enabledRef = ref(enabled)
  const events: string[] = []
  const triggers: Record<string, () => void> = {}
  const Host = defineComponent({
    setup() {
      const api = useDanmaku({
        mid: midRef,
        episode: ref(1),
        player: shallowRef(null),
        playing: ref(true),
        enabled: enabledRef,
        onPlayerEvent: (event, fn) => {
          events.push(event)
          triggers[event] = fn // 供用例手动触发（seeked 去重断言用）
          return () => undefined
        }
      })
      return { api }
    },
    render() {
      // 与 PlayView 模板同款: stage 常驻(修复后姿势), ref 直接绑 api.stage。
      // attachStage=false 模拟"模板回退到 v-if=providers 非空"的旧姿势: stage 不在 DOM。
      return attachStage ? h('div', { ref: this.api.stage }) : h('div')
    }
  })
  const w = mount(Host)
  type Api = ReturnType<typeof useDanmaku>
  const api = (w.vm as unknown as { api: Api }).api
  return { w, api, midRef, enabledRef, events, triggers }
}

describe('useDanmaku', () => {
  beforeEach(() => {
    installLib()
    localStorage.clear()
    m.getDanmuSource.mockReset()
    m.getDanmuBilibili.mockReset()
    m.getDanmuTencentEpisodes.mockReset()
    m.getDanmuTencentSegments.mockReset()
    m.getDanmuTencentComments.mockReset()
  })
  afterEach(() => {
    uninstallLib()
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
    // manager 建起且装载了弹幕
    expect(managerInstances).toHaveLength(1)
    expect(managerInstances[0].load).toHaveBeenCalledTimes(2) // 两段各 load 一次(非空)
  })

  it('回归·死锁顺序: stage 未挂载(模板回退 v-if 的旧姿势) ⇒ providers 仍先落库, UI 有按钮', async () => {
    // 旧实现先 ensureManager 后取源: stage null ⇒ manager 建不起 ⇒ 提前 return ⇒
    // providers 恒空、按钮全不显示(2026-10-25 线上事故)。修复后顺序相反, 本用例对顺序敏感。
    // mid 从无到有模拟首播(detail 加载后赋值), 触发 watch → loadDanmaku。
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api, midRef } = mountHost(undefined, true, false)
    midRef.value = 3
    await flush()
    expect(api.providers.value).toHaveLength(1)
    expect(api.danmuState.value).toBe<DanmuState>('failed') // 库建不起, 但入口可见
    expect(m.getDanmuBilibili).not.toHaveBeenCalled()
  })

  it('无候选源（平台无此片）⇒ unsupported, providers 空, 不建 manager', async () => {
    m.getDanmuSource.mockResolvedValue({ providers: [] })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(api.providers.value).toEqual([])
    expect(managerInstances).toHaveLength(0)
  })

  it('source 接口失败 ⇒ unsupported（用户要求查不到也要有显示, 不许静默）', async () => {
    m.getDanmuSource.mockRejectedValue(new Error('500'))
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('unsupported')
    expect(api.providers.value).toEqual([])
  })

  it('B站路径: 拉到弹幕 ⇒ ready 且装载', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(api.hasDanmaku.value).toBe(true)
    expect(managerInstances[0].load).toHaveBeenCalledTimes(1)
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

  it('腾讯路径: 平台无匹配集 ⇒ empty', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain({ matched: false })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('empty')
  })

  it('腾讯路径: comments 请求失败 ⇒ failed（区别于内容空）', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain({ commentsOk: false })
    const { api } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    expect(api.providers.value).toHaveLength(1)
  })

  it('开关关闭 ⇒ idle 且不发任何请求', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    const { api } = mountHost(3, false)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('idle')
    expect(m.getDanmuSource).not.toHaveBeenCalled()
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

  it('弹幕库未加载 ⇒ providers 仍落库(按钮可显示) + failed 态', async () => {
    uninstallLib()
    m.getDanmuSource.mockResolvedValue(biliSrc)
    const { api } = mountHost(3)
    await flush()
    expect(api.providers.value).toHaveLength(1)
    expect(api.danmuState.value).toBe<DanmuState>('failed')
    expect(m.getDanmuBilibili).not.toHaveBeenCalled() // 库没建起, 不进装载
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
    expect(m.getDanmuSource).toHaveBeenLastCalledWith(4)
    expect(api.danmuState.value).toBe<DanmuState>('ready')
  })

  it('回归·一窝蜂: B站整集装载后强制 seek 对齐(防 timeupdate 已推进 lastPosition 导致补发)', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    const mgr = managerInstances[0]
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(mgr.load).toHaveBeenCalledTimes(1)
    // 顺序铁律: load → seek → time（time 之前必须 seek 重定位, 否则从 position=0 补发全部历史弹幕）
    const [loadOrder, seekOrder, timeOrder] = [
      mgr.load.mock.invocationCallOrder[0],
      mgr.seek.mock.invocationCallOrder[0],
      mgr.time.mock.invocationCallOrder[0]
    ]
    expect(loadOrder).toBeLessThan(seekOrder)
    expect(seekOrder).toBeLessThan(timeOrder)
  })

  it('回归·一窝蜂: 腾讯每段装载后均 seek 对齐(段数=load 次数, 对齐次数=段数+兜底)', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain()
    const { api } = mountHost(4)
    await flush()
    const mgr = managerInstances[0]
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(mgr.load).toHaveBeenCalledTimes(2) // segments 2 段
    // 每段 load 后 alignManager 一次 + loadTencentEpisode 收尾兜底一次
    expect(mgr.seek).toHaveBeenCalledTimes(3)
  })

  it('弹幕设置: 默认值 + updateSettings 实时应用到 manager/CSS + 持久化', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    const mgr = managerInstances[0]
    const stage = api.stage.value as HTMLElement
    // 默认(2026-10-26): 自适应字号(无 CSS 变量)、透明度 0.8、区域半屏、速度 2(比旧默认慢一倍)、
    // 同屏上限 30(0=不限改为压密度)、顶部/底部开、彩色、描边
    expect(stage.style.getPropertyValue('--jc-danmu-size')).toBe('')
    expect(stage.style.opacity).toBe('0.8')
    expect(stage.style.height).toBe('50%')
    expect(mgr.options.scroll?.scale).toBe(2)
    expect(mgr.options.limit).toBe(30)
    expect(mgr.filter.allowTypes?.['5']).toBe(true)
    expect(mgr.filter.allowTypes?.['4']).toBe(true)
    expect(stage.classList.contains('jc-danmaku-stage--no-outline')).toBe(false)
    // 用户调: 字号 24 + 白色 + 半屏 + 顶部弹幕关 + 描边关
    api.updateSettings({ fontSize: 24, colored: false, area: 0.5, topMode: false, outline: false })
    await flush()
    expect(stage.style.getPropertyValue('--jc-danmu-size')).toBe('24px')
    expect(stage.style.height).toBe('50%')
    expect(mgr.filter.allowTypes?.['5']).toBe(false)
    expect(mgr.filter.addModifier).toHaveBeenCalled() // 白色模式注册了颜色改写器
    expect(stage.classList.contains('jc-danmaku-stage--no-outline')).toBe(true)
    // 持久化
    expect(localStorage.getItem('jc-danmaku-settings')).toContain('"fontSize":24')
  })

  it('弹幕设置: 恢复自适应档字号 ⇒ 移除行内 CSS 变量(交给全屏/移动端 CSS)', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    const stage = api.stage.value as HTMLElement
    api.updateSettings({ fontSize: 28 })
    expect(stage.style.getPropertyValue('--jc-danmu-size')).toBe('28px')
    api.updateSettings({ fontSize: 0 })
    expect(stage.style.getPropertyValue('--jc-danmu-size')).toBe('')
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
    // 再开 → 命中 srcCache + biliCache, 零请求, 直接重载装载
    enabledRef.value = true
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    expect(managerInstances[0].load).toHaveBeenCalledTimes(2) // 重载重新装载了一次
  })

  it('请求去重: 配置(速度)切换重载命中缓存, 不重复发请求', async () => {
    m.getDanmuSource.mockResolvedValue(biliSrc)
    m.getDanmuBilibili.mockResolvedValue({ comments })
    const { api } = mountHost(3)
    await flush()
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1)
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    api.updateSettings({ speed: 3 }) // speedChanged → loadDanmaku 重载（已渲染弹幕按新速度重跑）
    await flush()
    expect(m.getDanmuSource).toHaveBeenCalledTimes(1) // 命中缓存, 零请求
    expect(m.getDanmuBilibili).toHaveBeenCalledTimes(1)
    expect(managerInstances[0].options.scroll?.scale).toBe(3)
    expect(managerInstances[0].load).toHaveBeenCalledTimes(2)
  })

  it('请求去重: 腾讯 seek 回已装区域命中段缓存, 不重复发请求', async () => {
    m.getDanmuSource.mockResolvedValue(tencentSrc)
    mockTencentChain()
    const { api, triggers } = mountHost(4)
    await flush()
    expect(api.danmuState.value).toBe<DanmuState>('ready')
    expect(m.getDanmuTencentComments).toHaveBeenCalledTimes(2) // 两段首次各拉一次
    // seeked: loadedSegs.clear 后重装, 段数据命中 segCache ⇒ 零新增请求
    triggers.seeked()
    await flush()
    expect(m.getDanmuTencentComments).toHaveBeenCalledTimes(2)
    expect(api.danmuState.value).toBe<DanmuState>('ready')
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
    expect(s.speed).toBe(2) // 非法档位回新默认
    expect(s.limit).toBe(30) // 负数回默认(同屏上限 30)
  })

  it('readDanmakuSettings: 旧速度档(0.5..1.5) ×2 迁移到新档(整体慢一倍)', () => {
    // 旧默认 1(正常) → 新 2(正常); 旧 0.5(最快) → 新 1(最快); 旧 1.5(最慢) → 新 3(最慢)
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

  it('readDanmakuSettings: 无记录返回新默认(opacity 0.8 / area 0.5 / speed 2)', () => {
    const s = readDanmakuSettings()
    expect(s.opacity).toBe(0.8)
    expect(s.area).toBe(0.5)
    expect(s.speed).toBe(2)
  })
})
