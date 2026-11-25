<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import type { DomainRuleView } from '@/api/manage/system'

/**
 * 规则影响面预览（openManageDialog 内容组件，**只读**）。
 *
 * 价值在于回答两个问题：
 *   1) 这条规则在我们库里到底命中多少行？（0 = 我们没采集过这个旧域名，可直接标"无需替换"）
 *   2) 前若干条 before → after 长什么样？（确认替换口径没写错）
 *
 * 播放/下载地址在 JSON 大列里，本工具暂不自动执行 ⇒ 明确标注"仅展示"，避免误以为能一键替换。
 */
interface Props {
  rule: DomainRuleView
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'cancel'): void
}>()

const loading = ref(true)
const loadError = ref(false)
const hits = ref(0)
const sample = ref<{ mid: number; before: string; after: string }[]>([])

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const res = await manageApi.system.previewDomainRule(props.rule.id)
    hits.value = res.hits
    sample.value = res.sample ?? []
  } catch (e) {
    loadError.value = true
    toast('error', e instanceof Error ? e.message : '预览失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <div class="flex flex-wrap items-center gap-[var(--jc-space-2)] text-sm">
      <span class="rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] px-[var(--jc-space-2)] py-[2px] text-xs">
        {{ rule.fieldLabel || rule.field }}
      </span>
      <span class="text-xs text-secondary">{{ rule.scope === 'slave' ? '从站' : '主站' }}</span>
      <span class="break-all font-[var(--jc-fw-medium)]">{{ rule.fromHost }} → {{ rule.toHost }}</span>
    </div>

    <ManageErrorState v-if="loadError" title="预览加载失败" @retry="load" />

    <template v-else-if="loading">
      <p class="text-sm text-secondary">统计中…</p>
    </template>

    <template v-else>
      <p v-if="!rule.supported" class="rounded-[var(--jc-radius-md)] bg-[var(--jc-bg-subtle)] p-[var(--jc-space-3)] text-xs text-secondary">
        该字段（{{ rule.field }}）位于播放/下载的大 JSON 列里，本工具<b>只展示不执行</b>；
        请按源站说明人工处理后，在卡片上标「已完成」。
      </p>

      <p v-if="rule.supported" class="text-sm">
        命中 <strong class="text-lg">{{ hits }}</strong> 行
        <span v-if="hits === 0 && rule.state === 'applied'" class="text-xs text-secondary">
          —— 该规则已执行过，命中 0 行属正常
        </span>
        <span v-else-if="hits === 0" class="text-xs text-secondary">
          —— 库里没有该旧域名，说明我们没采集到，可以标「无需替换」
        </span>
      </p>

      <ul v-if="sample.length" class="flex max-h-[42vh] flex-col gap-1 overflow-auto text-xs">
        <li v-for="s in sample" :key="s.mid" class="break-all rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] p-[var(--jc-space-2)]">
          <span class="text-secondary">#{{ s.mid }}</span>
          <span class="mx-1">{{ s.before }}</span>
          <span class="text-secondary">→</span>
          <span class="ml-1">{{ s.after }}</span>
        </li>
      </ul>

      <p class="text-xs text-secondary">
        落点：{{ rule.targetColumn || '本库不存储该字段' }}
        <span v-if="rule.targetNote">—— {{ rule.targetNote }}</span>
      </p>
      <p v-if="rule.sourceNote" class="text-xs text-secondary">源站备注：{{ rule.sourceNote }}</p>
    </template>

    <div class="flex justify-end">
      <ManageButton variant="outline" @click="emit('cancel')">关闭</ManageButton>
    </div>
  </div>
</template>