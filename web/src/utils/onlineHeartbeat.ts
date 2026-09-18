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
 *   - 30s 心跳 + 页面隐藏停发(后台标签页不算在线), 恢复可见立即续报
 *   - payload: sid + watching; 登录用户附 Bearer token, 服务端解析 uid 用于"同一用户去重"
 *   - 隐私: 不上报路径/页面标题(与 IP/用户组合即观看行为隐私), 后台不展示"在看什么"
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

function watchingNow(): boolean {
  return webWatching || Date.now() - nativeLastAt < NATIVE_WATCH_WINDOW_MS
}

async function send(): Promise<void> {
  if (!sid) return
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
        watching: watchingNow()
      }),
      credentials: 'omit',
      keepalive: true
    })
  } catch {
    /* 心跳失败静默, 不打扰播放 */
  }
}

function stopTimer(): void {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
}

function onVisibility(): void {
  if (document.hidden) {
    stopTimer()
  } else if (started) {
    void send()
    timer = window.setInterval(() => void send(), HB_INTERVAL_MS)
  }
}

/** App.vue 挂载时调用: 启动全站在线心跳。 */
export function startOnlineHeartbeat(): void {
  if (started) return
  started = true
  sid = readSid()
  if (!sid) return
  void send()
  timer = window.setInterval(() => void send(), HB_INTERVAL_MS)
  document.addEventListener('visibilitychange', onVisibility)
}

/** Web 播放器(video.js)播放状态: true=播放中, false=暂停/结束。 */
export function setWebWatching(v: boolean): void {
  webWatching = v
}

/** Native(TV ExoPlayer)播放活跃(playerProgress 事件到达)。 */
export function touchNativeWatching(): void {
  nativeLastAt = Date.now()
}
