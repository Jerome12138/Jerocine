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

/**
 * 豆瓣榜位展示文案 —— 轮播首屏大图与影片详情页共用同一个函数, 保证两处显示完全一致。
 *
 * 输出形如「豆瓣·热门电影 No.2」: 榜单名一定带分类。榜单名由后端 douban.HotBoardLabel
 * 兜底(缺 hot_board 时按分类热榜补全, 如 pid 4 → 热门动漫), 正常都有值; 万一位次在、
 * 榜名却缺失(数据异常), 这里返回空串(调用方不渲染) —— 宁可不显示, 也不输出一个
 * 没有分类的「热门」。
 */
export function formatHotBadge(hotRank?: number | null, hotBoard?: string | null): string {
  const rank = hotRank ?? 0
  const board = (hotBoard ?? '').trim()
  if (rank <= 0 || !board) {
    return ''
  }
  return `豆瓣·${board} No.${rank}`
}

/**
 * 榜单"短名" —— 影片卡片角标空间有限, 平板档(md~lg)只放 2~3 字短名
 * (如「热门电影」→「电影」、「华语口碑剧集榜」→「华语榜」)。
 * 映射唯一来源在此, 未命中的榜单统一回落「热榜」(不裸露未裁剪的长榜名)。
 */
const HOT_BOARD_SHORT_MAP: Record<string, string> = {
  热门电影: '电影',
  热门剧集: '剧集',
  热门综艺: '综艺',
  热门动漫: '动漫',
  正在上映: '上映',
  一周口碑榜: '口碑',
  华语口碑剧集榜: '华语榜',
  全球口碑剧集榜: '全球榜'
}

export function hotBoardShort(hotBoard?: string | null): string {
  const board = (hotBoard ?? '').trim()
  if (!board) return '热榜'
  return HOT_BOARD_SHORT_MAP[board] ?? '热榜'
}

/**
 * 榜单全称(trim 后) —— 桌面档(≥lg)角标与卡片 title 用完整榜名。
 * 缺失时回落「热榜」(角标此时仍有位次, 需给出可显示的兜底名)。
 */
export function hotBoardFull(hotBoard?: string | null): string {
  const board = (hotBoard ?? '').trim()
  return board || '热榜'
}
