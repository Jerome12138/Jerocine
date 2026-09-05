/**
 * SWR (stale-while-revalidate) 内存缓存.
 *
 * 背景: 服务器在新加坡, 国内访问 /api/index 实测 ~7s. 页面无 KeepAlive,
 * 每次切回都重新 onMounted → 重新请求 → 再等数秒白屏. 本缓存让"切回已访问页"
 * 立即拿到上次结果, 后台静默刷新, 不再阻塞.
 *
 * 语义:
 *   - 命中且未过 freshMs: 直接返回缓存, 不发请求.
 *   - 命中但已过 freshMs: 立即返回旧数据, 同时后台 silent 刷新 (不闪全局 loading 条).
 *   - 未命中: 发请求 (显示 loading), 落缓存后返回.
 *   - 同 key 在途请求去重: 快速连点 / 快速切页不会重复打同一接口.
 *
 * 仅缓存「读多写少、可短暂陈旧」的内容接口 (首页 / 分类 / 详情);
 * 用户态、写操作、强一致数据不要走这里.
 */

interface Entry<T> {
  data: T
  ts: number
}

const store = new Map<string, Entry<unknown>>()
const inflight = new Map<string, Promise<unknown>>()

function revalidate<T>(key: string, fetcher: () => Promise<T>): Promise<T> {
  const p = fetcher()
    .then((data) => {
      store.set(key, { data, ts: Date.now() })
      return data
    })
    .finally(() => {
      inflight.delete(key)
    })
  inflight.set(key, p as Promise<unknown>)
  return p
}

/**
 * @param key      缓存键 (含参数, 如 `classify:1`)
 * @param fetcher  发起请求; 参数 silent=true 时应跳过全局 loading (后台刷新场景)
 * @param freshMs  新鲜窗口; 窗口内命中不再发请求. 默认 120s
 */
export function swrGet<T>(
  key: string,
  fetcher: (silent: boolean) => Promise<T>,
  freshMs = 120_000
): Promise<T> {
  const hit = store.get(key) as Entry<T> | undefined
  const pending = inflight.get(key) as Promise<T> | undefined

  if (hit) {
    // 已过新鲜期且无在途请求 → 后台静默刷新; 无论如何先立即返回旧数据
    if (Date.now() - hit.ts > freshMs && !pending) {
      void revalidate(key, () => fetcher(true)).catch(() => {
        /* 后台刷新失败保留旧数据, 不打扰用户 */
      })
    }
    return Promise.resolve(hit.data)
  }
  // 无缓存: 复用在途请求, 否则发起 (显示 loading)
  if (pending) return pending
  return revalidate(key, () => fetcher(false))
}

/** 失效缓存. 传前缀删匹配项, 不传清空全部. (如详情页发生互动后可 invalidateCache(`detail:${id}`)) */
export function invalidateCache(prefix?: string): void {
  if (!prefix) {
    store.clear()
    return
  }
  for (const k of [...store.keys()]) {
    if (k.startsWith(prefix)) store.delete(k)
  }
}
