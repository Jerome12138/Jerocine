<script setup lang="ts">
import { computed } from 'vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import ManageButton from '@/components/manage/ManageButton.vue'

/**
 * ManageErrorState — manage 区「错误文案 + 重试按钮」统一错误态（§6 组合组件，R1 评审裁决实施）。
 * 收口两处已漂移的同类块（FilmListView 卡片式 / LiveSourceListView 行内式），
 * 后续全仓其余 10+ 处同模式按交付报告「建议后续」逐页迁移。
 *
 * - card：页面级错误卡 —— bg-surface rounded-card 卡片内 BaseEmpty(hide-icon) + outline sm 重试钮；
 * - plain：行内浅卡片 —— 红字段落（标题：详情）+ 重试钮，底色/圆角/留白沿用原 .jc-livemgr__error。
 */
interface Props {
  /** 错误标题：card 形态=BaseEmpty 标题；plain 形态=段落前缀文案 */
  title: string
  /** 错误详情（接口返回 message 等）：card=BaseEmpty description；plain=标题后接「：详情」 */
  description?: string
  /** 重试按钮文案，默认「重试」 */
  retryLabel?: string
  /** 形态：card = 页面级错误卡（默认）；plain = 行内浅卡片 */
  variant?: 'card' | 'plain'
}

const props = withDefaults(defineProps<Props>(), {
  description: '',
  retryLabel: '重试',
  variant: 'card'
})

const emit = defineEmits<{ (e: 'retry'): void }>()

/** plain 形态段落文案：有详情拼「标题：详情」，无详情只显标题 */
const plainText = computed(() => (props.description ? `${props.title}：${props.description}` : props.title))
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
  <!-- plain 形态：行内浅卡片（LiveSourceListView 同款视觉：红字段落 + 重试钮） -->
  <div
    v-else
    class="flex flex-col items-center gap-[var(--jc-space-3)] rounded-[var(--jc-radius-md)] bg-surface px-[var(--jc-space-4)] py-[var(--jc-space-8)]"
  >
    <p class="text-[var(--jc-danger)]">{{ plainText }}</p>
    <ManageButton variant="outline" @click="emit('retry')">{{ props.retryLabel }}</ManageButton>
  </div>
</template>
