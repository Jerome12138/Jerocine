<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import BaseTag from '@/components/base/BaseTag.vue'
import type { DomainRuleProgress, DomainRuleView } from '@/api/manage/system'

/**
 * 替换执行进度（openManageDialog 内容组件，**只读**）。
 *
 * 执行是后端异步任务：本弹窗按 1s 轮询 /progress，展示 total/done/updated。
 * 页面关掉任务也会继续跑完，只是看不到进度。
 */
interface Props {
  rule: DomainRuleView
}
const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'confirm', progress: DomainRuleProgress): void
  (e: 'cancel'): void
}>()

const progress = ref<DomainRuleProgress>({
  ruleId: props.rule.id,
  field: props.rule.field,
  state: 'running',
  total: 0,
  done: 0,
  updated: 0,
  message: '准备中…'
})

let timer: number | undefined

const pct = computed(() => {
  const t = progress.value.total
  if (!t) return progress.value.state === 'done' ? 100 : 0
  return Math.min(100, Math.round((progress.value.done / t) * 100))
})

async function tick(): Promise<void> {
  try {
    const p = await manageApi.system.domainRuleProgress(props.rule.id)
    progress.value = p
    if (p.state === 'done' || p.state === 'failed') {
      stop()
      // 结束时主动提示一次（底部不再有关闭按钮，靠右上角 X 关）
      toast(p.state === 'done' ? 'success' : 'error', p.message || (p.state === 'done' ? '替换完成' : '执行失败'))
    }
  } catch {
    /* 网络抖动：下一轮继续 */
  }
}

function stop(): void {
  if (timer) {
    window.clearInterval(timer)
    timer = undefined
  }
}

onMounted(() => {
  void tick()
  timer = window.setInterval(tick, 1000)
})
onBeforeUnmount(stop)

</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-4)]">
    <!-- 概览 -->
    <div class="flex flex-col gap-[var(--jc-space-2)]">
      <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
        <BaseTag variant="purple" size="sm">{{ rule.fieldLabel || rule.field }}</BaseTag>
        <span class="text-xs text-secondary">{{ rule.scope === 'slave' ? '从站' : '主站' }}</span>
      </div>
      <p class="break-all text-sm font-[var(--jc-fw-medium)]">
        {{ rule.fromHost }} <span class="text-muted">→</span> {{ rule.toHost }}
      </p>
    </div>

    <!-- 进度 -->
    <div class="flex flex-col gap-[var(--jc-space-2)] rounded-[var(--jc-radius-md)] bg-[var(--jc-bg-subtle)] p-[var(--jc-space-4)]">
      <div class="flex items-baseline gap-[var(--jc-space-2)]">
        <span class="text-3xl font-[var(--jc-fw-semibold)] tabular-nums leading-none">{{ pct }}%</span>
        <span class="text-xs text-muted tabular-nums">
          {{ progress.done.toLocaleString() }} / {{ progress.total.toLocaleString() }} 行
        </span>
        <span class="ml-auto text-xs text-muted tabular-nums">已改动 {{ progress.updated.toLocaleString() }} 行</span>
      </div>

      <div class="h-2 w-full overflow-hidden rounded-full bg-[var(--jc-border-default)]">
        <div
          class="h-full rounded-full transition-all"
          :class="progress.state === 'failed' ? 'bg-[var(--jc-danger)]' : 'bg-[linear-gradient(90deg,var(--jc-brand-purple),var(--jc-brand-cyan))]'"
          :style="{ width: pct + '%' }"
        />
      </div>

      <p class="text-xs" :class="progress.state === 'failed' ? 'text-[var(--jc-danger)]' : 'text-muted'">
        {{ progress.message }}
      </p>
    </div>

    <p class="text-xs text-muted">
      每行都写了审计，可用反向规则回滚；关闭弹窗任务仍会继续执行。
    </p>
  </div>
</template>