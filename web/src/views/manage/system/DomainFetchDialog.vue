<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageButton from '@/components/manage/ManageButton.vue'
import type { DomainFetchResult, DomainSource } from '@/api/manage/system'

/**
 * 立即拉取公告 —— 先列出「当前支持的来源」，点哪个就拉哪个。
 *
 * 为什么要这个弹窗：公告站不止一个（也可能会分封面站/播放站/不同站点），
 * 直接在按钮上写死一个地址，加站就得改前端。来源清单由后端注册表下发
 * （GET /manage/domain-replace/sources），加站只改后端。
 *
 * 交互：点来源卡片 = 立即拉取。
 * 来源清单、字段口径、解析规则**全部后端维护**，前端只负责渲染，不做任何配置。
 * 拉取只写规则表，不动任何影片数据。
 */
const emit = defineEmits<{
  (e: 'confirm', result: DomainFetchResult): void
  (e: 'cancel'): void
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
    emit('confirm', r)
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
    <p class="text-xs text-secondary">
      选择一个来源立即拉取。<strong>只写入规则表</strong>，不会改动任何影片数据。
    </p>

    <p v-if="loading" class="text-sm text-secondary">加载来源清单…</p>

    <p v-else-if="!sources.length" class="text-sm text-secondary">后端还没有登记公告来源。</p>

    <div v-else class="flex flex-col gap-[var(--jc-space-2)]">
      <button
        v-for="s in sources"
        :key="s.key"
        type="button"
        class="jc-hotarea flex items-start gap-[var(--jc-space-3)] rounded-[var(--jc-radius-md)] border border-default p-[var(--jc-space-3)] text-left disabled:opacity-60"
        :disabled="!!busyKey"
        @click="pull(s.key)"
      >
        <span class="mt-[2px] rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] px-[var(--jc-space-2)] py-[2px] text-xs">
          {{ s.type === 'macrep' ? '替换助手' : s.type }}
        </span>
        <span class="min-w-0 flex-1">
          <span class="block text-sm font-[var(--jc-fw-medium)]">
            {{ s.name }}
            <span class="ml-1 text-xs text-secondary">{{ busyKey === s.key ? '拉取中…' : '' }}</span>
          </span>
          <span class="mt-1 block text-xs text-secondary">字段：{{ s.fieldHint }}</span>
          <span class="mt-1 block text-xs text-secondary">落点：{{ s.targetHint || '解析规则由后端维护' }}</span>
          <span v-if="s.note" class="mt-1 block break-all text-xs text-secondary">{{ s.note }}</span>
          <span class="mt-1 block break-all text-xs text-secondary opacity-70">{{ s.url }}</span>
        </span>
      </button>
    </div>

    <div class="flex justify-end">
      <ManageButton variant="ghost" @click="emit('cancel')">关闭</ManageButton>
    </div>
  </div>
</template>