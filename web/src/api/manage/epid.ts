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
  /** 对齐率（%，**mk 级主口径**：单源默认按对齐算；未对齐 = 多源分歧，见 classAlignment） */
  alignmentPct: number
  /** 期号来源分布 */
  byFromSrc: EpidFromSrcStat
  /** 完整覆盖率（§6.1 口径：派生行数 / Σ JSON_LENGTH(episodes_json)） */
  coverage?: { derivedRows: number; sourceEpisodes: number; pct: number }
  /** 覆盖率算不出来时的原因（表为空/查询失败等） */
  coverageError?: string

  // ---- 口径层（2026-11-11 / 2026-11-15 修订）----
  // 2026-11-17：**可比口径字段已移除**（后端 `AlignmentRateComparable` 已删除：无消费方 + 每 120s 全表聚合
  // 纯浪费 + 易被误读成"对齐率"）。历史组级字段 alignedGroups/totalGroups/alignmentPct 仍返回，仅作对照。
  /** true = 派生任务进行中（进度类指标只在此状态有意义） */
  deriving: boolean
  /** 进度集（任务进行中也给） */
  progress?: EpidProgress
  /**
   * true ⇒ 本轮**只算了进度集**（任务进行中）。
   * ⚠️ 此时对齐率/分类对齐/形态/覆盖率都是 0，**只是"没算"，不是"真的是 0"** ⇒ 不要当真值渲染。
   */
  onlyProgress: boolean
  /** 重型指标快照的采样时间（毫秒；0 = 后台还没算出来） */
  metricsAt: number
  /** true = 后台正在刷新，展示的是上一份快照 */
  metricsStale: boolean
  /**
   * 按分类跨源对齐（**槽位口径**，双粒度）：
   * 槽位 = (match_key, ep_index)。槽对齐 = 槽内非空 epid 唯一（单源槽默认对齐；多源槽同 epid）；
   * 未对齐槽 = 槽内 epid 多值（不同源对"第 N 集"给出不同身份——分歧）。
   * 集级 = 对齐槽位占比；片级 = 集级 100% 的片占比。
   */
  classAlignment: {
    pid: number
    category: string
    totalSlots: number
    alignedSlots: number
    slotPct: number
    totalMks: number
    alignedMks: number
    mkPct: number
  }[]
  /** 全库汇总（category="全部"） */
  classAlignmentAll: {
    pid: number
    category: string
    totalSlots: number
    alignedSlots: number
    slotPct: number
    totalMks: number
    alignedMks: number
    mkPct: number
  }
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

// ===== epid 源头校正层 · T4 管理端入口（2026-11-12）=====
//
// 裁定台账 movie_epid_override 的管理面（对齐 T4 后端五条路由）：
// import / GET 不受 D3 生效开关影响（先录入核对）；apply / recompute 关着时 403。

/** 台账行（GET /manage/epid/overrides 的 list 项 —— 后端白名单出参，红线③：evidence/审计列不下发） */
export interface EpidOverrideRow {
  id: number
  srcRowId: number
  epIndex: number
  matchKey: string
  mid: number
  keyRawName: string
  keyEpid: string
  verdict: 'corrected' | 'kept' | string
  newEpid: string
  provenance: string
  confidence: string
  reason: string
  status: string
}

/** 单分组计数行（verdict/provenance/status 三维） */
export interface EpidOverrideStat {
  name: string
  n: number
}

/** GET /manage/epid/overrides 响应：开关状态 + 台账列表 + 统计 */
export interface EpidOverrideListResp {
  /** D3 生效开关（内存态，重启回默认关） */
  enabled: boolean
  list: EpidOverrideRow[]
  stats: { byVerdict: EpidOverrideStat[]; byProvenance: EpidOverrideStat[]; byStatus: EpidOverrideStat[] }
}

/** POST /manage/epid/overrides/apply 响应（纯计数） */
export interface EpidOverrideApplyResp {
  considered: number
  applied: number
  inEffect: number
  kept: number
  stale: number
  missing: number
}

/**
 * import 单条候选（POST /manage/epid/overrides/import 的请求项）。
 *
 * 请求体 = 本结构的 JSON 数组；status/id/matchKey/mid/createdAt 由后端强制/回填（传了无效）。
 */
export interface EpidOverrideImportItem {
  srcRowId: number
  epIndex: number
  keyRawName: string
  keyEpid: string
  verdict: 'corrected' | 'kept'
  /** verdict=corrected 必填（目标 epid，5 段） */
  newEpid?: string
  provenance: 'rule' | 'external' | 'agent' | 'human'
  confidence: 'inferred' | 'verified'
  reason?: string
  evidence?: Record<string, unknown>
  ruleVersionAt?: string
}

/** POST /manage/epid/overrides/import 响应（纯计数 + 拒收明细） */
export interface EpidOverrideImportResp {
  inserted: number
  deduped: number
  rejected: { srcRowId: number; epIndex: number; reason: string }[]
}

/** GET /manage/epid/overrides 台账列表 + 统计 + 开关状态（?status= 可选过滤） */
export const overridesList = (status = ''): Promise<EpidOverrideListResp> =>
  http.get<unknown, EpidOverrideListResp>('/manage/epid/overrides', {
    params: status ? { status } : undefined,
    timeout: 30_000
  })

/**
 * POST /manage/epid/overrides/import —— 导入候选行 → pending。
 * 逐行校验/批内去重/键冲突/key_epid 对账在后端；部分拒收也是 200（看 rejected）。
 */
