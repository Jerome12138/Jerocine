<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { manageApi } from '@/api'
import type { DashboardStat, OnlineOverview } from '@/types/manage'
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

// ---- 在线实时统计(独立接口, 15s 轮询): UV 按登录用户/IP 去重, PV=活跃会话数 ----
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

function fmtTime(sec: number): string {
  return new Date(sec * 1000).toLocaleTimeString('zh-CN', { hour12: false })
}
function shortUa(ua: string): string {
  const s = ua.replace(/\s+/g, ' ').trim()
  return s.length > 36 ? `${s.slice(0, 36)}…` : s
}
function userLabel(s: { uid?: number; username?: string }): string {
  const name = (s.username ?? '').trim()
  if (name) return name
  return s.uid && s.uid > 0 ? `用户 #${s.uid}` : '游客'
}

/** 在线/在播放分开显示: 状态筛选 tab */
const statusFilter = ref<'all' | 'watching' | 'online'>('all')
const filteredSessions = computed(() => {
  if (!online.value) return []
  if (statusFilter.value === 'all') return online.value.sessions
  const want = statusFilter.value === 'watching'
  return online.value.sessions.filter((s) => s.watching === want)
})

const cards = computed(() => {
  if (!data.value) return []
  const d = data.value
  return [
    { label: '影片总数', value: d.filmCount ?? 0, icon: 'film', tint: 'from-[#9b49e7] to-[#4ad1e5]' },
    { label: '今日新增', value: d.todayNew ?? 0, icon: 'plus', tint: 'from-[#f59e0b] to-[#fbbf24]' },
    { label: '近 7 天新增', value: d.weekNew ?? 0, icon: 'film', tint: 'from-[#3b82f6] to-[#22c55e]' },
    { label: '采集源', value: d.collectCount ?? 0, icon: 'magic', tint: 'from-[#E50914] to-[#ff6b6b]' },
    { label: '定时任务', value: d.cronCount ?? 0, icon: 'clock', tint: 'from-[#22c55e] to-[#4ad1e5]' },
    { label: '已停采源', value: d.downSources ?? 0, icon: 'trash', tint: 'from-[#ef4444] to-[#f59e0b]' },
    {
      label: '待补采页',
      value: d.pendingFails ?? 0,
      icon: 'refresh',
      tint: 'from-[#ef4444] to-[#f59e0b]',
      to: '/manage/collect/failures'
    }
  ]
})
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-6)]">
    <header class="flex items-center justify-between">
      <h2 class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-bold)]">仪表盘</h2>
      <button
        type="button"
        class="text-sm text-link hover:underline min-h-[44px] px-[var(--jc-space-3)]"
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
          <span class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tabular-nums">
            {{ c.value }}
          </span>
        </div>
      </component>
    </div>

    <!-- 在线实时统计: 客户端 30s 心跳, 90s 未上报判离线; UV 登录按 uid / 游客按 IP 去重 -->
    <section v-if="!loading && !error" class="flex flex-col gap-[var(--jc-space-3)]">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-[var(--jc-space-3)]">
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#22c55e] to-[#4ad1e5]">
            <BaseIcon name="eye" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">在线人数（UV）</span>
            <span class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tabular-nums">
              {{ online?.uv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card px-[var(--jc-space-3)] py-[var(--jc-space-2)] flex items-center gap-[var(--jc-space-2)] min-h-[76px]">
          <div class="w-9 h-9 rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#3b82f6] to-[#22c55e]">
            <BaseIcon name="menu" size="18px" />
          </div>
          <div class="flex flex-col min-w-0 leading-tight">
            <span class="text-secondary text-xs">在线会话（PV）</span>
            <span class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tabular-nums">
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
            <span class="text-[var(--jc-fs-2xl)] font-[var(--jc-fw-black)] tabular-nums">
              {{ online?.watching ?? '—' }}
            </span>
          </div>
        </article>
      </div>

      <!-- 在线明细表单: 最近活跃倒序, 90s 无心跳自动消失; 可筛选"在播放/在线"分开查看 -->
      <div class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-3)]">
          <div class="flex items-center gap-[var(--jc-space-3)]">
            <h3 class="text-[var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">在线明细</h3>
            <div class="flex rounded-full bg-elevated p-0.5 text-xs">
              <button
                type="button"
                class="px-3 py-1 rounded-full min-h-[28px] transition-colors"
                :class="statusFilter === 'all' ? 'bg-surface shadow text-primary' : 'text-secondary'"
                @click="statusFilter = 'all'"
              >
                全部
              </button>
              <button
                type="button"
                class="px-3 py-1 rounded-full min-h-[28px] transition-colors"
                :class="statusFilter === 'watching' ? 'bg-surface shadow text-primary' : 'text-secondary'"
                @click="statusFilter = 'watching'"
              >
                在播放
              </button>
              <button
                type="button"
                class="px-3 py-1 rounded-full min-h-[28px] transition-colors"
                :class="statusFilter === 'online' ? 'bg-surface shadow text-primary' : 'text-secondary'"
                @click="statusFilter = 'online'"
              >
                在线
              </button>
            </div>
          </div>
          <span class="text-secondary text-xs">90s 无心跳自动离线 · 15s 自动刷新</span>
        </div>
        <div v-if="filteredSessions.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-secondary text-xs text-left border-b border-[var(--jc-border)]">
                <th class="py-2 pr-4 font-normal">IP / 归属地</th>
                <th class="py-2 pr-4 font-normal">用户</th>
                <th class="py-2 pr-4 font-normal">状态</th>
                <th class="py-2 pr-4 font-normal">页面</th>
                <th class="py-2 pr-4 font-normal">浏览器</th>
                <th class="py-2 pr-4 font-normal">进入时间</th>
                <th class="py-2 font-normal">最近活跃</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="s in filteredSessions"
                :key="s.sid"
                class="border-b border-[var(--jc-border)]/50 last:border-0"
              >
                <td class="py-2 pr-4 tabular-nums text-secondary">
                  <span class="inline-block">{{ s.ip || '—' }}</span>
                  <span v-if="s.ipRegion" class="block text-xs text-tertiary">{{ s.ipRegion }}</span>
                </td>
                <td class="py-2 pr-4 text-secondary">{{ userLabel(s) }}</td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex items-center gap-1 text-xs"
                    :class="s.watching ? 'text-[#34d399]' : 'text-[#3b82f6]'"
                  >
                    <span
                      class="w-1.5 h-1.5 rounded-full"
                      :class="s.watching ? 'bg-[#34d399]' : 'bg-[#3b82f6]'"
                    />
                    {{ s.watching ? '在播放' : '在线' }}
                  </span>
                </td>
                <td class="py-2 pr-4 text-secondary max-w-[140px] truncate" :title="s.path">{{ s.path || '—' }}</td>
                <td class="py-2 pr-4 text-secondary max-w-[180px] truncate" :title="s.ua">{{ shortUa(s.ua ?? '') }}</td>
                <td class="py-2 pr-4 tabular-nums text-secondary">{{ fmtTime(s.firstSeen) }}</td>
                <td class="py-2 tabular-nums text-secondary">{{ fmtTime(s.lastSeen) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <BaseEmpty v-else :title="statusFilter === 'all' ? '当前无人在线' : '该状态下暂无会话'" :description="'打开网站或播放器后 30s 内出现在这里'" />
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
