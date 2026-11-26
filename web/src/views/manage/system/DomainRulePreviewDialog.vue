<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { DomainRuleView } from '@/api/manage/system'
import { DOMAIN_RULE_STATE_MAP } from './domainRuleState'

/**
 * 规则影响面预览（openManageDialog 内容组件，**只读**）。
 *
 * 结构固定为四块，块之间用分隔线断开，避免"一片文字糊在一起"：
 *   ① 概览（类型 / 站点 / 域名）
 *   ② 影响（大字行数 + 一句说明）
 *   ③ 样例（改前 → 改后）
 *   ④ 来源备注
 */
interface Props {
  rule: DomainRuleView
}
const props = defineProps<Props>()

const emit = defineEmits<{
  /** 直接把"下一步操作"交回页面执行（页面负责确认/执行/刷新） */
  (e: 'confirm', payload: { action: 'apply' | 'skip' | 'reject' | 'reset' }): void
}>()

const loading = ref(true)
const loadError = ref(false)
const hits = ref(0)
const sample = ref<{ mid: number; before: string; after: string }[]>([])
const sites = ref<string[]>([])


const fieldVariant = computed<'brand' | 'purple' | 'default'>(() => {
  if (props.rule.field === 'vod_pic') return 'brand'
  if (props.rule.field === 'vod_play_url') return 'purple'
  return 'default'
})

const scopeText = computed(() => {
  if (props.rule.scope !== 'slave') return '主站'
  return props.rule.sourceCode ? `从站 · ${props.rule.sourceCode}` : '从站 · 全部'
})

/** 一句话说明：能不能执行 / 命中 0 行怎么理解 */
const desc = computed(() => {
  if (loading.value) return '统计中…'
  if (hits.value === 0) {
    if (props.rule.state === 'applied') return '该规则已执行过，命中 0 行属正常。'
    return '库里没有这个旧域名（我们没采集到），可以标「无需替换」。'
  }
  return props.rule.supported
    ? '可以在这里直接执行，执行时显示进度。'
    : props.rule.targetNote
})

async function load(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const res = await manageApi.system.previewDomainRule(props.rule.id)
    hits.value = res.hits
    sample.value = res.sample ?? []
    sites.value = res.sites ?? []
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
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <!-- ① 概览 -->
    <div class="flex flex-col gap-[var(--jc-space-2)]">
      <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
        <BaseTag :variant="fieldVariant" size="sm">{{ rule.fieldLabel || rule.field }}</BaseTag>
        <span class="text-xs text-secondary">{{ scopeText }}</span>
        <span v-if="rule.targetColumn" class="text-xs text-muted">落点 {{ rule.targetColumn }}</span>
        <StatusTag :value="rule.state" :map="DOMAIN_RULE_STATE_MAP" size="sm" class="ml-auto" />
      </div>
      <p class="break-all text-sm font-[var(--jc-fw-medium)]">
        {{ rule.fromHost }} <span class="text-muted">→</span> {{ rule.toHost }}
      </p>
    </div>

    <ManageErrorState v-if="loadError" title="预览加载失败" @retry="load" />

    <template v-else>
      <!-- 分隔线：用实心 1px 背景，保证深色底上可见 -->
      <div class="h-px w-full bg-[var(--jc-border-subtle)]" />

      <!-- ② 影响面 -->
      <div class="flex flex-col gap-[var(--jc-space-2)]">
        <p class="text-[11px] tracking-wide text-muted">影响面</p>
        <div class="flex flex-col gap-[var(--jc-space-2)] rounded-[var(--jc-radius-md)] bg-[var(--jc-bg-subtle)] p-[var(--jc-space-4)]">
          <div class="flex items-baseline gap-[var(--jc-space-2)]">
            <span class="text-3xl font-[var(--jc-fw-semibold)] tabular-nums leading-none">
              {{ loading ? '…' : hits.toLocaleString() }}
            </span>
            <span class="text-xs text-muted">行受影响</span>
            <span v-if="sites.length" class="ml-auto text-xs text-muted">涉及源 {{ sites.join('、') }}</span>
          </div>
          <p class="text-xs text-muted">{{ desc }}</p>
        </div>
      </div>

      <!-- ③ 样例 -->
      <template v-if="sample.length">
        <div class="h-px w-full bg-[var(--jc-border-subtle)]" />
        <div class="flex flex-col gap-[var(--jc-space-2)]">
          <p class="text-[11px] tracking-wide text-muted">样例 · 前 {{ sample.length }} 条</p>
          <ul class="flex max-h-[28vh] flex-col divide-y divide-[var(--jc-border-default)] overflow-auto rounded-[var(--jc-radius-md)] border border-default">
            <li v-for="s in sample" :key="s.mid" class="flex flex-col gap-1 p-[var(--jc-space-3)]">
              <span class="w-fit rounded-[var(--jc-radius-sm)] bg-[var(--jc-bg-subtle)] px-[var(--jc-space-2)] py-[1px] text-[11px] tabular-nums text-muted">
                #{{ s.mid }}
              </span>
              <span class="break-all text-xs text-muted line-through">{{ s.before }}</span>
              <span class="break-all text-xs text-[var(--jc-success)]">{{ s.after }}</span>
            </li>
          </ul>
        </div>
      </template>

      <!-- ④ 来源备注 -->
      <template v-if="rule.sourceNote">
        <div class="h-px w-full bg-[var(--jc-border-subtle)]" />
        <div class="flex flex-col gap-1">
          <p class="text-[11px] tracking-wide text-muted">来源备注</p>
          <p class="text-xs text-muted">{{ rule.sourceNote }}</p>
        </div>
      </template>
    </template>

    <!-- 操作栏：吸在滚动容器底部，保证任何内容长度下都可见 -->
    <div
      class="sticky bottom-0 z-[1] -mx-[var(--jc-space-5)] -mb-[var(--jc-space-5)] mt-[var(--jc-space-2)] flex flex-wrap items-center justify-end gap-[var(--jc-space-2)] border-t border-subtle bg-surface px-[var(--jc-space-5)] pb-[var(--jc-space-5)] pt-[var(--jc-space-3)]"
    >
      <template v-if="rule.state === 'pending'">
        <ManageButton variant="ghost" @click="emit('confirm', { action: 'skip' })">无需替换</ManageButton>
        <ManageButton
          variant="ghost"
          class="text-[var(--jc-danger)]"
          @click="emit('confirm', { action: 'reject' })"
        >拒绝</ManageButton>
        <ManageButton
          v-if="rule.supported"
          variant="gradient"
          @click="emit('confirm', { action: 'apply' })"
        >执行替换</ManageButton>
      </template>
      <ManageButton v-else variant="ghost" @click="emit('confirm', { action: 'reset' })">重置</ManageButton>
    </div>
  </div>
</template>
