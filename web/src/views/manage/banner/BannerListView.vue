<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { manageApi } from '@/api'
import type { Banner, BannerBoard, EffectiveSlide, InactiveBanner } from '@/types/manage'
import ManageButton from '@/components/manage/ManageButton.vue'
import ManageTable from '@/components/manage/ManageTable.vue'
import type { Column } from '@/components/manage/ManageTable.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import type { TabItem } from '@/components/manage/ManageTabs.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseLightbox from '@/components/base/BaseLightbox.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import { openManageDialog, manageDialogState } from '@/composables/useManageDialog'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { toast } from '@/api/http'
import BannerFormDialog from './BannerFormDialog.vue'
import type { BannerFormInitial } from './BannerFormDialog.vue'

/**
 * 首页轮播管理 —— 表格化(ManageTable) + 页签切换(生效位 / 未生效)。
 * 生效位 = 前 liveCount 位对外生效 + 其后热榜候选位(共 heroBoardCount=16 位, liveCount 分区)。
 * 手动位(banner): 完整增删改 + 启用/停用 + 上下移;
 * 自动位(热榜补位): 派生数据, 支持编辑采纳(转手动钉位, 即"启用")、禁用屏蔽、上移采纳;
 *   铁律: mid 出现在 banner 表(无论启用与否)就不再被自动补位选中。
 * 未生效区: 停用/缺图/未开始/已过期/超位的配置行, 可启用(转手动)/编辑/删除。
 */

const board = ref<BannerBoard>({ active: [], inactive: [] })
const loading = ref(true)
/** 初次/手动加载失败时的错误文案(非 silent 加载才置位; 30s 轮询失败不打扰) */
const loadError = ref('')
const view = ref<'active' | 'inactive'>('active')
/** 单片横图回填进行中的 mid 集合(防重复点击) */
const fillingMids = ref<Set<number>>(new Set())
let pollTimer: number | undefined

// 写操作统一 useAsyncAction: busy 全局防连点(行内按钮 :disabled)、confirm、失败 toast 单点。
const { busy, run } = useAsyncAction()

async function fillBackdrop(mid: number): Promise<void> {
  if (mid <= 0 || fillingMids.value.has(mid)) return
  fillingMids.value.add(mid)
  try {
    await run({
      action: async () => {
        const r = await manageApi.banner.fillBackdrop(mid) // API 内已 silent
        const tip =
          r.status === 'already' ? '该片已有横图（刷新后页面直接展示）'
          : r.status === 'miss' ? 'TMDB 检索无果（已标记未命中），暂不回填'
          : '横图已获取并回填'
        toast(r.status === 'filled' ? 'success' : 'info', `影片 #${mid}：${tip}`)
        await load(true)
      }
    })
  } finally {
    fillingMids.value.delete(mid)
  }
}

/** 豆瓣影评搜索链接(按影片名) */
function doubanUrl(name: string): string {
  return manageApi.banner.doubanSearchUrl(name)
}

/** 立即刷新榜单: 与每日 04:00 自动刷新共用一把锁, 撞车返 409 */
async function refreshHot(): Promise<void> {
  await run({
    confirm: {
      title: '立即刷新榜单？',
      desc: '拉取豆瓣 8 榜并重算热度、热榜快照与轮播候选位（约 1-2 分钟）。若上一轮刷新未结束会提示稍后再试。',
      okText: '立即刷新'
    },
    action: async () => {
      const rep = await manageApi.banner.refreshHotBoard()
      toast(
        'success',
        `榜单刷新完成：抓取 ${rep.fetched} 条 · 匹配 ${rep.matched} 部 · 落库 ${rep.applied} 部 · 耗时 ${(rep.elapsedMs / 1000).toFixed(1)}s`
      )
    },
    after: () => load(true)
  })
}

const active = computed(() => board.value.active ?? [])
const inactive = computed(() => board.value.inactive ?? [])
const liveCount = computed(() => board.value.liveCount ?? 5)
const manualCount = computed(() => active.value.filter((s) => s.source === 'banner').length)

const tabs = computed<TabItem<'active' | 'inactive'>[]>(() => [
  { value: 'active', label: '生效位', count: active.value.length },
  { value: 'inactive', label: '未生效', count: inactive.value.length }
])

