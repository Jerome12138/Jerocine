/**
 * 域名替换规则的状态词表（列表页与预览弹窗共用一份）。
 * 之前两处各写一份，改文案/配色容易漏 —— 收敛到这里。
 */
export const DOMAIN_RULE_STATE_MAP = {
  pending: { label: '待处理', variant: 'warning' as const },
  applied: { label: '已执行', variant: 'success' as const },
  skipped: { label: '无需替换', variant: 'default' as const },
  merged: { label: '已被取代', variant: 'info' as const },
  rejected: { label: '已拒绝', variant: 'danger' as const }
}