/** 文本 / 时间 格式化工具 */

/** 数字补零 */
function pad(n: number, w = 2): string {
  return n.toString().padStart(w, '0')
}

/** 时长（秒）→ HH:MM:SS / MM:SS */
export function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = s % 60
  if (h > 0) {
    return `${pad(h)}:${pad(m)}:${pad(ss)}`
  }
  return `${pad(m)}:${pad(ss)}`
}

/** 字节数 → 人类可读 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return '-'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 2 : 1)} ${units[i]}`
}

/** 时间戳（ms） → YYYY-MM-DD HH:mm */
export function formatDateTime(ts: number | string | Date): string {
  const d = ts instanceof Date ? ts : new Date(ts)
  if (Number.isNaN(d.getTime())) {
    return '-'
  }
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 截断 + ellipsis */
export function truncate(str: string, max: number): string {
  if (!str) {
    return ''
  }
  return str.length > max ? `${str.slice(0, max)}…` : str
}
