<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import * as collectApi from '@/api/manage/collect'
import type { SpiderJob } from '@/api/manage/collect'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import { toast } from '@/api/http'
import { confirm } from '@/composables/useConfirm'

/**
 * 采集任务监控
 *  - 2 秒轮询 spiderJobs 接口
 *  - 每条任务显示进度条 + 已用时长 + 状态标签
 *  - running → 可暂停 / 取消, paused → 可继续 / 取消
 *  - 已结束 (done/error/canceled) 30 分钟后服务端 GC 自动消失
 */

const jobs = ref<SpiderJob[]>([])
const loading = ref(false)
let timer: number | undefined
const POLL_ACTIVE_MS = 5000  // 有正在跑/暂停的任务: 5s 一刷 (原来 2s 太频繁)
const POLL_IDLE_MS = 30_000  // 全是已完成: 30s 一刷, 等 GC 即可

const hasActive = (list: SpiderJob[]): boolean =>
  list.some((j) => j.state === 'running' || j.state === 'paused' || j.state === 'pending')

async function load(): Promise<void> {
  loading.value = true
  try {
    const r = await collectApi.spiderJobs()
    jobs.value = Array.isArray(r) ? r : []
  } finally {
    loading.value = false
  }
  // 根据当前是否有活跃任务动态调轮询节奏 — 用户反馈"任务都已完成还在轮询"
  schedulePoll()
}

function schedulePoll(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  const delay = hasActive(jobs.value) ? POLL_ACTIVE_MS : POLL_IDLE_MS
  timer = window.setTimeout(load, delay)
}

function stopPoll(): void {
  if (timer !== undefined) {
    window.clearTimeout(timer)
    timer = undefined
  }
}

onMounted(() => {
  void load() // load 内部会自调 schedulePoll
})
onBeforeUnmount(stopPoll)

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

function stateVariant(s: SpiderJob['state']): 'default' | 'brand' | 'warning' | 'success' | 'danger' {
  switch (s) {
    case 'running': return 'brand'
    case 'paused': return 'warning'
    case 'done': return 'success'
    case 'error':
    case 'canceled': return 'danger'
    default: return 'default'
  }
}

function stateText(s: SpiderJob['state']): string {
  return ({
    pending: '排队中',
    running: '采集中',
    paused: '已暂停',
    done: '已完成',
    error: '出错',
    canceled: '已取消'
  } as Record<string, string>)[s] ?? s
}

async function pause(id: string): Promise<void> {
  try {
    await collectApi.spiderJobPause(id)
    toast('success', '已暂停')
    void load()
  } catch (e) {
    toast('error', (e as Error).message ?? '暂停失败')
  }
}

async function resume(id: string): Promise<void> {
  try {
    await collectApi.spiderJobResume(id)
    toast('success', '已继续')
    void load()
  } catch (e) {
    toast('error', (e as Error).message ?? '继续失败')
  }
}

async function cancelJob(j: SpiderJob): Promise<void> {
  const ok = await confirm({
    title: '取消采集任务',
    desc: `确认取消 [${j.sourceName}] 的采集? 已完成的页面会保留, 未完成的页面会丢弃.`
  })
  if (!ok) return
  try {
    await collectApi.spiderJobCancel(j.sourceId)
    toast('success', '已取消')
    void load()
  } catch (e) {
    toast('error', (e as Error).message ?? '取消失败')
  }
}
</script>

<template>
  <div class="container-page py-[var(--gf-space-5)]">
    <header class="flex items-center justify-between mb-[var(--gf-space-5)]">
      <h1 class="text-2xl font-[var(--gf-fw-semibold)]">采集任务监控</h1>
      <div class="flex items-center gap-[var(--gf-space-3)]">
        <span class="text-muted text-sm" v-if="loading">刷新中…</span>
        <BaseButton variant="outline" size="sm" @click="load">手动刷新</BaseButton>
      </div>
    </header>

    <div v-if="jobs.length === 0" class="gf-tm-empty">
      暂无正在运行 / 最近 30 分钟内结束的采集任务. 去
      <RouterLink to="/manage/collect/index" class="text-link">采集源列表</RouterLink>
      启动一个.
    </div>

    <div v-else class="flex flex-col gap-[var(--gf-space-4)]">
      <article v-for="j in jobs" :key="j.sourceId" class="gf-job">
        <header class="gf-job__head">
          <span class="gf-job__name">{{ j.sourceName }}</span>
          <BaseTag :variant="stateVariant(j.state)" size="sm">{{ stateText(j.state) }}</BaseTag>
          <span class="text-muted text-xs ml-auto">
            {{ j.hour > 0 ? `近 ${j.hour}h` : '全量' }} · 用时 {{ fmtElapsed(j.elapsedMs) }}
          </span>
        </header>

        <div class="gf-job__bar">
          <div
            class="gf-job__bar-fill"
            :class="{ 'gf-job__bar-fill--paused': j.state === 'paused' }"
            :style="{ width: fmtPercent(j) + '%' }"
          />
          <span class="gf-job__bar-text">
            已完成 {{ j.donePages }} 页 / 共 {{ j.totalPages || '?' }} 页
            <template v-if="j.failedPages > 0">
              · <span class="text-danger">失败 {{ j.failedPages }} 页</span>
            </template>
            · 进度 {{ fmtPercent(j) }}%
          </span>
        </div>

        <div class="gf-job__actions">
          <BaseButton
            v-if="j.state === 'running'"
            variant="outline"
            size="sm"
            @click="pause(j.sourceId)"
          >暂停</BaseButton>
          <BaseButton
            v-if="j.state === 'paused'"
            variant="primary"
            size="sm"
            @click="resume(j.sourceId)"
          >继续</BaseButton>
          <BaseButton
            v-if="j.state === 'running' || j.state === 'paused'"
            variant="danger"
            size="sm"
            @click="cancelJob(j)"
          >取消</BaseButton>
          <span v-if="j.note" class="text-muted text-xs ml-auto">{{ j.note }}</span>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.gf-job {
  background: var(--gf-bg-elevated);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
  padding: var(--gf-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-3);
}
.gf-job__head {
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  flex-wrap: wrap;
}
.gf-job__name {
  font-weight: var(--gf-fw-semibold);
  font-size: var(--gf-fs-md);
}
.gf-job__bar {
  position: relative;
  height: 22px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: var(--gf-radius-sm);
  overflow: hidden;
}
.gf-job__bar-fill {
  position: absolute;
  inset: 0 auto 0 0;
  background: linear-gradient(90deg, var(--gf-brand-purple), var(--gf-brand-cyan));
  transition: width var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-job__bar-fill--paused {
  background: linear-gradient(90deg, #d97706, #f59e0b);
  opacity: 0.7;
}
.gf-job__bar-text {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-primary);
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.7);
}
.gf-job__actions {
  display: flex;
  gap: var(--gf-space-2);
  align-items: center;
}
.gf-tm-empty {
  padding: var(--gf-space-12) var(--gf-space-4);
  text-align: center;
  color: var(--gf-text-muted);
  background: var(--gf-bg-elevated);
  border: 1px dashed var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
}
</style>
