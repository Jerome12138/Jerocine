<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { manageApi } from '@/api'
import type { DashboardStat, OnlineOverview, OnlineSession } from '@/types/manage'
import * as telemetryApi from '@/api/manage/telemetry'
import type { FilmStat, OverviewResp } from '@/api/manage/telemetry'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BasePagination from '@/components/base/BasePagination.vue'
import ManageTabs from '@/components/manage/ManageTabs.vue'
import type { TabItem } from '@/components/manage/ManageTabs.vue'
import { formatShortDateTime } from '@/utils/format'
import { useClientPagedList } from '@/composables/useClientPagedList'

const data = ref<DashboardStat | null>(null)
const loading = ref(true)
const error = ref('')

// ---- 埋点数据(与仪表盘同次加载, 独立容错): 内容热度 Top5 + 站点健康度 ----
const hotFilms = ref<FilmStat[]>([])
const mostFavorited = ref<FilmStat[]>([])
const teleOv = ref<OverviewResp | null>(null)

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  const results = await Promise.allSettled([
    manageApi.system.dashboard(),
    telemetryApi.hotFilms(7, 5),
    telemetryApi.mostFavorited(5),
    telemetryApi.overview(7)
  ])
  if (results[0].status === 'fulfilled') {
    data.value = results[0].value
  } else {
    error.value = results[0].reason instanceof Error ? results[0].reason.message : '加载失败'
  }
  // 埋点数据失败静默(区块显示空态), 不影响主看板
  if (results[1].status === 'fulfilled') hotFilms.value = results[1].value
  if (results[2].status === 'fulfilled') mostFavorited.value = results[2].value
  if (results[3].status === 'fulfilled') teleOv.value = results[3].value
  loading.value = false
  // 「今日/近7天」明细只在此前切换时拉取过一次, 刷新时顺带重拉当前时间段(实时视图走 15s 轮询)
  if (timeFilter.value !== 'live') void loadDaily(timeFilter.value === 'week' ? 7 : 1)
}
onMounted(load)

// ---- 在线实时统计(独立接口, 15s 轮询): UV 按登录用户/IP 去重, PV=窗口内页面访问次数(不去重) ----
const online = ref<OnlineOverview | null>(null)
let onlineTimer: number | undefined
async function loadOnline(): Promise<void> {
  try {
    online.value = await manageApi.system.onlineOverview()
  } catch {
    /* 轮询失败静默, 等下次 */
  }
}
onMounted(() => {
  void loadOnline()
  onlineTimer = window.setInterval(() => void loadOnline(), 15_000)
})
onUnmounted(() => {
  if (onlineTimer !== undefined) window.clearInterval(onlineTimer)
})

// ---- 今日/近7天访问用户明细: 切换时拉取一次, 排序(在播→在线→最近活跃)由后端保证 ----
const dailySessions = ref<OnlineSession[]>([])
const dailyError = ref('')
async function loadDaily(days: number): Promise<void> {
  // 先清空再拉取: 不沿用上一时间段的明细(切换瞬间与失败时都以空态呈现, 避免数据错标)
  dailySessions.value = []
  dailyError.value = ''
  try {
    const resp = await manageApi.system.onlineDailySessions(days)
    dailySessions.value = resp.sessions
  } catch (e) {
    dailyError.value = e instanceof Error ? e.message : '明细加载失败'
  }
}

function fmtCount(n: number): string {
  if (n >= 10000) return `${(n / 10000).toFixed(1)}万`
  return String(n)
}
/** 会话访问过的页面(新数据用 paths 多值, 旧数据回落到单值 path) */
function pathsOf(s: OnlineSession): string[] {
  if (s.paths && s.paths.length) return s.paths
  return s.path ? [s.path] : []
}

/** 机器人请求按原因分布(悬停显示): 合计 = 今日机器人请求 PV */
const BOT_REASON_LABEL: Record<string, string> = {
  ua: 'UA 关键词',
  webdriver: '无头浏览器',
  header: '缺浏览器头',
  datacenter: '机房 IP',
  behavior: '行为异常'
}
const botBreakdown = computed(() => {
  const m = online.value?.today?.botReasons
  if (!m) return ''
  return Object.entries(m)
    .sort((a, b) => b[1] - a[1])
    .map(([k, v]) => `${BOT_REASON_LABEL[k] ?? k}：${v}`)
    .join('\n')
})

