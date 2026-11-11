import { http } from '../http'

/** 单个来源的行数（self=集名自带期号 / bridge=他源桥接 / anchor=锚点推算 / none=推不出） */
export interface EpidFromSrcStat {
  [fromSrc: string]: number
}

/** 派生任务状态（GET /manage/epid/report 的 status） */
export interface EpidDeriveStatus {
  /** 是否有全量任务在跑 */
  running: boolean
  /** fill | rebuild */
  mode: string
  /** 全量任务起始毫秒（0 = 无） */
  startedAt: number
  /** 当前游标（源行 id；0 = 刚到表尾、下轮从头复核） */
  cursor: number
  /** true = 正在跑"热门趟"（只扫 mid 非空的源行） */
  hotPhase: boolean
  /** 本轮累计扫过的源行数（内存计数，0 成本） */
  scanned: number
  /** 本轮累计重建的源行数 */
  rebuilt: number
  /** 本轮累计批数 */
  batches: number
  /** 本轮已跑秒数（未在跑时为 0） */
  elapsedSec: number
  /** 本轮扫过源行/秒 */
  scanRate: number
  /** 本轮重建源行/秒 */
  rebuildRate: number
}

/**
 * 进度集（任务**进行中也给**）。
 *
 * 口径说明：这几项全部来自「写侧计数器（O(1)）+ 后台缓存快照 + 内存计数」，与表规模无关。
 * 旧实现用精确 `COUNT(*)`，派生表到 590 万行后要 19.2s / 8.6s ⇒ 撞超时被降级成 0
 * ⇒ 卡片显示 `0 / 0` 让人以为数据丢了 ✗（2026-11-11 线上事故）。
 */
export interface EpidProgress {
  /** false ⇒ 计数器未初始化且兜底计数超时：**别把这些 0 当真值** */
  countingAvailable: boolean
  /** 派生表行数 */
  derivedRows: number
  /** 已派生过的源行数 */
  derivedSrcRows: number
  /** 真相源源行总数 */
  srcRows: number
  /** 还缺多少源行（= srcRows − derivedSrcRows） */
  remainSrcRows: number
  scanned: number
  rebuilt: number
  batches: number
  elapsedSec: number
  scanRate: number
  rebuildRate: number
  /** 按重建速率粗估的剩余秒数（0 = 估不出；口径为**下界**） */
  etaSec: number
  /** 影片完整度：全部源行都已派生的影片数 */
  movieComplete: number
  /** 影片总数 */
  movieTotal: number
  /** 不完整影片里一共还缺多少个源行 */
  missingSrcRows: number
  /** 快照采样时间（毫秒；0 = 还没算出来） */
  movieStatAt: number
  /** true = 后台正在刷新，展示的是上一份快照 */
  movieStatStale: boolean
}

/** 派生验收指标（GET /manage/epid/report） */
export interface EpidReport {
  status: EpidDeriveStatus
  /** 派生表行数 */
  derivedRows: number
  /** 其中 epid 为空串的行数（裸「第N集」/推不出期号 —— 按 §4.7 L6 属正常；仅稳态口径下有意义） */
  emptyEpidRows: number
  /** 同 (match_key, epid) 出现在 ≥2 个 site 的组数（**全库口径**，仅作对照） */
  alignedGroups: number
  /** 参与统计的非空 epid 组数（全库口径） */
  totalGroups: number
  /** 对齐率（%，**全库口径**，不作门槛判定） */
  alignmentPct: number
  /** 期号来源分布 */
  byFromSrc: EpidFromSrcStat
  /** 完整覆盖率（§6.1 口径：派生行数 / Σ JSON_LENGTH(episodes_json)） */
  coverage?: { derivedRows: number; sourceEpisodes: number; pct: number }
  /** 覆盖率算不出来时的原因（表为空/查询失败等） */
  coverageError?: string

  // ---- 口径层（2026-11-11）：§4.8 门槛以**可比口径**为准 ----
  /** 可比口径：分母只含"真相源里 ≥2 site 且已成熟"的片 */
  comparableAligned: number
  comparableTotal: number
  comparablePct: number
  /** 进度集（任务进行中也给） */
  progress?: EpidProgress
  /**
   * true ⇒ 本轮**只算了进度集**（任务进行中）。
   * ⚠️ 此时下面对齐率/分层/成熟度/形态/覆盖率都是 0，**只是"没算"，不是"真的是 0"** ⇒ 不要当真值渲染。
   */
  onlyProgress: boolean
  /** 重型指标快照的采样时间（毫秒；0 = 后台还没算出来） */
  metricsAt: number
  /** true = 后台正在刷新，展示的是上一份快照 */
  metricsStale: boolean
  /** 源行口径成熟度（按分类）：分母=真相源全部源行，分子=其中已有派生行的源行数 */
  maturity: { pid: number; category: string; srcRows: number; derivedSrcRows: number }[]
  /** 分层"有身份率"（按分类，用 category 名称） */
  byCategory: { pid: number; category: string; rowCnt: number; nonEmpty: number; nonEmptyPct: number }[]
  /** 空 epid 的集名形态分档（回答"为什么空"） */
  emptyByShape: { shape: string; rowCnt: number }[]
  metricsError?: string
}

/** 一键全量派生的返回（POST /manage/epid/derive-all） */
export interface EpidDeriveAllResp {
  started: boolean
  mode: string
  batch: number
  maxBatches: number
  timeoutMin: number
}

/** GET /manage/epid/report —— 只读验收指标（对齐率/覆盖率/来源分布/进度） */
export const report = (): Promise<EpidReport> => http.get<unknown, EpidReport>('/manage/epid/report')

/**
 * POST /manage/epid/derive-all —— 一键全量派生（异步）。
 *
 * mode='fill'    补缺/更新：从表头起把"集内容变过"的源行补齐/更新，**不动已有**派生（默认）
 * mode='rebuild' 清空重建：**先 TRUNCATE 派生表**再全量重算（规则升级后用；期间出参无 epid，端侧自动回退）
 *
 * 注意：与采集收尾的自动触发互斥；已有任务在跑时返回 409。
 */
export const deriveAll = (
  mode: 'fill' | 'rebuild' = 'fill',
  opts: { batch?: number; maxBatches?: number; timeoutMin?: number } = {}
): Promise<EpidDeriveAllResp> => {
  const qs = new URLSearchParams({ mode })
  if (opts.batch) qs.set('batch', String(opts.batch))
  if (opts.maxBatches) qs.set('maxBatches', String(opts.maxBatches))
  if (opts.timeoutMin) qs.set('timeoutMin', String(opts.timeoutMin))
  return http.post<unknown, EpidDeriveAllResp>(`/manage/epid/derive-all?${qs.toString()}`, undefined, {
    timeout: 30_000
  })
}
