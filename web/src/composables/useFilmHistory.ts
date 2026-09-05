import { onBeforeUnmount, onMounted } from 'vue'
import { useHistoryStore, type HistoryRecord } from '@/stores/history'

/**
 * 观看历史 composable
 *
 * - 负责 PlayView 在卸载 / beforeunload 时调用 store.record(...)
 * - 提供 buildPlayLink 工具，统一拼装 /play?... query
 *
 * 注：核心存储逻辑已封装在 `useHistoryStore`（cookie + localStorage 双写）。
 * composable 仅做 PlayView 的"自动写入"生命周期粘合。
 */

export interface BuildLinkArgs {
  id: string | number
  source: string
  episodeIndex: number
  currentTime?: number
}

export function buildPlayLink({
  id,
  source,
  episodeIndex,
  currentTime = 0
}: BuildLinkArgs): string {
  const params = new URLSearchParams()
  params.set('id', String(id))
  params.set('source', source)
  params.set('episode', String(episodeIndex))
  if (currentTime > 0) {
    params.set('currentTime', String(Math.floor(currentTime)))
  }
  return `/play?${params.toString()}`
}

export interface UseFilmHistoryOptions {
  /** 拿到当前快照的回调，返回 null 则跳过本次写入 */
  collect: () => Omit<HistoryRecord, 'timeStamp'> | null
}

export interface UseFilmHistoryReturn {
  /** 立即写一次（用于切换 episode、组件卸载） */
  flush: () => void
  /** 列表（与 store 同源） */
  list: ReturnType<typeof useHistoryStore>['list']
}

export function useFilmHistory(opts: UseFilmHistoryOptions): UseFilmHistoryReturn {
  const store = useHistoryStore()

  function flush(): void {
    const snap = opts.collect()
    if (!snap) {
      return
    }
    store.record(snap)
  }

  // beforeunload 兜底（关闭 / 刷新 / 跳外部链接）
  function handleBeforeUnload(): void {
    flush()
  }

  onMounted(() => {
    if (typeof window !== 'undefined') {
      window.addEventListener('beforeunload', handleBeforeUnload)
    }
  })

  onBeforeUnmount(() => {
    flush()
    if (typeof window !== 'undefined') {
      window.removeEventListener('beforeunload', handleBeforeUnload)
    }
  })

  return { flush, list: store.list }
}
