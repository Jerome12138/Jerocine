import { ref, computed, type Ref, type ComputedRef } from 'vue'

/**
 * 本地点赞 composable.
 *
 * 后端目前没有点赞接口, 暂以 localStorage 持久化 "影片 id -> liked" 映射.
 * 单独抽出来是为了:
 *  - PlayView / FilmDetailView 可复用
 *  - 单测无视图依赖, 不必 mount video.js / pinia
 *
 * 不和 favoriteStore 合并, 因为收藏是真后端 (登录态绑定), 点赞是匿名本地态.
 */

const STORAGE_KEY = 'gf-likes'

function read(): Record<string, true> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as Record<string, true>) : {}
  } catch {
    return {}
  }
}

function write(map: Record<string, true>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map))
  } catch {
    /* 隐私模式忽略 */
  }
}

export interface LocalLikes {
  isLiked: (id: string | number) => ComputedRef<boolean>
  toggle: (id: string | number) => void
  /** 内部 map, 测试可用 */
  _map: Ref<Record<string, true>>
}

export function useLocalLikes(): LocalLikes {
  const map = ref<Record<string, true>>(read())
  return {
    _map: map,
    isLiked(id) {
      const key = String(id)
      return computed(() => !!map.value[key])
    },
    toggle(id) {
      const key = String(id)
      const next = { ...map.value }
      if (next[key]) delete next[key]
      else next[key] = true
      map.value = next
      write(next)
    }
  }
}
