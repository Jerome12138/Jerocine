import { http } from '../http'
import type {
  DailySessionsResp,
  DashboardStat,
  LoginSessionResp,
  OnlineOverview,
  RuntimeSettings,
  SiteBasic
} from '@/types/manage'

// ---- 后端契约 DTO (entity.SiteConfig 子集) ----
interface SiteConfigDTO {
  id: number
  siteName: string
  domain: string
  logo: string
  keyword: string
  description: string
  state: number // 0 开启 / 1 关闭
  hint: string
  /** TMDB 凭据掩码(如 0c06…676b), 未配置为空串; 明文永不回传 */
  tmdbKeyMasked?: string
  tmdbKeySet?: boolean
}

/** GET /manage/dashboard 仪表盘统计 */
export const dashboard = (): Promise<DashboardStat> =>
  http.get<unknown, DashboardStat>('/manage/dashboard')

/** GET /manage/online/overview 当前在线/观看人数(内存实时) */
export const onlineOverview = (): Promise<OnlineOverview> =>
  http.get<unknown, OnlineOverview>('/manage/online/overview')

/** GET /manage/online/daily-sessions 今日/近7天访问用户明细(?days=1|7) */
export const onlineDailySessions = (days: number): Promise<DailySessionsResp> =>
  http.get<unknown, DailySessionsResp>('/manage/online/daily-sessions', { params: { days } })

/** GET /manage/site-config 站点基础配置（响应同端点还带 TMDB 掩码状态，一并返回 ——
 *  页面初次加载一次取全，避免 getBasic/getTMDBKey 对同一接口连打两枪） */
export const getBasic = async (): Promise<SiteBasic & TMDBKeyState> => {
  const c = await http.get<unknown, SiteConfigDTO>('/manage/site-config')
  return {
    siteName: c.siteName,
    logo: c.logo,
    keyword: c.keyword,
    describe: c.description,
    domain: c.domain,
    state: c.state === 0,
    hint: c.hint,
    masked: c.tmdbKeyMasked ?? '',
    set: c.tmdbKeySet ?? false
  }
}

/** POST /manage/site-config 更新站点基础配置 */
export const updateBasic = (data: SiteBasic, config?: { silent?: boolean }): Promise<void> =>
  http.post<unknown, void>('/manage/site-config', {
    id: 1,
    siteName: data.siteName,
    domain: data.domain ?? '',
    logo: data.logo,
    keyword: data.keyword,
    description: data.describe,
    state: data.state ? 0 : 1,
    hint: data.hint ?? ''
  }, config)

// ---- TMDB 凭据(独立端点, 明文永不回传) ----

export interface TMDBKeyState {
  /** 已配置时的掩码(如 0c06…676b), 未配置为空串 */
  masked: string
  set: boolean
}

/** GET /manage/site-config 读取 TMDB key 状态(掩码) */
export const getTMDBKey = async (): Promise<TMDBKeyState> => {
  const c = await http.get<unknown, SiteConfigDTO>('/manage/site-config')
  return { masked: c.tmdbKeyMasked ?? '', set: c.tmdbKeySet ?? false }
}

/** POST /manage/tmdb-key 保存凭据(v3 key / v4 token), 服务端验真后热生效 */
export const setTMDBKey = (key: string, config?: { silent?: boolean }): Promise<void> =>
  http.post<unknown, void>('/manage/tmdb-key', { key }, config)

/** DELETE /manage/tmdb-key 清除凭据(横图回填停摆, 已落地图不受影响) */
export const clearTMDBKey = (config?: { silent?: boolean }): Promise<void> =>
  http.delete<unknown, void>('/manage/tmdb-key', config)

// ---- 运行参数(账号与安全 / 采集性能) ----

/** GET /manage/settings 读运行参数 */
export const getRuntimeSettings = (): Promise<RuntimeSettings> =>
  http.get<unknown, RuntimeSettings>('/manage/settings')

/** PUT /manage/settings 写运行参数（登录策略下次登录生效，采集并发下一轮采集生效） */
export const updateRuntimeSettings = (
  data: RuntimeSettings,
  config?: { silent?: boolean }
): Promise<RuntimeSettings> => http.put<unknown, RuntimeSettings>('/manage/settings', data, config)

// ---- 在线会话(当前生效的登录凭据) ----

/** GET /manage/sessions 最近活跃会话（keyword 过滤用户名/ID/IP） */
export const listSessions = (params: {
  keyword?: string
  limit?: number
}): Promise<LoginSessionResp> =>
  http.get<unknown, LoginSessionResp>('/manage/sessions', { params })

/** DELETE /manage/sessions/:id 踢掉单个会话（该 token 立即失效） */
export const revokeSession = (id: string): Promise<{ id: string; revoked: boolean }> =>
  http.delete<unknown, { id: string; revoked: boolean }>(
    `/manage/sessions/${encodeURIComponent(id)}`,
    { silent: true }
  )