function shortUa(ua: string): string {
  const s = ua.replace(/\s+/g, ' ').trim()
  return s.length > 36 ? `${s.slice(0, 36)}…` : s
}
function userLabel(s: { uid?: number; username?: string }): string {
  const name = (s.username ?? '').trim()
  if (name) return name
  return s.uid && s.uid > 0 ? `用户 #${s.uid}` : '游客'
}

/** 时间范围: 近 30 分钟活跃 / 今日 / 近7天 */
const timeFilter = ref<'live' | 'today' | 'week'>('live')
function switchTime(t: 'live' | 'today' | 'week'): void {
  if (timeFilter.value === t) return
  timeFilter.value = t
  if (t !== 'live') void loadDaily(t === 'week' ? 7 : 1)
}

/** 在线/在播放分开显示: 状态筛选 tab */
const statusFilter = ref<'all' | 'watching' | 'online'>('all')

// 在线明细两组 segment 页签数据源(D②: 手写胶囊迁 ManageTabs, 切换逻辑 switchTime/statusFilter 不动);
// 文案与 value 与原手写版逐字一致, 泛型收窄保证 emit 值可直接喂 switchTime/回填 ref
const TIME_TABS: TabItem<'live' | 'today' | 'week'>[] = [
  { value: 'live', label: '近 30 分钟' },
  { value: 'today', label: '今日' },
  { value: 'week', label: '近 7 天' }
]
const STATUS_TABS: TabItem<'all' | 'watching' | 'online'>[] = [
  { value: 'all', label: '全部' },
  { value: 'watching', label: '在播放' },
  { value: 'online', label: '在线' }
]
const filteredSessions = computed(() => {
  const base = timeFilter.value === 'live' ? online.value?.sessions ?? [] : dailySessions.value
  if (statusFilter.value === 'all') return base
  // 按三态筛选: "在线"不能混入已离线的行(历史明细里 30 分钟窗口外的会话)
  const want = statusFilter.value === 'watching' ? 'watching' : 'online'
  return base.filter((s) => sessionState(s, timeFilter.value !== 'live') === want)
})

// ---- 在线明细分页: 每页 10 条; 切换时间/状态回到第 1 页, 数据收缩时钳制页码
// (§C8#3 收口 useClientPagedList: 切片+末页钳制样板单点化, 筛选回页仍由本件 watch 负责) ----
const PAGE_SIZE = 10
const { page, paged: pagedSessions } = useClientPagedList(filteredSessions, PAGE_SIZE)
watch([timeFilter, statusFilter], () => {
  page.value = 1
})

/** 会话状态三态: 在播(绿) / 在线(蓝) / 已离线(灰, 仅历史明细) */
function sessionState(s: OnlineSession, historical: boolean): 'watching' | 'online' | 'offline' {
  // 历史(今日/近7天): 服务端 online 为 true 才算在线 —— 缺省与明确 false 都判已离线
  // (服务端曾因 omitempty 丢掉 false, 按 !== true 判定对两种线上形态都成立);
  // 实时明细无 online 字段(30 分钟窗口内), 均视为在线。
  if (historical) {
    if (s.online !== true) return 'offline'
  } else if (s.online === false) {
    return 'offline'
  }
  if (s.watching) return 'watching'
  return 'online'
}

const cards = computed(() => {
  if (!data.value) return []
  const d = data.value
  return [
    { label: '影片总数', value: d.filmCount ?? 0, icon: 'film', tint: 'from-[#9b49e7] to-[#4ad1e5]' },
    { label: '今日新增', value: d.todayNew ?? 0, icon: 'plus', tint: 'from-[#f59e0b] to-[#fbbf24]' },
    { label: '近 7 天新增', value: d.weekNew ?? 0, icon: 'film', tint: 'from-[#3b82f6] to-[#22c55e]' },
    { label: '启用采集源', value: d.collectCount ?? 0, icon: 'magic', tint: 'from-[#E50914] to-[#ff6b6b]' },
    { label: '定时任务', value: d.cronCount ?? 0, icon: 'clock', tint: 'from-[#22c55e] to-[#4ad1e5]' },
    {
      label: '待补采页',
      value: d.pendingFails ?? 0,
      icon: 'refresh',
      tint: 'from-[#ef4444] to-[#f59e0b]',
      to: '/manage/collect/failures'
    },
    {
      label: '7 天异常事件',
      value: teleOv.value?.errorCount ?? '—',
      icon: 'info',
      tint: 'from-[#f97316] to-[#ef4444]',
      to: '/manage/telemetry'
    },
    {
      label: 'API 平均耗时(7天)',
      value: teleOv.value ? `${Math.round(teleOv.value.avgApiMs)}ms` : '—',
      icon: 'chart',
      tint: 'from-[#06b6d4] to-[#3b82f6]',
      to: '/manage/telemetry'
    }
  ]
})
</script>

