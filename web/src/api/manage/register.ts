import { http } from '../http'
import type { Paginated } from '@/types/api'
import type { RegisterApplyActionResp, RegisterApplyRow } from '@/types/manage'

/**
 * 注册审核（apply 模式）。warning 语义: 审批决策先落库再发信,
 * 邮件失败不阻断 —— 响应 warning 透出 SMTP 原文, 后台可补发。
 */

/** GET /manage/register/applies 申请列表（status 缺省 -1 不限; 0 待审/1 已通过/2 已驳回; 时间倒序分页） */
export const list = (params: {
  status?: number
  page?: number
  size?: number
}): Promise<Paginated<RegisterApplyRow>> =>
  http.get<unknown, Paginated<RegisterApplyRow>>('/manage/register/applies', { params })

/** POST /manage/register/applies/:id/approve 通过（pending→approved + 发激活邮件; 重复/非 pending → 409） */
export const approve = (id: number): Promise<RegisterApplyActionResp> =>
  http.post<unknown, RegisterApplyActionResp>(
    `/manage/register/applies/${id}/approve`,
    undefined,
    { silent: true }
  )

/** POST /manage/register/applies/:id/reject 驳回（理由必填 ≤255，对齐 reject_reason VARCHAR(255); 驳回邮件失败仅 warning） */
export const reject = (id: number, reason: string): Promise<RegisterApplyActionResp> =>
  http.post<unknown, RegisterApplyActionResp>(
    `/manage/register/applies/${id}/reject`,
    { reason },
    { silent: true }
  )

/** POST /manage/register/applies/:id/resend-activation 补发激活邮件（approved 且未激活; 作废旧 token） */
export const resendActivation = (id: number): Promise<RegisterApplyActionResp> =>
  http.post<unknown, RegisterApplyActionResp>(
    `/manage/register/applies/${id}/resend-activation`,
    undefined,
    { silent: true }
  )
