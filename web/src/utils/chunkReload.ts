/**
 * chunk 加载失败自愈(带一次性闸门).
 *
 * 现状(改造前, `router/index.ts` 内联逻辑): 新 deploy 把 dist/assets 里所有 chunk 换名,
 * 而 WebView 缓存里的 index.html 还指向旧 hash ⇒ 懒加载 component 取旧 chunk → 404
 * (nginx `/assets/` 是 `try_files $uri =404`) → "'text/html' is not a valid JavaScript
 * MIME type" / "Failed to fetch dynamically imported module". 一旦命中就 location.reload()
 * 拿新 index.html + 新 chunk URL, 错误自愈。
 *
 * 引入 Service Worker 后**必须加闸门**(方案 §4.5, 否则线上必现死循环):
 *   SW 接管后 `location.reload()` 会命中 **precache 里的旧 index.html**(仍是旧 chunk 引用)
 *   ⇒ 再次 404 ⇒ 再次 reload ⇒ 成环。故用 sessionStorage 标记"本会话只自愈一次"。
 *
 * 另外, 刷新前先让 SW `registration.update()` 主动检查更新(拿到新 manifest) 再刷,
 * 最大化"一次刷新即拿到新版本"的成功率; 网络慢时用 1.5s 兜底定时器保证仍会刷新。
 *
 * 逻辑抽成纯函数 + 依赖注入, 便于单测(见 chunkReload.spec.ts)。
 */

/** 一次性闸门标记键 (sessionStorage) */
export const CHUNK_RELOAD_GUARD_KEY = 'jc-chunk-reload-once'
/** SW update 迟迟不回来时的兜底刷新延时(ms) */
export const CHUNK_RELOAD_FALLBACK_MS = 1500

export type ChunkReloadOutcome = 'reloaded' | 'guarded' | 'ignored'

/** 识别"动态 import 失败 / 拿到 HTML 当 JS"这类 chunk 加载错误 */
export function isChunkLoadError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err)
  return (
    msg.includes('dynamically imported module') ||
    msg.includes('text/html') ||
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed')
  )
}

export interface ChunkReloadDeps {
  /** 一次性闸门存储(通常 sessionStorage) */
  guardStore: Pick<Storage, 'getItem' | 'setItem'>
  /** 硬刷新页面 */
  reload: () => void
  /** 取 SW 注册(可能同步返回 undefined / Promise, 也可能抛错) */
  getRegistration: () => Promise<{ update: () => Promise<unknown> } | undefined> | undefined
  /** 埋点 */
  track: (err: unknown, category: string, extra?: Record<string, unknown>) => void
  /** 定时器(注入便于测试) */
  setTimer: (fn: () => void, ms: number) => void
}

/**
 * 处理 chunk 加载失败.
 * @returns 'ignored'  非 chunk 错误, 不处理
 *          'guarded'  本会话已自愈过一次, 只上报不刷新(防死循环)
 *          'reloaded' 已安排刷新(先 SW update 再刷, 带兜底)
 */
export function handleChunkLoadError(err: unknown, deps: ChunkReloadDeps): ChunkReloadOutcome {
  if (!isChunkLoadError(err)) return 'ignored'

  const msg = err instanceof Error ? err.message : String(err)

  // 闸门: 本会话已自愈过 → 只上报, 不再刷(避免 SW 接管后的 reload 死循环)
  try {
    if (deps.guardStore.getItem(CHUNK_RELOAD_GUARD_KEY)) {
      deps.track(err, 'chunk-load-reload-guarded', { msg })
      return 'guarded'
    }
    deps.guardStore.setItem(CHUNK_RELOAD_GUARD_KEY, '1')
  } catch {
    // sessionStorage 不可用(隐私模式等): 保守起见当作"已闸门", 不复刷
    deps.track(err, 'chunk-load-reload-guarded', { msg, reason: 'guard-store-unavailable' })
    return 'guarded'
  }

  deps.track(err, 'chunk-load-reload', { msg })

  let fired = false
  const fire = (): void => {
    if (fired) return
    fired = true
    // 留 50ms 给 telemetry flush, 再硬刷
    deps.setTimer(() => deps.reload(), 50)
  }

  let reg: Promise<{ update: () => Promise<unknown> } | undefined> | undefined
  try {
    reg = deps.getRegistration()
  } catch {
    reg = undefined
  }

  if (reg && typeof reg.then === 'function') {
    reg
      .then((r) => (r ? r.update() : undefined))
      .catch(() => {})
      .finally(fire)
    // 兜底: update 卡住时也要刷
    deps.setTimer(fire, CHUNK_RELOAD_FALLBACK_MS)
  } else {
    fire()
  }

  return 'reloaded'
}
