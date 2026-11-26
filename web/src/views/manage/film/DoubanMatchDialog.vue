<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageErrorState from '@/components/manage/ManageErrorState.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import type { DoubanCandidate } from '@/api/manage/film'

/**
 * 获取豆瓣 ID / 评分（openManageDialog 内容组件）。
 *
 * 为什么要弹窗：豆瓣搜索经常同名多版本（翻拍/剧版/影版），自动匹配只能给一个猜测；
 * 这里允许**改关键字重搜**、把候选列出来让人挑，选谁写谁（写 movie + movie_search）。
 * 也可以直接走「自动匹配」（后端按 同名 + 年份差≤1 规则挑，宁缺勿错）。
 */
interface Props {
  mid: number
  name: string
  year?: number
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'confirm', payload: { dbId: number }): void
}>()

const keyword = ref(props.name)
const loading = ref(false)
const error = ref('')
const list = ref<DoubanCandidate[]>([])
const picked = ref<number>(0)

async function search(): Promise<void> {
  const kw = keyword.value.trim()
  if (!kw) {
    toast('warning', '请填片名')
    return
  }
  loading.value = true
  error.value = ''
  try {
    const r = await manageApi.film.doubanCandidates(props.mid, kw)
    list.value = r.list ?? []
    picked.value = 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : '搜索失败'
    list.value = []
  } finally {
    loading.value = false
  }
}

/** 年份接近的候选高亮，提醒别选到翻拍 */
function yearHint(c: DoubanCandidate): 'success' | 'warning' | 'default' {
  if (!props.year || !c.year) return 'default'
  const d = Math.abs(c.year - props.year)
  if (d === 0) return 'success'
  if (d <= 1) return 'warning'
  return 'default'
}

onMounted(search)
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <div class="flex flex-col gap-[var(--jc-space-2)]">
      <p class="text-xs text-muted">
        当前片：{{ name }}<span v-if="year">（{{ year }}）</span>。可改片名重搜，选中哪条就写哪条。
      </p>
      <div class="flex gap-[var(--jc-space-2)]">
        <ManageInput
          v-model="keyword"
          class="flex-1"
          placeholder="片名（可加年份/别名）"
          @keydown.enter="search"
        />
        <ManageButton variant="outline" :loading="loading" @click="search">搜索</ManageButton>
      </div>
    </div>

    <ManageErrorState v-if="error" title="搜索失败" @retry="search" />

    <p v-else-if="loading" class="py-6 text-center text-sm text-secondary">搜索中…</p>

    <p v-else-if="!list.length" class="py-6 text-center text-sm text-secondary">
      没有候选。换个关键字（比如去掉副标题/用别名）再试。
    </p>

    <ul v-else class="flex max-h-[42vh] flex-col divide-y divide-[var(--jc-border-default)] overflow-auto rounded-[var(--jc-radius-md)] border border-default">
      <li v-for="c in list" :key="c.id">
        <button
          type="button"
          class="jc-hotarea flex w-full items-center gap-[var(--jc-space-3)] px-[var(--jc-space-3)] py-[var(--jc-space-2)] text-left"
          :class="picked === c.id ? 'bg-[var(--jc-bg-subtle)]' : ''"
          @click="picked = c.id"
        >
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm font-[var(--jc-fw-medium)]">{{ c.title }}</span>
            <span class="mt-1 block text-xs text-muted">豆瓣 ID {{ c.id }}</span>
          </span>
          <BaseTag v-if="c.year" :variant="yearHint(c)" size="sm">{{ c.year }}</BaseTag>
          <BaseTag v-if="c.type" variant="info" size="xs">{{ c.type === 'tv' ? '剧集' : '电影' }}</BaseTag>
          <span
            class="w-[18px] text-center text-sm"
            :class="picked === c.id ? 'text-[var(--jc-brand-purple)]' : 'text-transparent'"
          >✓</span>
        </button>
      </li>
    </ul>

    <div class="flex flex-wrap items-center justify-end gap-[var(--jc-space-2)] border-t border-subtle pt-[var(--jc-space-3)]">
      <ManageButton
        variant="ghost"
        @click="emit('confirm', { dbId: 0 })"
      >自动匹配</ManageButton>
      <ManageButton
        variant="gradient"
        :disabled="!picked"
        @click="emit('confirm', { dbId: picked })"
      >使用选中</ManageButton>
    </div>
  </div>
</template>