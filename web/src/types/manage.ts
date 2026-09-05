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
}

/** 后台影片搜索响应（{list, page}） */
export interface ManageFilmSearchResp {
  list: Card[]
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
  /** 任务类型 0=自动更新已启用站点 1=更新 ids */
  model: 0 | 1
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
}
