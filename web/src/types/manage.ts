import type { Card, Episode } from './film'
import type { PageMeta } from './api'

/** 后台影片详情的单个播放源(GET /manage/films/:mid/detail) */
export interface ManageFilmSource {
  siteId: string
  siteName: string
  playFrom: string
  master: boolean
  episodes: Episode[]
}

/** 后台影片详情影片主体(entity.Movie 子集, 仅取后台要展示的字段) */
export interface ManageMovie {
  mid: number
  name: string
  cover: string
  cName?: string
  year?: number
  area?: string
  language?: string
  director?: string
  actor?: string
  content?: string
  remarks?: string
  subTitle?: string
  state?: string
}

/** GET /manage/films/:mid/detail 响应 */
export interface ManageFilmDetailResp {
  movie: ManageMovie
  sources: ManageFilmSource[]
}

/** 按片名搜索单条命中(spider.SearchHit) */
export interface SourceSearchHit {
  sourceVodId: number
  name: string
  year: number
  typeName: string
  remarks: string
  cover: string
  episodes: number
}

/** 单源按片名搜索/采集结果(SpiderSearch / CollectFilm) */
export interface SourceFilmResult {
  sourceId: string
  sourceName: string
  master: boolean
  hits?: SourceSearchHit[]
  collected: number
  error?: string
  /**
   * 同名多版本候选: 单源精确采集(sourceId+mid/keyword)时, 若该源搜到多个同名影片
   * 无法判定采哪个, 后端返回候选列表且 collected=0(未采集), 需用户选定某版本后
   * 再以 {sourceId, vodId} 精确采集。
   */
  candidates?: SourceSearchHit[]
}

/**
 * POST /manage/spider/collect-film 入参。三种用法:
 * - 单源精确: {sourceId, mid} 或 {sourceId, keyword} → 返回单个 SourceFilmResult(可能含 candidates)
 * - 按选中版本采: {sourceId, vodId} → 直接采该 vod
 * - 多源: {keyword, sources?} → 返回 SourceFilmResult[]
 */
export interface CollectFilmPayload {
  keyword?: string
  mid?: number
  sources?: string[]
  /** 单源精确/按版本采集时的目标采集源 id */
  sourceId?: string
  /** 按选中版本采集时的源站影片 vod id(取自 SourceSearchHit.sourceVodId) */
  vodId?: number
}

/** 后台影片搜索参数（GET /manage/films, camelCase） */
export interface ManageFilmSearchParams {
  keyword?: string
  pid?: number
  cid?: number
  page?: number
  size?: number
  /** 软删态过滤：active=仅在架(默认) / deleted=回收站 / all=全部 */
  status?: ManageFilmStatus
}

/** 后台影片列表的软删态过滤值 */
export type ManageFilmStatus = 'active' | 'deleted' | 'all'

/** 后台影片列表行：公开 Card + 软删标记（deletedAt>0 即在回收站） */
export interface ManageFilmRow extends Card {
  deletedAt?: number
}

/** 后台影片搜索响应（{list, page}） */
export interface ManageFilmSearchResp {
  list: ManageFilmRow[]
  page: PageMeta
}

/** 后台手动新增影片 payload（POST /manage/films） */
export interface FilmAddPayload {
  mid?: number
  name: string
  enName?: string
  cid?: number
  pid?: number
  cName?: string
  cover?: string
  subTitle?: string
  area?: string
  language?: string
  year?: number
  director?: string
  actor?: string
  classTag?: string
  content?: string
  remarks?: string
  state?: string
}

/** 站点基础信息（对齐后端 BasicConfig） */
export interface SiteBasic {
  siteName: string
  logo: string
  keyword: string
  /**
   * 站点描述（后端 json 标签为 describe，注意不是 description）
   * 模板/SEO 用
   */
  describe: string
  domain?: string
  /** 站点开关 0/1（后端 state bool） */
  state?: boolean
  /** 站点关闭时的提示语 */
  hint?: string
}

/** 接口返回类型：0=JSON 1=XML（与后端 CollectResultModel 对齐） */
export type CollectResultModel = 0 | 1

/** 采集站等级：0=主站 1=附属（与后端 SourceGrade 对齐） */
export type SourceGrade = 0 | 1

/** 采集资源类型：0=视频 1=文章 2=演员 3=角色 4=网站（与后端 ResourceType 对齐） */
export type CollectType = 0 | 1 | 2 | 3 | 4

/** 采集源（对齐后端 FilmSource） */
export interface CollectSource {
  /** 唯一 ID（后端字符串 uuid） */
  id: string
  /** 采集站点备注名 */
  name: string
  /** 采集链接（后端字段名 uri） */
  uri: string
  /** 接口返回类型 0=json 1=xml */
  resultModel: CollectResultModel
  /** 站点等级 0=主站 1=附属 */
  grade: SourceGrade
  /** 是否同步图片 */
  syncPictures: boolean
  /** 采集资源类型 0..4 */
  collectType: CollectType
  /** 是否启用 */
  state: boolean
  /** 采集时间间隔 单位 ms */
  interval: number
  /** 仅端侧可达(服务器地域封无法访问, 如 bf): 服务端不测速/不自动停采, 测速走浏览器 */
  clientOnly?: boolean
}

