import { defineStore, storeToRefs } from 'pinia'
import { ref, toRaw, watch } from 'vue'
import { logger } from '@/utils/logger'
import { useUserStore } from './user'

/**
 * 收藏 store —— 登录态自动切换 远端 / 本地
 *
 * 设计同 history store:
 *  - 未登录: localStorage `filmFavorite` 持久化
 *  - 登录: GET /user/favorite 拉取; add/remove 走对应 POST/DELETE
 *  - 登录瞬间: 本地 push 上去再拉; 登出退回本地
 */

export interface FavoriteRecord {
  /** 影片 ID (mid 字符串形式) */
  id: string
  name: string
  picture?: string
  remarks?: string
  /** detail.descriptor.cName / 备注分类 */
  pid?: number
  cid?: number
  timeStamp: number
}

export type FavoriteMap = Record<string, FavoriteRecord>

const LS_KEY = 'filmFavorite'
const MAX_ITEMS = 200

function safeParse(raw: string): FavoriteMap {
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }
    const out: FavoriteMap = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (v && typeof v === 'object') {
        out[k] = v as FavoriteRecord
      }
    }
    return out
  } catch {
    return {}
  }
}

function loadFromLocal(): FavoriteMap {
  if (typeof localStorage === 'undefined') return {}
  try {
    return safeParse(localStorage.getItem(LS_KEY) ?? '')
  } catch {
    return {}
  }
}

function persistLocal(map: FavoriteMap): void {
  if (typeof localStorage === 'undefined') return
  const items = Object.values(map)
    .sort((a, b) => (b.timeStamp ?? 0) - (a.timeStamp ?? 0))
    .slice(0, MAX_ITEMS)
  const trimmed: FavoriteMap = {}
  for (const it of items) {
    trimmed[it.id] = { ...toRaw(it) }
  }
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(trimmed))
  } catch {
    /* ignore quota */
  }
}

export const useFavoriteStore = defineStore('favorite', () => {
  const map = ref<FavoriteMap>(loadFromLocal())
  const list = ref<FavoriteRecord[]>([])
  const remoteMode = ref(false)
  const remoteLoading = ref(false)

  function refreshList(): void {
    list.value = Object.values(map.value).sort(
      (a, b) => (b.timeStamp ?? 0) - (a.timeStamp ?? 0)
    )
  }

  refreshList()

  function isFavorited(id: string | number): boolean {
    return !!map.value[String(id)]
  }

  /** 添加收藏 (幂等; 已存在则刷新 timeStamp) */
  async function add(item: Omit<FavoriteRecord, 'timeStamp'> & { timeStamp?: number }): Promise<void> {
    if (!item.id) return
    const next: FavoriteRecord = {
      id: String(item.id),
      name: item.name,
      picture: item.picture,
      remarks: item.remarks,
      pid: item.pid,
      cid: item.cid,
      timeStamp: item.timeStamp ?? Date.now()
    }
    map.value[next.id] = next
    if (remoteMode.value) {
      try {
        const { add: addRemote } = await import('@/api/favorite')
        await addRemote({
          mid: Number(next.id) || 0,
          cid: next.cid ?? 0,
          pid: next.pid ?? 0,
          name: next.name,
          picture: next.picture ?? '',
          remarks: next.remarks ?? ''
        })
      } catch (e) {
        logger.warn('favorite add (remote) failed', e)
      }
    } else {
      persistLocal(map.value)
    }
    refreshList()
  }

  /** 取消收藏 */
  async function remove(id: string | number): Promise<void> {
    const key = String(id)
    if (!map.value[key]) return
    delete map.value[key]
    if (remoteMode.value) {
      try {
        const { remove: removeRemote } = await import('@/api/favorite')
        await removeRemote(key)
      } catch (e) {
        logger.warn('favorite remove (remote) failed', e)
      }
    } else {
      persistLocal(map.value)
    }
    refreshList()
  }

  /** 切换收藏 (用于详情页一键按钮) */
  async function toggle(item: Omit<FavoriteRecord, 'timeStamp'>): Promise<boolean> {
    if (isFavorited(item.id)) {
      await remove(item.id)
      return false
    }
    await add(item)
    return true
  }

  function fromRemote(r: import('@/types/favorite').RemoteFavoriteItem): FavoriteRecord {
    const mid = String(r.mid)
    const c = r.card
    return {
      id: mid,
      name: c?.name ?? '',
      picture: c?.cover ?? '',
      remarks: c?.remarks ?? '',
      pid: c?.pid ?? 0,
      cid: c?.cid ?? 0,
      timeStamp: r.createdAt ?? Date.now()
    }
  }

  async function syncFromRemote(): Promise<void> {
    remoteLoading.value = true
    try {
      const { list: listRemote } = await import('@/api/favorite')
      const resp = await listRemote({ page: 1, size: MAX_ITEMS })
      const next: FavoriteMap = {}
      for (const r of resp?.list ?? []) {
        const rec = fromRemote(r)
        next[rec.id] = rec
      }
      map.value = next
      refreshList()
    } catch (e) {
      logger.warn('favorite sync (remote) failed, keeping current map', e)
    } finally {
      remoteLoading.value = false
    }
  }

  async function mergeLocalIntoRemote(): Promise<void> {
    const locals = Object.values(map.value)
    if (locals.length === 0) return
    try {
      const { add: addRemote } = await import('@/api/favorite')
      for (const rec of locals) {
        const mid = Number(rec.id) || 0
        if (mid <= 0 || !rec.name) continue
        try {
          await addRemote({
            mid,
            cid: rec.cid ?? 0,
            pid: rec.pid ?? 0,
            name: rec.name,
            picture: rec.picture ?? '',
            remarks: rec.remarks ?? ''
          })
        } catch (e) {
          logger.warn('favorite merge (single) failed', rec.id, e)
        }
      }
    } catch (e) {
      logger.warn('favorite merge (remote) failed', e)
    }
  }

  const userStore = useUserStore()
  const { isLoggedIn } = storeToRefs(userStore)
  watch(
    isLoggedIn,
    async (now, prev) => {
      const firstRun = prev === undefined
      if (now && !prev) {
        if (!firstRun) {
          await mergeLocalIntoRemote()
        }
        remoteMode.value = true
        await syncFromRemote()
      } else if (!now && prev) {
        remoteMode.value = false
        map.value = loadFromLocal()
        refreshList()
      }
    },
    { immediate: true }
  )

  return {
    map,
    list,
    remoteMode,
    remoteLoading,
    isFavorited,
    add,
    remove,
    toggle,
    refresh: syncFromRemote
  }
})
