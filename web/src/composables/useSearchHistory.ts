import { ref, type Ref } from 'vue'

/**
 * 搜索历史 composable.
 *
 * 单独抽出来:
 *  - SearchView 用 (引导态展示)
 *  - PublicHeader 顶栏搜索框未来可复用
 *  - 单测无视图依赖
 *
 * 行为:
 *  - 最多 LIMIT 条 (默认 10)
 *  - add 时去重 + 移到最前
 *  - 大小写不敏感比较, 但保留原大小写
 *  - localStorage 持久化, 隐私模式静默降级 (内存态仍可用)
 */

const STORAGE_KEY = 'gf-search-history'
const DEFAULT_LIMIT = 10

function read(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((s): s is string => typeof s === 'string') : []
  } catch {
    return []
  }
}

function write(arr: string[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(arr))
  } catch {
    /* 隐私模式忽略 */
  }
}

export interface SearchHistory {
  history: Ref<string[]>
  add: (kw: string) => void
  remove: (kw: string) => void
  clear: () => void
}

export function useSearchHistory(limit = DEFAULT_LIMIT): SearchHistory {
  const history = ref<string[]>(read())

  function add(kw: string): void {
    const trimmed = kw.trim()
    if (!trimmed) return
    // 去重: 大小写不敏感对比, 保留新输入的大小写
    const lower = trimmed.toLowerCase()
    const filtered = history.value.filter((h) => h.toLowerCase() !== lower)
    const next = [trimmed, ...filtered].slice(0, limit)
    history.value = next
    write(next)
  }

  function remove(kw: string): void {
    const lower = kw.toLowerCase()
    const next = history.value.filter((h) => h.toLowerCase() !== lower)
    if (next.length === history.value.length) return
    history.value = next
    write(next)
  }

  function clear(): void {
    history.value = []
    write([])
  }

  return { history, add, remove, clear }
}
