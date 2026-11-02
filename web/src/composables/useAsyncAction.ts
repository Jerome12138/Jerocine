import { ref } from 'vue'
import type { Ref } from 'vue'
import { toast } from '@/api/http'
import { confirm } from '@/composables/useConfirm'
import type { ConfirmOptions } from '@/composables/useConfirm'

/**
 * 写操作统一包装（复核 #1 toast 口径收敛）:
 *   busy 并发拦截 + 可选 confirm + 单点成功/失败 toast + after 回调。
 *
 * 约定（§3.3）: action 内请求一律传 { silent: true } 关拦截器提示,
 * 成功/失败文案由 run 单点负责, 页面删除手写 busy try/finally 与 catch toast。
 * 列表加载请求【不】传 silent（错误仍由拦截器统一提示）。
 *
 * 用法:
 *   const { busy, run } = useAsyncAction()
 *   await run({
 *     confirm: { title: '删除？', okText: '删除', danger: true },
 *     action: () => manageApi.film.softDelete(row.mid, { silent: true }),
 *     successMsg: '已移入回收站',
 *     after: () => paged.load(),
 *   })
 */

export interface RunActionOptions {
  /** 实际请求；内部应传 { silent: true }（约定, TS 无法强制, spec/评审把关） */
  action: () => Promise<unknown>
  /** 确认弹窗选项；用户取消 → run 返回 false 且不执行 action */
  confirm?: ConfirmOptions
  /** 成功文案；省略则不弹成功 toast */
  successMsg?: string
  /** 成功后的回调（通常 reload） */
  after?: () => void | Promise<void>
}

export interface AsyncAction {
  busy: Ref<boolean>
  /** 返回是否实际执行成功（确认取消 / 并发拦截 / 失败均 false） */
  run: (options: RunActionOptions) => Promise<boolean>
}

/** 取消类错误静默（与 usePagedList 口径一致）: axios CanceledError / isCancel 标记 */
function isCanceledError(e: unknown): boolean {
  if (!e || typeof e !== 'object') return false
  const err = e as { name?: string; isCancel?: boolean; code?: string }
  return err.name === 'CanceledError' || err.isCancel === true || err.code === 'ERR_CANCELED'
}

/** 错误文案单点提取: Error.message 优先, 兜底「操作失败」 */
function errorMessage(e: unknown): string {
  const msg = (e as Error | undefined)?.message
  return msg || '操作失败'
}

export function useAsyncAction(): AsyncAction {
  const busy = ref(false)

  async function run(options: RunActionOptions): Promise<boolean> {
    // 并发拦截: 上一次未完成时忽略本次触发（防连点）
    if (busy.value) return false
    busy.value = true
    try {
      if (options.confirm) {
        const ok = await confirm(options.confirm)
        if (!ok) return false
      }
      try {
        await options.action()
      } catch (e: unknown) {
        // 失败不 rethrow（unhandled rejection 兜住）; 取消类错误静默
        if (!isCanceledError(e)) {
          toast('error', errorMessage(e))
        }
        return false
      }
      if (options.successMsg) {
        toast('success', options.successMsg)
      }
      await options.after?.()
      return true
    } finally {
      busy.value = false
    }
  }

  return { busy, run }
}
