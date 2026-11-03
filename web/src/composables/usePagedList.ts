import { ref, watch, type Ref } from 'vue'

/**
 * 分页列表状态收敛: load/search/翻页/末页回夹/请求序号守卫/loading/loadError
 * 的样板在此单点实现, 列表页只提供 fetch 闭包（筛选参数由闭包捕获）。
 *
 * 行为口径（manage-ui-spec §2.3）:
 *  - current 的 watch 是唯一加载触发器; search() = 回第 1 页（已在第 1 页则直接 load）
 *  - 请求序号守卫: 仅采纳最后一次请求结果, 旧响应后到丢弃
 *  - 末页回夹: 当前页超出总页数且仍有数据 → current 回到最后一个有效页（watch 自动重拉）
 *  - 取消类错误（axios CanceledError / isCancel）静默, 不落 loadError
 */

export interface PagedFetchResult<T> {
  list: T[]
  total: number
}

export interface PagedListOptions<T> {
  /** 拉取函数: 页码与页大小由 usePagedList 传入; 筛选参数由调用方闭包捕获 */
  fetch: (page: number, pageSize: number) => Promise<PagedFetchResult<T>>
  /** 初始页大小, 默认 20 */
  pageSize?: number
  /** 是否立即 load(), 默认 true */
  immediate?: boolean
}

export interface PagedListState<T> {
  rows: Ref<T[]>
  total: Ref<number>
  current: Ref<number>
  pageSize: Ref<number>
  loading: Ref<boolean>
  /** 失败信息文案（= Error.message）, 成功时清空 */
  loadError: Ref<string>
  /** 关键词等条件变化后调用: 回第 1 页并加载 */
  search: () => void
  /** 按当前页码加载（重试 = 直接调它） */
  load: () => Promise<void>
}

/** 取消类错误静默（与 useAsyncAction 口径一致） */
function isCanceledError(e: unknown): boolean {
  const err = e as { name?: string; isCancel?: boolean; code?: string }
  return err.name === 'CanceledError' || err.isCancel === true || err.code === 'ERR_CANCELED'
}

export function usePagedList<T>(options: PagedListOptions<T>): PagedListState<T> {
  const pageSize = ref(options.pageSize ?? 20)
  const rows = ref<T[]>([]) as Ref<T[]>
  const total = ref(0)
  const current = ref(1)
  const loading = ref(false)
  const loadError = ref('')

  let loadSeq = 0

  async function load(): Promise<void> {
    const my = ++loadSeq
    loading.value = true
    try {
      const resp = await options.fetch(current.value, pageSize.value)
      if (my !== loadSeq) return // 旧响应后到, 丢弃
      rows.value = resp.list
      total.value = resp.total
      loadError.value = ''
      // 末页回夹: 本页已空且当前页超出总页数（数据被删空后停在越界页）
      const pageCount = Math.ceil(resp.total / pageSize.value)
      if (resp.list.length === 0 && current.value > 1 && resp.total > 0 && pageCount < current.value) {
        current.value = pageCount // watch(current) 触发自动重拉
      }
    } catch (e) {
      if (my !== loadSeq) return
      if (!isCanceledError(e)) {
        loadError.value = (e as Error)?.message ?? '加载失败'
      }
    } finally {
      if (my === loadSeq) loading.value = false
    }
  }

  function search(): void {
    if (current.value === 1) {
      void load()
    } else {
      current.value = 1 // watch(current) 触发 load, 避免双请求
    }
  }

  watch(current, () => {
    void load()
  })

  if (options.immediate !== false) {
    void load()
  }

  return { rows, total, current, pageSize, loading, loadError, search, load }
}
