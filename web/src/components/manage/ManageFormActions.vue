<script setup lang="ts">
import ManageButton from '@/components/manage/ManageButton.vue'

/**
 * ManageFormActions — 弹窗/卡片表单 footer 原子（§C8#2）。
 * 收口 11 处手写「ghost 左钮 + 渐变提交钮」footer（UserResetPassword/FilmClassForm/
 * BannerForm/LiveSourceForm/UserForm/RegisterReject/TelemetryResolve/MailConfig/
 * DanmuConfig/SiteConfig×2），各类视觉漂移（间距档/上内距/吸底/primary 变体/点击驱动）
 * 以 props 保真保留，不做归一。
 *
 * - 左钮（cancelText）：语义上是「取消/重置/清除」动作位，动作由宿主绑 @cancel 接走；
 *   传空串 = 不渲染左钮（SiteConfig TMDB 未存 Key 时原款只留保存钮）。
 * - 提交钮：默认 variant=gradient + 原生 type=submit（表单 @submit 驱动，与原款一致）；
 *   TelemetryResolve 原款为实色 primary（confirmVariant）；LiveSource/Danmu/TMDB 保存 Key
 *   三处为点击驱动（confirmType="button" + 宿主 @confirm）。
 * - busy 只作用于提交钮 loading（ManageButton loading 期吞点击，防重复提交）。
 */
interface Props {
  /** 提交钮 loading（提交中禁点） */
  busy?: boolean
  /** 左钮文案；传空串不渲染左钮（默认「取消」） */
  cancelText?: string
  /** 提交钮文案 */
  confirmText: string
  /** 卡片内长表单吸底：mt-auto（DanmuConfig/SiteConfig 卡三处原款） */
  sticky?: boolean
  /** 双钮间距档：2=8px（短弹窗三处原款）/ 3=12px（默认，其余原款） */
  gap?: 2 | 3
  /** footer 上内距 pt-2（弹窗表单四款原款；卡片类不带） */
  pt?: boolean
  /** 左钮禁用（SiteConfig TMDB 清除钮 keyBusy/loadError 原款） */
  cancelDisabled?: boolean
  /** 提交钮禁用（SiteConfig 读档失败原款） */
  confirmDisabled?: boolean
  /** 提交钮 variant：gradient（默认）/ primary（TelemetryResolve 原款实色） */
  confirmVariant?: 'gradient' | 'primary'
  /** 提交钮原生 type：submit（默认，表单 @submit 驱动）/ button（点击驱动） */
  confirmType?: 'submit' | 'button'
}

const props = withDefaults(defineProps<Props>(), {
  busy: false,
  cancelText: '取消',
  sticky: false,
  gap: 3,
  pt: false,
  cancelDisabled: false,
  confirmDisabled: false,
  confirmVariant: 'gradient',
  confirmType: 'submit'
})

const emit = defineEmits<{
  /** 左钮点击（取消/重置/清除动作由宿主接） */
  (e: 'cancel'): void
  /** 提交钮点击（confirmType=button 时宿主必绑；type=submit 时由表单 @submit 接管，此事件无人监听亦无害） */
  (e: 'confirm'): void
}>()
</script>

<template>
  <div
    class="flex justify-end"
    :class="[
      props.gap === 2 ? 'gap-[var(--jc-space-2)]' : 'gap-[var(--jc-space-3)]',
      props.pt ? 'pt-[var(--jc-space-2)]' : '',
      props.sticky ? 'mt-auto' : ''
    ]"
  >
    <ManageButton
      v-if="props.cancelText !== ''"
      variant="ghost"
      type="button"
      :disabled="props.cancelDisabled"
      @click="emit('cancel')"
    >
      {{ props.cancelText }}
    </ManageButton>
    <ManageButton
      :variant="props.confirmVariant"
      :type="props.confirmType"
      :loading="props.busy"
      :disabled="props.confirmDisabled"
      @click="emit('confirm')"
    >
      {{ props.confirmText }}
    </ManageButton>
  </div>
</template>
