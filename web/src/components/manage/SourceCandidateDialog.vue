<script setup lang="ts">
import type { SourceSearchHit } from '@/types/manage'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'

/**
 * 同名多版本候选选择对话框。
 * 当单源精确采集返回 candidates(同名多个影片)时弹出, 用户点选某版本后 emit('pick', hit),
 * 由父组件以 {sourceId, vodId: hit.sourceVodId} 精确采集。FilmDetailView / FilmAddView 共用。
 */
interface Props {
  visible: boolean
  /** 候选影片列表 */
  candidates: SourceSearchHit[]
  /** 来源采集源名(标题展示用) */
  sourceName?: string
  /** 选中某项后是否处于采集中(锁定交互) */
  picking?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  sourceName: '',
  picking: false
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'pick', hit: SourceSearchHit): void
}>()

function meta(h: SourceSearchHit): string {
  return [h.year || '', h.typeName, h.remarks].filter(Boolean).join(' · ') || '—'
}

function pick(h: SourceSearchHit): void {
  if (props.picking) return
  emit('pick', h)
}
</script>

<template>
  <BaseDialog
    :visible="visible"
    :title="sourceName ? `选择版本 · ${sourceName}` : '选择版本'"
    width="560px"
    @update:visible="(v) => emit('update:visible', v)"
  >
    <p class="text-sm text-muted mb-[var(--gf-space-3)]">
      该源搜到多个同名影片，请选择要采集的版本：
    </p>

    <BaseEmpty v-if="!candidates.length" description="无候选版本" />

    <ul v-else class="flex flex-col gap-[var(--gf-space-2)]">
      <li
        v-for="h in candidates"
        :key="h.sourceVodId"
        class="flex items-center gap-[var(--gf-space-3)] p-[var(--gf-space-2)] rounded-[var(--gf-radius-md)] border border-default bg-surface cursor-pointer hover:border-[var(--gf-brand-primary)] transition-colors"
        :class="picking ? 'opacity-60 pointer-events-none' : ''"
        data-focusable="true"
        tabindex="0"
        role="button"
        :aria-label="`选择 ${h.name}`"
        @click="pick(h)"
        @keydown.enter.prevent="pick(h)"
        @keydown.space.prevent="pick(h)"
      >
        <div class="w-[40px] h-[54px] shrink-0 rounded-[var(--gf-radius-sm)] overflow-hidden bg-elevated">
          <BaseImage v-if="h.cover" :src="h.cover" :alt="h.name" ratio="3/4" />
        </div>
        <div class="min-w-0 flex-1 flex flex-col gap-[2px]">
          <span class="text-sm text-primary truncate">{{ h.name }}</span>
          <span class="text-xs text-muted truncate">{{ meta(h) }}</span>
          <span class="text-xs text-muted">{{ h.episodes }} 集</span>
        </div>
      </li>
    </ul>
  </BaseDialog>
</template>
