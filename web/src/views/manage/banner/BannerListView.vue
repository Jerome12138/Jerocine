<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { manageApi } from '@/api'
import type { Banner, BannerBoard, EffectiveSlide, InactiveBanner } from '@/types/manage'
import BaseButton from '@/components/base/BaseButton.vue'
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
 * 首页轮播管理 —— 列表 = 前 liveCount 位生效位 + 其后热榜候选位(共 heroBoardCount=16 位, liveCount 分区)。
 * 手动位(banner): 完整增删改 + 启用/停用 + 上下移;
 * 自动位(热榜补位): 派生数据, 支持编辑采纳(转手动钉位, 即"启用")、禁用屏蔽、上移采纳;
 *   铁律: mid 出现在 banner 表(无论启用与否)就不再被自动补位选中。
 *   热榜每日刷新会自动换掉未被编辑的候选位; 已采纳的手动行顺序持久化, 不受刷新影响。
 * 未生效区: 停用/缺图/未开始/已过期/超位的配置行, 可启用(转手动)/编辑/删除。
 * 横图 worker 只对生效位的影片回填 TMDB 横图; 轮播一有变动即触发补采。
 */

const board = ref<BannerBoard>({ active: [], inactive: [] })
const loading = ref(true)
/** 初次/手动加载失败时的错误文案(非 silent 加载才置位; 30s 轮询失败不打扰) */
const loadError = ref('')
const showInactive = ref(false)
/** 单片横图回填进行中的 mid 集合(防重复点击) */
const fillingMids = ref<Set<number>>(new Set())
let pollTimer: number | undefined

// 写操作统一 useAsyncAction: busy 全局防连点(行内按钮 :disabled)、confirm、失败 toast 单点。
// 各写请求一律 silent:true 关拦截器提示; 成功提示口径:
//   排序/启停/屏蔽不弹 toast(看板即时回读即反馈), 删除/保存/回填/榜单刷新有定制文案。
const { busy, run } = useAsyncAction()

/** 手动触发单片 TMDB 横图回填(轮播管理页「横图」按钮); 按 status 定制文案留在 action 内 */
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
      // refreshHotBoard 已 silent: true, 动态报告文案(带服务端观测值)由 action 自管
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
/** 对外生效位数(分区线: 之前 = 首页大图实时生效, 之后 = 候选位) */
const liveCount = computed(() => board.value.liveCount ?? 5)
/** 可见手动位数(上/下移边界用) */
const manualCount = computed(() => active.value.filter((s) => s.source === 'banner').length)

/** 图片点击放大(lightbox): 预览地址为空 = 关闭 */
const previewSrc = ref('')
const previewAlt = ref('')
function preview(url?: string, alt?: string): void {
  if (!url) return
  previewAlt.value = alt || ''
  previewSrc.value = url
}

/* ---- 未生效行元信息(需求⑥: 停用/缺图/过期行也要展示评分/标签/榜位/封面/链接) ---- */

/** 未生效行的展示名: 配置标题 > 关联影片元信息名 > 影片 #mid > 配置 id */
function inactiveName(row: InactiveBanner): string {
  return (
    row.title ||
    row.meta?.name ||
    (row.mid > 0 ? `影片 ${row.mid}` : `#${row.id}`)
  )
}

/** 未生效行可用横图(横图列专用): 配置横图 > 影片元信息横图。竖图不再占横图列(修复横竖颠倒) */
function inactiveImage(row: InactiveBanner): string {
  return row.image || row.meta?.backdrop || ''
}

/** 未生效行可用竖图(竖图列专用): 配置竖图 > 影片元信息封面 */
function inactivePoster(row: InactiveBanner): string {
  return row.poster || row.meta?.cover || ''
}

/** 未生效行豆瓣榜位文案 */
function inactiveHotLabel(row: InactiveBanner): string {
  const r = row.meta?.hotRank ?? 0
  if (!r) return '—'
  return `${row.meta?.hotBoard || '豆瓣'} No.${r}`
}

