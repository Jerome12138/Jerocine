import { reactive, ref, markRaw } from 'vue'
import type { Component, Ref } from 'vue'

/**
 * 函数化管理弹窗（硬性需求 3, 参考 useConfirm 同款单例 + Host 组件模式）。
 *
 * 用法:
 *   const payload = await openManageDialog<UserFormPayload>({
 *     title: '新增用户',
 *     component: UserFormDialog,
 *     props: { initial: editing.value },
 *     mobileMode: 'sheet',
 *   })
 *   if (!payload) return // null = 取消 / 关闭
 *
 * 内容组件契约: emits ('confirm', payload) | ('cancel'), footer 自治
 * （取消按钮 → emit('cancel'); 提交校验通过 → emit('confirm', 表单值)）。
 *
 * Promise 语义:
 *   confirm → resolve(payload)
 *   ESC / 遮罩 / cancel / Host 未挂载 / 重复 open 挤掉 → resolve(null)
 *
 * 形态继承 ManageSheet（modal / sheet / fullsheet 自适应），Host 在 App.vue
 * BaseConfirmDialog 同位挂载一次。同屏单实例（管理端现状无叠层需求）。
 */

export interface ManageDialogOptions {
  title: string
  /** 内容组件（组件对象即可, 内部 markRaw 存放, 页面无需 markRaw） */
  component: Component
  /** 透传给内容组件的 props（不含 open 状态） */
  props?: Record<string, unknown>
  /** 移动端形态（透传 ManageSheet.mobile-mode）, 默认 'sheet' */
  mobileMode?: 'sheet' | 'fullsheet'
  /** modal 宽度（透传 ManageSheet.width）, 省略用 ManageSheet 默认 480px */
  width?: string
}

export interface ManageDialogState {
  visible: boolean
  title: string
  component: Component | null
  props: Record<string, unknown>
  mobileMode: 'sheet' | 'fullsheet'
  width?: string
  resolve: ((v: unknown) => void) | null
}

/** 模块单例（同 confirmState 先例）; component 经 markRaw 存放避免组件被 reactive 代理 */
export const manageDialogState: ManageDialogState = reactive<ManageDialogState>({
  visible: false,
  title: '',
  component: null,
  props: {},
  mobileMode: 'sheet',
  width: undefined,
  resolve: null
})

/** ManageDialogHost onMounted 置 true / onBeforeUnmount 置 false */
export const manageDialogHostMounted: Ref<boolean> = ref(false)

export function openManageDialog<T = unknown>(options: ManageDialogOptions): Promise<T | null> {
  return new Promise<T | null>((resolve) => {
    // Host 未挂载（单测未挂 / 公共区误调用）→ 立即 resolve(null), 不置 visible
    if (!manageDialogHostMounted.value) {
      console.warn('[useManageDialog] Host 未挂载')
      resolve(null)
      return
    }
    // 已有待决弹窗 → 先以 null resolve 旧 Promise（同 useConfirm.ts 先例）
    if (manageDialogState.resolve) {
      const stale = manageDialogState.resolve
      manageDialogState.resolve = null
      stale(null)
    }
    manageDialogState.title = options.title
    manageDialogState.component = markRaw(options.component)
    manageDialogState.props = options.props ?? {}
    manageDialogState.mobileMode = options.mobileMode ?? 'sheet'
    manageDialogState.width = options.width
    manageDialogState.visible = true
    manageDialogState.resolve = resolve as (v: unknown) => void
  })
}

export function answerManageDialog(
  result: { type: 'confirm'; payload: unknown } | { type: 'close' }
): void {
  manageDialogState.visible = false
  const r = manageDialogState.resolve
  manageDialogState.resolve = null
  if (r) r(result.type === 'confirm' ? result.payload : null)
}
