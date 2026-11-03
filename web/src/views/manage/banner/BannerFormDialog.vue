<script setup lang="ts">
import { reactive } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageImageUpload from '@/components/manage/ManageImageUpload.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 轮播新增/编辑/采纳表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', true) | ('cancel'), footer 自带。
 *
 * 含独立校验与提交(三条校验分支 toast 拦截、提交失败弹窗停留输入不丢),
 * 提交成功后 emit('confirm'), 由发起页负责重拉 Board。
 * adoptSlot: 采纳自动位时的钉入位置(生效列表下标), 原样透传 save 第二参;
 * undefined = 新增(服务端自行 maxSort+1)/编辑(不钉位)。
 */

/** 表单输入态: mid 为输入字符串(ManageInput number 清空为 undefined, 归一 ''),
 *  起止为 datetime-local 字符串, 提交时 Number()/localToMs 收口 */
export interface BannerFormInitial {
  id: number
  title: string
  subtitle: string
  image: string
  poster: string
  mid: string
  link: string
  sort: number
  state: number
  startStr: string
  endStr: string
}

const props = withDefaults(
  defineProps<{ initial: BannerFormInitial; adoptSlot?: number }>(),
  { adoptSlot: undefined }
)
const emit = defineEmits<{
  (e: 'confirm', payload: boolean): void
  (e: 'cancel'): void
}>()

const form = reactive<BannerFormInitial>({ ...props.initial })

/** datetime-local 字符串 → ms; 空/非法 → 0 */
function localToMs(s: string): number {
  if (!s) return 0
  const t = new Date(s).getTime()
  return Number.isNaN(t) ? 0 : t
}

const { busy, run } = useAsyncAction()

function submit(): void {
  if (!form.image && !form.poster) {
    toast('error', '至少需要上传横图或竖图')
    return
  }
  if (!form.link.trim() && !(Number(form.mid) > 0)) {
    toast('error', '需要配置跳转：关联影片 ID 或自定义链接')
    return
  }
  const startAt = localToMs(form.startStr)
  const endAt = localToMs(form.endStr)
  if (startAt > 0 && endAt > 0 && endAt < startAt) {
    toast('error', '结束时间不能早于开始时间')
    return
  }
  void run({
    action: () =>
      manageApi.banner.save(
        {
          id: form.id,
          title: form.title,
          subtitle: form.subtitle,
          image: form.image,
          poster: form.poster,
          mid: Number(form.mid) > 0 ? Number(form.mid) : 0,
          link: form.link.trim(),
          sort: form.sort,
          state: form.state,
          startAt,
          endAt
        },
        props.adoptSlot,
        { silent: true }
      ),
    successMsg: '已保存',
    after: () => emit('confirm', true)
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submit">
    <ManageFormField label="标题" hint="留空则前台显示「为你推荐」">
      <ManageInput
        :model-value="form.title"
        placeholder="如：本周强推"
        @update:model-value="(v) => (form.title = String(v))"
      />
    </ManageFormField>

    <ManageFormField label="副标题" hint="展示在大标题下方的一行简介">
      <ManageInput
        :model-value="form.subtitle"
        placeholder="可选"
        @update:model-value="(v) => (form.subtitle = String(v))"
      />
    </ManageFormField>

    <ManageFormField label="横图（宽幅主视觉）" required hint="建议 21:9 或 16:9，桌面/大屏首屏主图">
      <ManageImageUpload v-model="form.image" ratio="21/9" />
    </ManageFormField>

    <ManageFormField label="竖图（窄屏兜底）" hint="建议 2:3；只给竖图时会用模糊铺底 + 侧栏竖海报">
      <ManageImageUpload v-model="form.poster" ratio="2/3" />
    </ManageFormField>

    <ManageFormField label="关联影片 ID" hint="填 mid 则点击进入该片详情；与自定义链接二选一">
      <ManageInput
        :model-value="form.mid"
        type="number"
        placeholder="如 12345"
        @update:model-value="(v) => (form.mid = v === undefined ? '' : String(v))"
      />
    </ManageFormField>

    <ManageFormField label="自定义链接" hint="站内路径（/filmClassify?Pid=1）或外链（https://…）；优先于关联影片">
      <ManageInput
        :model-value="form.link"
        placeholder="/filmClassify?Pid=1 或 https://…"
        @update:model-value="(v) => (form.link = String(v))"
      />
    </ManageFormField>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-[var(--jc-space-4)]">
      <ManageFormField label="生效开始" hint="留空 = 立即">
        <ManageInput
          :model-value="form.startStr"
          type="datetime-local"
          @update:model-value="(v) => (form.startStr = String(v))"
        />
      </ManageFormField>
      <ManageFormField label="生效结束" hint="留空 = 长期">
        <ManageInput
          :model-value="form.endStr"
          type="datetime-local"
          @update:model-value="(v) => (form.endStr = String(v))"
        />
      </ManageFormField>
    </div>

    <ManageFormField label="启用">
      <ManageSwitch
        :model-value="form.state === 0"
        @update:model-value="(v) => (form.state = v ? 0 : 1)"
      />
    </ManageFormField>

    <div class="flex justify-end gap-[var(--jc-space-3)] pt-[var(--jc-space-2)]">
      <ManageButton variant="ghost" type="button" @click="emit('cancel')">取消</ManageButton>
      <ManageButton variant="gradient" type="submit" :loading="busy">保存</ManageButton>
    </div>
  </form>
</template>
