import { ref, type Ref } from 'vue'

/**
 * 局部 loading composable —— 不与全局 UIStore 耦合
 *
 * 使用：
 *   const { loading, run } = useLoading()
 *   const data = await run(() => filmApi.getIndex())
 */
export function useLoading(): {
  loading: Ref<boolean>
  run: <T>(task: () => Promise<T>) => Promise<T>
} {
  const loading = ref(false)

  async function run<T>(task: () => Promise<T>): Promise<T> {
    loading.value = true
    try {
      return await task()
    } finally {
      loading.value = false
    }
  }

  return { loading, run }
}
