import { ref, watch, type Ref } from 'vue'
import {
  useRoute,
  useRouter,
  type LocationQueryRaw,
  type LocationQueryValue
} from 'vue-router'

/**
 * useQuerySync —— 路由 query <-> 数据 ref 双向绑定
 *
 * 设计要点：
 *  - 单向同步：route.query → params（业务 ref）
 *  - 主动写：调用 push() 触发 router.push 更新 URL
 *  - skip：undefined / null / '' 字段不写入 URL，也不带回 params
 *  - 类型保留：传入的 initial 决定字段类型 — 数字字段读 query 时会按原类型转换
 *  - 不在 watch 内部 router.push 来响应 ref 变化（避免循环）
 *
 * 使用示例：
 *  const { params, push } = useQuerySync({ search: '', current: 1 })
 *  watch(params, fetchData, { deep: true, immediate: true })
 *  function onPaginate (p: number) { push({ ...params.value, current: p }) }
 *
 * @param initial 初始值（决定字段名 / 字段类型）
 * @param options.path  路由 path（默认当前路由 path）
 * @param options.onChange 当 params 因 URL 变化而被同步时触发（不会在 push 后再触发一次）
 */
export interface UseQuerySyncOptions<T> {
  path?: string
  onChange?: (params: T) => void
}

export interface UseQuerySyncReturn<T> {
  params: Ref<T>
  push: (next: Partial<T>) => Promise<void>
  /** 仅替换不入栈 */
  replace: (next: Partial<T>) => Promise<void>
}

type Primitive = string | number | boolean

function isSkippable(v: unknown): boolean {
  return v === undefined || v === null || v === ''
}

/** 将 query 原始值（string | string[] | null）按 initial 类型转换 */
function coerce<V>(raw: LocationQueryValue | LocationQueryValue[], sample: V): V {
  if (raw === undefined || raw === null) {
    return sample
  }
  const value = Array.isArray(raw) ? raw[0] : raw
  if (value === null || value === undefined) {
    return sample
  }
  if (typeof sample === 'number') {
    const n = Number(value)
    return (Number.isFinite(n) ? n : sample) as V
  }
  if (typeof sample === 'boolean') {
    return (value === 'true' || value === '1') as unknown as V
  }
  // 字符串保持原样
  return value as unknown as V
}

/** 把 params 拍平到 LocationQueryRaw（跳过空值） */
function flatten<T extends Record<string, Primitive | '' | undefined | null>>(
  src: T
): LocationQueryRaw {
  const out: LocationQueryRaw = {}
  for (const [k, v] of Object.entries(src)) {
    if (isSkippable(v)) continue
    out[k] = String(v)
  }
  return out
}

export function useQuerySync<
  T extends Record<string, Primitive | '' | undefined | null>
>(initial: T, options: UseQuerySyncOptions<T> = {}): UseQuerySyncReturn<T> {
  const route = useRoute()
  const router = useRouter()

  /** 内部静默标记：push 写出后跳过下一次 watch 回调（避免重复 onChange） */
  let suppressNext = false

  function readFromRoute(): T {
    const next = { ...initial }
    for (const key of Object.keys(initial) as Array<keyof T>) {
      const sample = initial[key]
      const rawV = route.query[key as string]
      if (rawV === undefined || rawV === null) {
        // 空字段 / 未提供 → 用 initial 中的默认（通常为 ''）
        next[key] = sample
        continue
      }
      next[key] = coerce(rawV, sample)
    }
    return next
  }

  const params = ref<T>(readFromRoute()) as Ref<T>

  watch(
    () => route.query,
    () => {
      const next = readFromRoute()
      // 浅比较：不变就不触发 onChange / ref 替换
      let changed = false
      for (const k of Object.keys(initial) as Array<keyof T>) {
        if (next[k] !== params.value[k]) {
          changed = true
          break
        }
      }
      if (!changed) return
      params.value = next
      if (suppressNext) {
        suppressNext = false
        return
      }
      options.onChange?.(next)
    },
    { deep: false }
  )

  async function push(next: Partial<T>): Promise<void> {
    const merged = { ...params.value, ...next } as T
    // 本地立即同步，避免 watch 触发再 onChange
    params.value = merged
    suppressNext = true
    await router.push({
      path: options.path ?? route.path,
      query: flatten(merged)
    })
  }

  async function replace(next: Partial<T>): Promise<void> {
    const merged = { ...params.value, ...next } as T
    params.value = merged
    suppressNext = true
    await router.replace({
      path: options.path ?? route.path,
      query: flatten(merged)
    })
  }

  return { params, push, replace }
}
