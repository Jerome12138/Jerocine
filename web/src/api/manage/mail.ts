import { http } from '../http'
import type { MailConfigResp, MailConfigUpdate, MailTestReq } from '@/types/manage'

/** GET /manage/mail/config 读邮件与注册配置（凭据掩码 + 预设表） */
export const getMailConfig = (): Promise<MailConfigResp> =>
  http.get<unknown, MailConfigResp>('/manage/mail/config')

/** PUT /manage/mail/config 保存（smtpPass 空串=不修改；成功后回最新掩码配置） */
export const saveMailConfig = (data: MailConfigUpdate): Promise<MailConfigResp> =>
  http.put<unknown, MailConfigResp>('/manage/mail/config', data)

/** POST /manage/mail/test 用未保存的表单值直发测试邮件；失败 message 为 SMTP 原文错误 */
export const testMail = (data: MailTestReq): Promise<{ sent: boolean }> =>
  http.post<unknown, { sent: boolean }>('/manage/mail/test', data)
