/**
 * 友好文本周期（统一时间语义）校验工具。
 *
 * 规则与后端 `server/internal/taskcenter.ValidateSchedule` 完全一致
 * （every:Nm|h / daily:HH:MM / weekly:D HH:MM / monthly:D HH:MM），
 * 前端先校验只为省一次 422 往返 + 给出和列表页一致的中文提示。
 *
 * 空串合法：后端语义是"不定时 / 仅手动"，调用方若要"必填"请自行判空
 * （见 CronListView 的「请填写执行周期」）。
 *
 * ⚠️ 两个坑，改这段代码前务必先读：
 *
 * 1) **不要把这些正则搬回 `.vue` 文件**。uno.config.ts 启用了
 *    transformerVariantGroup，它会扫描 `.vue`/`.jsx`/`.html` 源码里
 *    `标识符:(...)` 形式的"变体组"并就地改写（`hover:(a b)` → `hover:a hover:b`）。
 *    正则字面量 `daily:([01]\d|2[0-3])` 会被它当成变体组改写为
 *    `daily:[01]\d|2[0-3]` —— **括号被吃掉且不报任何错**，于是校验恒失败，
 *    用户只会看到"周期格式不支持"的误报（2026-11-26 实测踩过）。
 *    UnoCSS 的 transformer 不扫 `.ts`，所以校验逻辑放这里最安全。
 *
 * 2) 这里刻意**不写"冒号紧跟左括号"的正则**（如 `/^daily:(\d{2})/`）：
 *    先用 `indexOf(':')` 切出 kind/rest，再对 rest 做无冒号前缀的匹配。
 *    这样即使将来有人把这段复制进 `.vue`，也不会被上一条的改写咬到。
 */

/** 周期写法提示（与后端报错文案同一份取值） */
export const SCHEDULE_HINT = 'every:30m / daily:03:00 / weekly:1 03:00 / monthly:1 04:00'

const RE_EVERY = /^(\d{1,3})(m|h)$/
const RE_DAILY = /^(\d{2}):(\d{2})$/
const RE_WEEKLY = /^([1-7]) (\d{2}):(\d{2})$/
const RE_MONTHLY = /^(\d{1,2}) (\d{2}):(\d{2})$/

const WEEK_LABEL = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']

/** 返回错误原因；`''` 表示通过（含空串 = 不定时） */
export function scheduleError(input: string): string {
  const v = input.trim()
  if (v === '') return ''
  const at = v.indexOf(':')
  if (at <= 0) return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
  const kind = v.slice(0, at)
  const rest = v.slice(at + 1)
  switch (kind) {
    case 'every': {
      const m = RE_EVERY.exec(rest)
      if (!m) return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
      const n = Number(m[1])
      if (n < 1) return '间隔必须 ≥1'
      if (m[2] === 'm' && n > 59) return '分钟间隔最大 59，更长的请用小时（如 every:6h）'
      return ''
    }
    case 'daily': {
      const m = RE_DAILY.exec(rest)
      if (!m) return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
      return hhmmError(m[1] ?? '', m[2] ?? '')
    }
    case 'weekly': {
      const m = RE_WEEKLY.exec(rest)
      if (!m) return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
      return hhmmError(m[2] ?? '', m[3] ?? '')
    }
    case 'monthly': {
      const m = RE_MONTHLY.exec(rest)
      if (!m) return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
      const d = Number(m[1])
      if (d < 1 || d > 28) return 'monthly 的日期只能是 1~28（避开月末不存在的日期）'
      return hhmmError(m[2] ?? '', m[3] ?? '')
    }
    default:
      return `周期格式不支持：${v}（可用 ${SCHEDULE_HINT}）`
  }
}

/** 是否合法（含空串 = 不定时） */
export function scheduleValid(input: string): boolean {
  return scheduleError(input) === ''
}

function hhmmError(hh: string, mm: string): string {
  const h = Number(hh)
  const m = Number(mm)
  if (h > 23 || m > 59) return `时间不合法：${hh}:${mm}（小时 00~23，分钟 00~59）`
  return ''
}

/** 周期 → 中文描述（与后端 DescribeSchedule 同口径；空串 = 仅手动） */
export function describeSchedule(input: string): string {
  const v = input.trim()
  if (v === '') return '仅手动'
  const at = v.indexOf(':')
  if (at <= 0) return v
  const kind = v.slice(0, at)
  const rest = v.slice(at + 1)
  if (kind === 'every') {
    const m = RE_EVERY.exec(rest)
    if (!m) return v
    return m[2] === 'm' ? `每 ${m[1]} 分钟` : `每 ${m[1]} 小时`
  }
  if (kind === 'daily') {
    const m = RE_DAILY.exec(rest)
    return m ? `每天 ${m[1]}:${m[2]}` : v
  }
  if (kind === 'weekly') {
    const m = RE_WEEKLY.exec(rest)
    return m ? `每${WEEK_LABEL[Number(m[1]) - 1]} ${m[2]}:${m[3]}` : v
  }
  if (kind === 'monthly') {
    const m = RE_MONTHLY.exec(rest)
    return m ? `每月 ${m[1]} 日 ${m[2]}:${m[3]}` : v
  }
  return v
}
