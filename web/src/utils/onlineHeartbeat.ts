/**
 * onlineHeartbeat - 在线/观看心跳上报(匿名).
 *
 * 设计:
 *   - 模块级单例: App.vue 挂载时 start(), 全站只跑一个定时器
 *   - sid 按设备生成(localStorage 持久化; 隐私模式回退内存随机, 刷新换新)
 *   - watching 双信号源:
 *       Web 播放器(video.js)  → setWebWatching(播放/暂停即时置位)
 *       Native(TV ExoPlayer) → touchNativeWatching(playerProgress 事件到达,
 *                              以"最近一次在 15s 内"判观看中, 事件 5s/tick)
 *   - 30s 心跳, 定时器不停: 页面不可见时"非观看中"跳过上报(后台标签页不算在线),
 *     观看中(后台听视频 / 原生播放器盖住 WebView)照常上报 ——
 *     安卓原生播放是独立 Activity 盖在 WebView 上, WebView 的 document.hidden 会变 true,
 *     若像旧版一样"隐藏即停表"播放期间就一发心跳都没有了
 *   - 恢复可见立即续报
 *   - PV: 首次载入与每次路由切换各计一次页面访问(心跳带 pv=1, 服务端不去重);
 *     同一页面停留期间的常规心跳不带 pv(服务端按路径变化兜底)
 *   - payload: sid + watching + path(pathname 仅路径, 不含 query/影片标识) + webdriver;
 *     登录用户附 Bearer token, 服务端解析 uid 用于"同一用户去重"
 *   - webdriver = navigator.webdriver 原值: 无头浏览器(headless/自动化)默认 true,
 *     是服务端识别机器人的信号之一(正常浏览器恒为 false/undefined)
 *   - 隐私: 只上报页面路径类别(如 /play、/detail), 不上报任何参数/影片标识,
 *     后台可分析页面分布但不识别"正在看哪部影片"
 *   - 上报失败静默, 不打扰播放主流程
 */

import { getToken } from '@/utils/token'

const HB_KEY = 'jc-online-sid'
const HB_INTERVAL_MS = 30_000
const HB_ENDPOINT = '/api/v1/online/heartbeat'
/** Native 播放: 最近一次 playerProgress 在 15s 内视为观看中(原生事件 5s/tick) */
const NATIVE_WATCH_WINDOW_MS = 15_000

function readSid(): string {
  try {
    let sid = localStorage.getItem(HB_KEY)
    if (!sid) {
      sid = crypto.randomUUID
        ? crypto.randomUUID()
        : `sid-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
      localStorage.setItem(HB_KEY, sid)
    }
    return sid
  } catch {
    return `sid-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  }
}

let sid = ''
let timer: number | undefined
let started = false
let webWatching = false
let nativeLastAt = 0
/** 下一次心跳是否对应一次页面访问(首载/路由切换置位, 发送即消费) */
let pendingPV = false

function watchingNow(): boolean {
  return webWatching || Date.now() - nativeLastAt < NATIVE_WATCH_WINDOW_MS
}

/** 页面不可见且非观看中 → 不算在线, 跳过本次上报 */
function shouldSkip(): boolean {
  return typeof document !== 'undefined' && document.hidden && !watchingNow()
}

async function send(): Promise<void> {
  if (!sid || shouldSkip()) return
  const pv = pendingPV
  pendingPV = false
  const token = getToken()
  try {
    await fetch(HB_ENDPOINT, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {})
      },
      body: JSON.stringify({
        sid,
        watching: watchingNow(),
        // 仅路径(不含 query/hash/影片标识): 后台看页面分布, 不识别具体影片
        path: typeof window !== 'undefined' ? window.location.pathname : '',
        // 页面访问标记: 首载/路由切换置位
        pv,
        // 无头浏览器自报(true)时服务端直接判机器人; 正常浏览器为 false
        webdriver: typeof navigator !== 'undefined' && navigator.webdriver === true
      }),
      credentials: 'omit',
      keepalive: true
    })
  } catch {
    /* 心跳失败静默, 不打扰播放 */
  }
}

function onVisibility(): void {
  // 仅"恢复可见"立即续报; 转隐藏不再停表(观看中仍需持续上报, 见文件头说明)
  if (!document.hidden && started) void send()
}

/** App.vue 挂载时调用: 启动全站在线心跳。首次心跳即一次页面访问。 */
export function startOnlineHeartbeat(): void {
  if (started) return
  started = true
  sid = readSid()
  if (!sid) return
  pendingPV = true // 首次载入 = 一次页面访问
  void send()
  timer = window.setInterval(() => void send(), HB_INTERVAL_MS)
  document.addEventListener('visibilitychange', onVisibility)
}

/** Web 播放器(ArtPlayer)播放状态: true=播放中, false=暂停/结束。 */
export function setWebWatching(v: boolean): void {
  webWatching = v
}

/** Native(TV ExoPlayer)播放活跃(playerProgress 事件到达)。 */
export function touchNativeWatching(): void {
  nativeLastAt = Date.now()
}

/** 页面访问打点(App.vue 的 router.afterEach 调用): 立即上报一次带 pv=1 的心跳。 */
export function trackPageview(): void {
  if (!started) return
  pendingPV = true
  void send()
}

/** 供测试/卸载使用: 停掉定时器(页面生命周期内通常不需要)。 */
export function stopOnlineHeartbeat(): void {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
  started = false
  document.removeEventListener('visibilitychange', onVisibility)
}
