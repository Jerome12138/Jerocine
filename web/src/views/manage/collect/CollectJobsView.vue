<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import type { SpiderJob } from '@/api/manage/collect'
import BaseButton from '@/components/base/BaseButton.vue'
import StatusTag from '@/components/manage/StatusTag.vue'
import type { StatusMap } from '@/components/manage/StatusTag.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 采集任务监控
 *  - 轮询 spiderJobs 接口: 有活跃任务 5s / 全部结束 30s; 轮询失败不中断链条(下轮继续)
 *  - 每条任务显示进度条 + 已用时长 + 状态标签, error 态展示服务端落库的失败原因
 *  - running → 可暂停 / 取消, paused → 可继续 / 取消
 *  - 已结束 (done/error/canceled) 30 分钟后服务端 GC 自动消失
 *  - 操作经 useAsyncAction: busy 防连点 + 单点 toast(写请求 silent, 不与拦截器叠提示)
 */

// 任务状态 → StatusTag 词汇表(与 TaskManageView 的 JOB_STATE_MAP 同源口径)
const JOB_STATE_MAP: StatusMap = {
  running: { label: '采集中', variant: 'brand' },
  paused: { label: '已暂停', variant: 'warning' },
  done: { label: '已完成', variant: 'success' },
  error: { label: '出错', variant: 'danger' },
  canceled: { label: '已取消', variant: 'danger' }
}

const jobs = ref<SpiderJob[]>([])
const loading = ref(false)
const loadFailed = ref(false) // 最近一次轮询失败: 提示"显示的是上次数据", 不落入空态误导
let timer: number | undefined
const POLL_ACTIVE_MS = 5000  // 有正在跑/暂停的任务: 5s 一刷 (原来 2s 太频繁)
const POLL_IDLE_MS = 30_000  // 全是已完成: 30s 一刷, 等 GC 即可

// 服务端只有 running/paused/canceled/done/error 五态, 无排队态(cache/jobprogress.go)
const hasActive = (list: SpiderJob[]): boolean =>
  list.some((j) => j.state === 'running' || j.state === 'paused')

