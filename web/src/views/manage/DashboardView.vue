<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import type { DashboardStat } from '@/types/manage'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'

const data = ref<DashboardStat | null>(null)
const loading = ref(true)
const error = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    data.value = await manageApi.system.dashboard()
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)

const cards = computed(() => {
  if (!data.value) return []
  const d = data.value
  return [
    { label: '影片总数', value: d.filmCount ?? 0, icon: 'film', tint: 'from-[#9b49e7] to-[#4ad1e5]' },
    { label: '今日新增', value: d.todayNew ?? 0, icon: 'plus', tint: 'from-[#f59e0b] to-[#fbbf24]' },
    { label: '近 7 天新增', value: d.weekNew ?? 0, icon: 'film', tint: 'from-[#3b82f6] to-[#22c55e]' },
    { label: '采集源', value: d.collectCount ?? 0, icon: 'magic', tint: 'from-[#E50914] to-[#ff6b6b]' },
    { label: '定时任务', value: d.cronCount ?? 0, icon: 'clock', tint: 'from-[#22c55e] to-[#4ad1e5]' },
    { label: '已停采源', value: d.downSources ?? 0, icon: 'trash', tint: 'from-[#ef4444] to-[#f59e0b]' }
  ]
})
</script>

<template>
  <div class="flex flex-col gap-[var(--gf-space-6)]">
    <header class="flex items-center justify-between">
      <h2 class="text-[var(--gf-fs-2xl)] font-[var(--gf-fw-bold)]">仪表盘</h2>
      <button
        type="button"
        class="text-sm text-link hover:underline min-h-[44px] px-[var(--gf-space-3)]"
        @click="load"
      >
        刷新
      </button>
    </header>

    <!-- loading 骨架 -->
    <div v-if="loading" class="grid grid-cols-1 md:grid-cols-3 gap-[var(--gf-space-4)]">
      <BaseSkeleton v-for="i in 3" :key="i" shape="rect" :height="'120px'" />
    </div>

    <!-- 错误 -->
    <BaseEmpty v-else-if="error" :title="error" :description="'点击右上角刷新重试'" />

    <!-- 3 张统计卡 (真实数据) -->
    <div v-else class="grid grid-cols-1 md:grid-cols-3 gap-[var(--gf-space-4)]">
      <article
        v-for="c in cards"
        :key="c.label"
        class="bg-surface rounded-card shadow-card p-[var(--gf-space-5)] flex items-center gap-[var(--gf-space-4)] min-h-[120px]"
      >
        <div
          class="w-[56px] h-[56px] rounded-full flex items-center justify-center text-white shrink-0"
          :class="['bg-gradient-to-br', c.tint]"
        >
          <BaseIcon :name="c.icon" size="28px" />
        </div>
        <div class="flex flex-col min-w-0">
          <span class="text-secondary text-sm">{{ c.label }}</span>
          <span class="text-[var(--gf-fs-3xl)] font-[var(--gf-fw-black)] tabular-nums">
            {{ c.value }}
          </span>
        </div>
      </article>
    </div>

    <!-- 提示: 更多图表/活动流待后端 -->
    <section v-if="!loading && !error" class="bg-surface rounded-card p-[var(--gf-space-5)]">
      <p class="text-secondary text-sm">
        <BaseIcon name="info" size="14px" class="inline align-middle mr-1" />
        更多统计图表（近 7 天新增 / 最近操作流等）需后端支持，待补。
      </p>
    </section>
  </div>
</template>
