<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as filmApi from '@/api/film'
import type { Card } from '@/types/film'
import type { PageMeta } from '@/types/api'
import FilmGrid from '@/components/film/FilmGrid.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import SkeletonGrid from '@/components/base/SkeletonGrid.vue'
import FilmFilterBar, { type FilterGroup } from '@/components/film/FilmFilterBar.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'

/**
 * /filmActor?name=xxx&current=1
 * 演员影片列表页(2026-11-07): 详情页「演职人员」的演员名可点进入。
 * 数据口径 = GET /films/by-actor —— 本库 actor 列按人名**等值成员匹配**(非子串,
 * "张翰"不会误中"张翰文"), 热度序(hot_score → hits → mid), 与公开列表同基准。
 */
const router = useRouter()

const { params, push } = useQuerySync<{ name: string; current: number; year: number; pid: number }>(
  { name: '', current: 1, year: 0, pid: 0 },
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

/** 筛选维度计数（后端 facets）与分类名映射（用于分类标签显示名字而非 pid） */
const facets = ref<filmApi.ActorFacets | null>(null)
const catName = ref<Map<number, string>>(new Map())
let facetsForName = ''

async function loadFacets(name: string): Promise<void> {
  if (facetsForName === name) return
  facetsForName = name
  try {
    facets.value = await filmApi.getActorFacets(name)
    if (catName.value.size === 0) {
      type CatNode = { id?: number; name: string; children?: CatNode[] }
      const tree = (await filmApi.getCategories()) as unknown as CatNode[]
      // 分类接口返回的是**树**，且节点上的 pid 是"父 id"（顶层为 0）。
      // 用 c.pid 当 key ⇒ 只有 0/父 id 有值，facets 的真实 pid 全查不到，界面退化成"分类1/分类2"。
      // 这里按自身 id、并展开子级建立映射。
      const flat: CatNode[] = []
      const walk = (list: CatNode[]): void => {
        for (const c of list) {
          flat.push(c)
          if (c.children?.length) walk(c.children)
        }
      }
      walk(tree ?? [])
      catName.value = new Map(flat.map((c) => [Number(c.id ?? 0), c.name]))
    }
  } catch {
    facets.value = null
  }
}

const filterGroups = computed<FilterGroup[]>(() => {
  const f = facets.value
  if (!f) return []
  const years = f.years.map((it) => ({ value: it.year, label: `${it.year} (${it.count})` }))
  const cats = f.categories.map((it) => ({
    value: it.pid,
    label: `${catName.value.get(it.pid) ?? `分类${it.pid}`} (${it.count})`
  }))
  return [
    {
      key: 'year',
      title: '年份',
      current: Number(params.value.year) || 0,
      options: [{ value: 0, label: '全部' }, ...years]
    },
    {
      key: 'pid',
      title: '分类',
      current: Number(params.value.pid) || 0,
      options: [{ value: 0, label: '全部' }, ...cats]
    }
  ]
})

/** 切筛选：重置到第 1 页（走 URL push，onChange 会自动 load） */
function onFilterChange(payload: { key: string; value: string | number }): void {
  const v = Number(payload.value) || 0
  if (payload.key === 'year') void push({ year: v, current: 1 })
  else if (payload.key === 'pid') void push({ pid: v, current: 1 })
}

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
    void loadFacets(name)
    const resp = await filmApi.getFilmsByActor(name, params.value.current || 1, {
      year: Number(params.value.year) || 0,
      pid: Number(params.value.pid) || 0,
      signal
    })
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
  // 走 URL push(兄弟视图同款): 直接改 params ref 既不回流 URL 也不触发 onChange → load()
  void push({ current: p })
}

// useQuerySync 的 onChange 只响应后续 query 变化, 首次挂载(详情页点演员名进入)必须显式起播
void load()
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
    <!-- 筛选（年份 / 分类，标签带数量） -->
    <FilmFilterBar v-if="filterGroups.length > 0" :groups="filterGroups" @change="onFilterChange" />

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
