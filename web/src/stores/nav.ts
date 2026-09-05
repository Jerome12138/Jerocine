import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { NavCategory } from '@/types/film'

/**
 * 顶级导航分类
 * 仅在首次访问公开布局时拉取一次
 */
export const useNavStore = defineStore('nav', () => {
  const list = ref<NavCategory[]>([])
  const loaded = ref(false)
  const loading = ref(false)

  async function ensureLoaded(): Promise<void> {
    if (loaded.value || loading.value) {
      return
    }
    loading.value = true
    try {
      const { getCategories } = await import('@/api/film')
      list.value = await getCategories()
      loaded.value = true
    } finally {
      loading.value = false
    }
  }

  function findById(id: number): NavCategory | undefined {
    function walk(items: NavCategory[]): NavCategory | undefined {
      for (const it of items) {
        if (it.id === id) {
          return it
        }
        if (it.children?.length) {
          const found = walk(it.children)
          if (found) {
            return found
          }
        }
      }
      return undefined
    }
    return walk(list.value)
  }

  return { list, loaded, loading, ensureLoaded, findById }
})