<template>
  <div class="flex min-h-0 flex-col gap-[var(--jc-space-6)]">
    <header class="flex items-center justify-between">
      <h2 class="text-[length:var(--jc-fs-2xl)] font-[var(--jc-fw-bold)]">仪表盘</h2>
      <button
        type="button"
        class="jc-hotarea text-[length:var(--jc-ctrl-fs)] text-link hover:underline min-h-[var(--jc-ctrl-h)] px-[var(--jc-ctrl-px)]"
        @click="load"
      >
        刷新
      </button>
    </header>

    <!-- loading 骨架 -->
    <div v-if="loading" class="grid grid-cols-1 md:grid-cols-3 gap-[var(--jc-space-4)]">
      <BaseSkeleton v-for="i in 3" :key="i" shape="rect" :height="'120px'" />
    </div>

    <!-- 错误 -->
    <BaseEmpty v-else-if="error" :title="error" :description="'点击右上角刷新重试'" />

    <!-- 统计卡 (真实数据): 紧凑布局, 提升信息密度 -->
    <div v-else class="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-[var(--jc-space-3)]">
      <component
        :is="c.to ? RouterLink : 'article'"
        v-for="c in cards"
        :key="c.label"
        :to="c.to"
        class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]"
        :class="c.to ? 'hover:bg-elevated transition-colors' : ''"
      >
        <div
          class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0"
          :class="['bg-gradient-to-br', c.tint]"
        >
          <BaseIcon :name="c.icon" size="18px" />
        </div>
        <div class="flex flex-col min-w-0 leading-tight">
          <span class="text-secondary text-xs truncate">{{ c.label }}</span>
          <!-- 可点击卡片: 数字用链接色提示可跳转 -->
          <span
            class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate"
            :class="c.to ? 'text-link' : ''"
          >
            {{ c.value }}
          </span>
        </div>
      </component>
    </div>

    <!-- 在线统计: 客户端 30s 心跳, 30 分钟内有过活动算在线; UV 登录按 uid / 游客按 IP 去重 -->
    <section v-if="!loading && !error" class="flex flex-col gap-[var(--jc-space-3)]">
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-[var(--jc-space-3)]">
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#22c55e] to-[#4ad1e5]">
            <BaseIcon name="eye" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">活跃人数（UV · 30分钟）</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.uv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#3b82f6] to-[#22c55e]">
            <BaseIcon name="menu" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">页面访问（PV · 30分钟）</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.pv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#E50914] to-[#f59e0b]">
            <BaseIcon name="play" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">正在观看</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.watching ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#8b5cf6] to-[#ec4899]">
            <BaseIcon name="eye" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">机器人来源（30分钟）</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.bots ?? '—' }}
            </span>
          </div>
        </article>
      </div>

      <!-- 今日累计: 自然日滚动, 与实时并列为运营日报维度 -->
      <div class="flex items-center gap-2">
        <span class="text-xs text-secondary whitespace-nowrap">今日累计</span>
        <div class="h-px flex-1 bg-[var(--jc-border)]" />
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-[var(--jc-space-3)]">
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#8b5cf6] to-[#ec4899]">
            <BaseIcon name="user" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">今日人数（UV）</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.today?.uv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#06b6d4] to-[#3b82f6]">
            <BaseIcon name="refresh" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">今日访问（PV）</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.today?.pv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#f59e0b] to-[#ef4444]">
            <BaseIcon name="chart" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">今日峰值活跃</span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.today?.peak ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#475569] to-[#64748b]">
            <BaseIcon name="clock" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs" :title="botBreakdown || '今日暂无机器人请求'">
              机器人请求 PV{{ online?.today?.botReasons ? ' ⓘ' : '' }}
            </span>
            <span class="text-[length:var(--jc-fs-lg)] md:text-[length:var(--jc-fs-xl)] font-[var(--jc-fw-black)] tabular-nums truncate">
              {{ online?.today?.botPV ?? '—' }}
            </span>
          </div>
        </article>
      </div>

      <!-- 在线明细表单: 最近活跃倒序, 30min 无心跳自动消失; 可筛选"在播放/在线"分开查看 -->
      <div class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-3)] flex-wrap gap-2">
          <div class="flex items-center gap-[var(--jc-space-3)] flex-wrap">
            <h3 class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">在线明细</h3>
            <!-- 时间范围: 实时 / 今日 / 近7天 (D②: 手写胶囊迁 ManageTabs segment, 视觉逐像素照抄) -->
            <ManageTabs
              variant="segment"
              :model-value="timeFilter"
              :tabs="TIME_TABS"
              @update:model-value="switchTime"
            />
            <!-- 状态筛选: 全部 / 在播放 / 在线 -->
            <ManageTabs
              variant="segment"
              :model-value="statusFilter"
              :tabs="STATUS_TABS"
              @update:model-value="statusFilter = $event"
            />
          </div>
          <span class="hidden md:inline text-secondary text-xs">
            {{ timeFilter === 'live' ? '近 30 分钟有活动算在线 · 15s 自动刷新' : (timeFilter === 'today' ? '今日访问用户 · 在播/在线优先' : '近 7 天访问用户 · 在播/在线优先') }}
          </span>
        </div>
        <div v-if="filteredSessions.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-secondary text-xs text-left border-b border-[var(--jc-border)]">
                <!-- 各列 min-width: 窄屏下表格横向滚动, 列不被内容挤窄 -->
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[160px]">IP / 归属地</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[100px]">用户</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[72px]">状态</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[84px]">访问次数</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[160px]">页面</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[160px]">浏览器</th>
                <th class="py-2 pr-4 font-normal whitespace-nowrap min-w-[88px]">进入时间</th>
                <th class="py-2 font-normal whitespace-nowrap min-w-[88px]">最近活跃</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="s in pagedSessions"
                :key="`${s.sid}|${s.ip}`"
                class="border-b border-[var(--jc-border)]/50 last:border-0"
              >
                <td class="py-2 pr-4 tabular-nums text-secondary">
                  <span class="inline-block">{{ s.ip || '—' }}</span>
                  <span v-if="s.ipRegion" class="block text-xs text-tertiary">{{ s.ipRegion }}</span>
                </td>
                <td class="py-2 pr-4 text-secondary max-w-[160px] truncate">{{ userLabel(s) }}</td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex items-center gap-1 text-xs"
                    :class="sessionState(s, timeFilter !== 'live') === 'watching' ? 'text-[#34d399]' : (sessionState(s, timeFilter !== 'live') === 'offline' ? 'text-[#94a3b8]' : 'text-[#3b82f6]')"
                  >
                    <span
                      class="w-1.5 h-1.5 rounded-full"
                      :class="sessionState(s, timeFilter !== 'live') === 'watching' ? 'bg-[#34d399]' : (sessionState(s, timeFilter !== 'live') === 'offline' ? 'bg-[#94a3b8]' : 'bg-[#3b82f6]')"
                    />
                    {{ sessionState(s, timeFilter !== 'live') === 'watching' ? '在播放' : (sessionState(s, timeFilter !== 'live') === 'offline' ? '已离线' : '在线') }}
                  </span>
                </td>
                <td class="py-2 pr-4 tabular-nums text-secondary">{{ s.visits ?? '—' }}</td>
                <td class="py-2 pr-4 text-secondary max-w-[220px]">
                  <span
                    v-if="pathsOf(s).length"
                    class="block truncate cursor-help"
                    :title="pathsOf(s).join('\n')"
                  >
                    {{ pathsOf(s).join('、') }}
                  </span>
                  <span v-else>—</span>
                </td>
                <td class="py-2 pr-4 text-secondary max-w-[180px] truncate" :title="s.ua">{{ shortUa(s.ua ?? '') }}</td>
                <td class="py-2 pr-4 tabular-nums text-secondary">{{ formatShortDateTime(s.firstSeen * 1000) }}</td>
                <td class="py-2 tabular-nums text-secondary">{{ formatShortDateTime(s.lastSeen * 1000) }}</td>
              </tr>
            </tbody>
          </table>
          <div v-if="filteredSessions.length > PAGE_SIZE" class="mt-3 flex justify-end">
            <BasePagination v-model:current="page" :total="filteredSessions.length" :page-size="PAGE_SIZE" />
          </div>
        </div>
        <BaseEmpty
          v-if="filteredSessions.length === 0"
          :title="dailyError ? `明细加载失败：${dailyError}` : (statusFilter === 'all' ? (timeFilter === 'live' ? '近 30 分钟无访问' : (timeFilter === 'today' ? '今日暂无访问' : '近 7 天暂无访问')) : '该状态下暂无会话')"
          :description="dailyError ? '点击右上角「刷新」重试' : (timeFilter === 'live' ? '30 分钟内有过访问就会出现在这里' : '在播/在播放的用户会优先展示在前面')"
        />
      </div>
    </section>

    <!-- 内容热度: 埋点访问量(近7天)与收藏量 Top5, 点击跳影片详情 -->
    <section v-if="!loading && !error" class="grid md:grid-cols-2 gap-[var(--jc-space-4)]">
      <div class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-3)]">
          <h3 class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">热点视频 Top 5</h3>
          <RouterLink to="/manage/telemetry" class="text-xs text-link hover:underline">查看全部</RouterLink>
        </div>
        <div v-if="hotFilms.length" class="flex flex-col">
          <RouterLink
            v-for="(f, i) in hotFilms"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="flex items-center gap-[var(--jc-space-2)] py-1.5 rounded min-h-[52px] hover:bg-elevated transition-colors"
          >
            <span class="w-5 text-secondary text-sm shrink-0">{{ i + 1 }}</span>
            <!-- BaseImage 内部是 width:100% 的比例盒, 需要外层固定宽度的包装(与全站海报用法一致) -->
            <div class="w-[36px] shrink-0">
              <BaseImage
                :src="f.cover"
                :alt="f.name"
                ratio="40/54"
                rounded="rounded-[var(--jc-radius-sm)]"
              />
            </div>
            <span class="flex-1 truncate text-sm">{{ f.name }}</span>
            <span class="text-secondary text-xs shrink-0">{{ fmtCount(f.count) }} 次</span>
          </RouterLink>
        </div>
        <BaseEmpty v-else title="暂无热点数据" description="近 7 天无影片访问埋点" />
      </div>
      <div class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-3)]">
          <h3 class="text-[length:var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">收藏最多 Top 5</h3>
          <RouterLink to="/manage/telemetry" class="text-xs text-link hover:underline">查看全部</RouterLink>
        </div>
        <div v-if="mostFavorited.length" class="flex flex-col">
          <RouterLink
            v-for="(f, i) in mostFavorited"
            :key="f.mid"
            :to="{ path: '/filmDetail', query: { link: String(f.mid) } }"
            class="flex items-center gap-[var(--jc-space-2)] py-1.5 rounded min-h-[52px] hover:bg-elevated transition-colors"
          >
            <span class="w-5 text-secondary text-sm shrink-0">{{ i + 1 }}</span>
            <div class="w-[36px] shrink-0">
              <BaseImage
                :src="f.cover"
                :alt="f.name"
                ratio="40/54"
                rounded="rounded-[var(--jc-radius-sm)]"
              />
            </div>
            <span class="flex-1 truncate text-sm">{{ f.name }}</span>
            <span class="text-secondary text-xs shrink-0">{{ fmtCount(f.count) }} 收藏</span>
          </RouterLink>
        </div>
        <BaseEmpty v-else title="暂无收藏数据" description="尚无用户收藏影片" />
      </div>
    </section>

    <!-- 提示: 更多图表/活动流待后端 -->
    <section v-if="!loading && !error" class="bg-surface rounded-card p-[var(--jc-space-5)]">
      <p class="text-secondary text-sm">
        <BaseIcon name="info" size="14px" class="inline align-middle mr-1" />
        更多统计图表（近 7 天新增 / 最近操作流等）需后端支持，待补。
      </p>
    </section>
  </div>
</template>
