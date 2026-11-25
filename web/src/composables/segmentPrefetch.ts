/**
 * HLS 分片并发预取（"预热 + 在途合并"方案）· 2026-11-25
 *
 * 背景：hls.js 的分片加载天生是**顺序**的（同一时刻只跑一个分片请求），遇到高 RTT / 限速
 * 就会"拉一段、停一下"。hls.js 1.7.4 提供 `fLoader` 可替换分片加载器，但完整接管加载链路
 * （回调契约/abort/生命周期）风险很高。
 *
 * 本模块选择**附加式**方案：不改 hls.js 的加载路径，只在旁边并发预取**接下来的 N 个分片**，
 * 使 hls.js 真正去取时命中浏览器 HTTP 缓存（若源站/CDN 允许缓存）。
 *
 * 安全设计（四条）：
 *  1. **自检可缓存性**：首次预取后检查 `Cache-Control`；出现 `no-store`/`no-cache`/`private`
 *     直接**自我禁用**，避免"预取一遍 + hls.js 再下一遍"把流量翻倍。
 *  2. **在途合并**：同一 URL 只发一个请求（Map<url, Promise>），重复触发不重复下载。
 *  3. **可取消**（2026-11-25 评审补充）：每个请求都带 AbortController；**用户跳转/换 level/销毁/
 *     预取被裁剪**时立即 abort —— 否则会继续下载"用户已经跳过的分片"，纯浪费带宽。
 *  4. **有界**：并发上限 + 队列上限；被裁剪的条目会**连带取消**其底层请求，使 maxEntries 真正生效。
 *
 * 风险边界（如实标注）：若源站返回 `no-store`/`private`，本模块自行关闭，此时收益为 0
 * （但也不会造成额外流量）。
 */

export interface PrefetchFragment {
  url: string
}

export interface SegmentPrefetcher {
  /** 告知当前分片列表 —— 由 hls.js 的 LEVEL_LOADED 驱动；换 level 会取消在途预取 */
  onLevel(fragments: PrefetchFragment[]): void
  /** 告知正在播的分片下标 —— 由 FRAG_CHANGED 驱动；发生跳转（seek）时取消在途预取 */
  onPlaying(index: number): void
  /** 档位变化时调整预取深度（0 = 清空并关闭） */
  setAhead(n: number): void
  /** 该会话是否仍在预取（自检未禁用） */
  readonly active: boolean
  destroy(): void
}

interface Opts {
  /** 预取深度（接下来 N 个分片）；0 = 关闭 */
  ahead: number
  /** 并发上限 */
  maxInflight?: number
  /** 在途合并缓存上限（超出即裁剪并取消） */
  maxEntries?: number
}

export function createSegmentPrefetcher(opts: Opts): SegmentPrefetcher {
  let ahead = Math.max(0, opts.ahead)
  const maxInflight = opts.maxInflight ?? 3
  const maxEntries = opts.maxEntries ?? 12

  let fragments: PrefetchFragment[] = []
  let playing = -1
  let disabled = false
  let inflight = 0
  const pending = new Map<string, Promise<void>>()
  const controllers = new Map<string, AbortController>()

  /** 取消单个条目（从两张表移除 + abort 底层请求） */
  function drop(url: string, doAbort: boolean): void {
    if (pending.delete(url) && doAbort) {
      controllers.get(url)?.abort()
    }
    controllers.delete(url)
  }

  /** 全部取消：换 level / 跳转 / 销毁 / 关闭档位时调用 */
  function abortAll(): void {
    for (const c of controllers.values()) c.abort()
    controllers.clear()
    pending.clear()
  }

  function trim(): void {
    while (pending.size > maxEntries) {
      const first = pending.keys().next().value
      if (first === undefined) break
      // 裁剪时**必须连带取消**，否则 maxEntries 只约束了 Map 大小、约束不了在途流量
      drop(first, true)
    }
  }

  function prefetch(url: string): void {
    if (disabled || inflight >= maxInflight || pending.has(url)) return
    const ctrl = new AbortController()
    controllers.set(url, ctrl)
    inflight += 1
    const p = fetch(url, { credentials: 'omit', signal: ctrl.signal })
      .then((res) => {
        // 自检：源站明确不允许缓存 ⇒ 预取无意义且会双倍流量，立即禁用
        const cc = (res.headers.get('cache-control') || '').toLowerCase()
        if (cc.includes('no-store') || cc.includes('no-cache') || cc.includes('private')) {
          disabled = true
          abortAll()
          return
        }
        return res.arrayBuffer().then(() => undefined)
      })
      .catch(() => undefined) // 含 AbortError：取消是预期行为，不视为错误
      .finally(() => {
        inflight = Math.max(0, inflight - 1)
        pending.delete(url)
        controllers.delete(url)
      })
    pending.set(url, p)
    trim()
  }

  function pump(): void {
    if (disabled || ahead <= 0 || playing < 0 || fragments.length === 0) return
    for (let k = 1; k <= ahead; k += 1) {
      const f = fragments[playing + k]
      if (!f?.url) continue
      prefetch(f.url)
    }
  }

  return {
    onLevel(list: PrefetchFragment[]): void {
      fragments = Array.isArray(list) ? list : []
      // 换 level（清晰度切换）⇒ 旧 level 的分片 URL 基本作废，取消在途
      abortAll()
      pump()
    },
    onPlaying(index: number): void {
      // 非连续前进 = 用户 seek/跳集 ⇒ 之前预取的分片多半已被跳过，取消以省流量
      if (playing >= 0 && Math.abs(index - playing) > ahead + 1) {
        abortAll()
      }
      playing = index
      pump()
    },
    setAhead(n: number): void {
      ahead = Math.max(0, n)
      if (ahead === 0) {
        abortAll() // 已含 clear
        return
      }
      pump()
    },
    get active(): boolean {
      return !disabled && ahead > 0
    },
    destroy(): void {
      abortAll()
      fragments = []
      playing = -1
      disabled = true
    }
  }
}