async function load(): Promise<void> {
  loading.value = true
  try {
    const r = await collectApi.spiderJobs()
    jobs.value = sortJobs(Array.isArray(r) ? r : [])
    loadFailed.value = false
  } catch {
    // 轮询失败只标记错误态, 保留上次数据; 续链由 pollTick 无条件执行, 不会断
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

// 排序: 运行中/暂停置顶, 其余(已结束)在后; 组内按开始时间倒序(未知时间排最后)。
// 服务端按 Redis SCAN 顺序返回, 不排序则卡片顺序跨轮询漂移, 难扫读。
function sortJobs(list: SpiderJob[]): SpiderJob[] {
  const active = (s: SpiderJob['state']): number => (s === 'running' || s === 'paused' ? 0 : 1)
  return [...list].sort((a, b) => {
    const byActive = active(a.state) - active(b.state)
    if (byActive !== 0) return byActive
    return b.startedAt - a.startedAt
  })
}

// 续链与取数分离: 取数失败也必须无条件续排, 否则一次网络抖动就永久停轮询
function pollTick(): void {
  void load()
  schedulePoll()
}

function schedulePoll(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  const delay = hasActive(jobs.value) ? POLL_ACTIVE_MS : POLL_IDLE_MS
  timer = window.setTimeout(pollTick, delay)
}

function stopPoll(): void {
  if (timer !== undefined) {
    window.clearTimeout(timer)
    timer = undefined
  }
}

onMounted(pollTick)
onBeforeUnmount(stopPoll)

// 操作进行中: busy 期间禁用全部卡片按钮(经 useAsyncAction 单点收敛)
const { busy, run } = useAsyncAction()

function fmtElapsed(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '-'
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  if (h > 0) return `${h}h ${m % 60}m ${s % 60}s`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
}

function fmtPercent(j: SpiderJob): number {
  if (!j.totalPages) return 0
  return Math.min(100, Math.floor((j.donePages / j.totalPages) * 100))
}

// 时长标签: hour=-1 全量 / >0 增量; 0 是服务端"未知"(旧数据), 不冒充"全量"
function hourLabel(hour: number): string {
  if (hour === -1) return '全量'
  if (hour > 0) return `近 ${hour}h`
  return '时长未知'
}

async function pause(j: SpiderJob): Promise<void> {
  await run({
    action: () => collectApi.spiderJobPause(j.sourceId, { silent: true }),
    successMsg: '已暂停',
    after: load
  })
}

async function resume(j: SpiderJob): Promise<void> {
  await run({
    action: () => collectApi.spiderJobResume(j.sourceId, { silent: true }),
    successMsg: '已继续',
    after: load
  })
}

async function cancelJob(j: SpiderJob): Promise<void> {
  await run({
    // 破坏性操作(未完成页面会丢弃)走危险样式
    confirm: {
      title: '取消采集任务',
      desc: `确认取消 [${j.sourceName}] 的采集? 已完成的页面会保留, 未完成的页面会丢弃.`,
      danger: true
    },
    action: () => collectApi.spiderJobCancel(j.sourceId, { silent: true }),
    successMsg: '已取消',
    after: load
  })
}

const hasJobs = computed(() => jobs.value.length > 0)
</script>

<template>
  <div class="container-page py-[var(--jc-space-5)]">
    <header class="flex items-center justify-between mb-[var(--jc-space-5)]">
      <h1 class="text-2xl font-[var(--jc-fw-semibold)]">采集任务监控</h1>
      <div class="flex items-center gap-[var(--jc-space-3)]">
        <span class="text-muted text-sm" v-if="loading">刷新中…</span>
        <BaseButton variant="outline" size="sm" @click="load">手动刷新</BaseButton>
      </div>
    </header>

    <!-- 有旧数据但上次刷新失败: 顶部浅提示, 列表仍展示 -->
    <div v-if="loadFailed && hasJobs" class="jc-job__stale text-center mb-[var(--jc-space-3)]">
      上次刷新失败, 以下为最近一次数据.
    </div>

    <div v-if="loadFailed && !hasJobs" class="jc-tm-empty">
      加载失败, 暂时拿不到任务列表.
      <BaseButton variant="outline" size="sm" class="mt-[var(--jc-space-3)]" @click="pollTick">点击重试</BaseButton>
    </div>

    <div v-else-if="!loadFailed && !hasJobs" class="jc-tm-empty">
      暂无正在运行 / 最近 30 分钟内结束的采集任务. 去
      <RouterLink to="/manage/collect" class="text-link">采集源列表</RouterLink>
      启动一个.
    </div>

    <div v-else class="flex flex-col gap-[var(--jc-space-4)]">
      <article v-for="j in jobs" :key="j.sourceId" class="jc-job">
        <header class="jc-job__head">
          <span class="jc-job__name">{{ j.sourceName }}</span>
          <StatusTag :value="j.state" :map="JOB_STATE_MAP" />
          <span class="text-muted text-xs ml-auto">
            {{ hourLabel(j.hour) }} · 用时 {{ fmtElapsed(j.elapsedMs) }}
          </span>
        </header>

        <div class="jc-job__bar">
          <div
            class="jc-job__bar-fill"
            :class="{ 'jc-job__bar-fill--paused': j.state === 'paused' }"
            :style="{ width: fmtPercent(j) + '%' }"
          />
          <span class="jc-job__bar-text">
            已完成 {{ j.donePages }} 页 / 共 {{ j.totalPages || '?' }} 页
            <template v-if="j.failedPages > 0">
              · <span class="text-danger">失败 {{ j.failedPages }} 页</span>
            </template>
            · 进度 {{ fmtPercent(j) }}%
          </span>
        </div>

        <!-- 失败原因: 服务端 JobFail 落库, 排障不必再跳 /manage/tasks -->
        <div v-if="j.state === 'error' && j.error" class="jc-job__error">
          <span class="flex-1 break-all">{{ j.error }}</span>
        </div>

        <div class="jc-job__actions">
          <BaseButton
            v-if="j.state === 'running'"
            variant="outline"
            size="sm"
            :disabled="busy"
            @click="pause(j)"
          >暂停</BaseButton>
          <BaseButton
            v-if="j.state === 'paused'"
            variant="primary"
            size="sm"
            :disabled="busy"
            @click="resume(j)"
          >继续</BaseButton>
          <BaseButton
            v-if="j.state === 'running' || j.state === 'paused'"
            variant="danger"
            size="sm"
            :disabled="busy"
            @click="cancelJob(j)"
          >取消</BaseButton>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.jc-job {
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
  padding: var(--jc-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-3);
}
.jc-job__head {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  flex-wrap: wrap;
}
.jc-job__name {
  font-weight: var(--jc-fw-semibold);
  font-size: var(--jc-fs-md);
}
.jc-job__bar {
  position: relative;
  height: 22px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: var(--jc-radius-sm);
  overflow: hidden;
}
.jc-job__bar-fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: linear-gradient(90deg, var(--jc-brand-purple), var(--jc-brand-cyan));
  transition: width var(--jc-dur-base) var(--jc-ease-standard);
}
.jc-job__bar-fill--paused {
  background: linear-gradient(90deg, #d97706, #f59e0b);
  opacity: 0.7;
}
.jc-job__bar-text {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-primary);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.7);
}
.jc-job__actions {
  display: flex;
  gap: var(--jc-space-2);
  align-items: center;
}
/* 失败原因红色错误框(样式同 TaskManageView 的 jc-job__error) */
.jc-job__error {
  display: flex;
  align-items: flex-start;
  gap: var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  color: var(--jc-color-danger, #ef4444);
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--jc-radius-sm);
  padding: var(--jc-space-2) var(--jc-space-3);
}
/* 有旧数据但上次刷新失败: 顶部一条浅提示 */
.jc-job__stale {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  background: var(--jc-bg-elevated);
  border: 1px solid var(--jc-border-subtle);
  border-radius: var(--jc-radius-sm);
  padding: var(--jc-space-2) var(--jc-space-3);
}
.jc-tm-empty {
  padding: var(--jc-space-12) var(--jc-space-4);
  text-align: center;
  color: var(--jc-text-muted);
  background: var(--jc-bg-elevated);
  border: 1px dashed var(--jc-border-subtle);
  border-radius: var(--jc-radius-md);
}
</style>
