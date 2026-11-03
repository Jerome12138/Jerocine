<script setup lang="ts">
import { reactive } from 'vue'
import * as telemetryApi from '@/api/manage/telemetry'
import type { TelemetryEventRow } from '@/api/manage/telemetry'
import { toast } from '@/api/http'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 标记问题已解决表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', true) | ('cancel'), footer 自带。
 *
 * 含独立校验与提交: commitId 必填前端先行拦截(服务端同样校验, 空值 422),
 * 提交失败弹窗停留(输入不丢); 提交成功后 emit('confirm'), 由发起页负责重拉列表。
 */
const props = defineProps<{ row: TelemetryEventRow }>()
const emit = defineEmits<{
  (e: 'confirm', payload: boolean): void
  (e: 'cancel'): void
}>()

// 预填: 重标解决(reopened)场景回显上次解决记录
const form = reactive({
  commitId: props.row.resolution?.resolvedCommitId ?? '',
  note: props.row.resolution?.note ?? ''
})

const { busy, run } = useAsyncAction()

function submit(): void {
  const commitId = form.commitId.trim()
  if (!commitId) {
    toast('error', '请填写修复 commit ID')
    return
  }
  void run({
    action: () =>
      telemetryApi.resolveIssue(
        {
          category: props.row.category,
          path: props.row.path,
          label: props.row.label,
          commitId,
          note: form.note.trim()
        },
        { silent: true }
      ),
    successMsg: '已标记解决',
    after: () => emit('confirm', true)
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-3)]" @submit.prevent="submit">
    <div class="text-sm text-secondary">
      <div><b>分类:</b> {{ row.category }}</div>
      <div><b>路径:</b> {{ row.path || '-' }}</div>
      <div><b>描述:</b> {{ row.label }}</div>
    </div>
    <ManageFormField label="修复 commit ID" required hint="git log 拿首 7-12 位">
      <ManageInput v-model="form.commitId" placeholder="e.g. c68eff0" />
    </ManageFormField>
    <ManageFormField label="备注 (可选)">
      <ManageInput v-model="form.note" placeholder="改了什么 / 为什么" />
    </ManageFormField>
    <p class="text-xs text-muted">
      标记后立即从活跃列表移除. 24 小时内的重复触发视为旧缓存/旧 deploy 遗留, 不算
      重开. 24 小时后到 30 天内若再次出现 → 自动变 "已重启" 重回活跃列表; 30 天都没
      再出现则状态变 "过期", 仍不显示.
    </p>
    <div class="flex justify-end gap-[var(--jc-space-3)] pt-[var(--jc-space-2)]">
      <ManageButton variant="ghost" type="button" @click="emit('cancel')">取消</ManageButton>
      <ManageButton variant="primary" type="submit" :loading="busy">保存</ManageButton>
    </div>
  </form>
</template>
