import { reactive } from 'vue'

/**
 * 全局确认对话框（替换原生 confirm() 阻塞弹窗）
 *
 * 使用：
 *   const ok = await confirm({ title: '确认删除？', desc: 'xxx 不可恢复', danger: true })
 *   if (!ok) return
 *
 * 状态由模块顶层单例维护，App.vue 中挂载唯一 ConfirmDialog 监听 state.visible 渲染。
 */

interface ConfirmOptions {
  title: string
  desc?: string
  okText?: string
  cancelText?: string
  /** 是否走危险样式（红色按钮） */
  danger?: boolean
}

interface ConfirmState {
  visible: boolean
  title: string
  desc: string
  okText: string
  cancelText: string
  danger: boolean
  resolve: ((v: boolean) => void) | null
}

export const confirmState = reactive<ConfirmState>({
  visible: false,
  title: '',
  desc: '',
  okText: '确认',
  cancelText: '取消',
  danger: false,
  resolve: null
})

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    if (confirmState.resolve) {
      // 已有待回应的 confirm — 先 reject 它（视为取消）
      confirmState.resolve(false)
    }
    confirmState.title = opts.title
    confirmState.desc = opts.desc ?? ''
    confirmState.okText = opts.okText ?? '确认'
    confirmState.cancelText = opts.cancelText ?? '取消'
    confirmState.danger = !!opts.danger
    confirmState.visible = true
    confirmState.resolve = resolve
  })
}

export function answerConfirm(ok: boolean): void {
  confirmState.visible = false
  const r = confirmState.resolve
  confirmState.resolve = null
  if (r) r(ok)
}