// ---- 域名替换工具（macrep，2026-11-25）----
export interface DomainRuleView {
  id: number
  field: string
  fieldLabel: string
  ruleType: string
  /** master 主站 / slave 从站 */
  scope: string
  /** scope=slave 时的采集源 id，空=全部从站 */
  sourceCode: string
  /** 来源站标识：lz / manual … */
  sourceKey: string
  /** 来源页 URL（可点开原文） */
  sourceUrl: string
  fromHost: string
  toHost: string
  /** 我们库里的落点（如 movie.cover），空=本库不存储 */
  targetColumn: string
  /** 落点说明 */
  targetNote: string
  announcedAt: number
  state: string
  /** 本工具的操作备注（如"已替换 1234 行"） */
  note: string
  /** 源站备注 */
  sourceNote: string
  /** 本工具能否自动执行（当前仅 vod_pic） */
  supported: boolean
}

/** 手工录入规则载荷 */
export interface DomainRuleManualPayload {
  field: string
  scope: string
  sourceCode: string
  fromHost: string
  toHost: string
  announcedAt: number
  sourceNote: string
}

export interface DomainRuleListResp {
  list: DomainRuleView[]
  total: number
  page: number
  size: number
  /** 各状态全量计数（pending/applied/skipped/merged/rejected） */
  counts: Record<string, number>
  /** 公告超过 7 天仍未处理的 pending 数 */
  stale: number
}

/** 一个可拉取的公告来源（后端注册表下发，前端只负责渲染） */
export interface DomainSource {
  key: string
  name: string
  type: string
  url: string
  fieldHint: string
  /** 这些字段落到我们库的哪里（后端口径） */
  targetHint: string
  note: string
}

export interface DomainFetchResult {
  source: string
  new: number
  /** 目标变更、旧行被标 merged 的条数 */
  merged: number
  /** 与库内完全一致、跳过的条数 */
  skipped: number
  rejected: number
  fetchedAt: number
}

export interface DomainPreviewResp {
  rule: DomainRuleView
  hits: number
  sample: { mid: number; before: string; after: string }[]
}

/** POST /manage/domain-replace/fetch 拉取公告（**只写规则表**，不执行替换） */
export const fetchDomainRules = (source = ''): Promise<DomainFetchResult> =>
  http.post<unknown, DomainFetchResult>('/manage/domain-replace/fetch', { source }, { silent: true })

/** POST /manage/domain-replace/rules 手工录入规则（只写规则表） */
export const createDomainRule = (payload: DomainRuleManualPayload): Promise<{ ok: boolean }> =>
  http.post<unknown, { ok: boolean }>('/manage/domain-replace/rules', payload, { silent: true })

/** PUT /manage/domain-replace/rules/:id 手工编辑单条规则（原地改） */
export const updateDomainRule = (
  id: number,
  payload: DomainRuleManualPayload
): Promise<{ id: number; ok: boolean }> =>
  http.put<unknown, { id: number; ok: boolean }>(
    `/manage/domain-replace/rules/${id}`,
    payload,
    { silent: true }
  )

/** GET /manage/domain-replace/sources 当前支持的公告来源 */
export const listDomainSources = (): Promise<{ list: DomainSource[] }> =>
  http.get<unknown, { list: DomainSource[] }>('/manage/domain-replace/sources')

/** GET /manage/domain-replace/rules 规则列表（分页）+ 全量告警计数 */
export const listDomainRules = (opts: {
  state?: string
  keyword?: string
  page?: number
  size?: number
} = {}): Promise<DomainRuleListResp> =>
  http.get<unknown, DomainRuleListResp>('/manage/domain-replace/rules', {
    params: {
      state: opts.state || undefined,
      keyword: opts.keyword?.trim() || undefined,
      page: opts.page ?? 1,
      size: opts.size ?? 24
    }
  })

/** GET /manage/domain-replace/rules/:id/preview 影响面预览（只读） */
export const previewDomainRule = (id: number): Promise<DomainPreviewResp> =>
  http.get<unknown, DomainPreviewResp>(`/manage/domain-replace/rules/${id}/preview`)

/** POST /manage/domain-replace/rules/:id/state 人工标记规则状态 */
export const setDomainRuleState = (
  id: number,
  state: string,
  note = ''
): Promise<{ id: number; state: string }> =>
  http.post<unknown, { id: number; state: string }>(
    `/manage/domain-replace/rules/${id}/state`,
    { state, note },
    { silent: true }
  )

/**
 * POST /manage/domain-replace/rules/:id/apply —— **执行替换（会改动业务数据）**
 * 必须逐字传入 confirmFrom/confirmTo 做二次确认；服务端只接受 vod_pic。
 */
export const applyDomainRule = (
  id: number,
  confirmFrom: string,
  confirmTo: string,
  batch = 2000,
  sleepMs = 30
): Promise<{ id: number; affected: number }> =>
  http.post<unknown, { id: number; affected: number }>(
    `/manage/domain-replace/rules/${id}/apply`,
    { confirmFrom, confirmTo, batch, sleepMs },
    { silent: true }
  )
