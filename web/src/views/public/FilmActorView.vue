<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import * as filmApi from '@/api/film'
import type { Card } from '@/types/film'
import type { PageMeta } from '@/types/api'
import FilmGrid from '@/components/film/FilmGrid.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import SkeletonGrid from '@/components/base/SkeletonGrid.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'

/**
 * /filmActor?name=xxx&current=1
 * 演员影片列表页(2026-11-07): 详情页「演职人员」的演员名可点进入。
 * 数据口径 = GET /films/by-actor —— 本库 actor 列按人名**等值成员匹配**(非子串,
 * "张翰"不会误中"张翰文"), 热度序(hot_score → hits → mid), 与公开列表同基准。
 */
const router = useRouter()

const { params } = useQuerySync<{ name: string; current: number }>(
  { name: '', current: 1 },
  {
    path: '/filmActor',
    onChange: () => {
      void load()
    }
  }
)

const list = ref<Card[]>([])
const page = ref<PageMeta>({ size: 30, current: 1, pageCount: 0, total: 0 })
const loading = ref(false)
const loaded = ref(false)
const errorMsg = ref<string>('')

const abortable = useAbortable()

async function load(): Promise<void> {
  const name = (params.value.name || '').trim()
  if (!name) {
    list.value = []
    loaded.value = true
    return
  }
  loading.value = true
  errorMsg.value = ''
  const signal = abortable.refresh()
  try {
    const resp = await filmApi.getFilmsByActor(name, params.value.current || 1, { signal })
    list.value = resp.list ?? []
    page.value = resp.page ?? page.value
    loaded.value = true
  } catch (e) {
    // 401/网络错误已被 http 拦截器 toast；这里仅清空业务态
    if ((e as { name?: string })?.name === 'CanceledError') return
    list.value = []
    errorMsg.value = '加载失败，请稍后重试'
    loaded.value = true
  } finally {
    loading.value = false
  }
}

function onPaginate(p: number): void {
  params.value = { ...params.value, current: p }
}
</script>

<template>
  <div
    class="jc-film-actor container-page flex flex-col gap-[var(--jc-space-5)] py-[var(--jc-space-6)]"
  >
    <!-- 页头: 演员名 + 参演总数 -->
    <header class="flex items-baseline gap-[var(--jc-space-3)]">
      <h1 class="text-xl font-semibold md:text-2xl">{{ params.name }}</h1>
      <span v-if="loaded && page.total > 0" class="text-secondary text-sm">
        共 {{ page.total }} 部参演影片
      </span>
    </header>

    <!-- 骨架屏(首载) -->
    <SkeletonGrid v-if="loading && !loaded" :count="12" />

    <!-- 结果网格 -->
    <FilmGrid v-if="!loading && list.length > 0" :items="list" />

    <!-- 分页 -->
    <div
      v-if="!loading && page.total > 0"
      class="mt-[var(--jc-space-6)] flex justify-center"
    >
      <BasePagination
        :current="page.current || 1"
        :page-size="page.size || 30"
        :total="page.total"
        @change="onPaginate"
      />
    </div>

    <!-- 空状态 / 加载失败 -->
    <BaseEmpty
      v-if="!loading && loaded && list.length === 0"
      :title="errorMsg || '该演员暂无收录影片'"
      :description="errorMsg ? '' : '换一位演员试试，或从首页分类发现更多内容'"
    >
      <template #action>
        <BaseButton variant="gradient" size="md" @click="router.back()">返回</BaseButton>
      </template>
    </BaseEmpty>
  </div>
</template>
