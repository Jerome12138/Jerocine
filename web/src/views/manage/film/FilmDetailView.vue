<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import { normalizeCollectResult } from '@/api/manage/film'
import type {
  CollectSource,
  ManageFilmDetailResp,
  ManageFilmSource,
  SourceFilmResult,
  SourceSearchHit
} from '@/types/manage'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import SourceCandidateDialog from '@/components/manage/SourceCandidateDialog.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

const route = useRoute()
const router = useRouter()
const data = ref<ManageFilmDetailResp | null>(null)
/** 全部采集源(用于合并展示未采集源) */
const collectSources = ref<CollectSource[]>([])
const loading = ref(true)
const error = ref('')

/** 当前影片 mid(来自 query.id) */
const mid = computed(() => String(route.query.id ?? '').trim())

/** 正在采集的源(按 siteId, 用于按钮 loading) */
const collecting = ref<Record<string, boolean>>({})

/** 候选选择对话框状态(同名多版本) */
const candidateDialog = ref<{
  visible: boolean
  sourceId: string
  sourceName: string
  list: SourceSearchHit[]
}>({ visible: false, sourceId: '', sourceName: '', list: [] })
/** 候选弹框内正在采集(锁定交互) */
const candidatePicking = ref(false)

const movie = computed(() => data.value?.movie ?? null)
const sources = computed(() => data.value?.sources ?? [])
/** 影片是否在回收站(软删): 公开读路径把已删影片当不存在, 跳转按钮需禁用并提示 */
const isDeletedMovie = computed(() => (movie.value?.deletedAt ?? 0) > 0)

/**
 * 合并行: 以「全部采集源 ∪ 详情中出现的源」为全集, 按 siteId 聚合。
 * - episodes: 该 siteId 在详情里所有 playFrom 的集数总和(0 = 未采集);
 * - master / siteName: 优先取采集源元信息, 缺失再回退详情/siteId;
 * - collected: episodes > 0;
 * - disabled: 源已停用(服务端仍允许单源手动采, 但要标出来避免误判可用状态)。
 */
interface MergedSource {
  siteId: string
  siteName: string
  master: boolean
  episodes: number
  collected: boolean
  disabled: boolean
}

const mergedSources = computed<MergedSource[]>(() => {
  // 1) 按 siteId 聚合详情里各源的集数
  const detailBySite = new Map<string, { name: string; master: boolean; episodes: number }>()
  for (const s of sources.value) {
    const prev = detailBySite.get(s.siteId)
    const eps = (prev?.episodes ?? 0) + s.episodes.length
    detailBySite.set(s.siteId, {
      name: prev?.name || s.siteName,
      master: prev?.master || s.master,
      episodes: eps
    })
  }

  const rows: MergedSource[] = []
  const used = new Set<string>()

  // 2) 以全部采集源为基准(保持采集源列表顺序)
  for (const c of collectSources.value) {
    used.add(c.id)
    const d = detailBySite.get(c.id)
    rows.push({
      siteId: c.id,
      siteName: c.name || d?.name || c.id,
      master: c.grade === 0,
      episodes: d?.episodes ?? 0,
      collected: (d?.episodes ?? 0) > 0,
      disabled: !c.state
    })
  }

  // 3) 详情里有、但采集源列表已无(被删除的源)的库存数据也补上
  for (const [siteId, d] of detailBySite) {
    if (used.has(siteId)) continue
    rows.push({
      siteId,
      siteName: d.name || siteId,
      master: d.master,
      episodes: d.episodes,
      collected: d.episodes > 0,
      disabled: false
    })
  }

  return rows
})

