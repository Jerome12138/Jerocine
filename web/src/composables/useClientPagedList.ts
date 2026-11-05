import { computed, ref, watch, type Ref } from 'vue'

/**
 * 前端切片分页状态收敛（§C8#3）: 数据已整表在前端的列表（Dashboard 在线明细 /
 * LiveSource 频道面板）共享「page ref + slice 切片 + 数据收缩末页钳制」样板。
 *
 * 行为口径（与原两处手写逐字等价, 钳制为 LiveSourceListView 原缺失项本次顺带修复）:
 *  - paged = source 切片 [(page-1)*size, page*size)
 *  - 越界钳制: source 收缩（筛选/删除）后 page 超出末页 → 回到最后一个有效页（≥1, 不落 0）
 *  - 回第 1 页由宿主在筛选 watch 里调 reset()（与原两处 watch([filters], () => page=1) 一致）
 *
 * （区别于 usePagedList: 那是服务端分页请求收敛; 本件不做请求。）
 */
export interface ClientPagedListState<T> {
  /** 当前页（1 基, 可 v-model:current 直绑 BasePagination） */
  page: Ref<number>
  /** 当前页切片 */
  paged: Ref<T[]>
  /** 总条数（= source 长度） */
  total: Ref<number>
  /** 总页数（向上取整, 空表 = 1） */
  pageCount: Ref<number>
  /** 回到第 1 页（筛选条件变化时宿主调用） */
  reset: () => void
}

export function useClientPagedList<T>(
  /** 整表数据源: ref/reactive 数组 ref 或 getter（可传 computed 的 .value getter） */
  source: Ref<T[]> | (() => T[]),
  /** 每页条数（静态, 两处原款均为常量 10/100） */
  pageSize: number
): ClientPagedListState<T> {
  const page = ref(1)

  const list = computed<T[]>(() => (typeof source === 'function' ? source() : source.value))

  const paged = computed<T[]>(() => {
    const start = (page.value - 1) * pageSize
    return list.value.slice(start, start + pageSize)
  })

  const total = computed(() => list.value.length)
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

  // 越界钳制: 数据收缩后 page 超出末页 → 回到最后一个有效页
  // (原 DashboardView 手写同款; LiveSourceListView 缺此 watch → 筛选变窄后停在越界页看空表, 本次顺带修)
  watch(list, (l) => {
    const max = Math.max(1, Math.ceil(l.length / pageSize))
    if (page.value > max) page.value = max
  })

  function reset(): void {
    page.value = 1
  }

  return { page, paged, total, pageCount, reset }
}
