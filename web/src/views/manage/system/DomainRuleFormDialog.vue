<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageFormActions from '@/components/manage/ManageFormActions.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import type { DomainRuleView } from '@/api/manage/system'

/**
 * 域名替换规则 —— **手工录入**表单（openManageDialog 内容组件）。
 *
 * 为什么需要手工录入：源站公告只覆盖它自己发过的域名；历史规则（原站 rep.html 时代）
 * 或我们自己发现的脏域名，只能人工补。
 *
 * 口径与解析器一致（服务端用 macrep.NormalizeHost 再校验一遍）：
 *   只填域名本身，不带 http://、路径、端口。
 *   封面(vod_pic)只可能来自主站 ⇒ 范围锁定主站；播放/下载可选主站或从站。
 */
/**
 * initial 有值 = **编辑**已有规则（PUT 原地改，不新增行）；
 * 无值 = 新增（拉取之外的补充规则）。
 */
interface Props {
  initial?: DomainRuleView | null
}
const props = withDefaults(defineProps<Props>(), { initial: null })
const isEdit = computed(() => !!props.initial)

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const fieldOptions = [
  { label: '视频封面（vod_pic）', value: 'vod_pic' },
  { label: '视频播放（vod_play_url）', value: 'vod_play_url' },
  { label: '视频下载（vod_down_url）', value: 'vod_down_url' }
]
const scopeOptions = [
  { label: '主站', value: 'master' },
  { label: '从站', value: 'slave' }
]

/** datetime-local 需要 'YYYY-MM-DDTHH:mm'（本地时区） */
function nowLocal(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

/** 毫秒 → datetime-local 需要的本地串 */
function toLocalInput(ms: number): string {
  if (!ms) return nowLocal()
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}

const form = reactive({
  field: props.initial?.field ?? 'vod_pic',
  scope: props.initial?.scope ?? 'master',
  sourceCode: props.initial?.sourceCode ?? '',
  fromHost: props.initial?.fromHost ?? '',
  toHost: props.initial?.toHost ?? '',
  at: toLocalInput(props.initial?.announcedAt ?? 0),
  note: props.initial?.sourceNote ?? ''
})

const slaves = ref<{ label: string; value: string }[]>([])
const isPic = computed(() => form.field === 'vod_pic')
const slaveOptions = computed(() => [{ label: '全部从站', value: '' }, ...slaves.value])

watch(
  () => form.field,
  () => {
    if (isPic.value) {
      form.scope = 'master'
      form.sourceCode = ''
    }
  }
)

onMounted(async () => {
  try {
    const list = await manageApi.collect.list()
    slaves.value = list
      .filter((s) => Number(s.grade) === 1)
      .map((s) => ({ label: s.name || s.id, value: s.id }))
  } catch {
    /* 拉不到从站列表不阻塞：仍可保存主站规则 */
  }
})

const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  const from = form.fromHost.trim()
  const to = form.toHost.trim()
  if (!from || !to) {
    toast('warning', '新旧域名都必填')
    return
  }
  if (/^https?:\/\//i.test(from) || /^https?:\/\//i.test(to)) {
    toast('warning', '只填域名本身，不要带 http:// 或 https://')
    return
  }
  if (/[/:?#]/.test(from) || /[/:?#]/.test(to)) {
    toast('warning', '域名里不能带路径、端口或参数')
    return
  }
  const at = form.at ? new Date(form.at).getTime() : NaN
  if (!Number.isFinite(at)) {
    toast('warning', '公告日期不合法')
    return
  }
  const payload = {
    field: form.field,
    scope: isPic.value ? 'master' : form.scope,
    sourceCode: !isPic.value && form.scope === 'slave' ? form.sourceCode : '',
    fromHost: from,
    toHost: to,
    announcedAt: at,
    sourceNote: form.note.trim()
  }
  void run({
    action: async () => {
      if (props.initial) {
        await manageApi.system.updateDomainRule(props.initial.id, payload)
      } else {
        await manageApi.system.createDomainRule(payload)
      }
    },
    successMsg: isEdit.value ? '规则已更新' : '规则已保存',
    after: () => emit('confirm')
  })
}
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-3)]">
    <p class="text-xs text-secondary">
      {{ isEdit ? '编辑这条规则（原地修改，不会新增一行；改了新旧域名会重置为待处理）。' : '用于公告页没有、或需要人工修正的规则。保存只写规则表，' }}
      <strong v-if="!isEdit">不会改动影片数据</strong>。
    </p>

    <ManageFormField label="替换字段" required>
      <ManageSelect v-model="form.field" :options="fieldOptions" aria-label="替换字段" />
    </ManageFormField>

    <ManageFormField
      label="生效范围"
      :hint="isPic ? '封面只可能来自主站，已锁定' : '播放/下载地址可能来自主站或某个从站'"
      required
    >
      <div class="flex flex-wrap items-center gap-[var(--jc-space-2)]">
        <ManageSelect
          v-model="form.scope"
          :options="scopeOptions"
          :disabled="isPic"
          aria-label="生效范围"
        />
        <ManageSelect
          v-if="!isPic && form.scope === 'slave'"
          v-model="form.sourceCode"
          :options="slaveOptions"
          aria-label="从站来源"
          class="min-w-[180px]"
        />
      </div>
    </ManageFormField>

    <ManageFormField label="旧域名" required hint="只填域名，如 img.lzipic.com">
      <ManageInput v-model="form.fromHost" placeholder="img.lzipic.com" :maxlength="255" />
    </ManageFormField>

    <ManageFormField label="新域名" required hint="只填域名，如 p.lzp261118.com">
      <ManageInput v-model="form.toHost" placeholder="p.lzp261118.com" :maxlength="255" />
    </ManageFormField>

    <ManageFormField label="公告日期" required hint="默认现在；历史规则按公告实际时间填，列表按它倒序">
      <ManageInput v-model="form.at" type="datetime-local" />
    </ManageFormField>

    <ManageFormField label="备注">
      <ManageTextarea v-model="form.note" placeholder="可选：来源、公告链接或人工判断依据" :maxlength="255" />
    </ManageFormField>

    <ManageFormActions
      :busy="busy"
      confirm-text="保存"
      confirm-type="button"
      pt
      @confirm="submit"
      @cancel="cancel"
    />
  </div>
</template>