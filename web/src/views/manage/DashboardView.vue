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
function userLabel(uid?: number): string {
  return uid && uid > 0 ? `用户 #${uid}` : '游客'
}

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

    <!-- 统计卡 (真实数据) -->
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-[var(--jc-space-4)]">
      <component
        :is="c.to ? RouterLink : 'article'"
        v-for="c in cards"
        :key="c.label"
        :to="c.to"
        class="bg-surface rounded-card shadow-card p-[var(--jc-space-5)] flex items-center gap-[var(--jc-space-4)] min-h-[120px]"
        :class="c.to ? 'hover:bg-elevated transition-colors' : ''"
      >
        <div
          class="w-[56px] h-[56px] rounded-full flex items-center justify-center text-white shrink-0"
          :class="['bg-gradient-to-br', c.tint]"
        >
          <BaseIcon :name="c.icon" size="28px" />
        </div>
        <div class="flex flex-col min-w-0">
          <span class="text-secondary text-sm">{{ c.label }}</span>
          <span class="text-[var(--jc-fs-3xl)] font-[var(--jc-fw-black)] tabular-nums">
            {{ c.value }}
          </span>
        </div>
      </component>
    </div>

    <!-- 在线实时统计: 客户端 30s 心跳, 90s 未上报判离线; UV 登录按 uid / 游客按 IP 去重 -->
    <section v-if="!loading && !error" class="flex flex-col gap-[var(--jc-space-4)]">
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-[var(--jc-space-4)]">
        <article class="bg-surface rounded-card shadow-card p-[var(--jc-space-5)] flex items-center gap-[var(--jc-space-4)] min-h-[120px]">
          <div class="w-[56px] h-[56px] rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#22c55e] to-[#4ad1e5]">
            <BaseIcon name="eye" size="28px" />
          </div>
          <div class="flex flex-col min-w-0">
            <span class="text-secondary text-sm">在线人数（UV）</span>
            <span class="text-[var(--jc-fs-3xl)] font-[var(--jc-fw-black)] tabular-nums">
              {{ online?.uv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card p-[var(--jc-space-5)] flex items-center gap-[var(--jc-space-4)] min-h-[120px]">
          <div class="w-[56px] h-[56px] rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#3b82f6] to-[#22c55e]">
            <BaseIcon name="menu" size="28px" />
          </div>
          <div class="flex flex-col min-w-0">
            <span class="text-secondary text-sm">在线会话（PV）</span>
            <span class="text-[var(--jc-fs-3xl)] font-[var(--jc-fw-black)] tabular-nums">
              {{ online?.pv ?? '—' }}
            </span>
          </div>
        </article>
        <article class="bg-surface rounded-card shadow-card p-[var(--jc-space-5)] flex items-center gap-[var(--jc-space-4)] min-h-[120px]">
          <div class="w-[56px] h-[56px] rounded-full flex items-center justify-center text-white shrink-0 bg-gradient-to-br from-[#E50914] to-[#f59e0b]">
            <BaseIcon name="play" size="28px" />
          </div>
          <div class="flex flex-col min-w-0">
            <span class="text-secondary text-sm">正在观看</span>
            <span class="text-[var(--jc-fs-3xl)] font-[var(--jc-fw-black)] tabular-nums">
              {{ online?.watching ?? '—' }}
            </span>
          </div>
        </article>
      </div>

      <!-- 在线明细表单: 最近活跃倒序, 90s 未心跳自动消失 -->
      <div class="bg-surface rounded-card shadow-card p-[var(--jc-space-5)]">
        <div class="flex items-center justify-between mb-[var(--jc-space-4)]">
          <h3 class="text-[var(--jc-fs-lg)] font-[var(--jc-fw-bold)]">在线明细</h3>
          <span class="text-secondary text-xs">90s 无心跳自动离线 · 15s 自动刷新</span>
        </div>
        <div v-if="online && online.sessions.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-secondary text-xs text-left border-b border-[var(--jc-border)]">
                <th class="py-2 pr-4 font-normal">IP</th>
                <th class="py-2 pr-4 font-normal">用户</th>
                <th class="py-2 pr-4 font-normal">状态</th>
                <th class="py-2 pr-4 font-normal">浏览器</th>
                <th class="py-2 pr-4 font-normal">进入时间</th>
                <th class="py-2 font-normal">最近活跃</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="s in online.sessions"
                :key="s.sid"
                class="border-b border-[var(--jc-border)]/50 last:border-0"
              >
                <td class="py-2 pr-4 tabular-nums text-secondary">{{ s.ip || '—' }}</td>
                <td class="py-2 pr-4 text-secondary">{{ userLabel(s.uid) }}</td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex items-center gap-1 text-xs"
                    :class="s.watching ? 'text-[#34d399]' : 'text-secondary'"
                  >
                    <span
                      class="w-1.5 h-1.5 rounded-full"
                      :class="s.watching ? 'bg-[#34d399]' : 'bg-[#9ca3af]'"
                    />
                    {{ s.watching ? '观看中' : '在线' }}
                  </span>
                </td>
                <td class="py-2 pr-4 text-secondary max-w-[180px] truncate" :title="s.ua">{{ shortUa(s.ua ?? '') }}</td>
                <td class="py-2 pr-4 tabular-nums text-secondary">{{ fmtTime(s.firstSeen) }}</td>
                <td class="py-2 tabular-nums text-secondary">{{ fmtTime(s.lastSeen) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <BaseEmpty v-else :title="'当前无人在线'" :description="'打开网站或播放器后 30s 内出现在这里'" />
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
