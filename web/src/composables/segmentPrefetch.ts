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
 * 锚点（2026-11-25 线上修复）：
 *  **以"hls 正在加载的分片"为基准**（FRAG_LOADING），预取它后面的 N 个 —— 而不是以"正在播放的分片"
 *  为基准（FRAG_CHANGED）。后者永远落在 hls 已经缓冲好的区间里，白下流量；线上表现就是
 *  "刚开开关时并发了一下（全打在已缓冲分片上），之后一直是一个一个地拉"。
 *
 * 安全设计（四条）：
 *  1. **自检可缓存性**：首次预取后检查 `Cache-Control`：
 *     - `no-store` ⇒ 完全不缓存，预取只会让流量翻倍 ⇒ **自我禁用**；
 *     - `no-cache` / `private` ⇒ **允许**（浏览器仍会存；`no-cache` 只是要重新校验，
 *       源站带 ETag 时 hls.js 拿到 304，只多一个 RTT，body 不必重下）。
 *     早期版本把 `no-cache` 也当成不可缓存而永久禁用，导致大量源站"只并发一次就退化"。
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
  /** 告知 hls **正在加载**的分片下标 —— 由 FRAG_LOADING 驱动；发生跳转/换集时取消在途预取 */
  onLoading(index: number): void
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

/** 单个预取请求的最长等待：超时即中止，避免 inflight 卡死把后续预取全堵住 */
const PREFETCH_TIMEOUT_MS = 30000

export function createSegmentPrefetcher(opts: Opts): SegmentPrefetcher {
  let ahead = Math.max(0, opts.ahead)
  const maxInflight = opts.maxInflight ?? 3
  const maxEntries = opts.maxEntries ?? 12

  let fragments: PrefetchFragment[] = []
  let playing = -1
  /** 已经预取到的最远分片下标：避免连续加载时把同一个后续分片重复预取 */
  let prefetchedUpTo = -1
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

  /**
   * 发起一个预取，返回"该分片是否已被覆盖"：
   *   true  = 本次发起成功，或已在途（pending 命中）；
   *   false = 因禁用/并发上限没发起 —— 调用方**不能**推进水位，否则这个分片永远补不上。
   */
  function prefetch(url: string): boolean {
    if (disabled) return false
    if (pending.has(url)) return true
    if (inflight >= maxInflight) return false
    const ctrl = new AbortController()
    controllers.set(url, ctrl)
    inflight += 1
    const timer = window.setTimeout(() => ctrl.abort(), PREFETCH_TIMEOUT_MS)
    const p = fetch(url, { credentials: 'omit', signal: ctrl.signal })
      .then((res) => {
        // 自检：只有 no-store 才值得禁用（no-cache/private 浏览器仍会存，最多多一次 304 校验）
        const cc = (res.headers.get('cache-control') || '').toLowerCase()
        if (cc.includes('no-store')) {
          disabled = true
          abortAll()
          return
        }
        return res.arrayBuffer().then(() => undefined)
      })
      .catch(() => undefined) // 含 AbortError：取消是预期行为，不视为错误
      .finally(() => {
        window.clearTimeout(timer)
        inflight = Math.max(0, inflight - 1)
        pending.delete(url)
        controllers.delete(url)
        // 腾出并发位就立刻补下一个（disabled 时 pump 内部直接返回）
        pump()
      })
    pending.set(url, p)
    trim()
    return true
  }

  function pump(): void {
    if (disabled || ahead <= 0 || playing < 0 || fragments.length === 0) return
    for (let k = 1; k <= ahead; k += 1) {
      const idx = playing + k
      const f = fragments[idx]
      if (!f?.url) continue
      // 已经在更早一轮预取过 ⇒ 跳过（否则 ahead=2 时每个分片会被预取两次）
      if (idx <= prefetchedUpTo) continue
      if (prefetch(f.url)) prefetchedUpTo = idx
    }
  }

  return {
    onLevel(list: PrefetchFragment[]): void {
      fragments = Array.isArray(list) ? list : []
      // 换 level（清晰度切换）⇒ 旧 level 的分片 URL 基本作废，取消在途，预取水位清零
      abortAll()
      prefetchedUpTo = -1
      pump()
    },
    onLoading(index: number): void {
      // 非连续跳变 = 用户 seek / 换集 / 换清晰度 ⇒ 之前预取的分片多半已被跳过，取消以省流量
      // 往前大幅跳 / 任何往回退 ⇒ 之前的水位不再适用
      if (playing >= 0 && (index < playing || Math.abs(index - playing) > ahead + 1)) {
        abortAll()
        prefetchedUpTo = -1
      }
      playing = index
      pump()
    },
    setAhead(n: number): void {
      const next = Math.max(0, n)
      if (next === ahead) return
      ahead = next
      prefetchedUpTo = -1 // 档位变了：按新深度重新算
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
