<script setup lang="ts">
import { computed } from 'vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageButton from '@/components/manage/ManageButton.vue'

/**
 * ManageErrorState — manage 区「错误文案 + 重试按钮」统一错误态（§6 组合组件，R1 评审裁决实施）。
 * 收口两处已漂移的同类块（FilmListView 卡片式 / LiveSourceListView 行内式），
 * §C8#1 追加 banner 形态收口 Cron/Failure/Collect 三处手写横幅。
 *
 * - card：页面级错误卡 —— bg-surface rounded-card 卡片内 BaseEmpty(hide-icon) + outline sm 重试钮；
 * - plain：行内浅卡片 —— 红字段落（标题：详情）+ 重试钮，底色/圆角/留白沿用原 .jc-livemgr__error；
 * - banner：单行横幅（CronListView/FailureListView/CollectListView 三处原款）——
 *   info 图标 + 「标题: 详情」半角冒号红字 + 右侧重试钮（ml-auto），文案差异保留
 *   （≠ plain 的全角冒号段落式）。
 */
interface Props {
  /** 错误标题：card 形态=BaseEmpty 标题；plain/banner 形态=段落前缀文案 */
  title: string
  /** 错误详情（接口返回 message 等）：card=BaseEmpty description；plain=标题后接「：详情」；banner=标题后接「: 详情」 */
  description?: string
  /** 重试按钮文案，默认「重试」（banner 三处原款传「点击重试」） */
  retryLabel?: string
  /** 形态：card = 页面级错误卡（默认）；plain = 行内浅卡片；banner = 单行横幅 */
  variant?: 'card' | 'plain' | 'banner'
}

const props = withDefaults(defineProps<Props>(), {
  description: '',
  retryLabel: '重试',
  variant: 'card'
})

const emit = defineEmits<{ (e: 'retry'): void }>()

/** plain 形态段落文案：有详情拼「标题：详情」，无详情只显标题 */
const plainText = computed(() => (props.description ? `${props.title}：${props.description}` : props.title))

/** banner 形态横幅文案：三处原款为半角冒号+空格「标题: 详情」（≠ plain 的全角冒号） */
const bannerText = computed(() => (props.description ? `${props.title}: ${props.description}` : props.title))
</script>

<template>
  <!-- card 形态：页面级错误卡（FilmListView 同款视觉，含 BaseEmpty 的 role=status 播报） -->
  <section v-if="props.variant === 'card'" class="bg-surface rounded-card shadow-card">
    <div class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-10)]">
      <BaseEmpty :title="props.title" :description="props.description" hide-icon />
      <ManageButton variant="outline" @click="emit('retry')">
        <BaseIcon name="refresh" size="16px" /> {{ props.retryLabel }}
      </ManageButton>
    </div>
  </section>
  <!-- banner 形态：单行横幅（CronListView/FailureListView/CollectListView 三处原款：info 图标 + 红字 + 右侧重试钮） -->
  <div
    v-else-if="props.variant === 'banner'"
    class="flex items-center gap-[var(--jc-space-2)] bg-elevated border border-[var(--jc-danger)] rounded-[var(--jc-radius-md)] px-[var(--jc-space-4)] py-[var(--jc-space-3)]"
  >
    <BaseIcon name="info" size="16px" class="text-[var(--jc-danger)]" />
    <span class="text-sm text-[var(--jc-danger)]">{{ bannerText }}</span>
    <ManageButton variant="outline" class="ml-auto" @click="emit('retry')">{{ props.retryLabel }}</ManageButton>
  </div>
  <!-- plain 形态：行内浅卡片（LiveSourceListView 同款视觉：红字段落 + 重试钮） -->
  <div
    v-else
    class="flex flex-col items-center gap-[var(--jc-space-3)] rounded-[var(--jc-radius-md)] bg-surface px-[var(--jc-space-4)] py-[var(--jc-space-8)]"
  >
    <p class="text-[var(--jc-danger)]">{{ plainText }}</p>
    <ManageButton variant="outline" @click="emit('retry')">{{ props.retryLabel }}</ManageButton>
  </div>
</template>