/** 未生效行是否可跳详情(关联了影片) */
function inactiveCanJump(row: InactiveBanner): boolean {
  return !row.link && !!row.mid && row.mid > 0
}
function inactiveGoLabel(row: InactiveBanner): string {
  if (row.link) return row.link
  if (row.mid > 0) return `影片 #${row.mid}`
  return '未配置'
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

/** 表单函数化: 标题逻辑留在页面, 校验/提交自治于 BannerFormDialog, 成功后重拉 Board */
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

/** 采纳自动位: 预填当前生效内容(含已回填横图), 保存后转手动位并钉在原位置 */
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

/** 生效位排序: 手动位换位 / 自动位上移采纳(后端返回新 Board) */
async function move(i: number, dir: 'up' | 'down'): Promise<void> {
  await run({
    action: async () => {
      board.value = await manageApi.banner.move(i, dir, { silent: true })
    }
  })
}

/** 启用/停用配置行(停用自动补位里的 mid = 屏蔽) */
async function toggleState(row: Banner): Promise<void> {
  await run({
    action: () =>
      manageApi.banner.save({ ...row, state: row.state === 0 ? 1 : 0 }, undefined, { silent: true }),
    after: () => load(true)
  })
}

/** 候选位「禁用」: 屏蔽影片的自动补位(后端返回新 Board) */
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
  if (s.source !== 'banner') return false // 自动位恒在尾部
  return i < manualCount.value - 1
}

/* ---- 展示辅助 ---- */

function goLabelOf(s: EffectiveSlide): string {
  if (s.link) return s.link
  if (s.mid && s.mid > 0) return `影片 #${s.mid}`
  return '未配置'
}

/** 跳转列可点击性: 关联影片的跳详情页; 自定义链接仅展示文本 */
function canJump(s: EffectiveSlide): boolean {
  return !s.link && !!s.mid && s.mid > 0
}

/** 豆瓣榜位列文案 */
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