/* ---- 预览放大(lightbox) ---- */
const previewSrc = ref('')
const previewAlt = ref('')
function preview(url?: string, alt?: string): void {
  if (!url) return
  previewAlt.value = alt || ''
  previewSrc.value = url
}

/* ---- 展示辅助 ---- */
function inactiveName(row: InactiveBanner): string {
  return row.title || row.meta?.name || (row.mid > 0 ? `影片 ${row.mid}` : `#${row.id}`)
}
function inactiveImage(row: InactiveBanner): string {
  return row.image || row.meta?.backdrop || ''
}
function inactiveHotLabel(row: InactiveBanner): string {
  const r = row.meta?.hotRank ?? 0
  if (!r) return '—'
  return `${row.meta?.hotBoard || '豆瓣'} No.${r}`
}
function inactiveCanJump(row: InactiveBanner): boolean {
  return !row.link && !!row.mid && row.mid > 0
}
function inactiveGoLabel(row: InactiveBanner): string {
  if (row.link) return row.link
  if (row.mid > 0) return `影片 #${row.mid}`
  return '未配置'
}
function inactiveMetaText(row: InactiveBanner): string {
  const parts = [row.meta?.year, row.meta?.area].filter(Boolean).join(' · ')
  const remarks = row.meta?.remarks ? ` · ${row.meta.remarks}` : ''
  return parts || remarks ? `${parts}${remarks}` : '无关联影片元信息'
}

function blankForm(): BannerFormInitial {
  return {
    id: 0, title: '', subtitle: '', image: '', poster: '',
    mid: '', link: '', sort: 0, state: 0, startStr: '', endStr: ''
  }
}

async function load(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
    loadError.value = ''
  }
  try {
    board.value = (await manageApi.banner.board()) ?? { active: [], inactive: [] }
  } catch (e) {
    if (!silent) loadError.value = (e as { message?: string })?.message || '加载失败'
  } finally {
    if (!silent) loading.value = false
  }
}

async function openForm(initial: BannerFormInitial, adoptSlot?: number): Promise<void> {
  const done = await openManageDialog<boolean>({
    title: initial.id > 0 ? '编辑轮播' : adoptSlot !== undefined ? '采纳自动位' : '新增轮播',
    component: BannerFormDialog,
    props: { initial, adoptSlot },
    mobileMode: 'fullsheet'
  })
  if (!done) return
  await load(true)
}

function openAdd(): void {
  void openForm(blankForm())
}

function openEdit(row: Banner): void {
  void openForm({
    id: row.id,
    title: row.title,
    subtitle: row.subtitle,
    image: row.image,
    poster: row.poster,
    mid: row.mid > 0 ? String(row.mid) : '',
    link: row.link,
    sort: row.sort,
    state: row.state,
    startStr: msToLocal(row.startAt),
    endStr: msToLocal(row.endAt)
  })
}

function adoptSlide(s: EffectiveSlide, slot: number): void {
  void openForm(
    {
      ...blankForm(),
      title: s.name,
      subtitle: s.subtitle ?? '',
      image: s.image ?? '',
      poster: s.poster ?? '',
      mid: s.mid && s.mid > 0 ? String(s.mid) : ''
    },
    slot
  )
}

async function move(i: number, dir: 'up' | 'down'): Promise<void> {
  await run({
    action: async () => {
      board.value = await manageApi.banner.move(i, dir, { silent: true })
    }
  })
}

async function toggleState(row: Banner): Promise<void> {
  await run({
    action: () =>
      manageApi.banner.save({ ...row, state: row.state === 0 ? 1 : 0 }, undefined, { silent: true }),
    after: () => load(true)
  })
}

async function disableByMid(mid: number): Promise<void> {
  if (mid <= 0) return
  await run({
    action: async () => {
      board.value = await manageApi.banner.disableByMid(mid, { silent: true })
    }
  })
}

async function remove(row: Banner): Promise<void> {
  await run({
    confirm: {
      title: '确认删除这张轮播？',
      desc: `「${row.title || `#${row.id}`}」删除后不可恢复`,
      okText: '删除',
      danger: true
    },
    action: () => manageApi.banner.remove(row.id, { silent: true }),
    successMsg: '已删除',
    after: () => load(true)
  })
}

