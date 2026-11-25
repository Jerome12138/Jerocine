<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageButton from '@/components/manage/ManageButton.vue'
import type { DomainSource } from '@/api/manage/system'

/**
 * 立即拉取公告 —— 列出后端登记的来源，点哪个拉哪个。
 *
 * 来源清单、字段口径、解析规则全部**后端维护**，前端不做任何配置。
 * 拉取只写规则表，不动任何影片数据。
 */
const emit = defineEmits<{
  (e: 'confirm', ok: boolean): void
}>()

const sources = ref<DomainSource[]>([])
const loading = ref(true)
const busyKey = ref('')

async function loadSources(): Promise<void> {
  loading.value = true
  try {
    const r = await manageApi.system.listDomainSources()
    sources.value = r.list ?? []
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '来源清单加载失败')
  } finally {
    loading.value = false
  }
}

async function pull(key: string): Promise<void> {
  if (busyKey.value) return
  busyKey.value = key
  try {
    const r = await manageApi.system.fetchDomainRules(key)
    toast(
      'success',
      `拉取完成：新增 ${r.new}、目标变更 ${r.merged}、一致跳过 ${r.skipped}、拒收 ${r.rejected}`
    )
    emit('confirm', true)
  } catch (e) {
    toast('error', e instanceof Error ? e.message : '拉取失败')
  } finally {
    busyKey.value = ''
  }
}

onMounted(loadSources)
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <p class="text-xs text-secondary">选择一个来源立即拉取；只写入规则表，不改动影片数据。</p>

    <p v-if="loading" class="py-6 text-center text-sm text-secondary">加载来源清单…</p>
    <p v-else-if="!sources.length" class="py-6 text-center text-sm text-secondary">
      后端还没有登记公告来源。
    </p>

    <ul v-else class="flex flex-col divide-y divide-[var(--jc-border-subtle)]">
      <li v-for="s in sources" :key="s.key" class="flex items-center gap-[var(--jc-space-3)] py-[var(--jc-space-3)]">
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <span class="text-sm font-[var(--jc-fw-medium)]">{{ s.name }}</span>
          <span class="text-xs text-secondary">字段：{{ s.fieldHint }}</span>
          <span class="break-all text-xs text-secondary opacity-70">{{ s.url }}</span>
        </div>
        <ManageButton
          variant="gradient"
          size="sm"
          :disabled="!!busyKey"
          :loading="busyKey === s.key"
          @click="pull(s.key)"
        >
          {{ busyKey === s.key ? '拉取中' : '拉取' }}
        </ManageButton>
      </li>
    </ul>

  </div>
</template>