export const overridesImport = (
  items: EpidOverrideImportItem[],
  config?: { silent?: boolean }
): Promise<EpidOverrideImportResp> =>
  http.post<unknown, EpidOverrideImportResp>('/manage/epid/overrides/import', items, {
    timeout: 60_000,
    ...config
  })

/**
 * POST /manage/epid/overrides/apply —— B 快速应用（只点改命中行，幂等）。
 * targets 省略/空 = 应用全部 active；开关关着 ⇒ 403。
 */
export const overridesApply = (
  targets?: { srcRowId: number; epIndex: number }[],
  config?: { silent?: boolean }
): Promise<EpidOverrideApplyResp> =>
  http.post<unknown, EpidOverrideApplyResp>(
    '/manage/epid/overrides/apply',
    targets && targets.length ? { targets } : undefined,
    { timeout: 60_000, ...config }
  )

/**
 * POST /manage/epid/overrides/recompute —— C 定向重算（整源行重算，force=true 绕过抖动守卫）。
 * 响应为 DeriveStats 展平（scanned/rebuilt/…/overrideApplied 等）。
 */
export const overridesRecompute = (
  targets: { srcRowId: number; epIndex: number }[],
  config?: { silent?: boolean }
): Promise<Record<string, number>> =>
  http.post<unknown, Record<string, number>>('/manage/epid/overrides/recompute', { targets }, {
    timeout: 120_000,
    ...config
  })

/** PUT /manage/epid/overrides/enabled —— D3 生效开关（内存态；重启回默认关 = fail-safe） */
export const overridesSetEnabled = (
  enabled: boolean,
  config?: { silent?: boolean }
): Promise<{ enabled: boolean }> =>
  http.put<unknown, { enabled: boolean }>('/manage/epid/overrides/enabled', { enabled }, {
    timeout: 15_000,
    ...config
  })

// ===== 对齐率分层漏斗（L0~L6，2026-11-18）=====
//
// 契约与 `server/openapi/openapi.yaml` 的 `GET /manage/epid/alignment/funnel` 一致。
// 口径：分母恒为全部非空槽位 / 全部有身份片；分子含 L0~L6 的救回
//       （L0 单源 / L1 槽位全等 / L2 值序列容忍 / L3 日期锚 / L4 期号锚 / L5 正片子集 / L6 纯 n·d 容忍），
//       只有 `mixedDiag`（多身份槽，不判定）与 `leftover`（真未对齐）算未对齐。
//
// P11（2026-11-18 b91d9a0）已收口：GET 只**读快照**（秒回；无快照时 `pending=true`），
// 重算走 `alignmentFunnelRefresh` 后台单飞（全量 ~9 分钟），完成后 GET 轮询 `computedAt` 变化。
export interface EpidFunnelStat {
  pid: number
  category: string
  /** 分母：全部非空 epid 槽位 */
  slotsTotal: number
  l0Single: number
  l1Slot: number
  l2Tolerant: number
  l3Date: number
  l4Num: number
  l5Subset: number
  /** L6 纯 n/纯 d 容忍层救回 */
  l6Tol: number
  /** L6.1 纯 n 整片 */
  l6Num: number
  /** L6.2 纯 d 整片 */
  l6Date: number
  /** L6.3 纯 d 按槽（同日唯一守卫） */
  l6Slot: number
  /** 诊断档：含 s/v 的多身份槽（不判定，计入未对齐） */
  mixedDiag: number
  leftover: number
  mksTotal: number
  mksAligned: number
  /** = slotsTotal − mixedDiag − leftover */
  alignedSlots: number
  /** 集级对齐率（分层口径，%） */
  slotPct: number
  /** 片级对齐率（分层口径，%） */
  mkPct: number
}

export interface EpidFunnelReport {
  byCategory: EpidFunnelStat[]
  all: EpidFunnelStat
  /** 快照生成时间（尚无快照时缺省/零值） */
  computedAt?: string
  /** 口径版本（L6 语义变更时递增，如 `L0-L6.3@2026-11-18`） */
  policyVersion?: string
  /** true=本次计算被分页上限截断（结果不完整） */
  truncated?: boolean
  /** true=还没有快照，需先 POST refresh */
  pending?: boolean
}

/**
 * GET /manage/epid/alignment/funnel —— 分层漏斗读数（**读快照**，P11 收口后秒回）。
 * 无快照时返回 `pending=true`（空集、不现算）；先用 alignmentFunnelRefresh 触发后台刷新。
 */
export const alignmentFunnel = (config?: { silent?: boolean }): Promise<EpidFunnelReport> =>
  http.get<unknown, EpidFunnelReport>('/manage/epid/alignment/funnel', {
    timeout: 30_000,
    ...config
  })

/**
 * POST /manage/epid/alignment/funnel/refresh —— 异步触发一次漏斗快照刷新（后台单飞）。
 * 已有任务在跑时 HTTP 409（problem+json）；全量 ~9 分钟，用 GET 轮询 `computedAt` 判断完成。
 * ⚠️ 全量 rebuild 派生期间勿触发（快照会固化中间态）。
 */
export const alignmentFunnelRefresh = (
  config?: { silent?: boolean }
): Promise<{ started: boolean; running: boolean }> =>
  http.post<unknown, { started: boolean; running: boolean }>('/manage/epid/alignment/funnel/refresh', undefined, {
    timeout: 15_000,
    ...config
  })