async function load(): Promise<void> {
  if (!mid.value) {
    error.value = '缺少影片 ID'
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  try {
    data.value = await manageApi.film.detail(mid.value)
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

/** 采集写操作统一 useAsyncAction: busy 并发拦截(重操作全局串行), 异常 toast 由 run
 *  单点; silent=true 关拦截器通用「操作成功」—— 单源采集业务失败时 HTTP 仍是 200,
 *  结果内 error 字段才代表真实成败, 由 handleCollectResult 自管提示。 */
const { busy, run } = useAsyncAction()

/** 采集单源: collectFilm({ sourceId, mid }) → 处理 collected / candidates */
async function collectOne(row: MergedSource): Promise<void> {
  if (!mid.value) return
  collecting.value[row.siteId] = true
  await run({
    action: async () => {
      const res = await manageApi.film.collectFilm(
        { sourceId: row.siteId, mid: Number(mid.value) },
        { silent: true }
      )
      const hit = pickResult(res, row.siteId)
      await handleCollectResult(hit, row.siteName, row.siteId)
    }
  })
  collecting.value[row.siteId] = false
}

/** 从(单个/数组)结果里取对应源结果 */
function pickResult(
  res: SourceFilmResult | SourceFilmResult[],
  siteId: string
): SourceFilmResult | undefined {
  const list = normalizeCollectResult(res)
  return list.find((r) => r.sourceId === siteId) ?? list[0]
}

/** 统一处理一条源采集结果: 成功重载 / 候选弹框 / 未搜到提示 */
async function handleCollectResult(
  hit: SourceFilmResult | undefined,
  sourceName: string,
  sourceId: string
): Promise<void> {
  if (!hit) {
    toast('error', `${sourceName} 未返回结果`)
    return
  }
  if (hit.error) {
    toast('error', `${sourceName} 采集失败: ${hit.error}`)
    return
  }
  if (hit.collected > 0) {
    toast('success', `${sourceName} 采集完成, 入库 ${hit.collected} 集`)
    await load()
    return
  }
  if (hit.candidates && hit.candidates.length) {
    candidateDialog.value = {
      visible: true,
      sourceId,
      sourceName,
      list: hit.candidates
    }
    return
  }
  toast('error', `${sourceName} 未搜到此片`)
}

/** 候选弹框选定某版本: collectFilm({ sourceId, vodId }) → 成功重载(异常 toast 由 run 单点) */
async function onPickCandidate(h: SourceSearchHit): Promise<void> {
  const { sourceId, sourceName } = candidateDialog.value
  if (!sourceId) return
  candidatePicking.value = true
  collecting.value[sourceId] = true
  await run({
    action: async () => {
      const res = await manageApi.film.collectFilm(
        { sourceId, vodId: h.sourceVodId },
        { silent: true }
      )
      const hit = pickResult(res, sourceId)
      if (hit && !hit.error && hit.collected > 0) {
        toast('success', `${sourceName} 采集完成, 入库 ${hit.collected} 集`)
        candidateDialog.value.visible = false
        await load()
      } else if (hit?.error) {
        toast('error', `${sourceName} 采集失败: ${hit.error}`)
      } else {
        toast('error', `${sourceName} 采集未入库`)
      }
    }
  })
  candidatePicking.value = false
  collecting.value[sourceId] = false
}

/** 去公开详情页(公开路由约定 query.link 承载 mid)。已软删影片公开页视为不存在, 不跳 */
function gotoDetail(): void {
  if (!mid.value || isDeletedMovie.value) return
  router.push({ path: '/filmDetail', query: { link: mid.value } })
}

/** 去公开播放页: source 用 `siteId:playFrom` 形式(与后端 PlaySource.id 对齐)。已软删不跳 */
function gotoPlay(): void {
  if (!mid.value || isDeletedMovie.value) return
  const first = sources.value[0]
  const query: Record<string, string> = { id: mid.value, episode: '0' }
  if (first) query.source = `${first.siteId}:${first.playFrom}`
  router.push({ path: '/play', query })
}

/** 采集源列表(独立于详情加载, 失败不阻断详情, 但要显式提示——否则"按源补采"悄悄不可用) */
const sourceListError = ref('')
async function loadSources(): Promise<void> {
  sourceListError.value = ''
  try {
    collectSources.value = await manageApi.collect.list()
  } catch (e) {
    sourceListError.value = (e as Error).message ?? '加载失败'
  }
}

onMounted(async () => {
  await load()
  await loadSources()
})
</script>

<template>
  <section class="bg-surface rounded-card shadow-card p-[var(--jc-space-6)] max-w-[1100px]">
    <header class="flex items-center justify-between mb-[var(--jc-space-5)] flex-wrap gap-[var(--jc-space-3)]">
      <h2 class="text-lg font-[var(--jc-fw-semibold)]">影片详情</h2>
      <div class="flex gap-[var(--jc-space-2)] flex-wrap">
        <ManageButton
          variant="outline"
          :disabled="!movie || isDeletedMovie"
          :title="isDeletedMovie ? '影片在回收站，公开页不可见；请先在影片列表「回收站」页签恢复' : ''"
          @click="gotoDetail"
        >
          <BaseIcon name="film" size="16px" /> 去详情页
        </ManageButton>
        <ManageButton
          variant="gradient"
          :disabled="!sources.length || isDeletedMovie"
          :title="isDeletedMovie ? '影片在回收站，公开页不可见；请先在影片列表「回收站」页签恢复' : ''"
          @click="gotoPlay"
        >
          <BaseIcon name="play" size="16px" /> 去播放页
        </ManageButton>
        <ManageButton variant="ghost" @click="router.back()">返回</ManageButton>
      </div>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton shape="rect" height="240px" />
      <BaseSkeleton v-for="i in 4" :key="i" shape="rect" height="20px" />
    </div>

    <BaseEmpty v-else-if="error || !movie" :description="error || '未找到影片'" />

    <template v-else>
      <!-- ===== 影片信息 ===== -->
      <article class="flex flex-col md:flex-row gap-[var(--jc-space-6)]">
        <div class="w-full md:w-[200px] shrink-0 rounded-[var(--jc-radius-lg)] overflow-hidden">
          <BaseImage :src="movie.cover" :alt="movie.name" ratio="3/4" />
        </div>
        <div class="flex-1 flex flex-col gap-[var(--jc-space-3)] min-w-0">
          <h3 class="text-[length:var(--jc-fs-2xl)] font-[var(--jc-fw-bold)]">
            {{ movie.name }}
            <span v-if="movie.subTitle" class="text-base text-muted font-normal ml-2">
              {{ movie.subTitle }}
            </span>
          </h3>
          <div class="flex flex-wrap gap-[var(--jc-space-2)]">
            <BaseTag v-if="isDeletedMovie" variant="danger">回收站</BaseTag>
            <BaseTag v-if="movie.cName" variant="purple">{{ movie.cName }}</BaseTag>
            <BaseTag v-if="movie.year">{{ movie.year }}</BaseTag>
            <BaseTag v-if="movie.area">{{ movie.area }}</BaseTag>
            <BaseTag v-if="movie.remarks" variant="info">{{ movie.remarks }}</BaseTag>
          </div>
          <p class="text-sm text-secondary">
            <span class="text-muted">导演：</span>{{ movie.director || '—' }}
          </p>
          <p class="text-sm text-secondary">
            <span class="text-muted">主演：</span>{{ movie.actor || '—' }}
          </p>
          <p class="text-sm text-secondary">
            <span class="text-muted">语言：</span>{{ movie.language || '—' }}
          </p>
          <p class="text-sm text-muted whitespace-pre-line max-h-[240px] overflow-auto">
            {{ movie.content || '暂无简介' }}
          </p>
        </div>
      </article>

      <!-- ===== 采集源(合并: 已采集显集数 + 未采集源) ===== -->
      <section class="mt-[var(--jc-space-6)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-3)]">
          <h4 class="text-base font-[var(--jc-fw-semibold)]">
            采集源 <span class="text-muted text-sm">({{ mergedSources.length }})</span>
          </h4>
        </div>

        <!-- 源列表加载失败: 显式提示(否则"未采集"行整体消失, 按源补采能力悄悄不可用) -->
        <div
          v-if="sourceListError"
          class="flex items-center gap-[var(--jc-space-2)] mb-[var(--jc-space-3)] text-sm text-[var(--jc-warning)] flex-wrap"
        >
          <BaseIcon name="info" size="14px" />
          <span>采集源列表加载失败，仅显示已采集源：{{ sourceListError }}</span>
          <ManageButton variant="ghost" @click="loadSources">重试</ManageButton>
        </div>

        <BaseEmpty v-if="!mergedSources.length" description="暂无采集源" />

        <div v-else class="flex flex-col gap-[var(--jc-space-2)]">
          <div
            v-for="row in mergedSources"
            :key="row.siteId"
            class="flex items-center gap-[var(--jc-space-3)] p-[var(--jc-space-3)] flex-wrap border border-default rounded-[var(--jc-radius-md)] bg-elevated"
          >
            <span class="font-[var(--jc-fw-medium)] text-primary">{{ row.siteName }}</span>
            <BaseTag v-if="row.master" variant="brand" size="xs">主站</BaseTag>
            <BaseTag v-if="row.disabled" variant="warning" size="xs">已停用</BaseTag>
            <BaseTag v-if="row.collected" variant="success" size="xs">{{ row.episodes }} 集</BaseTag>
            <BaseTag v-else variant="default" size="xs">未采集</BaseTag>
            <div class="flex-1" />
            <ManageButton
              :variant="row.collected ? 'outline' : 'primary'"
              size="sm"
              :loading="collecting[row.siteId]"
              :disabled="busy"
              :title="row.disabled ? '该源已停用，仅此处可手动采集' : ''"
              @click="collectOne(row)"
            >
              <BaseIcon name="magic" size="16px" /> {{ row.collected ? '重采' : '采集' }}
            </ManageButton>
          </div>
        </div>
      </section>
    </template>

    <!-- 同名多版本选择对话框(详情页与新增页共用组件) -->
    <SourceCandidateDialog
      v-model:visible="candidateDialog.visible"
      :candidates="candidateDialog.list"
      :source-name="candidateDialog.sourceName"
      :picking="candidatePicking"
      @pick="onPickCandidate"
    />
  </section>
</template>