/** 采集源 options（用于下拉） */
export interface CollectOption {
  id: string
  name: string
}

/** 采集执行参数（对齐后端 CollectParams） */
export interface CollectParams {
  /** 资源站 id（单个） */
  id: string
  /** 资源站 id 列表（批量时） */
  ids: string[]
  /** 采集时长（小时） */
  time: number
  /** 是否批量 */
  batch: boolean
}

/** 定时任务（对齐后端 FilmCollectTask） */
export interface CronTask {
  /** 唯一 uid */
  id: string
  /** 关联的采集站 id 列表 */
  ids: string[]
  /** 采集时长（小时） */
  time: number
  /** cron 表达式 */
  spec: string
  /** 任务类型 0=自动更新已启用站点 1=更新 ids 2=补采失败页 */
  model: 0 | 1 | 2
  /** 启用 */
  state: boolean
  /** 备注 */
  remark?: string
}

/** 影片分类 */
export interface FilmClass {
  id: number
  pid: number
  name: string
  ename?: string
  show: boolean
  sort?: number
  children?: FilmClass[]
}

/** 文件项（FileGallery 列表项） */
export interface FileItem {
  id: number | string
  name: string
  url: string
  size?: number
  type?: string
  createdAt?: number
}

/** 后端通用分页 */
export interface BackendPage {
  total: number
  current: number
  pageSize: number
  pageCount?: number
}

/** PhotoWall 响应：{ list, page } */
export interface PhotoWallResp {
  list: FileItem[]
  page: BackendPage
}

/** 仪表盘统计（GET /manage/dashboard, 后端 service.DashboardData） */
export interface DashboardStat {
  filmCount?: number
  collectCount?: number
  cronCount?: number
  todayNew?: number
  weekNew?: number
  downSources?: number
  /** 待补采的失败页数 */
  pendingFails?: number
}

/** 采集失败台账一行（GET /manage/collect-failures, 后端 entity.CollectFailure） */
export interface CollectFailure {
  id: number
  sourceId: string
  /** 失败的页码 */
  pageNo: number
  /** 发起采集时的时长参数：0=全量，>0 增量小时 */
  hours: number
  cause: string
  /** 0 待补采 / 1 已处理 */
  status: 0 | 1
  /** 重复失败次数 */
  attempts: number
  createdAt: number
}

/** 一轮补采的统计（POST /manage/collect-failures/recover 的回执里不含，仅供后续扩展） */
export interface RecoverStat {
  scanned: number
  widened: number
  replayed: number
  failed: number
  busy: number
}

/** 首页轮播 Banner（GET /manage/banners, 后端 entity.Banner） */
export interface Banner {
  id: number
  title: string
  subtitle: string
  /** 横图（宽幅主视觉） */
  image: string
  /** 竖图（窄屏/兜底） */
  poster: string
  /** 关联影片（跳详情）；0=不关联 */
  mid: number
  /** 自定义跳转（站内路径或外链）；优先于 mid */
  link: string
  /** 越小越靠前 */
  sort: number
  /** 0 启用 / 1 停用 */
  state: number
  /** 生效起（ms）；0=不限 */
  startAt: number
  /** 生效止（ms）；0=不限 */
  endAt: number
  createdAt?: number
  updatedAt?: number
}

/** 生效轮播位（GET /manage/banners/effective 的 active / 前台 GET /banners，后端 EffectiveSlide）。
 *  口径：手动配置位排前 + 热榜自动补位，共前 5 位，与首页大图完全同源。 */
export interface EffectiveSlide {
  /** banner=手动配置位 / fallback=热榜自动补位 */
  source: 'banner' | 'fallback'
  /** source=banner 时的配置 id */
  bannerId?: number
  mid?: number
  name: string
  subtitle?: string
  /** 生效中的宽幅主视觉：配置横图，或自动位的 TMDB 回填横图（空=尚未回填） */
  image?: string
  /** 竖图/封面 */
  poster?: string
  link?: string
  sort?: number
  state?: number
  /** 仅后台管理接口返回：手动位对应的完整配置行（编辑表单回填用），前台不返回 */
  banner?: Banner
}

/** 未生效配置行（不参与展示与自动补位，管理页折叠区） */
export interface InactiveBanner extends Banner {
  /** disabled=已停用 / noimage=缺横竖图 / pending=未开始 / expired=已过期 / overflow=超出前5位 */
  reason: 'disabled' | 'noimage' | 'pending' | 'expired' | 'overflow'
}

/** 后台轮播管理视图：生效位 + 未生效配置行 */
export interface BannerBoard {
  active: EffectiveSlide[]
  inactive: InactiveBanner[]
}

// ---- 用户管理 ----

/** 后台用户行(gorm.Model 序列化: ID/CreatedAt/UpdatedAt 无 json tag, 保持 Go 字段名) */
export interface ManageUserRow {
  ID: number
  userName: string
  role: number
  disabled: number
  CreatedAt: string
  UpdatedAt: string
}

export interface ManageUserListResp {
  list: ManageUserRow[]
  page: { current: number; size: number; total: number }
}
