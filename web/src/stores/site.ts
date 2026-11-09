import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { SiteConfig } from '@/types/config'
import { imgFallbackHosts } from '@/utils/imgHosts'
import { setBlobBase } from '@/utils/blobImg'

/**
 * 站点信息（siteName / logo 等），仅首次加载，全应用共享
 */
export const useSiteStore = defineStore('site', () => {
  const basic = ref<SiteConfig | null>(null)
  const loaded = ref(false)
  const loading = ref(false)

  async function ensureLoaded(): Promise<void> {
    if (loaded.value || loading.value) {
      return
    }
    loading.value = true
    try {
      const { getSiteConfig } = await import('@/api/config')
      basic.value = await getSiteConfig()
      // 图床域名链同步进模块级单供体(BaseImage 消费, 不走 pinia —— 见 utils/imgHosts.ts)
      imgFallbackHosts.value = basic.value?.imgFallbackHosts ?? ''
      // blob 对外基址同步进模块级单供体(blobImg 消费): 路径模式下据此识别前缀, 绝对域模式出参已是绝对 URL
      setBlobBase(basic.value?.fileBase)
      loaded.value = true
    } finally {
      loading.value = false
    }
  }

  function reset(): void {
    basic.value = null
    loaded.value = false
    imgFallbackHosts.value = ''
    setBlobBase('')
  }

  return { basic, loaded, loading, ensureLoaded, reset }
})