/** ms → datetime-local 需要的 'YYYY-MM-DDTHH:mm' */
function msToLocal(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 未生效行排期文案: 让「未开始/已过期」的判断依据(何时起/止)可见。
 *  返回 'YYYY-MM-DD HH:mm 起/止/~ …'; 未配置排期返回空串 */
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

/** 未生效行能否「启用」: 纯屏蔽行(disableByMid 产生的无图停用行)启用必被服务端
 *  422 拒绝(启用中轮播至少需要横图或竖图), 按钮常假 —— 直接隐藏, 引导走编辑补图 */
function canEnable(row: InactiveBanner): boolean {
  if (row.state === 0) return false
  return !(row.reason === 'disabled' && !row.image && !row.poster)
}

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
  <div
    class="rounded-[var(--jc-radius-md)] border border-default bg-elevated p-[var(--jc-space-4)]"
  >
    <div class="flex items-center justify-between mb-[var(--jc-space-2)] flex-wrap gap-[var(--jc-space-2)]">
      <div class="flex items-center gap-[var(--jc-space-2)] flex-wrap">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">首页轮播</h2>
        <span class="text-muted text-xs">
          共 {{ active.length }} 位 · 前 {{ liveCount }} 位对外生效与首页大图实时一致 · 8 榜前 2 权重混排 · 中国大陆保底 · 每 30 秒刷新
        </span>
      </div>
      <div class="flex gap-[var(--jc-space-2)]">
        <BaseButton variant="ghost" size="sm" :loading="busy" :disabled="busy" @click="refreshHot">
          <BaseIcon name="refresh" size="16px" /> 立即刷新榜单
        </BaseButton>
        <BaseButton variant="ghost" size="sm" @click="load()">
          <BaseIcon name="refresh" size="16px" /> 刷新
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAdd">
          <BaseIcon name="plus" size="16px" /> 新增
        </BaseButton>
      </div>
    </div>

    <div v-if="loading" class="text-muted text-sm py-[var(--jc-space-6)] text-center">加载中…</div>
    <BaseEmpty
      v-else-if="loadError"
      :title="`加载失败：${loadError}`"
      description="点击右上角「刷新」重试"
    />
    <div
      v-else-if="!active.length"
      class="text-muted text-sm py-[var(--jc-space-6)] text-center"
    >
      暂无生效轮播 —— 无可用配置且片库为空
    </div>

    <!-- 生效位 + 候选位(共 10, liveCount 分区) -->
    <ul v-else class="flex flex-col">
      <template v-for="(s, i) in active" :key="`${s.source}-${s.bannerId ?? 0}-${s.mid ?? 0}-${i}`">
        <li
          v-if="i === liveCount && active.length > liveCount"
          class="flex items-center gap-[var(--jc-space-2)] py-[var(--jc-space-2)] text-muted text-xs border-b border-default"
        >
          <span class="shrink-0">▾ 以下为候选位</span>
          <span class="truncate">8 榜前 2 · 权重混排 · 中国大陆保底 —— 暂不对外生效，未采纳候选随每日榜单刷新换新(手动行保持不变)；「编辑」采纳即启用手动位，「禁用」永久屏蔽该影片</span>
        </li>
        <li
          class="flex items-center gap-[var(--jc-space-3)] py-[var(--jc-space-2)] border-b border-default last:border-b-0"
        >
        <span class="w-4 text-center text-muted text-xs shrink-0">{{ i + 1 }}</span>
        <!-- 横图列(生效中的宽幅主视觉) -->
        <img
          v-if="s.image"
          :src="s.image"
          alt="横图"
          class="w-[96px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 cursor-zoom-in"
          @click="preview(s.image, s.name)"
        />
        <span
          v-else
          class="w-[96px] h-[54px] grid place-items-center text-muted text-xs rounded-[var(--jc-radius-sm)] bg-elevated shrink-0"
        >无横图</span>
        <!-- 竖图列(窄屏兜底海报) -->
        <img
          v-if="s.poster"
          :src="s.poster"
          alt="竖图"
          class="w-[36px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 cursor-zoom-in"
          @click="preview(s.poster, s.name)"
        />
        <span
          v-else
          class="w-[36px] h-[54px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated shrink-0"
        >无</span>
        <div class="flex flex-col min-w-0 flex-1">
          <span class="truncate">{{ s.name || '为你推荐' }}</span>
          <span class="text-muted text-xs truncate">
            {{ s.subtitle || (s.source === 'banner' ? '' : '热榜自动补入') }}
          </span>
        </div>
        <!-- 影片元信息(与首页大图描述行同源; 无元信息显示 —) -->
        <span
          class="w-[44px] text-center text-xs tabular-nums shrink-0"
          :class="s.dbScore ? 'text-[var(--jc-warning)]' : 'text-muted'"
          title="豆瓣评分"
        >{{ s.dbScore ? s.dbScore.toFixed(1) : '—' }}</span>
        <span
          class="w-[96px] text-xs text-secondary truncate hidden xl:inline shrink-0"
          :title="s.classTag || ''"
        >{{ s.classTag || '—' }}</span>
        <span
          class="w-[120px] text-xs text-secondary truncate hidden lg:inline shrink-0"
          :title="hotLabelOf(s)"
        >{{ hotLabelOf(s) }}</span>
        <BaseTag :variant="s.source === 'banner' ? 'success' : 'default'" size="sm" class="shrink-0">
          {{ s.source === 'banner' ? '手动' : '自动' }}
        </BaseTag>
        <!-- 大陆保底: 候选池里无中国大陆影片时, 用高分大陆片替换同分类最低分槽(需求②) -->
        <BaseTag v-if="s.guaranteed" variant="warning" size="sm" class="shrink-0" title="中国大陆保底位">
          大陆保底
        </BaseTag>
        <BaseTag
          v-if="s.source === 'fallback'"
          :variant="s.image ? 'success' : 'warning'"
          size="sm"
          class="hidden md:inline-flex shrink-0"
        >
          {{ s.image ? '横图已就绪' : '横图待回填' }}
        </BaseTag>
        <!-- 跳转列: 关联影片可点进详情页; 自定义链接仅展示 -->
        <RouterLink
          v-if="canJump(s)"
          :to="{ path: '/filmDetail', query: { link: String(s.mid) } }"
          class="text-xs text-link hover:underline w-[120px] truncate hidden lg:inline-block shrink-0"
          :title="`查看影片详情 ${goLabelOf(s)}`"
        >{{ goLabelOf(s) }}</RouterLink>
        <span v-else class="text-xs text-link w-[120px] truncate hidden lg:block shrink-0" :title="goLabelOf(s)">{{ goLabelOf(s) }}</span>
        <div class="flex gap-[var(--jc-space-1)] shrink-0 items-center">
          <a
            v-if="s.name"
            :href="doubanUrl(s.name)"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex w-[22px] h-[22px] rounded-[5px] overflow-hidden opacity-80 hover:opacity-100 shrink-0"
            :title="`豆瓣搜索「${s.name}」影评`"
          ><img src="/site-icons/douban.ico" alt="豆瓣" class="w-full h-full object-cover" /></a>
          <BaseButton
            v-if="(s.mid ?? 0) > 0"
            variant="ghost" size="sm"
            :loading="fillingMids.has(s.mid!)"
            title="手动触发 TMDB 检索并回填该片横图"
            @click="fillBackdrop(s.mid!)"
          >回填横图</BaseButton>
          <BaseButton
            variant="ghost" size="sm" :disabled="!canMoveUp(s, i) || busy"
            :title="s.source === 'banner' ? '上移' : '上移（转为手动位）'"
            @click="move(i, 'up')"
          >↑</BaseButton>
          <BaseButton
            variant="ghost" size="sm" :disabled="!canMoveDown(s, i) || busy"
            :title="s.source === 'banner' ? '下移' : '自动位恒在尾部'"
            @click="move(i, 'down')"
          >↓</BaseButton>
          <BaseButton
            v-if="s.source === 'banner'"
            variant="ghost" size="sm" :disabled="busy" @click="s.banner && openEdit(s.banner)"
          >编辑</BaseButton>
          <BaseButton v-else variant="ghost" size="sm" :disabled="busy" @click="adoptSlide(s, i)">编辑</BaseButton>
          <BaseButton
            variant="ghost" size="sm" :disabled="busy"
            :title="s.source === 'banner' ? '停用' : '停用（屏蔽该影片的自动补位）'"
            @click="s.source === 'banner' && s.banner ? toggleState(s.banner) : disableByMid(s.mid ?? 0)"
          >禁用</BaseButton>
          <BaseButton
            v-if="s.source === 'banner'"
            variant="danger" size="sm" :disabled="busy" @click="s.banner && remove(s.banner)"
          >删除</BaseButton>
        </div>
        </li>
      </template>
    </ul>

    <!-- 未生效配置行(折叠区) -->
    <div v-if="!loading && inactive.length" class="border-t border-default mt-[var(--jc-space-1)]">
      <button
        type="button"
        class="w-full flex items-center gap-[var(--jc-space-1)] py-[var(--jc-space-2)] text-muted text-xs"
        @click="showInactive = !showInactive"
      >
        <span>{{ showInactive ? '▾' : '▸' }}</span>
        <span>未生效（{{ inactive.length }} · 不参与展示与自动补位）</span>
      </button>
      <ul v-if="showInactive" class="flex flex-col pb-[var(--jc-space-2)]">
        <li
          v-for="row in inactive"
          :key="row.id"
          class="flex items-center gap-[var(--jc-space-3)] py-[var(--jc-space-2)] border-b border-default last:border-b-0"
        >
          <!-- 封面(配置图优先, 缺则用关联影片元信息封面; 需求⑥全图可点击放大) -->
          <img
            v-if="inactiveImage(row)"
            :src="inactiveImage(row)"
            :alt="inactiveName(row)"
            class="w-[96px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60 cursor-zoom-in"
            @click="preview(inactiveImage(row), inactiveName(row))"
          />
          <span
            v-else
            class="w-[96px] h-[54px] grid place-items-center text-muted text-xs rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60"
          >无图</span>
          <!-- 竖图列(与生效行对齐): 配置竖图, 缺则用影片封面 -->
          <img
            v-if="inactivePoster(row)"
            :src="inactivePoster(row)"
            alt="竖图"
            class="w-[36px] h-[54px] object-cover rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60 cursor-zoom-in hidden md:block"
            @click="preview(inactivePoster(row), inactiveName(row))"
          />
          <span
            v-else
            class="w-[36px] h-[54px] grid place-items-center text-muted text-[10px] rounded-[var(--jc-radius-sm)] bg-elevated shrink-0 opacity-60 hidden md:grid"
          >无</span>
          <div class="flex flex-col min-w-0 flex-1">
            <span class="truncate text-secondary">{{ inactiveName(row) }}</span>
            <span class="text-muted text-xs truncate">
              <template v-if="row.meta?.dbScore || row.meta?.year || row.meta?.area">
                <span :class="row.meta?.dbScore ? 'text-[var(--jc-warning)]' : ''">
                  {{ row.meta?.dbScore ? row.meta.dbScore.toFixed(1) + ' 分' : '' }}
                </span>
                <span v-if="row.meta?.year || row.meta?.area">
                  {{ row.meta?.dbScore ? ' · ' : '' }}{{ [row.meta?.year, row.meta?.area].filter(Boolean).join(' · ') }}
                </span>
                <span v-if="row.meta?.remarks"> · {{ row.meta.remarks }}</span>
              </template>
              <span v-else>无关联影片元信息</span>
            </span>
          </div>
          <!-- 排期列(需求⑦: 未开始/已过期的判断依据要能看到时间) -->
          <span
            v-if="row.startAt || row.endAt"
            class="text-muted text-xs whitespace-nowrap shrink-0"
            title="排期（生效起 ~ 生效止）"
          >{{ scheduleLabel(row) }}</span>
          <!-- 评分列(与生效行同宽) -->
          <span
            class="w-[44px] text-center text-xs tabular-nums shrink-0"
            :class="row.meta?.dbScore ? 'text-[var(--jc-warning)]' : 'text-muted'"
            title="豆瓣评分"
          >{{ row.meta?.dbScore ? row.meta.dbScore.toFixed(1) : '—' }}</span>
          <span
            class="w-[96px] text-xs text-secondary truncate hidden xl:inline shrink-0"
            :title="row.meta?.classTag || ''"
          >{{ row.meta?.classTag || '—' }}</span>
          <span
            class="w-[120px] text-xs text-secondary truncate hidden lg:inline shrink-0"
            :title="inactiveHotLabel(row)"
          >{{ inactiveHotLabel(row) }}</span>
          <BaseTag :variant="reasonStyle[row.reason].variant" size="sm" class="shrink-0">
            {{ reasonStyle[row.reason].text }}
          </BaseTag>
          <!-- 跳转列: 关联影片可点进详情; 自定义链接仅展示 -->
          <RouterLink
            v-if="inactiveCanJump(row)"
            :to="{ path: '/filmDetail', query: { link: String(row.mid) } }"
            class="text-xs text-link hover:underline w-[120px] truncate hidden lg:inline-block shrink-0"
            :title="`查看影片详情 ${inactiveGoLabel(row)}`"
          >{{ inactiveGoLabel(row) }}</RouterLink>
          <span v-else class="text-xs text-link w-[120px] truncate hidden lg:block shrink-0" :title="inactiveGoLabel(row)">{{ inactiveGoLabel(row) }}</span>
          <div class="flex gap-[var(--jc-space-1)] shrink-0 items-center">
            <a
              v-if="row.meta?.name || row.title"
              :href="doubanUrl(row.meta?.name || row.title || '')"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex w-[22px] h-[22px] rounded-[5px] overflow-hidden opacity-80 hover:opacity-100 shrink-0"
              :title="`豆瓣搜索「${row.meta?.name || row.title}」影评`"
            ><img src="/site-icons/douban.ico" alt="豆瓣" class="w-full h-full object-cover" /></a>
            <BaseButton
              v-if="row.mid > 0"
              variant="ghost" size="sm"
              :loading="fillingMids.has(row.mid)"
              title="手动触发 TMDB 检索并回填该片横图"
              @click="fillBackdrop(row.mid)"
            >回填横图</BaseButton>
            <BaseButton
              v-if="canEnable(row)"
              variant="ghost" size="sm" title="启用后转为手动位"
              :disabled="busy"
              @click="toggleState(row)"
            >启用</BaseButton>
            <BaseButton variant="ghost" size="sm" :disabled="busy" @click="openEdit(row)">编辑</BaseButton>
            <BaseButton variant="danger" size="sm" :disabled="busy" @click="remove(row)">删除</BaseButton>
          </div>
        </li>
      </ul>
    </div>
  </div>

  <!-- 轮播新增/编辑/采纳表单已函数化: openForm → openManageDialog + BannerFormDialog(见 script) -->

  <!-- 全图点击放大(生效/未生效所有封面共享一个 lightbox) -->
  <BaseLightbox v-model:src="previewSrc" :alt="previewAlt" />
</template>