function canMoveUp(s: EffectiveSlide, i: number): boolean {
  return s.source === 'banner' ? i > 0 : true
}
function canMoveDown(s: EffectiveSlide, i: number): boolean {
  if (s.source !== 'banner') return false
  return i < manualCount.value - 1
}
function goLabelOf(s: EffectiveSlide): string {
  if (s.link) return s.link
  if (s.mid && s.mid > 0) return `影片 #${s.mid}`
  return '未配置'
}
function canJump(s: EffectiveSlide): boolean {
  return !s.link && !!s.mid && s.mid > 0
}
function hotLabelOf(s: EffectiveSlide): string {
  if (!s.hotRank) return '—'
  return `${s.hotBoard || '豆瓣'} No.${s.hotRank}`
}

const reasonStyle: Record<InactiveBanner['reason'], { text: string; variant: 'danger' | 'warning' | 'default' }> = {
  disabled: { text: '已停用', variant: 'danger' },
  noimage: { text: '缺横竖图', variant: 'warning' },
  pending: { text: '未开始', variant: 'default' },
  expired: { text: '已过期', variant: 'default' },
  overflow: { text: '超出候选位', variant: 'default' }
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}
function msToLocal(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function scheduleLabel(row: InactiveBanner): string {
  const fmt = (ms: number): string => {
    const t = msToLocal(ms)
    return t ? t.replace('T', ' ') : ''
  }
  const s = fmt(row.startAt)
  const e = fmt(row.endAt)
  if (s && e) return `${s} ~ ${e}`
  if (s) return `${s} 起`
  if (e) return `${e} 止`
  return ''
}
function canEnable(row: InactiveBanner): boolean {
  if (row.state === 0) return false
  return !(row.reason === 'disabled' && !row.image && !row.poster)
}

/* ---- 表格行(派生态) ---- */
interface ActiveRow extends EffectiveSlide {
  _key: string
  _index: number
  _zone: 'live' | 'candidate'
  preview: string
  hotText: string
  goText: string
}
const activeRows = computed<ActiveRow[]>(() =>
  active.value.map((s, i) => ({
    ...s,
    _key: `${s.source}-${s.bannerId ?? 0}-${s.mid ?? 0}-${i}`,
    _index: i,
    _zone: i < liveCount.value ? 'live' : 'candidate',
    preview: '',
    hotText: hotLabelOf(s),
    goText: goLabelOf(s)
  }))
)

const activeColumns: Column<ActiveRow>[] = [
  { key: '_index', label: '#', width: '52px' },
  { key: 'preview', label: '预览', width: '150px' },
  { key: 'name', label: '名称' },
  { key: 'dbScore', label: '评分', width: '64px' },
  { key: 'classTag', label: '分类', width: '140px' },
  { key: 'hotText', label: '榜单', width: '150px' },
  { key: 'goText', label: '跳转', width: '150px' },
  { key: '_zone', label: '分区', width: '76px' }
]

interface InactiveRow extends InactiveBanner {
  preview: string
  metaText: string
  hotText: string
  goText: string
  scheduleText: string
  scoreText: string
  reasonText: string
  reasonVariant: 'danger' | 'warning' | 'default'
}
const inactiveRows = computed<InactiveRow[]>(() =>
  inactive.value.map((row) => ({
    ...row,
    preview: '',
    metaText: inactiveMetaText(row),
    hotText: inactiveHotLabel(row),
    goText: inactiveGoLabel(row),
    scheduleText: scheduleLabel(row),
    scoreText: row.meta?.dbScore ? row.meta.dbScore.toFixed(1) : '—',
    reasonText: reasonStyle[row.reason].text,
    reasonVariant: reasonStyle[row.reason].variant
  }))
)

const inactiveColumns: Column<InactiveRow>[] = [
  { key: 'preview', label: '预览', width: '150px' },
  { key: 'metaText', label: '名称 / 元信息' },
  { key: 'scoreText', label: '评分', width: '64px' },
  { key: 'hotText', label: '榜单', width: '150px' },
  { key: 'scheduleText', label: '排期', width: '230px' },
  { key: 'reasonText', label: '状态', width: '110px' },
  { key: 'goText', label: '跳转', width: '150px' }
]

onMounted(() => {
  load()
  pollTimer = window.setInterval(() => {
    if (!manageDialogState.visible) load(true) // 表单弹窗打开时暂停轮询, 避免编辑中被覆盖
  }, 30_000)
})

onUnmounted(() => {
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-[var(--jc-space-3)]">
    <header class="flex items-center justify-between gap-[var(--jc-space-3)] flex-wrap shrink-0">
      <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap min-w-0">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">首页轮播</h2>
        <span class="text-muted text-xs">
          共 {{ active.length }} 位 · 前 {{ liveCount }} 位对外生效 · 8 榜前 2 权重混排 · 中国大陆保底 · 每 30 秒刷新
        </span>
      </div>
      <div class="flex gap-[var(--jc-space-2)]">
        <ManageButton variant="ghost" :loading="busy" :disabled="busy" @click="refreshHot">
          <BaseIcon name="refresh" size="16px" /> 立即刷新榜单
        </ManageButton>
        <ManageButton variant="ghost" @click="load()">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </ManageButton>
        <ManageButton variant="gradient" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </ManageButton>
      </div>
    </header>

    <ManageTabs variant="segment" :model-value="view" :tabs="tabs" @update:model-value="view = $event" />

    <BaseEmpty
      v-if="loadError"
      :title="`加载失败：${loadError}`"
      description="点击右上角「刷新」重试"
    />

    <!-- 生效位 + 候选位 -->
    <ManageTable
      v-else-if="view === 'active'"
      :columns="activeColumns"
      :rows="activeRows"
      row-key="_key"
      :loading="loading"
      empty="暂无生效轮播 —— 无可用配置且片库为空"
      actions-width="320px"
    >
      <template #cell="{ row, col }">
        <span v-if="col.key === '_index'" class="text-muted text-xs">{{ row._index + 1 }}</span>
        <BaseTag v-else-if="col.key === '_zone'" :variant="row._zone === 'live' ? 'success' : 'default'" size="sm">
          {{ row._zone === 'live' ? '生效' : '候选' }}
        </BaseTag>
        <div v-else-if="col.key === 'preview'" class="flex items-center gap-[var(--jc-space-1)]">
          <img
            v-if="row.image"
            :src="row.image"
            alt="横图"
            class="w-[80px] h-[45px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated cursor-zoom-in"
            @click="preview(row.image, row.name)"
          />
          <span v-else class="w-[80px] h-[45px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated">无横图</span>
          <img
            v-if="row.poster"
            :src="row.poster"
            alt="竖图"
            class="w-[30px] h-[45px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated cursor-zoom-in"
            @click="preview(row.poster, row.name)"
          />
          <span v-else class="w-[30px] h-[45px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated">无</span>
        </div>
        <div v-else-if="col.key === 'name'" class="flex flex-col gap-[var(--jc-space-1)] min-w-0">
          <span class="truncate">{{ row.name || '为你推荐' }}</span>
          <span class="text-muted text-xs truncate">{{ row.subtitle || (row.source === 'banner' ? '' : '热榜自动补入') }}</span>
          <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap">
            <BaseTag :variant="row.source === 'banner' ? 'success' : 'default'" size="sm">
              {{ row.source === 'banner' ? '手动' : '自动' }}
            </BaseTag>
            <BaseTag v-if="row.guaranteed" variant="warning" size="sm">大陆保底</BaseTag>
            <BaseTag v-if="row.source === 'fallback'" :variant="row.image ? 'success' : 'warning'" size="sm">
              {{ row.image ? '横图已就绪' : '横图待回填' }}
            </BaseTag>
          </div>
        </div>
        <span
          v-else-if="col.key === 'dbScore'"
          class="tabular-nums"
          :class="row.dbScore ? 'text-[var(--jc-warning)]' : 'text-muted'"
        >{{ row.dbScore ? row.dbScore.toFixed(1) : '—' }}</span>
        <span v-else-if="col.key === 'classTag'" class="text-xs text-secondary truncate block max-w-[130px]" :title="row.classTag || ''">{{ row.classTag || '—' }}</span>
        <span v-else-if="col.key === 'hotText'" class="text-xs text-secondary truncate block max-w-[140px]" :title="row.hotText">{{ row.hotText }}</span>
        <RouterLink
          v-else-if="col.key === 'goText' && canJump(row)"
          :to="{ path: '/filmDetail', query: { link: String(row.mid) } }"
          class="text-xs text-link hover:underline block truncate max-w-[140px]"
          :title="`查看影片详情 ${row.goText}`"
        >{{ row.goText }}</RouterLink>
        <span v-else-if="col.key === 'goText'" class="text-xs text-link block truncate max-w-[140px]" :title="row.goText">{{ row.goText }}</span>
        <span v-else>{{ row[col.key] }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end flex-wrap items-center">
          <a
            v-if="row.name"
            :href="doubanUrl(row.name)"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex w-[22px] h-[22px] rounded-[5px] overflow-hidden opacity-80 hover:opacity-100 shrink-0"
            :title="`豆瓣搜索「${row.name}」影评`"
          ><img src="/site-icons/douban.ico" alt="豆瓣" class="w-full h-full object-cover" /></a>
          <ManageButton
            v-if="(row.mid ?? 0) > 0"
            variant="ghost" size="sm"
            :loading="fillingMids.has(row.mid!)"
            title="手动触发 TMDB 检索并回填该片横图"
            @click="fillBackdrop(row.mid!)"
          >回填横图</ManageButton>
          <ManageButton
            variant="ghost" size="sm" :disabled="!canMoveUp(row, row._index) || busy"
            :title="row.source === 'banner' ? '上移' : '上移（转为手动位）'"
            @click="move(row._index, 'up')"
          >↑</ManageButton>
          <ManageButton
            variant="ghost" size="sm" :disabled="!canMoveDown(row, row._index) || busy"
            :title="row.source === 'banner' ? '下移' : '自动位恒在尾部'"
            @click="move(row._index, 'down')"
          >↓</ManageButton>
          <ManageButton
            v-if="row.source === 'banner'"
            variant="ghost" size="sm" :disabled="busy" @click="row.banner && openEdit(row.banner)"
          >编辑</ManageButton>
          <ManageButton v-else variant="ghost" size="sm" :disabled="busy" @click="adoptSlide(row, row._index)">编辑</ManageButton>
          <ManageButton
            variant="ghost" size="sm" :disabled="busy"
            :title="row.source === 'banner' ? '停用' : '停用（屏蔽该影片的自动补位）'"
            @click="row.source === 'banner' && row.banner ? toggleState(row.banner) : disableByMid(row.mid ?? 0)"
          >禁用</ManageButton>
          <ManageButton
            v-if="row.source === 'banner'"
            variant="danger" size="sm" :disabled="busy" @click="row.banner && remove(row.banner)"
          >删除</ManageButton>
        </div>
      </template>

      <template #mobile-card="{ row }">
        <div class="flex flex-col gap-[var(--jc-space-2)]">
          <div class="flex items-start gap-[var(--jc-space-3)]">
            <div class="flex gap-[var(--jc-space-1)] shrink-0">
              <img v-if="row.image" :src="row.image" alt="横图" class="w-[88px] h-[50px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated" />
              <span v-else class="w-[88px] h-[50px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated">无横图</span>
            </div>
            <div class="min-w-0 flex-1 flex flex-col gap-[var(--jc-space-1)]">
              <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap">
                <span class="text-muted text-xs">#{{ row._index + 1 }}</span>
                <span class="truncate">{{ row.name || '为你推荐' }}</span>
                <BaseTag :variant="row._zone === 'live' ? 'success' : 'default'" size="sm">{{ row._zone === 'live' ? '生效' : '候选' }}</BaseTag>
              </div>
              <span class="text-muted text-xs truncate">{{ row.subtitle || (row.source === 'banner' ? '' : '热榜自动补入') }}</span>
              <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap">
                <BaseTag :variant="row.source === 'banner' ? 'success' : 'default'" size="sm">{{ row.source === 'banner' ? '手动' : '自动' }}</BaseTag>
                <BaseTag v-if="row.guaranteed" variant="warning" size="sm">大陆保底</BaseTag>
                <BaseTag v-if="row.source === 'fallback'" :variant="row.image ? 'success' : 'warning'" size="sm">{{ row.image ? '横图已就绪' : '横图待回填' }}</BaseTag>
                <span v-if="row.dbScore" class="text-xs tabular-nums text-[var(--jc-warning)]">{{ row.dbScore.toFixed(1) }} 分</span>
                <span v-if="row.hotText !== '—'" class="text-xs text-secondary truncate">{{ row.hotText }}</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap justify-end border-t border-subtle pt-[var(--jc-space-2)]">
            <RouterLink
              v-if="canJump(row)"
              :to="{ path: '/filmDetail', query: { link: String(row.mid) } }"
              class="text-xs text-link hover:underline mr-auto"
            >查看详情</RouterLink>
            <span v-else class="text-xs text-secondary truncate mr-auto">{{ row.goText }}</span>
            <ManageButton v-if="(row.mid ?? 0) > 0" variant="ghost" size="sm" :loading="fillingMids.has(row.mid!)" @click="fillBackdrop(row.mid!)">回填横图</ManageButton>
            <ManageButton variant="ghost" size="sm" :disabled="!canMoveUp(row, row._index) || busy" @click="move(row._index, 'up')">↑</ManageButton>
            <ManageButton variant="ghost" size="sm" :disabled="!canMoveDown(row, row._index) || busy" @click="move(row._index, 'down')">↓</ManageButton>
            <ManageButton
              v-if="row.source === 'banner'"
              variant="ghost" size="sm" :disabled="busy" @click="row.banner && openEdit(row.banner)"
            >编辑</ManageButton>
            <ManageButton v-else variant="ghost" size="sm" :disabled="busy" @click="adoptSlide(row, row._index)">编辑</ManageButton>
            <ManageButton
              variant="ghost" size="sm" :disabled="busy"
              @click="row.source === 'banner' && row.banner ? toggleState(row.banner) : disableByMid(row.mid ?? 0)"
            >禁用</ManageButton>
            <ManageButton v-if="row.source === 'banner'" variant="danger" size="sm" :disabled="busy" @click="row.banner && remove(row.banner)">删除</ManageButton>
          </div>
        </div>
      </template>
    </ManageTable>

    <!-- 未生效配置行 -->
    <ManageTable
      v-else
      :columns="inactiveColumns"
      :rows="inactiveRows"
      row-key="id"
      empty="暂无未生效配置"
      actions-width="280px"
    >
      <template #cell="{ row, col }">
        <div v-if="col.key === 'preview'" class="flex items-center gap-[var(--jc-space-1)]">
          <img
            v-if="inactiveImage(row)"
            :src="inactiveImage(row)"
            :alt="inactiveName(row)"
            class="w-[80px] h-[45px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated opacity-70 cursor-zoom-in"
            @click="preview(inactiveImage(row), inactiveName(row))"
          />
          <span v-else class="w-[80px] h-[45px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated opacity-70">无图</span>
        </div>
        <div v-else-if="col.key === 'metaText'" class="flex flex-col gap-[var(--jc-space-1)] min-w-0">
          <span class="truncate text-secondary">{{ inactiveName(row) }}</span>
          <span class="text-muted text-xs truncate" :title="row.metaText">{{ row.metaText }}</span>
        </div>
        <span v-else-if="col.key === 'scoreText'" class="tabular-nums" :class="row.meta?.dbScore ? 'text-[var(--jc-warning)]' : 'text-muted'">{{ row.scoreText }}</span>
        <span v-else-if="col.key === 'hotText'" class="text-xs text-secondary truncate block max-w-[140px]" :title="row.hotText">{{ row.hotText }}</span>
        <span v-else-if="col.key === 'scheduleText'" class="text-muted text-xs whitespace-nowrap">{{ row.scheduleText || '—' }}</span>
        <span v-else-if="col.key === 'reasonText'">
          <BaseTag :variant="row.reasonVariant" size="sm">{{ row.reasonText }}</BaseTag>
        </span>
        <RouterLink
          v-else-if="col.key === 'goText' && inactiveCanJump(row)"
          :to="{ path: '/filmDetail', query: { link: String(row.mid) } }"
          class="text-xs text-link hover:underline block truncate max-w-[140px]"
          :title="`查看影片详情 ${row.goText}`"
        >{{ row.goText }}</RouterLink>
        <span v-else-if="col.key === 'goText'" class="text-xs text-link block truncate max-w-[140px]" :title="row.goText">{{ row.goText }}</span>
        <span v-else>{{ row[col.key] }}</span>
      </template>

      <template #actions="{ row }">
        <div class="flex gap-[var(--jc-space-1)] justify-end flex-wrap items-center">
          <a
            v-if="row.meta?.name || row.title"
            :href="doubanUrl(row.meta?.name || row.title || '')"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex w-[22px] h-[22px] rounded-[5px] overflow-hidden opacity-80 hover:opacity-100 shrink-0"
            :title="`豆瓣搜索「${row.meta?.name || row.title}」影评`"
          ><img src="/site-icons/douban.ico" alt="豆瓣" class="w-full h-full object-cover" /></a>
          <ManageButton
            v-if="row.mid > 0"
            variant="ghost" size="sm"
            :loading="fillingMids.has(row.mid)"
            @click="fillBackdrop(row.mid)"
          >回填横图</ManageButton>
          <ManageButton v-if="canEnable(row)" variant="ghost" size="sm" :disabled="busy" @click="toggleState(row)">启用</ManageButton>
          <ManageButton variant="ghost" size="sm" :disabled="busy" @click="openEdit(row)">编辑</ManageButton>
          <ManageButton variant="danger" size="sm" :disabled="busy" @click="remove(row)">删除</ManageButton>
        </div>
      </template>

      <template #mobile-card="{ row }">
        <div class="flex flex-col gap-[var(--jc-space-2)]">
          <div class="flex items-start gap-[var(--jc-space-3)]">
            <div class="w-[88px] h-[50px] shrink-0 rounded-[var(--jc-radius-sm)] overflow-hidden bg-elevated grid place-items-center opacity-70">
              <img v-if="inactiveImage(row)" :src="inactiveImage(row)" :alt="inactiveName(row)" class="w-full h-full object-cover" />
              <span v-else class="text-muted text-xs">无图</span>
            </div>
            <div class="min-w-0 flex-1 flex flex-col gap-[var(--jc-space-1)]">
              <span class="truncate text-secondary">{{ inactiveName(row) }}</span>
              <span class="text-muted text-xs truncate">{{ row.metaText }}</span>
              <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap">
                <BaseTag :variant="row.reasonVariant" size="sm">{{ row.reasonText }}</BaseTag>
                <span v-if="row.meta?.dbScore" class="text-xs tabular-nums text-[var(--jc-warning)]">{{ row.scoreText }} 分</span>
                <span v-if="row.hotText !== '—'" class="text-xs text-secondary truncate">{{ row.hotText }}</span>
                <span v-if="row.scheduleText" class="text-xs text-muted">{{ row.scheduleText }}</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-[var(--jc-space-1)] flex-wrap justify-end border-t border-subtle pt-[var(--jc-space-2)]">
            <RouterLink
              v-if="inactiveCanJump(row)"
              :to="{ path: '/filmDetail', query: { link: String(row.mid) } }"
              class="text-xs text-link hover:underline mr-auto"
            >查看详情</RouterLink>
            <span v-else class="text-xs text-secondary truncate mr-auto">{{ row.goText }}</span>
            <ManageButton v-if="row.mid > 0" variant="ghost" size="sm" :loading="fillingMids.has(row.mid)" @click="fillBackdrop(row.mid)">回填横图</ManageButton>
            <ManageButton v-if="canEnable(row)" variant="ghost" size="sm" :disabled="busy" @click="toggleState(row)">启用</ManageButton>
            <ManageButton variant="ghost" size="sm" :disabled="busy" @click="openEdit(row)">编辑</ManageButton>
            <ManageButton variant="danger" size="sm" :disabled="busy" @click="remove(row)">删除</ManageButton>
          </div>
        </div>
      </template>
    </ManageTable>

    <!-- 全图点击放大(生效/未生效所有封面共享一个 lightbox) -->
    <BaseLightbox v-model:src="previewSrc" :alt="previewAlt" />
  </div>
</template>