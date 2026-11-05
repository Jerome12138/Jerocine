<script setup lang="ts">
import { reactive } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { LiveSourceManage } from '@/types/live'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageFormActions from '@/components/manage/ManageFormActions.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 直播源新增/编辑表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', payload) | ('cancel'), footer 自带。
 *
 * 校验(名称/地址必填、http(s) 前缀)与提交自治: 拦截/失败 toast 后弹窗停留(输入不丢);
 * 成功 emit('confirm', 载荷), 由发起页负责重拉源列表与频道数。
 */
export interface LiveSourceFormPayload {
  id: number
  name: string
  url: string
  remark: string
  sort: number
  state: number
}

interface Props {
  /** 编辑目标; null/缺省 = 新增 */
  initial?: LiveSourceManage | null
}
const props = withDefaults(defineProps<Props>(), { initial: null })

const emit = defineEmits<{
  (e: 'confirm', payload: LiveSourceFormPayload): void
  (e: 'cancel'): void
}>()

const isEdit = !!props.initial

const form = reactive({
  name: props.initial?.name ?? '',
  url: props.initial?.url ?? '',
  remark: props.initial?.remark ?? '',
  // ManageInput number 清空后是 undefined（非数字原样保留），'' 送后端会绑定 422——按 0 兜底
  sort: (props.initial?.sort ?? 0) as number | undefined,
  state: props.initial?.state ?? 0
})

const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  if (!form.name.trim() || !form.url.trim()) {
    toast('warning', '名称与地址必填')
    return
  }
  const lu = form.url.trim().toLowerCase()
  if (!lu.startsWith('http://') && !lu.startsWith('https://')) {
    toast('warning', '地址需以 http(s):// 开头')
    return
  }
  // 小数静默取整（Go 端 Sort 为 int，1.5 同样 422）
  const sortRaw = typeof form.sort === 'number' && Number.isFinite(form.sort) ? Math.trunc(form.sort) : null
  const sort = sortRaw ?? 0
  if (sortRaw === null) toast('warning', '排序需为整数，已按 0 处理')
  void run({
    action: async () => {
      const payload = {
        name: form.name.trim(),
        url: form.url.trim(),
        remark: form.remark,
        sort,
        state: form.state
      }
      if (isEdit) {
        await manageApi.live.updateLiveSource(props.initial!.id, payload, { silent: true })
      } else {
        await manageApi.live.createLiveSource(payload, { silent: true })
      }
    },
    successMsg: isEdit ? '直播源已保存' : '直播源已添加',
    after: () =>
      emit('confirm', {
        id: props.initial?.id ?? 0,
        name: form.name.trim(),
        url: form.url.trim(),
        remark: form.remark,
        sort,
        state: form.state
      })
  })
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <ManageFormField label="名称" required>
      <ManageInput v-model="form.name" placeholder="如： IPTV 主源" :maxlength="64" />
    </ManageFormField>
    <ManageFormField label="m3u 播放列表地址" required>
      <ManageInput v-model="form.url" placeholder="http(s):// 开头的 .m3u / .m3u8 地址" :maxlength="1024" />
    </ManageFormField>
    <ManageFormField label="备注">
      <ManageTextarea v-model="form.remark" placeholder="可选；记录来源/更新方式等" :maxlength="255" />
    </ManageFormField>
    <ManageFormField label="排序" hint="越小越靠前">
      <ManageInput v-model="form.sort" type="number" />
    </ManageFormField>
    <ManageFormField label="状态">
      <label class="inline-flex items-center gap-[var(--jc-space-2)] text-sm">
        <ManageSwitch
          :model-value="form.state === 0"
          @update:model-value="form.state = $event ? 0 : 1"
        />
        <span :class="form.state === 0 ? 'text-primary' : 'text-muted'">
          {{ form.state === 0 ? '启用' : '停用' }}
        </span>
      </label>
    </ManageFormField>
    <!-- footer(§C8#2 收口 ManageFormActions): 本件无 <form>, 提交钮点击驱动(confirm-type=button) -->
    <ManageFormActions
      :busy="busy"
      confirm-text="保存"
      pt
      confirm-type="button"
      @cancel="cancel"
      @confirm="submit"
    />
  </div>
</template>
